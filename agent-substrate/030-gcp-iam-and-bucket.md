# GCP IAM, Snapshot Bucket, and `kubectl` Context

This lab sets up the GCP-side dependencies Substrate needs on top of the cluster: the snapshot bucket, the IAM bindings for the `atelet` Workload Identity principal and the node service account, and (optionally) the cluster mutations to enable Pod Certificate beta APIs + Workload Identity if you didn't create the cluster with those flags.

## Lab Objectives

- Derive the `atelet` Workload Identity principal and the node service account
- (If needed) Enable Pod Certificate beta APIs + Workload Identity on the cluster
- Create a GCS snapshot bucket and grant `atelet` access to it
- Grant project-level image-pull + snapshot IAM to the node SA and the `atelet` principal
- Point `kubectl` at the cluster

## Prerequisites

- [010 — GKE cluster ready](010-gke-cluster-prereqs.md)
- [020 — `.ate-dev-env.sh` sourced](020-configure-env.md)

## Derive the Two Identities

The `atelet` DaemonSet authenticates to GCS via **Workload Identity**. It runs as the `atelet` Kubernetes ServiceAccount in `ate-system`, which maps to a Workload Identity principal you can grant IAM to:

```bash
export ATELET_PRINCIPAL="principal://iam.googleapis.com/projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/${PROJECT_ID}.svc.id.goog/subject/ns/ate-system/sa/atelet"
export NODE_SA="${PROJECT_NUMBER}-compute@developer.gserviceaccount.com"
```

