# Install kagent with Substrate Enabled

This lab installs kagent (OSS chart `0.9.7`) with `controller.substrate.enabled=true` and creates a default `WorkerPool` so that kagent `AgentHarness` resources can use `runtime: substrate`. Substrate **must already be installed and healthy** — see the ordering note below.

If you went through this workshop in order, you already have Substrate running ([040](040-install-substrate-helm.md)). What this lab adds is the kagent side of the integration on top of that.

![kagent /substrate dashboard](assets/images/kagent.png)

## Lab Objectives

- Install kagent CRDs (`oci://ghcr.io/kagent-dev/kagent/helm/kagent-crds`, chart `0.9.7`)
- Install kagent with `controller.substrate.*` flags pointing at `ate-system`
- Create a `kagent-default` `WorkerPool` via the same Helm release
- Verify the `/substrate` UI page is reachable and shows your workers

## Ordering Matters (Read This First)

If you set `controller.substrate.enabled: true` but `ate-api.ate-system.svc:443` isn't reachable, the kagent controller pod will **hard-exit on startup** (Substrate dial failure → `os.Exit(1)`) and crash-loop indefinitely. Without a healthy Substrate install, creating an Agent that needs Substrate will fail with:

![Substrate-not-installed error](assets/images/suberror.png)

So:

1. Install Substrate first ([040](040-install-substrate-helm.md))
2. Wait for `ate-system` pods to be `Ready`
3. **Then** run this lab

If you ever upgrade kagent without checking Substrate is healthy, the controller pod can get stuck looping for the same reason — set `controller.substrate.enabled: false` temporarily to recover.

## Prerequisites

- [040 — Substrate installed in `ate-system`](040-install-substrate-helm.md), all pods `Ready`
- `helm` v3
- An **Anthropic API key** (the install below uses the Anthropic provider; OpenAI and Gemini are also supported by kagent — adjust the `providers.*` flags accordingly)

## 1. Install kagent CRDs

```bash
helm upgrade kagent-crds \
  oci://ghcr.io/kagent-dev/kagent/helm/kagent-crds \
  --version 0.9.7 \
  -n kagent --create-namespace
```

## 2. Install kagent with the Substrate Flags

The flag list is long. Each `controller.substrate.*` flag is what wires kagent into Substrate; `substrateWorkerPool.*` creates the default `WorkerPool` for `AgentHarness` resources to consume.

```bash
helm upgrade --install kagent \
  oci://ghcr.io/kagent-dev/kagent/helm/kagent \
  --version 0.9.7 \
  -n kagent \
  --set providers.default=anthropic \
  --set providers.anthropic.apiKey="$ANTHROPIC_API_KEY" \
  --set controller.agentImage.tag="" \
  --set controller.skillsInitImage.tag="" \
  --set controller.image.registry="" \
  --set controller.image.repository=kagent-dev/kagent/controller \
  --set controller.image.tag="" \
  --set controller.image.pullPolicy="" \
  --set ui.image.registry="" \
  --set ui.image.repository=kagent-dev/kagent/ui \
  --set ui.image.tag="" \
  --set ui.image.pullPolicy="" \
  --set controller.substrate.enabled=true \
  --set controller.substrate.defaultWorkerPool.namespace=kagent \
  --set controller.substrate.defaultWorkerPool.name=kagent-default \
  --set substrateWorkerPool.create=true \
  --set substrateWorkerPool.name=kagent-default \
  --set substrateWorkerPool.replicas=1 \
  --set controller.substrate.ateApiEndpoint="dns:///api.ate-system.svc:443" \
  --set controller.substrate.ateApiInsecure=true \
  --set controller.substrate.atenetRouterURL="http://atenet-router.ate-system.svc:80" \
  --set controller.substrate.ateApiTokenFile="/var/run/secrets/tokens/ate-api/token" \
  --set substrateWorkerPool.ateomImage=ghcr.io/kagent-dev/substrate/ateom-gvisor:v0.0.6
```

What the key flags do:

