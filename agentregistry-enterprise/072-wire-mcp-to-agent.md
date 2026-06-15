# Wire an MCP Server to an Agent

This lab takes the GitHub Copilot MCP server from [071](071-register-github-copilot-mcp.md) and wires it into the `k8shelper` agent from [061](061-deploy-k8shelper-on-kagent.md) so the agent can call GitHub Copilot tools at runtime.

Use this flow after deleting any AgentRegistry `Agent`, `MCPServer`, or `Deployment` records.

## Resource Names

| Layer | Kind | Example name |
|-------|------|--------------|
| AgentRegistry MCP artifact | `ar.dev/v1alpha1` `MCPServer` | `github-copilot-mcp-server` |
| AgentRegistry MCP deployment | `ar.dev/v1alpha1` `Deployment` | `github-copilot-mcp-kagent` |
| AgentRegistry Agent artifact | `ar.dev/v1alpha1` `Agent` | `k8shelper` |
| AgentRegistry Agent deployment | `ar.dev/v1alpha1` `Deployment` | `k8shelper-kagent` |
| Generated kagent Agent CR | `kagent.dev/v1alpha2` `Agent` | `k8shelper` in `kagent` ns |
| Generated K8s Deployment | `apps/v1` `Deployment` | `k8shelper` in `kagent` ns |

Use AgentRegistry names with `arctl`. Use generated kagent / Kubernetes names with `kubectl`.

## Lab Objectives

- Build a k8shelper image that includes the current MCP loader and `list_available_tools`
- Register the GitHub Copilot MCP and deploy it to kagent (from [071](071-register-github-copilot-mcp.md))
- Register `k8shelper` with `mcpServers:` referencing that MCP artifact
- Deploy `k8shelper` with `deploymentRefs:` pointing at the MCP deployment
- Verify the agent sees the GitHub MCP tool surface inside the pod

## Prerequisites

- [051 — kagent Runtime](051-kagent-provider.md)
- [061 — k8shelper image built and pushed](061-deploy-k8shelper-on-kagent.md) (the Gemini variant is used as the worked example below)
- [071 — GitHub Copilot MCP registered + deployed to kagent](071-register-github-copilot-mcp.md)

## 1. Confirm the kagent Runtime is in Place

```bash
arctl get runtime kagent -o yaml
```

Expected shape:

```yaml
spec:
  type: Kagent
  config:
    kagentUrl: http://kagent-controller.kagent.svc.cluster.local:8083
    namespace: kagent
```

## 2. Build k8shelper with the MCP-aware Agent Code

The image must contain:

- [`assets/k8shelper-gemini/k8shelper/mcp_tools.py`](assets/k8shelper-gemini/k8shelper/mcp_tools.py) — reads `MCP_SERVERS_CONFIG`, filters incompatible tools (`issue_write` by default).
- [`assets/k8shelper-gemini/k8shelper/agent.py`](assets/k8shelper-gemini/k8shelper/agent.py) — includes `list_available_tools` so the model can disclose what MCP tools it sees.

```bash
cd assets/k8shelper-gemini
export K8SHELPER_IMAGE="<your-registry>/k8shelper:github-mcp"
docker buildx build --platform linux/amd64 -t "${K8SHELPER_IMAGE}" --push .
```

## 3. Register the k8shelper Agent with the MCP Reference

[`assets/providers/kagent/geminiagent/k8shelpergemini.yaml`](assets/providers/kagent/geminiagent/k8shelpergemini.yaml) is the worked example. Make sure it has both the `image` and the `mcpServers` block:

```yaml
apiVersion: ar.dev/v1alpha1
kind: Agent
metadata:
  name: k8shelper
  tag: "1.0.0"
spec:
  title: k8shelper
  description: "Kubernetes helper agent deployed through the kagent runtime"
  modelProvider: gemini
  modelName: gemini-3.5-flash
  source:
    image: ${K8SHELPER_IMAGE}
  mcpServers:
    - kind: MCPServer
      name: github-copilot-mcp-server
```

