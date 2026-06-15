# Install Agent Substrate (Helm OCI)

This is the **canonical** install path for the workshop. Substrate publishes two OCI Helm charts — `substrate-crds` and `substrate` — and you `helm install` both. The chart deploys the control plane (`ate-api-server`), the node DaemonSet (`atelet`), the router (`atenet`), the Pod Certificate signer (`podcertcontroller`), and the state store (`valkey`).

> The alternative is `./hack/install-ate.sh --deploy-ate-system`, which builds images from source with `ko` and applies the raw kustomize manifests. Use that path if you're contributing to Substrate; otherwise stay on Helm. The script path is documented in [appendix-install-script-alternative](appendix-install-script-alternative.md).

## Lab Objectives

- Install the Substrate CRDs chart
- Install the Substrate chart into `ate-system`
- Verify the control plane, DaemonSet, router, and Valkey come up

## Prerequisites

- [010 — GKE cluster ready](010-gke-cluster-prereqs.md)
- [030 — IAM + snapshot bucket](030-gcp-iam-and-bucket.md)
- `helm` v3
- `kubectl` pointed at the cluster

## Why the Order Matters

If you install **kagent with substrate enabled** before Substrate is up, the kagent controller will hard-exit on startup. From `go/core/pkg/app/app.go`:

```go
if cfg.Substrate.AteAPIEndpoint != "" {
    substrateAteClient, dialErr = substrate.Dial(...)
    if dialErr != nil {
        ...log...
        os.Exit(1)   // hard failure
    }
    ...
}
```

If `controller.substrate.ateApiEndpoint` isn't reachable, the kagent controller pod fails to start and crash-loops. **Always install Substrate first** (this lab), then kagent ([060](060-install-kagent-with-substrate.md)).

## 1. Install the CRDs

```bash
helm upgrade --install substrate-crds \
  oci://ghcr.io/kagent-dev/substrate/helm/substrate-crds
```

> Pin a chart version with `--version <X.Y.Z>` if you can. Upstream publishes floating OCI tags, and the workshop's "Validated Versions" table in [README.md](README.md#validated-versions) lists what was tested. Reproducible installs > floating tags.

Verify the CRDs landed:

```bash
kubectl get crd actortemplates.ate.dev workerpools.ate.dev
```

## 2. Install Substrate

```bash
helm upgrade --install substrate \
  oci://ghcr.io/kagent-dev/substrate/helm/substrate \
  --namespace ate-system --create-namespace
```

### If You're Not on GKE or Kind

The chart's JWT issuer defaults are GKE-flavored. If you took a different cluster path (AKS, EKS, self-managed), override the issuer + audience so the in-cluster `/substrate` UI page can validate tokens. The example below is for AKS:

```bash
helm upgrade --install substrate \
  oci://ghcr.io/kagent-dev/substrate/helm/substrate \
  --namespace ate-system --create-namespace \
  --set auth.jwt.issuer=https://aksenvironment01-dns01-xujbmtcz.hcp.westus.azmk8s.io \
  --set auth.jwt.audience=api.ate-system.svc 2>&1 | tail -20
```

The issuer URL is the cluster's OIDC issuer. Find it on AKS with:

```bash
az aks show -g <rg> -n <cluster> --query "oidcIssuerProfile.issuerUrl" -o tsv
```

On EKS:

```bash
aws eks describe-cluster --name <cluster> --query "cluster.identity.oidc.issuer" --output text
```

The audience can stay `api.ate-system.svc` for any of these — it's just the in-cluster Service DNS the kagent UI uses for validation.

## 3. Wait for the System Pods

```bash
kubectl get pods -n ate-system --watch
```

You should see (eventually):

| Pod prefix | Replicas | What it is |
|---|---|---|
| `ate-api-server` | 1+ | gRPC control plane |
| `atenet-router` | 1+ | Envoy + ext_proc routing actor traffic |
| `atelet` (DaemonSet) | one per node | Node-level supervisor + snapshot mover |
| `pod-certificate-controller` | 1 | Pod Certificate signer |
| `valkey` (StatefulSet) | 6 | State store (Redis-compatible) |
| `valkey-cluster-init` (Job) | 1 (Complete) | One-shot cluster bootstrap |

### If the Control Plane Pods Don't Come Up

The most common symptom is `ate-api-server`, `atenet-router`, or `valkey` stuck mounting the `podCertificate` volume. Two things to check:

1. **The Pod Certificate beta APIs are enabled** on the cluster — [030 step 2a](030-gcp-iam-and-bucket.md#2a-cluster-mutations--pod-certificate-beta-apis--workload-identity).
2. **The nodes the pods landed on are new enough** to honor the kubelet feature — see the reactive node-rollout note in [030 step 2a](030-gcp-iam-and-bucket.md#reactive-checks-run-only-if-step-3-in-040-hits-these-symptoms). Most-likely fix: create a fresh node pool with `--workload-metadata=GKE_METADATA`.

`kubectl describe pod` and `kubectl get events -n ate-system` will surface the exact error.

## 4. Confirm the Control Plane Is Reachable

```bash
kubectl get svc -n ate-system
```

Expected:

```
NAME             TYPE        PORT(S)
api              ClusterIP   443/TCP    # or ate-api-server depending on chart version
atenet-router    ClusterIP   80/TCP
valkey           ClusterIP   6379/TCP
```

Smoke-test reachability from inside the cluster (does not require `kubectl-ate` yet):

```bash
kubectl run grpc-probe -n ate-system --rm -i --restart=Never \
  --image=fullstorydev/grpcurl:latest -- \
  -insecure -d '{}' api.ate-system.svc:443 ateapi.Control/ListActors
```

You should see `{}` or an empty list — both confirm the service is up. `kubectl-ate` (next lab) does the same call with auto port-forwarding so you don't need `grpcurl`.

## What You Just Installed (Mapping to the Architecture)

| Component | Role | Reference |
|---|---|---|
| `ate-api-server` | Control plane — `ateapi.Control` + `ateapi.SessionIdentity` gRPC | [architecture.md](https://github.com/agent-substrate/substrate/blob/main/docs/architecture.md#control-plane-ate-api-server) |
| `atelet` (DaemonSet) | Node-level "herder" — manages physical pods, streams snapshots to GCS | [architecture.md](https://github.com/agent-substrate/substrate/blob/main/docs/architecture.md#node-supervisor-atelet--ateom) |
| `ateom` | Runs **inside** each worker pod; gRPC interface for `atelet` to trigger `runsc` checkpoint/restore | Same |
| `atenet` (router + DNS) | Envoy + ext_proc; intercepts `*.actors.resources.substrate.ate.dev` and triggers `ResumeActor` | [architecture.md](https://github.com/agent-substrate/substrate/blob/main/docs/architecture.md#networking-stack-atenet--envoy) |
| `podcertcontroller` | Issues per-pod mTLS certs for the system components | [architecture.md](https://github.com/agent-substrate/substrate/blob/main/docs/architecture.md#security--isolation) |
| `valkey` | High-perf state store for `Actor` + `Worker` rows | [architecture.md](https://github.com/agent-substrate/substrate/blob/main/docs/architecture.md#dynamic-instance-state-database-based) |

## Next

- [045 — Install the `kubectl-ate` CLI](045-install-kubectl-ate.md)