| Flag | Purpose |
|---|---|
| `providers.default=anthropic` + `providers.anthropic.apiKey` | LLM provider for kagent's own model calls. Swap to `openAI` / `gemini` / `ollama` if needed. |
| `controller.image.tag=""` etc. | Lets the chart use its built-in default image refs (avoids the `<registry>/<repo>:<tag>` triplet getting hard-coded to anything stale). |
| `controller.substrate.enabled=true` | Turns on the substrate integration in the controller. |
| `controller.substrate.defaultWorkerPool.{namespace,name}` | If an `AgentHarness` omits `spec.substrate.workerPoolRef`, this is the default. |
| `substrateWorkerPool.create=true` + `replicas=1` | Creates the `WorkerPool` resource in the same release. Without a `WorkerPool` you can't create any substrate-backed `AgentHarness`. |
| `controller.substrate.ateApiEndpoint="dns:///api.ate-system.svc:443"` | gRPC URL of the Substrate control plane. Note `dns:///` — this is gRPC name-resolver syntax, not a typo. |
| `controller.substrate.ateApiInsecure=true` | TLS off for the gRPC dial. Fine inside the cluster; flip to `false` and provide a cert for prod. |
| `controller.substrate.atenetRouterURL=http://atenet-router.ate-system.svc:80` | Where the harness gateway forwards traffic to the actor. |
| `controller.substrate.ateApiTokenFile=/var/run/secrets/tokens/ate-api/token` | Projected token volume the controller uses to identify itself to `ate-api`. |
| `substrateWorkerPool.ateomImage=ghcr.io/kagent-dev/substrate/ateom-gvisor:v0.0.6` | The "interior gVisor" image that runs inside each worker pod. **Pin this** — floating tags break across Substrate releases. |

## 3. Wait for kagent

```bash
kubectl get pods -n kagent -w
```

You should see:

| Pod prefix | Replicas |
|---|---|
| `kagent-controller` | 1 |
| `kagent-ui` | 1 |
| `kagent-postgresql` | 1 |
| Pre-built agents (`k8s-agent`, `istio-agent`, etc.) | several |

Smoke-test the kagent ↔ Substrate handshake:

```bash
kubectl run substrate-status-check -n kagent --rm -i --restart=Never \
  --image=curlimages/curl:8.10.1 -- \
  http://kagent-controller:8083/api/substrate/status
```

You're looking for `"enabled": true` in the response.

## 4. Open the `/substrate` UI

```bash
kubectl -n kagent port-forward service/kagent-ui 8080:8080
```

Browse to <http://localhost:8080/substrate>. You should see the `kagent-default` `WorkerPool` and its 1 warm worker listed — matching the screenshot at the top of this lab.

## Troubleshooting

- **Controller in `CrashLoopBackOff` immediately after install.** Substrate isn't healthy. `kubectl get pods -n ate-system` — if anything's not `Ready`, fix it before re-rolling kagent. As a last resort, temporarily disable the integration: `helm upgrade kagent ... --set controller.substrate.enabled=false` to get the controller back, debug Substrate, then re-enable.
- **`/substrate` page shows the suberror screenshot above.** Either Substrate isn't installed, the `ate-api` endpoint is wrong, or the `kagent-default` `WorkerPool` wasn't created (`substrateWorkerPool.create=true` skipped or failed).
- **`substrate-status-check` returns `enabled: false`.** The controller saw `controller.substrate.enabled=true` but failed to validate against `ate-api`. Inspect `kubectl logs -n kagent deploy/kagent-controller | grep -i substrate`.
- **`/api/substrate/status` returns a 404.** Wrong port — the controller API is on `8083`, not `8080`.

## Non-GKE Cluster Note

The Substrate Helm chart's JWT issuer defaults are GKE-flavored. If you're not on GKE (or kind), you also need to override the issuer + audience when installing Substrate so the kagent UI's `/substrate` page can validate tokens. See [040 step 2 — "If You're Not on GKE or Kind"](040-install-substrate-helm.md#if-youre-not-on-gke-or-kind).

## Next

- [070 — Substrate-Backed `AgentHarness` Walkthrough](070-kagent-agentharness.md)
