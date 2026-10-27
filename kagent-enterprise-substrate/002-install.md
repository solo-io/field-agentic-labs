# Install Substrate and kagent

The second mandatory setup lab. Agent Substrate is the sandbox runtime kagent runs agents in. Those agents are called Actors.

Substrate multiplexes a large set of idle Actors onto a set of warm Workers. Workers are Pods. An Actor is one sandboxed process: an agent, a coding harness, or an MCP server. A Worker is a long-running pod that hosts many Actors at once, each in its own sandbox: up to 1,000 by default, or fewer if the Actors' declared CPU and memory fill the Worker first. When an Actor is idle, Substrate suspends it to a snapshot and releases its place on the Worker. The next request resumes it, often on a different Worker, from that snapshot rather than a cold boot. Sandbox options are gVisor (software-level isolation) and microVM (hardware-level isolation). This lab installs the gVisor pool.

Kubernetes owns the Pods. Substrate owns Actor scheduling, snapshots, and routing. The `ateapi` server (Substrate's API server) exists because those workloads change too fast for the Kubernetes API.

After this lab, the cluster has the baseline that [010](010-create-an-agent.md) and [020](020-access-control.md) assume.

## Lab Objectives

- Install the kagent Enterprise CRDs and the Substrate CRDs
- Create the six cryptographic pools and the actor-id CA Secret the charts do not create
- Tell `ate-api-server` which JWTs to trust
- Install the pod-certificate controller and the PostgreSQL that kagent and Substrate share
- Install Agent Substrate Enterprise `0.5.0-alpha4-da3c2e5`
- Install kagent Enterprise `1.0.0-alpha11` pointed at that Substrate API
- Open the kagent UI

## Prerequisites

- [001 - Where Substrate Works](001-where-it-works.md). Continue only if your cluster serves `PodCertificateRequest` and `ClusterTrustBundle`.
- `kubectl`, `helm` v3, `curl`, `jq`, `openssl`, `envsubst`
- Solo kagent license key
- Enterprise agentgateway license key. The Substrate Enterprise chart runs the enterprise agentgateway as its router and egress gateway, and refuses to render without a license.
- An LLM provider API key (`OPENAI_API_KEY`, or `ANTHROPIC_API_KEY` if you use the Anthropic block below)
- On GKE, Workload Identity turned on if Actor snapshots go to Cloud Storage

`PodCertificateRequest` is how a Substrate pod gets the certificate it presents for mTLS. The charts do not ship TLS Secrets. Each pod asks Kubernetes to issue a short-lived certificate, and Substrate's pod-certificate controller signs it. The same mechanism issues the PostgreSQL login certificates, so no database password exists anywhere in this install.

> **Clean install only.** A cluster running an earlier kagent 1.0 alpha cannot be upgraded in place: the database schema changed and is not migrated. Back up your `Agent`, `AgentTemplate`, `Harness`, and `ModelConfig` objects, run [099](099-cleanup.md), then start here. The `Agent` field `spec.templateRef` is now `spec.template.ref`.

## What Gets Installed

| Component | Namespace | Installed by |
|---|---|---|
| kagent and Substrate CRDs | cluster-scoped | Step 2 |
| CA and JWT pools, actor-id CA, ate-api authentication | `podcertificate-controller-system`, `ate-system` | Steps 4 to 6 |
| Pod-certificate controller | `podcertificate-controller-system` | Step 7 |
| PostgreSQL (kagent state, Substrate state, checkpoints) | `kagent` | Step 8 |
| Agent Substrate control plane, `atelet`, RustFS snapshot store | `ate-system` | Step 10 |
| kagent Enterprise, UI, ClickHouse (traces), telemetry collector, gVisor worker pool | `kagent` | Step 11 |

## 1. Set the Versions

Run every step in the same terminal. Later steps reuse these variables.

```bash
export KAGENT_VERSION=1.0.0-alpha11
export SUBSTRATE_VERSION=0.5.0-alpha4-da3c2e5
export SUBSTRATE_IMAGE_REPO=us-docker.pkg.dev/solo-public/substrate-enterprise
export SUBSTRATE_API_VERSION=0.5.0-alpha2
export KAGENT_OSS_VERSION=v1.0.0-alpha14
export POSTGRES_IMAGE=docker.io/library/postgres:18.6-alpine3.23
```

| Variable | What it pins |
|---|---|
| `KAGENT_VERSION` | The kagent Enterprise charts |
| `SUBSTRATE_VERSION` | The three Substrate Enterprise charts and their images. The worker image must be the same build. |
| `SUBSTRATE_API_VERSION` | The upstream Substrate release kagent `1.0.0-alpha11` is built against: `kubectl-ate` and Substrate's database setup script |
| `KAGENT_OSS_VERSION` | The upstream kagent release kagent Enterprise `1.0.0-alpha11` is built on: the PostgreSQL manifest and kagent's database setup script |
| `POSTGRES_IMAGE` | The PostgreSQL image kagent's own installer uses |

## 2. Install the CRDs

```bash
helm install kagent-crds \
  oci://us-docker.pkg.dev/solo-public/kagent-enterprise-helm/charts/kagent-enterprise-crds \
  --version $KAGENT_VERSION --namespace kagent --create-namespace

helm install substrate-crds \
  oci://us-docker.pkg.dev/solo-public/enterprise-substrate-helm/substrate-crds \
  --version $SUBSTRATE_VERSION --namespace ate-system --create-namespace
```

`kagent-crds` installs the `api.kagent.dev` CRDs (`Agent`, `AgentTemplate`, `Harness`, ...) that [010](010-create-an-agent.md) uses. `substrate-crds` installs `WorkerPool`, `SandboxConfig`, and the other `ate.dev` CRDs. Install both before Substrate: the Substrate chart creates a `SandboxConfig`, and kagent creates the `WorkerPool`.

## 3. Install `kubectl-ate`

```bash
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
curl -fsSL -o kubectl-ate \
  "https://github.com/kagent-dev/substrate/releases/download/v${SUBSTRATE_API_VERSION}/kubectl-ate-${OS}-${ARCH}" &&
  chmod +x kubectl-ate
./kubectl-ate --help
```

Expected:

```
kubectl ate is a CLI tool to manage Actor and Worker lifecycles in an Agent Substrate.
```

## 4. Create the Cryptographic Pools

`kubectl-ate` generates these as Kubernetes Secrets for Substrate's certificate and identity components. No chart creates them.

| Pool | Purpose |
|---|---|
| `service-dns-ca-pool` | CA material for Substrate's service-DNS certificates and trust bundle. PostgreSQL serves TLS with one of these. |
| `postgres-ca-pool` | CA material for the PostgreSQL login certificates. |
| `pod-identity-ca-pool` | CA material for projected pod identity certificates. |
| `actor-id-jwt-pool` | Signing keys for actor identity JWTs. |
| `actor-id-ca-pool` | CA material for actor identity certificates. |
| `egress-mitm-ca-pool` | CA material for the egress gateway's destination certificates. Actors need its trust bundle to accept those connections. |

```bash
kubectl create namespace podcertificate-controller-system

./kubectl-ate admin make-ca-pool  --ca-id=1  --name=service-dns-ca-pool  --secret-namespace=podcertificate-controller-system
./kubectl-ate admin make-ca-pool  --ca-id=1  --name=postgres-ca-pool     --secret-namespace=podcertificate-controller-system
./kubectl-ate admin make-ca-pool  --ca-id=1  --name=pod-identity-ca-pool --secret-namespace=podcertificate-controller-system
./kubectl-ate admin make-jwt-pool --key-id=1 --name=actor-id-jwt-pool    --secret-namespace=ate-system
./kubectl-ate admin make-ca-pool  --ca-id=1  --name=actor-id-ca-pool     --secret-namespace=ate-system
./kubectl-ate admin make-ca-pool  --ca-id=1  --name=egress-mitm-ca-pool  --secret-namespace=ate-system --key-type=ECDSAP256
```

The pool commands fail if the Secret already exists. To rebuild a pool, delete its Secret and run the command again.

## 5. Publish the Actor-Identity CA

`ate-api-server` and the egress gateway compare Actor identity certificates with the root of the actor-id pool. Give the root to them as a PEM Secret. `head -1` must print `-----BEGIN CERTIFICATE-----`.

```bash
kubectl get secret actor-id-ca-pool -n ate-system -o jsonpath='{.data.pool}' \
  | base64 --decode \
  | jq -r '.CAs[0].RootCertificateDER' \
  | base64 --decode \
  | openssl x509 -inform der -outform pem > actor-id-ca.crt

head -1 actor-id-ca.crt

kubectl create secret generic actor-id-ca-certs -n ate-system --from-file=ca.crt=actor-id-ca.crt
```

## 6. Create the Authentication ConfigMap

This tells `ate-api-server` which JWTs to trust. The command creates a ConfigMap named `ate-api-authentication` in `ate-system`, with an `authentication.yaml` key. The Substrate chart mounts that file into `ate-api-server` and passes it as `--authentication-config`.

| Field | Meaning |
|---|---|
| `actorIdentityJWTProvider: kubernetes` | Selects the named JWT provider that may call Substrate's actor-identity JWT minting operation. |
| `jwtProviders[].name: kubernetes` | Names that provider. It must match the field above. |
| `issuer` | The exact `iss` claim Substrate expects in a bearer token: your cluster's service-account token issuer. It also uses this URL to discover signing keys. |
| `audiences: [api.ate-system.svc]` | Accepts tokens issued for ate-api, rather than accepting any valid service-account token. |
| `certificateAuthorityFile` | In-cluster issuers only. A CA bundle used by `ate-api-server` when it makes HTTPS requests for issuer discovery and signing keys. It is not the actor-id CA from the previous step. |
| `discoveryTokenFile` | In-cluster issuers only. The server pod's mounted service-account token, sent when the Kubernetes discovery or key endpoint requires authentication. It is not a token clients present to ate-api. |

Read your cluster's issuer:

```bash
export ISSUER=$(kubectl get --raw /.well-known/openid-configuration | jq -r .issuer)
echo $ISSUER
```

On GKE, `echo` prints `https://container.googleapis.com/v1/projects/<project>/locations/<location>/clusters/<cluster>`. On EKS it prints `https://oidc.eks.<region>.amazonaws.com/id/...`. Both publish their discovery document and signing keys publicly, so no CA file or discovery token is needed:

```bash
kubectl create configmap ate-api-authentication -n ate-system --from-literal=authentication.yaml="actorIdentityJWTProvider: kubernetes
jwtProviders:
- name: kubernetes
  issuer: ${ISSUER}
  audiences: [api.ate-system.svc]
"
```

On kind, kubeadm, and other clusters where `echo` prints `https://kubernetes.default.svc`, the discovery endpoint is the API server itself, so `ate-api-server` needs the cluster CA and its own token to reach it. Use this instead:

```bash
kubectl create configmap ate-api-authentication -n ate-system --from-literal=authentication.yaml='actorIdentityJWTProvider: kubernetes
jwtProviders:
- name: kubernetes
  issuer: https://kubernetes.default.svc
  audiences: [api.ate-system.svc]
  certificateAuthorityFile: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
  discoveryTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
'
```

> **A wrong issuer does not stop agents.** kagent reaches ate-api with certificates, so agents and chats still work. Token-authenticated calls fail instead, including `kubectl-ate`, with `token issuer "..." not trusted`. Step 11 checks this. To fix it later, recreate the ConfigMap and run `kubectl rollout restart deploy/ate-api-server -n ate-system`.

## 7. Install the Certificate Controller

The pod-certificate controller signs every pod certificate in this install. `postgresClients` maps the kagent controller's service account to the two PostgreSQL logins it uses, so the controller can request a login certificate for each. The chart already maps Substrate's own logins to the `ate-api-server` service account.

```bash
helm install substrate-podcert \
  oci://us-docker.pkg.dev/solo-public/enterprise-substrate-helm/substrate-podcert \
  --version $SUBSTRATE_VERSION --namespace podcertificate-controller-system \
  --set 'postgresClients[0]=kagent/kagent-controller=kagent_owner_user' \
  --set 'postgresClients[1]=kagent/kagent-controller=kagent_readwrite_user' \
  --wait --timeout 5m
```

## 8. Deploy PostgreSQL

kagent and Substrate share one PostgreSQL, each in its own schema. No chart deploys it. This step applies the same manifest kagent's own installer uses, then creates the roles and schemas.

Every login authenticates with a certificate the controller from step 7 signs, so the database has no passwords. The manifest also writes the four connection-string Secrets the charts read: `kagent-postgres-owner` and `kagent-postgres-readwrite` in `kagent`, `substrate-postgres-owner` and `substrate-postgres-readwrite` in `ate-system`.

```bash
curl -fsSL -o postgres.yaml \
  "https://raw.githubusercontent.com/kagent-dev/kagent/${KAGENT_OSS_VERSION}/go/core/cli/internal/commands/bundled_postgres.yaml"

NAMESPACE=kagent SUBSTRATE_NAMESPACE=ate-system \
  envsubst '${NAMESPACE} ${SUBSTRATE_NAMESPACE} ${POSTGRES_IMAGE}' < postgres.yaml \
  | kubectl apply --server-side -f -

kubectl rollout status deploy/kagent-postgresql -n kagent --timeout=5m
```

> **Keep the variable list on `envsubst`.** The manifest contains a shell script with its own `$` variables. Without the list, `envsubst` blanks them and PostgreSQL never reloads its rotated certificates.

Create kagent's roles and the `kagent` schema. Every password is empty, because every login uses a certificate:

```bash
curl -fsSL -o kagent-setup.sql \
  "https://raw.githubusercontent.com/kagent-dev/kagent/${KAGENT_OSS_VERSION}/go/core/pkg/postgressetup/setup.sql"

kubectl exec -i -n kagent deploy/kagent-postgresql -c postgresql -- \
  psql --no-psqlrc --set=ON_ERROR_STOP=1 --single-transaction -U postgres -d kagent \
  --set=kagent_schema=kagent \
  --set=kagent_owner_role=kagent_owner \
  --set=kagent_owner_user=kagent_owner_user \
  --set=kagent_owner_password= \
  --set=kagent_readwrite_role=kagent_readwrite \
  --set=kagent_readwrite_user=kagent_readwrite_user \
  --set=kagent_readwrite_password= \
  < kagent-setup.sql
```

Create Substrate's roles and the `substrate` schema:

```bash
curl -fsSL -o substrate-setup.sql \
  "https://raw.githubusercontent.com/kagent-dev/substrate/v${SUBSTRATE_API_VERSION}/pkg/postgressetup/setup.sql"

kubectl exec -i -n kagent deploy/kagent-postgresql -c postgresql -- \
  psql --no-psqlrc --set=ON_ERROR_STOP=1 --single-transaction -U postgres -d kagent \
  --set=substrate_schema=substrate \
  --set=substrate_owner_role=substrate_owner \
  --set=substrate_owner_user=substrate_owner_user \
  --set=substrate_owner_password= \
  --set=substrate_readwrite_role=substrate_readwrite \
  --set=substrate_readwrite_user=substrate_readwrite_user \
  --set=substrate_readwrite_password= \
  < substrate-setup.sql
```

Create the `enterprise` schema kagent Enterprise keeps its own tables in. The controller applies the migrations as `kagent_owner` but cannot create the schema itself; without it, the controller exits at startup with `create schema enterprise: permission denied for database`. The grants let `kagent_readwrite` read and write every table `kagent_owner` creates there.

```bash
kubectl exec -i -n kagent deploy/kagent-postgresql -c postgresql -- \
  psql --no-psqlrc --set=ON_ERROR_STOP=1 --single-transaction -U postgres -d kagent <<'EOF'
CREATE SCHEMA enterprise AUTHORIZATION kagent_owner;
REVOKE ALL ON SCHEMA enterprise FROM PUBLIC;
GRANT USAGE ON SCHEMA enterprise TO kagent_readwrite;
ALTER DEFAULT PRIVILEGES FOR ROLE kagent_owner IN SCHEMA enterprise GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO kagent_readwrite;
ALTER DEFAULT PRIVILEGES FOR ROLE kagent_owner IN SCHEMA enterprise GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO kagent_readwrite;
ALTER DEFAULT PRIVILEGES FOR ROLE kagent_owner IN SCHEMA enterprise GRANT EXECUTE ON ROUTINES TO kagent_readwrite;
ALTER DEFAULT PRIVILEGES FOR ROLE kagent_owner IN SCHEMA enterprise GRANT USAGE ON TYPES TO kagent_readwrite;
EOF
```

Confirm the schemas. The list includes `enterprise` and `kagent` owned by `kagent_owner`, and `substrate` owned by `substrate_owner`.

```bash
kubectl exec -n kagent deploy/kagent-postgresql -c postgresql -- psql -U postgres -d kagent -c '\dn'
```

## 9. Create the License and Provider Secrets

Both charts can create these Secrets from Helm values, but then the keys are stored in the Helm release and in any values file you keep. Create them directly instead. The values in steps 10 and 11 reference them by name.

```bash
export KAGENT_LICENSE_KEY=
export AGENTGATEWAY_LICENSE_KEY=
export OPENAI_API_KEY=
```

```bash
kubectl create secret generic enterprise-agentgateway-license -n ate-system \
  --from-literal=enterprise-agentgateway-license-key="${AGENTGATEWAY_LICENSE_KEY:?Set AGENTGATEWAY_LICENSE_KEY}"

kubectl create secret generic enterprise-kagent-license -n kagent \
  --from-literal=enterprise-kagent-license-key="${KAGENT_LICENSE_KEY:?Set KAGENT_LICENSE_KEY}"

kubectl create secret generic kagent-openai -n kagent \
  --from-literal=OPENAI_API_KEY="${OPENAI_API_KEY:?Set OPENAI_API_KEY}"
```

> **Anthropic instead of OpenAI.** Export `ANTHROPIC_API_KEY` and create `kagent-anthropic` instead of `kagent-openai`:
>
> ```bash
> kubectl create secret generic kagent-anthropic -n kagent \
>   --from-literal=ANTHROPIC_API_KEY="${ANTHROPIC_API_KEY:?Set ANTHROPIC_API_KEY}"
> ```

## 10. Install Agent Substrate

```bash
cat > substrate-values.yaml <<'EOF'
global:
  licensing:
    createSecret: false
    secretName: enterprise-agentgateway-license

usePodCertificateAPI: true

postgres:
  clientCertificates:
    enabled: true                  # certificate logins to the PostgreSQL from step 8

credentialProvider:
  namespacePolicies:
    - atespace: kagent
      allowedNamespaces: [kagent]

otel:
  endpoint: http://solo-enterprise-telemetry-collector.kagent.svc.cluster.local:4317
  traces:
    enabled: false
atelet:
  extraEnv:
    - name: OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE
      value: delta
EOF

helm install substrate oci://us-docker.pkg.dev/solo-public/enterprise-substrate-helm/substrate \
  --version $SUBSTRATE_VERSION --namespace ate-system -f substrate-values.yaml
```

`credentialProvider` lets the egress gateway read model and MCP credentials from Secrets in the `kagent` namespace. An empty grant denies every fetch, and agents' model calls go out without a key.

> **Release name and namespace are fixed.** Keep the release named `substrate` in `ate-system`. The chart's resource names and its RBAC assume both, and under another release name the golden actor fails to resume with a SPIFFE identity mismatch.

> **GKE and private Artifact Registry images.** `atelet` pulls Actor images itself, so pod `imagePullSecrets` do not apply. To let it pull private images with the node's credentials, add this to `atelet:` in `substrate-values.yaml`:
>
> ```yaml
>   imageCredentialProviderBinDir: /home/kubernetes/bin
>   imageCredentialProviderConfig: /etc/srv/kubernetes/cri_auth_config.yaml
> ```

Wait for each component:

```bash
kubectl rollout status deploy/podcertificate-controller -n podcertificate-controller-system --timeout=300s
for d in ate-api-server ate-controller atenet-router atenet-egress k8s-credential-provider rustfs; do
  kubectl rollout status deploy/$d -n ate-system --timeout=600s
done
kubectl rollout status ds/atelet -n ate-system --timeout=600s
```

Pods wait in `ContainerCreating` until their certificates are signed, and the kubelet retries failed certificate mounts on a backoff of about two minutes, so the first rollout can take several minutes. If a pod stays there, its warning events name the missing Secret or ConfigMap:

```bash
kubectl -n ate-system get events --field-selector type=Warning --sort-by=.lastTimestamp
```

## 11. Install kagent Enterprise

The values set:

- the cluster name, and the license Secret from step 9
- certificate logins to the PostgreSQL from step 8. The chart's default Secret names match what step 8 created.
- the controller's connection to Substrate
- the `kagent-default` WorkerPool on the matching gVisor worker image. One Worker can host every Actor in this workshop; 3 replicas spread them across nodes.
- the telemetry collector, and trace export for both the controller and the agent runtimes
- OpenAI, which creates the `default-model-config` ModelConfig from the `kagent-openai` Secret

`enterprise.oidc` is unset, which uses the bundled demo login.

```bash
cat > kagent-values.yaml <<EOF
global:
  cluster: kagent-demo
  licensing:
    createSecret: false
    secretName: enterprise-kagent-license

database:
  postgres:
    clientCertificate:
      enabled: true

controller:
  substrate:
    enabled: true

substrateWorkerPool:
  create: true
  name: kagent-default
  replicas: 3
  sandboxClass: gvisor
  workerImage: ${SUBSTRATE_IMAGE_REPO}/ateom-gvisor:v${SUBSTRATE_VERSION}

telemetry:
  enabled: true

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
    apiKeySecretRef: kagent-openai
    apiKeySecretKey: OPENAI_API_KEY
EOF

helm install kagent \
  oci://us-docker.pkg.dev/solo-public/kagent-enterprise-helm/charts/kagent-enterprise \
  --version $KAGENT_VERSION --namespace kagent -f kagent-values.yaml --wait --timeout 15m
```

> **Anthropic provider block.** Replace the `providers:` block above with:

```yaml
providers:
  default: anthropic
  anthropic:
    apiKeySecretRef: kagent-anthropic
    apiKeySecretKey: ANTHROPIC_API_KEY
```

Wait for the workers, then list them through ate-api. This also confirms the token issuer from step 6:

```bash
kubectl wait workerpool/kagent-default -n kagent --for=jsonpath='{.status.readyReplicas}'=3 --timeout=600s
./kubectl-ate get workers
```

Expected: three `WORKER_STATE_ACTIVE` workers in pool `kagent-default`, each with capacity for `1000` actors. The `ACTORS` column counts the Actors a Worker hosts; `CPU` and `MEMORY` show its budget, which the Actors' declared limits draw down. If the command fails with `token issuer "..." not trusted`, the issuer in step 6 does not match your cluster.

> **Keep the `http://` prefix on the OTLP endpoints.** Without it, the exporter attempts TLS against the collector's plaintext port and traces do not arrive.

> **`runtimeDefaults` is separate from the controller's own telemetry.** Agent runtimes only export traces when `config.runtimeDefaults.telemetry.traces.endpoint` is set. The controller's `config.telemetry` does not propagate to them.

> **Prompt and response capture is off.** To store prompts and responses in telemetry, add `captureContent: true` under `config.runtimeDefaults.telemetry` (and `config.telemetry` for the controller) only after a privacy review. Go ADK agents (`kagent: {}` Harnesses) also need `OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT=SPAN_ONLY` in the Harness `spec.env`: the Go ADK puts content on its model spans only in that mode, and kagent does not set it. The [trace-enabled demo](assets/agentdemo/adktraceenabled/README.md) does this.

> **Worker image and Substrate chart move together.** The `ateom-gvisor` tag must be the same build as the Substrate charts. A mismatched worker dies on startup and the WorkerPool never registers a worker.

> **Values that no longer apply.** Carried over from an alpha6 install, these are ignored or rejected: `substrate.enabled` on the CRD chart (the `WorkerPool` CRD now comes from `substrate-crds`), `controller.substrate.ateApiEndpoint` and `atenetRouterURL` (now `config.substrate.apiEndpoint` and `config.substrate.routerUrl`, whose defaults match a release named `substrate` in `ate-system`), and the `otel.*` block (now `config.telemetry` and `config.runtimeDefaults.telemetry`).

> **agentevals is experimental** in this release and stays off. This workshop does not cover it.

> **The values above use the built-in demo IdP.** To use your own, create the client-secret and set `enterprise.oidc`:

```bash
kubectl create secret generic kagent-enterprise-oidc-secret \
 --namespace kagent \
 --from-literal=clientSecret="$OIDC_CLIENT_SECRET"
```

```yaml
enterprise:
  oidc:
    issuer: "https://idp.example.com/realms/kagent"
    clientId: "kagent-enterprise"
    secretRef: "kagent-enterprise-oidc-secret"
    secretKey: "clientSecret"
```

## 12. Open the UI

```bash
kubectl port-forward -n kagent svc/kagent-ui 8080:8080
```

The UI is on `http://127.0.0.1:8080` while that port-forward is running.

## What's in Place After This Lab

| Component | Namespace | Role |
|---|---|---|
| `kagent-crds` Helm release | `kagent` | `api.kagent.dev` and enterprise CRDs |
| `substrate-crds` Helm release | `ate-system` | `WorkerPool`, `SandboxConfig`, and the other Substrate CRDs |
| CA and JWT pools | `ate-system`, `podcertificate-controller-system` | Identity material the charts do not create |
| `actor-id-ca-certs`, `ate-api-authentication` | `ate-system` | Actor-id trust root and JWT authentication config |
| `substrate-podcert` Helm release | `podcertificate-controller-system` | Signs pod and PostgreSQL login certificates |
| `kagent-postgresql` | `kagent` | Shared database, with the `kagent`, `enterprise`, and `substrate` schemas |
| License and provider Secrets | `kagent`, `ate-system` | Keys the charts reference by name |
| `substrate` Helm release | `ate-system` | Substrate control plane, `atelet`, RustFS snapshot store |
| `kagent` Helm release | `kagent` | kagent Enterprise, UI, ClickHouse, telemetry collector, gVisor pool `kagent-default` |
| `kubectl-ate` and the files from steps 5 and 8 | local working directory | Actor and Worker CLI, downloaded manifest and setup scripts |

This is the baseline [010](010-create-an-agent.md) assumes. Don't tear it down between labs.

## Cleanup

Don't clean this up until you're done with the workshop. The full teardown is [099](099-cleanup.md).

If the install failed partway, run [099](099-cleanup.md) and start again at step 1. The pools, the database, and the certificate trust bundles all have to be recreated together; reusing some of them with new copies of the others leaves pods unable to verify each other.

## Troubleshooting

| Symptom | Fix |
|---|---|
| Substrate pods stay in `ContainerCreating` | A pool or Secret from steps 4 to 9 is missing. `kubectl get events -n ate-system --field-selector type=Warning` names it. |
| `kagent-postgresql` stays in `ContainerCreating` | `substrate-podcert` is not running, or `postgres-ca-pool` is missing. |
| kagent controller exits with `create schema enterprise: permission denied` | The `enterprise` schema from step 8 was not created. |
| `kubectl-ate` fails with `token issuer "..." not trusted` | The issuer in `ate-api-authentication` does not match your cluster. Redo step 6 with the issuer `kubectl get --raw /.well-known/openid-configuration` reports, then restart `ate-api-server`. |
| Agent stays at `ActorTemplatePending` after the control plane is healthy | `ate-controller` backed off during the install. Restart it: `kubectl rollout restart deploy/ate-controller -n ate-system`. |
| Agent turns fail with a generic runtime error | The `credentialProvider` grant in step 10 is missing, so the egress gateway cannot read the API key Secret and model calls go out without a key. |
| Agent traces never show up | `config.runtimeDefaults.telemetry.traces.endpoint` is unset, or an endpoint is missing the `http://` prefix. |

## Next

- [010 - Create an Agent](010-create-an-agent.md)
- [020 - Access Control](020-access-control.md) - read this any time after the baseline is up. It applies nothing.
