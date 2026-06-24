# Remote MCP via kagent (GitHub Copilot)

Register the GitHub Copilot MCP as a **remote streamable-HTTP** MCP server with agentregistry, then deploy that catalog entry to the **kagent** runtime so kagent agents can call it.

## Lab Objectives

- Register an `MCPServer` with `remote.type: streamable-http` pointing at `https://api.githubcopilot.com/mcp`
- Deploy it to the kagent runtime
- Verify both the catalog entry and the runtime-side `RemoteMCPServer` CR are healthy

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) → [002a](002a-setup-oidc-keycloak.md) **or** [002b](002b-setup-oidc-entra.md) → [003](003-install-components.md)
- **kagent Enterprise installed** in the `kagent` namespace via the [kagent-enterprise workshop](https://github.com/solo-io/field-agentic-labs/tree/main/kagent-enterprise)
- **kagent registered as an agentregistry Runtime.** If you haven't done [020](020-kagent-runtime-and-agent.md), do steps 1-3 of it to register `Runtime: kagent`. You do not need to deploy `k8shelper` before this lab; in the full flow, you return to 020 after this lab to register and deploy the Agent.
- A GitHub Copilot MCP access token:

  ```bash
  export GITHUB_COPILOT_MCP_TOKEN=<your-github-copilot-mcp-token>
  ```

## 1. Register the MCP Catalog Entry

The manifest is at [`assets/mcp/github-copilot-mcpserver.yaml`](assets/mcp/github-copilot-mcpserver.yaml):

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

> For demos, rendering the token directly into the artifact is fine. For production, use the secret mechanism supported by your agentregistry deployment instead of literal values.

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
arctl get deployment github-copilot-mcp-kagent -o yaml
```

Look for:

- `phase: deployed`
- Conditions: `Ready=True`, `RuntimeConfigured=True`, `MCPServerURL=True`

## Cleanup

```bash
arctl delete deployment github-copilot-mcp-kagent
arctl delete mcp        github-copilot-mcp-server --tag latest

unset GITHUB_COPILOT_MCP_TOKEN
```

## Troubleshooting

| Symptom | Cause |
|---|---|
| `RemoteMCPServer` has `Accepted=False` with `unsupported protocol scheme ""` | The `spec.url` is missing `https://`. Re-render the manifest with `envsubst` and check the token didn't have a stray newline. |
| Deployment never leaves `deploying` | Check `arctl get deployment ... -o yaml` for `status.conditions`. Usually the kagent runtime can't reach the remote URL (egress / proxy) or the token is wrong. |

## Next

- [020 - kagent Agent (`k8shelper`)](020-kagent-runtime-and-agent.md) - wire this MCP into the k8shelper Agent
- [032 - Remote MCP through Agentgateway](032-mcp-through-agentgateway.md) - third MCP topology (no kagent involved)
- [050 - AccessPolicy](050-access-policies.md) - restrict which GitHub MCP tools a deployment is allowed to call
