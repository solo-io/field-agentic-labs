# Install Substrate and kagent

The second mandatory setup lab. Agent Substrate is the sandbox runtime kagent runs agents in. Those agents are called Actors.

Substrate multiplexes a large set of idle Actors onto a set of warm Workers. Workers are Pods. An Actor is one sandboxed process: an agent, a coding harness, or an MCP server. A Worker is a long-running pod that hosts at most one RUNNING Actor at a time. When the Actor is idle, Substrate suspends it and frees the Worker. The next request resumes it, often on a different Worker, from a snapshot rather than a cold boot. Sandbox options are gVisor (software-level isolation) and microVM (hardware-level isolation). This lab installs the gVisor pool.

Kubernetes owns the Pods. Substrate owns Actor scheduling, snapshots, and routing. The `ateapi` server (Substrate's API server) exists because those workloads change too fast for the Kubernetes API.

After this lab, the cluster has the baseline that [010](010-create-an-agent.md) and [020](020-access-control.md) assume.

## Lab Objectives

- Install the kagent Enterprise CRDs with the `WorkerPool` CRD enabled
- Install Agent Substrate `0.2.0-beta5`
- Create the five cryptographic pools and the actor-id CA Secret the chart does not create
- Tell `ate-api-server` which JWTs to trust
- Install kagent Enterprise `1.0.0-alpha3` pointed at that Substrate API
- Open the kagent UI

## Prerequisites

- [001 - Where Substrate Works](001-where-it-works.md). Continue only if your cluster can serve `PodCertificateRequest`, or it is already on Kubernetes v1.37 or above.
- `kubectl`, `helm` v3, `curl`, `jq`, `openssl`
- Solo kagent license key
- An LLM provider API key (`OPENAI_API_KEY`, or `ANTHROPIC_API_KEY` if you use the Anthropic block below)
- On GKE, Workload Identity turned on so Actor snapshots can be written to Cloud Storage

The cluster also needs `ClusterTrustBundle` and the corresponding projected-volume support on the nodes.

> **Kubernetes v1.37 and above.** The `PodCertificateRequest` gate is no longer required.

## What Gets Installed

- Postgres (kagent state, checkpoints, eval definitions)
- ClickHouse (OTel traces, Substrate requests, harness chat spans, agenteval results)
- kagent and Agent Substrate CRDs
- kagent and Agent Substrate

## 1. Install the kagent CRDs

```bash
helm install kagent-crds \
  oci://us-docker.pkg.dev/solo-public/kagent-enterprise-helm/charts/kagent-enterprise-crds \
  --version 1.0.0-alpha3 --namespace kagent --create-namespace \
  --set substrate.enabled=true
```

Without `substrate.enabled=true`, the `WorkerPool` CRD will not exist, so no Workers will be available to run Actors.

## 2. Install Agent Substrate

```bash
cat > substrate-values.yaml <<'EOF'
credentialProvider:
  namespacePolicies:
    - atespace: kagent
      allowedNamespaces: [kagent]
EOF

helm install substrate oci://ghcr.io/kagent-dev/substrate/helm/substrate \
  --version 0.2.0-beta5 --namespace ate-system --create-namespace \
  --wait=false -f substrate-values.yaml
```

`--wait=false` is intentional. The chart does not create the cryptographic Secrets below, so its pods cannot become Ready until those pools exist.

## 3. Install `kubectl-ate`

```bash
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
curl -fsSL -o kubectl-ate \
  "https://github.com/kagent-dev/substrate/releases/download/v0.2.0-beta5/kubectl-ate-${OS}-${ARCH}" &&
  chmod +x kubectl-ate
```

Run it and confirm the binary starts:

```bash
./kubectl-ate
```

Expected:

```
kubectl ate is a CLI tool to manage Actor and Worker lifecycles in an Agent Substrate.

Usage:
  kubectl-ate [command]

Available Commands:
  admin       Administration and debugging commands
  completion  Generate the autocompletion script for the specified shell
  create      Create a resource
  delete      Delete a resource
  get         Display one or many resources
  help        Help about any command
  logs        Print the logs for a resource
  pause       Pause a resource
  resume      Resume a resource
  suspend     Suspend a resource
  top         Display resource (CPU/Memory) usage
  update      Update a resource
```

## 4. Create the Cryptographic Pools

`kubectl-ate` generates these as Kubernetes Secrets for Substrate's certificate and identity components. The Substrate chart does not create them.

| Pool | Purpose |
|---|---|
| `service-dns-ca-pool` | CA material for Substrate's service-DNS certificates and trust bundle. |
| `pod-identity-ca-pool` | CA material for projected pod identity certificates. |
| `actor-id-jwt-pool` | Signing keys for actor identity JWTs. |
| `actor-id-ca-pool` | CA material for actor identity certificates. |
| `egress-mitm-ca-pool` | CA material for the egress gateway's destination certificates. Actors need its trust bundle to accept those connections. |

```bash
./kubectl-ate admin make-ca-pool  --ca-id=1  --name=service-dns-ca-pool  --secret-namespace=podcertificate-controller-system
./kubectl-ate admin make-ca-pool  --ca-id=1  --name=pod-identity-ca-pool --secret-namespace=podcertificate-controller-system
./kubectl-ate admin make-jwt-pool --key-id=1 --name=actor-id-jwt-pool    --secret-namespace=ate-system
./kubectl-ate admin make-ca-pool  --ca-id=1  --name=actor-id-ca-pool     --secret-namespace=ate-system
./kubectl-ate admin make-ca-pool  --ca-id=1  --name=egress-mitm-ca-pool  --secret-namespace=ate-system
```

## 5. Publish the Actor-Identity CA

`ate-api-server` compares Actor identity certificates with the root of the actor-id pool. Give the root to the server as a PEM Secret.

```bash
kubectl get secret actor-id-ca-pool -n ate-system -o jsonpath='{.data.pool}' \
  | base64 --decode \
  | jq -r '.CAs[0].RootCertificateDER' \
  | base64 --decode \
  | openssl x509 -inform der -outform pem > actor-id-ca.crt

kubectl create secret generic actor-id-ca-certs -n ate-system --from-file=ca.crt=actor-id-ca.crt
```

## 6. Create the Authentication ConfigMap

This tells `ate-api-server` which JWTs to trust. The command creates a ConfigMap named `ate-api-authentication` in `ate-system`, with an `authentication.yaml` key. The Substrate chart mounts that file into `ate-api-server` and passes it as `--authentication-config`.

| Field | Meaning |
|---|---|
| `actorIdentityJWTProvider: kubernetes` | Selects the named JWT provider that may call Substrate's actor-identity JWT minting operation. |
| `jwtProviders[].name: kubernetes` | Names that provider. It must match the field above. |
| `issuer` | The exact `iss` claim Substrate expects in a bearer token. It also uses this URL to discover signing keys. |
| `audiences: [api.ate-system.svc]` | Accepts tokens issued for ate-api, rather than accepting any valid service-account token. |
| `certificateAuthorityFile` | A CA bundle used by `ate-api-server` when it makes HTTPS requests for issuer discovery and signing keys. It is not the actor-id CA from the previous step. |
| `discoveryTokenFile` | The server pod's mounted service-account token, sent when the Kubernetes discovery or key endpoint requires authentication. It is not a token clients present to ate-api. |

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

## 7. Confirm Substrate Is Running

```bash
kubectl rollout status deploy/podcertificate-controller -n podcertificate-controller-system --timeout=300s
for d in ate-api-server ate-controller atenet-router atenet-egress k8s-credential-provider; do
  kubectl rollout status deploy/$d -n ate-system --timeout=300s
done
kubectl rollout status ds/atelet -n ate-system --timeout=300s
```

Each command should report success before you continue. A fresh install that skipped steps 4 through 6 stays unready here.

## 8. Install kagent Enterprise

```bash
export ENTERPRISE_LICENSE_KEY=
export OPENAI_API_KEY=
```

> **Anthropic instead of OpenAI.** Export `ANTHROPIC_API_KEY` and use the replacement `providers:` block under the install.

```bash
helm install kagent \
  oci://us-docker.pkg.dev/solo-public/kagent-enterprise-helm/charts/kagent-enterprise \
  --version 1.0.0-alpha3 --namespace kagent --wait --timeout 15m -f - <<EOF
global:
  cluster: kagent-demo
  licensing:
    createSecret: true
    licenseKey: "${ENTERPRISE_LICENSE_KEY:?Set ENTERPRISE_LICENSE_KEY}"

telemetry:
  enabled: true
  traces:
    enabled: true

otel:
  tracing:
    enabled: true
    exporter:
      otlp:
        endpoint: http://solo-enterprise-telemetry-collector.kagent.svc.cluster.local:4317
        insecure: true

controller:
  substrate:
    enabled: true
    ateApiEndpoint: "dns:///api.ate-system.svc:443"
    atenetRouterURL: "http://atenet-router.ate-system.svc:80"
    defaultWorkerPool:
      name: kagent-default

substrate:
  enabled: false

substrateWorkerPool:
  create: true
  name: kagent-default
  workerImage: ghcr.io/kagent-dev/substrate/ateom-gvisor:v0.2.0-beta5
  sandboxClass: gvisor

providers:
  openAI:
    apiKey: "${OPENAI_API_KEY:?Set OPENAI_API_KEY}"
EOF
```

> **Anthropic provider block.** Set `ANTHROPIC_API_KEY` and replace the `providers:` block above with:

```yaml
providers:
  default: anthropic
  anthropic:
    apiKey: "${ANTHROPIC_API_KEY:?Set ANTHROPIC_API_KEY}"
```

> **Keep the `http://` prefix on the OTLP endpoint.** Without it, the exporter attempts TLS against the collector's plaintext port and agent traces do not arrive.

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

## 9. Open the UI

```bash
kubectl port-forward -n kagent svc/kagent-ui 8080:8080
```

The UI is on `http://127.0.0.1:8080` while that port-forward is running.

## What's in Place After This Lab

| Component | Namespace | Role |
|---|---|---|
| `kagent-crds` Helm release | `kagent` | CRDs, including `WorkerPool` |
| `substrate` Helm release | `ate-system` | Substrate control plane and `atelet` |
| CA and JWT pools | `ate-system`, `podcertificate-controller-system` | Identity material the chart does not create |
| `actor-id-ca-certs`, `ate-api-authentication` | `ate-system` | Actor-id trust root and JWT authentication config |
| `kagent` Helm release | `kagent` | kagent Enterprise, gVisor pool `kagent-default` |
| `kubectl-ate` | local working directory | Actor and Worker CLI |

This is the baseline [010](010-create-an-agent.md) assumes. Don't tear it down between labs.

## Cleanup

Don't clean this up until you're done with the workshop. The full teardown is [099](099-cleanup.md).

Component-level rollback, if the install failed partway and you want to redo it:

```bash
helm uninstall kagent -n kagent 2>/dev/null || true
helm uninstall kagent-crds -n kagent 2>/dev/null || true
helm uninstall substrate -n ate-system 2>/dev/null || true
```

Re-run this lab from step 1 after that. [099](099-cleanup.md) also removes the namespaces, the pool Secrets, and the local files.

## Troubleshooting

| Symptom | Fix |
|---|---|
| `WorkerPool` CRD does not exist | Re-install the CRDs with `--set substrate.enabled=true`. |
| Substrate pods stay unready on a fresh install | Steps 4 through 6 have not been applied. The chart does not create those Secrets. |
| Agent traces never show up | The OTLP endpoint is missing the `http://` prefix, so the exporter tries TLS against the collector's plaintext port. |

## Next

- [010 - Create an Agent](010-create-an-agent.md)
- [020 - Access Control](020-access-control.md) - read this any time after the baseline is up. It applies nothing.
