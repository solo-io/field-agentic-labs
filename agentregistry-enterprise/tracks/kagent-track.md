# Track — kagent + MCP

A focused path for someone who wants to register the in-cluster **kagent** install (from the baseline) as an AgentRegistry runtime, deploy the `k8shelper` BYO agent, and wire it to a remote **GitHub Copilot MCP** server.

## Estimated Time

- ~75 minutes end-to-end (the BYO image build is the longest step)

## Prerequisites

- A Kubernetes cluster with a default `StorageClass` + working `LoadBalancer` Service controller
- A container registry your cluster can pull from (Docker Hub, GHCR, ECR, GAR, ACR, …) — you'll push your own k8shelper image
- A model API key (`ANTHROPIC_API_KEY` is the workshop default; Gemini variant available)
- A GitHub Copilot MCP access token
- `docker buildx`

## Order

1. [001 — Baseline Setup](../001-baseline-setup.md)
2. **Pick one OIDC path:** [002a — Keycloak](../002a-setup-oidc-keycloak.md) or [002b — Entra ID](../002b-setup-oidc-entra.md)
3. [003 — Install Components](../003-install-components.md) — installs kagent in-cluster as part of the baseline
4. [020 — kagent Runtime + k8shelper Agent](../020-kagent-runtime-and-agent.md) — register the runtime + build/push your image + deploy the Agent
5. [031 — Remote MCP via kagent (GitHub Copilot)](../031-mcp-remote-github-copilot.md) — register + deploy the MCP server
6. [060 — Tracing](../060-observability-tracing.md) — set the kagent Runtime `spec.telemetryEndpoint`
7. [061 — Trace Fan-Out Workaround](../061-trace-fanout.md) *(if traces still don't show — see the fan-out vs repoint discussion in 060)*
8. [050 — AccessPolicy](../050-access-policies.md) — including the per-MCP-tool restriction examples
9. [099 — Cleanup](../099-cleanup.md)

## What You Will Have at the End

- AgentRegistry Enterprise authenticated against your OIDC provider
- A `kagent` Runtime with `spec.telemetryEndpoint` pointed at the AgentRegistry collector
- `k8shelper` deployed via the kagent runtime, calling Anthropic (or Gemini) with the API key from a Kubernetes Secret
- The GitHub Copilot MCP registered, deployed to kagent, and wired into `k8shelper`
- Spans from real chat invocations visible in the AgentRegistry UI
- `AccessPolicy` records limiting which MCP tools `k8shelperanthropic-kagent` can invoke

## Next

- [AWS track](aws-track.md) — add Bedrock AgentCore as a second runtime
- [032 — MCP through Agentgateway](../032-mcp-through-agentgateway.md) — third MCP topology
- [070 — Register Agents from a GitLab Pipeline](../070-gitops-gitlab-ci.md)
