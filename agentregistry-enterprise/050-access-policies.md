# AccessPolicy — RBAC for the Catalog, Chat, and MCP Tools

AgentRegistry Enterprise enforces RBAC with `AccessPolicy` resources that map an OIDC principal (an Entra group object ID, an Entra app role value, a Keycloak group name, or a `Deployment` identity) to a set of actions on catalog resources.

This lab works through the policy model and shows worked examples for catalog read, catalog write, end-user chat, and per-MCP-tool agent runtime access.

## Lab Objectives

- Understand the `registry:*` vs `runtime:invoke` split
- Grant `are-admins` catalog read and write access
- Grant `are-admins` chat access against the `k8shelper` agent
- Restrict a deployment's MCP tool access (`Deployment` principal + `subresources: [tool/<name>]`)
- Verify with `arctl user whoami` and `arctl get accesspolicies`

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) → [002a](002a-setup-oidc-keycloak.md) **or** [002b](002b-setup-oidc-entra.md) → [003](003-install-components.md)
- The OIDC `groups` claim from 002a/002b is plumbed through (your `${GROUP_ADMINS}` / `${GROUP_READERS}` / `${GROUP_WRITERS}` env vars are still set)
- Some labs reference the `k8shelper` Agent from [020](020-kagent-runtime-and-agent.md) and the `github-copilot-mcp-server` MCP from [031](031-mcp-remote-github-copilot.md) — apply those first if you want to run the chat-access and MCP-tool examples below

## Policy Model

Two scope families, intentionally separate:

| Scope | Used for |
|---|---|
| `registry:*` | Agent Registry **control-plane** access: catalog visibility and CRUD on `Agent` / `MCPServer` / `Runtime` / `AccessPolicy` records. |
| `registry:read` | What list/get filtering uses. Lets a user **see** matching catalog resources, but does not grant chat or runtime invocation. |
| `runtime:invoke` (on `agent`) | Controls **chat / A2A** against the catalog `agent` that a `Deployment` targets. |
| `runtime:invoke` (on `server`) | Controls **MCP invocation** by runtime components (deployed agents, gateway-backed MCP traffic). |

Code-backed references in the AgentRegistry server tree:

- `internal/registry/authz/engine.go` — list filtering uses `registry:read`
- `internal/registry/api/handlers/a2a.go` — chat/A2A authorizes `runtime:invoke` on the deployment target `agent`
- `internal/accesspolicy/kagent/plan.go` — kagent fan-out only translates `runtime:invoke`; `Role` principals are dropped for kagent CRD fan-out
- `internal/agwsync/config_translate.go` — agentgateway authorization only emits rules for `runtime:invoke`

## Worked Examples

These examples reference the `are-admins` group identifier you exported in 002a/002b as `${GROUP_ADMINS}`. The value differs by IdP:

| OIDC backend | `${GROUP_ADMINS}` is | Notes |
|---|---|---|
| Keycloak ([002a](002a-setup-oidc-keycloak.md)) | The group's GUID (from the Keycloak admin API) | The `groups` claim emits `/are-admins` but `AccessPolicy` principals match against the GUID you exported |
| Entra ID ([002b](002b-setup-oidc-entra.md)) | The group's object ID (GUID) | Entra emits group object IDs in the `groups` claim |

If you're starting fresh in a new shell, re-export the GROUP_* values from 002a or 002b before running the examples below.

### 1. Catalog Read Access

Members of `are-admins` can read matching catalog resources. Deletion is intentionally omitted.

```bash
arctl apply -f - <<EOF
apiVersion: ar.dev/v1alpha1
kind: AccessPolicy
metadata:
  name: are-admins-read-catalog
spec:
  description: "Catalog read access for the are-admins group"
  principals:
    - kind: Role
      name: "${GROUP_ADMINS}"
  rules:
    - actions:
        - "registry:read"
      resources:
        - { kind: skill,  name: "*" }
        - { kind: server, name: "*" }
        - { kind: prompt, name: "*" }
EOF
```

### 2. Catalog Write Access

