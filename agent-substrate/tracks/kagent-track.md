# Track - Substrate + kagent End-to-End

Substrate → kagent integration → `AgentHarness`. Use this when the demo is "kagent harnesses running on Substrate workers", not "Substrate running actors directly."

The kagent integration is independent of the Substrate-native demos (010-013) - you can do this track without running any of them.

## Estimated Time

- ~2 hours end-to-end from a fresh GKE cluster

## Prerequisites

- A GCP project with billing on
- An **Anthropic API key** (kagent's default model provider)
- `envsubst` (for the parameterized `AgentHarness` manifest)

## Order

1. [001 - Baseline Setup](../001-baseline-setup.md)
2. [002 - GCP IAM + Snapshot Bucket](../002-gcp-iam-and-bucket.md)
3. [003 - Install Substrate](../003-install-substrate.md)
4. [020 - kagent Integration](../020-kagent-integration.md) - Part 1 installs kagent w/ substrate; Part 2 walks an `AgentHarness` end-to-end
5. [030 - Suspend / Resume Operations](../030-operations.md) - useful when the harness actor suspends
6. [099 - Cleanup](../099-cleanup.md)

## What You Will Have at the End

- Substrate running in `ate-system`, all infra pods Ready
- kagent installed with `controller.substrate.enabled=true`, the `kagent-default` `WorkerPool` created, the `/substrate` UI page showing your workers
- A substrate-backed `AgentHarness` (`openclaw-substrate-demo`) at `Ready=True`
- A generated `ActorTemplate` owned by the harness, plus an actor in Valkey serving the gateway endpoint

## Next

- [demos-track](demos-track.md) - Substrate-native demos for the architectural foundations
- [appendix-benchmarking](../appendix-benchmarking.md) - push the same `ate-api` kagent talks to with Locust
