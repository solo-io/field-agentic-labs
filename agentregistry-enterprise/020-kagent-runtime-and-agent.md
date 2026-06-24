# kagent Runtime + Agent (k8shelper)

Register an existing in-cluster **kagent Enterprise** install as an agentregistry **Runtime**, then deploy the `k8shelper` BYO agent on top. The agent ships as a Python ADK package in this repo; this lab assumes you already have a built image in a registry your cluster can pull from.

The lab walks through the **Anthropic / Claude** variant by default. The Gemini variant is a near-identical flow - the differences are called out in step 2.

> **Ordering with lab 031:** The checked-in `k8shelper` Agent references the GitHub Copilot MCP catalog entry from [031](031-mcp-remote-github-copilot.md). Run this lab through step 4 to register `Runtime/kagent` and create the model Secret, run [031](031-mcp-remote-github-copilot.md), then return here at step 5 to register and deploy the Agent.

## Lab Objectives

- Register `Runtime: kagent` pointing at the in-cluster kagent install
- Point the `Agent` manifest at a prebuilt `k8shelper` image
- Create the model API-key Secret in the `kagent` namespace
- Register the `Agent` + apply the `Deployment`, patch the generated kagent CR to inject the API-key Secret
- Verify the pod reaches `Ready=True`

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) → [002a](002a-setup-oidc-keycloak.md) **or** [002b](002b-setup-oidc-entra.md) → [003](003-install-components.md)
- **kagent Enterprise installed** in the `kagent` namespace. Install it via the [kagent-enterprise workshop](https://github.com/solo-io/field-agentic-labs/tree/main/kagent-enterprise) - labs 001 through 003 cover the install end-to-end. Confirm with `kubectl get pods -n kagent` showing the kagent controller + UI Ready.
- A prebuilt `k8shelper` image in a container registry your cluster nodes can pull from (Docker Hub, GHCR, ECR, GAR, ACR, etc.). Export the image reference before you start the lab:

  ```bash
  export K8SHELPER_IMAGE="<your-registry>/<your-repo>/k8shelper-anthropic:<tag>"
  ```
- An **Anthropic API key**:

  ```bash
  export ANTHROPIC_API_KEY=<your-anthropic-api-key>
  ```

> **For the Gemini variant**, swap `assets/k8shelper-anthropic/` for `assets/k8shelper-gemini/` everywhere below, and use `GOOGLE_API_KEY` + the Secret name `k8shelper-google` instead of `k8shelper-anthropic`. The rest is identical.

## 1. Enable kagent's Insecure Mode (For Demo Purposes Only)

Agentregistry sends requests to the kagent controller with an `X-User-Id` header. By default kagent Enterprise rejects unauthenticated headers - enable `INSECURE_MODE=true` so kagent accepts the forwarded identity from agentregistry:

```bash
kubectl set env deployment/kagent-controller -n kagent INSECURE_MODE=true
kubectl rollout status deployment/kagent-controller -n kagent --timeout=5m
```

> **Demo / POC only.** `INSECURE_MODE=true` disables kagent controller authn/authz. For production, configure kagent Enterprise and agentregistry to share a compatible OIDC audience (both can validate the same Entra / Keycloak tokens) or use a token-exchange flow.

Verify kagent now accepts the forwarded identity from the agentregistry server:

```bash
kubectl exec -n agentregistry-system deployment/agentregistry-enterprise-server -- \
  curl -i -H 'X-User-Id: admin@kagent.dev' \
  'http://kagent-controller.kagent.svc.cluster.local:8083/api/agents?namespace=kagent'
```

Expected: `HTTP/1.1 200 OK`.

## 2. Set the `k8shelper` Image

Set `K8SHELPER_IMAGE` to the prebuilt image you want agentregistry to deploy. The later `envsubst` command renders this value into the `Agent` catalog entry.

```bash
export K8SHELPER_IMAGE="<your-registry>/<your-repo>/k8shelper-anthropic:<tag>"
echo "K8SHELPER_IMAGE=${K8SHELPER_IMAGE}"
```

## 3. Register the kagent Runtime

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
```

```bash
arctl apply -f /tmp/kagent-runtime.yaml
```

If this returns `forbidden`, your user is authenticated but agentregistry does not see you as an admin. For the Keycloak path, agentregistry must be installed with `oidc.superuserRole=are-admins`; the current [003 install values](003-install-components.md#1-install-agentregistry-enterprise) set this automatically.

```bash
arctl get runtimes
```

Expected: a `Runtime/kagent` with `type: Kagent`.

`spec.telemetryEndpoint` is what agentregistry injects as `OTEL_EXPORTER_OTLP_ENDPOINT` into kagent-deployed agents. [060](060-observability-tracing.md) covers the trace plumbing in detail; you can leave it at the value above for now.

## 4. Create the Anthropic API-Key Secret

```bash
kubectl create secret generic k8shelper-anthropic \
  -n kagent \
  --from-literal=ANTHROPIC_API_KEY="${ANTHROPIC_API_KEY}" \
  --dry-run=client -o yaml | kubectl apply -f -
```

## 5. Register the Agent

The agent + deployment manifests are checked in:

- [`assets/providers/kagent/anthropicagent/k8shelperanthropic.yaml`](assets/providers/kagent/anthropicagent/k8shelperanthropic.yaml) - references `${K8SHELPER_IMAGE}` (from step 2)
- [`assets/providers/kagent/anthropicagent/ardeploy.yaml`](assets/providers/kagent/anthropicagent/ardeploy.yaml) - targets `Runtime: kagent`

> The Agent manifest references `MCPServer: github-copilot-mcp-server`. Current agentregistry versions validate that reference at apply time. Complete [031 - Remote MCP via kagent](031-mcp-remote-github-copilot.md), then return here before running this step.

```bash
envsubst < assets/providers/kagent/anthropicagent/k8shelperanthropic.yaml | arctl apply -f -
arctl get agents
```

## 6. Apply the Deployment

```bash
arctl apply -f assets/providers/kagent/anthropicagent/ardeploy.yaml
```

If you see `spec.targetRef: referenced resource not found`, you applied the Deployment before the Agent. Re-run step 5.

If you see `spec.deploymentRefs[0]: referenced resource not found` (referring to `github-copilot-mcp-kagent`), the MCP-server Deployment from [031](031-mcp-remote-github-copilot.md) doesn't exist. Apply it first or remove the `deploymentRefs` block from the Deployment YAML.

## 7. Patch the Generated kagent Agent to Inject the Secret

With agentregistry today, agentregistry can pass environment variables to kagent deployments, but the current `Deployment.spec.env` model supports literal string values only. It cannot express Kubernetes `valueFrom.secretKeyRef` entries.

Agentregistry's `Deployment.spec.env` accepts literal values only. Secret references have to be patched onto the generated `kagent.dev/v1alpha2 Agent` CR:

```bash
kubectl patch agent k8shelperanthropic -n kagent --type='json' -p='[
  {"op":"add","path":"/spec/byo/deployment/env/-","value":{"name":"ANTHROPIC_API_KEY","valueFrom":{"secretKeyRef":{"name":"k8shelper-anthropic","key":"ANTHROPIC_API_KEY"}}}}
]'

