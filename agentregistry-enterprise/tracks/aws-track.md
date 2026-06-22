# Track - AWS Bedrock AgentCore

A focused path for someone who wants to run agentregistry Enterprise and deploy an agent to **AWS Bedrock AgentCore**. Skips the kagent BYO-image lab and the MCP labs.

## Estimated Time

- ~60 minutes end-to-end on an existing managed Kubernetes cluster
- ~90 minutes if you have to bring up the cluster + sort out IAM permissions

## Prerequisites

- A Kubernetes cluster with a default `StorageClass` + working `LoadBalancer` Service controller
- An OIDC provider - either an Entra ID tenant (preferred for an AWS-heavy team) or Keycloak in-cluster
- An AWS account where you can create IAM roles + a CloudFormation stack

## Order

1. [001 - Baseline Setup](../001-baseline-setup.md)
2. **Pick one OIDC path:** [002a - Keycloak](../002a-setup-oidc-keycloak.md) or [002b - Entra ID](../002b-setup-oidc-entra.md)
3. [003 - Install Components](../003-install-components.md)
4. [010 - AWS Bedrock Runtime + demochatbot](../010-aws-bedrock-runtime.md)
5. [060 - Tracing](../060-observability-tracing.md) - set `telemetry.service.type=LoadBalancer` and the AWS Runtime `spec.telemetryEndpoint` so AgentCore traces land in the AR dashboard
6. [050 - AccessPolicy](../050-access-policies.md) - lock things down before you call it done
7. [099 - Cleanup](../099-cleanup.md)

## What You Will Have at the End

- Agentregistry Enterprise on Kubernetes, authenticated against your OIDC provider
- An `AWS` `Runtime` backed by an IAM role + External ID
- `demochatbot` deployed to AWS Bedrock AgentCore
- Spans from the chatbot landing in `agentregistry.otel_traces_json` and visible in the UI **Tracing** page
- `AccessPolicy` records granting `registry:read` and `runtime:invoke` to the `are-admins` group

## Next

- [Kagent track](kagent-track.md) - add an in-cluster runtime and wire up MCP servers
- [031 - Remote MCP via kagent](../031-mcp-remote-github-copilot.md) - pair this AWS agent with MCP tools served from kagent
