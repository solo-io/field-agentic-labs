# Provision a GKE Cluster (Terraform)

This lab uses the Terraform under [`assets/gke-terraform/`](assets/gke-terraform/) to bring up a single GKE cluster with autoscaling. It is the cluster the rest of the workshop targets. If you already have a Kubernetes cluster you're happy with, skip to [010](010-licenses-and-secrets.md).

## Lab Objectives

- Provision a GKE cluster with `e2-highcpu-8` nodes, autoscaling 1–3
- Wire `kubectl` to the new cluster
- Confirm nodes are Ready

## Prerequisites

- `gcloud` CLI authenticated against your project (`gcloud auth application-default login`)
- `terraform` ≥ 1.0
- `kubectl`

## What the Terraform Creates

[`assets/gke-terraform/main.tf`](assets/gke-terraform/main.tf):

- `google_container_cluster.primary` — REGULAR release channel, autoscaling 1–100 CPU / 1–1000 Gi memory, default VPC, deletion protection disabled
- `google_container_node_pool.primary_nodes` — `e2-highcpu-8` machines, 1–3 node autoscaling
- `google_service_account.default` — dedicated SA for the node pool with the `cloud-platform` OAuth scope (needed for image pulls from `*.pkg.dev`)
- Outputs: `cluster_name`, `cluster_endpoint`, `cluster_location`

Variables (defaults, override via `terraform.tfvars`):

| Variable | Default | Notes |
|---|---|---|
| `project_id` | *(required)* | GCP project ID |
| `region` | `northamerica-northeast5` | GKE regional cluster |
| `cluster_name` | `ambient-gke-cluster` | Cluster name |
| `node_count` | `3` | Initial node count |
| `machine_type` | `e2-highcpu-8` | Big enough for Istio Ambient + kagent + clickhouse |
| `min_node_count` | `1` | |
| `max_node_count` | `3` | |

## 1. Configure Your Variables

```bash
cd assets/gke-terraform
cp terraform.tfvars.example terraform.tfvars
```

Edit `terraform.tfvars` and set at least `project_id` and `region`. Example:

```hcl
project_id   = "your-gcp-project-id"
region       = "us-east1"
cluster_name = "kagent-ee-demo"
```

> **Security note:** the source repo committed a `terraform.tfvars` with a hardcoded project ID and a `terraform.tfstate` file. **Neither is committed here.** Treat `terraform.tfvars` as private and `terraform.tfstate` as secret — both are listed in [`assets/gke-terraform/.gitignore`](assets/gke-terraform/.gitignore). For shared use, configure a remote state backend (GCS bucket with object versioning).

## 2. Initialize and Plan

```bash
terraform init
terraform plan
```

Review the plan. You should see the cluster, node pool, and SA being created.

## 3. Apply

```bash
terraform apply
```

This takes 6–10 minutes. When it finishes, the outputs include the cluster name, endpoint, and location.

## 4. Wire kubectl to the Cluster

```bash
gcloud container clusters get-credentials \
  $(terraform output -raw cluster_name) \
  --region $(terraform output -raw cluster_location)

kubectl get nodes
```

You should see 1–3 nodes in `Ready`.

## 5. Sanity-Check the Node OAuth Scope

The Terraform deliberately sets `oauth_scopes = ["cloud-platform"]` on the node pool so the nodes can pull from Google Artifact Registry without an `imagePullSecret`. Confirm:

```bash
gcloud container node-pools describe \
  $(terraform output -raw cluster_name)-node-pool \
  --cluster $(terraform output -raw cluster_name) \
  --region $(terraform output -raw cluster_location) \
  --format='value(config.oauthScopes)'
```

You should see `https://www.googleapis.com/auth/cloud-platform`.

If you swap the node pool to one with the default `devstorage.read_only` scope, GAR image pulls will fail — you'll need an `imagePullSecret` instead (see the troubleshooting in [appendix-nemoclaw-oss.md](appendix-nemoclaw-oss.md#imagepullbackoff-403-forbidden) for the workaround).

## Destroy

When you're done with the workshop:

```bash
cd assets/gke-terraform
terraform destroy
```

## Using a Different Cluster

The remaining labs work on **any** Kubernetes cluster (GKE, EKS, AKS, Kind) that has:

- A default `StorageClass` (for kagent's bundled PostgreSQL / ClickHouse)
- A `LoadBalancer`-capable Service controller (or you can replace `service.type: LoadBalancer` with `port-forward` everywhere)
- Outbound internet access to `us-docker.pkg.dev` (Solo OCI charts), `quay.io`, `docker.io`, `get.pinniped.dev`

Just skip steps 1–4 above and make sure `kubectl get nodes` works against your cluster, then continue with [010](010-licenses-and-secrets.md).

## Next

- [010 — Licenses, Namespace, and Secrets](010-licenses-and-secrets.md)
