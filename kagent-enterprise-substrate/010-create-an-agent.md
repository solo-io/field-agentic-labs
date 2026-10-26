# Create an Agent

Create one substrate-backed agent. Four objects matter:

1. `ModelConfig` is the LLM the Actor uses.
2. `AgentTemplate` is what the agent is: model, prompt, tools. It is reusable across harnesses.
3. `Harness` is what runs it: the runtime, image, worker pool, and snapshot location.
4. `Agent` pairs one `AgentTemplate` with one `Harness`. It is the runnable object, and it carries the `Ready` condition.

The apply below creates the `Harness`, the `AgentTemplate`, and the `Agent`. It references a `ModelConfig` named `default-model-config`.

> **API group.** These objects are `api.kagent.dev/v1alpha3`. The older `kagent.dev` group, label matching through `Harness.spec.allowedAgentTemplates`, and the `AgentInstance` conversation object are gone. A `Harness` admits nothing on its own; the `Agent` does the pairing, and each conversation is a `Session`.

## Lab Objectives

- Apply a `Harness` on the `kagent-default` worker pool
- Apply an `AgentTemplate`
- Apply an `Agent` that pairs them
- See how an Actor is named, how a request wakes it, and where snapshots go

## Prerequisites

- Baseline setup complete: [001](001-where-it-works.md) → [002](002-install.md)
- Docker with `buildx`, or another tool that can print an image digest
- For Google Cloud Storage snapshots: `gcloud`, a project, and a bucket. The install's default snapshot bucket is the one created with the Substrate install (S3/RustFS). Use the GCS steps only when snapshots should go to Cloud Storage instead.

## 1. Create the Harness, AgentTemplate, and Agent

The Harness runs the kagent Go ADK runtime that kagent Enterprise `1.0.0-alpha11` is built on, `golang-adk:1.0.0-alpha14`. Substrate refuses a Harness image that is not pinned by digest, so resolve it first:

```bash
export RUNTIME_IMAGE=ghcr.io/kagent-dev/kagent/golang-adk@$(docker buildx imagetools inspect \
  ghcr.io/kagent-dev/kagent/golang-adk:1.0.0-alpha14 | awk '/^Digest:/ {print $2; exit}')
echo $RUNTIME_IMAGE
```

`echo` prints `ghcr.io/kagent-dev/kagent/golang-adk@sha256:...`.

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
    image: ${RUNTIME_IMAGE}
  substrate:
    workerPoolRef:
      name: kagent-default
    snapshotPolicy:
      location: gs://ate-snapshots/kagent/
---
apiVersion: api.kagent.dev/v1alpha3
kind: AgentTemplate
metadata:
  name: assistant
  namespace: kagent
spec:
  modelConfig:
    name: default-model-config
  description: A substrate-backed assistant used to verify this main build.
  systemPrompt: You are a helpful assistant running on kagent.
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

To run your own runtime image instead, replace `${RUNTIME_IMAGE}` with it. It must be pinned by digest (`repository@sha256:<digest>`); the `Harness` CRD rejects a tag. The image must also be built against the same kagent release: a runtime built for an older alpha exits before it serves its readiness check, and the Agent reports `GoldenActorCrashed`.

> **`atelet` has to pull the Harness image.** Workers pull Actor images themselves, so pod `imagePullSecrets` do not apply. If the pull fails, the golden actor fails at `CallAteletRestore` with an unauthenticated pull.

