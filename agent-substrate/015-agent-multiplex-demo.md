---
title: "Agent Substrate: Multiplexing Claude Code Agents Onto Fewer Pods"
description: >
  A hands-on lab running real Claude Code agents as Substrate actors: three
  agents share a two-pod WorkerPool, an explicit suspend frees capacity for
  the third, proving that actor count can exceed worker count.
tags: [agent-substrate, kubernetes, actors, claude-code, suspend-resume, multiplexing, oversubscription]
author: Michael Levan
---

# More Agents Than Pods

Tldr; Three Claude Code actors, two worker pods. An operator explicitly
suspends an idle actor to free a slot for another. The cluster is
oversubscribed on purpose, and capacity remains an honest, visible constraint.

This demo runs **real AI agent workload**: each actor is a container running the actual
`@anthropic-ai/claude-code` CLI in a loop. Wake, send a task to Claude,
print the answer, and idle. An operator or higher-level system can use those idle
windows to suspend actors and multiplex a smaller worker pool across them.
Substrate provides the lifecycle and scheduling primitives; this demo drives
the policy explicitly with `kubectl ate`.

This exercises the same density primitive as the upstream demo, at walkthrough
scale:

1. **Density**: 3 Claude Code actors on a 2-replica `WorkerPool`. The third
   actor cannot run until a slot frees - and you'll see the honest capacity
   signal when it can't.
2. **Real workload**: each running actor makes genuine Claude API calls and
   streams the results through actor logs.

## What Substrate pieces this uses

| Concept | Where it lives |
|---|---|
| Claude Code workload | `demos/claude-code-multiplex/workload/` - a `node:20-slim` image with the Claude Code CLI and a `run.sh` loop: run `claude --print "$TASK"`, sleep `INTERVAL_SECONDS`, repeat. |
| `WorkerPool` (2 replicas) | `claude-workerpool` in namespace `claude-multiplex-demo`, labeled `workload: claude-multiplex`. The templates bind to it via `workerSelector.matchLabels`. |
| `ActorTemplate` x3 | `agent-luna`, `agent-mars`, `agent-orion` - same image, different `TASK` prompt. `ANTHROPIC_API_KEY` comes from a Secret via `valueFrom.secretKeyRef`. |
| Explicit resume | These agents serve no HTTP, so there's no traffic to wake them through the `atenet-router`. Resume is `kubectl ate resume actor` (the same `Control.ResumeActor` RPC the router would call). |
| Full-state snapshots | The templates set only `snapshotsConfig.location` - no `onPause`/`onCommit` tiering - so a suspend captures the full sandbox (RAM included), unlike the counter template's cheaper `Data` tier. |

---

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) -> [002](002-gcp-iam-and-bucket.md) -> [003](003-install-substrate.md).
  The counter demo is not required for this lab.
- The `kubectl-ate` plugin installed (`go install ./cmd/kubectl-ate` from the
  Substrate repo checkout) and on your `PATH`. It auto-port-forwards to the
  `ate-api-server`, so no manual port-forward is needed for the CLI steps.
- An **Anthropic API key**. The agents make real Claude API calls.
- Local tools beyond the setup lab's list:

| Tool | Why |
|---|---|
| `docker` with `buildx` | The workload is a Dockerfile image (Node + Claude Code CLI), not a Go binary - `ko` doesn't apply, so the deploy step builds and pushes it with `docker buildx`. The Docker daemon must be running. |
| `jq` | The deploy step uses it to resolve the pushed image's sha256 digest. |

Verify Docker before deploying:

```bash
docker info
docker buildx version
```

> **This lab costs real money in two ways**: each RUNNING agent calls the
> Anthropic API every 45 seconds, and the workload image build pushes to your
> registry. Suspend the actors when you're not actively demoing, and don't
> leave the demo running overnight.

> **Use a dedicated, short-lived Anthropic key.** The Secret is resolved into
> the workload environment, and this demo intentionally takes full-memory
> snapshots. The key is therefore present in golden and actor snapshots in
> GCS. Deleting actors or Kubernetes resources does not delete those objects;
> the Cleanup section removes the dedicated snapshot prefix separately.

> **Build reproducibility:** the upstream workload Dockerfile currently
> installs `@anthropic-ai/claude-code@latest`. The resulting image is pinned by
> digest for one deployment, but a later rebuild may contain a different Claude
> Code version. Pin a tested package version in the Dockerfile for repeatable
> runs.

Run everything from the root of your Substrate repo checkout, with your env
file sourced (it carries `BUCKET_NAME` and `KO_DOCKER_REPO`, which the deploy
step requires):

```bash
source .ate-dev-env.sh
```

Confirm the control plane is healthy before starting:

```bash
kubectl get pods -n ate-system
```

---

## Step 1 - Deploy the demo (templates, pool, Secret)

Export your Anthropic key (`read -s` keeps it out of your shell history):

