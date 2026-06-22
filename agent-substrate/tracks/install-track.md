# Track — Install + First Demo

Fastest path from "no cluster" to "stateful counter actor surviving suspend/resume."

## Estimated Time

- ~60 minutes on an existing GKE Standard cluster
- ~90 minutes if you also provision the cluster

## Order

1. [001 — Baseline Setup](../001-baseline-setup.md)
2. [002 — GCP IAM + Snapshot Bucket](../002-gcp-iam-and-bucket.md)
3. [003 — Install Substrate](../003-install-substrate.md)
4. [010 — Counter Demo](../010-counter-demo.md)
5. [099 — Cleanup](../099-cleanup.md)

## What You Will Have at the End

- A GKE Standard cluster with Pod Certificate beta APIs + Workload Identity enabled
- A GCS snapshot bucket with the right `atelet` IAM
- Substrate installed via Helm OCI; control plane + router + 6× `valkey` + `atelet` DaemonSet all Ready
- `kubectl-ate` on your `PATH`
- A counter actor demonstrably preserving state across suspend → snapshot → resume

If the counter's value picks up where it left off after the suspend in [010](../010-counter-demo.md) — even when it lands on a different worker — Substrate is working.

## Next

- [demos-track](demos-track.md) — Sandbox + Agent-Secret + Claude Code Multiplex
- [kagent-track](kagent-track.md) — Substrate + kagent end-to-end
