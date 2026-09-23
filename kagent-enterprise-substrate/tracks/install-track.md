# Track - Install and One Agent

The full path through this workshop: confirm the cluster can run Substrate, install it with kagent Enterprise, create one agent, then read how the mTLS path works.

## Prerequisites

- A Kubernetes cluster at v1.36 or above, or v1.37 or above where `PodCertificateRequest` is already on
- `kubectl`, `helm` v3, `curl`, `jq`, `openssl`
- A Solo kagent license key
- An LLM provider API key
- A container image `ate-api` can pull, for [010](../010-create-an-agent.md)
- On GKE, Workload Identity, if Actor snapshots go to Cloud Storage

## Order

1. [001 - Where Substrate Works](../001-where-it-works.md)
2. [002 - Install Substrate and kagent](../002-install.md)
3. [010 - Create an Agent](../010-create-an-agent.md)
4. [020 - Access Control](../020-access-control.md)
5. [099 - Cleanup](../099-cleanup.md)

The kagent Helm install waits up to 15 minutes (`--timeout 15m`). Substrate is installed with `--wait=false` and only becomes Ready after the cryptographic pools in 002.

## What You Will Have at the End

Before cleanup:

- Agent Substrate `0.2.0-beta5` in `ate-system`, with the five CA/JWT pools and `ate-api-authentication`
- kagent Enterprise `1.0.0-alpha3` in `kagent`, with worker pool `kagent-default` on `ghcr.io/kagent-dev/substrate/ateom-gvisor:v0.2.0-beta5`
- A `Harness` named `kagent` and an `AgentTemplate` named `assistant`
- A picture of the three mTLS hops and where model and MCP credentials are injected

## Next

- [099 - Cleanup](../099-cleanup.md) - when the cluster should go back to empty
