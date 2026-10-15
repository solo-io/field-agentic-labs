# Agent Substrate and kagent Enterprise on EKS

Install Agent Substrate Enterprise and Solo Enterprise for kagent on an existing Amazon EKS cluster, with AWS services in place of the in-cluster defaults: Aurora PostgreSQL for state and traces, and S3 for Actor snapshots.

EKS does not serve the Kubernetes `PodCertificateRequest` and `ClusterTrustBundle` APIs, and you cannot turn them on in a managed control plane. Substrate's default mode needs them. kagent Enterprise's **ate-ca compatibility mode** delivers the same mTLS identities without them, on Kubernetes back to v1.34. This lab opts into that mode and creates the material it needs, using only public tools.

Running on EKS needs three things the charts leave to you:

1. **ate-ca compatibility mode**, because `PodCertificateRequest` and `ClusterTrustBundle` are not served.
2. **The right ate-api token issuer.** On EKS it is the cluster's OIDC URL, not `https://kubernetes.default.svc`.
3. **AWS credentials for the snapshot store.** `atelet` and `ate-api-server` both read and write S3, through EKS Pod Identity and IRSA.

The lab ends with four demos you can show on the finished install.

## Lab Objectives

- Add a node group for gVisor workers, and the EKS Pod Identity add-on
- Create an S3 bucket for Actor snapshots, with one IAM role for `atelet` (Pod Identity) and `ate-api-server` (IRSA)
- Create an Aurora PostgreSQL cluster with the roles, logins, and schemas kagent and Substrate expect
- Create Substrate's identity material and the ate-ca compatibility material
- Install `substrate-crds`, `substrate-podcert`, and `substrate`, then kagent Enterprise in compatibility mode with Postgres-only telemetry
- Run an Agent whose golden snapshot lands in S3
- Run the demos: Kubernetes 1.36 support, Aurora-backed state, traces in Postgres, and an agent as a terminal

## Prerequisites

- An existing EKS cluster on Kubernetes v1.34 or above, and its name and region
- `kubectl` access to the cluster, as a cluster admin
- Two private subnets in different Availability Zones, in the cluster's VPC, with outbound internet access through a NAT gateway. The node group and Aurora both use them. The nodes pull images from `ghcr.io` and `us-docker.pkg.dev`, and agents call the model provider.
- EC2 quota for 24 more vCPUs of on-demand standard instances (3 × `m7i.2xlarge`)
- An IAM role for worker nodes (with `AmazonEKSWorkerNodePolicy`, `AmazonEKS_CNI_Policy`, and `AmazonEC2ContainerRegistryReadOnly`)
- AWS CLI v2, with permissions for EKS, EC2, RDS, S3, and IAM
- `eksctl`, only to register the cluster's OIDC provider in IAM
- `kubectl`, `helm` v3, `curl`, `jq`, and `openssl` 1.1.1 or 3.x
- Solo kagent license key
- Enterprise agentgateway license key. The Substrate Enterprise chart runs the enterprise agentgateway and refuses to render without one.
- `OPENAI_API_KEY`
- For the agent-as-a-terminal demo: `ANTHROPIC_API_KEY`, Docker with `buildx`, and a **public** container repository you can push to

## What This Lab Does **Not** Do

