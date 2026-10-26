# Cleanup

Tear down everything the workshop installed. Run this when you're done.

[010](010-create-an-agent.md) has its own Cleanup section, which removes the `Agent`, `Harness`, and `AgentTemplate` and returns the cluster to the post-baseline state. This lab tears down the baseline itself: the kagent and Substrate Helm releases from [002](002-install.md), the database, the pool Secrets, the trust bundles, and the namespaces.

[001](001-where-it-works.md) and [020](020-access-control.md) create nothing.

> **Run 010's cleanup first** if you applied the agent. Deleting the CRD release first leaves those objects behind without a controller.

## Recommended Order

1. [010 Cleanup](010-create-an-agent.md#cleanup), if you ran that lab.
2. This lab. Uninstall Helm in reverse install order, then delete the namespaces and the cluster-scoped trust bundles, then the local files.

## 1. Remove the Agent from 010

```bash
kubectl delete agent assistant -n kagent --ignore-not-found
kubectl delete harness kagent -n kagent --ignore-not-found
kubectl delete agenttemplate assistant -n kagent --ignore-not-found
```

If you added the Cloud Storage IAM bindings in 010, remove them with that lab's cleanup before you drop the local shell variables.

## 2. Uninstall the Helm Releases

Uninstall kagent before Substrate, and each chart before its CRDs.

```bash
helm uninstall kagent -n kagent --wait 2>/dev/null || true
helm uninstall substrate -n ate-system --wait 2>/dev/null || true
helm uninstall substrate-podcert -n podcertificate-controller-system --wait 2>/dev/null || true
helm uninstall kagent-crds -n kagent --wait 2>/dev/null || true
helm uninstall substrate-crds -n ate-system --wait 2>/dev/null || true
```

Uninstalling a CRD release deletes its CRDs, and every remaining object of those kinds with them.

## 3. Delete the Namespaces

Helm uninstall does not delete the namespaces. These live in them and go with them:

- the PostgreSQL deployment, its volume, and the four connection-string Secrets from [002 step 8](002-install.md#8-deploy-postgresql)
- the license and provider Secrets from step 9, and the optional OIDC Secret
- the pool Secrets (`service-dns-ca-pool`, `postgres-ca-pool`, `pod-identity-ca-pool`, `actor-id-jwt-pool`, `actor-id-ca-pool`, `egress-mitm-ca-pool`), `actor-id-ca-certs`, and `ate-api-authentication`

```bash
kubectl delete namespace kagent ate-system podcertificate-controller-system --ignore-not-found --wait=false
kubectl wait --for=delete namespace/kagent namespace/ate-system namespace/podcertificate-controller-system --timeout=300s
```

## 4. Delete the Trust Bundles

The pod-certificate controller publishes its CA roots as cluster-scoped `ClusterTrustBundle` objects. They outlive the namespaces. Leave them behind and a reinstall's new pools are distrusted by pods that still read the old bundles, so delete them:

```bash
kubectl get clustertrustbundles -o name | grep 'ate\.dev' | while read -r bundle; do kubectl delete "$bundle"; done
```

Only the bundles whose names contain `ate.dev` belong to Substrate (`egress-mitm.ate.dev`, `podidentity.podcert.ate.dev`, `postgres.podcert.ate.dev`, `servicedns.podcert.ate.dev`). Leave the others; Kubernetes manages them.

## 5. Remove Local Files

These are the files [002](002-install.md) writes into the working directory:

```bash
rm -f kubectl-ate actor-id-ca.crt postgres.yaml kagent-setup.sql substrate-setup.sql substrate-values.yaml kagent-values.yaml
```

None of them holds a key: the license and provider keys went straight into Secrets. Clear them from the shell:

```bash
unset KAGENT_LICENSE_KEY AGENTGATEWAY_LICENSE_KEY OPENAI_API_KEY ANTHROPIC_API_KEY OIDC_CLIENT_SECRET
```

## What's Gone

| Component | Removed by |
|---|---|
| `Agent` / `Harness` / `AgentTemplate` | Step 1 |
| `kagent`, `substrate`, `substrate-podcert`, `kagent-crds`, `substrate-crds` releases, and their CRDs | Step 2 |
| PostgreSQL and its data, all Secrets, pools, authentication ConfigMap | Step 3 |
| Substrate's `ClusterTrustBundle` objects | Step 4 |
| `kubectl-ate`, the downloaded manifest and setup scripts, the values files | Step 5 |

## Next

To rebuild the baseline, start again at [001](001-where-it-works.md).
