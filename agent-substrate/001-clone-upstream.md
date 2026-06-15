# Clone the Upstream Substrate Repo

This workshop **links out to** the upstream Agent Substrate repository rather than mirroring it. Almost every command in the labs that follow is run from inside that clone (`./hack/install-ate.sh`, `go install ./cmd/kubectl-ate`, `./hack/teardown.sh`, etc.). Clone it now and `cd` into it before continuing.

## Lab Objectives

- Clone `github.com/agent-substrate/substrate` to a working directory
- Confirm you can run `git rev-parse --show-toplevel` from anywhere in the tree (Substrate's `hack/` scripts depend on it)
- Understand what lives where in the upstream tree

## Prerequisites

- `git` (any recent version)

## Clone

```bash
git clone https://github.com/agent-substrate/substrate.git
cd substrate
```

> **Run every Substrate command (`./hack/*.sh`, `go install ./cmd/kubectl-ate`, `go run ./tools/setup-gcp`) from the root of this clone.** The shell scripts resolve their working tree via `git rev-parse --show-toplevel`, and the demo manifests live inside this repo at relative paths.

Confirm:

```bash
git rev-parse --show-toplevel
# /<path>/substrate
```

## Repo Tour (What You Care About)

| Path | What it is | Used in lab |
|---|---|---|
| `hack/install-ate.sh` | Master installer for Substrate + all four demos. Sources `.ate-dev-env.sh`. | [040](040-install-substrate-helm.md) (referenced as alternative), [050](050-counter-demo.md)–[053](053-claude-code-multiplex.md) |
| `hack/ate-dev-env.sh.example` | Canonical env template (`PROJECT_ID`, `CLUSTER_NAME`, `BUCKET_NAME`, `KO_DOCKER_REPO`, etc.) | [020](020-configure-env.md) |
| `hack/teardown.sh` | GCP teardown (granular flags) | [099](099-cleanup.md) |
| `hack/create-kind-cluster.sh` | Local `kind` cluster + registry | [appendix-kind-quickstart](appendix-kind-quickstart.md) |
| `tools/setup-gcp/` | Go-based GCP provisioner (`go run ./tools/setup-gcp --all`) | [030](030-gcp-iam-and-bucket.md) (alternative to raw `gcloud`) |
| `manifests/ate-install/` | The raw Kubernetes YAMLs (used by `install-ate.sh`) | [appendix-install-script-alternative](appendix-install-script-alternative.md) |
| `cmd/kubectl-ate/` | `kubectl-ate` CLI source + README | [045](045-install-kubectl-ate.md) |
| `demos/counter/` | Stateful counter HTTP server demo | [050](050-counter-demo.md) |
| `demos/sandbox/` | Alpine sandbox + REPL client demo | [051](051-sandbox-demo.md) |
| `demos/agent-secret/` | Self-suspending agent with persistent RAM secret | [052](052-agent-secret-demo.md) |
| `demos/claude-code-multiplex/` | Three Claude Code agents on two pods (DRAFT) | [053](053-claude-code-multiplex.md) |
| `docs/architecture.md` | Master design doc — concepts, lifecycle phases, state model | linked from [000](000-overview.md) |
| `docs/api-guide.md` | CRD reference + gRPC API reference | linked from [040](040-install-substrate-helm.md), [070](070-kagent-agentharness.md) |
| `docs/observability.md` | Logs / metrics / tracing | [090](090-observability.md) |
| `cmd/kubectl-ate/README.md` | Full CLI reference (159 lines) | [045](045-install-kubectl-ate.md) |
| `benchmarking/` | Locust load tests + Prometheus/Grafana | [appendix-benchmarking](appendix-benchmarking.md) |
| `internal/`, `pkg/`, `vendor/`, `LICENSES/` | Go source + vendored deps + license tree. Read-only reference; not directly used by any lab. | — |

## A Note on Versioning

The upstream README is explicit: Substrate is in **VERY early development** and APIs are not stable. The validated combination this workshop was authored against is captured in the [README's "Validated Versions" table](README.md#validated-versions). If you check out a much newer or older commit than that, flags, chart names, and CRD shapes can shift under you.

## Next

- [010 — GKE Cluster Prerequisites](010-gke-cluster-prereqs.md)
