# Agent-Secret Demo (Zero-Idle + RAM Persistence)

A Go server that demonstrates **Substrate's "Zero-Idle" lifecycle**: the agent process calls `SuspendActor` **on itself** after responding to a request, freeing the worker as soon as it goes idle. The same demo also proves that a **secret generated in volatile RAM** at process start survives the suspend/resume cycle perfectly — i.e. Substrate doesn't "restart" containers, it rehydrates **living processes**.

The high-density side: this lab includes a "Wave Pulse" script that creates 24 stateful actors against a worker pool of 8, then pulses traffic at them in three waves to visually show multiplexing.

## Lab Objectives

- Deploy the `agent-secret` demo
- Scale the `WorkerPool` to 8 to make multiplexing visible
- Create one actor and watch its status flip `RUNNING` → (7s idle) → `SUSPENDED` **automatically**
- Confirm a second request returns the **same** RAM-resident secret
- Create 23 more actors and run the Wave Pulse to see oversubscription in action

## Prerequisites

- [040 — Substrate installed](040-install-substrate-helm.md)
- [045 — `kubectl-ate` on `PATH`](045-install-kubectl-ate.md)
- [020 — `.ate-dev-env.sh` sourced](020-configure-env.md)
- `ko` (for the image build)
- `watch` (for the live status feed)

## 1. Deploy

```bash
./hack/install-ate.sh --deploy-demo-agent-secret
```

Wait for the template:

```bash
kubectl wait --for=condition=Ready actortemplate/agent-secret \
  -n ate-demo-secret-agent-v2 --timeout=5m
```

## 2. Scale the Worker Pool to 8

To make multiplexing visible later, scale the physical pool up:

```bash
kubectl patch workerpool agent-secret -n ate-demo-secret-agent-v2 \
  --type='merge' -p '{"spec":{"replicas":8}}'
```

Wait for the warm pods to land:

```bash
kubectl get pods -n ate-demo-secret-agent-v2 -w
```

## 3. Port-Forward and Watch Status

```bash
# Terminal 1: router
kubectl port-forward -n ate-system svc/atenet-router 8000:80
```

```bash
# Terminal 2: live actor status
watch -n 1 kubectl ate get actors
```

## 4. Basic Interaction — Zero-Idle in Action

```bash
kubectl ate create actor my-agent --template ate-demo-secret-agent-v2/agent-secret

curl -H "Host: my-agent.actors.resources.substrate.k8s.io" http://localhost:8000
```

> Note: the demo's DNS suffix in the upstream README is `actors.resources.substrate.k8s.io` (different from the `actors.resources.substrate.ate.dev` used by the counter demo). The agent and router agree on this — what matters is consistency, not the exact suffix.

Watch Terminal 2. The actor:

1. Flips to `STATUS_RUNNING` **instantly** on the request.
2. After **~7 seconds** of idle ("visibility linger" window), flips back to `STATUS_SUSPENDED` — **automatically**, because the agent process called `SuspendActor` on itself.

## 5. Prove the Secret Persists

Send another request to the same actor:

```bash
curl -H "Host: my-agent.actors.resources.substrate.k8s.io" http://localhost:8000
```

The "Identity" secret in the response is **identical** to the first response — even though the actor was checkpointed, the worker pod wiped, and the actor resumed on whatever worker was free this time around. Substrate restored the process's volatile RAM, not just rebooted the container.

## 6. The Wave Pulse — Density at Scale

Create 23 more actors:

```bash
for i in {001..023}; do
  kubectl ate create actor session-$i --template ate-demo-secret-agent-v2/agent-secret
done
```

Now pulse traffic at them in three waves of 8 (one wave per `WorkerPool` capacity slice):

```bash
for wave in 0 1 2; do
  echo "Triggering Wave $((wave + 1))..."
  for i in {1..8}; do
    num=$(printf "%03d" $((wave * 8 + i)))
    curl -s -H "Host: session-$num.actors.resources.substrate.k8s.io" http://localhost:8000 &
  done
  sleep 8   # 7s linger + 1s buffer
done
```

In Terminal 2 you'll see the "conveyor belt": each wave fills the 8 physical workers, processes its 8 actors, then those actors auto-suspend during the `sleep 8` window — freeing the workers in time for the next wave.

With this pattern you can scale the **logical** actor count to thousands (try the loop again with 200) while keeping the **physical** pod count fixed.

## Cleanup

```bash
# Delete the 24 actors
for i in {001..023}; do
  kubectl ate suspend actor session-$i 2>/dev/null
  kubectl ate delete  actor session-$i 2>/dev/null
done
kubectl ate suspend actor my-agent
kubectl ate delete  actor my-agent

# Remove the demo
./hack/install-ate.sh --delete-demo-agent-secret
```

## What This Demo Adds

| Aspect | Counter ([050](050-counter-demo.md)) | Sandbox ([051](051-sandbox-demo.md)) | Agent-Secret (this lab) |
|---|---|---|---|
| Suspend trigger | Manual `kubectl ate suspend` | Implicit on REPL `exit` | **The agent suspends itself** (`SuspendActor` from inside) |
| State preserved | In-memory counter | RAM + filesystem | **Volatile RAM** (secret) across many cycles |
| Multiplexing shown? | No | No | Yes — 24 actors on 8 workers |

This demo is the foundation for any "thousands of idle agents, handful of pods" pattern. The Zero-Idle behavior is what makes Substrate cost-efficient at scale.

## Next

- [053 — Claude Code Multiplex](053-claude-code-multiplex.md) — the same pattern, but the workload is a real LLM agent
- [080 — Operations](080-operations-suspend-resume.md)
