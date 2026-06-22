# Cleanup & Common Troubleshooting

Tear down everything created across the workshop. Run in order - actors first, then demos, then Helm releases, then GCP IAM + bucket, and finally the cluster (or leave it for the next workshop).

## Cleanup

### 1. Per-Lab Actors

Anything you created with `kubectl ate create actor <id>`:

```bash
# Suspend (only suspended actors can be deleted)
kubectl ate suspend actor <id>
kubectl ate delete actor <id>
```

Use `kubectl ate get actors` first to see what's still around. Bulk-delete pattern (verify the JSON shape on your version):

```bash
kubectl ate get actors -o json \
 | jq -r '.items[].id' \
 | while read id; do
 kubectl ate suspend actor "$id" 2>/dev/null
 kubectl ate delete actor "$id" 2>/dev/null
 done
```

### 2. Demos (Labs 010-013)

Each demo registers a `--delete-demo-<name>` flag with `hack/install-ate.sh` - use them rather than deleting the namespace by hand, because the script also cleans up the `WorkerPool` and `ActorTemplate`:

```bash
./hack/install-ate.sh --delete-demo-counter 2>/dev/null || true
./hack/install-ate.sh --delete-demo-sandbox 2>/dev/null || true
./hack/install-ate.sh --delete-demo-agent-secret 2>/dev/null || true
./hack/install-ate.sh --delete-demo-claude-code-multiplex 2>/dev/null || true
```

### 3. kagent Integration (Lab 020)

```bash
# Any AgentHarness resources you created
kubectl delete agentharness <name> -n kagent --ignore-not-found

# Gateway-token Secret
kubectl delete secret my-substrate-gateway-token -n kagent --ignore-not-found

# kagent Helm releases (this removes the default WorkerPool too if substrateWorkerPool.create was true)
helm uninstall kagent -n kagent 2>/dev/null || true
helm uninstall kagent-crds -n kagent 2>/dev/null || true

kubectl delete namespace kagent 2>/dev/null || true
```

### 4. Substrate System

If you installed via Helm ([003](003-install-substrate.md)):

```bash
helm uninstall substrate -n ate-system 2>/dev/null || true
helm uninstall substrate-crds 2>/dev/null || true
kubectl delete namespace ate-system 2>/dev/null || true
```

If you installed via the script path ([appendix-install-script-alternative](appendix-install-script-alternative.md)):

```bash
./hack/install-ate.sh --delete-ate-system
# or, scorched-earth (also removes any demos still around):
./hack/install-ate.sh --delete-all
```

### 5. GCP Resources

```bash
./hack/teardown.sh \
 --revoke-gke-node-permissions \
 --delete-iam-policy-bindings \
 --delete-snapshot-bucket
```

> **Only run this in a dedicated demo project.** The teardown removes *whole* IAM bindings and **deletes the bucket**. In a shared project, those roles or that bucket may predate the demo or be used by unrelated workloads. If in doubt, remove the specific member/role pairs by hand instead of running `--delete-iam-policy-bindings`.

`./hack/teardown.sh --all` would *also* run `--delete-cluster` and `--delete-gvisor-node-pool`. If you brought your own cluster, **don't** use `--all` - use the granular flags above.

#### Project-level `atelet` roles aren't reversed by teardown

