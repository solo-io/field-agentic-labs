# Observability - Tracing Setup (kagent + AWS AgentCore)

Agentregistry Enterprise stores traces in ClickHouse and displays them in the UI under **Tracing**. This lab configures the trace path for two different runtime topologies:

```text
AWS AgentCore --> Agentregistry collector --> Agentregistry ClickHouse

kagent agent --> kagent collector --> kagent ClickHouse
                                 \-> Agentregistry collector --> Agentregistry ClickHouse
```

AWS AgentCore exports directly because it runs outside the cluster. kagent agents export to the kagent collector, which fans `traces/genai` out to both backends so both UIs retain the same execution traces.

## How Tracing Works

Four components matter:

1. **Agentregistry ClickHouse** stores trace rows in `agentregistry.otel_traces_json`.
2. **The Agentregistry telemetry collector** receives OTLP on ports `4317` (gRPC) and `4318` (HTTP).
3. **The kagent telemetry collector** receives agent traces and stores them in `platformdb.otel_traces_json`.
4. **`Runtime.spec.telemetryEndpoint`** is exported to Agentregistry-deployed workloads as the generic `OTEL_EXPORTER_OTLP_ENDPOINT`.

For kagent workloads, the kagent controller also injects `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`. The trace-specific variable takes precedence over the generic endpoint. Keep it pointed at the kagent collector, then fan out from that collector.

Use `spec.telemetryEndpoint` rather than `spec.runtimeConfig` for telemetry. `spec.runtimeConfig` is for deployment parameters such as AWS region, workdir, VPC subnets, and security groups.

## Lab Objectives

- Confirm Agentregistry ClickHouse and telemetry are enabled
- Expose the Agentregistry collector for external AWS AgentCore workloads
- Configure AWS AgentCore to export directly to Agentregistry
- Keep kagent traces flowing through the kagent collector
- Fan out kagent `traces/genai` to both ClickHouse backends
- Verify both UIs receive traces from real agent invocations

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) -> [002a](002a-setup-oidc-keycloak.md) **or** [002b](002b-setup-oidc-entra.md) -> [003](003-install-components.md)
- At least one Runtime registered: [010 (AWS)](010-aws-bedrock-runtime.md) and/or [020 (kagent)](020-kagent-runtime-and-agent.md)
- For the kagent section: kagent Enterprise installed in the `kagent` namespace with the `solo-enterprise-telemetry-collector` StatefulSet running
- `jq` and `yq` for safely deriving the fan-out configuration from the live ConfigMap

## 1. Confirm Agentregistry Telemetry

The values from [003](003-install-components.md) already include:

```yaml
clickhouse:
  enabled: true

telemetry:
  enabled: true
```

Confirm the collector and ClickHouse are running:

```bash
kubectl get pods,services -n agentregistry-system \
  -l app.kubernetes.io/instance=agentregistry-enterprise
```

For external runtimes such as AWS AgentCore, expose the collector with a `LoadBalancer`. Set the version to match the Agentregistry release you installed:

```bash
export AGENTREGISTRY_VERSION=2026.7.0

helm upgrade agentregistry-enterprise \
  oci://us-docker.pkg.dev/solo-public/agentregistry-enterprise/helm/agentregistry-enterprise \
  --version "${AGENTREGISTRY_VERSION}" \
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

The internal Agentregistry collector endpoint is:

```text
http://agentregistry-enterprise-telemetry-collector.agentregistry-system.svc.cluster.local:4318
```

## 2. Configure the AWS AgentCore Trace Path

Skip this section if you are only using kagent.

AWS AgentCore runs outside Kubernetes, so it must use the external Agentregistry collector address.

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

Existing deployments do not automatically restart when the Runtime changes. Re-apply or redeploy the AgentCore deployment so it receives the endpoint.

## 3. Configure the kagent Trace Path

Skip this section if you are only using AWS AgentCore.

### Register the Generic Runtime Telemetry Endpoint

Keep the kagent Runtime's generic telemetry endpoint pointed at Agentregistry. kagent's trace-specific endpoint, configured next, overrides this value for traces.

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

Agentregistry injects this as `OTEL_EXPORTER_OTLP_ENDPOINT` into BYO agents.

### Keep Trace Export Pointed at kagent

Check the trace-specific values injected by the kagent controller:

```bash
kubectl get configmap kagent-controller -n kagent \
  -o jsonpath='{.data.OTEL_EXPORTER_OTLP_TRACES_ENDPOINT}{"\n"}{.data.OTEL_EXPORTER_OTLP_TRACES_PROTOCOL}{"\n"}{.data.OTEL_TRACING_ENABLED}{"\n"}'
