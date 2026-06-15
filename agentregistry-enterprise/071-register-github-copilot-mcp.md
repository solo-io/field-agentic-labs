# Register a Remote Streamable-HTTP MCP Server (GitHub Copilot)

This lab registers the GitHub Copilot MCP as a **remote streamable-HTTP** MCP server with AgentRegistry, then deploys that registration to the `kagent` runtime so kagent agents can call it.

## Lab Objectives

- Register an `MCPServer` with `remote.type: streamable-http`
- Deploy that MCP server to the kagent runtime
- Verify both the catalog entry and the runtime deployment are healthy

## Prerequisites

- [051 — kagent Runtime registered](051-kagent-provider.md)
- A GitHub Copilot MCP access token, exported as `GITHUB_COPILOT_MCP_TOKEN`

## 1. Register the MCP Artifact

[`assets/mcp/github-copilot-mcpserver.yaml`](assets/mcp/github-copilot-mcpserver.yaml):

```yaml
apiVersion: ar.dev/v1alpha1
kind: MCPServer
metadata:
  name: github-copilot-mcp-server
  tag: latest
spec:
  description: GitHub Copilot MCP Server to interact with GitHub repositories, issues, pull requests, and Copilot coding-agent tasks
  remote:
    type: streamable-http
    url: https://api.githubcopilot.com/mcp
    headers:
      - name: Authorization
        value: ${GITHUB_COPILOT_MCP_TOKEN}
```

Render the env var and apply:

```bash
envsubst < assets/mcp/github-copilot-mcpserver.yaml | arctl apply -f -
arctl get mcps
arctl get mcp github-copilot-mcp-server --tag latest -o yaml
```

> For demos, rendering the token directly into the artifact is fine. For production, use the secret mechanism supported by your AgentRegistry deployment instead of literal values in the manifest.

## 2. Deploy the MCP to kagent

[`assets/mcp/github-copilot-mcp-deploy.yaml`](assets/mcp/github-copilot-mcp-deploy.yaml):

```yaml
apiVersion: ar.dev/v1alpha1
kind: Deployment
metadata:
  name: github-copilot-mcp-kagent
spec:
  runtimeRef:
    kind: Runtime
    name: kagent
  targetRef:
    kind: MCPServer
    name: github-copilot-mcp-server
    tag: latest
```

```bash
arctl apply -f assets/mcp/github-copilot-mcp-deploy.yaml
arctl get deployments
arctl get deployment github-copilot-mcp-kagent -o yaml
```

You're looking for:

- `phase: deployed`
- Conditions: `Ready=True`, `RuntimeConfigured=True`, `MCPServerURL=True`

## 3. (Optional) Inspect the Generated `RemoteMCPServer` CR

The kagent runtime creates a `kagent.dev RemoteMCPServer` for this:

```bash
kubectl get remotemcpservers.kagent.dev -n kagent
kubectl get remotemcpserver github-copilot-mcp-server -n kagent -o yaml
```

A healthy one has:

- `Accepted=True`
- `spec.url` set to `https://api.githubcopilot.com/mcp`
- Populated `status.discoveredTools`

## Troubleshooting

| Symptom | Cause |
|---|---|
| `RemoteMCPServer` has `Accepted=False` with `unsupported protocol scheme ""` | The `spec.url` is missing `http://` / `https://`. For GitHub Copilot MCP use `https://api.githubcopilot.com/mcp`. |
| `arctl apply` succeeds but the deployment never leaves `deploying` | Check `arctl get deployment github-copilot-mcp-kagent -o yaml` for `status.conditions`. Most often the kagent runtime cannot reach the remote URL or the token is wrong. |

## Next

- [072 — Wire the MCP Server to an Agent](072-wire-mcp-to-agent.md)
