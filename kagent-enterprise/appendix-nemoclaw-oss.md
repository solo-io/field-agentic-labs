# Appendix - Kagent OSS + OpenShell (NemoClaw) Sandbox

> **This lab uses kagent OSS (`kagent-dev/kagent`), not Solo Enterprise for kagent.** The install commands, Helm charts, and CRDs are different. **Do not run this on a cluster that already has kagent-enterprise installed** - the two products co-exist poorly. Use a separate cluster.

This appendix walks through deploying the OSS kagent build from the `eitanya/openshell` branch with the OpenShell/OpenClaw sandbox integration on a GKE cluster. The integration adds sandbox CRDs, an OpenShell gRPC backend, SSH proxy support, and UI components for managing sandboxed agents.

## Architecture

```
OpenShell Gateway (NVIDIA)            ── StatefulSet in `openshell` ns
  │
  │ gRPC :8080 (must be reachable before kagent starts)
  ▼
Agent Sandbox Controller              ── StatefulSet in `agent-sandbox-system` ns
  │  (reconciles sandboxes.agents.x-k8s.io CRs)
  ▼
Kagent (with OpenShell integration)   ── kagent controller + UI + agents in `kagent` ns
```

The OpenShell gateway **must** be deployed before kagent. The kagent controller will `CrashLoopBackOff` if it can't reach `openshell.openshell.svc.cluster.local:8080` on startup.

## Prerequisites

- A running Kubernetes cluster (amd64 nodes)
- `kubectl` against the cluster
- `helm` v3
- `gcloud` authenticated with access to a GAR repo
- Docker Desktop running
- Docker `buildx` with multi-platform support
- Go 1.26.1+ (for controller manifest generation)
- An LLM provider API key (Anthropic, OpenAI, etc.)
- kagent OSS at v0.9.2 or higher

## 1. Clone the Repositories

```bash
git clone https://github.com/kagent-dev/kagent.git
cd kagent
git fetch origin eitanya/openshell
git checkout -b eitanya/openshell origin/eitanya/openshell
```

```bash
git clone https://github.com/kagent-dev/OpenShell.git
cd OpenShell
git checkout feat/k8s-supervisor-sideload-fork
```

## 2. Build and Push Kagent Images

The openshell branch has unreleased changes to controller, UI, and CRDs - you must build from source.

### Configure the Container Registry (GAR)

```bash
gcloud auth configure-docker us-docker.pkg.dev --quiet

export DOCKER_REGISTRY=us-docker.pkg.dev/<YOUR_PROJECT>/<YOUR_REPO>
export DOCKER_REPO=kagent-dev/kagent
export OPENSHELL_REGISTRY=us-docker.pkg.dev/<YOUR_PROJECT>/<YOUR_REPO>/openshell
```

### Grant GKE Nodes Pull Access

```bash
gcloud container clusters describe <CLUSTER_NAME> --region <REGION> \
  --format="json(nodePools[].config.serviceAccount)"

gcloud artifacts repositories add-iam-policy-binding <REPO_NAME> \
  --location=<LOCATION> \
  --member="serviceAccount:<SA_EMAIL>" \
  --role="roles/artifactregistry.reader" \
  --project=<PROJECT>
```

