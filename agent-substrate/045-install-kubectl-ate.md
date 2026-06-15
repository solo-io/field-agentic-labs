# Install the `kubectl-ate` CLI

`kubectl-ate` is the `kubectl` plugin for managing Substrate actors and workers. It talks to `ate-api-server` over gRPC and (by default) auto-establishes a background port-forward to the in-cluster Service so you don't need to manage tunnels by hand.

## Lab Objectives

- `go install` the plugin from the upstream source
- Confirm `kubectl` discovers it as `kubectl ate`
- Walk through the global flags and the actor / worker / logs / admin command groups

## Prerequisites

- [001 — upstream Substrate repo cloned](001-clone-upstream.md)
- [040 — Substrate installed](040-install-substrate-helm.md)
- Go ≥ 1.26.3 (see [010](010-gke-cluster-prereqs.md))

## 1. Install

From the root of the cloned `substrate/` repo:

```bash
go install ./cmd/kubectl-ate
```

This drops a `kubectl-ate` binary at `$(go env GOPATH)/bin/kubectl-ate` (typically `~/go/bin/kubectl-ate`).

`kubectl` discovers plugins by scanning `PATH` for binaries named `kubectl-<verb>`. Make sure your Go bin directory is on `PATH`:

```bash
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc   # adjust for your shell
source ~/.zshrc
```

## 2. Verify

```bash
which kubectl-ate         # -> .../go/bin/kubectl-ate
kubectl ate --help
```

If `kubectl ate` still reports `unknown command "ate" for "kubectl"`, the binary isn't on your `PATH` — re-check the `export` and that `go install` succeeded.

## 3. Global Flags

| Flag | Short | Default | Notes |
|---|---|---|---|
| `--kubeconfig` | | `~/.kube/config` | Plumbed to client-go for cluster discovery |
| `--endpoint` | | (auto) | Manual gRPC endpoint override (e.g. `localhost:8080`). Skips the auto port-forward — useful when running inside the cluster or pointing at a LoadBalancer. |
| `--output` | `-o` | `table` | `table` / `json` / `yaml` |
| `--trace` | | `false` | Enables on-demand OpenTelemetry tracing for the request (see [090](090-observability.md)) |

## 4. Auto Port-Forwarding

By default `kubectl-ate` reads your `~/.kube/config`, discovers the `ate-api-server` pods in `ate-system`, and opens a temporary background port-forward tunnel for the duration of each command. You don't need to manage the tunnel.

If you want to bypass the tunnel — for example to route through a `LoadBalancer` or when running inside a Pod that already has cluster DNS — pass `--endpoint`:

```bash
kubectl ate get actors --endpoint=api.ate-system.svc:443
```

## 5. Quick Tour of the Commands

### `get` — list / inspect

```bash
kubectl ate get actors                # all actors
kubectl ate get actor <id> -o yaml    # one actor, full YAML
kubectl ate get workers               # all physical workers
```

> **Heads-up:** Actors and Workers are **not** Kubernetes CRDs — they live in the Substrate control plane (Valkey), not in `etcd`. `kubectl get actor` returns nothing; only `kubectl ate get …` queries the control plane. `kubectl get actortemplate` and `kubectl get workerpool` *do* work because those are CRDs.

#### `kubectl ate get actor` columns

| Column | Meaning |
|---|---|
| `NAMESPACE` | Namespace of the `ActorTemplate` |
| `TEMPLATE` | `ActorTemplate` name |
| `ID` | Actor ID (user-provided for app actors; UUID for the golden actor materialised during `ResumeGoldenActor`) |
| `STATUS` | `STATUS_RESUMING` / `STATUS_RUNNING` / `STATUS_SUSPENDING` / `STATUS_SUSPENDED` |
| `ATEOM POD` | Worker pod (namespace/name) currently hosting the actor. Empty while suspended. |
| `ATEOM IP` | Pod IP of that worker. Empty while suspended. |
| `VERSION` | Monotonic counter incremented on every state transition (resume / suspend / checkpoint) |

#### `kubectl ate get worker` columns

| Column | Meaning |
|---|---|
| `NAMESPACE` | `WorkerPool` namespace |
| `POOL` | `WorkerPool` name |
| `POD` | Worker pod name |
| `STATUS` | `FREE` (idle) or `ASSIGNED` (hosting an actor) |
| `ASSIGNED ACTOR` | If `ASSIGNED`, the actor reference `<namespace>/<template>/<actor-id>` |

### Lifecycle — `create` / `resume` / `suspend` / `delete`

The actor ID must be a valid **DNS-1123 label** (lowercase alphanumeric + hyphens):

```bash
kubectl ate create actor my-actor --template=ate-demo-counter/counter
kubectl ate resume  actor my-actor
kubectl ate suspend actor my-actor
kubectl ate delete  actor my-actor    # only allowed when SUSPENDED
```

### `logs` — stream actor stdout/stderr

The verb requires a resource type. `kubectl ate logs <id>` alone prints help.

```bash
kubectl ate logs actors my-actor      # follows by default
```

Streamable while the actor is `STATUS_RUNNING`. For history across worker reassignments / suspensions, route through a centralized log backend — see [090](090-observability.md).

### `admin` — bootstrap + debug

```bash
# Generate a CA pool and push to a Secret
kubectl ate admin make-ca-pool \
  --name workerpool-ca-certs \
  --secret-namespace ate-system \
  --ca-id "1"

# Generate a JWT authority pool and push to a Secret
kubectl ate admin make-jwt-pool \
  --name session-id-jwt-pool \
  --secret-namespace ate-system \
  --key-id "1"

# DANGEROUS: flush all Actor/Worker tracking state from Redis. Dev-only.
kubectl ate admin debug-flush-redis
```

## Smoke Test the Install

```bash
kubectl ate get workers
```

If you ran [040](040-install-substrate-helm.md) but haven't created a `WorkerPool` yet (no demo deployed), this returns an empty list — that's fine. Move on to [050](050-counter-demo.md).

## Next

- [050 — Counter Demo](050-counter-demo.md)
