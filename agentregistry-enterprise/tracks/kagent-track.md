# Track - kagent + MCP

A focused path for someone who wants to register an existing in-cluster **kagent Enterprise** install as an agentregistry runtime, deploy the `k8shelper` BYO agent, and wire it to a remote **GitHub Copilot MCP** server.

## Estimated Time

- ~75 minutes end-to-end (the BYO image build is the longest step) - **add the time to install kagent Enterprise via its own workshop** if you don't have it on the cluster yet

## Prerequisites

- A Kubernetes cluster with a default `StorageClass` + working `LoadBalancer` Service controller
- **kagent Enterprise installed** in the `kagent` namespace via the [kagent-enterprise workshop](https://github.com/solo-io/field-agentic-labs/tree/main/kagent-enterprise) - labs 001 through 003
- A container registry your cluster can pull from (Docker Hub, GHCR, ECR, GAR, ACR, …) - you'll push your own k8shelper image
- A model API key (`ANTHROPIC_API_KEY` is the workshop default; Gemini variant available)
- A GitHub Copilot MCP access token
- `docker buildx`

## Order

1. **Install kagent Enterprise** via [its own workshop](https://github.com/solo-io/field-agentic-labs/tree/main/kagent-enterprise) (labs 001 - 003) if not already installed
2. [001 - Baseline Setup](../001-baseline-setup.md)
3. **Pick one OIDC path:** [002a - Keycloak](../002a-setup-oidc-keycloak.md) or [002b - Entra ID](../002b-setup-oidc-entra.md)
4. [003 - Install Components](../003-install-components.md) - installs agentregistry + Enterprise Agentgateway
5. [020 - kagent Runtime + k8shelper Agent](../020-kagent-runtime-and-agent.md) steps 1-4 - register the runtime, set the image, and create the model API-key Secret
6. [031 - Remote MCP via kagent (GitHub Copilot)](../031-mcp-remote-github-copilot.md) - register + deploy the MCP server referenced by the checked-in k8shelper Agent
7. Return to [020](../020-kagent-runtime-and-agent.md) step 5 - register and deploy the Agent
8. [060 - Tracing + Fan-Out](../060-observability-tracing.md) - keep traces on the kagent collector and forward them to Agentregistry too
9. [050 - AccessPolicy](../050-access-policies.md) - including the per-MCP-tool restriction examples
10. [099 - Cleanup](../099-cleanup.md)

## What You Will Have at the End

- Agentregistry Enterprise authenticated against your OIDC provider
- A `kagent` Runtime with generic telemetry pointed at Agentregistry and trace-specific export pointed at the kagent collector
- `k8shelper` deployed via the kagent runtime, calling Anthropic (or Gemini) with the API key from a Kubernetes Secret
- The GitHub Copilot MCP registered, deployed to kagent, and wired into `k8shelper`
- Spans from real chat invocations visible in both the kagent and Agentregistry UIs
- `AccessPolicy` records limiting which MCP tools `k8shelperanthropic-kagent` can invoke

## Next

- [AWS track](aws-track.md) - add Bedrock AgentCore as a second runtime
- [032 - MCP through Agentgateway](../032-mcp-through-agentgateway.md) - third MCP topology