> **Inline or referenced template.** `spec.template` takes exactly one of `ref` (an `AgentTemplate` in the `Agent`'s namespace) or `inline` (the full template spec). The Harness is always referenced through `harnessRef`; it cannot be inlined, so platform owners manage runtime policy separately from agent authors. Older alphas used `spec.templateRef`, which this release rejects.

Confirm the objects exist, and wait for the `Agent` to report `Ready`:

```bash
kubectl get harness,agenttemplate,agent -n kagent
kubectl wait agent/assistant -n kagent --for=condition=Ready --timeout=300s
```

You should see the `kagent` Harness, the `assistant` AgentTemplate, and the `assistant` Agent. `Ready` means the golden snapshot is prepared and a conversation can start.

## Actor Identity

The identity of an Actor is (`atespace`, name). An `atespace` is Substrate's own isolation boundary, not a Kubernetes namespace. In kagent the `atespace` is the agent's namespace, and the actor name is derived from the `Session` id (`session-` plus the lowercased id). A `Session` is one conversation. It lives in the controller's database, not as a Kubernetes object.

## 2. Grant the Snapshot Bucket

Snapshots go to the default bucket created during the Substrate install (S3/RustFS) unless you point `atelet` at Google Cloud Storage.

For Cloud Storage, both `atelet` and `ate-api-server` open the snapshot store, so both need the bucket. Add these IAM bindings. `YOUR_PROJECT_NUMBER`, `YOUR_PROJECT_ID`, and `YOUR_BUCKET` are yours to fill in. The principal paths are otherwise fixed: the `atelet` and `ate-api-server` Kubernetes service accounts in `ate-system`.

```bash
WI="principal://iam.googleapis.com/projects/YOUR_PROJECT_NUMBER/locations/global/workloadIdentityPools/YOUR_PROJECT_ID.svc.id.goog/subject/ns/ate-system/sa"
BUCKET="gs://YOUR_BUCKET"

for sa in atelet ate-api-server; do
  gcloud storage buckets add-iam-policy-binding "$BUCKET" \
    --member="$WI/$sa" --role=roles/storage.objectAdmin
  gcloud storage buckets add-iam-policy-binding "$BUCKET" \
    --member="$WI/$sa" --role=roles/storage.bucketViewer
done
```

> **Switching the snapshot backend to GCS.** Any `atelet.storageBackend` other than `s3` selects Cloud Storage for both `atelet` and `ate-api-server`, authenticated through Workload Identity. Add this to the `substrate-values.yaml` from [002 step 10](002-install.md#10-install-agent-substrate), then upgrade the release:

```yaml
rustfs:
  enabled: false                   # snapshots go to Cloud Storage

atelet:
  storageBackend: gcs
```

```bash
helm upgrade substrate oci://us-docker.pkg.dev/solo-public/enterprise-substrate-helm/substrate \
  --version $SUBSTRATE_VERSION --namespace ate-system -f substrate-values.yaml
```

The Harness `snapshotPolicy.location` then names your bucket, for example `gs://YOUR_BUCKET/kagent/`.

## How a Request Reaches an Actor

A request arrives at the router with `ate-target-actor: <atespace>/<actor>`. If the Actor is suspended, the router assigns a free Worker, `atelet` restores the snapshot into it, and the request is forwarded. Resume on the access log is `triggered` (woke it), `none` (already warm), or `joined` (waited on someone else's wake-up).

[020](020-access-control.md) is the certificate path behind that hop.

## Where the Data Goes

Substrate access logs and metrics flow into ClickHouse. Enterprise handlers expose actor-request, activation, and fleet-capacity reads from that data.

## Cleanup

Removes the objects this lab created. The [002](002-install.md) baseline stays.

```bash
kubectl delete agent assistant -n kagent --ignore-not-found
kubectl delete harness kagent -n kagent --ignore-not-found
kubectl delete agenttemplate assistant -n kagent --ignore-not-found
```

If you added the Cloud Storage bindings, remove the same roles:

```bash
for sa in atelet ate-api-server; do
  gcloud storage buckets remove-iam-policy-binding "$BUCKET" \
    --member="$WI/$sa" --role=roles/storage.objectAdmin
  gcloud storage buckets remove-iam-policy-binding "$BUCKET" \
    --member="$WI/$sa" --role=roles/storage.bucketViewer
done
```

`$WI` and `$BUCKET` are the variables from step 2. A GCS backend change on the Substrate release is removed with the Substrate uninstall in [099](099-cleanup.md).

## Next

- [020 - Access Control](020-access-control.md)
- [099 - Cleanup](099-cleanup.md) - when you are done with the workshop
