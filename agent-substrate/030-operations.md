# Suspend / Resume Operations

Day-2 ops for Substrate actors. The single most common question is "why is my actor in `STATUS_SUSPENDED`?" — answer: **by design**. Substrate suspends actors when they're idle to reclaim worker pods. This lab walks through the two ways to wake one back up: `kubectl-ate` (the friendly path) and `grpcurl` against `ateapi.Control` (the raw path, useful when the CLI is misbehaving or when you need to script against the API from outside).

## Lab Objectives

- Confirm an actor is `STATUS_SUSPENDED` (and understand why)
- Resume it with `kubectl ate resume actor` or with raw `grpcurl ResumeActor`
- Check its state with `GetActor`
- Know when to use `--boot` (cold-boot, bypass snapshot) vs the default warm resume

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) → [002](002-gcp-iam-and-bucket.md) → [003](003-install-substrate.md)
- A suspended actor — easiest to set up is one of the demos from [010](010-counter-demo.md)–[013](013-claude-code-multiplex.md)
- (For the `grpcurl` path) `grpcurl` installed locally

## Why Actors Get Suspended

The whole point of Substrate is to free worker pods when actors aren't doing work. Three things suspend an actor:

| Trigger | Where it happens |
|---|---|
| Manual `kubectl ate suspend actor <id>` | You |
| Self-suspend (`SuspendActor` called from inside the agent process) | The agent itself — see [052](012-agent-secret-demo.md) |
| Idle timeout (visibility linger window) | The agent in [052](012-agent-secret-demo.md); not all workloads have this |

Once `SUSPENDED`, the actor has **no `ATEOM POD` / `ATEOM IP`** — the worker is back in the pool. The actor's snapshot lives in your GCS bucket; the row in Valkey points at it. Resuming is what hydrates that snapshot back into a worker.

## Path 1 — Resume with `kubectl-ate`

This is the friendly path. Auto port-forward, structured output, less typing:

```bash
# See current state
kubectl ate get actor my-counter-1

# Wake it up
kubectl ate resume actor my-counter-1

# Or just send traffic — the atenet router resumes on first request
curl -X POST -H "Host: my-counter-1.actors.resources.substrate.ate.dev" \
  http://localhost:8000
```

`resume` returns once the actor is `RUNNING` and bound to a worker IP. From there, `kubectl ate get actor my-counter-1` will show `STATUS_RUNNING`, the `ATEOM POD`, and the `ATEOM IP`.

## Path 2 — Resume with `grpcurl`

Sometimes you need the raw API — e.g. you're scripting from outside the cluster, or `kubectl-ate` is broken on the host you're on, or you just want to see what's happening on the wire.

`ate-api-server` listens on a TLS gRPC port. Port-forward it:

```bash
kubectl port-forward -n ate-system svc/api 18443:443
```

> The Service name may be `api`, `ate-api-server`, or `ateapi` depending on chart version. `kubectl get svc -n ate-system` will tell you.

Then call `ResumeActor`:

```bash
grpcurl -insecure \
  -d '{"actor_id":"<YOUR_ACTOR_ID>"}' \
  localhost:18443 ateapi.Control/ResumeActor
```

Check state:

```bash
grpcurl -insecure \
  -d '{"actor_id":"<YOUR_ACTOR_ID>"}' \
  localhost:18443 ateapi.Control/GetActor
```

> Replace `<YOUR_ACTOR_ID>` with the actual actor ID — e.g. `my-counter-1`, or for the kagent integration, `ahr-kagent-<your-harness-name>`.

