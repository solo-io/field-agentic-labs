---
title: "Agent Substrate Observability Lab"
description: >
  Deploy a self-hosted Prometheus and Grafana stack, scrape Agent Substrate's
  built-in Prometheus endpoints, generate actor wake traffic, and observe
  routing latency, lifecycle activity, snapshots, and worker occupancy.
tags: [agent-substrate, kubernetes, observability, prometheus, grafana, opentelemetry]
author: Michael Levan
---

# Agent Substrate Observability Lab

This lab turns the latency measurements from the [cost comparison benchmark](050-cost-comparison-benchmark.md) into
live operational views, using a fully self-hosted **Prometheus + Grafana**
stack. Nothing in this lab depends on GKE, Cloud Monitoring, or any managed
telemetry service - it works on any Kubernetes cluster running Agent Substrate.

You will:

- Deploy Prometheus and Grafana (kube-prometheus-stack).
- Scrape Substrate's built-in Prometheus metrics endpoints with `PodMonitor`s.
- Provision a Grafana dashboard for routing, lifecycle, and snapshot signals.
- Reuse the cost benchmark's in-cluster curl client pattern.
- Generate repeated actor wake, request, and suspend cycles.
- Watch routing and restore latency percentiles live.
- Count lifecycle RPCs and inspect snapshot activity.
- Watch worker assignment and resource usage while load is running.
- Compare the dashboard p95 with the client-measured p95.

Run all commands from the root of the `substrate` repository.

## What The Current Source Supports

Substrate's services **dual-export** their metrics
(`internal/serverboot/serverboot.go` registers both readers on one meter
provider):

1. **OTLP push** - on a GKE install, to
   `opentelemetry-collector.gke-managed-otel.svc:4317` (the GKE-managed path's
   endpoint). On **kind**, the install overlay
   (`manifests/ate-install/kind/kustomization.yaml`) repoints `ate-api-server`
   and `atelet` at `opentelemetry-collector.otel-system.svc:4317`; only the
   router keeps the GKE address. Either way this lab ignores the OTLP path; a
   missing collector does not affect the Prometheus path, but the exporter
   logs recurring export errors and keeps retrying over the network.
2. **A Prometheus `/metrics` endpoint on port `9090`** of `ate-api-server`,
   `atelet`, and `atenet-router` - each pod even carries
   `prometheus.io/scrape: "true"` annotations. This is what this lab scrapes.

Envoy (the data plane of the `atenet-router` pod) is the exception: it serves
Prometheus metrics on its admin port `9901` at `/stats/prometheus`.

Because the Prometheus exporter names differ from the OTLP names (dots become
underscores, a unit suffix is appended), the table below lists the names **as
scraped** - verified against a live cluster:

| Goal | Metric (Prometheus name) | Important interpretation |
|---|---|---|
| Wake-path p95 | `atenet_router_route_duration_seconds` (labels `actor_template_namespace`, `actor_template_name`, `outcome`) | Router duration ends when the worker endpoint is resolved. It excludes actor execution and the response. Seconds. |
| Full request p95 | `envoy_http_downstream_rq_time` (label `envoy_http_conn_manager_prefix="ingress_http"`) | Includes routing, actor execution, and the response. **Milliseconds.** Always filter to `ingress_http` - the `admin` conn manager's own stats dominate otherwise. |
| Resume/suspend activity | `rpc_server_call_duration_seconds_count` (labels `rpc_method`, `rpc_response_status_code`) | `rpc_method` carries the full path, e.g. `ateapi.Control/ResumeActor`, `atelet.AteomHerder/Restore`. Counts completed RPC attempts, not guaranteed state transitions. |
| Snapshot activity | `atelet_snapshot_size_bytes` and atelet `Checkpoint` RPCs | `atelet_snapshot_size_bytes` is recorded **once per snapshot file** (label `kind`: pages, state, config, ...), *before* the file's move/upload succeeds - so its `_count` is "snapshot files observed", not completed snapshots. Use successful `atelet.AteomHerder/Checkpoint` RPCs for operations/sec, and group the size histogram by `kind`. |
| Worker utilization | `kubectl ate get workers` and `kubectl top` | Current Substrate releases do not emit a worker-pool occupancy gauge. This lab calculates assigned workers live. |

Because the `rpc_method` values fully disambiguate the sender
(`ateapi.Control/*` vs `atelet.AteomHerder/*`), none of the queries in this
lab need `job` filters.

