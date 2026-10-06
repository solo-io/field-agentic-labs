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
- A container image `ate-api` can pull. You will substitute it for `YOUR_IMAGE_PATH`.
- For Google Cloud Storage snapshots: `gcloud`, a project, and a bucket. The install's default snapshot bucket is the one created with the Substrate install (S3/RustFS). Use the GCS steps only when snapshots should go to Cloud Storage instead.

## 1. Create the Harness, AgentTemplate, and Agent

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
    image: YOUR_IMAGE_PATH
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
  templateRef:
    name: assistant
  harnessRef:
    name: kagent
EOF
```

Replace `YOUR_IMAGE_PATH` before you apply. It must be pinned by digest (`repository@sha256:<digest>`); the `Harness` CRD rejects a tag. The rest of the manifest stays as written.

> **`ate-api` has to pull the Harness image.** If it cannot, `ate-api-server` reports that the golden actor fails at `CallAteletRestore`. The underlying error is an unauthenticated pull.

> **References or inline specs.** `templateRef` and `harnessRef` point at objects in the `Agent`'s namespace. An `Agent` can instead carry the full spec inline under `template` and `harness`. Each side takes exactly one of the two.

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

For Cloud Storage, add this IAM binding. `YOUR_PROJECT`, `YOURPROJECT`, and `YOUR_BUCKET` are yours to fill in. The principal path is otherwise fixed: the `ate-api-server` Kubernetes service account in `ate-system`.

```bash
API="principal://iam.googleapis.com/projects/YOUR_PROJECT/locations/global/workloadIdentityPools/YOURPROJECT.svc.id.goog/subject/ns/ate-system/sa/ate-api-server"
BUCKET="gs://YOUR_BUCKET"

gcloud storage buckets add-iam-policy-binding "$BUCKET" \
  --member="$API" --role=roles/storage.objectAdmin

gcloud storage buckets add-iam-policy-binding "$BUCKET" \
  --member="$API" --role=roles/storage.bucketViewer
```

> **Switching the snapshot backend to GCS.** Upgrade the existing Substrate Helm install with `--set atelet.storageBackend=gcs`. In `substrate-values.yaml` that looks like:

```bash
cat > substrate-values.yaml <<'EOF'
credentialProvider:
  namespacePolicies:
    - atespace: kagent
      allowedNamespaces: [kagent]

atelet:
  gcpAuthForImagePulls: true
  storageBackend: gcs
EOF
```

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

If you added the Cloud Storage binding, remove the same two roles:

```bash
gcloud storage buckets remove-iam-policy-binding "$BUCKET" \
  --member="$API" --role=roles/storage.objectAdmin

gcloud storage buckets remove-iam-policy-binding "$BUCKET" \
  --member="$API" --role=roles/storage.bucketViewer
```

`$API` and `$BUCKET` are the variables from step 2. A GCS backend change on the Substrate release is removed with the Substrate uninstall in [099](099-cleanup.md).

## Next

- [020 - Access Control](020-access-control.md)
- [099 - Cleanup](099-cleanup.md) - when you are done with the workshop
