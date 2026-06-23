# Approval Workflows

When you grant a non-admin group `registry:publish` / `registry:edit` ([050](050-access-policies.md)), those users can submit new catalog assets - but a stricter setup is to require an **admin approval** on every submission before it lands in the production catalog. Agentregistry Enterprise gates this with a single Helm config knob: `config.requireCreateApproval=true`. Once on, every `Agent`, `MCPServer`, `Skill`, and `Prompt` a non-admin submits goes into an **Administrative Request** queue that an admin approves (or rejects) from the UI or the `/v0/approve` HTTP API.

> **Scope:** approval gating covers catalog assets - `Agent`, `MCPServer`, `Skill`, `Prompt`. **`Deployment` resources are not approval-gated.** A user with `registry:read` on the runtime + `runtime:invoke` on an existing agent can still create deployments without admin sign-off.

## Lab Objectives

- Enable `config.requireCreateApproval=true` on an existing or fresh agentregistry install
- Grant a non-admin group writer-level access via `AccessPolicy` ([050](050-access-policies.md) refresher)
- Submit a catalog `Agent` as the non-admin user and confirm it lands in the approval queue, not the catalog
- Approve the request both from the UI and from the `/v0/approve` HTTP API
- Verify the approved asset shows up in the production catalog

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) → [002a](002a-setup-oidc-keycloak.md) **or** [002b](002b-setup-oidc-entra.md) → [003](003-install-components.md)
- You need **two** OIDC users available: an admin (in `are-admins`) and a non-admin (in `are-readers` or `are-writers`). Both 002a (Keycloak `admin` / `reader` / `writer` users) and 002b (groups + group memberships) cover this.
- Familiarity with [050 - AccessPolicy](050-access-policies.md). The first step of this lab grants writer-level permissions to a non-admin group; that's exactly the catalog-write pattern from 050.
- A `Runtime` registered ([010](010-aws-bedrock-runtime.md) or [020](020-kagent-runtime-and-agent.md)) - optional, just so you have something interesting to look at in the catalog

## 1. Enable the Feature Flag

The Helm value is `config.requireCreateApproval` on the `agentregistry-enterprise` chart.

```bash
helm upgrade --install agentregistry-enterprise \
  oci://us-docker.pkg.dev/solo-public/agentregistry-enterprise/helm/agentregistry-enterprise \
  --version 2026.6.1 \
  --namespace agentregistry-system \
  --reuse-values \
  --set config.requireCreateApproval=true
```

> **`--reuse-values` preserves your OIDC, telemetry, and chart-image settings from [003](003-install-components.md).** If `/tmp/are-values.yaml` from 003 is still on disk, you can use `-f /tmp/are-values.yaml --set config.requireCreateApproval=true` instead - same result.

## 2. Verify the Flag Took

```bash
kubectl -n agentregistry-system get configmap agentregistry-enterprise \
  -o jsonpath='{.data.REQUIRE_CREATE_APPROVAL}{"\n"}'
```

Expected:

```
true
```

If you see nothing or `false`, the `helm upgrade` didn't reach the controller - confirm the release name + namespace, then `kubectl rollout status -n agentregistry-system deploy/agentregistry-enterprise-server`.

## 3. Grant a Non-Admin Group Writer Access

Approval workflows are only interesting if you have a non-admin user who can *submit* but not *commit*. Pick a group that isn't in `oidc.superuserRole`, then grant it `registry:publish` + `registry:edit` via `AccessPolicy`.

> **The principal is the GUID** you exported in 002a/002b as `${GROUP_READERS}` (or `${GROUP_WRITERS}`). See [050 - AccessPolicy](050-access-policies.md) for the full principal model.

The parameterized manifest is at [`assets/access-policies/writer-group-policy.yaml`](assets/access-policies/writer-group-policy.yaml):

```bash
export GROUP_GUID="${GROUP_READERS}"   # or GROUP_WRITERS — any non-admin group works
envsubst < assets/access-policies/writer-group-policy.yaml | arctl apply -f -
```

