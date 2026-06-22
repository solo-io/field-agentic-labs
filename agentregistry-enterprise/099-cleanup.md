# Cleanup

Tear down everything the workshop installed. Run this when you're done.

Each unit-of-value lab ([010](010-aws-bedrock-runtime.md)-[070](070-gitops-gitlab-ci.md)) has its **own** Cleanup section that returns the cluster to the post-baseline state. This lab is for tearing down the **baseline itself** - the agentregistry / kagent / Enterprise Agentgateway installs from [003](003-install-components.md), plus the OIDC backend from [002a](002a-setup-oidc-keycloak.md) or [002b](002b-setup-oidc-entra.md), plus the namespace from [001](001-baseline-setup.md).

> **Always run each unit-of-value lab's own cleanup first.** Some labs create AWS / external resources (IAM roles, CloudFormation stacks, image pushes) that won't get cleaned up by the cluster-side teardown below. The teardown chain matters because Helm releases own Secrets / ConfigMaps / CRD instances - if you delete the namespace before `helm uninstall`, you'll leave finalizer-stuck resources behind.

## Recommended Order

1. **Each unit-of-value lab's `## Cleanup`** - run the cleanup section of every lab you ran (010 → 020 → 030 → … → 070). Skip the ones you didn't run.
2. **This lab** - tear down the three component Helm releases, then the OIDC backend, then the namespaces, then local files.

## 1. Run Each Lab's Cleanup First

Quick checklist - confirm each ran:

```bash
arctl get runtimes
arctl get agents
arctl get mcps
arctl get prompts
arctl get accesspolicies
arctl get deployments
```

Every list should be empty (or contain only items you intentionally left for another reason). If anything is still there, find which lab created it and run that lab's Cleanup section.

## 2. Uninstall the Three Component Helm Releases

Order matters - uninstall in reverse install order to avoid dependency issues:

```bash
# Enterprise Agentgateway
helm uninstall agentgateway -n agentgateway-system 2>/dev/null || true
helm uninstall agentgateway-crds -n agentgateway-system 2>/dev/null || true

# kagent
helm uninstall kagent -n kagent 2>/dev/null || true
helm uninstall kagent-crds -n kagent 2>/dev/null || true

# agentregistry Enterprise
helm uninstall agentregistry-enterprise -n agentregistry-system 2>/dev/null || true
```

## 3. Wait for Helm-Managed Resources to Finalize

Some CRDs (ClickHouse, PostgreSQL) have finalizers. Give them a minute, then check what's left:

```bash
sleep 30
kubectl get all -n agentregistry-system 2>/dev/null
kubectl get all -n kagent 2>/dev/null
kubectl get all -n agentgateway-system 2>/dev/null
```

If anything is stuck `Terminating` for more than a few minutes, force-finalize:

```bash
kubectl get <kind> <name> -n <namespace> -o json \
 | jq '.metadata.finalizers = null' \
 | kubectl replace --raw "/api/v1/namespaces/<namespace>/<kind>/<name>/finalize" -f -
```

## 4. Delete the Three Component Namespaces

```bash
kubectl delete namespace agentregistry-system --ignore-not-found
kubectl delete namespace kagent --ignore-not-found
kubectl delete namespace agentgateway-system --ignore-not-found
```

## 5. Tear Down the OIDC Backend

### Keycloak (if you took [002a](002a-setup-oidc-keycloak.md))

```bash
kubectl delete namespace keycloak --ignore-not-found
```

### Entra ID (if you took [002b](002b-setup-oidc-entra.md))

```bash
# Make sure ARE_*_CLIENT_ID + GROUP_* are still in your shell, or look them up:
# az ad app list --filter "startswith(displayName,'are-')" --query "[].{name:displayName,id:appId}" -o table
# az ad group list --filter "startswith(displayName,'are-')" --query "[].{name:displayName,id:id}" -o table

az ad app delete --id "${ARE_BACKEND_CLIENT_ID}"
az ad app delete --id "${ARE_CLI_CLIENT_ID}"
az ad app delete --id "${ARE_UI_CLIENT_ID}"

az ad group delete --group "${GROUP_ADMINS}"
az ad group delete --group "${GROUP_READERS}"
az ad group delete --group "${GROUP_WRITERS}"
```

## 6. Local Cleanup

```bash
# Temp files
rm -f /tmp/are-values.yaml /tmp/aws-runtime.yaml /tmp/kagent-runtime.yaml \
 /tmp/agentregistry-cf.yaml

# arctl binary + PATH entry
rm -rf "$HOME/.arctl"
# (remove the PATH export from ~/.zshrc / ~/.bashrc by hand)

# Env vars (all the OIDC + AWS + image vars the workshop set)
unset OIDC_PROVIDER OIDC_ISSUER OIDC_BACKEND OIDC_PUBLIC_CLIENT \
 ARE_CLI_CLIENT_ID ARE_BACKEND_CLIENT_ID ARE_UI_CLIENT_ID \
 TENANT_ID BACKEND_CLIENT_SECRET SCOPE_ID \
 GROUP_ADMINS GROUP_READERS GROUP_WRITERS \
 AR_IP ARCTL_API_BASE_URL ARCTL_API_TOKEN \
 AWS_ACCOUNT_ID AWS_REGION AWS_ROLE_ARN AWS_EXTERNAL_ID \
 K8SHELPER_IMAGE ANTHROPIC_API_KEY GITHUB_COPILOT_MCP_TOKEN \
 KC_IP KC_TOKEN
```

## 7. (Optional) Cluster

If the cluster was created **only** for this workshop, delete it now. If it's a shared cluster, leave it.

## Verify

```bash
kubectl get ns | grep -E 'agentregistry-system|kagent|agentgateway-system|keycloak' || echo "All workshop namespaces removed"
helm list -A | grep -E 'agentregistry|kagent|agentgateway' || echo "No workshop Helm releases left"
```

Both should print the "all removed" message.

## What's Left in the Cluster

These are intentionally **not** removed by this lab:

| Resource | Why |
|---|---|
| Cluster-scoped Gateway API CRDs | Other workloads may depend on them; removing requires `kubectl delete -f https://github.com/kubernetes-sigs/gateway-api/...` from [003 step 3](003-install-components.md#3-install-enterprise-agentgateway) |
| Your default `StorageClass` | Pre-existed |
| Your `LoadBalancer` controller | Pre-existed |
| Container images pushed to your registry in [020](020-kagent-runtime-and-agent.md#2-build--push-the-k8shelper-image) | Outside the cluster - use your registry's tooling |
| AWS CloudFormation stack from [010](010-aws-bedrock-runtime.md) | Should already be deleted by 010's Cleanup; double-check with `aws cloudformation list-stacks` |

## Troubleshooting

### Namespace stuck `Terminating`

```bash
kubectl get namespace <name> -o json \
 | jq '.spec.finalizers = []' \
 | kubectl replace --raw "/api/v1/namespaces/<name>/finalize" -f -
```

### `helm uninstall` returns "release not found"

Already gone - proceed to the namespace delete.

### `arctl get ...` returns 401 / connection refused

The agentregistry server is already down. Skip the `arctl get` checks in step 1 and go straight to step 2.
