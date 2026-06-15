# kagent Runtime

This lab registers an existing **kagent** installation as an AgentRegistry Runtime. AgentRegistry will then create kagent CRDs in the `kagent` namespace whenever you deploy an `Agent` against this Runtime.

## Lab Objectives

- Apply a `Runtime/kagent` manifest pointing at the kagent controller
- Enable kagent's `INSECURE_MODE` so it accepts the `X-User-Id` header AgentRegistry forwards
- Verify the kagent API returns 200 from inside the AgentRegistry pod

## Prerequisites

- [040 — `arctl` authenticated](040-arctl-auth.md)
- An existing kagent installation in the `kagent` namespace, including the `kagent-controller` Deployment

## 1. Apply the Runtime

```bash
cat > /tmp/kagent-runtime.yaml <<'EOF'
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
EOF

arctl apply -f /tmp/kagent-runtime.yaml
arctl get runtimes
```

Expected: a `kagent` runtime with `type: Kagent`.

`spec.telemetryEndpoint` is the URL deployed kagent agents will use as `OTEL_EXPORTER_OTLP_ENDPOINT`. The full tracing story (including the fact that kagent injects a higher-priority traces-only endpoint) is covered in [090](090-observability-tracing.md) and [091](091-trace-fanout-workaround.md).

## 2. Enable `INSECURE_MODE` on the kagent Controller

> **Demo only.** This disables kagent controller authn/authz. Use only in isolated development clusters. For production, configure both sides to share a compatible OIDC audience/issuer, or use a token-exchange flow.

AgentRegistry sends requests to the kagent controller with an `X-User-Id` header. In its default mode, the controller will reject those requests. Enable `INSECURE_MODE`:

```bash
kubectl set env deployment/kagent-controller -n kagent INSECURE_MODE=true
kubectl rollout status deployment/kagent-controller -n kagent --timeout=5m
```

> **Helm note:** if you manage kagent via Helm, persist this in values under `controller.env` so the next `helm upgrade` doesn't revert it.

## 3. Verify the kagent API Accepts Requests from AgentRegistry

```bash
kubectl exec -n agentregistry-system deployment/agentregistry-enterprise-server -- \
  curl -i -H 'X-User-Id: admin@kagent.dev' \
  'http://kagent-controller.kagent.svc.cluster.local:8083/api/agents?namespace=kagent'
```

Expected: `HTTP/1.1 200 OK`.

If you see `no session found` or `401 Unauthorized`, `INSECURE_MODE` didn't take effect:

```bash
kubectl get deploy kagent-controller -n kagent -o yaml | grep -A1 INSECURE_MODE
kubectl get pods -n kagent -l app=kagent-controller
```

Make sure the new pod (with the env var) is the one running.

## Troubleshooting

| Error from `arctl get deployment <name> -o yaml` | Cause |
|---|---|
| `kagent URL is required` | Runtime is missing `spec.config.kagentUrl` |
| `authentication token expired during deployment; please retry` | AgentRegistry maps **every** kagent API 401 to this. Usually means the kagent controller rejected the forwarded identity. Enable `INSECURE_MODE` or wire up matching OIDC. |
| `image must be specified` (when applying an Agent) | kagent requires `spec.source.image` on the Agent. See [061](061-deploy-k8shelper-on-kagent.md). |
| Image pull failures | The image must be public, or the runtime namespace must have image pull secrets that exist. |

To deploy into another namespace, either update `spec.config.namespace` on this Runtime, or create a second `Runtime` with a different name and namespace.

## Next

- [061 — Deploy `k8shelper` on kagent](061-deploy-k8shelper-on-kagent.md)
- [071 — Register a Remote MCP Server (GitHub Copilot)](071-register-github-copilot-mcp.md) → [072 — Wire MCP to an Agent](072-wire-mcp-to-agent.md)
