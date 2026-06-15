# Sandbox Demo

A sandboxed Alpine Linux execution environment running as a Substrate actor. Demonstrates two things on top of what [050](050-counter-demo.md) already showed:

1. An actor can host a **persistent filesystem** (writes to `/` survive suspend → snapshot → resume)
2. A separate **REPL client** can drive the actor interactively, giving you something close to a stateful per-user shell session

> **⚠ Security:** the demo has **no authorization checks**. The sandbox actor will execute any client-provided commands with no validation. Do not expose this to untrusted networks; do not deploy in production.

## Lab Objectives

- Deploy the sandbox demo (`WorkerPool` + `ActorTemplate` in `ate-demo-sandbox`)
- Create a sandbox actor (`my-sandbox-1`)
- Build and run the REPL client; execute commands; confirm filesystem state persists across the implicit suspend on `exit`

## Prerequisites

- [040 — Substrate installed](040-install-substrate-helm.md)
- [045 — `kubectl-ate` on `PATH`](045-install-kubectl-ate.md)
- [020 — `.ate-dev-env.sh` sourced](020-configure-env.md)
- `ko` (`go install github.com/google/ko@latest`)
- Go (for building the REPL client)

## Components

| Path (in the cloned `substrate/` repo) | Purpose |
|---|---|
| `demos/sandbox/main.go` | Server that runs **inside** the actor. Exposes `/process` to execute commands. |
| `demos/sandbox/client/` | CLI REPL the user drives from their laptop. |
| `demos/sandbox/sandbox.yaml.tmpl` | `WorkerPool` + `ActorTemplate` template (`${BUCKET_NAME}` envsubst at apply time). |

## 1. Deploy

From the root of the cloned `substrate/` repo:

```bash
./hack/install-ate.sh --deploy-demo-sandbox
```

This builds the Alpine-based sandbox server image with `ko`, creates the `ate-demo-sandbox` namespace, and applies the `WorkerPool` + `ActorTemplate`.

Wait for the template to be ready:

```bash
kubectl wait --for=condition=Ready actortemplate/sandbox-template \
  -n ate-demo-sandbox --timeout=5m
```

## 2. Create an Actor

```bash
kubectl ate create actor my-sandbox-1 --template ate-demo-sandbox/sandbox-template
```

## 3. Port-Forward Two Services

The REPL client talks to **both** the `ate-api-server` (gRPC, for actor lifecycle) and `atenet-router` (for the actor's `/process` endpoint). Run two port-forwards in separate terminals:

```bash
# Terminal 1: ate-api-server
kubectl port-forward -n ate-system svc/api 8080:443
```

```bash
# Terminal 2: atenet-router
kubectl port-forward -n ate-system svc/atenet-router 8000:80
```

> The Service name (`api` vs `ate-api-server`) may vary by chart version. `kubectl get svc -n ate-system` will tell you what's actually there.

## 4. Build and Run the REPL Client

In a third terminal, from the cloned `substrate/` repo:

```bash
go build -o bin/sandbox-client ./demos/sandbox/client

./bin/sandbox-client \
  --ateapi=localhost:8080 \
  --atenet=localhost:8000 \
  --id=my-sandbox-1
```

Once you see the `sandbox>` prompt, you have an interactive shell **inside the actor**:

```text
sandbox> pwd
/
sandbox> ls -la
sandbox> echo "Hello" > /tmp/test.txt
sandbox> cat /tmp/test.txt
Hello
```

## 5. Prove Filesystem State Persists

The interesting bit. Type `exit` at the `sandbox>` prompt — this **automatically suspends** the actor (the client calls `SuspendActor` on close). `runsc` checkpoints both memory and the writable layer of the container's filesystem to your GCS bucket.

Confirm the actor is `SUSPENDED`:

```bash
kubectl ate get actor my-sandbox-1
```

Now re-open the REPL — same actor ID, same args:

```bash
./bin/sandbox-client \
  --ateapi=localhost:8080 \
  --atenet=localhost:8000 \
  --id=my-sandbox-1
```

Your file is still there (possibly on a **different** worker pod):

```text
sandbox> cat /tmp/test.txt
Hello
```

This is the disk-side analog of the counter demo's RAM-side state preservation.

## Cleanup

```bash
kubectl ate suspend actor my-sandbox-1   # if still running
kubectl ate delete  actor my-sandbox-1

./hack/install-ate.sh --delete-demo-sandbox
```

## What This Demo Adds Over the Counter

| Aspect | Counter ([050](050-counter-demo.md)) | Sandbox (this lab) |
|---|---|---|
| State preserved | In-memory counter integer | Both RAM **and** writable filesystem layer |
| Trigger for suspend | Manual `kubectl ate suspend` | Implicit — `exit` in the REPL client |
| Driver | `curl` with `Host:` header | Custom REPL client speaking to both `ateapi` and `atenet` |
| Security | None demonstrated | None enforced (executes arbitrary commands) |

## Next

- [052 — Agent-Secret Demo (self-suspending, RAM-resident secret)](052-agent-secret-demo.md)
- [053 — Claude Code Multiplex](053-claude-code-multiplex.md)