```bash
read -rsp 'Anthropic API key: ' ANTHROPIC_API_KEY
echo
export ANTHROPIC_API_KEY
```

ActorTemplates do not read Secrets directly. Instead, `ate-api-server` resolves the
`secretKeyRef` values. Its `ServiceAccount` intentionally has no default
cluster-wide Secret access. Create the namespace and grant it permission to
read only this demo's `anthropic-api-key` Secret before deploying:

```bash
kubectl create namespace claude-multiplex-demo \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl apply -f - <<'EOF'
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: ate-api-server-env-sources
  namespace: claude-multiplex-demo
rules:
- apiGroups: [""]
  resources: ["secrets"]
  resourceNames: ["anthropic-api-key"]
  verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: ate-api-server-env-sources
  namespace: claude-multiplex-demo
subjects:
- kind: ServiceAccount
  name: ate-api-server
  namespace: ate-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: ate-api-server-env-sources
EOF
```

Deploy. `BUCKET_NAME` and `KO_DOCKER_REPO` are already in your environment
from the env file:

```bash
./hack/install-ate.sh --deploy-demo-claude-code-multiplex
```

This builds the workload image with `docker buildx`, pushes it to
`${KO_DOCKER_REPO}/claude-multiplex-demo-workload`, and applies a rendered
manifest containing:

- the `claude-multiplex-demo` namespace,
- an `anthropic-api-key` Secret (the templates consume it via
  `valueFrom.secretKeyRef`; the key does not appear in the public
  ActorTemplate spec, but its resolved value is captured in full snapshots),
- the 2-replica `WorkerPool` `claude-workerpool`,
- three `ActorTemplate`s: `agent-luna`, `agent-mars`, `agent-orion`.

Ensure the resources were created:

```bash
kubectl get actortemplate,workerpool -n claude-multiplex-demo
```

Wait for all three templates to finish their golden snapshots before creating
actors:

```bash
kubectl wait --for=condition=Ready \
  actortemplates.ate.dev/agent-luna \
  actortemplates.ate.dev/agent-mars \
  actortemplates.ate.dev/agent-orion \
  -n claude-multiplex-demo \
  --timeout=10m
```

You see two Pods because the goal is to see multiple Agents across only two
Workers (Pods).

---

## Step 2 - Create three agents

Actors live in an atespace (the tenancy boundary; see the
[multi-tenancy lab](014-multi-tenancy-teleport.md)).

Create one for the demo, then one actor per template:

```bash
kubectl ate create atespace agents
```

```bash
kubectl ate create actor luna  --template claude-multiplex-demo/agent-luna  --atespace agents
kubectl ate create actor mars  --template claude-multiplex-demo/agent-mars  --atespace agents
kubectl ate create actor orion --template claude-multiplex-demo/agent-orion --atespace agents
```

```bash
kubectl ate get actors --atespace agents
```

All three show `STATUS_SUSPENDED` with no `ATEOM POD`. **Three agents exist
and hold zero worker assignments.** The two WorkerPool pods remain running and
ready; suspended actors consume none of those slots. The same pool could hold
records and snapshots for many more suspended actors.

---

## Step 3 - Wake two agents and watch real Claude output

Resume two of the three (the pool has exactly two slots):

```bash
kubectl ate resume actor luna --atespace agents
kubectl ate resume actor mars --atespace agents

kubectl ate get actors --atespace agents
```

`luna` and `mars` go `STATUS_RESUMING` -> `STATUS_RUNNING`, each bound to a
different worker in the `ATEOM POD` column. Stream **Mars's** logs because Mars
is the actor Step 4 will suspend:

```bash
kubectl ate logs actor mars --atespace agents -f
```

`kubectl ate logs` preserves the structured log envelope, so each workload line
appears as JSON with `time` and `message` fields. You'll see the loop ticking:

```json
{"time":"2026-07-16T14:22:07Z","message":"[demo-actor:mars] === tick 3 at 14:22:07Z ==="}
{"time":"2026-07-16T14:22:07Z","message":"[demo-actor:mars] running: Give me one concise tip for learning a new programming language. One sentence."}
{"time":"2026-07-16T14:22:09Z","message":"Build one small project immediately and learn concepts as you need them."}
{"time":"2026-07-16T14:22:09Z","message":"[demo-actor:mars] tick 3 done; sleeping 45s"}
```

That's a genuine one-shot Claude API round-trip from inside a gVisor-sandboxed
actor. Press `Ctrl-C` after you see a completed tick.

> Don't expect the ticks to start at 1. The golden snapshot was captured
> after the workload had already started (that's the point - actors hydrate
> from a warm checkpoint, not a cold boot), so the loop resumes wherever the
> snapshot left it.

---

## Step 4 - The density squeeze: three agents, two slots

Try to wake the third agent while both workers are held:

```bash
kubectl ate resume actor orion --atespace agents
```

If both slots are still occupied, this fails with something like
`no free workers available`. **That error is the demo, not a bug** - it's an
oversubscribed system telling you the truth about capacity. Confirm who's
holding the slots:

```bash
kubectl ate get workers
```

Both workers show an assigned actor. Apply the multiplexing policy explicitly:
suspend Mars to free a slot, then retry Orion:

```bash
kubectl ate suspend actor mars --atespace agents
kubectl ate resume actor orion --atespace agents

kubectl ate get actors --atespace agents
```

`orion` is `STATUS_RUNNING`, `mars` is `STATUS_SUSPENDED` (checkpointed to
the bucket, slot released). Wait for Orion to complete at least one Claude call
before moving on:

```bash
kubectl ate logs actor orion --atespace agents -f
```

After you see `tick ... done`, press `Ctrl-C`. All three agents have now done
real work; at no point did a third worker pod exist.

> Substrate does not infer idleness from the workload's `sleep`. Production
> rotation requires the actor, an operator, or a higher-level policy system to
> call `SuspendActor`. This walkthrough uses explicit CLI calls so the policy
> boundary is visible.

---

## Step 5 - Optional: the dashboard

The demo ships a small Go dashboard that renders workers, actors, and pod
logs from the `ateapi` gRPC service. Treat it as an observational stage aid,
not as the lifecycle driver for this walkthrough. In one terminal,
port-forward the API; in another, run the UI:

```bash
# Terminal 1
kubectl port-forward svc/api 8080:443 -n ate-system

# Terminal 2 (from the Substrate repo root)
cd demos/claude-code-multiplex/ui
PORT=8090 ATEAPI_ADDR=localhost:8080 go run .
```

Open `http://localhost:8090`. The pods and agents panels are live cluster
state (via `ListWorkers` / `ListActors`); the logs pane reads real pod logs
via `client-go`.

> **Honesty note for the stage:** the "Give a task" button does not invoke
> Claude or call a Substrate lifecycle API. Its queued -> running -> completed
> badges are client-side timers (see `ui/server.go`'s `computeState`). Point
> the audience at the actor status column and logs pane for real signals.
>
> The current UI also has incomplete scoping: task selection calls unscoped
> `ListActors`, the actor panel can include the three `ate-golden` actors, and
> unassigned workers from other pools can appear. Use it on an otherwise clean
> demo cluster and cross-check `kubectl ate get actors --atespace agents` and
> `kubectl ate get workers` before making density claims.

---

## Cleanup

Suspend and delete the three demo actors:

```bash
for a in luna mars orion; do
  kubectl ate suspend actor "$a" --atespace agents 2>/dev/null || true
  kubectl ate delete actor "$a" --atespace agents || true
done
kubectl ate get actors --atespace agents
kubectl ate delete atespace agents
```

Remove the demo resources (namespace, Secret, WorkerPool, templates):

```bash
./hack/install-ate.sh --delete-demo-claude-code-multiplex
```

Kubernetes and Actor deletion do **not** remove object-store snapshots. If the
prefix is dedicated to this demo, delete its golden and actor snapshots so the
captured Anthropic key is not retained:

```bash
gcloud storage rm --recursive \
  "gs://${BUCKET_NAME}/claude-multiplex-demo/"
```

Verify `BUCKET_NAME` carefully before running that destructive command. The
Substrate control plane and cluster are untouched - tear those down via the
main [cleanup lab](099-cleanup.md) if you're done with them. The workload image
remains in your registry; delete it there if you want it gone. Unset the key
from your shell and revoke the dedicated key in Anthropic when finished:
`unset ANTHROPIC_API_KEY`.

---

## Troubleshooting

- **`no free workers available` on resume** - expected whenever both slots
  are held (Step 4). `kubectl ate get workers` shows who's holding them;
  suspend one and retry.
- **Deploy fails asking for `ANTHROPIC_API_KEY` / `BUCKET_NAME` /
  `KO_DOCKER_REPO`** - the install script hard-requires all three in the
  environment. Re-source `.ate-dev-env.sh` and re-export the key.
- **`Docker is not reachable` / `docker.sock: connect: no such file or
  directory`** - the Docker client is installed but its daemon is not running.
  Start Docker Desktop (or your alternative daemon), wait until it is ready,
  and confirm `docker info` succeeds before rerunning the deploy.
- **`docker buildx` push fails** - your Docker credential helper isn't
  configured for the registry. Re-run
  `gcloud auth configure-docker gcr.io` (or your Artifact Registry host).
- **Golden actors fail with `secrets "anthropic-api-key" is forbidden`** -
  ate-api-server needs the namespace-scoped `ate-api-server-env-sources`
  RoleBinding to resolve `secretKeyRef`. Apply the Role and RoleBinding YAML
  from Step 1 if either resource is missing.
