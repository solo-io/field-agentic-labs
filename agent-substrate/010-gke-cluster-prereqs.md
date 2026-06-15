# GKE Cluster Prerequisites

Substrate's control plane (`ate-api-server`, `atenet-router`, `valkey`) mounts a `podCertificate` projected volume, which requires the Pod Certificate **beta APIs** on the cluster — off by default in upstream Kubernetes. GKE is the supported managed path because it exposes a knob to enable those APIs. This lab covers the cluster-level requirements; the next two labs handle env config and IAM/bucket setup.

## Lab Objectives

- Pick / confirm a **GKE Standard** cluster (not Autopilot)
- Confirm the cluster's region/zone, version, and a node pool with room for the worker pods
- Verify the required Google tools and APIs are on
- Install Go ≥ 1.26.3 (needed to build `kubectl-ate`)

## Why GKE / Why Standard

- **Pod Certificate beta APIs.** `ate-api-server`, `atenet-router`, and the 6-replica `valkey` StatefulSet all mount `podCertificate` projected volumes. The volume must mount for the pod to run. GKE's `--enable-kubernetes-unstable-apis` is the supported knob — see [appendix-why-gke](appendix-why-gke.md) for the full rationale.
- **Autopilot is unsupported.** `atelet` runs a `privileged` container and mounts a `hostPath`; the worker pods it manages do the same. Autopilot rejects both. Use a **GKE Standard** cluster.
- **No special node pool or `runtimeClassName`.** `runsc` runs nested **inside** the worker pod, so a standard node pool is enough. Use any 4-vCPU machine type (e.g. `c3-standard-4`) — pick one with stock in your region.

## Prerequisites

- A GCP **project** with billing enabled. Set it once in your shell:

  ```bash
  export PROJECT_ID=<your-project-id>
  ```

- Authenticated three ways — CLI login (for `gcloud`), Application Default Credentials (for client libraries — `tools/setup-gcp`), and a Docker credential helper (so `ko` can push):

  ```bash
  gcloud auth login
  gcloud auth application-default login --project="$PROJECT_ID"
  gcloud auth configure-docker gcr.io
  # Or, for Artifact Registry:
  # gcloud auth configure-docker us-docker.pkg.dev
  ```

- Required APIs on the project:

  ```bash
  gcloud services enable \
    cloudresourcemanager.googleapis.com \
    container.googleapis.com \
    networkconnectivity.googleapis.com \
    serviceusage.googleapis.com \
    storage.googleapis.com \
    --project="$PROJECT_ID"
  ```

## Required Tools

| Tool | Version | Why |
|---|---|---|
| Go | ≥ 1.26.3 | Matches Substrate's `go.mod` toolchain. Builds `kubectl-ate`. |
| `gcloud` | recent (verified 484.0.0) | Cluster, registry, IAM. |
| `kubectl` | matches your cluster minor | Substrate targets the latest stable Kubernetes release and the one prior. |
| `git` | any recent | The `hack/` scripts resolve the repo root via `git rev-parse`. |
| `openssl` | any recent | `hack/install-ate.sh` uses it for the valkey CA cert (DER → PEM). |
| `curl` | any recent | Drives traffic to the actor in [050](050-counter-demo.md). |
| `helm` | v3 | [040](040-install-substrate-helm.md) and [060](060-install-kagent-with-substrate.md). |

> **`ko` is installed for you.** `hack/install-ate.sh` invokes `ko` to build the Substrate images and push them to your `KO_DOCKER_REPO`. If you take the Helm path in [040](040-install-substrate-helm.md), `ko` is not in the loop — the chart pulls pre-built images from `ghcr.io/kagent-dev/substrate/...`.

## Bring the Cluster Up

If you already have a GKE Standard cluster, skip ahead to [Verify](#verify). If you need to create one, the simplest path is:

```bash
gcloud container clusters create substrate-poc \
  --location=us-central1-c \
  --project="$PROJECT_ID" \
  --machine-type=c3-standard-4 \
  --num-nodes=2 \
  --workload-pool="${PROJECT_ID}.svc.id.goog" \
  --enable-kubernetes-unstable-apis=certificates.k8s.io/v1beta1/podcertificaterequests,certificates.k8s.io/v1beta1/clustertrustbundles
```

> Creating with `--workload-pool` and `--enable-kubernetes-unstable-apis` from the start means you skip the cluster-update step in [030](030-gcp-iam-and-bucket.md) (Step 2a). Pools created after the beta APIs are enabled automatically get the kubelet feature support.

Alternative — Substrate's own provisioner (Go-based, drives `gcloud` for you):

```bash
go run ./tools/setup-gcp --all
```

See `go run ./tools/setup-gcp --help` for granular options (`--create-cluster`, `--create-gvisor-node-pool`, etc.).

## Verify

```bash
kubectl get nodes
gcloud container clusters describe <cluster> \
  --location=<location> --project="$PROJECT_ID" \
  --format='value(workloadIdentityConfig.workloadPool)'

# List enabled beta APIs (k8s 1.30+):
gcloud container clusters describe <cluster> \
  --location=<location> --project="$PROJECT_ID" \
  --format='value(enableKubernetesUnstableApis)'
```

You should see:

- `workloadPool: <PROJECT_ID>.svc.id.goog`
- The enabled beta APIs include both `podcertificaterequests` and `clustertrustbundles`

## Multi-Pool Considerations

`atelet` runs as a **DaemonSet** (every node) and the worker pods it manages have **no node selector**, so Substrate workloads can land on *any* node pool. If your cluster has more than one pool:

- Repeat the **GKE Metadata Server** check (see [030 step 2a](030-gcp-iam-and-bucket.md#2a-cluster-mutations--pod-certificate-beta-apis--workload-identity)) for every pool that may run Substrate workloads
- Repeat the **node SA discovery + IAM grants** for every pool — pools created with a custom node service account will not inherit the default Compute Engine SA's bindings
- Repeat the **beta-API node rollout** for every pool that predates the `--enable-kubernetes-unstable-apis` step

## `runsc` and `--allow-connected-on-save`

Substrate checkpoints actors with `runsc`. The current checkpoint/restore path requires a `runsc` build that supports `--allow-connected-on-save` (works around a networking-resumption bug). The demo `ActorTemplate`s under `demos/*/`.yaml.tmpl pin a specific `runsc` nightly that has it — you don't need to install `runsc` yourself.

## Next

- [020 — Configure Your Environment (`ate-dev-env.sh`)](020-configure-env.md)
