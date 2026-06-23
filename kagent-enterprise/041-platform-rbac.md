# Platform RBAC for kagent CRDs

`AccessPolicy` controls what agents and users can do at the **runtime** layer. Plain Kubernetes RBAC controls what cluster identities can do at the **platform** layer - who can list, create, and edit the kagent CRDs (`agents`, `mcpservers`, `modelconfigs`). This lab demonstrates the kagent-specific RBAC pattern: a `ClusterRole` granting read access to the kagent CRDs, bound to a `ServiceAccount`, then verified with `kubectl auth can-i`.

## Lab Objectives

- Create a `ServiceAccount` `test-reader` in the `kagent` namespace
- Define a `ClusterRole` (`kagent-crd-viewer`) granting `get/list/watch` on `agents`, `mcpservers`, `modelconfigs` in the `kagent.dev` API group
- Bind the role to the SA with a `ClusterRoleBinding`
- Verify read access works and write access is denied

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) → [002](002-licenses-and-secrets.md) → [003](003-install-kagent-enterprise.md)
## 1. Create the ServiceAccount

```bash
kubectl create serviceaccount test-reader -n kagent
```

## 2. Define the ClusterRole

```bash
kubectl apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kagent-crd-viewer
rules:
  - apiGroups: ["kagent.dev"]
    resources: ["agents", "mcpservers", "modelconfigs"]
    verbs: ["get", "list", "watch"]
EOF
```

If you want broader read access (every kagent CRD in every API group), add `policy.kagent-enterprise.solo.io` and `enterpriseagentgateway.solo.io` rules - though usually you'd factor that into a separate role.

## 3. Bind the ClusterRole to the ServiceAccount

```bash
kubectl apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kagent-viewer-binding
subjects:
  - kind: ServiceAccount
    name: test-reader
    namespace: kagent
roleRef:
  kind: ClusterRole
  name: kagent-crd-viewer
  apiGroup: rbac.authorization.k8s.io
EOF
```

## 4. Verify Read Access Works

```bash
kubectl auth can-i get mcpservers.kagent.dev --as=system:serviceaccount:kagent:test-reader
```

Expected: `yes`.

## 5. Verify Write Access Is Denied

```bash
kubectl auth can-i create mcpservers.kagent.dev --as=system:serviceaccount:kagent:test-reader
```

Expected: `no`.

## 6. Confirm by Actually Trying to Create

```bash
kubectl apply -f - --as=system:serviceaccount:kagent:test-reader <<EOF
apiVersion: kagent.dev/v1alpha1
kind: MCPServer
metadata:
  name: test-reader-only
  namespace: kagent
  labels:
    kagent.solo.io/waypoint: "true"
spec:
  deployment:
    image: mcp/everything
    port: 3000
    cmd: npx
    args:
      - "-y"
      - "@modelcontextprotocol/server-github"
  transportType: stdio
EOF
```

Expected error:

```
Error from server (Forbidden): error when creating "STDIN": mcpservers.kagent.dev is forbidden:
  User "system:serviceaccount:kagent:test-reader" cannot create resource "mcpservers" in API group
  "kagent.dev" in the namespace "kagent"
```

## Cleanup

```bash
kubectl delete serviceaccount test-reader -n kagent
kubectl delete clusterrolebinding kagent-viewer-binding
kubectl delete clusterrole kagent-crd-viewer
```

## When to Use This vs `AccessPolicy`

| Use plain K8s RBAC when… | Use `AccessPolicy` when… |
|---|---|
| Restricting who can *manage* CRDs (apply, delete, edit) | Restricting who can *invoke* an agent at runtime |
| Granting CI/CD pipelines or operators read-only access | Restricting which MCP tools an agent can call |
| Standard Kubernetes platform governance | OIDC-claim-based runtime authz (groups, preferred_username, …) |

In practice both are needed: RBAC for "who can shape the deployment", `AccessPolicy` for "who can use what's deployed".

## Next

- [080 - Kubernetes OIDC Auth with Pinniped + Keycloak](060-pinniped-keycloak.md) - replace the implicit cluster-admin identity in your kubeconfig with Keycloak-fronted users + groups
