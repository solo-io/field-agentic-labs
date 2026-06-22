# Appendix - Install Substrate from Source (`install-ate.sh`)

The canonical install in the main workshop is Helm OCI ([040](003-install-substrate.md)). This appendix covers the **alternative** install - `./hack/install-ate.sh --deploy-ate-system` - which builds the Substrate images from source with `ko` and applies the raw kustomize manifests.

Use this path when:

- You're **contributing to Substrate** and need to test local code changes
- You want to install a specific git commit, not a published chart
- The Helm chart is broken / unavailable for your situation
- You want the full upstream control over individual components (granular `--deploy-ate-apiserver`, `--deploy-atenet`, etc.)

Otherwise, stay on the Helm path - it's faster, version-pinnable, and matches how the rest of this workshop authors examples.

## Lab Objectives

- Install Substrate via `./hack/install-ate.sh --deploy-ate-system`
- Understand which env vars the script reads
- Know the granular install / delete flags
- Understand why this path is unsupported for the kagent integration in [060](020-kagent-integration.md)

## Prerequisites

- [001 - upstream Substrate repo cloned](001-baseline-setup.md)
- [010 - GKE cluster ready](001-baseline-setup.md) (or [appendix-kind-quickstart](appendix-kind-quickstart.md) for local)
- [020 - `.ate-dev-env.sh` sourced](001-baseline-setup.md) - the script reads `PROJECT_ID`, `BUCKET_NAME`, `KO_DOCKER_REPO`, optionally `KUBECTL_CONTEXT`
- [030 - IAM + bucket](002-gcp-iam-and-bucket.md)
- `ko` (the image builder): `go install github.com/google/ko@latest`
- Docker credential helper for `KO_DOCKER_REPO` (e.g. `gcloud auth configure-docker gcr.io`)

## What the Script Does

`hack/install-ate.sh --deploy-ate-system` runs end-to-end:

1. Sources `.ate-dev-env.sh` from the repo root if present
2. (If `KUBECTL_CONTEXT` is unset) calls `gcloud container clusters get-credentials` for the context defined in env
3. Builds the Substrate images with `ko build ./cmd/{ateapi,atecontroller,atelet,atenet,ateom-gvisor,podcertcontroller}` and pushes to `KO_DOCKER_REPO`
4. Runs `openssl` to convert the bundled valkey CA cert (DER → PEM)
5. Applies the kustomize-rendered manifests from `manifests/ate-install/` against your cluster (envsubst-substituted with `${BUCKET_NAME}` etc.)
6. Waits for the system pods to be `Ready`

Compared to Helm:

| Aspect | `install-ate.sh` (this appendix) | Helm OCI ([040](003-install-substrate.md)) |
|---|---|---|
| Image source | Built from source with `ko`, pushed to your registry | Pre-built at `ghcr.io/kagent-dev/substrate/...` |
| Versioning | Per-commit (whatever `git rev-parse HEAD` resolves to) | Chart version (`--version <X.Y.Z>`) |
| Cluster modifications | Same | Same (chart RBAC + namespace + Service + Deployments + StatefulSet) |
| Customization | Edit the kustomize manifests | `--set` / `-f values.yaml` |
| Required tools | `ko`, Docker, `openssl` | `helm` |
| Used by upstream CI | Yes | (Chart is consumed by users) |

## Install

From the root of the cloned `substrate/` repo:

```bash
./hack/install-ate.sh --deploy-ate-system
```

Wait for the system pods:

```bash
kubectl get pods -n ate-system -w
```

You should see the same set as the Helm path - `ate-api-server`, `atenet-router`, `atelet` (DaemonSet), `pod-certificate-controller`, and the 6-replica `valkey` StatefulSet (plus the `valkey-cluster-init` Job).

## Granular Flags

```bash
./hack/install-ate.sh --help
```

Component-level deploys (apply only the pieces you want):

```bash
./hack/install-ate.sh --deploy-ate-apiserver
./hack/install-ate.sh --deploy-atelet
./hack/install-ate.sh --deploy-atenet
./hack/install-ate.sh --deploy-pod-certificate-controller
./hack/install-ate.sh --deploy-valkey
```

Same with `--delete-*`:

```bash
./hack/install-ate.sh --delete-ate-system
./hack/install-ate.sh --delete-atelet
# ...
```

And the per-demo flags from labs [050](010-counter-demo.md)-[053](013-claude-code-multiplex.md):

```bash
./hack/install-ate.sh --deploy-demo-counter
./hack/install-ate.sh --deploy-demo-sandbox
./hack/install-ate.sh --deploy-demo-agent-secret
./hack/install-ate.sh --deploy-demo-claude-code-multiplex

./hack/install-ate.sh --delete-all
```

## Why the kagent Integration ([060](020-kagent-integration.md)) Targets the Helm Path

The kagent install in [060](020-kagent-integration.md) sets `controller.substrate.ateApiEndpoint="dns:///api.ate-system.svc:443"` - that's the Service name the **Helm chart** publishes. The script path sometimes publishes the same name and sometimes publishes `ate-api-server.ate-system.svc:443` instead, depending on the commit.

If you go down this appendix's path and then try to do [060](020-kagent-integration.md):

1. **Check what's actually there:** `kubectl get svc -n ate-system | grep -E 'api|ateapi'`
2. **Adjust the kagent install flag** to match: `--set controller.substrate.ateApiEndpoint="dns:///<actual-name>.ate-system.svc:443"`

Same goes for `controller.substrate.atenetRouterURL`. Whatever the script publishes is what you wire kagent up to.

## Switching Paths Later

If you start with the script path and want to move to Helm, the safe approach is **uninstall everything first**:

```bash
./hack/install-ate.sh --delete-all
kubectl delete namespace ate-system

# Then re-install via Helm
helm upgrade --install substrate-crds \
 oci://ghcr.io/kagent-dev/substrate/helm/substrate-crds
helm upgrade --install substrate \
 oci://ghcr.io/kagent-dev/substrate/helm/substrate \
 --namespace ate-system --create-namespace
```

The CRDs in the Helm `substrate-crds` chart and the CRDs the script applies should be identical at compatible versions, but mixing them on the same cluster is the kind of thing that "works until it doesn't."

## Related

- [040 - Install Substrate (Helm OCI) - the canonical path](003-install-substrate.md)
- [appendix-kind-quickstart](appendix-kind-quickstart.md) - uses `hack/install-ate-kind.sh`, a kind-specific wrapper around `install-ate.sh`
- [099 - Cleanup](099-cleanup.md) - `--delete-all` is the script-path teardown
