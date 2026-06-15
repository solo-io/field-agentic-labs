# Substrate-Backed `AgentHarness` Walkthrough

End-to-end walkthrough of running a kagent `AgentHarness` on Substrate. You verify the schema, pick a `WorkerPool`, open the `/substrate` UI, create a gateway-token Secret, apply the `AgentHarness` with `runtime: substrate`, watch it reach `Ready=True`, inspect the resources Substrate created for it, and call its gateway endpoint.

## What kagent Provides

When `controller.substrate.enabled=true`, kagent supports `runtime: substrate` on `kagent.dev/v1alpha2` `AgentHarness` resources. For each substrate harness, kagent:

- Watches the `AgentHarness` resources with `spec.runtime: substrate`
- References an existing Substrate `WorkerPool` (configurable per harness or via a default)
- **Generates one `ActorTemplate` per substrate `AgentHarness`**
- Uses `ate-api` to create, resume, and delete actors
- Exposes a browser/API gateway path through the kagent controller

kagent **does not** install Substrate and **does not** own `WorkerPool` capacity — those are platform/substrate-admin concerns.

## Lab Objectives

- Confirm kagent's `AgentHarness` CRD has `substrate` in its `runtime` enum
- Pick the right `WorkerPool`
- Create a gateway-token Secret (random, never commit)
- Apply an `AgentHarness` (`openclaw-substrate-demo`) with `runtime: substrate`
- Watch condition progression `Accepted` → `ActorTemplateReady` → `ActorReady` → `Ready`
- Inspect the generated `ActorTemplate` and call the harness gateway

## Prerequisites

- [060 — kagent installed with Substrate enabled](060-install-kagent-with-substrate.md)
- A reachable Substrate `WorkerPool` (the default `kagent-default` created by [060](060-install-kagent-with-substrate.md) is fine)
- `envsubst` (for the parameterized manifest)

## Demo Values You'll Need

You will replace these with your own values. The source documentation hardcoded a `felevan` bucket; this workshop parameterizes everything.

| Setting | Value to use |
|---|---|
| kagent namespace | `kagent` |
| Substrate namespace | `ate-system` |
| `ate-api` service | `api.ate-system.svc:443` |
| `atenet-router` URL | `http://atenet-router.ate-system.svc:80` |
| Default WorkerPool (created by [060](060-install-kagent-with-substrate.md)) | `kagent/kagent-default` |
| Snapshot bucket (yours) | `gs://<YOUR_SNAPSHOT_BUCKET>/kagent/<HARNESS_NAME>/` |

Export the per-harness values once:

```bash
export HARNESS_NAME=openclaw-substrate-demo
export WORKER_POOL_NAME=kagent-default
export SNAPSHOT_BUCKET="gs://<YOUR_SNAPSHOT_BUCKET>/kagent/${HARNESS_NAME}/"
export SUBSTRATE_GATEWAY_TOKEN=$(openssl rand -hex 32)
```

> **Use a real random gateway token.** The `openssl rand -hex 32` above generates 256 bits of entropy. **Do not commit the token.** If you do, rotate it immediately.

## 1. Verify kagent Has the Substrate Schema

```bash
kubectl get crd agentharnesses.kagent.dev \
  -o jsonpath='{.spec.versions[?(@.name=="v1alpha2")].schema.openAPIV3Schema.properties.spec.properties.runtime.enum}'
```

Expected:

```text
["openshell","substrate"]
```

If you don't see `substrate` in the list, kagent is too old / the wrong chart — confirm you installed `0.9.7` per [060](060-install-kagent-with-substrate.md).

## 2. Find a WorkerPool

```bash
kubectl get workerpools.ate.dev -A
```

If [060](060-install-kagent-with-substrate.md) created the default for you, you'll see `kagent/kagent-default`. Pin it explicitly in `spec.substrate.workerPoolRef.name`, or omit `workerPoolRef` and let kagent's `controller.substrate.defaultWorkerPool.name` setting fill it in.

## 3. Open the `/substrate` UI

```bash
kubectl -n kagent port-forward service/kagent-ui 8080:8080
```

Open <http://localhost:8080/substrate> — you should see the `kagent-default` pool. The page updates live as you apply harnesses in the next steps.

## 4. Create the Gateway Token Secret

The OpenClaw / NemoClaw gateway authenticates inbound requests with a bearer token. Use a real random token; store it in a Kubernetes Secret. The manifest at [`assets/agentharness/gateway-token.yaml`](assets/agentharness/gateway-token.yaml) takes `${SUBSTRATE_GATEWAY_TOKEN}` from the env:

```bash
envsubst < assets/agentharness/gateway-token.yaml | kubectl apply -f -
```

Confirm:

```bash
kubectl get secret my-substrate-gateway-token -n kagent
```

## 5. Create a Substrate AgentHarness

The parameterized manifest is at [`assets/agentharness/openclaw-substrate-demo.yaml`](assets/agentharness/openclaw-substrate-demo.yaml). It uses `${HARNESS_NAME}`, `${WORKER_POOL_NAME}`, and `${SNAPSHOT_BUCKET}` — all three exported above.