Two related things in the repo that this lab does **not** use:

- `benchmarking/monitoring.yaml` provisions a small Prometheus/Grafana stack,
  but it only scrapes **Locust client metrics** in the `benchmarking`
  namespace - not the Substrate service metrics above. Don't confuse it with
  this lab's stack.
- `tools/setup-gcp/dashboards/` contains **Google Cloud Monitoring** dashboard
  JSON for the GKE-managed path. Those files cannot be imported into Grafana;
  this lab provisions its own Grafana dashboard instead.

## Prerequisites

You need:

- Baseline setup complete: [001](001-baseline-setup.md) -> [002](002-gcp-iam-and-bucket.md) -> [003](003-install-substrate.md).
- Agent Substrate and the [counter demo](010-counter-demo.md) installed.
- `kubectl`, `kubectl-ate`, `helm` (>= 3.15), and Go installed.
- Cluster capacity for the monitoring stack (roughly 1 CPU / 1.5 GiB across
  Prometheus, Grafana, operator, and exporters).
- `metrics-server` if you want the optional `kubectl top` view.

Source the environment used to install Substrate:

```bash
source .ate-dev-env.sh
```

If the counter demo is not yet deployed (no `ate-demo-counter` namespace on
the cluster), deploy it and install the CLI from the substrate repo root -
this is the same flow as the [counter demo](010-counter-demo.md):

```bash
./hack/install-ate.sh --deploy-demo-counter
go install ./cmd/kubectl-ate
```

On a **kind** cluster, use the kind wrapper instead - it sets the local
registry, the Kustomize overlay, and the in-cluster rustfs bucket:

```bash
./hack/install-ate-kind.sh --deploy-demo-counter
```

Confirm the demo resources exist:

```bash
kubectl get actortemplate,workerpool -n ate-demo-counter
```

The `counter` ActorTemplate must be `Ready` before continuing:

```bash
kubectl wait --for=condition=Ready actortemplate/counter \
  -n ate-demo-counter \
  --timeout=5m
```

## Step 1: Deploy Prometheus And Grafana

Install kube-prometheus-stack into a dedicated `monitoring` namespace.
Alertmanager is disabled (unused by this lab), and the two
`*SelectorNilUsesHelmValues=false` settings make Prometheus pick up **any**
`PodMonitor`/`ServiceMonitor` in the cluster rather than only ones labeled
with the Helm release - that's what lets Step 2's plain PodMonitors work:

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update prometheus-community

helm upgrade --install monitoring \
  prometheus-community/kube-prometheus-stack \
  --namespace monitoring --create-namespace \
  --set alertmanager.enabled=false \
  --set prometheus.prometheusSpec.podMonitorSelectorNilUsesHelmValues=false \
  --set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false
```

Verify the stack is healthy:

```bash
kubectl get pods -n monitoring
```

Expect the operator, `prometheus-monitoring-kube-prometheus-prometheus-0`,
Grafana, kube-state-metrics, and a node-exporter per node, all `Running`.

## Step 2: Scrape Substrate And Envoy

Two `PodMonitor`s cover everything. The first targets the declared port
`9090` on all three Substrate services; the second targets Envoy's admin port
`9901` at `/stats/prometheus`. (`portNumber` requires the port to be declared
on the container - all four are.)

```bash
kubectl apply -f - <<'EOF'
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  name: substrate-services
  namespace: monitoring
spec:
  namespaceSelector:
    matchNames: [ate-system]
  selector:
    matchExpressions:
    - key: app
      operator: In
      values: [ate-api-server, atelet, atenet-router]
  podMetricsEndpoints:
  - portNumber: 9090
    path: /metrics
    interval: 15s
---
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  name: substrate-envoy
  namespace: monitoring
spec:
  namespaceSelector:
    matchNames: [ate-system]
  selector:
    matchLabels:
      app: atenet-router
  podMetricsEndpoints:
  - portNumber: 9901
    path: /stats/prometheus
    interval: 15s
