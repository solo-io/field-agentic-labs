# Observability — Tracing Setup (kagent + AWS AgentCore)

AgentRegistry Enterprise stores traces in ClickHouse and displays them in the UI under **Tracing**. Workloads send OTLP to the AgentRegistry telemetry collector, which writes to `agentregistry.otel_traces_json`.

This lab covers the setup needed for both in-cluster runtimes (kagent) and external runtimes (AWS Bedrock AgentCore). The kagent-controller traces fan-out workaround lives in [091](091-trace-fanout-workaround.md).

## How Tracing Works

Three components:

1. **ClickHouse** stores trace rows in `agentregistry.otel_traces_json`.
2. **The bundled OpenTelemetry Collector** receives OTLP on ports `4317` (gRPC) and `4318` (HTTP).
3. **`Runtime.spec.telemetryEndpoint`** is what AgentRegistry exports to deployed workloads as `OTEL_EXPORTER_OTLP_ENDPOINT`.

Use `spec.telemetryEndpoint` (not `spec.runtimeConfig`) to enable trace export for a runtime. `spec.runtimeConfig` is for deployment parameters — AWS region, workdir, VPC subnets, etc.

## Lab Objectives

- Confirm ClickHouse + telemetry are enabled in the Helm chart
- Expose the telemetry collector as a `LoadBalancer` for external runtimes (AWS AgentCore)
- Set `spec.telemetryEndpoint` on the `AWS` and `kagent` Runtimes
- Verify spans land in `agentregistry.otel_traces_json`

## Prerequisites

- [030 — AgentRegistry installed](030-install-agentregistry-helm.md) with `clickhouse.enabled: true` and `telemetry.enabled: true`
- [050](050-aws-provider.md) and/or [051](051-kagent-provider.md) — at least one Runtime registered

## 1. Enable Telemetry in the Helm Chart

The values from [030](030-install-agentregistry-helm.md) already include:

```yaml
clickhouse:
  enabled: true

telemetry:
  enabled: true
```

For **external** runtimes such as AWS AgentCore, expose the collector with a `LoadBalancer`:

```yaml
telemetry:
  enabled: true
  service:
    type: LoadBalancer
```

Apply to an existing install:

```bash
helm upgrade agentregistry-enterprise \
  oci://us-docker.pkg.dev/solo-public/agentregistry-enterprise/helm/agentregistry-enterprise \
  --version 2026.5.4 \
  -n agentregistry-system \
  --reuse-values \
  --set telemetry.service.type=LoadBalancer \
  --wait --timeout 10m
```

Get the external OTLP endpoint:

```bash
export OTEL_COLLECTOR_HOST=$(kubectl get svc agentregistry-enterprise-telemetry-collector \
  -n agentregistry-system \
  -o jsonpath='{.status.loadBalancer.ingress[0].ip}{.status.loadBalancer.ingress[0].hostname}')

export OTEL_HTTP_ENDPOINT="http://${OTEL_COLLECTOR_HOST}:4318"
echo "${OTEL_HTTP_ENDPOINT}"
```

For **in-cluster** runtimes use the internal service DNS:

```
http://agentregistry-enterprise-telemetry-collector.agentregistry-system.svc.cluster.local:4318
```

## 2. Set `spec.telemetryEndpoint` on the Runtimes

### AWS Bedrock AgentCore Runtime

AWS AgentCore runs **outside** Kubernetes, so it must use the external collector address.

```bash
export AWS_ROLE_ARN="<RoleArn from CloudFormation output>"
export AWS_EXTERNAL_ID="<ExternalId from CloudFormation output>"
export AWS_REGION="us-east-1"
export OTEL_HTTP_ENDPOINT="http://${OTEL_COLLECTOR_HOST}:4318"

cat > /tmp/aws-runtime.yaml <<EOF
apiVersion: ar.dev/v1alpha1
kind: Runtime
metadata:
  name: AWS
spec:
  type: BedrockAgentCore
  telemetryEndpoint: "${OTEL_HTTP_ENDPOINT}"
  config:
    region: "${AWS_REGION}"
    roleArn: "${AWS_ROLE_ARN}"
    externalId: "${AWS_EXTERNAL_ID}"
EOF

arctl apply -f /tmp/aws-runtime.yaml
arctl get runtime AWS -o yaml
```

Existing deployments **do not** automatically restart when the runtime changes. Re-apply or redeploy so the workload picks up the new endpoint:

```yaml
apiVersion: ar.dev/v1alpha1
kind: Deployment
metadata:
  name: demochatbot
spec:
  targetRef:
    kind: Agent
    name: demochatbot
    tag: "1.0.4"
  runtimeRef:
    kind: Runtime
    name: AWS
  runtimeConfig:
    region: us-east-1
    workdir: agentregistry-enterprise/demochatbot-a2a
```

### kagent Runtime

kagent runs in the same cluster, so use the service DNS:

```yaml
apiVersion: ar.dev/v1alpha1
kind: Runtime
metadata:
  name: kagent
spec:
  type: Kagent
  telemetryEndpoint: http://agentregistry-enterprise-telemetry-collector.agentregistry-system.svc.cluster.local:4318
  config:
    kagentUrl: http://kagent-controller.kagent.svc.cluster.local:8083
    namespace: kagent
```

AgentRegistry injects `OTEL_EXPORTER_OTLP_ENDPOINT` into kagent BYO agents from this value.

#### Repoint kagent's Injected Trace Endpoint