```

The endpoint must be the kagent collector:

```text
solo-enterprise-telemetry-collector.kagent.svc.cluster.local:4317
```

Persist that endpoint through the kagent Helm release. Set `KAGENT_ENT_VERSION` to the installed chart version:

```bash
export KAGENT_ENT_VERSION="<installed-kagent-enterprise-version>"

helm upgrade kagent \
  oci://us-docker.pkg.dev/solo-public/kagent-enterprise-helm/charts/kagent-enterprise \
  --version "${KAGENT_ENT_VERSION}" \
  -n kagent \
  --reuse-values \
  --set otel.tracing.enabled=true \
  --set otel.tracing.exporter.otlp.endpoint=solo-enterprise-telemetry-collector.kagent.svc.cluster.local:4317 \
  --set otel.tracing.exporter.otlp.protocol=grpc \
  --set otel.tracing.exporter.otlp.insecure=true \
  --wait --timeout 5m
```

The injected environment is applied when kagent regenerates an Agent Deployment. Force each test Agent to reconcile:

```bash
kubectl annotate agent <agent-name> -n kagent \
  tracing.agentregistry.dev/restarted-at="$(date -u +%Y%m%d%H%M%S)" --overwrite

kubectl rollout status deployment/<agent-name> -n kagent --timeout=5m
```

Verify the generated Deployment points traces to kagent, not directly to Agentregistry:

```bash
kubectl get deployment <agent-name> -n kagent \
  -o jsonpath='{range .spec.template.spec.containers[0].env[?(@.name=="OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")]}{.value}{"\n"}{end}'
```

### Fan Out kagent Traces to Agentregistry

The management chart's `traces/genai` pipeline normally exports only to `clickhouse/telemetry`. The fan-out adds a second exporter:

```yaml
exporters:
  otlp/agentregistry:
    endpoint: agentregistry-enterprise-telemetry-collector.agentregistry-system.svc.cluster.local:4317
    tls:
      insecure: true
    retry_on_failure:
      enabled: true
      initial_interval: 5s
      max_interval: 30s
      max_elapsed_time: 300s

service:
  pipelines:
    traces/genai:
      exporters:
        - clickhouse/telemetry
        - otlp/agentregistry
```

Only `traces/genai` is forwarded. Istio/mesh traces remain in kagent's backend.

The checked-in [patched ConfigMap](assets/observability/solo-enterprise-telemetry-collector-config.patched.yaml) shows the expected full result. Because the ConfigMap is Helm-managed and can change between chart releases, derive the actual patch from the live `relay` value instead of replacing the whole ConfigMap.

Save a rollback copy and generate the patched relay:

```bash
export KAGENT_OTEL_BACKUP="/tmp/solo-enterprise-telemetry-collector-config.$(date -u +%Y%m%dT%H%M%SZ).yaml"
export KAGENT_OTEL_RELAY="/tmp/solo-enterprise-telemetry-relay.yaml"
export KAGENT_OTEL_PATCHED="/tmp/solo-enterprise-telemetry-relay.patched.yaml"

kubectl get configmap solo-enterprise-telemetry-collector-config \
  -n kagent -o yaml > "${KAGENT_OTEL_BACKUP}"

kubectl get configmap solo-enterprise-telemetry-collector-config \
  -n kagent -o jsonpath='{.data.relay}' > "${KAGENT_OTEL_RELAY}"

yq '.exporters."otlp/agentregistry" = {
      "endpoint": "agentregistry-enterprise-telemetry-collector.agentregistry-system.svc.cluster.local:4317",
      "tls": {"insecure": true},
      "retry_on_failure": {
        "enabled": true,
        "initial_interval": "5s",
        "max_interval": "30s",
        "max_elapsed_time": "300s"
      }
    }
    | .service.pipelines."traces/genai".exporters
      |= (. + ["otlp/agentregistry"] | unique)' \
  "${KAGENT_OTEL_RELAY}" > "${KAGENT_OTEL_PATCHED}"
```

Inspect the two intended settings before applying:

```bash
yq '{
  "agentregistry_exporter": .exporters."otlp/agentregistry",
  "traces_genai_exporters": .service.pipelines."traces/genai".exporters
}' "${KAGENT_OTEL_PATCHED}"
```

Patch only the `relay` data key, then restart the collector:

```bash
kubectl patch configmap solo-enterprise-telemetry-collector-config \
  -n kagent --type merge \
  --patch "$(jq -Rs '{data:{relay:.}}' < "${KAGENT_OTEL_PATCHED}")"

kubectl rollout restart statefulset solo-enterprise-telemetry-collector -n kagent
kubectl rollout status statefulset solo-enterprise-telemetry-collector \
  -n kagent --timeout=5m