```yaml
apiVersion: kagent.dev/v1alpha2
kind: AgentHarness
metadata:
  name: ${HARNESS_NAME}
  namespace: kagent
spec:
  backend: openclaw          # openclaw or nemoclaw
  runtime: substrate
  description: OpenClaw harness running on Agent Substrate
  modelConfigRef: default-model-config
  substrate:
    workerPoolRef:
      name: ${WORKER_POOL_NAME}
    gatewayTokenSecretRef:
      name: my-substrate-gateway-token
    snapshotsConfig:
      location: ${SNAPSHOT_BUCKET}
```

Apply:

```bash
envsubst < assets/agentharness/openclaw-substrate-demo.yaml | kubectl apply -f -
```

> If the kagent controller has `controller.substrate.defaultWorkerPool.name=kagent-default` configured (it does, after [060](060-install-kagent-with-substrate.md)), the `workerPoolRef:` block is **optional** — you can drop it and let the default take over. Pinning it explicitly is more discoverable; both work.

## 6. Watch Readiness

```bash
kubectl get agentharness "$HARNESS_NAME" -n kagent -w
```

Full status:

```bash
kubectl get agentharness "$HARNESS_NAME" -n kagent -o yaml
```

Expected condition progression:

```text
Accepted              → manifest passes admission
ActorTemplateReady    → kagent created the ActorTemplate, Substrate built the golden snapshot
ActorReady            → ate-api created/resumed the actor
Ready                 → end-to-end ready; the gateway path will serve traffic
```

If any condition stays `False`, look at the `message` field on that condition — it usually points at the actual issue (`workerPoolRef is required`, golden-snapshot timeout, gateway-token Secret missing).

## 7. Inspect Generated Substrate Resources

```bash
kubectl get workerpools.ate.dev -A
kubectl get actortemplates.ate.dev -A
kubectl get agentharnesses.kagent.dev -n kagent
```

You should see a new `ActorTemplate` in the `kagent` namespace owned by the `AgentHarness` (check via `kubectl get actortemplate <name> -n kagent -o yaml` and look at `metadata.ownerReferences`). The `WorkerPool` is **externally owned** — it predates the harness and outlives it.

kagent's ownership model:

| Resource | Owner | Lifecycle |
|---|---|---|
| `AgentHarness` | You | You apply and delete it |
| `ActorTemplate` (generated) | The `AgentHarness` (owner reference) | Garbage-collected by Kubernetes when the harness is deleted |
| `WorkerPool` | Platform / substrate admin | Long-lived; outlives any individual harness |
| `Actor` (in Valkey) | kagent controller | Created on harness `Ready`; **deleted** by kagent when the harness is deleted (not GC'd) |

## 8. Use the Harness Gateway

Once `Ready=True`, kagent exposes a gateway path through the controller:

```
/api/agentharnesses/kagent/${HARNESS_NAME}/gateway/
```

Port-forward the controller (in a separate terminal if the `kagent-ui` port-forward is already running):

```bash
kubectl -n kagent port-forward service/kagent-controller 8083:8083
```

Open <http://localhost:8083/api/agentharnesses/kagent/openclaw-substrate-demo/gateway/>.

The gateway path will exercise the substrate-backed actor — first request triggers an on-demand `ResumeActor` if the actor is suspended (visible in `kubectl ate get actor` flipping to `RUNNING`), then proxies through.

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| `Missing actortemplates.ate.dev or workerpools.ate.dev` | Substrate isn't installed — [040](040-install-substrate-helm.md). |
| `spec.substrate.workerPoolRef is required` | The default WorkerPool isn't configured on the controller. Either set `spec.substrate.workerPoolRef.name` on the harness or rerun [060](060-install-kagent-with-substrate.md) with `controller.substrate.defaultWorkerPool.name=kagent-default`. |
| `ActorTemplateReady=False` | Substrate hasn't finished building the golden snapshot. `kubectl describe actortemplate <name> -n kagent` and `kubectl get pods -n kagent` to find the Golden Pod and look at its logs. |
| `ActorReady=False` | kagent created the template, but `ate-api` hasn't created or resumed the actor yet. `kubectl logs -n kagent deploy/kagent-controller \| grep -i substrate`. |
| Gateway returns `503 Service Unavailable` | Check `controller.substrate.atenetRouterURL` resolves, actor is `RUNNING` (`kubectl ate get actor`), and the gateway-token Secret exists. |
| Gateway returns `401 Unauthorized` | Wrong / missing gateway token. The Secret `my-substrate-gateway-token` must have key `token` — `kubectl get secret my-substrate-gateway-token -n kagent -o yaml`. |

## Notes

- Substrate support is for `AgentHarness`, **not** the regular `Agent` CRD.
- Supported substrate harness backends today: `openclaw` and `nemoclaw`.
- `gatewayTokenSecretRef` is preferred over the inline `gatewayToken` field.
- `snapshotsConfig.location` must be a `gs://` URI — Substrate's GCS path. Pick a per-harness subfolder so you can correlate snapshots to harnesses cleanly.
- Deleting the `AgentHarness` deletes the actor (kagent calls `DeleteActor`) and Kubernetes GC removes the generated `ActorTemplate`. The `WorkerPool` is untouched.

## Cleanup

```bash
kubectl delete agentharness "$HARNESS_NAME" -n kagent
kubectl delete secret my-substrate-gateway-token -n kagent

# (Optional) confirm the generated ActorTemplate was GC'd
kubectl get actortemplates.ate.dev -A
```

## Next

- [080 — Suspend / Resume Operations](080-operations-suspend-resume.md)
- [090 — Observability](090-observability.md)
