# Expose a Remote MCP Server Through Agentgateway

A third deployment topology for the same GitHub Copilot remote MCP server from [031](031-mcp-remote-github-copilot.md) - this time the MCP doesn't run *inside* a runtime like kagent at all. Agentregistry catalogs it, then a `Virtual` runtime tells agentregistry to expose it as a child route on an **Agentgateway** Gateway. MCP clients hit a public path (`/registry/github-copilot`) on the gateway and the gateway brokers the connection to the upstream `https://api.githubcopilot.com/mcp`.

This is the right pattern when:

- You don't want every agent runtime (kagent, AgentCore, etc.) to know how to reach the remote MCP
- You want one place to apply gateway policies (auth, rate limits, mTLS to the upstream) for MCP traffic
- You're integrating non-kagent clients (Claude Code, Claude Desktop, MCP-aware tools running outside the cluster)

## Architecture

```
client
  │
  │ MCP request
  ▼
[ Agentgateway Gateway (agentgateway-system) ]
  │
  │ parent HTTPRoute  ──▶  delegates /registry to children
  ▼
[ child HTTPRoute (agentregistry-system) ]   ← created by agentregistry
  │
  │ EnterpriseAgentgatewayBackend            ← created by agentregistry
  ▼
remote MCP server (https://api.githubcopilot.com/mcp)
```

**Two ownership boundaries:**

| Owner | Owns |
|---|---|
| Gateway admin | The Kubernetes `Gateway` + the parent `HTTPRoute` that delegates `/registry` to children |
| Agentregistry | The catalog `MCPServer`, the `Deployment` targeting `Runtime/virtual-default`, and the child `HTTPRoute` + `EnterpriseAgentgatewayBackend` it generates |

The `agentregistry.solo.io/runtime` label on the parent `Gateway` + parent `HTTPRoute` is what binds the two halves together - it tells agentregistry "any Deployment that targets `Runtime: virtual-default` should plumb its child route into the resources carrying this label."

## Lab Objectives

- Stand up a parent `Gateway` + parent `HTTPRoute` carrying the `agentregistry.solo.io/runtime: virtual-default` label
- Confirm the seeded `virtual-default` Runtime exists (or create it)
- Catalog the GitHub Copilot remote MCP server in agentregistry
- Deploy it to the `Virtual` runtime with a `pathSuffix`
- Verify the generated child `HTTPRoute` + `EnterpriseAgentgatewayBackend`
- Hit the exposed `/registry/github-copilot` path through the gateway

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) → [002a](002a-setup-oidc-keycloak.md) **or** [002b](002b-setup-oidc-entra.md) → [003](003-install-components.md). Step 3 of 003 installs Enterprise Agentgateway with `GatewayClass: enterprise-agentgateway` + the Kubernetes Gateway API CRDs - both required by this lab.
- A GitHub Copilot MCP access token:

  ```bash
  export GITHUB_COPILOT_MCP_TOKEN="<github-token>"
  ```

## 1. Create the Parent Gateway and Route

The `agentregistry.solo.io/runtime` label binds these Kubernetes resources to an agentregistry `Virtual` runtime. The manifest at [`assets/mcp/agentgateway/parent-gateway-and-route.yaml`](assets/mcp/agentgateway/parent-gateway-and-route.yaml) creates both:

```bash
kubectl apply -f assets/mcp/agentgateway/parent-gateway-and-route.yaml
```

What it does:

- **`Gateway/remote-mcp-gateway`** - HTTP listener on `:80` in `agentgateway-system`, labeled `agentregistry.solo.io/runtime: virtual-default`
- **`HTTPRoute/remote-mcp-delegate`** - same label, parents to `remote-mcp-gateway`, delegates `/registry` to *any* child `HTTPRoute` (`name: "*"`) in the `agentregistry-system` namespace

That second part is the delegation: when agentregistry creates a child route under itself, it gets stitched into this parent route's `/registry` prefix automatically.

The delegation is possible due to the label:
```
labels:
  agentregistry.solo.io/runtime: virtual-default
```

## 2. Confirm the `Virtual` Runtime Exists

