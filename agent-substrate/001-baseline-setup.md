# Baseline Setup

The first mandatory setup lab. Goes from "I have a Kubernetes cluster" to "I have the local tools + cloned source needed to install Substrate." Subsequent labs assume this baseline is in place.

## Lab Objectives

- Confirm your cluster meets Substrate's prerequisites (GKE Standard with `--enable-kubernetes-unstable-apis`, or another cluster where you control the apiserver feature gates)
- Install Go ≥ 1.26.3, `kubectl`, `helm`, `gcloud`, `git`, `openssl`
- Clone the upstream Substrate repo
- Source the env-file template

## Why GKE

Three Substrate system components (`ate-api-server`, `atenet-router`, `valkey`) mount a `podCertificate` projected volume that requires Kubernetes apiserver-level beta APIs (`certificates.k8s.io/v1beta1/podcertificaterequests`, `clustertrustbundles`). These are off by default upstream. GKE Standard exposes the `--enable-kubernetes-unstable-apis` flag to flip them on; managed AKS and EKS don't expose this knob.

If you're not on GKE, the local-dev escape hatch is `kind` - see [appendix-kind-quickstart.md](appendix-kind-quickstart.md). For the full GKE rationale, see [appendix-why-gke.md](appendix-why-gke.md).

## Prerequisites

- A GKE **Standard** cluster (not Autopilot - `atelet` runs a privileged container + hostPath that Autopilot rejects). Or any cluster where you can pass apiserver feature gates.
- A GCP project with billing enabled (used for the snapshot bucket in [002](002-gcp-iam-and-bucket.md), even if the cluster is elsewhere)
- `gcloud` authenticated three ways:

  ```bash
  gcloud auth login
  gcloud auth application-default login --project=<your-project-id>
  gcloud auth configure-docker gcr.io
  ```

## 1. Confirm Local Tools

| Tool | Required version | Check |
|---|---|---|
| `git` | any recent | `git --version` |
| `go` | ≥ 1.26.3 | `go version` |
| `kubectl` | matches cluster minor | `kubectl version --client` |
| `helm` | v3 | `helm version --short` |
| `gcloud` | recent (484.0.0+ validated) | `gcloud version` |
| `openssl` | any recent | `openssl version` |

Quick check loop:

```bash
for cmd in git go kubectl helm gcloud openssl; do
  printf '%-10s ' "$cmd"
  command -v "$cmd" >/dev/null && echo "OK" || echo "MISSING"
done
```

If Go is missing, install it from <https://go.dev/dl/>. The `kubectl-ate` CLI you'll build in [003](003-install-substrate.md) needs Go ≥ 1.26.3.

## 2. Confirm the Cluster

```bash
kubectl version
kubectl get nodes
```

Verify it's a Standard (not Autopilot) GKE cluster - Autopilot won't accept Substrate's workloads:

```bash
gcloud container clusters describe <cluster> \
  --location=<region-or-zone> --project=<project> \
  --format='value(autopilot.enabled)'
# Expected: empty (Standard). "True" = Autopilot, will not work.
```

If you haven't yet enabled the Pod Certificate beta APIs on the cluster, do that now (it's an additive update):

```bash
gcloud container clusters update <cluster> \
  --location=<location> --project=<project> \
  --enable-kubernetes-unstable-apis=certificates.k8s.io/v1beta1/podcertificaterequests,certificates.k8s.io/v1beta1/clustertrustbundles
```

> Existing nodes don't always honor the kubelet feature for `podCertificate` volumes - only nodes created after the feature was enabled do. If [003 step 1](003-install-substrate.md#1-install-substrate-helm) shows `ate-api-server` / `atenet-router` / `valkey` stuck failing to mount the cert volume, create a fresh node pool: `gcloud container node-pools create new-pool --cluster=<cluster> --machine-type=c3-standard-4 --workload-metadata=GKE_METADATA --num-nodes=2`.

## 3. Clone the Upstream Substrate Repo

Everything from [002 onwards](002-gcp-iam-and-bucket.md) runs from inside this clone:

```bash
git clone https://github.com/agent-substrate/substrate.git
cd substrate
```

Confirm `git rev-parse --show-toplevel` returns the clone path - Substrate's `hack/` scripts depend on it.

> Substrate is in **VERY early development**. APIs are not stable. Check out a specific tag/commit if you want reproducibility; otherwise `main` is fine for a POC.

## 4. Source the Environment File

`hack/install-ate.sh` (and the GCP commands in [002](002-gcp-iam-and-bucket.md)) read variables from `.ate-dev-env.sh` at the repo root:

```bash
cp hack/ate-dev-env.sh.example .ate-dev-env.sh
```

Edit `.ate-dev-env.sh` and set at minimum:

| Variable | Purpose |
|---|---|
| `PROJECT_ID` | Target GCP project |
| `GCE_REGION` | Region for the snapshot bucket (e.g. `us-central1`) |
| `CLUSTER_LOCATION` | Zone / region your cluster lives in (e.g. `us-central1-c`) |
| `CLUSTER_NAME` | Your existing GKE cluster name |
| `BUCKET_NAME` | GCS bucket for actor snapshots - **must be globally unique** |
| `KO_DOCKER_REPO` | Where `ko` pushes images (e.g. `gcr.io/<project>/ate-images`) |

`PROJECT_NUMBER` auto-derives from `gcloud projects describe`. Leave the creation-only vars (`CLUSTER_VERSION`, `NODE_POOL_*`, `NETWORK`, etc.) at their defaults - they're consumed by `tools/setup-gcp` if you ask it to create a cluster, and unused otherwise.

Source it:

```bash
source .ate-dev-env.sh
```

A mirror of the upstream template is also at [`assets/env/ate-dev-env.sh.example`](assets/env/ate-dev-env.sh.example) in this workshop.

Confirm the values:

```bash
for V in PROJECT_ID PROJECT_NUMBER GCE_REGION CLUSTER_LOCATION CLUSTER_NAME BUCKET_NAME KO_DOCKER_REPO; do
  printf '%-20s %s\n' "$V" "${!V}"
done
```

> **Do not commit `.ate-dev-env.sh`** - it's gitignored upstream for a reason. Your `PROJECT_ID` and `KO_DOCKER_REPO` are leaked if you do.

## What's In Place After This Lab

| Resource | State |
|---|---|
| GKE Standard cluster | Up, beta APIs enabled |
| Local tools | `git`, `go ≥ 1.26.3`, `kubectl`, `helm`, `gcloud`, `openssl` all present |
| `gcloud` | Authenticated (login + ADC + docker helper) |
| Substrate repo | Cloned, you're `cd`'d into it |
| `.ate-dev-env.sh` | Sourced, env vars exported |

This is the **baseline** every unit-of-value lab from 010 onwards assumes.

## Cleanup

Roll back this lab only when you're done with the entire workshop. Full teardown is in [099](099-cleanup.md). The local-only steps:

```bash
# Drop the cloned repo + env file
cd ..
rm -rf substrate

# Unset env vars
unset PROJECT_ID PROJECT_NUMBER GCE_REGION CLUSTER_LOCATION CLUSTER_NAME \
      BUCKET_NAME KO_DOCKER_REPO NODE_POOL_NAME GVISOR_NODE_MACHINE_TYPE
```

The cluster mutation (`--enable-kubernetes-unstable-apis`) cannot be undone - see [appendix-why-gke.md](appendix-why-gke.md). If you want a pristine cluster, delete and recreate it.

## Next

- [002 - GCP IAM, Snapshot Bucket, and `kubectl` Context](002-gcp-iam-and-bucket.md)
