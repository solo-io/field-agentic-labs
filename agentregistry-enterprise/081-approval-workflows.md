# Approval Workflows

When you grant a non-admin group `registry:publish` / `registry:edit` ([080](080-access-policies.md)), those users can submit new catalog assets — but a stricter setup is to require an **admin approval** on every submission before it lands in the production catalog. AgentRegistry Enterprise gates this with a single Helm config knob: `config.requireCreateApproval=true`. Once on, every `Agent`, `MCPServer`, `Skill`, and `Prompt` a non-admin submits goes into an **Administrative Request** queue that an admin approves (or rejects) from the UI or the `/v0/approve` HTTP API.

> **Scope:** approval gating covers catalog assets — `Agent`, `MCPServer`, `Skill`, `Prompt`. **`Deployment` resources are not approval-gated.** A user with `registry:read` on the runtime + `runtime:invoke` on an existing agent can still create deployments without admin sign-off.

## Lab Objectives

- Enable `config.requireCreateApproval=true` on an existing or fresh AgentRegistry install
- Grant a non-admin group writer-level access via `AccessPolicy` ([080](080-access-policies.md) refresher)
- Submit a catalog `Agent` as the non-admin user and confirm it lands in the approval queue, not the catalog
- Approve the request both from the UI and from the `/v0/approve` HTTP API
- Verify the approved asset shows up in the production catalog

## Prerequisites

- [030 — AgentRegistry Enterprise installed](030-install-agentregistry-helm.md)
- [040 — `arctl` authenticated](040-arctl-auth.md) with **both** an admin and a non-admin login available
- [080 — AccessPolicy](080-access-policies.md) — you'll grant `registry:publish` + `registry:edit` to a non-admin group
- A `Runtime` registered ([050](050-aws-provider.md) or [051](051-kagent-provider.md))
- A deployment of at least one agent in the catalog ([060](060-deploy-demochatbot-on-aws.md) or [061](061-deploy-k8shelper-on-kagent.md)) so you can see the UI flow against existing content

## 1. Enable the Feature Flag

The Helm value is `config.requireCreateApproval` on the `agentregistry-enterprise` chart.

### On an existing install

```bash
helm upgrade --install agentregistry-enterprise \
  oci://us-docker.pkg.dev/solo-public/agentregistry-enterprise/helm/agentregistry-enterprise \
  --version 2026.6.0 \
  --namespace agentregistry-system \
  --reuse-values \
  --set config.requireCreateApproval=true
```

> **`--reuse-values` preserves your existing OIDC, AWS, telemetry, and chart-image settings.** If you edited `/tmp/are-values.yaml` from [030](030-install-agentregistry-helm.md), add `-f /tmp/are-values.yaml` instead (or in addition) and drop `--reuse-values`. Mixing the two flags is fine as long as you understand `-f` wins on overlapping keys.

### On a fresh install

Add the same flag to your initial `helm upgrade --install` from [030](030-install-agentregistry-helm.md):

```yaml
# /tmp/are-values.yaml — append this block
config:
  requireCreateApproval: true
```

## 2. Verify the Flag Took

```bash
kubectl -n agentregistry-system get configmap agentregistry-enterprise \
  -o jsonpath='{.data.REQUIRE_CREATE_APPROVAL}{"\n"}'
```

Expected:

```
true
```

If you see nothing or `false`, the `helm upgrade` didn't reach the controller — confirm the release name + namespace, then `kubectl rollout status -n agentregistry-system deploy/agentregistry-enterprise`.

## 3. Grant a Non-Admin Group Writer Access

Approval workflows are only interesting if you have a non-admin user who can *submit* but not *commit*. Pick a group that isn't in `oidc.superuserRole`, then grant it `registry:publish` + `registry:edit` via `AccessPolicy`.