EOF
```

Port-forward Prometheus and confirm the targets are up:

```bash
kubectl -n monitoring port-forward svc/monitoring-kube-prometheus-prometheus 9090:9090
```

Open [http://localhost:9090/targets](http://localhost:9090/targets).

Paste the following and you will see calls to Substrate:

```promql
rpc_server_call_duration_seconds_count
```

## Step 3: Provision The Grafana Dashboard

Grafana's dashboard sidecar (enabled by default in the chart) loads any
ConfigMap labeled `grafana_dashboard: "1"`. Provision the lab dashboard:

```bash
kubectl apply -f - <<'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: substrate-observability-dashboard
  namespace: monitoring
  labels:
    grafana_dashboard: "1"
data:
  substrate-observability.json: |
    {
      "title": "Agent Substrate Observability",
      "uid": "substrate-obs",
      "timezone": "utc",
      "refresh": "30s",
      "time": { "from": "now-30m", "to": "now" },
      "panels": [
        { "type": "row", "title": "Routing", "gridPos": {"h": 1, "w": 24, "x": 0, "y": 0} },
        {
          "type": "timeseries", "title": "Routing latency (wake path) p50 / p95 / p99",
          "gridPos": {"h": 8, "w": 12, "x": 0, "y": 1},
          "datasource": { "type": "prometheus", "uid": "prometheus" },
          "fieldConfig": { "defaults": { "unit": "s" }, "overrides": [] },
          "targets": [
            { "refId": "A", "datasource": { "type": "prometheus", "uid": "prometheus" }, "expr": "histogram_quantile(0.50, sum by (le) (rate(atenet_router_route_duration_seconds_bucket{actor_template_namespace=\"ate-demo-counter\", actor_template_name=\"counter\", outcome=\"ok\"}[5m])))", "legendFormat": "p50" },
            { "refId": "B", "datasource": { "type": "prometheus", "uid": "prometheus" }, "expr": "histogram_quantile(0.95, sum by (le) (rate(atenet_router_route_duration_seconds_bucket{actor_template_namespace=\"ate-demo-counter\", actor_template_name=\"counter\", outcome=\"ok\"}[5m])))", "legendFormat": "p95" },
            { "refId": "C", "datasource": { "type": "prometheus", "uid": "prometheus" }, "expr": "histogram_quantile(0.99, sum by (le) (rate(atenet_router_route_duration_seconds_bucket{actor_template_namespace=\"ate-demo-counter\", actor_template_name=\"counter\", outcome=\"ok\"}[5m])))", "legendFormat": "p99" }
          ]
        },
        {
          "type": "timeseries", "title": "Routing outcomes (platform-wide, req/s)",
          "gridPos": {"h": 8, "w": 6, "x": 12, "y": 1},
          "datasource": { "type": "prometheus", "uid": "prometheus" },
          "fieldConfig": { "defaults": { "unit": "reqps" }, "overrides": [] },
          "targets": [
            { "refId": "A", "datasource": { "type": "prometheus", "uid": "prometheus" }, "expr": "sum by (outcome) (rate(atenet_router_route_duration_seconds_count[5m]))", "legendFormat": "{{outcome}}" }
          ]
        },
        {
          "type": "timeseries", "title": "Envoy full request p95 (includes actor time)",
          "gridPos": {"h": 8, "w": 6, "x": 18, "y": 1},
          "datasource": { "type": "prometheus", "uid": "prometheus" },
          "fieldConfig": { "defaults": { "unit": "ms" }, "overrides": [] },
          "targets": [
            { "refId": "A", "datasource": { "type": "prometheus", "uid": "prometheus" }, "expr": "histogram_quantile(0.95, sum by (le) (rate(envoy_http_downstream_rq_time_bucket{envoy_http_conn_manager_prefix=\"ingress_http\"}[5m])))", "legendFormat": "p95" }
          ]
        },
        { "type": "row", "title": "Actor Lifecycle", "gridPos": {"h": 1, "w": 24, "x": 0, "y": 9} },
        {
          "type": "timeseries", "title": "Control-plane lifecycle RPCs (ok, ops/s)",
          "gridPos": {"h": 8, "w": 8, "x": 0, "y": 10},
          "datasource": { "type": "prometheus", "uid": "prometheus" },
          "fieldConfig": { "defaults": { "unit": "ops" }, "overrides": [] },
          "targets": [
            { "refId": "A", "datasource": { "type": "prometheus", "uid": "prometheus" }, "expr": "sum by (rpc_method) (rate(rpc_server_call_duration_seconds_count{rpc_method=~\"ateapi.Control/(Create|Resume|Suspend|Pause|Delete)Actor\", rpc_response_status_code=\"OK\"}[5m]))", "legendFormat": "{{rpc_method}}" }
          ]
        },
        {
          "type": "timeseries", "title": "Worker restore / checkpoint (ok, ops/s)",
          "gridPos": {"h": 8, "w": 8, "x": 8, "y": 10},
          "datasource": { "type": "prometheus", "uid": "prometheus" },
          "fieldConfig": { "defaults": { "unit": "ops" }, "overrides": [] },
          "targets": [
            { "refId": "A", "datasource": { "type": "prometheus", "uid": "prometheus" }, "expr": "sum by (rpc_method) (rate(rpc_server_call_duration_seconds_count{rpc_method=~\"atelet.AteomHerder/(Restore|Checkpoint)\", rpc_response_status_code=\"OK\"}[5m]))", "legendFormat": "{{rpc_method}}" }
          ]
        },
        {
          "type": "timeseries", "title": "Restore latency p95",
          "gridPos": {"h": 8, "w": 8, "x": 16, "y": 10},
          "datasource": { "type": "prometheus", "uid": "prometheus" },
          "fieldConfig": { "defaults": { "unit": "s" }, "overrides": [] },
          "targets": [
            { "refId": "A", "datasource": { "type": "prometheus", "uid": "prometheus" }, "expr": "histogram_quantile(0.95, sum by (le) (rate(rpc_server_call_duration_seconds_bucket{rpc_method=\"atelet.AteomHerder/Restore\", rpc_response_status_code=\"OK\"}[5m])))", "legendFormat": "p95" }
          ]
        },
        {
          "type": "timeseries", "title": "gRPC non-OK responses (ops/s)",
          "gridPos": {"h": 8, "w": 8, "x": 0, "y": 18},
          "datasource": { "type": "prometheus", "uid": "prometheus" },
          "fieldConfig": { "defaults": { "unit": "ops" }, "overrides": [] },
          "targets": [
            { "refId": "A", "datasource": { "type": "prometheus", "uid": "prometheus" }, "expr": "sum by (rpc_method, rpc_response_status_code) (rate(rpc_server_call_duration_seconds_count{rpc_response_status_code!=\"OK\"}[5m]))", "legendFormat": "{{rpc_method}} {{rpc_response_status_code}}" }
          ]
        },
        { "type": "row", "title": "Snapshots", "gridPos": {"h": 1, "w": 24, "x": 0, "y": 26} },
        {
          "type": "timeseries", "title": "Snapshot file size p95 (by kind)",
          "gridPos": {"h": 8, "w": 12, "x": 0, "y": 27},
          "datasource": { "type": "prometheus", "uid": "prometheus" },
          "fieldConfig": { "defaults": { "unit": "bytes" }, "overrides": [] },
          "targets": [
            { "refId": "A", "datasource": { "type": "prometheus", "uid": "prometheus" }, "expr": "histogram_quantile(0.95, sum by (le, kind) (rate(atelet_snapshot_size_bytes_bucket{actor_template_namespace=\"ate-demo-counter\", actor_template_name=\"counter\"}[5m])))", "legendFormat": "{{kind}} p95" }
          ]
        },
        {
          "type": "timeseries", "title": "Checkpoints completed vs snapshot files observed (platform-wide, ops/s)",
          "gridPos": {"h": 8, "w": 12, "x": 12, "y": 27},
          "datasource": { "type": "prometheus", "uid": "prometheus" },
          "fieldConfig": { "defaults": { "unit": "ops" }, "overrides": [] },
          "targets": [
            { "refId": "A", "datasource": { "type": "prometheus", "uid": "prometheus" }, "expr": "sum(rate(rpc_server_call_duration_seconds_count{rpc_method=\"atelet.AteomHerder/Checkpoint\", rpc_response_status_code=\"OK\"}[5m]))", "legendFormat": "checkpoints/s" },
            { "refId": "B", "datasource": { "type": "prometheus", "uid": "prometheus" }, "expr": "sum(rate(atelet_snapshot_size_bytes_count[5m]))", "legendFormat": "snapshot files/s (recorded pre-upload)" }
          ]
        }
      ],
      "schemaVersion": 39
    }