```bash
envsubst < assets/providers/kagent/geminiagent/k8shelpergemini.yaml | arctl apply -f -
arctl get agent k8shelper --tag 1.0.0 -o yaml
```

## 4. Deploy k8shelper with `deploymentRefs` for the MCP

[`assets/providers/kagent/geminiagent/ardeploy.yaml`](assets/providers/kagent/geminiagent/ardeploy.yaml) must reference the MCP deployment name **exactly** as you applied it in [071](071-register-github-copilot-mcp.md):

```yaml
apiVersion: ar.dev/v1alpha1
kind: Deployment
metadata:
  name: k8shelper-kagent
spec:
  targetRef:
    kind: Agent
    name: k8shelper
    tag: "1.0.0"
  runtimeRef:
    kind: Runtime
    name: kagent
  deploymentRefs:
    - name: github-copilot-mcp-kagent
  env:
    MODEL_PROVIDER: gemini
    MODEL_NAME: gemini-3.5-flash
```

```bash
arctl apply -f assets/providers/kagent/geminiagent/ardeploy.yaml
arctl get deployment k8shelper-kagent -o yaml
```

Look for:

- `phase: deployed`
- `Ready=True`, `RuntimeConfigured=True`
- runtime metadata pointing at namespace `kagent`

## 5. Inject the Model API-Key Secret

