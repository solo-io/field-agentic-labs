# Trace Fan-Out Workaround for kagent

## Problem

The agentregistry **Dashboard** (Agent Runs / Operations / Token Usage) and **Tracing** page show **No Data**, even though kagent agents are running. Root cause: kagent-managed agents split telemetry across two backends.

| Signal | Env var on the agent | Destination |
|---|---|---|
| logs / metrics | `OTEL_EXPORTER_OTLP_ENDPOINT` | `agentregistry-enterprise-telemetry-collector` (`agentregistry-system`) ✅ |
| **traces** | `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | **`solo-enterprise-telemetry-collector` (`kagent`)** ❌ |

So traces only ever reach the **kagent** ClickHouse (`platformdb.otel_traces_json`), never the **agentregistry** ClickHouse (`agentregistry.otel_traces_json`) that the agentregistry dashboard reads. The agents' trace endpoint is set by the Helm value `otel.tracing.exporter.otlp.endpoint` on the `kagent` (kagent-enterprise) release.

There are two ways to fix this:

1. **Repoint kagent's trace exporter** at the agentregistry collector - see [060](060-observability-tracing.md#repoint-kagents-injected-trace-endpoint). Trade-off: kagent's UI loses the traces.
2. **Fan out** - keep kagent's exporter pointed at its own collector, but have **that** collector forward `traces/genai` to the agentregistry collector as well. Both backends see the traces; both UIs work.

This lab is the fan-out option.

## Lab Objectives

- Apply the patched `solo-enterprise-telemetry-collector-config` ConfigMap that adds an OTLP exporter targeting the agentregistry collector
- Restart the kagent collector StatefulSet so it picks up the new pipeline
- Verify both ClickHouse databases receive trace rows

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) → [002a](002a-setup-oidc-keycloak.md) **or** [002b](002b-setup-oidc-entra.md) → [003](003-install-components.md)
- An existing kagent install with the `solo-enterprise-telemetry-collector` StatefulSet in the `kagent` namespace
- [020 - kagent Runtime registered](020-kagent-runtime-and-agent.md)

## How the Patch Works

Inside the kagent collector ConfigMap, add a new `otlp/agentregistry` exporter and wire it onto the existing `traces/genai` pipeline alongside the original `clickhouse/telemetry` exporter:

```yaml
exporters:
 otlp/agentregistry: # ADDED
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
 - clickhouse/telemetry # existing - kagent UI
 - otlp/agentregistry # ADDED - agentregistry dashboard
```

Only `traces/genai` is forwarded (not `traces/istio`) so the agentregistry dashboard isn't polluted with mesh spans.

## Apply the Fan-Out

The patched and the original ConfigMaps are checked in as assets:

- [`assets/observability/solo-enterprise-telemetry-collector-config.patched.yaml`](assets/observability/solo-enterprise-telemetry-collector-config.patched.yaml) - the applied ConfigMap
- [`assets/observability/backups/solo-enterprise-telemetry-collector-config.backup.yaml`](assets/observability/backups/solo-enterprise-telemetry-collector-config.backup.yaml) - original, for rollback

```bash
kubectl apply -f assets/observability/solo-enterprise-telemetry-collector-config.patched.yaml
kubectl -n kagent rollout restart statefulset solo-enterprise-telemetry-collector
kubectl -n kagent rollout status statefulset solo-enterprise-telemetry-collector --timeout=5m
```

## Verify

Both ClickHouses should report a growing count while you invoke agents:

```bash
# agentregistry ClickHouse (the new path)
kubectl -n agentregistry-system exec agentregistry-enterprise-clickhouse-shard0-0 -- \
 clickhouse-client -q "SELECT count(), max(Timestamp) FROM agentregistry.otel_traces_json"

# kagent ClickHouse (should still receive - unchanged)
kubectl -n kagent exec kagent-mgmt-clickhouse-shard0-0 -- \
 clickhouse-client -q "SELECT count(), max(Timestamp) FROM platformdb.otel_traces_json"
```

If you don't see the agentregistry count moving, send a real chat to a kagent agent - card fetches alone do not emit spans.

## Durability Caveat

This ConfigMap is **Helm-managed** by release `kagent-mgmt` (chart `management`). Neither the `kagent-mgmt` chart nor the agentregistry chart exposes a value for an extra trace exporter, so this is a live patch.

**`helm upgrade kagent-mgmt` will revert it.** Re-apply after every upgrade:

```bash
kubectl apply -f assets/observability/solo-enterprise-telemetry-collector-config.patched.yaml
kubectl -n kagent rollout restart statefulset solo-enterprise-telemetry-collector
```

For a permanent fix, upstream the exporter into the management chart's collector template.

## Cleanup

```bash
kubectl apply -f assets/observability/backups/solo-enterprise-telemetry-collector-config.backup.yaml
kubectl -n kagent rollout restart statefulset solo-enterprise-telemetry-collector
```

## Next

- [060 - Tracing setup (the other half - runtime endpoints)](060-observability-tracing.md)