That manifest grants `registry:read` / `registry:publish` / `registry:edit` on `agent` / `server` / `runtime` resources to `Role:${GROUP_GUID}`. Without `requireCreateApproval=true`, those users would commit directly to the catalog. **With** it on, every submission becomes an Administrative Request.

Confirm:

```bash
arctl get accesspolicy writers-group-catalog-write -o yaml
```

## 4. Submit a Catalog Asset as the Non-Admin User

Pick **either** the UI flow or the CLI flow below. The result is the same - a pending Administrative Request - and the rest of the lab works regardless of which submission path you took.

### UI submission

1. Log into the UI as a member of the group you just granted.
2. Go to **Catalog** > **+ Create > Agent**.
3. Fill in any test agent (name, description, source, model - anything works for the approval test).
4. Click **Create**.

Instead of landing in the catalog, your new asset appears as an **Administrative Request** on the same Catalog view. Jump to [Step 6 - UI approval](#ui-approval).

### CLI submission (`arctl`)

The CLI flow exercises the same gate. Approval gating works for catalog assets - `Agent`, `MCPServer`, `Skill`, `Prompt` - but not for `Deployment`.

```bash
# Make sure you're logged in as the NON-admin user
arctl user login \
  --oidc-issuer-url "$OIDC_ISSUER" \
  --oidc-client-id "$OIDC_CLIENT_ID"

arctl user whoami
```

Confirm `whoami` shows the expected non-admin group/role and that the user is **not** in `oidc.superuserRole`. Then submit:

```bash
arctl apply -f - <<EOF
apiVersion: ar.dev/v1alpha1
kind: Agent
metadata:
  name: approval-test-agent
  tag: "1.0.0"
spec:
  title: approval-test-agent
  description: "Test agent for approval workflow validation"
  modelProvider: anthropic
  modelName: claude-sonnet-4-6
  source:
    image: docker.io/python:3.13-slim
EOF
```

The asset is **staged for approval**, not committed.

Confirm it's not visible as a normal catalog item yet:

```bash
arctl get agent approval-test-agent --tag 1.0.0
```

Expected: `not found` (or equivalent) - the asset is in the approval queue.

## 5. List Pending Approval Requests (CLI)

> If you submitted via the UI in step 4, you can skip straight to [Step 6 - UI approval](#ui-approval). Steps 5 and "[Step 6 - HTTP API approval](#http-api-approval)" walk through the CLI-only path because there is no `arctl approve` subcommand yet.

Use the `/v0/approve` HTTP API with the bearer token from your CLI login.

```bash
export ARCTL_API_TOKEN=$(arctl user info --show-tokens | jq -r .access_token)

curl -s \
  -H "Authorization: Bearer ${ARCTL_API_TOKEN}" \
  "${ARCTL_API_BASE_URL}/v0/approve" | jq .
```

Expected response - a pending request for:

```text
kind: Agent
namespace: default
name: approval-test-agent
tag: 1.0.0
```

If `metadata.namespace` was omitted from the submitted YAML, the namespace defaults to `default`. Note whatever the API returns - you'll need the exact `kind` / `namespace` / `name` / `tag` tuple to approve.

## 6. Approve the Request

Two paths - pick whichever matches how you submitted in step 4. The admin user needs to be in `oidc.superuserRole` for either.

### UI approval

1. Log into the UI as an **administrator** account (a member of `oidc.superuserRole`).
2. Go to **Catalog**.
3. The Administrative Request is listed. Click **Approve** (or **Reject**).

The approved asset moves from the request queue into the production catalog immediately. Jump to [Step 7](#7-verify-the-asset-is-in-the-catalog).

### HTTP API approval

Re-authenticate as the admin and refresh the token:

```bash
arctl user login \
  --oidc-issuer-url "$OIDC_ISSUER" \
  --oidc-client-id "$OIDC_CLIENT_ID"

arctl user whoami
export ARCTL_API_TOKEN=$(arctl user info --show-tokens | jq -r .access_token)
```

Approve the request - the `items[*]` tuple must match what `GET /v0/approve` returned in step 5:

```bash
curl -s -X POST \
  -H "Authorization: Bearer ${ARCTL_API_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
        "action": "approve",
        "items": [
          {"kind":"Agent","namespace":"default","name":"approval-test-agent","tag":"1.0.0"}
        ]
      }' \
  "${ARCTL_API_BASE_URL}/v0/approve" | jq .
```

`action` accepts `approve` or `reject`. Reject removes the request from the queue without committing.

## 7. Verify the Asset Is in the Catalog

```bash
arctl get agent approval-test-agent --tag 1.0.0
```

The agent now returns successfully - it's a normal catalog entry. Any AccessPolicy that grants `registry:read` on `agent` resources will now surface it for users in that group.

## What Approval Gating Covers (and Doesn't)

| Resource kind | Approval-gated? | Notes |
|---|---|---|
| `Agent` | Yes | New catalog entries + edits |
| `MCPServer` | Yes | Same |
| `Skill` | Yes | Same |
| `Prompt` | Yes | Same |
| `Deployment` | **No** | A user with `runtime:invoke` on an existing agent + `registry:read` on a runtime can create deployments directly. Approval gating is about what enters the catalog, not what gets deployed from it. |
| `Runtime` | Yes (via `registry:publish` + `registry:edit`) | Submitting / editing a Runtime is treated the same as any catalog asset |
| `AccessPolicy` | No | AccessPolicy management is an admin-only path; non-admins shouldn't have write access to it in the first place |

If you need approval gating on deployments specifically, control it through `AccessPolicy` ([050](050-access-policies.md)) - only grant `runtime:invoke` on the agents you want users to be able to deploy.

## Cleanup

```bash
# Remove the test agent (if approval was granted)
arctl delete agent approval-test-agent --tag 1.0.0

# Or reject any still-pending request from step 4
export ARCTL_API_TOKEN=$(arctl user info --show-tokens | jq -r .access_token)
curl -s -X POST \
  -H "Authorization: Bearer ${ARCTL_API_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
        "action": "reject",
        "items": [
          {"kind":"Agent","namespace":"default","name":"approval-test-agent","tag":"1.0.0"}
        ]
      }' \
  "${ARCTL_API_BASE_URL}/v0/approve" | jq .

# Remove the writer-group policy
arctl delete accesspolicy writers-group-catalog-write

# Disable the feature flag (optional — flipping it off does not retroactively
# release queued requests; clear those with reject first)
helm upgrade --install agentregistry-enterprise \
  oci://us-docker.pkg.dev/solo-public/agentregistry-enterprise/helm/agentregistry-enterprise \
  --version 2026.6.1 \
  --namespace agentregistry-system \
  --reuse-values \
  --set config.requireCreateApproval=false
```

## Troubleshooting

| Symptom | Fix |
|---|---|
| `REQUIRE_CREATE_APPROVAL` is still empty after `helm upgrade` | Wrong release name or namespace. Confirm with `helm list -n agentregistry-system`. Then `kubectl rollout status -n agentregistry-system deploy/agentregistry-enterprise-server`. |
| Non-admin user can still commit directly to the catalog | They're in `oidc.superuserRole`. `arctl user whoami` and double-check the mapped roles. Admins bypass the approval queue. |
| `/v0/approve` returns 401 | Token expired. Re-run `arctl user login` and re-export `ARCTL_API_TOKEN`. |
| `/v0/approve` returns 403 | You're authenticated but not an admin. Approve requires `oidc.superuserRole`. |
| `/v0/approve` POST returns 404 / empty `items` | The `kind` / `namespace` / `name` / `tag` tuple in your POST doesn't match what `GET /v0/approve` listed. Most often: wrong namespace (`default` vs the namespace in your submitted YAML). |
| Submitted Agent never shows up in the queue | Confirm the submitter has `registry:publish` (just `registry:read` isn't enough - the asset never gets created at all). Re-check the AccessPolicy in step 3. |

## Next

- [050 - AccessPolicy / RBAC](050-access-policies.md) - the foundation this lab builds on
- [060 - Observability / Tracing](060-observability-tracing.md)
