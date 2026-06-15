# Appendix — Local `kind` Quickstart

For laptop development without GCP. Substrate ships `hack/create-kind-cluster.sh` + a `manifests/ate-install/kind/` overlay that handles the apiserver flag plumbing for you. **No GCS** — the kind overlay ships [rustfs](https://github.com/rustfs/rustfs), an S3-compatible local snapshot store, so the counter demo's state still persists across suspend/resume on your laptop.

The main workshop targets GKE because that's the supported managed path (see [appendix-why-gke](appendix-why-gke.md)). The kind path is **for development only** and produces an ephemeral cluster.

## Lab Objectives

- Stand up a local `kind` cluster wired for Substrate (Pod Certificate apiserver flags, local registry)
- Install Substrate + the counter demo against it
- Build `kubectl-ate` and prove suspend/resume works on your laptop
- Tear down the cluster + registry cleanly

## Prerequisites

- Docker Desktop running
- Go ≥ 1.26.3
- `kubectl`
- The upstream Substrate repo cloned ([001](001-clone-upstream.md))

> `kind` is **fetched automatically** by Substrate's hack scripts via its `tools/` go.mod — you don't need to install it separately.

## 1. Create the Cluster + Local Registry

From the root of the cloned `substrate/` repo:

```bash
hack/create-kind-cluster.sh
```

This:

- Creates a kind cluster with the Pod Certificate beta APIs enabled at the apiserver (via the cluster config — kind exposes the apiserver flags directly)
- Stands up a local container registry the cluster can pull from
- Configures Docker / kind networking so `KO_DOCKER_REPO` and local image refs Just Work

## 2. Install Substrate

```bash
hack/install-ate-kind.sh --deploy-ate-system
```

This is the kind-specific wrapper around `install-ate.sh`. It applies the manifests from `manifests/ate-install/` **and** the `manifests/ate-install/kind/` overlay (which adds rustfs, Prometheus, Jaeger, and the OTel Collector — all in the `otel-system` namespace).

Wait for the system pods:

```bash
kubectl get pods -n ate-system -w
```

## 3. Install the Counter Demo

```bash
hack/install-ate-kind.sh --deploy-demo-counter
```

Wait:

```bash
kubectl wait --for=condition=Ready actortemplate/counter \
  -n ate-demo-counter --timeout=5m
```

## 4. Install `kubectl-ate` and Create an Actor

```bash
go install ./cmd/kubectl-ate
export PATH="$PATH:$(go env GOPATH)/bin"

kubectl ate create actor my-counter-1 --template ate-demo-counter/counter
```

## 5. Port-Forward and Drive Traffic

```bash
# Terminal 1
kubectl port-forward -n ate-system svc/atenet-router 8000:80
```

```bash
# Terminal 2
curl -X POST -H "Host: my-counter-1.actors.resources.substrate.ate.dev" \
  -i http://localhost:8000/
```

Repeat, suspend, repeat — same proof-of-state-survival as the GKE counter lab in [050](050-counter-demo.md).

## 6. Observability — Bundled Prometheus + Jaeger

The kind overlay also installs Prometheus + an OTel Collector + Jaeger in `otel-system`:

```bash
# Prometheus
kubectl port-forward -n otel-system svc/prometheus 9090:9090
# -> http://localhost:9090

# Jaeger (after running any command with --trace)
kubectl port-forward -n otel-system svc/jaeger 16686:16686
# -> http://localhost:16686
kubectl ate get actor my-counter-1 --trace
```

## 7. Cleanup

```bash
# Remove the Substrate install + demo
hack/install-ate-kind.sh --delete-all

# Delete the kind cluster + its registry
./hack/delete-kind-cluster.sh
```

## What the kind Overlay Adds

| Component | Namespace | Purpose |
|---|---|---|
| `rustfs` | `ate-system` | S3-compatible local snapshot store. Replaces GCS for the kind path. |
| `prometheus` | `otel-system` | Scrapes the `rpc_*` / `http_*` series from `ate-api` and `atelet` |
| `otel-collector` | `otel-system` | Receives traces from the `--trace` flag |
| `jaeger` (all-in-one) | `otel-system` | UI for the traces |

## Limitations vs the GKE Path

| Aspect | kind | GKE |
|---|---|---|
| Snapshot store | `rustfs` (in-cluster, ephemeral) | GCS (durable) |
| Cluster lifetime | Ephemeral (dies with the Docker container) | Persistent |
| Pod Certificate support | apiserver flags passed via kind config | `--enable-kubernetes-unstable-apis` |
| Workload Identity | n/a | Yes |
| Image registry | Local kind registry | GAR / GCR via `KO_DOCKER_REPO` |
| Observability | Prometheus + Jaeger in-cluster | Cloud Monitoring + Cloud Trace |
| OK for | Development, contributor work, demos on a flight | Real demo environments, performance work |

## Related

- [001 — Clone upstream](001-clone-upstream.md)
- [appendix-why-gke](appendix-why-gke.md) — why kind works locally but managed-AKS/EKS don't
- [050 — Counter Demo](050-counter-demo.md) — the same demo on GKE
