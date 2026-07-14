# The Teleporting Actor: Atespace Multi-Tenancy + State Teleport

Two tenants each run an actor with the *same ID*, and Substrate keeps their state, addressing, and snapshots completely separate. Then an actor is suspended (checkpointed to object storage, worker released) and resumed onto a **different worker pod** — and its durable state continues exactly where it left off. The pod is cattle; the actor's state is the stable thing.

This lab reuses the counter workload from [010](010-counter-demo.md), so there is nothing new to build — the new concepts are **Atespaces** and the **teleport**.

## What Substrate Pieces This Uses

| Concept | Where it lives |
|---|---|
| `Atespace` | Tenancy/addressing boundary for actors, independent of Kubernetes namespaces. Actors are keyed `actor:<atespace>:<id>`. |
| DNS mesh routing | `Host: <actor-id>.<atespace>.actors.resources.substrate.ate.dev` through the `atenet-router`. The atespace segment is **required** - an actor ID is only unique within its atespace. |
| Suspend / resume | Suspend checkpoints the actor's sandbox and uploads the snapshot to the GCS bucket; resume restores it into any free worker. The actor is never in two places, and between the two it exists only as an object in the bucket. |
| Counter demo workload | Reused as the actor. Its response prints an in-memory count and a durable-file count - the file count is the state-survival proof across suspend (see the snapshot-tiering note in step 2). Worker assignment is read from `kubectl ate get actors`. |

## Lab Objectives

- Create two Atespaces (`tenant-a`, `tenant-b`) and an actor named `agent-1` in **each**
- Drive traffic to both through the DNS mesh and prove their state never mixes
- Suspend tenant-a's actor, occupy its old worker, and resume it onto a **different** worker with its durable state intact
- Understand snapshot tiering (`onPause` / `onCommit`) and why the RAM counter resets while the file counter survives

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) → [002](002-gcp-iam-and-bucket.md) → [003](003-install-substrate.md)
- The **counter demo** deployed - [010](010-counter-demo.md) step 1 (`./hack/install-ate.sh --deploy-demo-counter`); the actors in this lab are created from its template
- The `kubectl-ate` plugin installed (`go install ./cmd/kubectl-ate` from the Substrate repo checkout) and reachable on your `PATH` (kubectl discovers plugins by scanning `PATH` for `kubectl-*` binaries)
- Local tools: `curl`, `kubectl`

Confirm the control plane and template are healthy before starting:

```bash
kubectl get pods -n ate-system
kubectl get actortemplate counter -n ate-demo-counter
```

The `counter` ActorTemplate should show `Ready`, which means its golden snapshot exists in your GCS bucket.

## 1. Two Tenants, One Agent ID

An `atespace` is to Actors as `namespace` is to Pods.

Create two Atespaces and an actor named `agent-1` in **each**. The `kubectl-ate` CLI auto-port-forwards to the `ate-api-server`, so these run directly from your terminal:

```bash
kubectl ate create atespace tenant-a
kubectl ate create atespace tenant-b

kubectl ate create actor agent-1 --template=ate-demo-counter/counter --atespace=tenant-a
kubectl ate create actor agent-1 --template=ate-demo-counter/counter --atespace=tenant-b
```

You can now list the Actors running.

Note: there is no "default" atespace, so listing always takes `--atespace` or `-A` for all:

```bash
kubectl ate get actors -A
```

You'll see both `agent-1` entries with their atespace, status, assigned worker (`<namespace>/<pod>`), and IP columns.

Now drive traffic through the DNS mesh. Port-forward the router and address each actor by its full host, actor ID **and** atespace:

```bash
kubectl -n ate-system port-forward svc/atenet-router 8000:80 &

# Hit tenant-a's agent-1 three times
for i in 1 2 3; do
  curl -s -H "Host: agent-1.tenant-a.actors.resources.substrate.ate.dev" http://localhost:8000/
done

# Hit tenant-b's agent-1 once
curl -s -H "Host: agent-1.tenant-b.actors.resources.substrate.ate.dev" http://localhost:8000/
```

