# GCP IAM, Snapshot Bucket, and `kubectl` Context

The second mandatory setup lab. Creates the GCS bucket Substrate uses for actor snapshots, grants the `atelet` Workload Identity principal + your GKE node service account the IAM bindings they need, and points `kubectl` at the cluster.

## Lab Objectives

- Enable Workload Identity on the cluster (if not already)
- Confirm or enable the GKE Metadata Server on every node pool that runs Substrate workloads
- Create the GCS snapshot bucket and grant `atelet` bucket-scoped IAM
- Grant project-level IAM to the `atelet` principal + the node service account (image pulls + snapshot writes)
- Configure `kubectl` to point at the cluster

## Prerequisites

- [001 - Baseline Setup](001-baseline-setup.md) completed (cluster up, beta APIs enabled, repo cloned, `.ate-dev-env.sh` sourced)
- `gcloud` authenticated against `${PROJECT_ID}` (login + ADC)

```bash
# Confirm everything from 001 is still in your shell:
for V in PROJECT_ID PROJECT_NUMBER CLUSTER_NAME CLUSTER_LOCATION GCE_REGION BUCKET_NAME KO_DOCKER_REPO; do
  if [ -z "${!V}" ]; then echo "MISSING: $V"; else printf '  OK  %-20s %s\n' "$V" "${!V}"; fi
done
```

If anything's missing, re-source `.ate-dev-env.sh` from the substrate repo root.

## 1. Derive the Two IAM Identities

The `atelet` DaemonSet authenticates to GCS via Workload Identity. Build the principal string for the K8s SA it runs as (`atelet` in `ate-system`):

```bash
export ATELET_PRINCIPAL="principal://iam.googleapis.com/projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/${PROJECT_ID}.svc.id.goog/subject/ns/ate-system/sa/atelet"
export NODE_SA="${PROJECT_NUMBER}-compute@developer.gserviceaccount.com"
```

> `NODE_SA` defaults to the **default Compute Engine SA**. If you created your cluster with a custom node SA, discover the real value:
>
> ```bash
> gcloud container node-pools describe <pool-name> \
>   --cluster="${CLUSTER_NAME}" --location="${CLUSTER_LOCATION}" \
>   --project="${PROJECT_ID}" --format='value(config.serviceAccount)'
> ```
>
> Set `NODE_SA` to whatever that returns. Repeat for every pool that may run Substrate workloads - `atelet` runs as a DaemonSet so it lands on every node.

## 2. Enable Workload Identity on the Cluster

Additive update, safe to re-run:

```bash
gcloud container clusters update "${CLUSTER_NAME}" \
  --location="${CLUSTER_LOCATION}" --project="${PROJECT_ID}" \
  --workload-pool="${PROJECT_ID}.svc.id.goog"
```

## 3. Confirm the GKE Metadata Server On Every Pool

The `atelet` Workload Identity binding only resolves on pools running the GKE Metadata Server. Pools created after `--workload-pool` is enabled get it by default; pre-existing pools may not.

For each pool:

```bash
gcloud container node-pools describe <pool-name> --cluster="${CLUSTER_NAME}" \
  --location="${CLUSTER_LOCATION}" --project="${PROJECT_ID}" \
  --format='value(config.workloadMetadataConfig.mode)'
```

`GKE_METADATA` = good. Blank or `GCE_METADATA` = enable it:

```bash
gcloud container node-pools update <pool-name> --cluster="${CLUSTER_NAME}" \
  --location="${CLUSTER_LOCATION}" --project="${PROJECT_ID}" \
  --workload-metadata=GKE_METADATA
```

## 4. Create the Snapshot Bucket

```bash
gcloud storage buckets create "gs://${BUCKET_NAME}" \
  --project="${PROJECT_ID}" --location="${GCE_REGION}" \
  --uniform-bucket-level-access
```

If the bucket name is taken globally, `BUCKET_NAME` collides with someone else's. Pick a different one in `.ate-dev-env.sh` and re-source.

## 5. Bucket-Scoped IAM for `atelet`

```bash
gcloud storage buckets add-iam-policy-binding "gs://${BUCKET_NAME}" \
  --member="${ATELET_PRINCIPAL}" --role=roles/storage.objectAdmin
gcloud storage buckets add-iam-policy-binding "gs://${BUCKET_NAME}" \
  --member="${ATELET_PRINCIPAL}" --role=roles/storage.bucketViewer
```

## 6. Project-Level IAM (Image Pull + Snapshot Access)

```bash
# Node SA — for image pulls
gcloud projects add-iam-policy-binding "${PROJECT_ID}" \
  --member="serviceAccount:${NODE_SA}" --role=roles/storage.objectViewer
gcloud projects add-iam-policy-binding "${PROJECT_ID}" \
  --member="serviceAccount:${NODE_SA}" --role=roles/artifactregistry.reader

# atelet — for project-scope snapshot writes + image pulls
gcloud projects add-iam-policy-binding "${PROJECT_ID}" \
  --member="${ATELET_PRINCIPAL}" --role=roles/storage.objectAdmin
gcloud projects add-iam-policy-binding "${PROJECT_ID}" \
  --member="${ATELET_PRINCIPAL}" --role=roles/artifactregistry.reader
```

## 7. Configure `kubectl`

```bash
gcloud container clusters get-credentials "${CLUSTER_NAME}" \
  --location="${CLUSTER_LOCATION}" --project="${PROJECT_ID}"

kubectl get nodes
```

If you already have a kubeconfig context for the cluster (`KUBECTL_CONTEXT` set in `.ate-dev-env.sh`), `install-ate.sh` will use it and skip this step.

## Verify

```bash
# atelet bindings
gcloud projects get-iam-policy "${PROJECT_ID}" \
  --flatten='bindings[].members' \
  --format='table(bindings.role)' \
  --filter="bindings.members:${ATELET_PRINCIPAL}"

# Bucket-scoped atelet bindings
gcloud storage buckets get-iam-policy "gs://${BUCKET_NAME}" \
  --format='table(bindings.role,bindings.members)' \
  --filter="bindings.members:${ATELET_PRINCIPAL}"
```

## Cleanup

Roll back this lab only when you're done with the entire workshop. Full teardown is in [099](099-cleanup.md). The granular GCP rollback:

```bash
# Bucket
gcloud storage buckets delete "gs://${BUCKET_NAME}" --project="${PROJECT_ID}" --quiet

# Project IAM bindings
gcloud projects remove-iam-policy-binding "${PROJECT_ID}" \
  --member="${ATELET_PRINCIPAL}" --role=roles/storage.objectAdmin       --condition=None
gcloud projects remove-iam-policy-binding "${PROJECT_ID}" \
  --member="${ATELET_PRINCIPAL}" --role=roles/artifactregistry.reader   --condition=None
gcloud projects remove-iam-policy-binding "${PROJECT_ID}" \
  --member="serviceAccount:${NODE_SA}" --role=roles/storage.objectViewer  --condition=None
gcloud projects remove-iam-policy-binding "${PROJECT_ID}" \
  --member="serviceAccount:${NODE_SA}" --role=roles/artifactregistry.reader --condition=None

unset ATELET_PRINCIPAL NODE_SA
```

The Workload Identity + Metadata Server cluster mutations are kept - see [099](099-cleanup.md) for the rest.

## Command-Accuracy Note

These `gcloud` flags were verified against Google Cloud SDK **484.0.0**. Flags shift between releases - confirm with `gcloud <group> <command> --help` if yours differs.

## Next

- [003 - Install Substrate](003-install-substrate.md)
