# Plugins (Catalog Asset Quickstart)

`Plugin` is a first-class agentregistry catalog asset, same family as `Agent`, `MCPServer`, `Skill`, and `Prompt`. It is a **versioned pointer** to an external plugin bundle (today: a **git** repo URL + branch/commit, optional subfolder), plus metadata (title, description, harnesses such as `claude-code`).

Manage plugins with **`arctl`**, not `kubectl` — they live in the agentregistry catalog, not in etcd as cluster CRDs.

**Important (v2026.7.1 UI):** On Catalog, **+ Create** typically offers Agent, MCP Server, Skill, and Prompt only. There is often **no Plugin** entry in that menu on this release, even though the **API** exposes `/v0/plugins`. Use **YAML + `arctl apply`** for this lab.

This is a short catalog-asset lab: create → list → verify Ready → delete. About ten minutes end-to-end.

## Lab Objectives

- Understand what a Plugin is (and is not)
- Apply a `Plugin` from this repo's assets
- List and inspect it with `arctl` / authenticated API
- Confirm the controller resolves the git source to Ready
- Delete it

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) → [002a](002a-setup-oidc-keycloak.md) **or** [002b](002b-setup-oidc-entra.md) → [003](003-install-components.md)
- Enterprise `arctl` configured against your agentregistry URL (from baseline)
- Agentregistry Enterprise **v2026.7.1** or newer (Plugins API present)

## What a Plugin is (and is not)

| | |
|--|--|
| **Is** | Catalog artifact: git (or future OCI) **source pointer** + metadata |
| **Is not** | A Kubernetes Deployment you run with `kubectl apply` |
| **Controller** | Resolves the ref to a concrete **commit**, scans manifest/inventory into **status** |
| **Bytes** | Not hosted by the registry; harnesses materialize the pin at use/deploy time |

Phase-1 model: source pointer + resolve. Prefer a public git repo the registry can clone.

---

## 1. List Plugins

```bash
arctl get plugins
```

On a fresh install you may see none, or only what your environment already registered.

---

## 2. Create a Plugin

The manifest is at [`assets/plugins/demo-formatter.yaml`](assets/plugins/demo-formatter.yaml):

```yaml
apiVersion: ar.dev/v1alpha1
kind: Plugin
metadata:
  name: demo-formatter
  tag: "1.0.0"
spec:
  title: Demo Formatter
  description: Sample plugin registered from a public git repo (lite workshop lab).
  harnesses:
    - claude-code
  source:
    type: git
    git:
      repository:
        url: https://github.com/solo-io/agentregistry-dev-samples
        branch: main
```

Apply it:

```bash
arctl apply -f assets/plugins/demo-formatter.yaml
```

Expected (wording may vary slightly by CLI version):

```
Plugin/demo-formatter (1.0.0) created
```

> **If [approval workflows](051-approval-workflows.md) are enabled** and you are not an admin, the result may be `staged` instead of `created`. Approve the request as an admin before the plugin is fully registered. Confirm whether `Plugin` is gated in your build the same way as `Agent` / `MCPServer` / `Skill` / `Prompt`.

---

## 3. Verify the Plugin

```bash
arctl get plugins
arctl get plugin demo-formatter --tag "1.0.0" -o yaml
```

Look for:

- Spec fields you applied (title, description, git URL, harnesses)
- **Status Ready** (or equivalent condition) once the controller pins a **commit**
- `resolvedSource` / commit SHA when resolution succeeds

Authenticated REST (optional):

```bash
# After you have a bearer token from your login flow:
curl -sS -H "Authorization: Bearer $TOKEN" \
  "${AGENTREGISTRY_URL}/v0/plugins?limit=50"

curl -sS -H "Authorization: Bearer $TOKEN" \
  "${AGENTREGISTRY_URL}/v0/plugins/demo-formatter"
```

Unauthenticated `GET /v0/plugins` returns **401** on Enterprise — expected.

---

## 4. How plugins get “used” (not a standalone deploy)

Plugins are **catalog resources**. There is no separate “deploy this plugin as a pod” step in this lab.

| Goal | How |
|------|-----|
| **Register** | `arctl apply` (this lab) |
| **Resolve** | Controller pins git → Ready |
| **Consume** | Reference from an Agent / harness (`spec.plugins: [{ kind: Plugin, name: …, tag: … }]`) so deploy-time materialization can install the pin into the harness layout |

That full agent-wiring path is out of scope here; this lab stops at a healthy catalog entry.

---

## Why Plugins Are a Catalog Asset

| Concern | Ad-hoc git URL in a harness config | `Plugin` catalog asset |
|---|---|---|
| Version pinning | Moving branch tip | Named + `tag`, pin commit in status |
| Reuse | Copy/paste URLs | Reference by `name` + `tag` |
| Access control | Implicit | Can participate in catalog RBAC / approvals (build-dependent) |
| Auditability | Buried in agent/harness config | Top-level catalog + API list |

---

## Troubleshooting

| Symptom | Check |
|--------|--------|
| No Plugin in UI **+ Create** | Expected on v2026.7.1 Catalog chrome — use `arctl apply` |
| **401** on `/v0/plugins` | Log in; use `arctl` session or Bearer token |
| Never **Ready** | Repo public/reachable from the registry; branch valid; server logs on `agentregistry-enterprise-server` |
| Apply rejected | DNS-1123 `metadata.name`; `source.type: git` and `repository.url` set |

---

## Cleanup

```bash
arctl delete plugin demo-formatter --tag "1.0.0"
```

If delete syntax differs slightly in your `arctl` build, use `arctl delete --help` and the same name/tag you applied.

---

## Next

- [040 - Prompts](040-prompts.md) - another short catalog-asset lab
- [050 - AccessPolicy](050-access-policies.md) - catalog RBAC (resource kinds include `plugin` in the authz model)
- [051 - Approval Workflows](051-approval-workflows.md) - staged vs created for catalog submissions
- [020 - kagent Runtime + Agent](020-kagent-runtime-and-agent.md) - agents that can reference catalog assets
