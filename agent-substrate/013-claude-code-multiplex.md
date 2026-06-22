# Claude Code Multiplex (3 Agents on 2 Pods)

Three Claude Code agents (`luna`, `mars`, `orion`) sharing a 2-pod `WorkerPool`. Substrate suspends idle agents and resumes them on demand, so the cluster runs **fewer pods than agents**. A small Go web UI drives "give a task" against random idle agents and renders queued / running / completed badges.

> **⚠ UPSTREAM DRAFT.** The upstream README is explicit that this PR is a draft and it applies **three runtime workarounds** for open Substrate issues. The demo works, but is the most fragile of the four. If you don't need a real-LLM demo, [052](012-agent-secret-demo.md) is the more reliable "high-density multiplexing" story.

## Upstream Blockers (Currently Worked Around)

| Upstream issue | Symptom | Workaround in the demo |
|---|---|---|
| **`#189`** | Atelet OCI bundle gaps - missing `Args`, `Secret`, symlinks | Bundled fix patch applied at deploy time |
| **`#197` Bug 2a** | `valueFrom.secretKeyRef` on `ActorTemplate` container env not supported | `ANTHROPIC_API_KEY` passed as a plain `value:` env var (envsubst-substituted at apply time). The key ends up in the rendered manifest as plaintext - **scrub history** after teardown if your shell or registry logs it. |
| **`#197` Bug 3** | Atelet symlink resolution | Fix PR forthcoming |

Treat this demo as a preview, not a stable pattern.

## Lab Objectives

- Deploy the multiplex demo (3 `ActorTemplate`s on a 2-pod `WorkerPool`, all in `claude-multiplex-demo`)
- Run the dashboard UI locally
- Click "Give a task" and watch the state flow: `queued` → `running` → `completed`, then auto-suspend
- Observe that the third agent stays suspended while the other two run

## What It Shows

The video walkthrough is here: <https://storage.googleapis.com/yojowa-claw-demo-screenshots/multiplex-demo-2026-05-18-captions.webm>

Lifecycle flow per agent:

1. Click "Give a task" → UI picks a random idle agent and calls `CreateActor` (or resumes an existing one) → badge flips to `queued`
2. Substrate finds a free pod, binds the agent → badge flips to `running`
3. The agent calls Claude, writes a result, exits → badge flips to `completed`
4. Substrate notices the inactivity and suspends the agent after a short idle window
5. The released pod becomes available for the next queued task on a different agent

With three agents and two pods, the third agent stays suspended (state snapshotted) until a pod opens up.

## Components (in the cloned `substrate/` repo)