> If your node pool uses the `default` compute SA with `devstorage.read_only` (common default), the IAM binding alone is insufficient - you need an `imagePullSecret` (see [Troubleshooting](#imagepullbackoff-403-forbidden)).

### Build (linux/amd64)

```bash
cd kagent

make controller-manifests

DOCKER_REGISTRY=$DOCKER_REGISTRY \
DOCKER_REPO=$DOCKER_REPO \
DOCKER_BUILD_ARGS="--push --platform linux/amd64" \
  make build-controller build-ui build-kagent-adk build-skills-init

# app image depends on kagent-adk — must run after
DOCKER_REGISTRY=$DOCKER_REGISTRY \
DOCKER_REPO=$DOCKER_REPO \
DOCKER_BUILD_ARGS="--push --platform linux/amd64" \
  make build-app

export VERSION=$(git describe --tags --always)
echo "Image tag: $VERSION"
```

### Generate Helm Charts

```bash
make helm-version
```

## 3. Deploy the OpenShell Gateway (First)

### Build and Push OpenShell Images

```bash
cd OpenShell
export OPENSHELL_TAG=$(git rev-parse --short HEAD)

for COMP in gateway supervisor; do
  DOCKER_PLATFORM=linux/amd64 \
  DOCKER_PUSH=1 \
  IMAGE_REGISTRY=$OPENSHELL_REGISTRY \
  IMAGE_TAG=$OPENSHELL_TAG \
    tasks/scripts/docker-build-image.sh $COMP
done
```

### Install the OpenShell Helm Chart

```bash
helm upgrade --install openshell deploy/helm/openshell \
  -n openshell --create-namespace \
  --set server.disableTls=true \
  --set server.disableGatewayAuth=true \
  --set service.type=ClusterIP \
  --set service.metricsPort=0 \
  --set image.repository=${OPENSHELL_REGISTRY}/gateway \
  --set image.tag=${OPENSHELL_TAG} \
  --set image.pullPolicy=Always \
  --set supervisor.image.repository=${OPENSHELL_REGISTRY}/supervisor \
  --set supervisor.image.tag=${OPENSHELL_TAG} \
  --set server.sandboxImagePullPolicy=IfNotPresent
```

### Create the SSH Handshake Secret

```bash
kubectl -n openshell create secret generic openshell-ssh-handshake \
  --from-literal=secret=$(openssl rand -hex 32)
```

### Apply the Sandbox CRD

```bash
kubectl apply -f deploy/kube/manifests/agent-sandbox.yaml
```

Creates the `agent-sandbox-system` namespace, the `sandboxes.agents.x-k8s.io` CRD, and the sandbox controller StatefulSet.

### Wait + Verify

```bash
kubectl -n openshell rollout status statefulset/openshell --timeout=120s
kubectl get pods -n openshell
kubectl get pods -n agent-sandbox-system
```

Expected:

```
# openshell namespace
NAME          READY   STATUS    RESTARTS   AGE
openshell-0   1/1     Running   0          60s

# agent-sandbox-system namespace
NAME                         READY   STATUS    RESTARTS   AGE
agent-sandbox-controller-0   1/1     Running   0          45s
```

## 4. Install Kagent OSS

```bash
cd kagent

helm install kagent-crds ./helm/kagent-crds/ \
  --namespace kagent --create-namespace \
  --wait --timeout 5m

helm install kagent ./helm/kagent/ \
  --namespace kagent --create-namespace \
  --timeout 5m --wait \
  --set registry=$DOCKER_REGISTRY \
  --set tag=$VERSION \
  --set imagePullPolicy=Always \
  --set controller.image.pullPolicy=Always \
  --set ui.image.pullPolicy=Always \
  --set ui.service.type=LoadBalancer \
  --set providers.default=anthropic \
  --set providers.anthropic.apiKey=<YOUR_ANTHROPIC_API_KEY>
```

Per-provider flags:

| Provider | Flags |
|---|---|
| Anthropic | `--set providers.default=anthropic --set providers.anthropic.apiKey=<KEY>` |
| OpenAI | `--set providers.default=openAI --set providers.openAI.apiKey=<KEY>` |
| Gemini | `--set providers.default=gemini --set providers.gemini.apiKey=<KEY>` |
| Ollama | `--set providers.default=ollama` |

## 5. Verify

```bash
kubectl get pods -n kagent
```

Expected (all 1/1):

```
NAME                                              READY   STATUS
kagent-controller-<hash>                          1/1     Running
kagent-ui-<hash>                                  1/1     Running
kagent-kmcp-controller-manager-<hash>             1/1     Running
kagent-postgresql-<hash>                          1/1     Running
kagent-grafana-mcp-<hash>                         1/1     Running
kagent-querydoc-<hash>                            1/1     Running
kagent-tools-<hash>                               1/1     Running
k8s-agent-<hash>                                  1/1     Running
istio-agent-<hash>                                1/1     Running
kgateway-agent-<hash>                             1/1     Running
promql-agent-<hash>                               1/1     Running
observability-agent-<hash>                        1/1     Running
helm-agent-<hash>                                 1/1     Running
argo-rollouts-conversion-agent-<hash>             1/1     Running
cilium-policy-agent-<hash>                        1/1     Running
cilium-manager-agent-<hash>                       1/1     Running
cilium-debug-agent-<hash>                         1/1     Running
```

Controller logs:

```bash
kubectl logs -n kagent -l app.kubernetes.io/component=controller --tail=20
```

You should see:

```
"msg":"Starting KAgent Controller"
"msg":"running database migrations"
"msg":"database migrations complete"
```

If you see `unable to build openshell sandbox backends`, OpenShell isn't reachable - see [Troubleshooting](#controller-crashloopbackoff-unable-to-build-openshell-sandbox-backends).

UI:

```bash
export KAGENT_IP=$(kubectl get svc kagent-ui -n kagent -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
echo "Kagent UI: http://$KAGENT_IP:8080"
# Or:
kubectl -n kagent port-forward svc/kagent-ui 8080:8080
```

## Sandbox CRD

### Basic Sandbox

Pick a `linux/amd64` sandbox image:

```bash
export OPENCLAW_SANDBOX_IMAGE=ghcr.io/nvidia/nemoclaw/sandbox-base:latest
docker buildx imagetools inspect "$OPENCLAW_SANDBOX_IMAGE" | grep 'linux/amd64'
```

Apply:

```bash
kubectl apply -f - <<EOF
apiVersion: kagent.dev/v1alpha2
kind: AgentHarness
metadata:
  name: my-sandbox
  namespace: kagent
spec:
  backend: openclaw
  image: ${OPENCLAW_SANDBOX_IMAGE}
  modelConfigRef: default-model-config
  description: "my openclaw agent"
EOF

kubectl get sandboxes -n kagent
```

#### GKE OpenShell Network Namespace Workaround

On GKE amd64 nodes, the OpenShell supervisor may need privileged access to create network namespaces. If the generated backend pod fails with `ip netns add` or `mount --make-shared /run/netns Permission denied`, patch the generated backend and recycle:

```bash
kubectl patch sandboxes.agents.x-k8s.io kagent-my-sandbox -n openshell --type=json \
  -p='[
    {"op":"add","path":"/spec/podTemplate/spec/containers/0/securityContext/privileged","value":true},
    {"op":"add","path":"/spec/podTemplate/spec/containers/0/securityContext/allowPrivilegeEscalation","value":true}
  ]'

kubectl delete pod kagent-my-sandbox -n openshell --wait=false
```

### Sandbox with Telegram

Channels (Telegram, Discord, Slack) only work with the `openclaw` or `nemoclaw` backends.

#### Set Up a Telegram Bot

1. Chat with **@BotFather** on Telegram, `/newbot`, save the bot token.
2. To get your user ID, chat with **@RawDataBot** - send any message, it replies with JSON.

#### Create the Bot Token Secret

```bash
kubectl -n kagent create secret generic telegram-credentials \
  --from-literal=bot-token='<your-bot-token>' \
  --dry-run=client -o yaml | kubectl apply -f -
```

#### Create the Sandbox

```bash
kubectl apply -f - <<EOF
apiVersion: kagent.dev/v1alpha2
kind: Sandbox
metadata:
  name: my-claw
  namespace: kagent
spec:
  backend: openclaw
  image: ${OPENCLAW_SANDBOX_IMAGE}
  modelConfigRef: default-model-config
  description: "my openclaw agent"
  channels:
  - name: telegram
    type: telegram
    telegram:
      allowedUserIDs:
      - "your-telegram-chat-id"
      botToken:
        valueFrom:
          type: Secret
          name: telegram-credentials
          key: bot-token
EOF
```

### Supported Backends

| Backend | `spec.backend` | Notes |
|---|---|---|
| OpenShell | `openshell` | Base sandbox with exec/SSH, no messenger channels |
| OpenClaw | `openclaw` | Sandbox with OpenClaw runtime, supports channels |
| NemoClaw | `nemoclaw` | Sandbox with NemoClaw runtime, supports channels |

### Supported Channels (openclaw / nemoclaw only)

| Channel | Required | Optional |
|---|---|---|
| Telegram | `botToken` | `allowedUserIDs`, `allowedUserIDsFrom` |
| Discord | `botToken`, `channelAccess` | `allowlistChannels` (required when `channelAccess: allowlist`) |
| Slack | `botToken`, `appToken`, `channelAccess` | `allowlistChannels`, `interactiveReplies` (default: true) |

## Troubleshooting

### Controller CrashLoopBackOff: "unable to build openshell sandbox backends"

```bash
kubectl get pods -n openshell

kubectl run -it --rm debug --image=busybox --restart=Never -- \
  nslookup openshell.openshell.svc.cluster.local

kubectl run -it --rm debug --image=busybox --restart=Never -- \
  wget -qO- --timeout=5 http://openshell.openshell.svc.cluster.local:8080/healthz
```

If OpenShell was deployed *after* kagent, restart the controller:

```bash
kubectl rollout restart deployment kagent-controller -n kagent
```

### ImagePullBackOff: 403 Forbidden

The GKE nodes can't pull from your registry. Two fixes:

**Option A - `imagePullSecret`** (good for default node pools with limited OAuth scope):

```bash
kubectl create secret docker-registry gar-pull-secret \
  --namespace kagent \
  --docker-server=us-docker.pkg.dev \
  --docker-username=oauth2accesstoken \
  --docker-password="$(gcloud auth print-access-token)"

kubectl create secret docker-registry gar-pull-secret \
  --namespace openshell \
  --docker-server=us-docker.pkg.dev \
  --docker-username=oauth2accesstoken \
  --docker-password="$(gcloud auth print-access-token)"
```

Then add `--set 'imagePullSecrets[0].name=gar-pull-secret'` to the kagent and OpenShell `helm install/upgrade` commands.

> The `oauth2accesstoken` credential expires in ~1 hour. For longer-lived access, use a service account key or Workload Identity.

**Option B - IAM** (requires `cloud-platform` OAuth scope on the node pool - the Terraform in [001](001-baseline-setup.md) does this):

```bash
gcloud artifacts repositories add-iam-policy-binding <REPO> \
  --location=<LOCATION> \
  --member="serviceAccount:<NODE_SA_EMAIL>" \
  --role="roles/artifactregistry.reader" \
  --project=<PROJECT>
```

### Agent pods stuck at 0/1

Agent pods take 30-60s to initialize. If they stay at 0/1 after 2 minutes:

```bash
kubectl logs -n kagent <agent-pod-name>
```

## Cleanup

```bash
helm uninstall kagent       -n kagent 2>/dev/null || true
helm uninstall kagent-crds  -n kagent 2>/dev/null || true
kubectl delete namespace kagent 2>/dev/null || true

helm uninstall openshell -n openshell 2>/dev/null || true
kubectl delete namespace openshell 2>/dev/null || true

kubectl delete -f OpenShell/deploy/kube/manifests/agent-sandbox.yaml --ignore-not-found
kubectl delete namespace agent-sandbox-system --ignore-not-found
```

## Reference

| Component | Value |
|---|---|
| Kagent branch | `eitanya/openshell` (`kagent-dev/kagent`) |
| OpenShell branch | `feat/k8s-supervisor-sideload-fork` (`kagent-dev/OpenShell`) |
| OpenShell gateway image | `${OPENSHELL_REGISTRY}/gateway:${OPENSHELL_TAG}` |
| OpenShell supervisor image | `${OPENSHELL_REGISTRY}/supervisor:${OPENSHELL_TAG}` |
| OpenClaw sandbox image | `${OPENCLAW_SANDBOX_IMAGE}` |
| Kagent UI port | 8080 |
| Kagent controller API port | 8083 |
| OpenShell gRPC port | 8080 |
| OpenShell health port | 8081 |