kubectl rollout status deployment/k8shelperanthropic -n kagent --timeout=5m
```

> Direct patches to generated kagent resources can be overwritten by a future agentregistry redeploy. If you need this to survive a redeploy, re-run the patch in your CI/CD after each `arctl apply`.

## 8. Verify

```bash
kubectl get agents.kagent.dev   -n kagent k8shelperanthropic -o yaml
kubectl get pods                -n kagent -l kagent=k8shelperanthropic
kubectl get svc                 -n kagent -l kagent=k8shelperanthropic
kubectl get deploy k8shelperanthropic -n kagent -o yaml \
  | grep -E 'MODEL_NAME|MODEL_PROVIDER|ANTHROPIC_API_KEY|image:'
```

Expected: `MODEL_PROVIDER=anthropic`, `MODEL_NAME=claude-sonnet-4-6`, the Secret reference, and your image.

## Model Configuration Notes

The agent uses Google ADK with LiteLLM for Anthropic, so it calls Anthropic **directly** with `ANTHROPIC_API_KEY` (not via Vertex). When `MODEL_PROVIDER=anthropic`, the agent code in [`assets/k8shelper-anthropic/k8shelper/agent.py`](assets/k8shelper-anthropic/k8shelper/agent.py) prefixes `MODEL_NAME` with `anthropic/` for LiteLLM if you pass a bare Claude model name.

## Cleanup

```bash
# agentregistry side
arctl delete deployment k8shelperanthropic-kagent
arctl delete agent      k8shelperanthropic --tag 1.0.0
arctl delete runtime    kagent

# Kubernetes side: the Secret + the patched kagent CR
kubectl delete secret k8shelper-anthropic -n kagent
# (kagent's controller garbage-collects the kagent Agent CR + Deployment when
# the agentregistry Deployment is deleted above)

# Roll back the INSECURE_MODE change if you want kagent back to its default authn
kubectl set env deployment/kagent-controller -n kagent INSECURE_MODE-

# Local temp files
rm -f /tmp/kagent-runtime.yaml

unset K8SHELPER_IMAGE ANTHROPIC_API_KEY
```

To remove the image you pushed in step 2, use your registry's tooling (`docker image rm`, `gh release delete`, etc.).

## Next

- [031 - Remote MCP via kagent (GitHub Copilot)](031-mcp-remote-github-copilot.md) - wire MCP tools into this Agent
- [050 - AccessPolicy](050-access-policies.md) - restrict which tools the Agent is allowed to call
- [060 - Observability / Tracing](060-observability-tracing.md) - see traces from this Agent in the AR dashboard