| Path | Purpose |
|---|---|
| `demos/claude-code-multiplex/claude-code-multiplex.yaml.tmpl` | Namespace + WorkerPool + 3 ActorTemplates (single envsubst template) |
| `hack/install-demo-claude-code-multiplex.sh` | Sourced by `install-ate.sh`; registers `--deploy-demo-claude-code-multiplex` and `--delete-demo-claude-code-multiplex` |
| `demos/claude-code-multiplex/workload/` | Workload container source (Dockerfile + Python + Claude Code wrapper) - built with `docker buildx`, **not** `ko` |
| `demos/claude-code-multiplex/ui/` | Static dashboard (`index.html`) + Go HTTP server (`server.go`) |

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) → [002](002-gcp-iam-and-bucket.md) → [003](003-install-substrate.md)
- `.ate-dev-env.sh` still sourced (`KO_DOCKER_REPO` is reused as the prefix for the workload image)
- `docker buildx` with multi-platform support (the workload is a Python+Claude image, so `ko` doesn't apply here)
- An **Anthropic API key**:

 ```bash
 export ANTHROPIC_API_KEY=<your-anthropic-api-key>
 ```

## 1. Deploy

From the root of the cloned `substrate/` repo:

```bash
ANTHROPIC_API_KEY=sk-ant-... \
BUCKET_NAME="$BUCKET_NAME" \
 ./hack/install-ate.sh --deploy-demo-claude-code-multiplex
```

This:

1. Creates the `claude-multiplex-demo` namespace
2. Builds the workload image with `docker buildx`, pushes it to `${KO_DOCKER_REPO}/claude-multiplex-demo-workload`, captures the pushed sha256 digest
3. Substitutes the digest-pinned image reference plus `ANTHROPIC_API_KEY` and `BUCKET_NAME` into the manifest template at apply time
4. Applies the 2-pod `WorkerPool` and three `ActorTemplate`s (`luna`, `mars`, `orion`)

> The deploy step **embeds your `ANTHROPIC_API_KEY` as a plain env var** in the rendered manifest (workaround for upstream `#197 Bug 2a`). Don't post the rendered manifest as a gist, and run [099 cleanup](099-cleanup.md) when done.

## 2. Port-Forward `ateapi` for the Dashboard

The dashboard talks to `ate-api-server` over gRPC (port-forwarded to your laptop) and reads pod logs from the Kubernetes API via `client-go` (so it picks up your `~/.kube/config` for cluster context).

```bash
# Terminal 1: ateapi port-forward - keep alive for the lifetime of the demo
kubectl port-forward svc/ateapi 8080:8080 -n ate-system
```

> The Service name (`ateapi` vs `api` vs `ate-api-server`) may vary by chart version - `kubectl get svc -n ate-system` will tell you. Adjust both the `-n ate-system svc/<name>` arg and the `ATEAPI_ADDR` env var below.

## 3. Run the Dashboard

```bash
# Terminal 2
cd demos/claude-code-multiplex/ui
PORT=8090 ATEAPI_ADDR=localhost:8080 go run .
```

Or build a binary:

```bash
go build -o ui-server .
PORT=8090 ATEAPI_ADDR=localhost:8080 ./ui-server
```

Dashboard env vars:

| Var | Default | Purpose |
|---|---|---|
| `PORT` | `8080` | TCP port the dashboard binds. Pick something **other than** `ATEAPI_ADDR`'s port when both run on the same host. |
| `ATEAPI_ADDR` | `localhost:8080` | Address of the Substrate `ateapi` gRPC service. |
| `DEMO_NAMESPACE` | `claude-multiplex-demo` | Namespace the dashboard filters to and reads pod logs from. |

Smoke-test:

```bash
curl localhost:8090/healthz
# {"logs":true} ← means client-go picked up the cluster context
```

## 4. Drive the Demo

Open <http://localhost:8090>.

Click **"Give a task"**:

- Badge → `queued` (agent has work, no pod yet)
- Substrate binds the agent to a free pod → badge → `running`
- Agent calls Claude, writes a result, exits → badge → `completed`
- After a short idle window, Substrate suspends the agent - released pod available for the next task on a **different** agent

Click "Give a task" a few more times. With three agents and two pods, you'll see the third agent **stay suspended** while the other two run. When one finishes and suspends, the third can be resumed.

## 5. Look Behind the UI

```bash
# Logical actor state (3 actors, 2 currently RUNNING + 1 SUSPENDED)
kubectl ate get actors

# Physical worker state (2 pods, both ASSIGNED most of the time)
kubectl ate get workers

# Actor logs (the Claude Code transcript)
kubectl ate logs actors luna
```

## Cleanup

```bash
./hack/install-ate.sh --delete-demo-claude-code-multiplex
```

This removes the `claude-multiplex-demo` namespace and everything in it. Stop the port-forward in Terminal 1 and the UI in Terminal 2.

> If your `ANTHROPIC_API_KEY` ended up in shell history / CI logs because of the deploy invocation, consider rotating the key.

## Scaling Past Three Agents

The same template pattern extends. To run **10 agents on 3 pods** or **100 on 20**:

- Bump `replicas` on the `WorkerPool` in `claude-code-multiplex.yaml.tmpl`
- Add more `ActorTemplate`s (or use one template and create more actors from it with `kubectl ate create actor <id> --template ...`)

The same upstream workarounds apply until the linked issues land.

## Next

- [060 - Install kagent with Substrate Enabled](020-kagent-integration.md) - wire kagent on top, so `AgentHarness` resources can run on Substrate workers
- [080 - Operations](030-operations.md)