The script removes the *bucket-scoped* `atelet` bindings but has no reverse for the project-level grants from [002 step 6](002-gcp-iam-and-bucket.md#6-project-level-iam-image-pull--snapshot-access). Remove by hand:

```bash
gcloud projects remove-iam-policy-binding "$PROJECT_ID" \
 --member="$ATELET_PRINCIPAL" --role=roles/storage.objectAdmin --condition=None
gcloud projects remove-iam-policy-binding "$PROJECT_ID" \
 --member="$ATELET_PRINCIPAL" --role=roles/artifactregistry.reader --condition=None
```

`--condition=None` targets the unconditioned bindings that [002](002-gcp-iam-and-bucket.md) created.

#### Custom node SAs aren't reversed either

`--revoke-gke-node-permissions` only revokes from the **default** Compute Engine SA (it's hardcoded in `hack/teardown.sh`). If you used a custom `NODE_SA`, remove its bindings by hand:

```bash
gcloud projects remove-iam-policy-binding "$PROJECT_ID" \
 --member="serviceAccount:${NODE_SA}" --role=roles/storage.objectViewer --condition=None
gcloud projects remove-iam-policy-binding "$PROJECT_ID" \
 --member="serviceAccount:${NODE_SA}" --role=roles/artifactregistry.reader --condition=None
```

#### Cluster mutations from [001 step 2](001-baseline-setup.md#2-confirm-the-cluster) are NOT rolled back

The cleanup above leaves these in place:

- The enabled Pod Certificate beta APIs (`certificates.k8s.io/v1beta1/podcertificaterequests`, `clustertrustbundles`)
- Workload Identity
- The GKE Metadata Server on any pool you flipped on
- The node rollout / fresh pool you created

Per the [GKE beta API docs](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/use-beta-apis), **enabled beta APIs cannot be disabled** on a cluster - `gcloud` has no `--disable-kubernetes-unstable-apis`. Workload Identity *can* be turned off with `--disable-workload-identity` if you need to. If you want a pristine cluster, delete and recreate it.

### 6. Local Files

```bash
rm -f .ate-dev-env.sh # if you copied it from the example
rm -f keycloak-tls.{crt,key} pinniped-kubeconfig.yaml # if you went through any auth side-quests
```

The `kubectl-ate` binary in `$(go env GOPATH)/bin` - remove if you want:

```bash
rm -f "$(go env GOPATH)/bin/kubectl-ate"
```

## Common Troubleshooting (Consolidated)

### `kubectl ate` not found

```bash
which -a kubectl-ate
ls "$(go env GOPATH)/bin"
```

Add `$(go env GOPATH)/bin` to your `PATH` and re-run `go install ./cmd/kubectl-ate`. See [003](003-install-substrate.md).

### System pods (`ate-api-server` / `atenet-router` / `valkey`) stuck not-ready

```bash
kubectl get pods -n ate-system
kubectl describe pod <stuck-pod> -n ate-system
kubectl get events -n ate-system --sort-by=.lastTimestamp
```

Most-common causes:

1. **Pod Certificate beta APIs not enabled** on the cluster. Re-run [001 step 2](001-baseline-setup.md#2-confirm-the-cluster).
2. **Nodes too old to honor the kubelet feature.** Create a fresh pool with `--workload-metadata=GKE_METADATA` - see the [reactive node-rollout note in 030](001-baseline-setup.md#2-confirm-the-cluster).

### Image pull errors

Confirm:

- The node SA (per pool) has `roles/artifactregistry.reader`
- `KO_DOCKER_REPO` matches where `ko` pushed images
- If using Artifact Registry instead of `gcr.io`, you ran `gcloud auth configure-docker us-docker.pkg.dev` (or your specific AR host)

### Snapshot read/write errors

Work through each layer:

- **Bucket-scoped** atelet bindings exist ([002 step 5](002-gcp-iam-and-bucket.md#5-bucket-scoped-iam-for-atelet)): `roles/storage.objectAdmin` + `roles/storage.bucketViewer`
- `$BUCKET_NAME` actually exists (`gcloud storage buckets describe gs://$BUCKET_NAME`)
- **Project-level** atelet bindings exist ([002 step 6](002-gcp-iam-and-bucket.md#6-project-level-iam-image-pull--snapshot-access))
- The atelet Workload Identity resolves - i.e. the GKE Metadata Server is enabled on the pool the atelet pod landed on. On a multi-pool cluster it's easy to miss a pool.

### Checkpoint/restore fails

Your `runsc` likely lacks `--allow-connected-on-save`. The demo templates pin a known-good `runsc` nightly. If you're writing your own template, mirror the `runsc:` block from `demos/counter/counter.yaml.tmpl`.

### kagent controller in `CrashLoopBackOff` immediately

`controller.substrate.enabled=true` but Substrate isn't reachable. Either fix Substrate first, or `helm upgrade kagent ... --set controller.substrate.enabled=false` to get the controller back, then debug.

### Substrate UI page (`/substrate` on kagent) shows "no substrate detected"

- Substrate isn't installed
- The `ate-api` endpoint on the kagent install is wrong (`controller.substrate.ateApiEndpoint`)
- The default `WorkerPool` wasn't created (`substrateWorkerPool.create=true` skipped or failed)

See [020](020-kagent-integration.md).

### "actor is not currently running on any worker pod"

By design. The actor is `STATUS_SUSPENDED`. Resume it - see [030](030-operations.md).

### Reset dynamic state (destructive - dev only)

```bash
kubectl ate admin debug-flush-redis
```

Flushes all Actor and Worker tracking state from Valkey. Snapshots in GCS and `ActorTemplate` / `WorkerPool` CRDs are untouched. **Never run this on a production cluster.**

## Reference Card

| Component | Value |
|---|---|
| Substrate Helm charts | `oci://ghcr.io/kagent-dev/substrate/helm/{substrate,substrate-crds}` (pin a version) |
| `ateom-gvisor` image | `ghcr.io/kagent-dev/substrate/ateom-gvisor:v0.0.6` |
| kagent chart | `oci://ghcr.io/kagent-dev/kagent/helm/kagent` `0.9.7` |
| `kubectl-ate` install | `go install ./cmd/kubectl-ate` from the upstream repo root |
| `ate-api-server` Service | `api.ate-system.svc:443` (gRPC) - name varies by chart version |
| `atenet-router` Service | `atenet-router.ate-system.svc:80` |
| Substrate DNS suffix | `<actor-id>.actors.resources.substrate.ate.dev` |
| GKE Cloud SDK | verified at `484.0.0` |
| GKE machine type | `c3-standard-4` (any 4-vCPU works) |
