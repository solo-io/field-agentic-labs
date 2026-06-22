# Track - All Four Demos

All four Substrate demos in sequence. Each one adds a different dimension on top of the previous.

## Estimated Time

- ~2 hours if Substrate is already installed
- Add the install-track time on top if you're starting from a fresh cluster

## Prerequisites

- Completed the [install-track](install-track.md) (or the equivalent steps yourself)
- For [013](../013-claude-code-multiplex.md): an Anthropic API key + `docker buildx`

## What Each Demo Adds

| Demo | What's new | Lab |
|---|---|---|
| **Counter** | Baseline - stateful HTTP, in-memory counter survives suspend/resume | [010](../010-counter-demo.md) |
| **Sandbox** | Filesystem state (not just RAM) persists; custom REPL client | [011](../011-sandbox-demo.md) |
| **Agent-Secret** | Self-suspending agent; volatile RAM secret across many cycles; 24 actors on 8 workers | [012](../012-agent-secret-demo.md) |
| **Claude Code Multiplex** | Real LLM workload (3 agents on 2 pods); live dashboard UI | [013](../013-claude-code-multiplex.md) |

## Order

1. [010 - Counter Demo](../010-counter-demo.md)
2. [011 - Sandbox Demo](../011-sandbox-demo.md)
3. [012 - Agent-Secret Demo](../012-agent-secret-demo.md)
4. [013 - Claude Code Multiplex](../013-claude-code-multiplex.md) - read the DRAFT callout first
5. [030 - Suspend/Resume Operations](../030-operations.md) - useful when actors get stuck
6. [099 - Cleanup](../099-cleanup.md)

## Run All Four in One Cluster?

Yes - each lives in its own namespace and has its own `WorkerPool`. You can keep all four installed and switch between them. The only resource constraint is total worker-pod count - make sure your cluster autoscaler can handle ~20 standing pods.

## Next

- [kagent-track](kagent-track.md) - kagent on top of Substrate
- [appendix-benchmarking](../appendix-benchmarking.md) - push the counter and `ate-api` hard with Locust
