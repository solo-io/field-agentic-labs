# Track — Install + First Demo

The fastest path from "no cluster" to "stateful counter actor that survives suspend/resume". Skips the kagent integration, the second/third/fourth demos, and the appendices.

## Estimated Time

- ~60 minutes if you already have a GKE Standard cluster
- ~90 minutes if you're provisioning the cluster too

## Prerequisites

- A GCP project with billing on
- `gcloud`, `kubectl`, `helm`, Go ≥ 1.26.3, `git`, `openssl`, `curl`
- An LLM API key only if you plan to chain into the kagent integration later — otherwise no LLM needed for this track

## Order

1. [000 — Overview](../000-overview.md)
2. [001 — Clone Upstream](../001-clone-upstream.md)
3. [010 — GKE Cluster Prereqs](../010-gke-cluster-prereqs.md)
4. [020 — Configure Env](../020-configure-env.md)
5. [030 — GCP IAM + Bucket](../030-gcp-iam-and-bucket.md)
6. [040 — Install Substrate (Helm OCI)](../040-install-substrate-helm.md)
7. [045 — Install `kubectl-ate`](../045-install-kubectl-ate.md)
8. [050 — Counter Demo](../050-counter-demo.md)
9. [099 — Cleanup](../099-cleanup.md)

## What You'll Have at the End

- A GKE Standard cluster with Pod Certificate beta APIs + Workload Identity enabled
- A GCS snapshot bucket with the right `atelet` IAM
- Substrate installed via Helm OCI; `ate-api-server`, `atenet-router`, 6× `valkey`, and the `atelet` DaemonSet all Ready
- `kubectl-ate` on your `PATH`
- A counter actor (`my-counter-1`) demonstrably preserving state across suspend → snapshot → resume

If the counter's value picks up where it left off after the suspend in [050](../050-counter-demo.md) — even when it lands on a different worker — Substrate is working.

## Next

- [demos-track](demos-track.md) — Sandbox + Agent-Secret + Claude Code Multiplex
- [kagent-track](kagent-track.md) — Add kagent on top, run an `AgentHarness` on Substrate