> **For Entra Groups**, the principal is the **object ID (GUID)**, not the display name. See [080 step 3](080-access-policies.md#worked-examples) for the GUID model. For Keycloak / Entra app roles, replace the GUID with the role value.

The parameterized manifest is at [`assets/access-policies/writer-group-policy.yaml`](assets/access-policies/writer-group-policy.yaml):

```bash
export GROUP_GUID="<your-non-admin-group-object-id>"
envsubst < assets/access-policies/writer-group-policy.yaml | arctl apply -f -
```

That manifest grants `registry:read` / `registry:publish` / `registry:edit` on `agent` / `server` / `runtime` resources to `Role:${GROUP_GUID}`. Without `requireCreateApproval=true`, those users would commit directly to the catalog. **With** it on, every submission becomes an Administrative Request.

Confirm:

```bash
arctl get accesspolicy writers-group-catalog-write -o yaml
```

## 4. Submit a Catalog Asset as the Non-Admin User

### Via the UI

1. Log into the UI as a member of the group you just granted.
2. Go to **Catalog** > **+ Create > Agent**.
3. Fill in any test agent (name, description, source, model — anything works for the approval test).
4. Click **Create**.

Instead of landing in the catalog, your new asset appears as an **Administrative Request** on the same Catalog view.

### Via `arctl`

The CLI flow exercises the same gate. The HTTP approval API works for catalog assets — `Agent`, `MCPServer`, `Skill`, `Prompt` — but not for `Deployment`.

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

Expected: `not found` (or equivalent) — the asset is in the approval queue.

## 5. List Pending Approval Requests

There is no `arctl approve` command (yet). Use the `/v0/approve` HTTP API with the bearer token from your CLI login.

```bash
export ARCTL_API_TOKEN=$(arctl user info --show-tokens | jq -r .access_token)

curl -s \
  -H "Authorization: Bearer ${ARCTL_API_TOKEN}" \
  "${ARCTL_API_BASE_URL}/v0/approve" | jq .
```

Expected response — a pending request for:

```text
kind: Agent
namespace: default
name: approval-test-agent
tag: 1.0.0
```

If `metadata.namespace` was omitted from the submitted YAML, the namespace defaults to `default`. Note whatever the API returns — you'll need the exact `kind` / `namespace` / `name` / `tag` tuple to approve.

## 6. Approve the Request

### Via the UI

1. Log into the UI as an **administrator** account (a member of `oidc.superuserRole`).
2. Go to **Catalog**.
3. The Administrative Request is listed. Click **Approve** (or **Reject**).

The approved asset moves from the request queue into the production catalog immediately.

### Via the HTTP API

Re-authenticate as the admin and refresh the token:

```bash
arctl user login \
  --oidc-issuer-url "$OIDC_ISSUER" \
  --oidc-client-id "$OIDC_CLIENT_ID"

arctl user whoami
export ARCTL_API_TOKEN=$(arctl user info --show-tokens | jq -r .access_token)
```

Approve the request — the `items[*]` tuple must match what `GET /v0/approve` returned in step 5:

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

The agent now returns successfully — it's a normal catalog entry. Any AccessPolicy that grants `registry:read` on `agent` resources will now surface it for users in that group.

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

If you need approval gating on deployments specifically, control it through `AccessPolicy` ([080](080-access-policies.md)) — only grant `runtime:invoke` on the agents you want users to be able to deploy.

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
  --version 2026.6.0 \
  --namespace agentregistry-system \
  --reuse-values \
  --set config.requireCreateApproval=false
```

## Troubleshooting

| Symptom | Fix |
|---|---|
| `REQUIRE_CREATE_APPROVAL` is still empty after `helm upgrade` | Wrong release name or namespace. Confirm with `helm list -n agentregistry-system`. Then `kubectl rollout status -n agentregistry-system deploy/agentregistry-enterprise`. |
| Non-admin user can still commit directly to the catalog | They're in `oidc.superuserRole`. `arctl user whoami` and double-check the mapped roles. Admins bypass the approval queue. |
| `/v0/approve` returns 401 | Token expired. Re-run `arctl user login` and re-export `ARCTL_API_TOKEN`. |
| `/v0/approve` returns 403 | You're authenticated but not an admin. Approve requires `oidc.superuserRole`. |
| `/v0/approve` POST returns 404 / empty `items` | The `kind` / `namespace` / `name` / `tag` tuple in your POST doesn't match what `GET /v0/approve` listed. Most often: wrong namespace (`default` vs the namespace in your submitted YAML). |
| Submitted Agent never shows up in the queue | Confirm the submitter has `registry:publish` (just `registry:read` isn't enough — the asset never gets created at all). Re-check the AccessPolicy in step 3. |

## Next

- [080 — AccessPolicy / RBAC](080-access-policies.md) — the foundation this lab builds on
- [099 — Cleanup](099-cleanup.md)