```

Confirm the live pipeline has both exporters:

```bash
kubectl get configmap solo-enterprise-telemetry-collector-config \
  -n kagent -o jsonpath='{.data.relay}' \
  | yq '.service.pipelines."traces/genai".exporters'
```

> The `kagent-mgmt` Helm release owns this ConfigMap. A future `helm upgrade kagent-mgmt` will restore the chart-rendered configuration and remove the extra exporter. Re-run the backup, patch, and rollout steps after an upgrade. A chart-level extra-exporter value would be the permanent solution.

## 4. Verify Both Trace Destinations

Send a real chat or tool request to a kagent Agent. Fetching only `/.well-known/agent-card.json` does not produce an execution trace.

Confirm the agent still exports through the kagent collector:

```bash
kubectl get deployment <agent-name> -n kagent -o yaml \
  | grep -A 1 OTEL_EXPORTER_OTLP_TRACES_ENDPOINT
```

Both ClickHouse timestamps should advance after the same invocation.

Agentregistry ClickHouse:

```bash
kubectl -n agentregistry-system exec agentregistry-enterprise-clickhouse-shard0-0 -- \
  clickhouse-client -q "SELECT count(), max(Timestamp) FROM agentregistry.otel_traces_json"
```

kagent ClickHouse:

```bash
kubectl -n kagent exec kagent-mgmt-clickhouse-shard0-0 -- \
  clickhouse-client -q "SELECT count(), max(Timestamp) FROM platformdb.otel_traces_json"
```

Check the fan-out collector for exporter failures:

```bash
kubectl logs statefulset/solo-enterprise-telemetry-collector \
  -n kagent --since=10m | grep -Ei 'error|failed|dropp' || true
```

For AWS AgentCore, only the Agentregistry count is expected to advance because AWS does not traverse the kagent collector.

## 5. Open the UIs

- In Agentregistry Enterprise, navigate to **Tracing**.
- In Solo Enterprise for kagent, open the agent execution trace view.

The same kagent invocation should be available in both products after collector fan-out.

## Troubleshooting

### Agentregistry grows but kagent does not

The agent is probably exporting directly to Agentregistry. Check `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`; it must point to `solo-enterprise-telemetry-collector.kagent.svc.cluster.local:4317` for fan-out to work.

### kagent grows but Agentregistry does not

- Confirm `otlp/agentregistry` exists in the live kagent collector ConfigMap.
- Confirm `traces/genai.exporters` contains both exporters.
- Check collector logs for DNS, connection, or OTLP export errors.
- Confirm the Agentregistry collector Service is reachable on port `4317`.

### Neither backend grows

- Confirm tracing is enabled in the kagent Helm values.
- Confirm the generated Agent Deployment contains the trace-specific endpoint.
- Confirm the agent image emits OpenTelemetry traces.
- Send a real chat or tool call rather than fetching the agent card.

### AWS AgentCore cannot reach the collector

AWS AgentCore cannot resolve Kubernetes service DNS names. Use the Agentregistry collector's external `LoadBalancer` address on port `4318`.

## Cleanup

Return the cluster to its pre-lab collector configuration.

Restore the live backup without replacing unrelated ConfigMap metadata:

```bash
yq -r '.data.relay' "${KAGENT_OTEL_BACKUP}" > /tmp/solo-enterprise-telemetry-relay.restore.yaml

kubectl patch configmap solo-enterprise-telemetry-collector-config \
  -n kagent --type merge \
  --patch "$(jq -Rs '{data:{relay:.}}' < /tmp/solo-enterprise-telemetry-relay.restore.yaml)"

kubectl rollout restart statefulset solo-enterprise-telemetry-collector -n kagent
kubectl rollout status statefulset solo-enterprise-telemetry-collector \
  -n kagent --timeout=5m
```

If you changed the Agentregistry collector Service to `LoadBalancer` only for this lab, restore it:

```bash
helm upgrade agentregistry-enterprise \
  oci://us-docker.pkg.dev/solo-public/agentregistry-enterprise/helm/agentregistry-enterprise \
  --version "${AGENTREGISTRY_VERSION}" \
  -n agentregistry-system \
  --reuse-values \
  --set telemetry.service.type=ClusterIP \
  --wait --timeout 5m
```

```bash
rm -f "${KAGENT_OTEL_RELAY}" "${KAGENT_OTEL_PATCHED}" \
  /tmp/solo-enterprise-telemetry-relay.restore.yaml

unset AGENTREGISTRY_VERSION OTEL_COLLECTOR_HOST OTEL_HTTP_ENDPOINT \
  KAGENT_ENT_VERSION KAGENT_OTEL_BACKUP KAGENT_OTEL_RELAY KAGENT_OTEL_PATCHED
```

## Next

- [062 - Audit Logging](062-audit-logging.md) - export Agentregistry control-plane events locally or to Splunk
