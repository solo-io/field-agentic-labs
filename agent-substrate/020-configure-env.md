# Configure Your Environment (`ate-dev-env.sh`)

`hack/install-ate.sh` (and `hack/teardown.sh`, and the `gcloud` snippets in [030](030-gcp-iam-and-bucket.md)) all source `.ate-dev-env.sh` from the root of the cloned Substrate repo. **You need this file even if you brought your own cluster** — the install script reads it directly.

## Lab Objectives

- Copy the upstream env template to `.ate-dev-env.sh`
- Set the seven values that actually matter
- `source` the file
- Understand which variables are "consumed by `setup-gcp` at cluster creation" and which are read by every script

## Prerequisites

- [001 — upstream Substrate repo cloned](001-clone-upstream.md)

## 1. Copy the Example

From the root of the cloned `substrate/` repo:

```bash
cp hack/ate-dev-env.sh.example .ate-dev-env.sh
```

> A mirror of this template is also at [`assets/env/ate-dev-env.sh.example`](assets/env/ate-dev-env.sh.example) in this workshop, so you can read it without cloning.

## 2. Set the Values That Matter

Edit `.ate-dev-env.sh`. The variables that **every script** reads:

| Variable | Example | Purpose |
|---|---|---|
| `PROJECT_ID` | `my-substrate-proj` | Target GCP project. |
| `PROJECT_NUMBER` | `123456789012` | Numeric project ID. The example file auto-derives it via `gcloud projects describe ${PROJECT_ID} --format='value(projectNumber)'`. Used to build IAM principals in [030](030-gcp-iam-and-bucket.md). |
| `GCE_REGION` | `us-central1` | Region for the **snapshot bucket** (multi-region works too — set it to whatever you want the bucket in). |
| `CLUSTER_LOCATION` | `us-central1-c` | Zone (or region) your **cluster** lives in. This — not `GCE_REGION` — is what `gcloud container` commands take as `--location`. |
| `CLUSTER_NAME` | `substrate-poc` | Name of your GKE cluster. |
| `BUCKET_NAME` | `snapshot-substrate-test-${PROJECT_ID}` | GCS bucket for actor snapshots. **Must be globally unique** — the example file templates `${PROJECT_ID}` into it, which usually keeps it unique. |
| `KO_DOCKER_REPO` | `gcr.io/${PROJECT_ID}/ate-images` | Where `ko` pushes images. If you took the Helm install in [040](040-install-substrate-helm.md), this only matters when you build demo workloads ([050](050-counter-demo.md) onwards). |

And one **optional** but **strongly recommended** for an existing cluster:

| Variable | Example | Purpose |
|---|---|---|
| `KUBECTL_CONTEXT` | `gke_my-proj_us-central1-c_substrate-poc` | If set, `install-ate.sh` uses this kubeconfig context and **skips** its `gcloud get-credentials` call. Without it the script tries to fetch credentials, which can clobber a context you've configured by hand. |

### "Creation-only" — leave alone

The example file also carries:

```
CLUSTER_VERSION
NODE_POOL_NAME
NODE_POOL_VERSION
GVISOR_NODE_MACHINE_TYPE
NETWORK
SUBNETWORK
KO_DEFAULTPLATFORMS
```

These are **only** consumed by `tools/setup-gcp` when it *creates* a cluster for you. If you brought your own cluster, leave them at the defaults — nothing in this workshop's lab flow reads them.

## 3. Source the File

```bash
source .ate-dev-env.sh
```

Confirm the values stick:

```bash
echo "PROJECT_ID=$PROJECT_ID"
echo "PROJECT_NUMBER=$PROJECT_NUMBER"
echo "CLUSTER_NAME=$CLUSTER_NAME"
echo "BUCKET_NAME=$BUCKET_NAME"
echo "KO_DOCKER_REPO=$KO_DOCKER_REPO"
```

You'll need to `source` this file in **every new shell** where you run Substrate commands. Many of the snippets in the rest of the workshop assume these env vars are set.

## 4. Don't Commit `.ate-dev-env.sh`

It's gitignored in the upstream repo by default. **Do not commit it** — your `PROJECT_ID`, `KO_DOCKER_REPO`, and (transitively) `PROJECT_NUMBER` are all leaked if you do.

## Next

- [030 — GCP IAM, Snapshot Bucket, and `kubectl` Context](030-gcp-iam-and-bucket.md)