EOF
```

Port-forward Grafana:

```bash
kubectl -n monitoring port-forward svc/monitoring-grafana 3000:80
```

Open the Grafana dashboard: [http://localhost:3000](http://localhost:3000).

The login is `admin` and the chart-managed password:

```bash
kubectl get secret monitoring-grafana -n monitoring \
  -o jsonpath='{.data.admin-password}' | base64 -d; echo
```

You'll find the **Agent Substrate Observability** dashboard under Grafana's dashboards.

Sidenote: Most panels stay empty until traffic flows, which is Step 5. In
particular, the percentile panels (routing latency, Envoy p95, restore p95)
show **no line at all** at zero traffic: `histogram_quantile` over a zero
rate returns `NaN`, which Grafana renders as empty. Only the counter-rate
panels draw a flat zero line while idle. Use the dashboard's 30-minute time
range during the run - a 24-hour window compresses the ten-minute load run
into a sliver.

Scoping note - the panels deliberately mix two scopes:

- **Template-scoped to `ate-demo-counter/counter`**: routing latency and
  snapshot file size, so they aggregate the same population as the Step 5
  load generator and the client results captured for the Results Template.
- **Platform-wide, on purpose**: *Routing outcomes* - template labels are
  only populated once `ResumeActor` succeeds
  (`cmd/atenet/internal/router/extproc.go`), so invalid hosts, missing
  actors, capacity failures, and resume errors all carry **empty** template
  labels; scoping this panel would hide exactly the failures it exists to
  show. The *checkpoints vs snapshot files* panel is also fully platform-wide:
  the checkpoint RPC metric carries no template identity, so its files-side
  series is left unscoped too - otherwise unrelated checkpoints would distort
  the apparent files-per-checkpoint ratio. The gRPC panels are platform-wide
  by the same necessity.

## Step 4: Prepare The Observable Workload

This uses the same temporary curl client design as the cost comparison
benchmark, but gives the observability run its own Kubernetes namespace and
atespace.

```bash
export ACTOR_COUNT=20
export OBS_NAMESPACE=observability
export OBS_ATESPACE=observability
export OBS_ACTOR_PREFIX=obs-counter
export TEMPLATE_REF=ate-demo-counter/counter
export WORKER_NAMESPACE=ate-demo-counter
export SUBSTRATE_ROUTER_URL=http://atenet-router.ate-system.svc:80
export OBS_DURATION_SECONDS=600
export OBS_RESULTS_FILE=observability-wake-results.tsv
```

Create the namespace and benchmark client:

```bash
kubectl create namespace "$OBS_NAMESPACE" \
  --dry-run=client -o yaml | kubectl apply -f -
