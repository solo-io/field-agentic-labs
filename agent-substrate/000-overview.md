# Overview & Architecture

**Agent Substrate** is a Kubernetes-native system that multiplexes many idle "actors" (agent-like workloads) onto a small pool of warm "worker" pods. It builds on top of Kubernetes Pods + autoscaling but **takes the Kubernetes control plane out of the critical path** to hit sub-100ms activation latency.

The core trick: agents spend most of their time idle waiting for input. Instead of one Pod per agent, Substrate suspends each actor to a snapshot (RAM + disk) when idle, frees the Pod, and resumes from the snapshot on whatever worker is free when the next request arrives.

## The Two Layers

```
┌──────────────────────────────────────────────────────────────────────────┐
│  Kubernetes API (CRDs — declarative, low-churn)                          │
│                                                                          │
│   WorkerPool  ────────────────  ActorTemplate                            │
│   (pool of warm pods)           (image / cmd / env, "golden snapshot")   │
└──────────────────────────────────────────────────────────────────────────┘
                                  │
                                  │ Substrate control plane reads these
                                  ▼
┌──────────────────────────────────────────────────────────────────────────┐
│  Substrate control plane (ate-api-server)  ──  state in Valkey/Redis     │
│                                                (NOT etcd — high churn)   │
│                                                                          │
│   Actor                  Worker                                          │
│   (instance of a         (a single pod                                   │
│    template, RUNNING     in a WorkerPool,                                │
│    or SUSPENDED)         FREE or ASSIGNED)                               │
└──────────────────────────────────────────────────────────────────────────┘
```

`WorkerPool` and `ActorTemplate` live in the Kubernetes API as CRDs because they're declarative and rarely change. `Actor` and `Worker` instance state lives in **Valkey** (Redis-compatible) because there can be millions of them and they update many times per second — putting that in etcd would melt the API server.

## Key Components

| Component | What it does |
|---|---|
| `ate-api-server` | The gRPC control plane (`ateapi.Control`). `CreateActor` / `ResumeActor` / `SuspendActor` / `DeleteActor` / `GetActor` / `ListActors` / `ListWorkers`. |
| `atecontroller` | Kubernetes controller reconciling `WorkerPool` and `ActorTemplate` CRDs. |
| `atelet` | Node-level DaemonSet on every node. Supervises worker pods, streams snapshots to/from GCS, talks to the control plane. |
| `ateom` | "Interior gVisor" container that runs **inside** each worker pod. Provides a gRPC interface for `atelet` to trigger `RunWorkload`, `CheckpointWorkload`, `RestoreWorkload`. |
| `atenet` | Envoy + ext_proc router. Reads the actor ID from the `Host` header and triggers `ResumeActor` on cache miss. |
| `podcertcontroller` | "Polyfill" Pod Certificate signer that issues per-pod mTLS certs for the system components. Needs the Pod Certificate beta APIs on the cluster — see [appendix-why-gke](appendix-why-gke.md). |
| `valkey` | High-perf Redis-compatible state store for `Actor` and `Worker` rows. Deployed as a 6-replica `StatefulSet`. |
| `kubectl-ate` | `kubectl` plugin for managing actor / worker lifecycles from the command line. |

## Actor Lifecycle

Four phases — all of them go through `ate-api-server` / `atelet` / `ateom`, **not** through the Kubernetes scheduler:

1. **Creation (`CreateActor`)** — Record written to Valkey as `SUSPENDED`, referencing an `ActorTemplate`. **No pod consumed.** New actors hydrate from the template's "golden snapshot" (Version 0).
2. **Activation (`ResumeActor`)** — Triggered by an inbound request hitting the `atenet` router (or an explicit API call). Control plane claims a warm worker from the `WorkerPool`; `atelet` + `ateom` restore the snapshot into the gVisor sandbox; status → `RUNNING`.
3. **Hibernation (`SuspendActor`)** — `ateom` freezes the process and captures a memory+disk snapshot with `runsc`; `atelet` streams the snapshot to GCS; the worker pod is wiped and returned to the pool; status → `SUSPENDED`.
4. **Deletion** — Only `SUSPENDED` actors can be deleted. After deletion, the snapshot is garbage-collected (GC is not yet implemented as of the validated version).

