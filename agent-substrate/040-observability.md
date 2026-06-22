# Logs, Metrics, and Tracing

Substrate gives you three observability surfaces:

| Signal | How | Today | Roadmap |
|---|---|---|---|
| **Logs** | Container stdout/stderr wrapped into structured JSON; metadata labels (`ate.dev/actor_id`, `ate.dev/actor_template`, `ate.dev/actor_namespace`) injected | Live via `kubectl ate logs`; historical via any backend that indexes structured JSON | Already production-shaped |
| **Metrics** | OpenTelemetry on `ate-api` + `atelet` system services | `rpc.server.call.duration`, `http.server.request.duration`, scraped by Prometheus on kind | Actor-level metrics with `ate.dev` labels are on the roadmap |
| **Traces** | OpenTelemetry, **on-demand** via the `--trace` flag | Local: Jaeger via the kind overlay. GCP: Cloud Trace via Managed OTel | Continuous (always-on) sampling planned |

This lab covers all three on the validated paths (GKE for "real" environments + the kind overlay for local dev).

## Lab Objectives

- Stream live actor logs with `kubectl ate logs actors <id>`
- Understand the `ate.dev/*` metadata labels and how they let you correlate logs across worker pod migrations
- (On kind) Open the bundled Prometheus + Jaeger UIs
- (On GKE) Enable Cloud Trace + Managed OTel and trace a CLI call

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) → [002](002-gcp-iam-and-bucket.md) → [003](003-install-substrate.md)
- At least one running actor — easiest is the counter from [010](010-counter-demo.md)

## 1. Logs

### Live Inspection — `kubectl ate logs`

```bash
kubectl ate logs actors <actor-id>
kubectl ate logs actors <actor-id> --follow         # or -f
```

By default `kubectl ate logs` queries the Kubernetes API of the worker pod where the actor is **currently** running. If the actor is suspended, it tells you so immediately:

```bash
$ kubectl ate logs actors test
Error: actor test is not currently running on any worker pod
```

When the actor **is** running, you get clean JSON lines stripped of Substrate metadata — same shape as `kubectl logs`:

```
{"time":"2026-05-22T21:49:15.23700774Z","message":"Actor started"}
{"time":"2026-05-22T21:49:15.255765354Z","count":0,"fshash":"mCY7...","level":"INFO","msg":"Count"}
{"time":"2026-05-22T21:49:25.263744806Z","count":1,"fshash":"mCY7...","level":"INFO","msg":"Count"}
```

With `--follow`, the CLI is **actor-aware** — if the actor suspends and resumes on a different worker pod, the stream automatically re-attaches:

```
$ kubectl ate logs actors test -f
Actor is currently running on pod ate-demo-counter/counter-deployment-d8f99-m7d96
{"time":"...","count":0,...}
{"time":"...","count":1,...}
Actor is currently running on pod ate-demo-counter/counter-deployment-ab123-x4y5z
{"time":"...","count":2,...}
```

### Historical Logs — Centralized Backend

For continuous history across past worker pods and suspension cycles, route logs to a centralized backend (Google Cloud Logging, Loki, Splunk, anything that indexes structured JSON). Substrate's pipeline injects these labels on every log row:

| Label | Meaning |
|---|---|
| `ate.dev/actor_id` | The actor ID (e.g. `my-counter-1`) |
| `ate.dev/actor_template` | The `ActorTemplate` name (e.g. `counter`) |
| `ate.dev/actor_namespace` | The `ActorTemplate` namespace (e.g. `ate-demo-counter`) |

Three common queries (examples in Google Cloud Log Explorer syntax — adapt to your backend):

**Actor-centric — one actor's lifetime across many worker pods:**

```text
labels.actor_id="test"
```

**Template-centric — all actors of a kind, e.g. for error-rate analysis:**

```text
labels.actor_template="counter"
```

**Pod-centric — every actor multiplexed onto a specific worker pod:**

```text
resource.labels.pod_name="counter-deployment-c995fdf4c-m7d96"
```

The pod-centric view is what you'll use to investigate "noisy neighbor" suspicions when you scale density up.

## 2. Metrics

### Local (kind)

The kind overlay (`manifests/ate-install/kind/`) auto-provisions Prometheus in the `otel-system` namespace.

```bash
kubectl port-forward -n otel-system svc/prometheus 9090:9090
```

Open <http://localhost:9090>. Useful queries to start with:

