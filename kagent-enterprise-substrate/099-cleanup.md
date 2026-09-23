# Cleanup

Tear down everything the workshop installed. Run this when you're done.

[010](010-create-an-agent.md) has its own Cleanup section, which removes the `Harness` and `AgentTemplate` and returns the cluster to the post-baseline state. This lab tears down the baseline itself: the kagent and Substrate Helm releases from [002](002-install.md), the pool Secrets, and the namespaces.

[001](001-where-it-works.md) and [020](020-access-control.md) create nothing.

> **Run 010's cleanup first** if you applied the harness. Deleting the CRD release first leaves those objects behind without a controller.

## Recommended Order

1. [010 Cleanup](010-create-an-agent.md#cleanup), if you ran that lab.
2. This lab. Uninstall Helm in reverse install order, then delete the namespaces, then the local files.

## 1. Remove the Agent from 010

```bash
kubectl delete harness kagent -n kagent --ignore-not-found
kubectl delete agenttemplate assistant -n kagent --ignore-not-found
```

If you added the Cloud Storage IAM binding in 010, remove it with that lab's cleanup before you drop the local shell variables.

## 2. Uninstall the Helm Releases

Uninstall kagent before its CRDs, and uninstall both before Substrate. `substrate.enabled=false` on the kagent release means the worker pool is created by that release, not by a second Substrate copy inside kagent.

```bash
helm uninstall kagent -n kagent 2>/dev/null || true
helm uninstall kagent-crds -n kagent 2>/dev/null || true
helm uninstall substrate -n ate-system 2>/dev/null || true
```

The optional OIDC secret, if you created it:

```bash
kubectl delete secret kagent-enterprise-oidc-secret -n kagent --ignore-not-found
```

## 3. Delete the Namespaces

Helm uninstall does not delete the namespaces. The pool Secrets (`service-dns-ca-pool`, `pod-identity-ca-pool`, `actor-id-jwt-pool`, `actor-id-ca-pool`, `egress-mitm-ca-pool`), `actor-id-ca-certs`, and `ate-api-authentication` live in these namespaces and go with them.

```bash
kubectl delete namespace kagent --ignore-not-found
kubectl delete namespace ate-system --ignore-not-found
kubectl delete namespace podcertificate-controller-system --ignore-not-found
```

## 4. Remove Local Files

These are the files [002](002-install.md) writes into the working directory:

```bash
rm -f substrate-values.yaml actor-id-ca.crt kubectl-ate
```

Clear the license and provider variables from the shell:

```bash
unset ENTERPRISE_LICENSE_KEY OPENAI_API_KEY ANTHROPIC_API_KEY OIDC_CLIENT_SECRET
```

## What's Gone

| Component | Removed by |
|---|---|
| `Harness` / `AgentTemplate` | Step 1 |
| `kagent`, `kagent-crds`, `substrate` releases | Step 2 |
| Pool Secrets, authentication ConfigMap, actor-id CA Secret | Step 3 |
| `substrate-values.yaml`, `actor-id-ca.crt`, `kubectl-ate` | Step 4 |

## Next

To rebuild the baseline, start again at [001](001-where-it-works.md).