```bash
arctl apply -f - <<EOF
apiVersion: ar.dev/v1alpha1
kind: AccessPolicy
metadata:
  name: are-admins-catalog-write
spec:
  description: "Catalog read, publish, and edit access for the are-admins group"
  principals:
    - kind: Role
      name: "${GROUP_ADMINS}"
  rules:
    - actions:
        - "registry:read"
        - "registry:publish"
        - "registry:edit"
      resources:
        - { kind: agent,   name: "*" }
        - { kind: server,  name: "*" }
        - { kind: runtime, name: "*" }
EOF
```

### 3. End-User Chat Access

Chatting with an agent requires `runtime:invoke` on the catalog `agent` that the `Deployment` targets. The A2A handler resolves the `Deployment` to `targetRef.name` and checks this permission against the user's `Role` principals.

```bash
arctl apply -f - <<EOF
apiVersion: ar.dev/v1alpha1
kind: AccessPolicy
metadata:
  name: are-admins-k8shelper-chat
spec:
  description: "Allow are-admins users to invoke the k8shelper agent"
  principals:
    - kind: Role
      name: "${GROUP_ADMINS}"
  rules:
    - actions:
        - "runtime:invoke"
      resources:
        - { kind: agent, name: k8shelper }
EOF
```

### 4. Agent Runtime Access to MCP Tools

This controls what a **deployed agent** can invoke at runtime. Use a `Deployment` principal for the deployed agent and grant `runtime:invoke` on the MCP `server`.

Omitting `subresources` allows all tools on the server. Adding `subresources` limits access to specific MCP tools — each entry must use the prefix `tool/<name>`.

Allow all tools on `github-copilot-mcp-server`:

```bash
arctl apply -f - <<EOF
apiVersion: ar.dev/v1alpha1
kind: AccessPolicy
metadata:
  name: k8shelper-github-copilot-tools
spec:
  description: "Allow the k8shelper deployment to invoke GitHub Copilot MCP tools"
  principals:
    - kind: Deployment
      name: k8shelper-kagent
  rules:
    - actions:
        - "runtime:invoke"
      resources:
        - { kind: server, name: github-copilot-mcp-server }
EOF
```

Or limit to a specific tool:

```bash
arctl apply -f - <<EOF
apiVersion: ar.dev/v1alpha1
kind: AccessPolicy
metadata:
  name: k8shelper-github-copilot-create-issue
spec:
  description: "Allow the k8shelper deployment to invoke only selected GitHub Copilot MCP tools"
  principals:
    - kind: Deployment
      name: k8shelper-kagent
  rules:
    - actions:
        - "runtime:invoke"
      resources:
        - kind: server
          name: github-copilot-mcp-server
          subresources:
            - tool/create_issue
EOF
```

## Verify

```bash
# Confirm your mapped roles contain the principals used above
arctl user whoami

# List applied policies
arctl get accesspolicies

# Catalog visibility is controlled by registry:read
arctl get agents
arctl get runtimes

# User chat/invoke requires runtime:invoke on the target catalog agent.
# If catalog read works but chat fails, double-check the User Chat Access policy.
```

## Choosing a Principal Kind

| Principal | When to use it |
|---|---|
| `Role` | The user authenticates via OIDC and their token carries a group/role claim. The `name` is the claim value (GUID for Entra groups, role value for Entra app roles, group name for Keycloak). |
| `Deployment` | The caller is a deployed agent acting on its own behalf — for example, `k8shelper-kagent` calling an MCP server. The `name` is the AgentRegistry `Deployment` name. |

## Cleanup

Remove any policies you applied above:

```bash
arctl delete accesspolicy are-admins-read-catalog            2>/dev/null || true
arctl delete accesspolicy are-admins-catalog-write           2>/dev/null || true
arctl delete accesspolicy are-admins-k8shelper-chat          2>/dev/null || true
arctl delete accesspolicy k8shelper-github-copilot-tools     2>/dev/null || true
arctl delete accesspolicy k8shelper-github-copilot-create-issue 2>/dev/null || true
```

## Next

- [051 — Approval Workflows](051-approval-workflows.md) — gate every catalog submission behind admin approval
- [060 — Observability / Tracing](060-observability-tracing.md)