```promql
# Is every component being scraped?
up

# RPC latencies on ate-api
rpc_server_call_duration_seconds_bucket

# HTTP traffic on the router
http_server_request_duration_seconds_bucket
```

**Status → Targets** in the UI lists the discovered pods. Storage is `emptyDir`, so metrics are lost on Prometheus pod restart.

### GKE

Set up **Google Managed Prometheus** on the cluster — see [GKE → Managed Prometheus](https://docs.cloud.google.com/stackdriver/docs/managed-prometheus). The Substrate pods expose the same OTel metrics; Managed Prometheus scrapes them automatically once enabled.

> **Actor-level metrics are on the roadmap.** Today the `rpc_*` / `http_*` series are infrastructure-level — they tell you how the control plane and router are doing, not per-actor performance. Planned OTel instrumentation will add actor labels (`ate.dev/actor_id`, etc.) so you can slice by actor / template / pool. Track upstream for progress.

## 3. Tracing

Tracing in Substrate is **on-demand**: you opt in per request with the `--trace` flag on `kubectl ate`. The CLI generates a trace ID, signals the server to trace the request, and the server propagates OpenTelemetry context across the call stack.

### Local (kind)

The kind overlay also installs an OpenTelemetry Collector and a Jaeger all-in-one in `otel-system`. No extra setup needed.

```bash
kubectl port-forward -n otel-system svc/jaeger 16686:16686 &

# Generate a trace
kubectl ate get actor my-counter-1 --trace
# or
kubectl ate suspend actor my-counter-1 --trace
```

Copy the printed Trace ID into the Jaeger search box at <http://localhost:16686>, or pick `ateapi` / `atelet` under **Service** and click **Find Traces**.

### GKE — Cloud Trace + Managed OTel

Two prerequisites:

1. **Cloud Trace API on:**

   ```bash
   gcloud services enable cloudtrace.googleapis.com --project=$PROJECT_ID
   ```

2. **Managed OpenTelemetry on the cluster:**

   ```bash
   gcloud beta container clusters update "$CLUSTER_NAME" \
     --project="$PROJECT_ID" \
     --location="$CLUSTER_LOCATION" \
     --managed-otel-scope=COLLECTION_AND_INSTRUMENTATION_COMPONENTS
   ```

Then run any command with `--trace`:

```bash
kubectl ate get actor my-counter-1 --trace
```

Copy the printed Trace ID into [Cloud Trace](https://console.cloud.google.com/traces/list) and inspect.

## What's in a Trace

A typical `ResumeActor` trace shows the full call chain:

1. CLI → `ateapi.Control/ResumeActor`
2. Control plane → Valkey (`GetActor`, `ClaimWorker`)
3. Control plane → `atelet` (RPC to assign a worker)
4. `atelet` → `ateom` (RPC inside the worker pod)
5. `ateom` → `runsc restore`
6. Status writeback to Valkey
7. Response to CLI

Spans for each, with parent/child relationships. Read [`docs/observability.md`](https://github.com/agent-substrate/substrate/blob/main/docs/observability.md) and the developer-side [`docs/dev/best-practices/tracing.md`](https://github.com/agent-substrate/substrate/blob/main/docs/dev/best-practices/tracing.md) for the full picture.

## Grafana Dashboard

The upstream repo ships an `ate-api` gRPC dashboard at `monitoring/dashboards/ate-grpc-dashboard.json`. Import it into Grafana (file → Import) and point it at your Prometheus data source. Good starting view for control-plane health.

## Cleanup

This lab is mostly read-only — `kubectl ate logs --follow`, port-forwards to Prometheus / Jaeger, and `kubectl ate ... --trace` calls don't create persistent resources. Just `Ctrl-C` any port-forwards.

If you imported the Grafana dashboard JSON in the last section, remove it through the Grafana UI (Dashboards → select → Delete) when you're done.

If you enabled Cloud Trace API + Managed OTel on GKE specifically for this lab:

```bash
# Roll back Managed OTel on the cluster
gcloud beta container clusters update "${CLUSTER_NAME}" \
  --project="${PROJECT_ID}" --location="${CLUSTER_LOCATION}" \
  --managed-otel-scope=DISABLED

# Disable the Cloud Trace API on the project (only if no other workload uses it)
gcloud services disable cloudtrace.googleapis.com --project="${PROJECT_ID}"
```

## Next

- [099 — Cleanup](099-cleanup.md)
- [appendix-benchmarking](appendix-benchmarking.md) — drive real traffic at the control plane with Locust