```

```bash
kubectl delete pod benchmark-client \
  -n "$OBS_NAMESPACE" \
  --ignore-not-found

kubectl run benchmark-client \
  -n "$OBS_NAMESPACE" \
  --image=curlimages/curl:8.10.1 \
  --restart=Never \
  --command -- sleep 86400

kubectl wait --for=condition=Ready pod/benchmark-client \
  -n "$OBS_NAMESPACE" \
  --timeout=2m
```

Create the atespace if it does not already exist:

```bash
if ! kubectl ate get atespace "$OBS_ATESPACE" >/dev/null 2>&1; then
  kubectl ate create atespace "$OBS_ATESPACE"
fi
```

Create the actors:

```bash
for i in $(seq 1 "$ACTOR_COUNT"); do
  actor=$(printf '%s-%03d' "$OBS_ACTOR_PREFIX" "$i")

  if ! kubectl ate get actor "$actor" \
    --atespace "$OBS_ATESPACE" >/dev/null 2>&1; then
    kubectl ate create actor "$actor" \
      --template "$TEMPLATE_REF" \
      --atespace "$OBS_ATESPACE"
  fi
done
```

Confirm that the actors begin suspended:

```bash
kubectl ate get actors --atespace "$OBS_ATESPACE"
kubectl ate get workers
```

## Step 5: Generate Traffic And Watch The Live Views

Keep the **Agent Substrate Observability** Grafana dashboard open with
30-second auto-refresh and a 30-minute window. The panels to narrate during the
run are:

- `Routing latency (wake path) p50 / p95 / p99`
- `Envoy full request p95` - the number a client actually feels
- `Control-plane lifecycle RPCs` and `Worker restore / checkpoint`
- `Checkpoints completed vs snapshot files observed`

The load generator below is what populates those panels. Run it from the same
shell where you ran the Step 4 export block so the Bash child inherits the lab
variables.

The client pod runs a finite `sleep`. Recreate it if it is no longer `Running`:

```bash
if [ "$(kubectl get pod benchmark-client -n "$OBS_NAMESPACE" \
    -o jsonpath='{.status.phase}' 2>/dev/null)" != "Running" ]; then
  kubectl delete pod benchmark-client -n "$OBS_NAMESPACE" --ignore-not-found
  kubectl run benchmark-client \
    -n "$OBS_NAMESPACE" \
    --image=curlimages/curl:8.10.1 \
    --restart=Never \
    --command -- sleep 86400
  kubectl wait --for=condition=Ready pod/benchmark-client \
    -n "$OBS_NAMESPACE" --timeout=2m
