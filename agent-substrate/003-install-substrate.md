# Install Substrate (Helm + `kubectl-ate`)

The third (and last) mandatory setup lab. Installs Substrate via Helm OCI, then builds the `kubectl-ate` CLI from the cloned source so you can drive actors from the command line.

After [001](001-baseline-setup.md) + [002](002-gcp-iam-and-bucket.md) + this lab, you have the full **baseline** that every unit-of-value lab from 010 onwards assumes.

## Lab Objectives

- Install the Substrate CRDs chart (`substrate-crds`)
- Install the Substrate chart (`substrate`) into the `ate-system` namespace
- Verify the control plane (`ate-api-server`), router (`atenet-router`), DaemonSet (`atelet`), and state store (`valkey`) come up Ready
- `go install` the `kubectl-ate` plugin and confirm `kubectl ate` works

## Two Install Paths

| Path | When to use it | Lab |
|---|---|---|
| **Helm OCI** (this lab) | Default. Pre-built images, version-pinnable, matches the workshop's authoring | This lab |
| **`./hack/install-ate.sh --deploy-ate-system`** | You're contributing to Substrate or want to install from local source via `ko` | [appendix-install-script-alternative](appendix-install-script-alternative.md) |

Don't mix them on the same cluster.

## Prerequisites

- [001](001-baseline-setup.md) + [002](002-gcp-iam-and-bucket.md) completed
- You're `cd`'d into the cloned `substrate/` repo from [001 step 3](001-baseline-setup.md#3-clone-the-upstream-substrate-repo)
- `.ate-dev-env.sh` sourced (env vars from 001 + the IAM identities from 002 still in your shell)

```bash
for V in PROJECT_ID CLUSTER_NAME BUCKET_NAME KO_DOCKER_REPO; do
 if [ -z "${!V}" ]; then echo "MISSING: $V"; fi
done
```

## 1. Install Substrate (Helm)

### CRDs

```bash
helm upgrade --install substrate-crds \
 oci://ghcr.io/kagent-dev/substrate/helm/substrate-crds
```

Verify:

```bash
kubectl get crd actortemplates.ate.dev workerpools.ate.dev
```

### Substrate

```bash
helm upgrade --install substrate \
 oci://ghcr.io/kagent-dev/substrate/helm/substrate \
 --namespace ate-system --create-namespace
```

> Upstream publishes floating OCI tags. For reproducibility, pin with `--version <X.Y.Z>` - the workshop was authored against the tag listed in the README's "Validated On" section.

> **Not on GKE?** The chart's JWT issuer defaults are GKE-flavored. For AKS / EKS / self-managed, override:
>
> ```bash
> helm upgrade --install substrate \
> oci://ghcr.io/kagent-dev/substrate/helm/substrate \
> --namespace ate-system --create-namespace \
> --set auth.jwt.issuer=<your-cluster-oidc-issuer-url> \
> --set auth.jwt.audience=api.ate-system.svc
> ```

## 2. Wait for the System Pods

```bash
kubectl get pods -n ate-system --watch
```

You should see (eventually all Ready):

| Pod prefix | Replicas | What it is |
|---|---|---|
| `ate-api-server` | 1+ | gRPC control plane |
| `atenet-router` | 1+ | Envoy + ext_proc routing actor traffic |
| `atelet` (DaemonSet) | one per node | Node supervisor + snapshot mover |
| `pod-certificate-controller` | 1 | Pod Certificate signer |
| `valkey` (StatefulSet) | 6 | State store |
| `valkey-cluster-init` (Job) | 1 (Complete) | One-shot bootstrap |

### If `ate-api-server` / `atenet-router` / `valkey` Stay Pending

Most-likely cause: nodes don't honor the `podCertificate` projected volume because they were created before the beta APIs were enabled. See [001 step 2](001-baseline-setup.md#2-confirm-the-cluster) - the fix is a fresh node pool with `--workload-metadata=GKE_METADATA`.

`kubectl describe pod` + `kubectl get events -n ate-system` will show the exact error.

## 3. Install the `kubectl-ate` CLI

From the root of the cloned `substrate/` repo:

```bash
go install ./cmd/kubectl-ate
```

This drops a binary at `$(go env GOPATH)/bin/kubectl-ate`. Put that directory on `PATH`:

```bash
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc
source ~/.zshrc
```

Verify `kubectl` discovers it as `kubectl ate`:

```bash
which kubectl-ate
kubectl ate --help
```

If `kubectl ate` reports `unknown command "ate" for "kubectl"`, your `$(go env GOPATH)/bin` isn't on `PATH`. Re-check the export.

## 4. Smoke-Test the Control Plane

```bash
# Service is reachable
kubectl get svc -n ate-system

# kubectl-ate finds the control plane (auto port-forwards)
kubectl ate get workers
```

Empty list is expected - no `WorkerPool` exists yet (those come with the demos in [010](010-counter-demo.md)-[013](013-claude-code-multiplex.md)).

## What's In Place After This Lab

| Component | Where | Role |
|---|---|---|
| Substrate CRDs (`actortemplates.ate.dev`, `workerpools.ate.dev`) | Cluster-scoped | Catalog resources for actors + worker pools |
| `ate-api-server` | `ate-system` | gRPC control plane (`ateapi.Control`, `ateapi.SessionIdentity`) |
| `atelet` (DaemonSet) | every node | Node-level supervisor + snapshot mover |
| `atenet-router` | `ate-system` | Actor traffic router (`*.actors.resources.substrate.ate.dev`) |
| `pod-certificate-controller` | `ate-system` | mTLS cert signer |
| `valkey` (StatefulSet) | `ate-system` | High-perf state store |
| `kubectl-ate` | local `PATH` | CLI plugin |

This is the **baseline**. Every unit-of-value lab from 010 onwards assumes it's in place.

## Cleanup

This lab installs the baseline that every unit-of-value lab relies on. Don't clean this up until you're done with the workshop. Full teardown is in [099](099-cleanup.md). Component-level rollback in case the install partially failed and you want to redo:

```bash
helm uninstall substrate -n ate-system 2>/dev/null || true
helm uninstall substrate-crds 2>/dev/null || true
kubectl delete namespace ate-system --ignore-not-found

rm -f "$(go env GOPATH)/bin/kubectl-ate"
```

## Next

Every unit-of-value lab from here on is self-contained. Pick one:

- [010 - Counter Demo (stateful HTTP, suspend/resume)](010-counter-demo.md) ← **start here for the canonical Substrate walkthrough**
- [011 - Sandbox Demo (Alpine shell + REPL client)](011-sandbox-demo.md)
- [012 - Agent-Secret Demo (Zero-Idle + RAM persistence)](012-agent-secret-demo.md)
- [013 - Claude Code Multiplex (3 agents on 2 pods)](013-claude-code-multiplex.md)
- [020 - kagent Integration (substrate-backed `AgentHarness`)](020-kagent-integration.md)
- [030 - Suspend / Resume Operations](030-operations.md)
- [040 - Observability (logs, metrics, traces)](040-observability.md)