Agentregistry seeds `virtual-default` on startup. Confirm:

```bash
arctl get runtime virtual-default -o yaml
```

Expected shape:

```yaml
apiVersion: ar.dev/v1alpha1
kind: Runtime
metadata:
  name: virtual-default
spec:
  type: Virtual
```

If it's missing, the seed step didn't run (older installs, fresh installs from before the seed landed, etc.). Apply the asset:

```bash
arctl apply -f assets/mcp/agentgateway/virtual-default-runtime.yaml
```

## 3. Catalog the Remote MCP Server

The manifest at [`assets/mcp/agentgateway/github-copilot-remote-mcp.yaml`](assets/mcp/agentgateway/github-copilot-remote-mcp.yaml) catalogs GitHub Copilot's MCP as a remote streamable-HTTP entry with a `Bearer` token in the upstream auth header:

```bash
envsubst < assets/mcp/agentgateway/github-copilot-remote-mcp.yaml | arctl apply -f -
```

Verify it exists in the catalog:

```bash
arctl get mcp github-copilot-remote-mcp --tag latest -o yaml
```

> For demos, the token is rendered into the catalog entry. For production, use the secret mechanism supported by your agentregistry deployment instead of committing or sharing literal credentials.

> **Naming differs from [031](031-mcp-remote-github-copilot.md).** That lab catalogs the same upstream MCP as `github-copilot-mcp-server` (for the kagent runtime). This lab uses `github-copilot-remote-mcp` (for the Virtual runtime). They're independent catalog entries - you can have both registered simultaneously and they'll get deployed to different runtimes.

## 4. Deploy the Remote MCP to the Virtual Runtime

The `runtimeConfig.route.pathSuffix` is appended under the parent route's prefix. With `/registry` from step 1 and `pathSuffix: /github-copilot` here, the exposed path becomes:

```
/registry/github-copilot
```

```bash
arctl apply -f assets/mcp/agentgateway/github-copilot-remote-mcp-deploy.yaml
```

Verify the Deployment:

```bash
arctl get deployment github-copilot-remote-mcp-agw -o yaml
```

Look for:

- `Ready=True`
- `reason: DeployedViaAgentgateway`
- `status.details.agentgateway.exposedAt`

> `pathSuffix` must be **unique** across every Deployment that targets this `Virtual` runtime through the same parent route. Two deployments with the same suffix produce colliding child routes.

## 5. Inspect the Generated Agentgateway Resources

Agentregistry creates child resources in its install namespace whenever a `Virtual` runtime Deployment goes Ready:

```bash
kubectl -n agentregistry-system get httproutes.gateway.networking.k8s.io
kubectl -n agentregistry-system get enterpriseagentgatewaybackends.enterpriseagentgateway.solo.io
```

For troubleshooting:

```bash
kubectl -n agentregistry-system describe httproute
kubectl -n agentregistry-system describe enterpriseagentgatewaybackend.enterpriseagentgateway.solo.io
```

The child `HTTPRoute` is what the parent route's `backendRefs: [{kind: HTTPRoute, name: "*"}]` delegates to. The `EnterpriseAgentgatewayBackend` is what handles the upstream connection - including the `Authorization: Bearer ...` header from step 3 - to `api.githubcopilot.com`.

## 6. Get the Gateway Address

```bash
kubectl -n agentgateway-system get gateway remote-mcp-gateway
```

Depending on your environment, the address might appear on the `Gateway` status (`.status.addresses`) or on the Agentgateway-managed Service (`.status.loadBalancer.ingress`).

```bash
export AGW_ADDRESS="<gateway-address>"
```

## 7. Call the Exposed MCP Endpoint

The exact MCP request depends on which MCP client you use. As a basic MCP transport connectivity check, send a minimal JSON-RPC `initialize` request:

```bash
curl -i -X POST \
  -H "Accept: application/json, text/event-stream" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"curl","version":"0.0.1"}}}' \
  "http://${AGW_ADDRESS}/registry/github-copilot"
```

