# Counter Demo

A stateful Go HTTP server that increments an in-memory counter on every request. This is **the canonical first demo** - it proves the full suspend → snapshot → resume cycle works on your cluster, and the counter value persisting across the cycle is the most obvious visual confirmation that Substrate is doing what it says.

Even though the workshop's [040](003-install-substrate.md) install used Helm, the demos still ship as part of the upstream repo's `hack/install-ate.sh` (it builds the demo image with `ko`, applies the templated YAML, and creates the namespace + `WorkerPool` + `ActorTemplate`).

## Lab Objectives

- Deploy the counter `WorkerPool` + `ActorTemplate`
- Create an actor (`my-counter-1`) from the template
- Drive traffic through `atenet-router` and watch the counter increment
- **Suspend** the actor, confirm `STATUS_SUSPENDED`, then send another request and confirm the counter **continues from where it left off**

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) → [002](002-gcp-iam-and-bucket.md) → [003](003-install-substrate.md)
- `.ate-dev-env.sh` still sourced (the demo deploy reads `$BUCKET_NAME` and `$KO_DOCKER_REPO`)
- `ko` installed (for the demo image build): `go install github.com/google/ko@latest`

## 1. Deploy the Demo

From the root of the cloned `substrate/` repo:

```bash
./hack/install-ate.sh --deploy-demo-counter
```

This:

1. Builds the counter server image with `ko` and pushes it to `${KO_DOCKER_REPO}`
2. Creates the `ate-demo-counter` namespace
3. Applies the `WorkerPool` and `ActorTemplate` from `demos/counter/counter.yaml.tmpl` (with `${BUCKET_NAME}` envsubst'd at apply time)
4. Substrate's controller starts a temporary **Golden Pod** that boots the counter binary; `runsc` checkpoints it into Version 0 ("the golden snapshot"); the template enters `Ready`

Wait for the golden snapshot:

```bash
kubectl wait --for=condition=Ready actortemplate/counter \
 -n ate-demo-counter --timeout=5m
```

What got created:

```bash
kubectl get workerpool,actortemplate -n ate-demo-counter
```

- **`WorkerPool` `counter`** - the warm pool of standby pods
- **`ActorTemplate` `counter`** - immutable definition of the counter actor

> Do not hand-edit `demos/counter/counter.yaml.tmpl`. `${BUCKET_NAME}` is substituted at apply time by the deploy script.

## 2. Create an Actor

The actor ID must be a valid DNS-1123 label (lowercase alphanumeric + hyphens):

```bash
kubectl ate create actor my-counter-1 --template ate-demo-counter/counter
```

The `--template` value is `<namespace>/<name>`. The actor starts in **`STATUS_SUSPENDED`** - no worker is consumed yet, and the `Actor` row in Valkey points at the golden snapshot so the first resume hydrates instantly.

```bash
kubectl ate get actor my-counter-1
```

## 3. Drive Traffic Through the Router

Substrate routes to actors via a uniform DNS name: `<actor-id>.actors.resources.substrate.ate.dev`. Port-forward `atenet-router` and pass the actor name in the `Host` header.

**Terminal 1** - port-forward:

```bash
kubectl port-forward -n ate-system svc/atenet-router 8000:80
```

**Terminal 2** - send the first request (this triggers an on-demand `ResumeActor`):

```bash
curl -X POST -H "Host: my-counter-1.actors.resources.substrate.ate.dev" \
 http://localhost:8000
```

The first request is the most interesting one - the router pauses traffic, the control plane claims a warm worker from the `counter` `WorkerPool`, `atelet` + `ateom` restore the snapshot into the sandbox, and the request is forwarded to the (now-resumed) actor.

Hit it a few more times and watch the counter increment in the response body. Confirm the actor is `RUNNING` and bound to a worker:

```bash
kubectl ate get actor my-counter-1
kubectl ate get workers
```

## 4. Prove State Survives Suspend/Resume

This is the payoff. Suspend the actor - Substrate checkpoints the full memory + disk state to your GCS bucket and reclaims the worker pod:

```bash
kubectl ate suspend actor my-counter-1
kubectl ate get actor my-counter-1
# STATUS_SUSPENDED, no worker
```

Send another request:

```bash
curl -X POST -H "Host: my-counter-1.actors.resources.substrate.ate.dev" \
 http://localhost:8000
```

The router resumes the actor from the snapshot - **possibly on a different worker pod** - and the counter **continues** from where it left off instead of resetting. That's the entire point of Substrate.

Stream logs across the lifecycle to see it in action:

```bash
kubectl ate logs actors my-counter-1
```

You'll see the `Count` log lines preserve their value across the suspend/resume boundary.

## What Just Happened (Mapping to the Architecture)

| Step | Substrate mechanism |
|---|---|
| Create actor | Valkey row written as `SUSPENDED`, referencing the `counter` `ActorTemplate`. No pod consumed. |
| First request | `atenet` reads the actor ID from the `Host` header, queries the control plane, triggers `ResumeActor`. |
| Resume | Control plane claims a warm worker from the `counter` `WorkerPool`; `atelet` + `ateom` restore the snapshot into the gVisor sandbox; status → `RUNNING`. |
| Suspend | `ateom` checkpoints memory + disk via `runsc`; `atelet` streams the snapshot to the GCS bucket; the worker is wiped and returned to the pool; status → `SUSPENDED`. |
| Resume again | Actor rehydrates from its **last** snapshot - in-memory state (the counter) persists across the cycle, possibly on a different worker. |

`WorkerPool` and `ActorTemplate` live in the Kubernetes API as CRDs. `Actor` and `Worker` instance state live in Valkey - keeping the Kubernetes control plane out of the request hot path.

## Cleanup

Delete the actor (only suspended actors can be deleted):

```bash
kubectl ate suspend actor my-counter-1 # if still running
kubectl ate delete actor my-counter-1
```

Remove the demo resources (namespace, `WorkerPool`, `ActorTemplate`):

```bash
./hack/install-ate.sh --delete-demo-counter
```

## Troubleshooting

- **`actortemplate/counter` never reaches `Ready`.** Look at the temporary Golden Pod: `kubectl describe actortemplate counter -n ate-demo-counter` and `kubectl get pods -n ate-demo-counter`. The usual cause is the image pull (verify `KO_DOCKER_REPO` and the node SA's `roles/artifactregistry.reader` from [030](002-gcp-iam-and-bucket.md)).
- **First `curl` hangs.** Confirm `kubectl port-forward` is still alive in Terminal 1, and that the actor exists: `kubectl ate get actor my-counter-1`.
- **`STATUS_RUNNING` but `curl` returns connection refused.** The `atenet-router` Service may not have endpoints yet. `kubectl get pods,svc,ep -n ate-system` and confirm the router pod is `Ready`.
- **Snapshot read/write errors during suspend.** Walk back through [030 step 2c](002-gcp-iam-and-bucket.md#2c-bucket-scoped-iam-for-atelet) (bucket-scoped IAM) and [030 step 2d](002-gcp-iam-and-bucket.md#2d-project-level-iam-image-pull--atelet-snapshot-access) (project-level IAM). On a multi-pool cluster make sure the **GKE Metadata Server** is enabled on the pool the `atelet` pod landed on - see [030 step 2a](002-gcp-iam-and-bucket.md#reactive-checks-run-only-if-step-3-in-040-hits-these-symptoms).
- **Checkpoint fails.** Your `runsc` needs `--allow-connected-on-save`. The demo template pins a `runsc` nightly that has it; if you're using a custom template, point it at a build that does too.

## Next

- [051 - Sandbox Demo](011-sandbox-demo.md) - same pattern, different workload (Alpine shell + REPL client)
- [080 - Suspend / Resume Operations](030-operations.md) - including waking suspended actors via `grpcurl`