fi
```

Before starting the load, **optionally** open another shell and run the watcher
below. This shell does not inherit the Step 4 variables, so set the worker
namespace explicitly. Leave the watcher running, then return to the Step 4
shell to start the load generator.

This is occupancy, meaning workers currently assigned to actors. It is distinct
from CPU utilization. Stop the watcher with `Ctrl-C` after the load run.

```bash
export WORKER_NAMESPACE=ate-demo-counter

while true; do
  clear
  date -u

  workers=$(kubectl ate get workers)
  printf '%s\n' "$workers"
  printf '%s\n' "$workers" | awk '
    NR > 1 {
      total++
      if ($4 == "ASSIGNED") assigned++
    }
    END {
      if (total > 0) {
        printf "\nworker_occupancy=%d/%d (%.1f%%)\n",
          assigned, total, (assigned / total) * 100
      }
    }'

  printf '\nWorker pod resources:\n'
  kubectl top pods -n "$WORKER_NAMESPACE" 2>/dev/null || true
  sleep 2
done
```

Run the load as one Bash heredoc. Each actor is verified as suspended before
and after its request, so every numeric sample represents a suspended wake. A
failed request is recorded as `FAILED`; because the router may continue a
resume for up to 15 seconds after its caller disconnects, the script waits out
that detached operation before suspending the actor.

```bash
bash <<'BASH'
# Do not enable `set -e`: expected command failures are handled explicitly.
set -u

: "${OBS_RESULTS_FILE:?run the Step 4 export block first}"
: "${OBS_DURATION_SECONDS:?run the Step 4 export block first}"
: "${ACTOR_COUNT:?run the Step 4 export block first}"
: "${OBS_ACTOR_PREFIX:?run the Step 4 export block first}"
: "${OBS_ATESPACE:?run the Step 4 export block first}"
: "${OBS_NAMESPACE:?run the Step 4 export block first}"
: "${SUBSTRATE_ROUTER_URL:?run the Step 4 export block first}"

get_actor_status() {
  kubectl ate get actor "$1" --atespace "$OBS_ATESPACE" 2>/dev/null |
    awk 'NR == 2 { print $4 }'
}

ensure_suspended() {
  local actor=$1
  local actor_status
  local attempt

  for attempt in 1 2 3 4 5 6 7 8 9 10; do
    actor_status=$(get_actor_status "$actor")
    if [ "$actor_status" = "STATUS_SUSPENDED" ]; then
      return 0
    fi
    if [ "$actor_status" = "STATUS_CRASHED" ]; then
      printf 'ERROR: %s is CRASHED and cannot be measured\n' "$actor" >&2
      return 1
    fi

    kubectl ate suspend actor "$actor" \
      --atespace "$OBS_ATESPACE" >/dev/null 2>&1 || true
    sleep 3
  done

  actor_status=$(get_actor_status "$actor")
  printf 'ERROR: %s did not reach STATUS_SUSPENDED (status=%s)\n' \
    "$actor" "${actor_status:-unknown}" >&2
  return 1
}

printf 'round\tactor\twake_seconds\n' > "$OBS_RESULTS_FILE"

deadline=$((SECONDS + OBS_DURATION_SECONDS))
round=0
successful_wakes=0
failed_wakes=0