Expected: `HTTP/1.1 200 OK`, `content-type: text/event-stream`, and an `mcp-session-id` response header.

If you omit `text/event-stream`, the MCP endpoint can return `406 Not Acceptable` with `mcp: client must accept text/event-stream`. If you use a plain `GET`, it can return `400 Bad Request` with `mcp: session ID is required`. Both mean the Agentgateway route is reachable, but the request is not a valid MCP streamable-HTTP initialize request.

For real MCP traffic, point your MCP client at:

```
http://<gateway-address>/registry/github-copilot
```

## How This Compares to [031](031-mcp-remote-github-copilot.md)

Same upstream MCP server, three different ways to get an agent to talk to it:

| Topology | Runtime kind | How agents reach it | Best for |
|---|---|---|---|
| **kagent runtime** ([031](031-mcp-remote-github-copilot.md)) | `Kagent` | Each kagent agent dials the upstream directly via a generated `RemoteMCPServer` CR | Agents already running on kagent; tight per-agent control |
| **stdio sidecar** ([030](030-mcp-local-stdio.md)) | `Kagent` | Agent process spawns the MCP server locally via `command:` | Self-contained, no external network |
| **Virtual runtime + Agentgateway** (this lab) | `Virtual` | Any MCP client (kagent, Claude Code, external) hits a path on the shared gateway | Centralized policy, non-kagent clients, gateway-level auth/observability |

You can register the same upstream MCP under multiple catalog entries (one per topology) and deploy them simultaneously - they don't interfere.

## Troubleshooting

### `Deployment` has `NoGatewayBound`

The `agentregistry.solo.io/runtime` label is missing or mismatched. Confirm both the Gateway and the parent HTTPRoute carry it:

```bash
kubectl -n agentgateway-system get gateway remote-mcp-gateway --show-labels
kubectl -n agentgateway-system get httproute remote-mcp-delegate --show-labels
```

The label value must match the agentregistry runtime name exactly:

```
agentregistry.solo.io/runtime=virtual-default
```

### No child `HTTPRoute` was created

```bash
arctl get deployment github-copilot-remote-mcp-agw -o yaml
```

Common causes:

| Cause | Fix |
|---|---|
| `runtimeRef.name` doesn't match the Gateway/HTTPRoute label | Re-check both - case-sensitive |
| Parent HTTPRoute delegates to the wrong namespace | The `backendRefs[*].namespace` on the parent route must be `agentregistry-system` (or wherever agentregistry runs) |
| The MCP catalog entry is missing `spec.remote` | Re-apply step 3 and check `arctl get mcp ... -o yaml` |
| `runtimeConfig.route.pathSuffix` is missing or collides with another Deployment | Pick a unique suffix; check `arctl get deployments` for existing entries on the same runtime |

### Upstream TLS or auth fails

For an `https://` remote MCP URL, agentregistry configures Agentgateway to TLS to the upstream by default. If the upstream needs custom TLS (client certs, custom CAs, mTLS) or non-bearer auth handling, attach the appropriate Agentgateway policy to the generated `EnterpriseAgentgatewayBackend` in `agentregistry-system`.

### Two MCPs colliding at the same path

If two `Deployment`s used `pathSuffix: /github-copilot` against the same `Virtual` runtime and parent route, the second one's child route either fails to bind or one shadows the other. Pick distinct suffixes - `/github-copilot-v1` vs `/github-copilot-v2`, or use different parent routes with different prefixes if you want stable URLs across versions.

## Cleanup

```bash
arctl delete deployment github-copilot-remote-mcp-agw
arctl delete mcp github-copilot-remote-mcp --tag latest

kubectl -n agentgateway-system delete httproute remote-mcp-delegate
kubectl -n agentgateway-system delete gateway remote-mcp-gateway
```

Leave `Runtime/virtual-default` in place - it's the seeded default and other Deployments may rely on it.

## Next

- [040 - Prompts](040-prompts.md)
- [050 - AccessPolicy](050-access-policies.md) - add `registry:read` on `server` resources for the groups that should see this MCP in the catalog
- [060 - Observability / Tracing](060-observability-tracing.md)
