# Expose a Remote MCP Server Through Agentgateway

A third deployment topology for the same GitHub Copilot remote MCP server from [071](071-register-github-copilot-mcp.md) — this time the MCP doesn't run *inside* a runtime like kagent at all. AgentRegistry catalogs it, then a `Virtual` runtime tells AgentRegistry to expose it as a child route on an **Agentgateway** Gateway. MCP clients hit a public path (`/registry/github-copilot`) on the gateway and the gateway brokers the connection to the upstream `https://api.githubcopilot.com/mcp`.

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
[ child HTTPRoute (agentregistry-system) ]   ← created by AgentRegistry
  │
  │ AgentgatewayBackend                      ← created by AgentRegistry
  ▼
remote MCP server (https://api.githubcopilot.com/mcp)
```

**Two ownership boundaries:**

| Owner | Owns |
|---|---|
| Gateway admin | The Kubernetes `Gateway` + the parent `HTTPRoute` that delegates `/registry` to children |
| AgentRegistry | The catalog `MCPServer`, the `Deployment` targeting `Runtime/virtual-default`, and the child `HTTPRoute` + `AgentgatewayBackend` it generates |

The `agentregistry.solo.io/runtime` label on the parent `Gateway` + parent `HTTPRoute` is what binds the two halves together — it tells AgentRegistry "any Deployment that targets `Runtime: virtual-default` should plumb its child route into the resources carrying this label."

## Lab Objectives

- Stand up a parent `Gateway` + parent `HTTPRoute` carrying the `agentregistry.solo.io/runtime: virtual-default` label
- Confirm the seeded `virtual-default` Runtime exists (or create it)
- Catalog the GitHub Copilot remote MCP server in AgentRegistry
- Deploy it to the `Virtual` runtime with a `pathSuffix`
- Verify the generated child `HTTPRoute` + `AgentgatewayBackend`
- Hit the exposed `/registry/github-copilot` path through the gateway

## Prerequisites

- [030 — AgentRegistry installed](030-install-agentregistry-helm.md) (assumed install namespace: `agentregistry-system`)
- [040 — `arctl` authenticated](040-arctl-auth.md)
- Enterprise Agentgateway installed with `GatewayClass: enterprise-agentgateway` and the Kubernetes Gateway API CRDs available
- A GitHub Copilot MCP access token, exported as `GITHUB_COPILOT_MCP_TOKEN`:

  ```bash
  export GITHUB_COPILOT_MCP_TOKEN="<github-token>"
  ```

## 1. Create the Parent Gateway and Route

The `agentregistry.solo.io/runtime` label binds these Kubernetes resources to an AgentRegistry `Virtual` runtime. The manifest at [`assets/mcp/agentgateway/parent-gateway-and-route.yaml`](assets/mcp/agentgateway/parent-gateway-and-route.yaml) creates both:

```bash
kubectl apply -f assets/mcp/agentgateway/parent-gateway-and-route.yaml
```

What it does:

- **`Gateway/remote-mcp-gateway`** — HTTP listener on `:80` in `agentgateway-system`, labeled `agentregistry.solo.io/runtime: virtual-default`
- **`HTTPRoute/remote-mcp-delegate`** — same label, parents to `remote-mcp-gateway`, delegates `/registry` to *any* child `HTTPRoute` (`name: "*"`) in the `agentregistry-system` namespace

That second part is the delegation: when AgentRegistry creates a child route under itself, it gets stitched into this parent route's `/registry` prefix automatically.

## 2. Confirm the `Virtual` Runtime Exists

AgentRegistry seeds `virtual-default` on startup. Confirm:

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

> For demos, the token is rendered into the catalog entry. For production, use the secret mechanism supported by your AgentRegistry deployment instead of committing or sharing literal credentials.

> **Naming differs from [071](071-register-github-copilot-mcp.md).** That lab catalogs the same upstream MCP as `github-copilot-mcp-server` (for the kagent runtime). This lab uses `github-copilot-remote-mcp` (for the Virtual runtime). They're independent catalog entries — you can have both registered simultaneously and they'll get deployed to different runtimes.

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

AgentRegistry creates child resources in its install namespace whenever a `Virtual` runtime Deployment goes Ready:

```bash
kubectl -n agentregistry-system get httproutes.gateway.networking.k8s.io
kubectl -n agentregistry-system get agentgatewaybackends.agentgateway.dev
```

For troubleshooting:

```bash
kubectl -n agentregistry-system describe httproute
kubectl -n agentregistry-system describe agentgatewaybackend
```

The child `HTTPRoute` is what the parent route's `backendRefs: [{kind: HTTPRoute, name: "*"}]` delegates to. The `AgentgatewayBackend` is what handles the upstream connection — including the `Authorization: Bearer ...` header from step 3 — to `api.githubcopilot.com`.

## 6. Get the Gateway Address

```bash
kubectl -n agentgateway-system get gateway remote-mcp-gateway
kubectl -n agentgateway-system get svc
```

Depending on your environment, the address might appear on the `Gateway` status (`.status.addresses`) or on the Agentgateway-managed Service (`.status.loadBalancer.ingress`).

```bash
export AGW_ADDRESS="<gateway-address>"
```

## 7. Call the Exposed MCP Endpoint

The exact MCP request depends on which MCP client you use. As a basic connectivity check:

```bash
curl -i "http://${AGW_ADDRESS}/registry/github-copilot"
```

For real MCP traffic, point your MCP client at:

```
http://<gateway-address>/registry/github-copilot
```

If the parent route has `hostnames` configured (this lab's doesn't), include the expected `Host` header:

```bash
curl -i \
  -H "Host: mcp.example.com" \
  "http://${AGW_ADDRESS}/registry/github-copilot"
```

## How This Compares to [071](071-register-github-copilot-mcp.md)

Same upstream MCP server, three different ways to get an agent to talk to it:

| Topology | Runtime kind | How agents reach it | Best for |
|---|---|---|---|
| **kagent runtime** ([071](071-register-github-copilot-mcp.md) + [072](072-wire-mcp-to-agent.md)) | `Kagent` | Each kagent agent dials the upstream directly via a generated `RemoteMCPServer` CR | Agents already running on kagent; tight per-agent control |
| **stdio sidecar** ([070](070-register-local-mcp.md)) | `Kagent` | Agent process spawns the MCP server locally via `command:` | Self-contained, no external network |
| **Virtual runtime + Agentgateway** (this lab) | `Virtual` | Any MCP client (kagent, Claude Code, external) hits a path on the shared gateway | Centralized policy, non-kagent clients, gateway-level auth/observability |

You can register the same upstream MCP under multiple catalog entries (one per topology) and deploy them simultaneously — they don't interfere.

## Troubleshooting

### `Deployment` has `NoGatewayBound`

The `agentregistry.solo.io/runtime` label is missing or mismatched. Confirm both the Gateway and the parent HTTPRoute carry it:

```bash
kubectl -n agentgateway-system get gateway remote-mcp-gateway --show-labels
kubectl -n agentgateway-system get httproute remote-mcp-delegate --show-labels
```

The label value must match the AgentRegistry runtime name exactly:

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
| `runtimeRef.name` doesn't match the Gateway/HTTPRoute label | Re-check both — case-sensitive |
| Parent HTTPRoute delegates to the wrong namespace | The `backendRefs[*].namespace` on the parent route must be `agentregistry-system` (or wherever AgentRegistry runs) |
| The MCP catalog entry is missing `spec.remote` | Re-apply step 3 and check `arctl get mcp ... -o yaml` |
| `runtimeConfig.route.pathSuffix` is missing or collides with another Deployment | Pick a unique suffix; check `arctl get deployments` for existing entries on the same runtime |

### Upstream TLS or auth fails

For an `https://` remote MCP URL, AgentRegistry configures Agentgateway to TLS to the upstream by default. If the upstream needs custom TLS (client certs, custom CAs, mTLS) or non-bearer auth handling, attach the appropriate Agentgateway policy to the generated `AgentgatewayBackend` in `agentregistry-system`.

### Two MCPs colliding at the same path

If two `Deployment`s used `pathSuffix: /github-copilot` against the same `Virtual` runtime and parent route, the second one's child route either fails to bind or one shadows the other. Pick distinct suffixes — `/github-copilot-v1` vs `/github-copilot-v2`, or use different parent routes with different prefixes if you want stable URLs across versions.

## Cleanup

```bash
arctl delete deployment github-copilot-remote-mcp-agw
arctl delete mcp github-copilot-remote-mcp --tag latest

kubectl -n agentgateway-system delete httproute remote-mcp-delegate
kubectl -n agentgateway-system delete gateway remote-mcp-gateway
```

Leave `Runtime/virtual-default` in place — it's the seeded default and other Deployments may rely on it.

## Next

- [075 — Prompt Quickstart](075-prompt-quickstart.md)
- [080 — AccessPolicy](080-access-policies.md) — add `registry:read` on `server` resources for the groups that should see this MCP in the catalog
