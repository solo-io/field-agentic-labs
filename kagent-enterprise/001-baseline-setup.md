# Baseline Setup

The first of four mandatory setup labs. Takes you from "I have a Kubernetes cluster" to "I have the local tools needed to install Solo Enterprise for kagent." Subsequent labs assume this baseline is in place.

## Lab Objectives

- Confirm cluster prerequisites (Kubernetes ≥ 1.29, default `StorageClass`, `LoadBalancer`-capable)
- Install the local tools the workshop expects
- Walk through the GKE cluster path that the rest of the workshop is validated against

## What This Lab Does *Not* Do

This lab is on purpose minimal. It does not install kagent, Gloo Operator, Enterprise Agentgateway, or any OIDC provider. Those come in:

- [002 - Licenses + Secrets](002-licenses-and-secrets.md)
- [003 - Install Kagent Enterprise](003-install-kagent-enterprise.md)
- [004 - Install Enterprise Agentgateway](004-install-enterprise-agentgateway.md)

After **001 → 002 → 003 → 004**, you have the baseline that every unit-of-value lab from 010 onwards assumes.

## Prerequisites

- A running Kubernetes cluster (≥ 1.29). The workshop is validated on **GKE Standard**, but other flavors work as long as they meet the cluster checks below.
- `kubectl` configured against the cluster
- `helm` v3
- `openssl`
- (Optional but recommended) `jq`, `envsubst`

## 1. Confirm the Cluster

```bash
kubectl version
kubectl get nodes
kubectl get storageclass
```

You need at least one `StorageClass` with `(default)` in the output - kagent's bundled PostgreSQL and Solo Istio's ClickHouse both request PVs. If none is marked default:

```bash
kubectl annotate storageclass <name> storageclass.kubernetes.io/is-default-class=true
```

Confirm a `LoadBalancer` Service can actually get an external address:

```bash
kubectl create deployment lb-smoke --image=nginx
kubectl expose deployment lb-smoke --port=80 --type=LoadBalancer
kubectl get svc lb-smoke -w
# Wait for EXTERNAL-IP, then:
kubectl delete deployment lb-smoke && kubectl delete svc lb-smoke
```

If `EXTERNAL-IP` stays `<pending>` indefinitely, install / fix MetalLB / kube-vip / `cloud-provider-kind` before continuing. The workshop assumes this works.

## 2. (Optional) Provision GKE

If you don't have a cluster yet, the workshop is validated on a GKE Standard cluster created with the Terraform under [`assets/gke-terraform/`](assets/gke-terraform/):

```bash
cd assets/gke-terraform
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars: set project_id + region + cluster_name
terraform init
terraform apply

gcloud container clusters get-credentials \
 $(terraform output -raw cluster_name) \
 --region $(terraform output -raw cluster_location)

kubectl get nodes
cd ../..
```

The Terraform creates a regional cluster with autoscaling (1-3 `c3-standard-4` nodes) and the `cloud-platform` OAuth scope on the node service account so the nodes can pull from Artifact Registry without an `imagePullSecret`. See [`assets/gke-terraform/`](assets/gke-terraform/) for the full module.

> **Don't commit `terraform.tfvars`** - your `project_id` leaks if you do. It's gitignored alongside `terraform.tfstate`.

## 3. Sanity-Check Local Tools

```bash
for cmd in kubectl helm openssl jq envsubst; do
 printf '%-12s ' "$cmd"
 command -v "$cmd" >/dev/null && echo "OK" || echo "MISSING"
done
```

If `envsubst` is missing on macOS:

```bash
brew install gettext
brew link --force gettext
```

## What's In Place After This Lab

| Resource | State |
|---|---|
| Kubernetes cluster | Up, default `StorageClass`, `LoadBalancer` works |
| Local tools (`kubectl`, `helm`, `openssl`, `jq`, `envsubst`) | Present |

## Cleanup

Roll back this lab only when you're done with the entire workshop. Full teardown is in [099](099-cleanup.md). If you provisioned a GKE cluster in step 2 specifically for this workshop:

```bash
cd assets/gke-terraform
terraform destroy
cd ../..
```

## Next

- [002 - Licenses, Namespace, and Secrets](002-licenses-and-secrets.md)