while (( SECONDS < deadline )); do
  round=$((round + 1))

  for i in $(seq 1 "$ACTOR_COUNT"); do
    if (( SECONDS >= deadline )); then
      break
    fi

    actor=$(printf '%s-%03d' "$OBS_ACTOR_PREFIX" "$i")
    actor_host="${actor}.${OBS_ATESPACE}.actors.resources.substrate.ate.dev"

    if ! ensure_suspended "$actor"; then
      exit 1
    fi

    if wake_seconds=$(kubectl exec \
      -n "$OBS_NAMESPACE" benchmark-client -- \
      curl -sS --fail-with-body --max-time 60 -o /dev/null -w '%{time_total}' \
      -X POST \
      -H "Host: ${actor_host}" \
      "$SUBSTRATE_ROUTER_URL"); then
      printf '%s\t%s\t%s\n' \
        "$round" "$actor" "$wake_seconds" | tee -a "$OBS_RESULTS_FILE"
      successful_wakes=$((successful_wakes + 1))
    else
      printf '%s\t%s\tFAILED\n' \
        "$round" "$actor" | tee -a "$OBS_RESULTS_FILE"
      failed_wakes=$((failed_wakes + 1))
      sleep 16
    fi

    if ! ensure_suspended "$actor"; then
      printf 'Aborting to prevent a warm request from entering the wake results.\n' >&2
      exit 1
    fi
  done
done

printf 'successful_wakes=%d\nfailed_wakes=%d\n' \
  "$successful_wakes" "$failed_wakes"
BASH
```

Each successful cycle should produce:

- One routed request and one `ResumeActor` RPC.
- One atelet `Restore` when the actor was actually suspended.
- One explicit `SuspendActor` RPC.
- One atelet `Checkpoint` when suspension writes a snapshot.

The exact RPC counts can differ because the metrics count RPC attempts and the
control plane may retry or handle an operation idempotently.

## Optional: Actor-Emitted Telemetry And Traces

Everything above observes the Substrate **platform**. Two extensions are out
of scope for this lab but worth knowing:

- **Application metrics/traces emitted inside actors.** Deploy an
  OpenTelemetry Collector as a stable Service, and configure actor workloads
  (via their ActorTemplate) to export OTLP to it. One critical caveat: actor
  identity must come from the bind-mounted file **`/run/ate/actor-id`** -
  with two limitations of its own. It exists only for **gVisor** actors (the
  micro-VM runtime drops the mount - a documented `KNOWN GAP` in
  `cmd/ateom-microvm/spec.go`; the guest cannot see host paths). And it
  contains only the actor **name**, which is unique only within an atespace -
  globally useful telemetry identity needs at least `(atespace, name)`, which
  the mount does not currently provide. Never take identity from an
  environment variable like
  `OTEL_RESOURCE_ATTRIBUTES=actor.id=...`. Env vars are resolved before the
  golden snapshot and frozen into checkpointed memory, so every restored actor
  would report the *golden* actor's identity. Note that this cuts against
  standard OTel usage: SDKs normally initialize resource attributes **once at
  startup**, which is exactly the state a snapshot freezes. A correct
  implementation needs migration-aware enrichment **inside the sandbox** -
  re-read the file per emission, or rebuild the tracer/meter providers after
  each resume. Collector-side stamping is not a shortcut: a centralized
  collector cannot see `/run/ate/actor-id`, so it can only attach identity the
  workload already sent, or resolve the sending worker's IP to an actor via
  the control plane at ingest time - a lookup that is racy across
  suspend/resume precisely because workers are reused by different actors.
- **Platform traces.** Substrate's own traces are OTLP-push only (there is no
  Prometheus equivalent for traces). Capturing them self-hosted requires an
  OTel Collector plus a trace backend (Jaeger or Tempo) - and the senders are
  configured in **three different places**:
  - the Go services (`ate-api-server`, `atelet`, `atenet-router`) use the
    `OTEL_EXPORTER_OTLP_ENDPOINT` env var on their Deployments/DaemonSet;
  - **Envoy's** spans are configured separately through the router's
    `--otlp-collector-address` flag
    (`manifests/ate-install/atenet-router.yaml`) - the router provisions
    Envoy's tracer via xDS, and an empty value disables Envoy tracing;
  - **ateom worker pods receive no OTLP configuration at all** - the
    WorkerPool controller injects only `POD_UID`
    (`workerpool_apply.go`), so worker-side spans are not part of the
    current story.

  The `kubectl ate ... --trace` flag emits the trace ID to correlate against
  whichever backend receives the spans.

Known gaps in current platform metrics (upstream roadmap items): no worker
occupancy gauge, no actor-state counts, and no per-actor labels on the gRPC or
restore metrics. Actor-level correlation across worker migrations is limited
to logs and the router's trace spans today - the spans do carry atespace and
actor name, but trace continuity across components is incomplete.

## Results Template

| Observation | Value |
|---|---:|
| Client wake p95 | `<seconds>` |
| Router wake-path p95 | `<seconds>` |
| Atelet Restore p95 | `<seconds>` |
| Envoy full-request p95 | `<milliseconds>` |
| Successful ResumeActor RPCs | `<count>` |
| Successful Restore RPCs | `<count>` |
| Successful SuspendActor RPCs | `<count>` |
| Successful Checkpoint RPCs | `<count>` |
| Peak assigned workers | `<assigned>/<total>` |
| Peak worker CPU/memory | `<kubectl top observation>` |

The useful demo-day story is the correlation across views: request traffic
rises, suspended actors restore, workers become assigned, full-request latency
shows the cold-start cost, checkpoints appear after suspension, and workers
return to `FREE` without deleting the logical actors.

## Cleanup

> **Ownership warning.** Cleanup deletes the lab's resources
> **unconditionally**: the `obs-counter-*` actors, the `observability`
> atespace and namespace, and (optionally) the `monitoring` Helm release and
> namespace. These names are treated as lab-owned - do not point the lab at
> pre-existing resources under the same names on a shared cluster, or cleanup
> will remove them.

Delete the actors created by this lab. Both commands tolerate per-actor
failure so one crashed actor (deletable only once suspended) doesn't block
cleanup of the rest:

```bash
for i in $(seq 1 "$ACTOR_COUNT"); do
  actor=$(printf '%s-%03d' "$OBS_ACTOR_PREFIX" "$i")
  kubectl ate suspend actor "$actor" \
    --atespace "$OBS_ATESPACE" >/dev/null 2>&1 || true
  kubectl ate delete actor "$actor" \
    --atespace "$OBS_ATESPACE" || true
