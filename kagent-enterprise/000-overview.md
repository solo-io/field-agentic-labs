# Overview & Architecture

Solo Enterprise for kagent is Solo.io's enterprise build of kagent — the Kubernetes agent runtime — packaged with `AccessPolicy` for fine-grained authz, OIDC for UI and API login, MCP server lifecycle management, and integration with Solo Istio (Ambient), Gloo Gateway, and Enterprise Agentgateway. This workshop walks through installing it, layering it with MCP servers and agents, and applying policy + auth end-to-end.

## High-Level Picture

```
                    ┌────────────────────────┐
                    │  OIDC IdP              │
                    │  (Keycloak / Entra ID) │
                    └──────────┬─────────────┘
                               │ token (groups / preferred_username)
                               ▼
                    ┌────────────────────────────┐
   you (kubectl) ──▶│  Kagent Enterprise UI/API  │
                    │  (kagent namespace)        │
                    │                            │
                    │  ┌──────────────────────┐  │
                    │  │ controller (CRDs)    │  │
                    │  └──────────────────────┘  │
                    │  ┌──────────────────────┐  │
                    │  │ ui + postgres        │  │
                    │  └──────────────────────┘  │
                    └────────────┬───────────────┘
                                 │ reconciles
              ┌──────────────────┴─────────────────────┐
              ▼                                        ▼
    ┌──────────────────┐                  ┌──────────────────────┐
    │   Agents (CRD)   │  uses tools from │   MCPServers (CRD)   │
    │  Declarative /   │ ───────────────▶ │   stdio / streamable │
    │  BYO container   │                  │   kagent-managed pod │
    └──────────────────┘                  └──────────────────────┘
              ▲                                        ▲
              │ AccessPolicy (UserGroup→Agent)         │ AccessPolicy (Agent→Tool)
              │ + JWT @ waypoint (Istio Ambient)       │ enforced @ waypoint
              │                                        │
    ┌────────────────────────────────────────────────────────────┐
    │  Enterprise Agentgateway (agentgateway-system)             │
    │  • prompt guards • OBO token exchange (Entra)              │
    │  • routes /llm to in-cluster LLM proxy                     │
    └────────────────────────────────────────────────────────────┘
```

## Key CRDs

| API group | Kind | Purpose |
|---|---|---|
| `kagent.dev/v1alpha2` | `Agent` | An agent. `type: Declarative` (model config + system prompt + tool refs) or `type: BYO` (your own container image). |
| `kagent.dev/v1alpha1` / `v1alpha2` | `MCPServer` | An MCP server. kagent deploys it as a Pod (`deployment.image` + `cmd`/`args`) and wires it into agents. |
| `kagent.dev/v1alpha2` | `ModelConfig` | LLM provider + model + API key reference (`apiKeyPassthrough: true` enables OBO/key passthrough). |
| `policy.kagent-enterprise.solo.io/v1alpha1` | `AccessPolicy` | RBAC. Subject `Agent` → restrict which MCP tools the agent can call. Subject `UserGroup` → restrict which users can invoke an Agent based on JWT claims. |
| `enterpriseagentgateway.solo.io/v1alpha1` | `EnterpriseAgentgatewayPolicy` | Prompt guards, token exchange config (Entra OBO), per-route policy on the gateway. |
| `enterpriseagentgateway.solo.io/v1alpha1` | `EnterpriseAgentgatewayParameters` | Dataplane env (STS URI, STS auth token, logging level). |
| `gateway.kgateway.dev/v1alpha1` | `HTTPListenerPolicy` | Access logs (JSON) on the kgateway Gateway. |
| `gateway.networking.k8s.io/v1` | `Gateway` / `HTTPRoute` / `ReferenceGrant` | Standard Gateway API resources used by both Gloo Gateway (`gloo-system`) and Enterprise Agentgateway. |

## Two Install Models

The workshop covers two different ways to bring up kagent-enterprise — they are **not** interchangeable on the same cluster.

