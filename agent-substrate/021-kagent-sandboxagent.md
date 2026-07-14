# kagent SandboxAgent on Substrate

[020](020-kagent-integration.md) walked the `AgentHarness` path - kagent generating an `ActorTemplate` per harness and driving the actor through `ate-api`. kagent has a second substrate-backed resource: **`SandboxAgent`**, a declarative agent that runs on the Substrate platform (`platform: substrate`). This short lab deploys one and shows where to see it.

## Why Install Order Matters (Recap)

The `ate-system` Substrate control plane (CRDs, `ate-api-server`, `atenet-router`, at least one `WorkerPool`, etc.) must be installed and healthy **before** you enable the integration on the kagent side. When you set:

```
controller:
  substrate:
    enabled: true
    ateApiEndpoint: "dns:///api.ate-system.svc:443"
    ...
```

the kagent controller does this at startup (see `go/core/pkg/app/app.go:548` in the kagent repo):

```
if cfg.Substrate.AteAPIEndpoint != "" {
    substrateAteClient, dialErr = substrate.Dial(...)
    if dialErr != nil {
        ...log...
        os.Exit(1)   // hard failure
    }
    ...
}
```

If the endpoint isn't reachable (or the substrate components aren't there yet), the controller pod will fail to start and will keep crash-looping. The same applies to the `WorkerPool`: kagent's Helm install creates `kagent-default` (via `substrateWorkerPool.create=true` in [020](020-kagent-integration.md)) because without a pool you can't create a substrate-backed agent at all:

![](assets/images/suberror.png)

## Lab Objectives

- Confirm the kagent → Substrate integration is active
- Deploy a **declarative `SandboxAgent`** with `platform: substrate`
- Inspect the deployed SandboxAgents and see them in the `/substrate` dashboard

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) → [002](002-gcp-iam-and-bucket.md) → [003](003-install-substrate.md)
- **[020](020-kagent-integration.md) Part 1 complete**: kagent installed with `controller.substrate.enabled=true` and the `kagent/kagent-default` `WorkerPool` created (the [020](020-kagent-integration.md) Helm command does both)
- A default `ModelConfig` in the `kagent` namespace (`default-model-config` - created by the kagent install)

## 1. Confirm the Integration Is Active

```bash
kubectl run substrate-status-check -n kagent --rm -i --restart=Never \
  --image=curlimages/curl:8.10.1 -- \
  http://kagent-controller:8083/api/substrate/status
```

Expected response includes:

```json
"enabled": true
```

And confirm the WorkerPool exists:

```bash
kubectl get workerpools.ate.dev -A
```

You should see `kagent/kagent-default`.

## 2. Deploy a SandboxAgent

Example declarative Substrate Agent deployment:

```yaml
kubectl apply -f - <<EOF
apiVersion: kagent.dev/v1alpha2
kind: SandboxAgent
metadata:
  name: test123
  namespace: kagent
spec:
  declarative:
    modelConfig: default-model-config
    runtime: go
    systemMessage: |-
      You're a helpful agent, made by the kagent team.

      # Instructions
          - If user question is unclear, ask for clarification before running any tools
          - Always be helpful and friendly
          - If you don't know how to answer the question DO NOT make things up, tell the user "Sorry, I don't know how to answer that" and ask them to clarify the question further
          - If you are unable to help, or something goes wrong, refer the user to https://kagent.dev for more information or support.

      # Response format:
          - ALWAYS format your response as Markdown
          - Your response will include a summary of actions you took and an explanation of the result
          - If you created any artifacts such as files or resources, you will include those in your response as well
  description: my nifty substrate agent
  platform: substrate
  substrate: {}
  type: Declarative
EOF
```

## 3. Inspect the Deployed SandboxAgents

To check Substrate Agents deployed, run the following:

```bash
kubectl get SandboxAgent -A
```

The `/substrate` page in the kagent UI (see [020](020-kagent-integration.md)) shows your workers alongside the substrate-backed agents:

```bash
kubectl -n kagent port-forward service/kagent-ui 8080:8080
```

Open `http://localhost:8080/substrate`.

## Cleanup

```bash
kubectl delete sandboxagent test123 -n kagent
```

The kagent install, the `kagent-default` `WorkerPool`, and the Substrate control plane are untouched - tear those down via [020's Cleanup](020-kagent-integration.md#cleanup) and [099](099-cleanup.md) when you're done with them.

## Troubleshooting

- **kagent controller crash-loops after enabling substrate** - the `ateApiEndpoint` isn't reachable; the controller hard-exits at startup if the dial fails (see the recap at the top). Confirm the Substrate control plane is healthy (`kubectl get pods -n ate-system`) and the endpoint value matches the actual `api` Service in `ate-system`.
- **Error creating the agent about a missing WorkerPool** - the integration requires at least one `WorkerPool` (the screenshot above). Re-check the [020](020-kagent-integration.md) Helm flags `substrateWorkerPool.create=true` / `substrateWorkerPool.name=kagent-default`.
- **`kubectl get SandboxAgent` returns "the server doesn't have a resource type"** - your kagent CRDs predate `SandboxAgent`. Confirm the CRD chart version installed in [020](020-kagent-integration.md) (`kagent-crds` `0.9.7` or later).
- **Agent created but nothing shows on `/substrate`** - check the controller logs: `kubectl logs -n kagent deploy/kagent-controller`, and verify the status endpoint from step 1 still reports `"enabled": true`.

## Next

- [020 - kagent Integration](020-kagent-integration.md) - the `AgentHarness` path, if you skipped it
- [030 - Suspend / Resume Operations](030-operations.md) - operating the actors kagent creates
- [099 - Cleanup](099-cleanup.md)
