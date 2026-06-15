# Track — All Four Demos

All four Substrate demos in sequence — counter, sandbox, agent-secret, claude-code-multiplex. Each adds a different dimension on top of the one before.

## Estimated Time

- ~2 hours if Substrate is already installed (counter ~20m, sandbox ~30m, agent-secret ~30m, claude-code-multiplex ~40m)
- Add the install track time on top if you're starting from a fresh cluster

## Prerequisites

- Completed the [install-track](install-track.md) (or the equivalent steps yourself)
- For [053](../053-claude-code-multiplex.md): an Anthropic API key + `docker buildx`

## What Each Demo Adds

| Demo | What's new | Lab |
|---|---|---|
| **Counter** | Baseline — stateful HTTP, in-memory counter survives suspend/resume | [050](../050-counter-demo.md) |
| **Sandbox** | Filesystem state (not just RAM) persists; custom REPL client; implicit suspend on `exit` | [051](../051-sandbox-demo.md) |
| **Agent-Secret** | Self-suspending agent (calls `SuspendActor` on itself); volatile RAM secret across many cycles; **24 actors on 8 workers** (Wave Pulse) | [052](../052-agent-secret-demo.md) |
| **Claude Code Multiplex** | Real LLM workload (3 agents on 2 pods); live dashboard UI; queued/running/completed badges | [053](../053-claude-code-multiplex.md) — **upstream DRAFT** |

## Order

1. [050 — Counter Demo](../050-counter-demo.md)
2. [051 — Sandbox Demo](../051-sandbox-demo.md)
3. [052 — Agent-Secret Demo](../052-agent-secret-demo.md)
4. [053 — Claude Code Multiplex](../053-claude-code-multiplex.md) — **read the DRAFT callout first**
5. [080 — Suspend/Resume Operations](../080-operations-suspend-resume.md) — useful when actors get stuck
6. [099 — Cleanup](../099-cleanup.md)

## Run All Four in One Cluster?

Yes — they all live in their own namespaces (`ate-demo-counter`, `ate-demo-sandbox`, `ate-demo-secret-agent-v2`, `claude-multiplex-demo`) and each has its own `WorkerPool`. You can keep them all installed simultaneously and switch between them.

The only resource constraint is total worker pod count. With:

| Demo | Default `WorkerPool` replicas |
|---|---|
| counter | 5 |
| sandbox | 5 |
| agent-secret (after [052 step 2](../052-agent-secret-demo.md#2-scale-the-worker-pool-to-8)) | 8 |
| claude-code-multiplex | 2 |

…you're looking at ~20 standing worker pods, each `c3-standard-4`-ish. Make sure your GKE cluster autoscaler can scale up to handle that.

## Cleanup of Just the Demos

Without uninstalling Substrate or kagent:

```bash
./hack/install-ate.sh --delete-demo-counter
./hack/install-ate.sh --delete-demo-sandbox
./hack/install-ate.sh --delete-demo-agent-secret
./hack/install-ate.sh --delete-demo-claude-code-multiplex
```

## Next

- [kagent-track](kagent-track.md) — kagent on top of Substrate
- [appendix-benchmarking](../appendix-benchmarking.md) — push the counter and the `ate-api` hard with Locust