done

kubectl ate get actors --atespace "$OBS_ATESPACE"
kubectl ate delete atespace "$OBS_ATESPACE"
kubectl delete namespace "$OBS_NAMESPACE"
rm -f "$OBS_RESULTS_FILE"
```

Remove the monitoring stack if you are done with it (the PodMonitors and
dashboard ConfigMap live in the `monitoring` namespace, so they go with it):

```bash
helm uninstall monitoring -n monitoring
kubectl delete namespace monitoring
```

## Troubleshooting

- **PodMonitor targets missing from Status > Targets**: confirm the two
  `*SelectorNilUsesHelmValues=false` Helm values from Step 1 - without them
  Prometheus only selects release-labeled monitors. Then check
  `kubectl get podmonitor -n monitoring`.
- **Targets shown but DOWN**: the Substrate pods must be Running in
  `ate-system`; `portNumber` only works for ports declared on the container
  (9090 and 9901 are declared by the stock manifests - a fork that removed
  them would need `targetPort`-era manifests updated).
- **PodMonitor rejected with an unknown-field error for `portNumber`**: the
  cluster carries prometheus-operator CRDs older than this chart - `helm
  upgrade` does **not** upgrade pre-existing CRDs. Update them from the
  chart's `charts/crds/` (or a matching prometheus-operator release) before
  re-applying.
- **`atenet_router_route_duration_seconds` returns no series**: no request
  has passed through the router since it started. OTel instruments export
  only after first use - send one actor request and re-query.
- **Envoy p95 looks absurdly low or flat**: you forgot the
  `envoy_http_conn_manager_prefix="ingress_http"` filter and are measuring
  Envoy's admin endpoint serving Prometheus scrapes.
- **Dashboard missing in Grafana**: the sidecar needs the
  `grafana_dashboard: "1"` label on the ConfigMap and up to a minute to sync;
  check `kubectl logs -n monitoring deploy/monitoring-grafana -c grafana-sc-dashboard`.
- **Lifecycle counts do not equal curl samples**: RPC metrics count attempts.
  Retries, idempotent resume calls, failures, and scrape timing can all create
  differences. Compare `Restore` with actual wakes and group by
  `rpc_response_status_code` when investigating.
- **Worker occupancy is always zero**: sequential counter requests can complete
  between two-second samples. Reduce the watcher sleep interval for a more
  granular view.
- **`kubectl top` fails**: `metrics-server` is missing or not ready. Worker
  assignment, Prometheus metrics, and dashboard steps still work.