> **`NODE_SA` defaults to the default Compute Engine SA.** GKE recommends a custom node SA, and your existing pools may already use one. Discover what your pool actually runs as:
>
> ```bash
> gcloud container node-pools describe <pool-name> \
>   --cluster="$CLUSTER_NAME" --location="$CLUSTER_LOCATION" \
>   --project="$PROJECT_ID" --format='value(config.serviceAccount)'
> ```
>
> If it returns anything other than `default`, set `NODE_SA` to **that** SA address. The image-pull bindings in [Step 2d](#2d-project-level-iam-image-pull--atelet-snapshot-access) must go to whatever SA each pool actually runs as. On a multi-pool cluster, repeat this for every pool that may run Substrate workloads.

## 2a. Cluster Mutations — Pod Certificate Beta APIs + Workload Identity

Skip this section if you created the cluster in [010](010-gke-cluster-prereqs.md) with `--enable-kubernetes-unstable-apis` and `--workload-pool`. Both are additive updates — safe to re-run.

```bash
gcloud container clusters update "$CLUSTER_NAME" \
  --location="$CLUSTER_LOCATION" --project="$PROJECT_ID" \
  --enable-kubernetes-unstable-apis=certificates.k8s.io/v1beta1/podcertificaterequests,certificates.k8s.io/v1beta1/clustertrustbundles

gcloud container clusters update "$CLUSTER_NAME" \
  --location="$CLUSTER_LOCATION" --project="$PROJECT_ID" \
  --workload-pool="${PROJECT_ID}.svc.id.goog"
```

### Reactive checks (run only if Step 3 in [040](040-install-substrate-helm.md) hits these symptoms)

> **Node rollout — check reactively.** The `podCertificate` projected volume is a **kubelet**-level feature baked in at node creation. Existing nodes generally need to be **recreated** before they honor it. Rather than recreate pre-emptively, let the install tell you: install Substrate in [040](040-install-substrate-helm.md) and watch `kubectl get pods -n ate-system`. If `ate-api-server` / `atenet-router` / `valkey` come up Ready, your nodes already serve it — do nothing. If they're stuck failing to mount the cert volume (`kubectl describe pod` shows `MountVolume.SetUp failed ... ClusterTrustBundle projection is not supported in static kubelet mode`, or `credential bundle is not issued yet` that never clears), recreate nodes.
>
> The reliable remedy is a **fresh node pool** (born after the beta-API enablement, with the Metadata Server set):
>
> ```bash
> gcloud container node-pools create <new-pool> --cluster="$CLUSTER_NAME" \
>   --location="$CLUSTER_LOCATION" --project="$PROJECT_ID" \
>   --machine-type=c3-standard-4 --num-nodes=2 --workload-metadata=GKE_METADATA
> ```
>
> Then migrate workloads off the old pool so the stuck pods reschedule on the new nodes.
>
> An in-place node-pool upgrade also works **only if it actually recreates nodes** — i.e. the target version is *later* than the current node version. Same-version upgrades on an existing cluster are no-ops.

> **GKE Metadata Server — check, then update if needed.** The `atelet` Workload Identity binding only resolves on pools running the GKE Metadata Server. Pools created *after* you enable `--workload-pool` get it by default; pre-existing pools may not.
>
> Check each pool:
>
> ```bash
> gcloud container node-pools describe <pool-name> --cluster="$CLUSTER_NAME" \
>   --location="$CLUSTER_LOCATION" --project="$PROJECT_ID" \
>   --format='value(config.workloadMetadataConfig.mode)'
> ```
>
> If it prints `GKE_METADATA`, you're set. If it's blank or `GCE_METADATA`:
>
> ```bash
> gcloud container node-pools update <pool-name> --cluster="$CLUSTER_NAME" \
>   --location="$CLUSTER_LOCATION" --project="$PROJECT_ID" --workload-metadata=GKE_METADATA
> ```

## 2b. Create the Snapshot Bucket

```bash
gcloud storage buckets create "gs://${BUCKET_NAME}" \
  --project="$PROJECT_ID" --location="$GCE_REGION" --uniform-bucket-level-access
```

The bucket name must be **globally unique** across all of GCS. If it's taken, change `BUCKET_NAME` in `.ate-dev-env.sh` and re-source.

## 2c. Bucket-Scoped IAM for `atelet`

```bash
gcloud storage buckets add-iam-policy-binding "gs://${BUCKET_NAME}" \
  --member="$ATELET_PRINCIPAL" --role=roles/storage.objectAdmin
gcloud storage buckets add-iam-policy-binding "gs://${BUCKET_NAME}" \
  --member="$ATELET_PRINCIPAL" --role=roles/storage.bucketViewer
```

## 2d. Project-Level IAM (image pull + `atelet` snapshot access)

The node SA needs to pull images; the `atelet` principal needs project-level snapshot + image-pull permissions:

```bash
gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="serviceAccount:${NODE_SA}" --role=roles/storage.objectViewer
gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="serviceAccount:${NODE_SA}" --role=roles/artifactregistry.reader

gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="$ATELET_PRINCIPAL" --role=roles/storage.objectAdmin
gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="$ATELET_PRINCIPAL" --role=roles/artifactregistry.reader
```

## 2e. Point `kubectl` at Your Cluster

```bash
gcloud container clusters get-credentials "$CLUSTER_NAME" \
  --location="$CLUSTER_LOCATION" --project="$PROJECT_ID"
kubectl get nodes
```

If you set `KUBECTL_CONTEXT` in `.ate-dev-env.sh` ([020](020-configure-env.md)) and already have credentials, this `get-credentials` call is what `install-ate.sh` would skip.

## Verify

```bash
# atelet bindings
gcloud projects get-iam-policy "$PROJECT_ID" \
  --flatten="bindings[].members" \
  --format='table(bindings.role)' \
  --filter="bindings.members:${ATELET_PRINCIPAL}"

# Bucket exists + atelet bindings on the bucket
gcloud storage buckets describe "gs://${BUCKET_NAME}" \
  --project="$PROJECT_ID" --format='value(name,location)'
gcloud storage buckets get-iam-policy "gs://${BUCKET_NAME}" \
  --format='table(bindings.role,bindings.members)' \
  --filter="bindings.members:${ATELET_PRINCIPAL}"
```

## Command-Accuracy Note

The `gcloud` flags above were verified against Google Cloud SDK **484.0.0**. Flags drift between releases — confirm with `gcloud <group> <command> --help` if yours differs.

## Next

- [040 — Install Agent Substrate (Helm OCI)](040-install-substrate-helm.md)
