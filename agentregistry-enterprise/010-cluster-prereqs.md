# Cluster Prerequisites (EKS, EBS CSI, StorageClass)

AgentRegistry Enterprise needs a Kubernetes cluster with:

- Kubernetes 1.29+
- A working default `StorageClass` backed by a CSI driver (the bundled PostgreSQL and ClickHouse both request PVs)
- Internet egress (or a mirror) to pull `oci://us-docker.pkg.dev/solo-public/...` charts and images
- An ingress strategy — `LoadBalancer` Service on managed clouds, or an Istio/Kubernetes Gateway on private clusters (see [035](035-private-cluster-istio-routing.md))

This lab walks through the **private AWS EKS** path because it is the most constrained. If you are using GKE, AKS, or Kind with a default StorageClass already, you can skip to [020](020-setup-entra.md) or [021](021-setup-keycloak.md).

## Lab Objectives

- (Optional) Provision a private EKS cluster with Terraform
- Install the AWS EBS CSI driver via EKS Pod Identity
- Make `gp3` the default StorageClass
- Verify a `PersistentVolumeClaim` binds

## Option A — Provision a Private EKS Cluster with Terraform

The Terraform under [`assets/private-eks/`](assets/private-eks/) provisions a private EKS cluster + VPC + private subnets. It is the cluster used to validate the rest of this workshop's private-cluster labs.

> **Security note:** the source for this content (`demo-private-k8s-cluster-config/private-eks/`) had a committed `terraform.tfstate`. **That file is intentionally not copied into this workshop repo.** Always treat `terraform.tfstate` as sensitive — it can contain ARNs, secret values, and resource IDs you do not want in git. Use a remote state backend (S3 + DynamoDB locking) for any shared use of this Terraform.

```bash
cd assets/private-eks
terraform init
terraform plan -var "cluster_name=are-private" -var "region=us-east-1"
terraform apply -var "cluster_name=are-private" -var "region=us-east-1"

# Configure kubectl
aws eks update-kubeconfig --name are-private --region us-east-1
kubectl get nodes
```

## Option B — Use an Existing Cluster

Confirm:

```bash
kubectl version
kubectl get nodes
kubectl get storageclass
```

You need at least one `StorageClass` with `(default)` in the output. If you have one, jump to [020](020-setup-entra.md) / [021](021-setup-keycloak.md).

## Install the EBS CSI Driver (EKS 1.27+)

The legacy in-tree `gp2` provisioner no longer works on EKS 1.27+. The recommended way to give the EBS CSI controller AWS permissions is **EKS Pod Identity** — no OIDC provider quota issues, no IRSA cert hoops.

```bash
export CLUSTER_NAME=are-private
export REGION=us-east-1

# 1. Install the Pod Identity Agent addon
aws eks create-addon \
  --cluster-name "$CLUSTER_NAME" \
  --addon-name eks-pod-identity-agent \
  --region "$REGION"

# 2. Create the IAM role with the Pod Identity trust policy
cat > /tmp/ebs-csi-trust.json <<'TRUST'
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": { "Service": "pods.eks.amazonaws.com" },
    "Action": [ "sts:AssumeRole", "sts:TagSession" ]
  }]
}
TRUST

aws iam create-role \
  --role-name "${CLUSTER_NAME}-ebs-csi-role" \
  --assume-role-policy-document file:///tmp/ebs-csi-trust.json

aws iam attach-role-policy \
  --role-name "${CLUSTER_NAME}-ebs-csi-role" \
  --policy-arn arn:aws:iam::aws:policy/service-role/AmazonEBSCSIDriverPolicy

EBS_CSI_ROLE_ARN=$(aws iam get-role \
  --role-name "${CLUSTER_NAME}-ebs-csi-role" \
  --query "Role.Arn" --output text)

# 3. Bind the role to the EBS CSI controller's ServiceAccount
aws eks create-pod-identity-association \
  --cluster-name "$CLUSTER_NAME" \
  --namespace kube-system \
  --service-account ebs-csi-controller-sa \
  --role-arn "$EBS_CSI_ROLE_ARN" \
  --region "$REGION"

# 4. Install the EBS CSI driver addon
aws eks create-addon \
  --cluster-name "$CLUSTER_NAME" \
  --addon-name aws-ebs-csi-driver \
  --service-account-role-arn "$EBS_CSI_ROLE_ARN" \
  --region "$REGION"
```

Wait for the addon to become `ACTIVE`:

```bash
aws eks describe-addon \
  --cluster-name "$CLUSTER_NAME" \
  --addon-name aws-ebs-csi-driver \
  --region "$REGION" \
  --query "addon.status" --output text
```

Verify controller pods are Ready (no `CrashLoopBackOff`):

```bash
kubectl get pods -n kube-system -l app.kubernetes.io/name=aws-ebs-csi-driver
```

## Make `gp3` the Default StorageClass

```bash
kubectl apply -f - <<'EOF'
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: gp3
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
provisioner: ebs.csi.aws.com
parameters:
  type: gp3
volumeBindingMode: WaitForFirstConsumer
allowVolumeExpansion: true
EOF

# Remove the default annotation from any legacy gp2 class
kubectl annotate storageclass gp2 storageclass.kubernetes.io/is-default-class- 2>/dev/null || true
```

Verify:

```bash
kubectl get storageclass
```

You should see `gp3 (default)` using the `ebs.csi.aws.com` provisioner.

## Smoke Test — Bind a PVC

```bash
kubectl apply -f - <<'EOF'
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: csi-smoketest
  namespace: default
spec:
  accessModes: [ReadWriteOnce]
  resources: { requests: { storage: 1Gi } }
EOF

kubectl get pvc csi-smoketest -w
# Wait for STATUS=Bound, then:
kubectl delete pvc csi-smoketest
```

If the PVC stays `Pending` for more than a couple of minutes, describe it and check the CSI controller logs:

```bash
kubectl describe pvc csi-smoketest
kubectl -n kube-system logs -l app.kubernetes.io/name=aws-ebs-csi-driver -c ebs-plugin --tail=50
```

## Next

- [020 — Microsoft Entra ID OIDC](020-setup-entra.md), or
- [021 — Keycloak OIDC](021-setup-keycloak.md)
