# Track - Install and One Agent

The full path through this workshop: confirm the cluster can run Substrate, install it with kagent Enterprise, create one agent, then read how the mTLS path works.

## Prerequisites

- A Kubernetes cluster that serves `PodCertificateRequest` and `ClusterTrustBundle` (see [001](../001-where-it-works.md))
- `kubectl`, `helm` v3, `curl`, `jq`, `openssl`, `envsubst`
- A Solo kagent license key and an enterprise agentgateway license key
- An LLM provider API key
- Docker with `buildx`, to resolve the runtime image digest in [010](../010-create-an-agent.md)
- On GKE, Workload Identity, if Actor snapshots go to Cloud Storage

## Order

1. [001 - Where Substrate Works](../001-where-it-works.md)
2. [002 - Install Substrate and kagent](../002-install.md)
3. [010 - Create an Agent](../010-create-an-agent.md)
4. [020 - Access Control](../020-access-control.md)
5. [099 - Cleanup](../099-cleanup.md)

The kagent Helm install waits up to 15 minutes (`--timeout 15m`). Substrate pods wait for their certificates, so its first rollout can take several minutes.

## What You Will Have at the End

Before cleanup:

- Agent Substrate Enterprise `0.5.0-alpha4-da3c2e5` in `ate-system`, with the six CA/JWT pools and `ate-api-authentication`
- A PostgreSQL in `kagent` shared by kagent and Substrate, with certificate logins
- kagent Enterprise `1.0.0-alpha11` in `kagent`, with worker pool `kagent-default` on `us-docker.pkg.dev/solo-public/substrate-enterprise/ateom-gvisor:v0.5.0-alpha4-da3c2e5`
- A `Harness` named `kagent`, an `AgentTemplate` named `assistant`, and an `Agent` named `assistant`
- A picture of the three mTLS hops and where model and MCP credentials are injected

## Next

- [099 - Cleanup](../099-cleanup.md) - when the cluster should go back to empty
