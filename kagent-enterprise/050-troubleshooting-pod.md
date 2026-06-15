# Validate Your Install — Fix a Broken Pod with the `k8s-agent`

This lab is a five-minute smoke test of the kagent install. You deploy a deliberately-broken Pod (an Nginx Pod with the image tag `nginx:latesttttt` — note the typo), then ask the pre-built `k8s-agent` Agent to diagnose and fix it. If kagent is installed correctly and connected to your LLM, the agent will walk through `kubectl describe`, identify the bad image tag, and offer to apply the fix.

## Lab Objectives

- Deploy a Pod that intentionally fails (`ImagePullBackOff`)
- Use the pre-built `k8s-agent` Agent in the UI to debug and fix it
- Confirm the agent has working tool access and your LLM provider is responding

## Prerequisites

- [020 — Kagent Enterprise installed](020-install-kagent-enterprise.md)
- The kagent UI is reachable (port-forward from [020 step 4](020-install-kagent-enterprise.md#4-work-around-the-ui-backend-bug-port-forward) if needed)
- A `default-model-config` exists and your LLM provider is healthy

## 1. Deploy the Broken Pod

```bash
kubectl apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: ngi14
spec:
  containers:
  - name: nginx
    image: nginx:latesttttt
    ports:
    - containerPort: 80
EOF
```

Confirm it fails:

```bash
kubectl get pod ngi14
kubectl describe pod ngi14
```

You should see `ImagePullBackOff` with `Failed to pull image "nginx:latesttttt"`.

## 2. Open the UI and Find `k8s-agent`

In the Solo Enterprise for kagent UI, find the pre-built **k8s-agent** Agent.

## 3. Prompt the Agent

```
Why is the Nginx Pod failing in my default namespace?
```

A good run looks like this — the agent should:

1. Call something like `pods_list_in_namespace(namespace="default")` and find `ngi14`.
2. Call `pods_get` and `events_list` to gather details.
3. Identify the image tag typo (`nginx:latesttttt` should be `nginx:latest`).
4. Either explain the fix or offer to apply a patched manifest.

If the agent can suggest the fix but its tools don't include `pods_delete` / `resources_create_or_update`, you'll need to apply the corrected Pod manually — that's fine, it's still proof that the tool surface, LLM, and reasoning loop all work end-to-end.

## 4. (Optional) Clean Up

```bash
kubectl delete pod ngi14
```

## What This Test Tells You

Running this successfully proves:

| Capability | Why it matters |
|---|---|
| The UI talks to the controller | Port-forward + `uiBackendHost` workaround is in place |
| The controller can call your LLM | `llm-api-keys` is correct, `default-model-config` is correct |
| The agent's tools resolve | The MCP server's tool surface is mounted into the agent runtime |
| The agent can list and read cluster state | RBAC for the agent's ServiceAccount is intact |

If the agent says it has no tools, see [040](040-mcp-connection-agent-config.md) — your MCP server is probably not set up yet. If the agent errors on the LLM call, double-check `llm-api-keys` and the `ModelConfig`.

## Next

- [060 — `AccessPolicy`: Agent → MCP](060-accesspolicy-agent-to-mcp.md) — restrict the tools this agent (or any other) can call