The counter responds with:

```
hello from: <sandbox-ip> | preserved memory count: 3 | preserved file counter: 3
hello from: <sandbox-ip> | preserved memory count: 1 | preserved file counter: 1
```

**That's the tenancy proof.** You gave the Actors the same name, so same actor ID, same template, same WorkerPool, but tenant-a's count is 3 and tenant-b's is 1. Different DNS names, different state-store keys, different snapshot paths in the bucket. Neither tenant can address or observe the other's actor. (First requests may take a moment; a newly created actor starts `SUSPENDED`, and the router's ext_proc filter auto-resumes it from the golden snapshot before forwarding.)

> The `hello from:` address is the actor's **sandbox-internal link-local IP**.
> It looks the same regardless of which worker pod is serving, so it does NOT
> identify the worker. Worker identity comes from the `ATEOM POD` column:

```bash
kubectl ate get actors --atespace=tenant-a
```

Record tenant-a's `ATEOM POD` value (e.g. `ate-demo-counter/counter-deployment-xxxxx-yyyyy`). You'll compare it after the teleport.

## 2. The Teleport: Suspend, Move Workers, State Intact

Current state of tenant-a's agent:

```bash
kubectl ate get actors --atespace=tenant-a
curl -s -H "Host: agent-1.tenant-a.actors.resources.substrate.ate.dev" http://localhost:8000/
# hello from: <sandbox-ip> | preserved memory count: 4 | preserved file counter: 4
```

Note the `ATEOM POD` column value. Suspend the actor as its snapshot uploads to the bucket and the worker slot is released:

```bash
kubectl ate suspend actor agent-1 --atespace=tenant-a
```

```bash
kubectl ate get actors --atespace=tenant-a

# STATUS: STATUS_SUSPENDED, ATEOM POD: <none>
```

> Don't be surprised if the actor already shows `STATUS_SUSPENDED` before you
> run the command. Substrate **idle-suspends actors automatically** after a
> short quiet period. That's the core efficiency feature doing its job; if it
> beat you to it, just skip the suspend and continue.

**Occupy its old worker.** The resume of the Actor is likely to land elsewhere. The counter demo's WorkerPool has 5 replicas; parking a few filler actors on the pool takes the old slot. Use **3** fillers, not more: tenant-b's `agent-1` may still be running (idle-suspend takes about a minute), and 3 fillers + tenant-b + the slot you're saving for the wake-up accounts for all 5 workers. (Placement picks any free worker. This makes a different worker *likely*, not guaranteed. If the agent lands on the same pod, suspend it and repeat with another filler.)

```bash
for i in 1 2 3; do
  kubectl ate create actor filler-$i --template=ate-demo-counter/counter --atespace=tenant-b
  curl -s -H "Host: filler-$i.tenant-b.actors.resources.substrate.ate.dev" http://localhost:8000/ > /dev/null
done
```

> If the wake-up curl below returns `actor "agent-1" unavailable: no free
> workers available`, the pool is genuinely full - every worker is assigned to
> a running actor (`kubectl ate get workers` shows who's holding each slot).
> That's an honest capacity signal from an oversubscribed system, and the fix
> is the same thing Substrate does on idle: free a slot. Suspend a filler
> (`kubectl ate suspend actor filler-1 --atespace=tenant-b`) and retry.
> Don't count on idle-suspension freeing one quickly - in practice actors can
> stay RUNNING for a while, so the manual suspend is the reliable path.

```bash
kubectl ate get workers
```

Wake `agent-1` by sending traffic to it. The router's ext_proc filter sees the suspended actor and resumes it before forwarding the request:

```bash
curl -s -H "Host: agent-1.tenant-a.actors.resources.substrate.ate.dev" http://localhost:8000/
```

```bash
# hello from: <sandbox-ip> | preserved memory count: 1 | preserved file counter: 5
kubectl ate get actors --atespace=tenant-a
```

Read the response and the `ATEOM POD` column together:

- **`ATEOM POD` shows a different worker pod** than the one you recorded before the suspend → the actor physically moved.
- **`preserved file counter: 5`** → the actor's durable state survived the move (it was 4 before suspend; this request made it 5). Not re-initialized. Restored from the bucket onto a different pod.
- **`preserved memory count: 1`** → the in-RAM count reset. This is expected with **this template's** snapshot tiering, and it's worth explaining on stage rather than hiding:

> **Snapshot tiering (`onPause` / `onCommit`).** The stock counter template
> sets `snapshotsConfig.onPause: Full` and `onCommit: Data`: a *pause* (kept
> on the node) captures full RAM + disk, but a *suspend* (uploaded to storage)
> captures only `Data` - the `durableDir` volume contents. So across a
> suspend/resume teleport, the file counter survives and the RAM counter
> restarts from the golden snapshot. If you want full RAM to survive suspend -
> the agent-secret demo's whole story, see [012](012-agent-secret-demo.md) -
> use a template that omits `onPause`/`onCommit` (the default captures
> everything, at the cost of a bigger snapshot). For this lab the file counter
> is the state-survival proof, and the cheaper `Data` tier is itself a good
> talking point: you choose per template how much state is worth persisting.

## Cleanup

```bash
# Actors and atespaces. Deleting a RUNNING actor is refused
# (FailedPrecondition), so suspend first - unless idle-suspension already did
# it for you, in which case the suspend returns immediately.
for i in 1 2 3; do
  kubectl ate suspend actor filler-$i --atespace=tenant-b 2>/dev/null
  kubectl ate delete actor filler-$i --atespace=tenant-b
done
kubectl ate suspend actor agent-1 --atespace=tenant-a 2>/dev/null
kubectl ate delete actor agent-1 --atespace=tenant-a
kubectl ate suspend actor agent-1 --atespace=tenant-b 2>/dev/null
kubectl ate delete actor agent-1 --atespace=tenant-b
kubectl ate delete atespace tenant-a
kubectl ate delete atespace tenant-b
# Leave the system-managed `ate-golden` atespace alone - the control plane
# uses it for golden-snapshot actors.

# Stop the router port-forward
kill %1 2>/dev/null
```

The Substrate control plane, counter demo, and cluster are untouched - remove the counter demo via [010's Cleanup](010-counter-demo.md#cleanup) and tear down the baseline via [099](099-cleanup.md) if you're done with them.

## Troubleshooting

- **`unknown flag: --atespace`** - your `kubectl-ate` binary predates Atespaces. Rebuild it from the current checkout: `go install ./cmd/kubectl-ate`, and make sure no stale binary appears earlier on `PATH` (`which kubectl-ate`).
- **Wake-up curl returns `no free workers available`** - the pool is genuinely full. `kubectl ate get workers` shows who holds each slot; suspend a filler and retry (see the callout in step 2).
- **The actor resumed onto the *same* worker** - placement picks any free worker, so a different pod is likely but not guaranteed. Suspend it and repeat with another filler occupying the slot.
- **First request to a new actor is slow or times out once** - a newly created actor starts `SUSPENDED`; the router auto-resumes it from the golden snapshot on first request. Retry once and check `kubectl ate get actors -A`.
- **`preserved memory count` reset after suspend** - expected with the counter template's `onCommit: Data` tiering; the durable-file counter is the survival proof. See the snapshot-tiering callout in step 2.

## Recap

| Beat | What was proven |
|---|---|
| Same actor ID, two Atespaces | Tenants can't collide: separate state, DNS, and snapshots - with zero coordination between them. |
| Suspend → resume across workers | Durable state (the file counter) followed the actor onto a different worker pod. The RAM tier reset by template design (`onCommit: Data`) - snapshot tiering is a per-template cost/fidelity dial. |

The talk line: **"The pod died. The actor didn't. State follows the actor, not the worker."**

## Next

- [012 - Agent-Secret Demo](012-agent-secret-demo.md) - full-RAM snapshot survival (the tier this lab's template skips)
- [030 - Suspend / Resume Operations](030-operations.md) - resuming actors via `grpcurl` and bulk operations
- [099 - Cleanup](099-cleanup.md)
