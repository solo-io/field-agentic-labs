# Track — Policy Deep Dive

A focused path through `AccessPolicy` (both subject kinds), prompt guards on the gateway, and Kubernetes RBAC for the kagent CRDs. Assumes you've already done the install track.

## Estimated Time

- ~60 minutes (after the install track is done)

## Prerequisites

- Completed the [install-track](install-track.md) — kagent + an MCP server + a working agent

For [061 (UserGroup AccessPolicy)](../061-accesspolicy-usergroup.md) you also need a Keycloak (or any OIDC IdP) whose tokens carry a `preferred_username` claim. If you don't have one, do [080](../080-k8s-token-passthrough-pinniped.md) first — it stands up Keycloak in-cluster — and reuse the same realm here.

For [070 (Prompt Guards)](../070-prompt-guards.md) you need [025 — Enterprise Agentgateway](../025-install-enterprise-agentgateway.md) installed.

## Order

1. [025 — Install Enterprise Agentgateway](../025-install-enterprise-agentgateway.md) *(if not already installed)*
2. [060 — `AccessPolicy`: Agent → MCP (Declarative + BYO)](../060-accesspolicy-agent-to-mcp.md)
3. [061 — `AccessPolicy`: UserGroup → Agent (OIDC JWT)](../061-accesspolicy-usergroup.md)
4. [070 — Prompt Guards](../070-prompt-guards.md)
5. [071 — Platform RBAC for kagent CRDs](../071-platform-rbac.md)
6. [099 — Cleanup](../099-cleanup.md) (just the policy/RBAC sections)

## What You Will Have at the End

- A working pattern for restricting which MCP tools an Agent can call (`Agent` subject, `DENY`/`ALLOW`)
- A working pattern for restricting which users can invoke an Agent (`UserGroup` subject, OIDC JWT validation at the waypoint)
- A prompt-guard policy rejecting requests matching a regex with **403 Forbidden** at the gateway
- A `ClusterRole` + `ClusterRoleBinding` giving a `ServiceAccount` read-only access to kagent CRDs, with a verification using `kubectl auth can-i`

## Mental Model

| Layer | What it controls | What it uses |
|---|---|---|
| Platform RBAC ([071](../071-platform-rbac.md)) | Who can apply / edit / delete kagent CRDs | K8s `ClusterRole` + `ClusterRoleBinding` |
| `AccessPolicy` (`UserGroup`, [061](../061-accesspolicy-usergroup.md)) | Which end-users can invoke an Agent | OIDC JWT claims at the Agent's waypoint |
| `AccessPolicy` (`Agent`, [060](../060-accesspolicy-agent-to-mcp.md)) | Which MCP tools an Agent can call | Per-tool `targetRef.tools` allow/deny at the MCP server's waypoint |
| Prompt guards ([070](../070-prompt-guards.md)) | Which request bodies even reach the LLM | Regex / webhook / built-in classifier at the gateway |

In production all four typically apply. Start from the inside out: tools first, then users, then content, then who manages the CRDs.

## Next

- [obo-track](obo-track.md) — Add Entra OBO for true end-to-end user identity through to the LLM