| Path | Chart(s) | Version | Lab |
|---|---|---|---|
| **Gloo Operator (canonical, full Solo stack)** | `gloo-operator` plus `KagentController` / `KagentManagementController` / `ServiceMeshController` / `GatewayController` CRs | Operator `0.4.0`, controller `0.1.5`, Istio Ambient `1.27.1`, Gloo Gateway `2.0.0` | [020](020-install-kagent-enterprise.md) |
| **Direct Helm (OBO scenario only)** | `kagent-mgmt` + `kagent-crds` + `kagent-enterprise` (and `enterprise-agentgateway`) | `0.3.12` (`enterprise-agentgateway` at `v2.2.0`) | [090](090-obo-entra.md) |

The Gloo Operator path is the canonical install for everything that doesn't need OBO. The direct-Helm path is what the OBO end-to-end was authored against. They installed from different chart streams during the workshop's authoring window — pick one per cluster, don't mix.

## Recommended Lab Flow

If you're doing the workshop linearly:

1. **[001](001-provision-gke.md)** Provision GKE (Terraform).
2. **[010](010-licenses-and-secrets.md)** Set up licenses, the `kagent` namespace, the OBO `jwt` Secret, and your LLM key.
3. **[020](020-install-kagent-enterprise.md)** Install Solo Enterprise for kagent via the Gloo Operator.
4. **[025](025-install-enterprise-agentgateway.md)** Install Enterprise Agentgateway.
5. **[030](030-access-logs.md)** Turn on JSON access logs.
6. **[040](040-mcp-connection-agent-config.md)** → **[041](041-agent-skills.md)** → **[042](042-build-custom-mcp-server.md)** MCP foundations.
7. **[050](050-troubleshooting-pod.md)** Smoke-test with the pre-built `k8s-agent`.
8. **[060](060-accesspolicy-agent-to-mcp.md)** → **[061](061-accesspolicy-usergroup.md)** Apply `AccessPolicy`.
9. **[070](070-prompt-guards.md)** → **[071](071-platform-rbac.md)** Guardrails and Kubernetes RBAC.
10. **[080](080-k8s-token-passthrough-pinniped.md)** *(independent)* Make `kubectl` itself OIDC.
11. **[090](090-obo-entra.md)** *(separate cluster recommended)* End-to-end Entra OBO.
12. **[099](099-cleanup.md)** Tear down.

The OBO lab ([090](090-obo-entra.md)) is end-to-end and re-installs both kagent and agentgateway. If you want the OBO lab specifically, follow the [obo-track](tracks/obo-track.md) which skips the Gloo Operator install.

## Asset Conventions

YAML, Python, and Terraform live under [`assets/`](assets/) and are referenced by relative path, e.g.:

```bash
kubectl apply -f assets/observability/kagent-gateway-access-logs.yaml
kubectl apply -f assets/obo/ui-https-gateway.yaml
```

Where a manifest contains environment variable placeholders (`${TENANT_ID}`, `${KAGENT_BACKEND_CLIENT_ID}`, etc.), the lab uses `envsubst` (or an explicit `python3` substitution for nested heredoc safety) before `kubectl apply`.

## Service & Port Reference

| Service | Namespace | Port | Purpose |
|---------|-----------|------|---------|
| `kagent-enterprise-ui` | `kagent` | 8090 | Solo Enterprise UI backend (Gloo Operator path) |
| `solo-enterprise-ui` | `kagent` | 80 | Solo Enterprise UI (direct-Helm path) |
| `kagent-controller` | `kagent` | 8083 | kagent controller API |
| `enterprise-agentgateway` | `agentgateway-system` | 7777 | Token exchange (STS) endpoint |
| `agentgateway-entra-testing` (Gateway) | `agentgateway-system` | 8080 / 443 | HTTP listener for `/llm` + HTTPS listener for UI |
| `llm-obo-proxy` | `agentgateway-system` | 8080 | In-cluster LLM proxy (OBO target) |