- Create the EKS cluster or its VPC
- Use IAM authentication for Aurora. The lab uses password logins to keep setup short. See the production note in [step 5](#5-create-the-aurora-postgresql-cluster).
- Deploy ClickHouse. Traces are stored in Postgres. The dashboard pages that need ClickHouse say so.
- Run the microVM sandbox class. gVisor runs on regular EC2 instances.

## Validated On

| Component | Version |
|---|---|
| EKS | Kubernetes v1.36.4, 3 × `m7i.2xlarge`, Amazon Linux 2023, amd64 |
| Aurora PostgreSQL | 16.15, Serverless v2 |
| kagent Enterprise chart and CRDs | `1.0.0-alpha10` |
| Agent Substrate Enterprise charts (`substrate-crds`, `substrate-podcert`, `substrate`) | `0.5.0-alpha3-e492046` |
| `kubectl-ate` | `v0.5.0-alpha2` |
| Worker image | `us-docker.pkg.dev/solo-public/substrate-enterprise/ateom-gvisor:v0.5.0-alpha3-e492046` |
| Harness runtimes | `ghcr.io/kagent-dev/kagent/golang-adk:1.0.0-alpha13`, `ghcr.io/kagent-dev/kagent/claude-harness:1.0.0-alpha13` |

## 1. Set the Environment Variables

Run every step in the same terminal. Later steps reuse these variables. S3 bucket names are global, so the bucket name includes your account ID and region.

```bash
export CLUSTER=
export REGION=
export NODE_ROLE_ARN=
export SUBNETA=
export SUBNETB=

export ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
export BUCKET=substrate-snapshots-${ACCOUNT_ID}-${REGION}
export ROLE_NAME=substrate-snapshots
```

Point `kubectl` at the cluster:

```bash
aws eks update-kubeconfig --name $CLUSTER --region $REGION
kubectl version
```

## 2. Add the Worker Nodes

gVisor runs on regular EC2 instances: its default platform (systrap) needs no hardware virtualization, and Substrate ships gVisor builds for amd64 and arm64. Only the microVM class needs KVM, which means `.metal` instances or nested virtualization.

```bash
aws eks create-nodegroup --cluster-name $CLUSTER --region $REGION --nodegroup-name substrate \
  --node-role $NODE_ROLE_ARN --subnets $SUBNETA $SUBNETB \
  --instance-types m7i.2xlarge --ami-type AL2023_x86_64_STANDARD --disk-size 100 \
  --scaling-config minSize=3,maxSize=5,desiredSize=3

aws eks wait nodegroup-active --cluster-name $CLUSTER --region $REGION --nodegroup-name substrate
```

Add the EKS Pod Identity agent. `atelet` gets its S3 credentials through it. If the cluster already has the add-on, skip `create-addon`.

```bash
aws eks create-addon --cluster-name $CLUSTER --region $REGION --addon-name eks-pod-identity-agent
aws eks wait addon-active --cluster-name $CLUSTER --region $REGION --addon-name eks-pod-identity-agent
```

No EBS volumes are needed: with RustFS and ClickHouse turned off, neither chart creates a PersistentVolumeClaim.

```bash
kubectl get nodes
```

## 3. Create the S3 Snapshot Bucket

By default the Substrate chart deploys RustFS, an in-cluster S3-compatible store, and writes Actor snapshots to it. This lab turns RustFS off and uses a real S3 bucket. `us-east-1` does not accept a `LocationConstraint`, and every other region requires one.

```bash
if [ "$REGION" = "us-east-1" ]; then
  aws s3api create-bucket --bucket $BUCKET --region $REGION
else
  aws s3api create-bucket --bucket $BUCKET --region $REGION \
    --create-bucket-configuration LocationConstraint=$REGION
fi

aws s3api put-public-access-block --bucket $BUCKET --region $REGION \
  --public-access-block-configuration BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true
```

## 4. Give Substrate Access to the Bucket

Two Substrate components use the snapshot store:

- `atelet` (DaemonSet) uploads and downloads snapshots when Actors suspend and resume.
- `ate-api-server` manages the stored snapshots.

They get credentials differently, because the chart exposes different settings:

| Component | Mechanism | Why |
|---|---|---|
| `atelet` | EKS Pod Identity | The chart has no ServiceAccount annotations value for it. Pod Identity does not set a region, so the region is passed with `atelet.extraEnv`. |
| `ate-api-server` | IRSA | The chart has `ateApi.serviceAccountAnnotations` but no `extraEnv`. The IRSA webhook injects `AWS_REGION`. |

Create a policy scoped to the bucket:

```bash
cat > substrate-snapshots-policy.json <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:ListBucket", "s3:GetBucketLocation"],
      "Resource": "arn:aws:s3:::${BUCKET}"
    },
    {
      "Effect": "Allow",
      "Action": ["s3:GetObject", "s3:PutObject", "s3:DeleteObject", "s3:AbortMultipartUpload"],
      "Resource": "arn:aws:s3:::${BUCKET}/*"
    }
  ]
}
EOF

aws iam create-policy --policy-name $ROLE_NAME --policy-document file://substrate-snapshots-policy.json
```

IRSA needs the cluster's OIDC provider registered in IAM. The command is a no-op if it already is.

```bash
eksctl utils associate-iam-oidc-provider --cluster $CLUSTER --region $REGION --approve

export OIDC_PROVIDER=$(aws eks describe-cluster --name $CLUSTER --region $REGION \
  --query 'cluster.identity.oidc.issuer' --output text | sed 's#^https://##')
```

Create one role both components can assume, Pod Identity for `atelet` and IRSA for `ate-api-server`:

```bash
cat > substrate-snapshots-trust.json <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AteletPodIdentity",
      "Effect": "Allow",
      "Principal": { "Service": "pods.eks.amazonaws.com" },
      "Action": ["sts:AssumeRole", "sts:TagSession"]
    },
    {
      "Sid": "AteApiIRSA",
      "Effect": "Allow",
      "Principal": { "Federated": "arn:aws:iam::${ACCOUNT_ID}:oidc-provider/${OIDC_PROVIDER}" },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "${OIDC_PROVIDER}:sub": "system:serviceaccount:ate-system:ate-api-server",
          "${OIDC_PROVIDER}:aud": "sts.amazonaws.com"
        }
      }
    }
  ]
}
EOF

aws iam create-role --role-name $ROLE_NAME --assume-role-policy-document file://substrate-snapshots-trust.json
aws iam attach-role-policy --role-name $ROLE_NAME \
  --policy-arn arn:aws:iam::${ACCOUNT_ID}:policy/${ROLE_NAME}
```

Associate the role with the `atelet` ServiceAccount **before** installing Substrate. Pod Identity injects credentials when a pod is created, so `atelet` pods that already exist would need a restart.

```bash
aws eks create-pod-identity-association --cluster-name $CLUSTER --region $REGION \
  --namespace ate-system --service-account atelet \
  --role-arn arn:aws:iam::${ACCOUNT_ID}:role/${ROLE_NAME}
```

The `ate-api-server` side is a ServiceAccount annotation in the Substrate values, in [step 12](#12-install-agent-substrate).

## 5. Create the Aurora PostgreSQL Cluster

kagent Enterprise and Substrate share one Aurora cluster and one database. Each uses its own roles and schema:

| Who | Login user | Role assumed on each connection | Schema(s) |
|---|---|---|---|
| kagent controller | `kagent_user` | `kagent_owner` | `kagent`, `enterprise` |
| Substrate ate-api (migrations) | `substrate_owner_user` | `substrate_owner` | `substrate` |
| Substrate ate-api (runtime) | `substrate_readwrite_user` | `substrate_readwrite` | `substrate` |

These are the charts' default role, schema, and Secret names, so the Helm values stay small.

> **For production, use IAM auth through the RDS IAM proxy.** No database passwords are stored in the cluster: an `rds-iam-proxy` sidecar signs a short-lived IAM token for each connection, using EKS Pod Identity or IRSA and `rds-db:connect`. kagent configures it under `enterprise.database.postgres.auth.runtime.aws-iam`. Substrate's ate-api configures it separately, under its own `enterprise.database.postgres.auth`, with both `migration` and `runtime` identities. The kagent-side proxy cannot be combined with agentevals.

Set the database variables. Hex passwords stay safe inside connection-string URLs.

```bash
export AURORA_ID=kagent-aurora
export PG_VERSION=16.15
export PG_MASTER_USER=kagentadmin
export PG_MASTER_PASSWORD=$(openssl rand -hex 24)
export PG_DATABASE=kagent

export KAGENT_DB_PASSWORD=$(openssl rand -hex 24)
export SUBSTRATE_OWNER_PASSWORD=$(openssl rand -hex 24)
export SUBSTRATE_READWRITE_PASSWORD=$(openssl rand -hex 24)
```

If `16.15` is no longer offered in your region, list the current 16.x versions and pick one:

```bash
aws rds describe-db-engine-versions --engine aurora-postgresql --region $REGION \
  --query "DBEngineVersions[?starts_with(EngineVersion, '16.')].EngineVersion" --output text
```

Look up the cluster's VPC and security group:

```bash
export VPC_ID=$(aws eks describe-cluster --name $CLUSTER --region $REGION \
  --query 'cluster.resourcesVpcConfig.vpcId' --output text)
export CLUSTER_SG=$(aws eks describe-cluster --name $CLUSTER --region $REGION \
  --query 'cluster.resourcesVpcConfig.clusterSecurityGroupId' --output text)
echo $VPC_ID $CLUSTER_SG
```

Create a DB subnet group from the two subnets, and a security group that allows TCP 5432 only from the EKS cluster security group:

```bash
aws rds create-db-subnet-group --region $REGION \
  --db-subnet-group-name $AURORA_ID \
  --db-subnet-group-description "kagent Aurora" \
  --subnet-ids $SUBNETA $SUBNETB

export AURORA_SG=$(aws ec2 create-security-group --region $REGION \
  --group-name $AURORA_ID --description "kagent Aurora" --vpc-id $VPC_ID \
  --query GroupId --output text)

aws ec2 authorize-security-group-ingress --region $REGION \
  --group-id $AURORA_SG --protocol tcp --port 5432 --source-group $CLUSTER_SG
```

Create the Aurora Serverless v2 cluster, encrypted, and one writer instance that is not publicly accessible:

```bash
aws rds create-db-cluster --region $REGION \
  --db-cluster-identifier $AURORA_ID \
  --engine aurora-postgresql --engine-version $PG_VERSION \
  --master-username $PG_MASTER_USER --master-user-password $PG_MASTER_PASSWORD \
  --db-subnet-group-name $AURORA_ID --vpc-security-group-ids $AURORA_SG \
  --storage-encrypted \
  --serverless-v2-scaling-configuration MinCapacity=0.5,MaxCapacity=4

aws rds create-db-instance --region $REGION \
  --db-instance-identifier ${AURORA_ID}-1 \
  --db-cluster-identifier $AURORA_ID \
  --engine aurora-postgresql --db-instance-class db.serverless \
  --no-publicly-accessible
```

Wait for the instance. This takes several minutes.

```bash
aws rds wait db-instance-available --region $REGION --db-instance-identifier ${AURORA_ID}-1
```

AWS assigns the writer endpoint when the cluster is created, as `<AURORA_ID>.cluster-<random-id>.<region>.rds.amazonaws.com`:

```bash
export AURORA_ENDPOINT=$(aws rds describe-db-clusters --region $REGION \
  --db-cluster-identifier $AURORA_ID --query 'DBClusters[0].Endpoint' --output text)
echo $AURORA_ENDPOINT
```

## 6. Create the Database, Roles, and Schemas

Neither chart creates database roles, logins, or schemas. Both only read connection-string Secrets, and the objects have to exist before the pods start:

- kagent: the `kagent` and `enterprise` schemas must exist before the controller first starts. Without the `enterprise` schema, the controller exits at startup.
- Substrate: expects the `substrate_owner_user` and `substrate_readwrite_user` logins and the `substrate` schema to exist. Its chart has no step that creates them.

Aurora sits in private subnets, so run `psql` from a pod in the cluster. The pod only connects to Aurora; nothing is stored in it. Any host that can reach port 5432 works too, such as an EC2 bastion in the VPC.

```bash
kubectl run pg-client --image=postgres:18.6-alpine3.23 --restart=Never \
  --env="PGHOST=$AURORA_ENDPOINT" --env="PGUSER=$PG_MASTER_USER" \
  --env="PGPASSWORD=$PG_MASTER_PASSWORD" --env="PGSSLMODE=require" \
  --command -- sleep infinity
kubectl wait pod/pg-client --for=condition=Ready --timeout=120s
```

Create the database:

```bash
kubectl exec pg-client -- psql -d postgres -c "CREATE DATABASE $PG_DATABASE;"
```

Create kagent's roles and the `kagent` and `enterprise` schemas. kagent's own setup script creates `kagent_user` without a password, for certificate logins. With Aurora, create the same objects with a password.

Aurora's master user is not a true superuser, and on PostgreSQL 16 and above, creating a role does not let its creator `SET ROLE` to it. `CREATE SCHEMA ... AUTHORIZATION kagent_owner` needs exactly that, so the master user first grants itself `kagent_owner`. It can, because it created the role.

```bash
kubectl exec -i pg-client -- psql -d $PG_DATABASE --set=ON_ERROR_STOP=1 <<EOF
CREATE ROLE kagent_owner NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION;
CREATE ROLE kagent_user LOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION PASSWORD '${KAGENT_DB_PASSWORD}';
GRANT kagent_owner TO kagent_user;
GRANT kagent_owner TO CURRENT_USER;
CREATE SCHEMA kagent AUTHORIZATION kagent_owner;
CREATE SCHEMA enterprise AUTHORIZATION kagent_owner;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
REVOKE ALL ON SCHEMA kagent FROM PUBLIC;
REVOKE ALL ON SCHEMA enterprise FROM PUBLIC;
EOF
```

Run Substrate's setup. It uses `setup.sql` from the Substrate release kagent Enterprise `1.0.0-alpha10` pins (`kagent-dev/substrate` `v0.5.0-alpha2`). The script creates `substrate_owner` and then a schema owned by it, so it hits the same Aurora restriction. Pre-create the role and grant it to the master user first; the script accepts an existing role whose attributes match.

```bash
kubectl exec -i pg-client -- psql -d $PG_DATABASE --set=ON_ERROR_STOP=1 <<EOF
CREATE ROLE substrate_owner NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION;
GRANT substrate_owner TO CURRENT_USER;
EOF
```

All seven variables are required:

```bash
curl -fsSL https://raw.githubusercontent.com/kagent-dev/substrate/v0.5.0-alpha2/pkg/postgressetup/setup.sql \
  -o substrate-setup.sql

kubectl exec -i pg-client -- psql -d $PG_DATABASE --single-transaction --set=ON_ERROR_STOP=1 \
  --set=substrate_schema=substrate \
  --set=substrate_owner_role=substrate_owner \
  --set=substrate_owner_user=substrate_owner_user \
  --set=substrate_owner_password=$SUBSTRATE_OWNER_PASSWORD \
  --set=substrate_readwrite_role=substrate_readwrite \
  --set=substrate_readwrite_user=substrate_readwrite_user \
  --set=substrate_readwrite_password=$SUBSTRATE_READWRITE_PASSWORD \
  < substrate-setup.sql
```

Confirm the schemas. The list includes `enterprise` and `kagent` owned by `kagent_owner`, and `substrate` owned by `substrate_owner`.

```bash
kubectl exec pg-client -- psql -d $PG_DATABASE -c '\dn'
```

Delete the client pod. The demos start a new one.

```bash
kubectl delete pod pg-client
```

## 7. Create the Connection Secrets

These are the only database objects the charts read. `sslmode=require` encrypts the connection. `verify-full` would also need the RDS CA bundle mounted in the pods.

```bash
kubectl create namespace kagent --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace ate-system --dry-run=client -o yaml | kubectl apply -f -

kubectl -n kagent create secret generic kagent-postgres \
  --from-literal=connectionString="postgresql://kagent_user:${KAGENT_DB_PASSWORD}@${AURORA_ENDPOINT}:5432/${PG_DATABASE}?sslmode=require"

kubectl -n ate-system create secret generic substrate-postgres-owner \
  --from-literal=ownerConnectionString="postgresql://substrate_owner_user:${SUBSTRATE_OWNER_PASSWORD}@${AURORA_ENDPOINT}:5432/${PG_DATABASE}?sslmode=require"

kubectl -n ate-system create secret generic substrate-postgres-readwrite \
  --from-literal=readWriteConnectionString="postgresql://substrate_readwrite_user:${SUBSTRATE_READWRITE_PASSWORD}@${AURORA_ENDPOINT}:5432/${PG_DATABASE}?sslmode=require"
```

The Secret, role, and schema names are the charts' defaults, so the install values in steps 12 and 13 don't repeat them.

## 8. Install `kubectl-ate`

```bash
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
curl -fsSL -o kubectl-ate \
  "https://github.com/kagent-dev/substrate/releases/download/v0.5.0-alpha2/kubectl-ate-${OS}-${ARCH}" &&
  chmod +x kubectl-ate
./kubectl-ate --help
```

## 9. Install the CRDs and Substrate's Identity Material

Substrate is three releases you install yourself: `substrate-crds`, `substrate-podcert`, and `substrate`. Install order matters, because each release depends on the one before it:

```
kagent-enterprise-crds -> substrate-crds -> identity material -> ate-ca material
  -> substrate-podcert -> substrate -> kagent-enterprise
```

```bash
export KAGENT_VERSION=1.0.0-alpha10
export SUBSTRATE_VERSION=0.5.0-alpha3-e492046
export SUBSTRATE_IMAGE_REPO=us-docker.pkg.dev/solo-public/substrate-enterprise
```

Install the CRDs:

```bash
helm upgrade --install kagent-crds \
  oci://us-docker.pkg.dev/solo-public/kagent-enterprise-helm/charts/kagent-enterprise-crds \
  --version $KAGENT_VERSION --namespace kagent --create-namespace

helm upgrade --install substrate-crds \
  oci://us-docker.pkg.dev/solo-public/enterprise-substrate-helm/substrate-crds \
  --version $SUBSTRATE_VERSION --namespace ate-system --create-namespace
```

Create the CA and JWT pools. `kubectl-ate` generates them as Secrets. `postgres-ca-pool` is required even with Aurora, because the certificate controller mounts it at startup.

```bash
kubectl create namespace podcertificate-controller-system

./kubectl-ate admin make-ca-pool --ca-id=1 --name=service-dns-ca-pool  --secret-namespace=podcertificate-controller-system
./kubectl-ate admin make-ca-pool --ca-id=1 --name=postgres-ca-pool     --secret-namespace=podcertificate-controller-system
./kubectl-ate admin make-ca-pool --ca-id=1 --name=pod-identity-ca-pool --secret-namespace=podcertificate-controller-system
./kubectl-ate admin make-jwt-pool --key-id=1 --name=actor-id-jwt-pool   --secret-namespace=ate-system
./kubectl-ate admin make-ca-pool --ca-id=1 --name=actor-id-ca-pool     --secret-namespace=ate-system
./kubectl-ate admin make-ca-pool --ca-id=1 --name=egress-mitm-ca-pool  --secret-namespace=ate-system --key-type=ECDSAP256
```

Publish the actor-id CA root as a PEM Secret. ate-api and the egress gateway verify Actor identity certificates against it. Without it, `atenet-egress` stays in `Init` with `secret "actor-id-ca-certs" not found`. `head -1` must print `-----BEGIN CERTIFICATE-----`.

```bash
kubectl get secret actor-id-ca-pool -n ate-system -o jsonpath='{.data.pool}' \
  | base64 --decode | jq -r '.CAs[0].RootCertificateDER' | base64 --decode \
  | openssl x509 -inform der -outform pem > actor-id-ca.crt

head -1 actor-id-ca.crt

kubectl create secret generic actor-id-ca-certs -n ate-system --from-file=ca.crt=actor-id-ca.crt
```

If `head -1` prints nothing, the pipeline failed. Check that `actor-id-ca-pool` exists and that `jq` is installed before creating the Secret.

Tell ate-api which tokens to trust. On EKS the issuer is the cluster's OIDC URL, not `https://kubernetes.default.svc` (the kind and kubeadm value). EKS publishes the issuer's discovery document publicly, so no CA file or discovery token is needed. `echo $ISSUER` prints `https://oidc.eks.<region>.amazonaws.com/id/...`.

```bash
export ISSUER=$(kubectl get --raw /.well-known/openid-configuration | jq -r .issuer)
echo $ISSUER

cat > authentication.yaml <<EOF
actorIdentityJWTProvider: kubernetes
jwtProviders:
- name: kubernetes
  issuer: ${ISSUER}
  audiences: [api.ate-system.svc]
EOF

kubectl create configmap ate-api-authentication -n ate-system --from-file=authentication.yaml
```

## 10. Create the ate-ca Compatibility Material

In compatibility mode, the ate-ca issuer needs a TLS certificate, and every compatibility-mode workload needs trust bundles. Neither chart creates them. These commands use only `openssl`, `jq`, and `kubectl`.

The issuer's serving certificate is self-signed RSA 2048, valid 10 years, and its own CA, because the agents verify the issuer against this exact certificate. `-addext` needs OpenSSL 1.1.1 or 3.x. If your `openssl` rejects it, use Homebrew's `openssl@3`.

```bash
HOST=ate-ca.podcertificate-controller-system.svc

openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
  -keyout ate-ca-tls.key -out ate-ca-tls.crt \
  -subj "/CN=${HOST}" \
  -addext "subjectAltName=DNS:${HOST},DNS:${HOST}.cluster.local" \
  -addext "basicConstraints=critical,CA:TRUE" \
  -addext "keyUsage=digitalSignature,keyEncipherment,keyCertSign" \
  -addext "extendedKeyUsage=serverAuth"

kubectl create secret tls ate-ca-server-tls -n podcertificate-controller-system \
  --cert=ate-ca-tls.crt --key=ate-ca-tls.key
```

The trust bundles are the root certificates of the pod-identity and service-DNS CA pools from step 9:

```bash
pool_roots() {
  kubectl get secret "$1" -n podcertificate-controller-system -o jsonpath='{.data.pool}' \
    | base64 --decode | jq -r '.CAs[].RootCertificateDER' \
    | while read -r der; do echo "$der" | base64 --decode | openssl x509 -inform der -outform pem; done
}
pool_roots pod-identity-ca-pool > podidentity-trust.pem
pool_roots service-dns-ca-pool  > servicedns-trust.pem
```

Publish all three ConfigMaps in each namespace that runs a compatibility-mode workload. Pods cannot mount a ConfigMap from another namespace, so the kagent controller needs its own copies:

```bash
for ns in podcertificate-controller-system ate-system kagent; do
  kubectl create configmap ate-ca-server-ca         -n $ns --from-file=ca.crt=ate-ca-tls.crt
  kubectl create configmap ate-ca-podidentity-trust -n $ns --from-file=trust-bundle.pem=podidentity-trust.pem
  kubectl create configmap ate-ca-servicedns-trust  -n $ns --from-file=trust-bundle.pem=servicedns-trust.pem
done
```

These ConfigMaps do not update themselves. If a CA is ever rotated, republish them with the old and new roots overlapping, and restart the consumers.

## 11. Install the Certificate Controller

`ateCa.enabled=true` runs the ate-ca issuer:

```bash
helm upgrade --install substrate-podcert \
  oci://us-docker.pkg.dev/solo-public/enterprise-substrate-helm/substrate-podcert \
  --version $SUBSTRATE_VERSION --namespace podcertificate-controller-system \
  --set ateCa.enabled=true --wait --timeout 5m
```

## 12. Install Agent Substrate

Confirm everything the charts mount but do not create is in place. Every line should list the objects. A `NotFound` points to the step that was missed: the pools, `actor-id-ca-certs`, and `ate-api-authentication` come from step 9, `ate-ca-server-tls` and the trust ConfigMaps from step 10, and the connection Secrets from step 7.

```bash
kubectl get secret -n podcertificate-controller-system service-dns-ca-pool postgres-ca-pool pod-identity-ca-pool ate-ca-server-tls
kubectl get secret -n ate-system actor-id-jwt-pool actor-id-ca-pool egress-mitm-ca-pool actor-id-ca-certs

kubectl get secret -n ate-system substrate-postgres-owner substrate-postgres-readwrite
kubectl get secret -n kagent kagent-postgres

kubectl get configmap -n ate-system ate-api-authentication ate-ca-server-ca ate-ca-podidentity-trust ate-ca-servicedns-trust
kubectl get configmap -n podcertificate-controller-system ate-ca-server-ca ate-ca-podidentity-trust ate-ca-servicedns-trust
kubectl get configmap -n kagent ate-ca-server-ca ate-ca-podidentity-trust ate-ca-servicedns-trust
```

The values combine the earlier steps: the license, compatibility mode, password logins to Aurora (the Secrets use the chart's default names), S3 snapshots, and the credential grant that lets the egress gateway read model API keys from the `kagent` namespace.

```bash
export AGENTGATEWAY_LICENSE_KEY=
```

```bash
cat > substrate-values.yaml <<EOF
global:
  licensing:
    licenseKey: "${AGENTGATEWAY_LICENSE_KEY:?Set AGENTGATEWAY_LICENSE_KEY}"

usePodCertificateAPI: false        # ate-ca compatibility mode

postgres:
  clientCertificates:
    enabled: false                 # password logins (Aurora)

rustfs:
  enabled: false                   # snapshots go to S3

atelet:
  storageBackend: s3
  extraEnv:
    - name: AWS_REGION
      value: ${REGION}
    - name: OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE
      value: delta

ateApi:
  serviceAccountAnnotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::${ACCOUNT_ID}:role/${ROLE_NAME}

credentialProvider:
  namespacePolicies:
    - atespace: kagent
      allowedNamespaces: [kagent]

otel:
  endpoint: http://solo-enterprise-telemetry-collector.kagent.svc.cluster.local:4317
  traces:
    enabled: false
EOF

helm upgrade --install substrate \
  oci://us-docker.pkg.dev/solo-public/enterprise-substrate-helm/substrate \
  --version $SUBSTRATE_VERSION --namespace ate-system -f substrate-values.yaml
```

> **Release name and namespace are fixed.** Keep the release named `substrate` in `ate-system`. The chart's resource names, its RBAC, and ate-api's identity checks all assume both.

> **`substrate-values.yaml` holds the license key.** Don't commit it. The [Cleanup](#cleanup) deletes it.

Wait for each component:

```bash
kubectl rollout status deploy/podcertificate-controller -n podcertificate-controller-system --timeout=300s
for d in ate-api-server ate-controller atenet-router atenet-egress k8s-credential-provider; do
  kubectl rollout status deploy/$d -n ate-system --timeout=300s
done
kubectl rollout status ds/atelet -n ate-system --timeout=300s
```

If a pod stays in `Init` or `ContainerCreating`, check the warning events. A missing Secret or ConfigMap is named there:

```bash
kubectl -n ate-system get pods
kubectl -n ate-system get events --field-selector type=Warning --sort-by=.lastTimestamp
```

After you create the missing object, the kubelet retries the mount on its own. No restart is needed.

## 13. Install kagent Enterprise

The values set:

- the license and cluster name
- Substrate in compatibility mode, with the ate-ca agent image from the same Substrate release
- the `kagent-default` WorkerPool on the matching gVisor worker image. Each Agent keeps a golden Actor on a worker, plus room for conversations. 4 workers fit 3 `m7i.2xlarge` nodes.
- Postgres-only telemetry ("observability lite"). Traces land in Aurora, in the `enterprise` schema. ClickHouse and agentevals must be off.
- OpenAI, which creates the `default-model-config` ModelConfig

The Postgres connection uses the chart's defaults (the `kagent-postgres` Secret, the `kagent_owner` role, and the `kagent` and `enterprise` schemas), which match step 6. `enterprise.oidc` is unset, which uses the bundled demo login.

```bash
export KAGENT_LICENSE_KEY=
export OPENAI_API_KEY=
```

```bash
cat > kagent-values.yaml <<EOF
global:
  cluster: ${CLUSTER}
  licensing:
    licenseKey: "${KAGENT_LICENSE_KEY:?Set KAGENT_LICENSE_KEY}"
  telemetry:
    backend: postgres              # observability lite

telemetry:
  enabled: true
clickhouse:
  enabled: false
agentevals:
  enabled: false

enterprise:
  substrate:
    usePodCertificateAPI: false
    ateCAAgentImage: ${SUBSTRATE_IMAGE_REPO}/atecaagent:v${SUBSTRATE_VERSION}
  telemetry:
    postgres:
      maxSize: 250Mi               # cap trace storage in Aurora

controller:
  substrate:
    enabled: true

substrateWorkerPool:
  create: true
  replicas: 4
  workerImage: ${SUBSTRATE_IMAGE_REPO}/ateom-gvisor:v${SUBSTRATE_VERSION}

config:
  telemetry:
    traces:
      endpoint: http://solo-enterprise-telemetry-collector.kagent.svc.cluster.local:4317
  runtimeDefaults:
    telemetry:
      traces:
        endpoint: http://solo-enterprise-telemetry-collector.kagent.svc.cluster.local:4317

providers:
  default: openAI
  openAI:
    apiKey: "${OPENAI_API_KEY:?Set OPENAI_API_KEY}"
EOF

helm upgrade --install kagent \
  oci://us-docker.pkg.dev/solo-public/kagent-enterprise-helm/charts/kagent-enterprise \
  --version $KAGENT_VERSION --namespace kagent -f kagent-values.yaml --wait --timeout 15m
```

> **`kagent-values.yaml` holds the license and OpenAI keys.** Don't commit it.

> **Worker image and Substrate chart move together.** The `ateom-gvisor` and `atecaagent` tags must be the same build as the Substrate charts. A mismatched worker dies on startup and the WorkerPool never registers a worker.

Wait for the WorkerPool, then restart `ate-controller`:

```bash
kubectl wait workerpool/kagent-default -n kagent --for=jsonpath='{.status.readyReplicas}'=4 --timeout=600s
kubectl rollout restart deploy/ate-controller -n ate-system
kubectl rollout status deploy/ate-controller -n ate-system --timeout=120s
```

Why the restart:

- During install, `ate-controller` starts before what it depends on is ready (ate-api, the workers, the certificates), so its early reconciles fail.
- Each failure makes it wait longer before retrying, with no upper limit, and some of its calls have no timeout.
- Even after everything is healthy, the controller can sit in a long backoff, and an Agent's golden snapshot stays at `ResumeGoldenActor` / `ActorTemplatePending`.
- The restart resets the backoff, so it reconciles promptly against the healthy control plane.

## 14. Verify the Install

All pods are running:

```bash
kubectl get pods -n ate-system
kubectl get pods -n kagent
kubectl get pods -n podcertificate-controller-system
```

`atelet` got Pod Identity credentials and a region:

```bash
kubectl -n ate-system get ds atelet -o jsonpath='{.spec.template.spec.containers[0].env[*].name}' | tr ' ' '\n' | grep AWS_REGION
kubectl -n ate-system get pods -o name | grep atelet | head -1 \
  | xargs -I{} kubectl -n ate-system get {} -o jsonpath='{.spec.containers[0].env[*].name}' | tr ' ' '\n' | grep AWS_CONTAINER_CREDENTIALS_FULL_URI
```

Expected:

```
AWS_REGION
AWS_CONTAINER_CREDENTIALS_FULL_URI
```

`ate-api-server` got IRSA:

```bash
kubectl -n ate-system get pod -l app=ate-api-server -o jsonpath='{.items[0].spec.containers[0].env[*].name}' \
  | tr ' ' '\n' | grep -E 'AWS_ROLE_ARN|AWS_WEB_IDENTITY_TOKEN_FILE|AWS_REGION'
```

Expected:

```
AWS_REGION
AWS_ROLE_ARN
AWS_WEB_IDENTITY_TOKEN_FILE
```

## 15. Create Your First Agent

Substrate refuses a Harness image that is not pinned by digest. The Harness below uses the kagent Go ADK runtime kagent Enterprise `1.0.0-alpha10` pairs with, `golang-adk:1.0.0-alpha13`. To resolve the digest yourself:

```bash
docker buildx imagetools inspect ghcr.io/kagent-dev/kagent/golang-adk:1.0.0-alpha13 | grep -m1 Digest
```

An `Agent` names its template under `spec.template` (`ref` or `inline`) and its Harness with `spec.harnessRef`. The Harness writes snapshots to the bucket from step 3.

```bash
kubectl apply -f - <<EOF
apiVersion: api.kagent.dev/v1alpha3
kind: Harness
metadata:
  name: kagent
  namespace: kagent
spec:
  kagent: {}
  workload:
    image: ghcr.io/kagent-dev/kagent/golang-adk@sha256:ae01c3a6d7e0ed2e044253a26abbfb1a9426f3176363842ca9a954762c274d36
  substrate:
    workerPoolRef:
      name: kagent-default
    snapshotPolicy:
      location: s3://${BUCKET}/kagent/
---
apiVersion: api.kagent.dev/v1alpha3
kind: AgentTemplate
metadata:
  name: assistant
  namespace: kagent
spec:
  modelConfig:
    name: default-model-config
  description: A substrate-backed assistant on EKS.
  systemPrompt: You are a helpful assistant running on kagent and Agent Substrate.
---
apiVersion: api.kagent.dev/v1alpha3
kind: Agent
metadata:
  name: assistant
  namespace: kagent
spec:
  template:
    ref:
      name: assistant
  harnessRef:
    name: kagent
EOF
```

`Ready` means Substrate booted the sandbox and stored its golden snapshot:

```bash
kubectl wait agent/assistant -n kagent --for=condition=Ready --timeout=600s
```

The golden snapshot is in S3:

```bash
aws s3 ls s3://$BUCKET/kagent/ --recursive | head
```

Expected, with your own IDs and timestamps:

```
2026-10-15 14:37:15      78569 kagent/atespaces/ate-golden/tags/<tag-id>/checkpoint.img.zstd
2026-10-15 14:37:15        901 kagent/atespaces/ate-golden/tags/<tag-id>/durable-dir-data.tar.zstd
2026-10-15 14:37:15        660 kagent/atespaces/ate-golden/tags/<tag-id>/manifest.json
2026-10-15 14:37:15    1458550 kagent/atespaces/ate-golden/tags/<tag-id>/pages.img.zstd
2026-10-15 14:37:15       2162 kagent/atespaces/ate-golden/tags/<tag-id>/pages_meta.img.zstd
```

Open the UI:

```bash
kubectl port-forward -n kagent svc/kagent-ui 8080:8080
```

The UI is on `http://127.0.0.1:8080` while that port-forward is running.

## 16. Demo: Kubernetes 1.36 Without the PodCertificate API

By default, Substrate needs the Kubernetes PodCertificate APIs. They are on by default from v1.37. Before that, you enable them on the control plane, which EKS and AKS don't let you do. kagent Enterprise supports Kubernetes back to v1.34 by running Substrate in ate-ca compatibility mode.

The cluster is on v1.36:

```bash
kubectl get nodes
```

And it does not serve the APIs Substrate's default mode needs:

```bash
kubectl api-resources | grep -i -E "podcertificate|clustertrustbundle" || echo "(none)"
```

Expected:

```
(none)
```

Every Substrate and kagent pod is running anyway, with mTLS on, because of the compatibility-mode settings from steps 10 through 13.

## 17. Demo: kagent State in Aurora

kagent v1 doesn't bundle PostgreSQL. Its state lives in whatever database you point it at, here Aurora.

There is no database in the cluster:

```bash
kubectl get pods -A | grep -i -E "postgres|pgsql" || echo "no Postgres running in the cluster"
kubectl get statefulsets -A
```

kagent points at Aurora. Show the connection string with the password masked, next to the Aurora cluster. The hostnames match.

```bash
kubectl -n kagent get secret kagent-postgres -o jsonpath='{.data.connectionString}' | base64 -d \
  | sed -E 's#(://[^:]+:)[^@]+@#\1****@#'; echo

aws rds describe-db-clusters --region $REGION --db-cluster-identifier $AURORA_ID \
  --query 'DBClusters[0].{endpoint:Endpoint,engine:Engine,version:EngineVersion,status:Status}' --output table
```

Look inside Aurora:

```bash
kubectl run pg-client --image=postgres:18.6-alpine3.23 --restart=Never \
  --env="PGHOST=$AURORA_ENDPOINT" --env="PGUSER=$PG_MASTER_USER" \
  --env="PGPASSWORD=$PG_MASTER_PASSWORD" --env="PGSSLMODE=require" \
  --command -- sleep infinity
kubectl wait pod/pg-client --for=condition=Ready --timeout=120s

kubectl exec pg-client -- psql -d $PG_DATABASE -c '\dn'
```

`kagent`, `enterprise`, and `substrate` are there. Step 6 created them empty. The tables in them were created by kagent's and Substrate's own migrations when they first started.

Take a "before" snapshot of every table that holds rows:

```bash
kubectl exec pg-client -- psql -d $PG_DATABASE -c "
  SELECT schemaname, relname AS table, n_live_tup AS rows
  FROM pg_stat_user_tables
  WHERE schemaname IN ('kagent','enterprise','substrate') AND n_live_tup > 0
  ORDER BY schemaname, rows DESC;"
```

Similar to:

```
 schemaname |            table            | rows
------------+-----------------------------+------
 enterprise | telemetry_schema_migrations |    3
 kagent     | schema_migrations           |    2
 kagent     | toolserver                  |    1
 kagent     | agent_definition            |    1
 kagent     | agent_revision              |    1
 kagent     | runtime_revision            |    1
 substrate  | schema_migrations           |    4
 substrate  | workers                     |    4
 substrate  | worker_outbox_p202610151830 |    2
 substrate  | atespaces                   |    2
 substrate  | authorization_model         |    1
 substrate  | tags                        |    1
 substrate  | actor_templates             |    1
 substrate  | worker_outbox_trim          |    1
 substrate  | store                       |    1
(15 rows)
```

In the UI, select `assistant` and ask `what can you do?`. Run the query again. The session, the conversation events, the Actor, and the trace are now rows in Aurora:

```
 schemaname |            table            | rows
------------+-----------------------------+------
 enterprise | telemetry_schema_migrations |    3
 enterprise | genai_spans_p20261015       |    2
 enterprise | genai_turns                 |    1
 kagent     | session_task_event          |   59
 kagent     | schema_migrations           |    2
 kagent     | agent_revision              |    1
 kagent     | agent_definition            |    1
 kagent     | a2a_context                 |    1
 kagent     | session                     |    1
 kagent     | runtime_instance            |    1
 kagent     | toolserver                  |    1
 kagent     | runtime_revision            |    1
 kagent     | session_task                |    1
 substrate  | workers                     |    4
 substrate  | schema_migrations           |    4
 substrate  | atespaces                   |    2
 substrate  | worker_outbox_p202610151830 |    2
 substrate  | worker_outbox_p202610151845 |    2
 substrate  | store                       |    1
 substrate  | authorization_model         |    1
 substrate  | worker_outbox_trim          |    1
 substrate  | actor_templates             |    1
 substrate  | actor_egress_policies       |    1
 substrate  | actors                      |    1
 substrate  | tags                        |    1
(25 rows)
```

Optional: in the RDS console, the cluster's **DatabaseConnections** metric shows the kagent and Substrate connections. Performance Insights, if enabled, shows their queries.

Keep `pg-client` running for the next demo.

## 18. Demo: Traces in Postgres, No ClickHouse

By default, kagent stores traces and dashboard data in ClickHouse. kagent v1 can store them in Postgres instead, with no ClickHouse to deploy or operate. This install does that, into Aurora.

The settings are already in `kagent-values.yaml` from step 13:

```yaml
global:
  telemetry:
    backend: postgres        # traces go to Postgres instead of ClickHouse
clickhouse:
  enabled: false             # no ClickHouse deployed
agentevals:
  enabled: false             # required: agentevals results can only go to ClickHouse
telemetry:
  enabled: true              # the collector still runs; it forwards GenAI traces to the controller
enterprise:
  telemetry:
    postgres:
      maxSize: 250Mi         # optional cap on trace storage (retention defaults to 720h)
```

The chart refuses to render `backend: postgres` while ClickHouse or agentevals is on, so the three settings go together.

No extra database is needed. The kagent controller writes traces through its existing connection, the `kagent-postgres` Secret, into the `enterprise` schema. Substrate's database connections are not involved.

There is no ClickHouse:

```bash
kubectl get pods -n kagent | grep -i clickhouse || echo "no ClickHouse in this install"
kubectl get pods -n kagent
```

Count the rows in the `enterprise` schema, chat with `assistant` in the UI, then count again. The span and turn tables grow with each turn.

```bash
kubectl exec pg-client -- psql -d $PG_DATABASE -c "
  SELECT relname AS table, n_live_tup AS rows
  FROM pg_stat_user_tables
  WHERE schemaname = 'enterprise'
  ORDER BY rows DESC;"
```

Open the trace in the UI. Traces, the trace detail tree, the entity and execution graphs, and the dashboard's trace figures all work. ClickHouse-only data is not available, so these pages say so:

- the Substrate runtime figures: the dashboard's fleet tiles, panels, and cost table, and the chat session analytics
- logs, metrics, and Kubernetes object history

That's expected with this backend, not a broken install. Use ClickHouse when you need those.

Delete the client pod:

```bash
kubectl delete pod pg-client
```

## 19. Demo: Agent as a Terminal

Build a Claude Code Harness image with CLIs pre-installed. Claude Code runs with its built-in tools, including the shell, inside the gVisor sandbox, so the agent works as a terminal: "check this repo", "parse this JSON", "run this script".

The tools are baked in because an Actor can only reach the destinations kagent compiles from its AgentTemplate: the model, its MCP servers, and telemetry. Package mirrors, `git clone` from the internet, and `curl` to arbitrary hosts are blocked, and Actors get no Kubernetes token. So the image ships local tools (`bash`, `git`, `jq`, `yq`, `python3`, `ripgrep`, `tree`) and a small git repo at `/opt/demo-workspace` to work on.

The image must be in a **public** registry. `atelet` pulls Actor images itself, without the kubelet's credentials, so a private registry needs extra Substrate configuration. Set `TERMINAL_REPO` to a public repository you can push to, such as `public.ecr.aws/<alias>/agent-terminal` or `ghcr.io/<user>/agent-terminal`.

```bash
export ANTHROPIC_API_KEY=
export TERMINAL_REPO=
```

Create the demo workspace:

```bash
mkdir -p agent-terminal/demo-workspace

cat > agent-terminal/demo-workspace/services.json <<'EOF'
{
  "services": [
    { "name": "checkout", "namespace": "shop", "replicas": 3, "ready": 3, "image": "shop/checkout:2.4.1" },
    { "name": "payments", "namespace": "shop", "replicas": 2, "ready": 1, "image": "shop/payments:1.9.0" },
    { "name": "catalog", "namespace": "shop", "replicas": 4, "ready": 4, "image": "shop/catalog:3.0.2" },
    { "name": "notifier", "namespace": "ops", "replicas": 1, "ready": 0, "image": "ops/notifier:latest" },
    { "name": "gateway", "namespace": "edge", "replicas": 2, "ready": 2, "image": "edge/gateway:1.12.0" }
  ]
}
EOF

cat > agent-terminal/demo-workspace/check_services.py <<'EOF'
"""Report services that are not fully ready, from services.json."""
import json
import sys
from pathlib import Path


def main() -> int:
    data = json.loads(Path(__file__).with_name("services.json").read_text())
    unhealthy = [s for s in data["services"] if s["ready"] < s["replicas"]]
    for s in unhealthy:
        print(f"{s['namespace']}/{s['name']}: {s['ready']}/{s['replicas']} ready ({s['image']})")
    return 1 if unhealthy else 0


if __name__ == "__main__":
    sys.exit(main())
EOF

cat > agent-terminal/demo-workspace/deploy.yaml <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: notifier
  namespace: ops
spec:
  replicas: 1
  selector:
    matchLabels:
      app: notifier
  template:
    metadata:
      labels:
        app: notifier
    spec:
      containers:
        - name: notifier
          image: ops/notifier:latest
EOF
```

Create the Dockerfile. The base is the `claude-harness` kagent Enterprise `1.0.0-alpha10` pairs with: Alpine, running as UID 65532.

```bash
cat > agent-terminal/Dockerfile <<'EOF'
FROM ghcr.io/kagent-dev/kagent/claude-harness:1.0.0-alpha13@sha256:ceaf7caa7366b96c11cb85ad0bad435a35fcd2704c657b895ee3e73978045db8

USER root
# bash, git, and ripgrep already ship in the base image.
RUN apk add --no-cache jq yq python3 tree

# A git identity for commits the agent makes inside the sandbox.
RUN git config --system user.name "agent-terminal" \
    && git config --system user.email "agent-terminal@example.com"

# The demo repo lives in the image, because the sandbox cannot clone from the internet.
COPY demo-workspace/ /opt/demo-workspace/
RUN cd /opt/demo-workspace \
    && git init -q -b main \
    && git -c user.name=platform -c user.email=platform@example.com add . \
    && git -c user.name=platform -c user.email=platform@example.com commit -q -m "Initial demo workspace" \
    && chown -R 65532:65532 /opt/demo-workspace

USER 65532:65532
EOF
```

Build and push. The nodes are `amd64`, so build for that platform:

```bash
docker buildx build --platform linux/amd64 -t $TERMINAL_REPO:v1 --push agent-terminal
```

Pin the image by digest. `echo $TERMINAL_IMAGE` prints `<repo>@sha256:...`.

```bash
export TERMINAL_IMAGE=$TERMINAL_REPO@$(docker buildx imagetools inspect $TERMINAL_REPO:v1 | awk '/^Digest:/{print $2; exit}')
echo $TERMINAL_IMAGE
```

Create the Anthropic key Secret. The egress gateway injects the key on the way out, so the sandbox never holds it. This works because the Secret is in the `kagent` namespace, which the Substrate credential grant from step 12 allows.

```bash
kubectl -n kagent create secret generic anthropic-api-key \
  --from-literal=ANTHROPIC_API_KEY="${ANTHROPIC_API_KEY:?Set ANTHROPIC_API_KEY}"
```

Create the ModelConfig, the Claude Harness on the shared `kagent-default` WorkerPool, the AgentTemplate, and the Agent:

```bash
kubectl apply -f - <<EOF
apiVersion: api.kagent.dev/v1alpha3
kind: ModelConfig
metadata:
  name: claude-model-config
  namespace: kagent
spec:
  provider: Anthropic
  model: claude-haiku-4-5
  apiKeySecret: anthropic-api-key
  apiKeySecretKey: ANTHROPIC_API_KEY
---
apiVersion: api.kagent.dev/v1alpha3
kind: Harness
metadata:
  name: agent-terminal
  namespace: kagent
spec:
  claude: {}
  workload:
    image: ${TERMINAL_IMAGE}
  substrate:
    workerPoolRef:
      name: kagent-default
    snapshotPolicy:
      location: s3://${BUCKET}/kagent/agent-terminal/
---
apiVersion: api.kagent.dev/v1alpha3
kind: AgentTemplate
metadata:
  name: agent-terminal
  namespace: kagent
spec:
  modelConfig:
    name: claude-model-config
  description: A terminal-style agent with a platform-approved toolset, running in a gVisor sandbox.
  systemPrompt: |
    You are a terminal assistant working in a sandbox. Use your shell tool to run
    commands. The project is a git repository at /opt/demo-workspace.
    Available tools: bash, git, jq, yq, python3, ripgrep, tree.
    You have no internet access and cannot install packages. If a task needs
    something that is not installed, say so instead of trying to install it.
---
apiVersion: api.kagent.dev/v1alpha3
kind: Agent
metadata:
  name: agent-terminal
  namespace: kagent
spec:
  template:
    ref:
      name: agent-terminal
  harnessRef:
    name: agent-terminal
EOF

kubectl wait agent/agent-terminal -n kagent --for=condition=Ready --timeout=600s
```

In the UI, select `agent-terminal` and try these in order:

| Prompt | What it shows |
|---|---|
| "Show me what's in /opt/demo-workspace and its git history." | Real shell commands (`tree`, `git log`) inside the sandbox |
| "Which services in services.json are not fully ready? Use jq." | Parsing data with a baked-in CLI |
| "Run check_services.py and explain the output." | Running a script. It flags `payments` (1/2) and `notifier` (0/1). |
| "deploy.yaml uses an unpinned :latest image. Change it to ops/notifier:1.4.0 and commit the change." | Editing a file and committing with `git`, inside the sandbox |
| "Install htop for me." | It can't. The agent runs as a non-root user, so the package manager refuses, and no package mirror is reachable from the sandbox. The platform team decides what's in the sandbox. |

Between prompts, the agent's Actor suspends and resumes from its snapshot. Changes made earlier in the same conversation, like the commit, should still be there in the next turn. Confirm it in rehearsal by asking for `git log` after the commit prompt.

## What's in Place After This Lab

| Component | Where | Role |
|---|---|---|
| `substrate` node group, `eks-pod-identity-agent` add-on | EKS | gVisor worker nodes; Pod Identity for `atelet` |
| S3 bucket `$BUCKET` | AWS | Actor snapshots |
| IAM policy and role `substrate-snapshots`, Pod Identity association | AWS | S3 access for `atelet` (Pod Identity) and `ate-api-server` (IRSA) |
| Aurora cluster `kagent-aurora`, its subnet group and security group | AWS | kagent state and traces (`kagent`, `enterprise`), Substrate state (`substrate`) |
| `kagent-postgres`, `substrate-postgres-owner`, `substrate-postgres-readwrite` | `kagent`, `ate-system` | Aurora connection strings |
| `kagent-crds`, `substrate-crds` Helm releases | `kagent`, `ate-system` | CRDs |
| CA and JWT pools, `actor-id-ca-certs`, `ate-api-authentication` | `ate-system`, `podcertificate-controller-system` | Identity material the charts don't create |
| `ate-ca-server-tls`, ate-ca trust ConfigMaps | `podcertificate-controller-system`, `ate-system`, `kagent` | ate-ca compatibility mode |
| `substrate-podcert` Helm release | `podcertificate-controller-system` | Certificate controller with the ate-ca issuer |
| `substrate` Helm release | `ate-system` | Substrate control plane and `atelet` |
| `kagent` Helm release | `kagent` | kagent Enterprise, gVisor pool `kagent-default`, Postgres-only telemetry |
| `assistant` and `agent-terminal` Agents | `kagent` | Demo agents |
| `kubectl-ate`, values files, certificates, `agent-terminal/` | local working directory | CLI and install inputs |

## Cleanup

Remove the demo agents first. Deleting the releases first leaves those objects behind without a controller.

```bash
kubectl delete agent assistant agent-terminal -n kagent --ignore-not-found
kubectl delete agenttemplate assistant agent-terminal -n kagent --ignore-not-found
kubectl delete harness kagent agent-terminal -n kagent --ignore-not-found
kubectl delete modelconfig claude-model-config -n kagent --ignore-not-found
kubectl delete secret anthropic-api-key -n kagent --ignore-not-found
kubectl delete pod pg-client --ignore-not-found
```

Uninstall the releases in reverse install order, then delete the namespaces. The pool Secrets, connection Secrets, and ConfigMaps go with the namespaces.

```bash
helm uninstall kagent -n kagent
helm uninstall substrate -n ate-system
helm uninstall substrate-podcert -n podcertificate-controller-system
helm uninstall substrate-crds -n ate-system
helm uninstall kagent-crds -n kagent

kubectl delete namespace kagent ate-system podcertificate-controller-system --ignore-not-found
```

Delete Aurora. `--skip-final-snapshot` discards the data for good.

```bash
aws rds delete-db-instance --region $REGION --db-instance-identifier ${AURORA_ID}-1
aws rds wait db-instance-deleted --region $REGION --db-instance-identifier ${AURORA_ID}-1

aws rds delete-db-cluster --region $REGION --db-cluster-identifier $AURORA_ID --skip-final-snapshot
aws rds wait db-cluster-deleted --region $REGION --db-cluster-identifier $AURORA_ID

aws rds delete-db-subnet-group --region $REGION --db-subnet-group-name $AURORA_ID

export AURORA_SG=$(aws ec2 describe-security-groups --region $REGION \
  --filters "Name=group-name,Values=$AURORA_ID" "Name=vpc-id,Values=$VPC_ID" \
  --query 'SecurityGroups[0].GroupId' --output text)
aws ec2 delete-security-group --region $REGION --group-id $AURORA_SG
```

Delete the snapshot bucket and its contents, the Pod Identity association, and the IAM role and policy:

```bash
aws s3 rb s3://$BUCKET --force

export ASSOCIATION_ID=$(aws eks list-pod-identity-associations --cluster-name $CLUSTER --region $REGION \
  --namespace ate-system --service-account atelet --query 'associations[0].associationId' --output text)
aws eks delete-pod-identity-association --cluster-name $CLUSTER --region $REGION --association-id $ASSOCIATION_ID

aws iam detach-role-policy --role-name $ROLE_NAME --policy-arn arn:aws:iam::${ACCOUNT_ID}:policy/${ROLE_NAME}
aws iam delete-role --role-name $ROLE_NAME
aws iam delete-policy --policy-arn arn:aws:iam::${ACCOUNT_ID}:policy/${ROLE_NAME}
```

Delete the node group, if nothing else runs on it. The Pod Identity add-on and the IAM OIDC provider may be shared with other workloads, so this lab leaves them.

```bash
aws eks delete-nodegroup --cluster-name $CLUSTER --region $REGION --nodegroup-name substrate
aws eks wait nodegroup-deleted --cluster-name $CLUSTER --region $REGION --nodegroup-name substrate
```

Remove the local files. Several hold keys or passwords.

```bash
rm -rf kubectl-ate substrate-values.yaml kagent-values.yaml authentication.yaml substrate-setup.sql \
  actor-id-ca.crt ate-ca-tls.crt ate-ca-tls.key podidentity-trust.pem servicedns-trust.pem \
  substrate-snapshots-policy.json substrate-snapshots-trust.json agent-terminal

unset KAGENT_LICENSE_KEY AGENTGATEWAY_LICENSE_KEY OPENAI_API_KEY ANTHROPIC_API_KEY \
  PG_MASTER_PASSWORD KAGENT_DB_PASSWORD SUBSTRATE_OWNER_PASSWORD SUBSTRATE_READWRITE_PASSWORD
```

## Troubleshooting

| Symptom | Fix |
|---|---|
| Pods hang in `ContainerCreating` on projected certificate volumes | Compatibility mode is not set. Check `usePodCertificateAPI: false` in `substrate-values.yaml`, `enterprise.substrate.usePodCertificateAPI: false` in `kagent-values.yaml`, and `ateCa.enabled=true` on `substrate-podcert`. |
| `atenet-egress` stuck at `Init:0/1`, event `secret "actor-id-ca-certs" not found` | Create the Secret from step 9. The kubelet retries the mount on its own. |
| A pod is stuck in `Init` or `ContainerCreating` on another missing Secret or ConfigMap | Its events name the object. Run the pre-flight check in step 12. |
| `CREATE SCHEMA` fails with a `SET ROLE` permission error | Run `GRANT <owner role> TO CURRENT_USER` first (step 6). Aurora's master user is not a superuser, and on PostgreSQL 16 and above creating a role doesn't let its creator `SET ROLE` to it. |
| kagent controller exits at startup | The `enterprise` schema is missing. Create it as in step 6. |
| An Agent stays at `ResumeGoldenActor` / `ActorTemplatePending` | Restart `ate-controller` (step 13) to clear its retry backoff. |
| ate-api rejects tokens | `ate-api-authentication` has the wrong issuer. It must be the EKS OIDC URL from `kubectl get --raw /.well-known/openid-configuration`. |
| `./kubectl-ate get ...` fails with `neither v1 nor v1beta1 ClusterTrustBundle is served` | The public `kubectl-ate` `v0.5.0-alpha2` has no compatibility-mode fallback for these commands. Use `kubectl` to inspect the install. The `admin make-*-pool` commands in step 9 are not affected. |
| `atelet` has no `AWS_CONTAINER_CREDENTIALS_FULL_URI` | The Pod Identity association was created after the `atelet` pods. Run `kubectl rollout restart ds/atelet -n ate-system`. |
| The Actor image fails to pull | The repository is private. `atelet` pulls without the kubelet's credentials, so use a public repository. |
| The Substrate install notes print blank `credentials:` lines | Cosmetic. The chart's notes template is out of date. The install is not affected. |

## Next

- [020 - Access Control](../020-access-control.md) - the three mTLS hops, and where model and MCP credentials are injected. It applies nothing.
