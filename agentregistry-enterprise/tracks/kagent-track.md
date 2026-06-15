# Track — kagent + MCP

A focused path through the workshop for someone who wants to run AgentRegistry Enterprise with **Keycloak** OIDC, register an existing **kagent** install as a runtime, deploy the `k8shelper` agent, and wire it to a remote **GitHub Copilot MCP** server. Skips the AWS / Entra / private-EKS labs.

## Estimated Time

- ~90 minutes end-to-end on a managed Kubernetes cluster with kagent already installed
- Add ~30 minutes to deploy Keycloak from scratch via [021](../021-setup-keycloak.md)

## Prerequisites

- A Kubernetes cluster with a default `StorageClass` (or follow [010](../010-cluster-prereqs.md))
- An existing **kagent** install in the `kagent` namespace
- A container registry your cluster can pull from
- A model API key (`GOOGLE_API_KEY` for the Gemini variant, `ANTHROPIC_API_KEY` for the Claude variant)
- A GitHub Copilot MCP access token

## Order

1. [001 — Install `arctl`](../001-install-arctl.md)
2. [021 — Keycloak OIDC](../021-setup-keycloak.md) *(skip if you have an external IdP — just export the equivalent values)*
3. [030 — Install AgentRegistry Enterprise (Helm)](../030-install-agentregistry-helm.md) — use the **Keycloak** values block in section 2b
4. [040 — Authenticate `arctl`](../040-arctl-auth.md) — use the **`arctl user login`** flow in section 2
5. [051 — kagent Runtime](../051-kagent-provider.md)
6. [061 — Deploy `k8shelper` on kagent](../061-deploy-k8shelper-on-kagent.md) — pick the Gemini **or** Anthropic variant
7. [071 — Register the GitHub Copilot MCP](../071-register-github-copilot-mcp.md)
8. [072 — Wire the MCP to the Agent](../072-wire-mcp-to-agent.md)
9. [090 — Tracing Setup](../090-observability-tracing.md) — set the kagent Runtime `spec.telemetryEndpoint`
10. [091 — Trace Fan-Out Workaround](../091-trace-fanout-workaround.md) *(if traces still don't show — see the fan-out vs repoint trade-off discussion in 090)*
11. [080 — AccessPolicy / RBAC](../080-access-policies.md) — including the per-MCP-tool restrictions
12. [099 — Cleanup](../099-cleanup.md)

## What You Will Have at the End

- AgentRegistry Enterprise on Kubernetes, authenticated against Keycloak (or your external IdP)
- A `kagent` Runtime with telemetry pointed at the AgentRegistry collector
- `k8shelper` deployed via the kagent runtime, calling Gemini (or Claude) with `ANTHROPIC_API_KEY` / `GOOGLE_API_KEY` from a Kubernetes Secret
- The GitHub Copilot MCP registered, deployed to kagent, and exposed to `k8shelper` via `mcpServers:` + `deploymentRefs:`
- `list_available_tools` returning the GitHub MCP tool set (without `issue_write`)
- Spans from real chat invocations visible in the AgentRegistry UI
- `AccessPolicy` records limiting which MCP tools `k8shelper-kagent` can invoke

## Next

- [AWS track](aws-track.md) — to add Bedrock AgentCore as a second runtime
- [095 — Register Agents from a GitLab Pipeline](../095-gitops-gitlab-ci.md)