(Same as [061 step 3](061-deploy-k8shelper-on-kagent.md#3-create-the-api-key-secret) — repeated here for completeness.)

```bash
kubectl create secret generic k8shelper-google \
  -n kagent \
  --from-literal=GOOGLE_API_KEY="${GOOGLE_API_KEY}" \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl patch agent k8shelper -n kagent --type='json' -p='[
  {"op":"add","path":"/spec/byo/deployment/env/-","value":{"name":"GOOGLE_API_KEY","valueFrom":{"secretKeyRef":{"name":"k8shelper-google","key":"GOOGLE_API_KEY"}}}}
]'

kubectl rollout status deployment/k8shelper -n kagent --timeout=5m
```

## 6. Verify MCP Wiring

```bash
# Verify AgentRegistry injected MCP config into the generated kagent workload
kubectl get agent k8shelper -n kagent -o yaml | grep -i mcp
kubectl get deploy k8shelper -n kagent -o yaml \
  | grep -E 'MCP_SERVERS_CONFIG|MCP_SERVERS_CONFIG_PATH|mcp-servers.json'

# Inspect the kagent RemoteMCPServer
kubectl get remotemcpservers.kagent.dev -n kagent
kubectl get remotemcpserver github-copilot-mcp-server -n kagent -o yaml
```

A healthy `RemoteMCPServer`:

- `Accepted=True`
- `spec.url` is `https://api.githubcopilot.com/mcp` (or another valid `http://` / `https://` URL)
- `status.discoveredTools` is populated

From inside the agent pod, confirm the agent sees the GitHub MCP tools:

```bash
kubectl exec -i -n kagent deploy/k8shelper -- python - <<'PY'
import asyncio
from k8shelper.agent import root_agent

async def main():
    all_tools = []
    for tool in root_agent.tools:
        if hasattr(tool, "get_tools"):
            all_tools.extend(await tool.get_tools())
        else:
            all_tools.append(tool)

    names = [getattr(t, "name", getattr(t, "__name__", type(t).__name__)) for t in all_tools]
    print("tool_count", len(names))
    print("has_list_available_tools", "list_available_tools" in names)
    print("has_github_tools",
          any(n in names for n in ["search_repositories", "create_pull_request", "get_me"]))
    print("has_issue_write", "issue_write" in names)

    for tool in root_agent.tools:
        if hasattr(tool, "close"):
            await tool.close()

asyncio.run(main())
PY
```

Expected:

```text
has_list_available_tools True
has_github_tools True
has_issue_write False
```

> `issue_write` is filtered out by default because the GitHub Copilot MCP schema for that tool includes a boolean-only enum that Gemini rejects when converting MCP tools to function declarations. Override `MCP_DISABLED_TOOLS` only if your model/runtime accepts that schema.

## Existing-Image Workaround

Prefer rebuilding the image. Use this only when you must run an older image that doesn't include the current `agent.py` and `mcp_tools.py`.

Create `/tmp/mcp-servers.json`:

```json
[
  {
    "name": "github-copilot-mcp-server",
    "type": "remote",
    "url": "https://api.githubcopilot.com/mcp",
    "headers": {
      "Authorization": "${GITHUB_COPILOT_MCP_TOKEN}"
    }
  }
]
```

Create the Secret and a ConfigMap with the current source files:

```bash
envsubst < /tmp/mcp-servers.json > /tmp/mcp-servers.rendered.json

kubectl create secret generic k8shelper-mcp-servers \
  -n kagent \
  --from-file=mcp-servers.json=/tmp/mcp-servers.rendered.json \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl create configmap k8shelper-code-override \
  -n kagent \
  --from-file=agent.py=assets/k8shelper-gemini/k8shelper/agent.py \
  --from-file=mcp_tools.py=assets/k8shelper-gemini/k8shelper/mcp_tools.py \
  --dry-run=client -o yaml | kubectl apply -f -
```

Then patch the generated kagent Agent to add `MCP_SERVERS_CONFIG_PATH` and mount the override files. The first patch adds the env var without replacing existing ones:

```bash
kubectl patch agent k8shelper -n kagent --type json -p='[
  {"op":"add","path":"/spec/byo/deployment/env/-","value":{"name":"MCP_SERVERS_CONFIG_PATH","value":"/config/mcp-servers.json"}}
]'

kubectl patch agent k8shelper -n kagent --type merge -p '{
  "spec": {
    "byo": {
      "deployment": {
        "volumes": [
          {"name":"mcp-servers-config","secret":{"secretName":"k8shelper-mcp-servers","items":[{"key":"mcp-servers.json","path":"mcp-servers.json"}]}},
          {"name":"k8shelper-code-override","configMap":{"name":"k8shelper-code-override","items":[{"key":"mcp_tools.py","path":"mcp_tools.py"},{"key":"agent.py","path":"agent.py"}]}}
        ],
        "volumeMounts": [
          {"name":"mcp-servers-config","mountPath":"/config","readOnly":true},
          {"name":"k8shelper-code-override","mountPath":"/app/k8shelper/mcp_tools.py","subPath":"mcp_tools.py","readOnly":true},
          {"name":"k8shelper-code-override","mountPath":"/app/k8shelper/agent.py","subPath":"agent.py","readOnly":true}
        ]
      }
    }
  }
}'

kubectl rollout status deployment/k8shelper -n kagent --timeout=5m
```

Direct patches to generated kagent resources can be overwritten by future AgentRegistry redeploys.

## Troubleshooting

- **MCP deployment is `deployed` but k8shelper has no GitHub tools.** Confirm the image was built from the current `assets/k8shelper-gemini/` source, that `MCP_SERVERS_CONFIG` or `MCP_SERVERS_CONFIG_PATH` is present in the generated Deployment, and that `list_available_tools` shows up in the live pod.
- **Gemini returns `400 INVALID_ARGUMENT` for a function-declaration enum.** Confirm `issue_write` is filtered out; check `MCP_DISABLED_TOOLS` — default should include `issue_write`.
- **The model says it only has `roll_die` and `check_prime`.** You're running an old image without `list_available_tools`. Rebuild.

## Next

- [080 — AccessPolicy / RBAC](080-access-policies.md)
- [090 — Observability / Tracing](090-observability-tracing.md)