> `-insecure` is fine for the port-forward (TLS is on, but the cert chain doesn't include `localhost`). Don't `-insecure` over the open internet.

## Useful `ateapi.Control` Calls

Full surface is documented in [docs/api-guide.md §5](https://github.com/agent-substrate/substrate/blob/main/docs/api-guide.md#5-control-plane-grpc-api) in the upstream repo. The common ones:

| RPC | Purpose | Notes |
|---|---|---|
| `CreateActor` | Register a logical actor | `actor_id` (DNS-1123), `actor_template_namespace`, `actor_template_name` |
| `ResumeActor` | Assign a worker, restore snapshot, return worker IP | `actor_id`, optional `boot: true` to skip snapshots (cold boot) |
| `SuspendActor` | Snapshot to GCS, free the worker | `actor_id` |
| `DeleteActor` | Remove from Valkey + GC the snapshot (when GC lands) | Only allowed when `STATUS_SUSPENDED` |
| `GetActor` / `ListActors` | Query state | |
| `ListWorkers` | Query the physical pool | |

### Cold-Boot vs Warm Resume

`ResumeActor` accepts `{"boot": true}` to bypass the snapshot and perform a **cold boot** from the `ActorTemplate`'s definition. Use this when:

- The snapshot is corrupt (e.g. a partial GCS upload from a crashed `atelet`)
- You changed the `ActorTemplate` and want the actor to pick up the new entry point (otherwise the snapshot will keep restoring the old code path)
- You want to reset the actor's RAM + filesystem back to "first boot"

```bash
grpcurl -insecure \
  -d '{"actor_id":"my-counter-1","boot":true}' \
  localhost:18443 ateapi.Control/ResumeActor
```

The actor still ends up `STATUS_RUNNING`, but its in-memory counter (if it's the counter demo) is back to zero — because there was no snapshot to restore from.

## Bulk Operations

If you're running a fleet (the agent-secret Wave Pulse in [052](012-agent-secret-demo.md), or a hundred kagent harnesses), the most useful loop is:

```bash
# Suspend everything currently RUNNING (e.g. before cluster maintenance)
kubectl ate get actors -o json \
  | jq -r '.items[] | select(.status=="STATUS_RUNNING") | .id' \
  | while read id; do
      kubectl ate suspend actor "$id"
    done
```

> Verify the JSON shape on your version with `kubectl ate get actors -o json | jq '.items[0]'` — field names may differ across releases.

## Reset Dynamic State (Destructive — Dev Only)

If your Valkey state gets wedged (extremely rare; usually means a bug), nuke it from orbit:

```bash
kubectl ate admin debug-flush-redis
```

This **flushes all Actor and Worker tracking state**. Snapshots in GCS are untouched; `ActorTemplate` and `WorkerPool` CRDs are untouched. After flushing you'll need to re-create the actors you want.

**Never run this on a production cluster.**

## Common Symptoms

| Symptom | Diagnosis |
|---|---|
| `kubectl ate get actor` shows `STATUS_SUSPENDED` but you expected `RUNNING` | Working as intended. Send traffic or call `ResumeActor`. |
| Actor stuck in `STATUS_RESUMING` for > 30s | `atelet` is having trouble pulling the snapshot from GCS, or the worker pod is wedged. `kubectl logs -n ate-system -l app=atelet` and check the pod that's hosting the worker. |
| Actor stuck in `STATUS_SUSPENDING` for > 30s | `runsc` is failing to checkpoint — usually missing `--allow-connected-on-save`, see [040 troubleshooting](003-install-substrate.md#3-wait-for-the-system-pods) |
| `ResumeActor` returns `FailedPrecondition` | The `ActorTemplate` isn't `Ready` (golden snapshot still building). `kubectl get actortemplate <name> -A`. |
| `DeleteActor` returns `FailedPrecondition` | Actor is not `STATUS_SUSPENDED`. Suspend first, then delete. |

## Cleanup

This lab is mostly read-only — `ResumeActor` / `GetActor` / `SuspendActor` change actor state but don't create any new resources. If the lab created actors of its own (e.g. you ran the bulk-suspend loop against a test fleet), clean those up with the same `kubectl ate delete actor` pattern from [010 step 4](010-counter-demo.md#4-prove-state-survives-suspendresume).

If you opened a port-forward to `ate-api-server` for the `grpcurl` examples (`kubectl port-forward -n ate-system svc/api 18443:443`), just `Ctrl-C` it.

## Next

- [040 — Observability](040-observability.md)
- [099 — Cleanup](099-cleanup.md)
