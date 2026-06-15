# Track — AWS Bedrock AgentCore

A focused path through the workshop for someone who wants to run AgentRegistry Enterprise with **Microsoft Entra ID** OIDC and deploy an agent to **AWS Bedrock AgentCore**. Skips the kagent / MCP / private-cluster labs.

## Estimated Time

- ~90 minutes end-to-end on an existing managed Kubernetes cluster
- Add ~60 minutes if you provision a fresh EKS cluster

## Prerequisites

- A Kubernetes cluster with a default `StorageClass` (or follow [010](../010-cluster-prereqs.md) to bring one up)
- An Entra ID tenant where you can create app registrations and groups
- An AWS account where you can create IAM roles + a CloudFormation stack

## Order

1. [001 — Install `arctl`](../001-install-arctl.md)
2. [010 — Cluster Prerequisites](../010-cluster-prereqs.md) *(skip if you have a healthy cluster already)*
3. [020 — Microsoft Entra ID OIDC](../020-setup-entra.md)
4. [030 — Install AgentRegistry Enterprise (Helm)](../030-install-agentregistry-helm.md) — use the **Entra** values block in section 2a
5. [040 — Authenticate `arctl`](../040-arctl-auth.md) — use the **manual device-code** flow in section 3
6. [050 — AWS Bedrock AgentCore Provider](../050-aws-provider.md)
7. [060 — Deploy the Demo Chatbot on AWS](../060-deploy-demochatbot-on-aws.md)
8. [090 — Tracing Setup](../090-observability-tracing.md) — set `telemetry.service.type=LoadBalancer` and the AWS Runtime `spec.telemetryEndpoint`
9. [080 — AccessPolicy / RBAC](../080-access-policies.md) — lock things down before you call it done
10. [099 — Cleanup](../099-cleanup.md)

## What You Will Have at the End

- AgentRegistry Enterprise on Kubernetes, authenticated against Entra
- An AWS `Runtime` backed by an IAM role + External ID
- `demochatbot` v1.0.4 deployed to AWS Bedrock AgentCore
- Spans from the chatbot landing in `agentregistry.otel_traces_json` and visible in the UI **Tracing** page
- `AccessPolicy` records granting `registry:read` and `runtime:invoke` to the `are-admins` Entra group

## Next

- [Kagent track](kagent-track.md) — to add an in-cluster runtime and wire up MCP servers
- [072 — Wire MCP to an Agent](../072-wire-mcp-to-agent.md)