The payoff: in-memory state (the counter in [050](050-counter-demo.md), the secret in [052](052-agent-secret-demo.md)) survives the cycle. The next resume can land on a **different** worker and the actor still picks up where it left off.

## Why Pod Certificates Need GKE

Three system components mount a `podCertificate` projected volume: `ate-api-server`, `atenet-router`, and `valkey` (a 6-replica StatefulSet plus the cluster-init Job — so several pods). That volume requires the apiserver to serve `certificates.k8s.io/v1beta1` + the `PodCertificateRequest`, `ClusterTrustBundle`, and `ClusterTrustBundleProjection` feature gates — **off by default** as of Kubernetes 1.36.

GKE exposes the `--enable-kubernetes-unstable-apis` flag for exactly this purpose (lab [010](010-gke-cluster-prereqs.md) uses it). Managed AKS and EKS don't expose this knob today; for those, you'd need a cluster where you control the apiserver flags (Cluster API / kubeadm / k3s). See [appendix-why-gke](appendix-why-gke.md) for the full rationale.

## Recommended Lab Flow

1. **[000](000-overview.md) → [001](001-clone-upstream.md)** Read this, then clone the upstream repo.
2. **[010](010-gke-cluster-prereqs.md)** GKE Standard cluster with Pod Certificate beta APIs + Workload Identity enabled.
3. **[020](020-configure-env.md) → [030](030-gcp-iam-and-bucket.md)** Env file + IAM + snapshot bucket.
4. **[040](040-install-substrate-helm.md) → [045](045-install-kubectl-ate.md)** Install Substrate (Helm OCI) + `kubectl-ate`.
5. **[050](050-counter-demo.md)** First demo — counter. Validates the whole stack.
6. **[051](051-sandbox-demo.md) → [052](052-agent-secret-demo.md) → [053](053-claude-code-multiplex.md)** Optional additional demos.
7. **[060](060-install-kagent-with-substrate.md) → [070](070-kagent-agentharness.md)** Kagent integration on top.
8. **[080](080-operations-suspend-resume.md) → [090](090-observability.md)** Day-2 ops.
9. **[099](099-cleanup.md)** Tear down.

If you only want the kagent integration end-to-end, follow the [kagent-track](tracks/kagent-track.md).

## Asset Conventions

This workshop **links out to the upstream repo** instead of mirroring it. After [001](001-clone-upstream.md), `cd` into the cloned `substrate/` directory and run commands from there. The labs reference paths like `hack/install-ate.sh`, `demos/counter/`, `cmd/kubectl-ate/` — all of which live in the upstream clone, not in this workshop repo.

The only files checked in under [`assets/`](assets/) here are:

| Path | Purpose |
|---|---|
| `assets/env/ate-dev-env.sh.example` | Copy of the upstream env template, so you can read it without cloning |
| `assets/agentharness/openclaw-substrate-demo.yaml` | Parameterized `AgentHarness` (no hardcoded `felevan` bucket) for [070](070-kagent-agentharness.md) |
| `assets/agentharness/gateway-token.yaml` | Gateway token Secret template for [070](070-kagent-agentharness.md) |
| `assets/images/{kagent.png,suberror.png}` | Two screenshots referenced from [060](060-install-kagent-with-substrate.md) |

## Service & Port Reference

| Service | Namespace | Port | Purpose |
|---------|-----------|------|---------|
| `ate-api-server` (sometimes `api`) | `ate-system` | 443 (gRPC, TLS) | Control plane — `ateapi.Control` + `ateapi.SessionIdentity` |
| `atenet-router` | `ate-system` | 80 | Envoy router for `*.actors.resources.substrate.ate.dev` |
| `valkey` | `ate-system` | 6379 | State store (Redis-compatible) |
| `kagent-controller` | `kagent` | 8083 | kagent controller API (substrate status, harness gateway) |
| `kagent-ui` | `kagent` | 8080 | kagent UI |
