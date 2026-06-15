# Track — Kagent on Substrate

Substrate → kagent → `AgentHarness` end-to-end. Use this when the demo you're trying to give is "kagent harnesses running on Substrate workers", not "Substrate running stateful actors in isolation".

The kagent integration is independent of the Substrate-native demos in [050](../050-counter-demo.md)–[053](../053-claude-code-multiplex.md) — you can do this track without running any of those.

## Estimated Time

- ~2 hours end-to-end from a fresh GKE cluster

## Prerequisites

- A GCP project with billing on, `gcloud`, `kubectl`, `helm`, Go ≥ 1.26.3, `openssl`, `git`
- An **Anthropic API key** (kagent's default provider in the install command)
- `envsubst` (for the parameterized `AgentHarness` template)

## Order

1. [000 — Overview](../000-overview.md)
2. [001 — Clone Upstream](../001-clone-upstream.md)
3. [010 — GKE Cluster Prereqs](../010-gke-cluster-prereqs.md)
4. [020 — Configure Env](../020-configure-env.md)
5. [030 — GCP IAM + Bucket](../030-gcp-iam-and-bucket.md)
6. [040 — Install Substrate (Helm OCI)](../040-install-substrate-helm.md)
7. [045 — Install `kubectl-ate`](../045-install-kubectl-ate.md) *(optional but lets you `kubectl ate get actors` to see the harness's actor)*
8. [060 — Install kagent with Substrate Enabled](../060-install-kagent-with-substrate.md)
9. [070 — Substrate-Backed `AgentHarness`](../070-kagent-agentharness.md)
10. [080 — Suspend / Resume Operations](../080-operations-suspend-resume.md) — useful for waking the harness actor when it suspends
11. [099 — Cleanup](../099-cleanup.md)

## What You'll Have at the End

- Substrate running in `ate-system`, all infra pods Ready (`ate-api-server`, `atenet-router`, 6× `valkey`, `atelet` DaemonSet, `pod-certificate-controller`)
- kagent installed with `controller.substrate.enabled=true`, the `kagent-default` `WorkerPool` created, and the `/substrate` UI page showing your workers
- An `AgentHarness` named `openclaw-substrate-demo` running on the substrate runtime with `Ready=True`
- A generated `ActorTemplate` owned by the `AgentHarness` (visible in `kubectl get actortemplates.ate.dev -A`)
- An actor in Valkey (visible via `kubectl ate get actors` — the actor ID will be `ahr-kagent-openclaw-substrate-demo` or similar)
- A working gateway endpoint at `/api/agentharnesses/kagent/openclaw-substrate-demo/gateway/` on the kagent controller

## The Ownership Model in One Picture

```
You apply an AgentHarness
            │
            │ kagent controller reconciles
            ▼
┌─────────────────────────────────────────────────────────┐
│  AgentHarness (kagent.dev/v1alpha2)                     │
│    spec.runtime: substrate                              │
│    spec.substrate.workerPoolRef → kagent-default        │
└─────────────────────────────────────────────────────────┘
            │
            │ kagent generates  (owner reference)
            ▼
┌─────────────────────────────────────────────────────────┐
│  ActorTemplate (ate.dev/v1alpha1)                       │
│  ─ container image, env, workerPoolRef                  │
│  ─ snapshotsConfig.location → gs://your-bucket/...      │
└─────────────────────────────────────────────────────────┘
            │
            │ Substrate builds the golden snapshot
            ▼
┌─────────────────────────────────────────────────────────┐
│  Actor (in Valkey, NOT a CRD)                           │
│  ─ ahr-kagent-openclaw-substrate-demo                   │
│  ─ initial STATUS_SUSPENDED, hydrates on first request  │
└─────────────────────────────────────────────────────────┘
```

Deleting the `AgentHarness` deletes the actor (kagent calls `DeleteActor`) and Kubernetes GC removes the generated `ActorTemplate`. The `WorkerPool` survives — it's owned by the platform, not the harness.

## Variations

- **Use an external `WorkerPool`** instead of the one kagent creates: install kagent with `substrateWorkerPool.create=false` and set `spec.substrate.workerPoolRef.name` per harness.
- **Multiple harnesses, one pool**: apply more `AgentHarness` resources with different names — each gets its own `ActorTemplate` and actor, all multiplexed onto the same `WorkerPool`.
- **NemoClaw instead of OpenClaw**: change `spec.backend` from `openclaw` to `nemoclaw` on the `AgentHarness`. Same surrounding integration.

## Next

- [demos-track](demos-track.md) — Substrate-native demos for the architectural foundations behind the kagent integration
- [appendix-benchmarking](../appendix-benchmarking.md) — push the same `ate-api` that kagent talks to with Locust