The kagent controller injects its own tracing env into every generated Agent Deployment from the `kagent-controller` ConfigMap. The OpenTelemetry SDK treats `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` as **higher priority** than `OTEL_EXPORTER_OTLP_ENDPOINT`, so the AgentRegistry endpoint is ignored unless you also override the kagent-injected one.

Check the current values:

```bash
kubectl get configmap kagent-controller -n kagent \
  -o jsonpath='{.data.OTEL_EXPORTER_OTLP_TRACES_ENDPOINT}{"\n"}{.data.OTEL_EXPORTER_OTLP_TRACES_PROTOCOL}{"\n"}{.data.OTEL_TRACING_ENABLED}{"\n"}'
```

If it points at kagent's collector (e.g., `solo-enterprise-telemetry-collector.kagent.svc.cluster.local:4317`), repoint it.

**Recommended — Helm values.** The kagent (enterprise) chart templates the trace env from `otel.tracing.exporter.otlp.*`:

```bash
helm upgrade kagent <chart> \
  -n kagent \
  --reuse-values \
  --set otel.tracing.enabled=true \
  --set otel.tracing.exporter.otlp.endpoint=agentregistry-enterprise-telemetry-collector.agentregistry-system.svc.cluster.local:4317 \
  --set otel.tracing.exporter.otlp.protocol=grpc \
  --set otel.tracing.exporter.otlp.insecure=true \
  --wait --timeout 5m
```

**Temporary — direct ConfigMap patch.** Use this only if you cannot run a Helm upgrade right now. Persist the same values in Helm so the next `helm upgrade` does not revert them:

```bash
kubectl patch configmap kagent-controller -n kagent --type merge -p '{
  "data": {
    "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "agentregistry-enterprise-telemetry-collector.agentregistry-system.svc.cluster.local:4317",
    "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL": "grpc",
    "OTEL_EXPORTER_OTLP_TRACES_INSECURE": "true",
    "OTEL_TRACING_ENABLED": "true"
  }
}'

kubectl rollout restart deployment/kagent-controller -n kagent
kubectl rollout status deployment/kagent-controller -n kagent --timeout=5m
```

The injected env only applies when kagent regenerates an Agent Deployment. Force each kagent Agent to reconcile so existing pods pick up the new endpoint:

```bash
kubectl annotate agent <agent-name> -n kagent \
  tracing.agentregistry.dev/restarted-at="$(date -u +%Y%m%d%H%M%S)" --overwrite

kubectl rollout status deployment/<agent-name> -n kagent --timeout=5m
```

Verify:

```bash
kubectl get deploy <agent-name> -n kagent \
  -o jsonpath='{range .spec.template.spec.containers[0].env[?(@.name=="OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")]}{.value}{"\n"}{end}'
```

> **Tip:** only real agent invocations (chats / tool calls) emit spans. Fetching `/.well-known/agent-card.json` does not, so the trace table stays empty until you send a real prompt.

> **Note:** the [091](091-trace-fanout-workaround.md) "trace fan-out" workaround is a complementary approach — instead of repointing kagent's trace exporter, you keep both backends and have the kagent collector forward `traces/genai` to the AgentRegistry collector. Pick one.

## 3. Verify the Pipeline

```bash
# Collector service + pods
kubectl get svc agentregistry-enterprise-telemetry-collector -n agentregistry-system
kubectl get pods -n agentregistry-system -l app.kubernetes.io/component=telemetry-collector

# ClickHouse tables
kubectl exec -n agentregistry-system statefulset/agentregistry-enterprise-clickhouse-shard0 -- \
  clickhouse-client --user default --password password \
  --query 'SHOW TABLES FROM agentregistry'

# Trace count (should grow as you invoke agents)
kubectl exec -n agentregistry-system statefulset/agentregistry-enterprise-clickhouse-shard0 -- \
  clickhouse-client --user default --password password \
  --query 'SELECT count() FROM agentregistry.otel_traces_json'
```

## 4. Open the UI

In AgentRegistry Enterprise, navigate to **Tracing**. Tracing access currently requires a registry admin role.

## Troubleshooting

### `otel_traces_json` exists but has zero rows

The tracing schema is there, but no workload has successfully exported traces yet. Check:

- The runtime has `spec.telemetryEndpoint` set.
- The deployment was re-applied **after** setting `spec.telemetryEndpoint`.
- The agent image actually emits OpenTelemetry traces.
- A real chat or tool call has been sent (card fetches don't emit spans).
- For kagent runtimes, `kagent-controller` injects `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`, which **overrides** `OTEL_EXPORTER_OTLP_ENDPOINT`. Repoint it (above) or apply the fan-out from [091](091-trace-fanout-workaround.md).
- External runtimes can reach the collector endpoint (security groups, NACLs).
- Collector logs don't show exporter or ClickHouse write errors.

### AWS AgentCore cannot reach the collector

AWS AgentCore cannot resolve Kubernetes service DNS names. Use the collector `LoadBalancer` endpoint `http://<external-ip-or-hostname>:4318`.

### kagent traces go to kagent instead of AgentRegistry

```bash
kubectl get deploy <agent-name> -n kagent -o yaml | grep -i OTEL
```

If `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` points to kagent's collector, traces will appear in the kagent UI rather than AgentRegistry.

### Collector is internal only

```bash
kubectl get svc agentregistry-enterprise-telemetry-collector -n agentregistry-system
```

If TYPE is `ClusterIP`, set `telemetry.service.type=LoadBalancer` for external runtime tracing.

## Next

- [091 — Trace Fan-Out Workaround](091-trace-fanout-workaround.md) — alternative to repointing kagent's exporter
