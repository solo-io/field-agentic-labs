# Install Kagent Enterprise (Gloo Operator)

This lab installs Solo Enterprise for kagent via the **Gloo Operator**. The operator drives four Solo product controllers from CRs you apply into the `kagent` namespace:

- `ServiceMeshController` → installs Solo Istio in Ambient mode (`1.27.1`)
- `GatewayController` → installs Gloo Gateway (`2.0.0`)
- `KagentManagementController` → installs the Solo Enterprise management plane
- `KagentController` → installs the kagent runtime controller (image tag `0.1.5`)

This is the canonical install for the rest of the workshop. If you only need the OBO scenario, skip ahead to [090](070-obo-entra.md) - it installs kagent with a different chart pattern.

## Lab Objectives

- Install the Gloo Operator with all three license keys + `KAGENT_CONTROLLER: true`
- Apply a `ConfigMap` + four operator CRs to install Solo Istio Ambient, Gloo Gateway, the Solo Enterprise management plane, and the kagent runtime
- Verify all four product surfaces come up
- Work around a known UI bug by port-forwarding `kagent-enterprise-ui`

## Prerequisites

- [001 - Baseline Setup](001-baseline-setup.md) completed
- [002 - Licenses, Namespace, and Secrets](002-licenses-and-secrets.md) completed - namespace, license env vars, OIDC env vars, LLM Secret, `jwt` Secret, `kagent-backend-secret` all in place

## 1. Install the Gloo Operator

```bash
helm upgrade -i gloo-operator \
  oci://us-docker.pkg.dev/solo-public/gloo-operator-helm/gloo-operator \
  --version 0.4.0 \
  -n kagent \
  --create-namespace \
  --values - <<EOF
manager:
  env:
    KAGENT_CONTROLLER: true
    WATCH_NAMESPACES: "kagent"
    GLOO_GATEWAY_LICENSE_KEY: ${GLOO_GATEWAY_LICENSE_KEY}
    AGENTGATEWAY_LICENSE_KEY: ${AGENTGATEWAY_LICENSE_KEY}
    SOLO_ISTIO_LICENSE_KEY: ${SOLO_LICENSE_KEY}
EOF
```

The operator pod runs in `kagent` and watches `kagent` for the CRs you apply next.

```bash
kubectl get pods -n kagent -l app.kubernetes.io/name=gloo-operator
```

## 2. Apply the Operator ConfigMap + Four Controller CRs

`gloo-extensions-config` is a ConfigMap of Helm values that the operator merges into the charts it installs. `values.management`, `values.gloo`, and `values.kagent` map to the management, gateway, and runtime charts respectively.

> **Known UI bug:** the UI currently has a hard-coded reference to `localhost:8090` for its backend. `ui.frontend.uiBackendHost: "http://localhost:8090"` plus a `kubectl port-forward` to `kagent-enterprise-ui:8090` in [Step 4](#4-work-around-the-ui-backend-bug-port-forward) is the workaround.

Make sure `CLUSTER1_NAME`, `OIDC_BACKEND`, `OIDC_ISSUER`, `BACKEND_CLIENT_SECRET`, `authEndpoint`, `logoutEndpoint`, and `tokenEndpoint` are exported in your shell before running the heredoc. If you don't have OIDC values yet, drop the `oidc:` blocks and add them later via `kubectl edit`.

```bash
export CLUSTER1_NAME=<your-cluster-name>
# OIDC_BACKEND, OIDC_ISSUER, BACKEND_CLIENT_SECRET come from 010.
export authEndpoint=<oidc-auth-endpoint>
export logoutEndpoint=<oidc-logout-endpoint>
export tokenEndpoint=<oidc-token-endpoint>

kubectl apply -n kagent -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: gloo-extensions-config
data:
  values.management: |
    cluster: ${CLUSTER1_NAME}
    ui:
      frontend:
        uiBackendHost: "http://localhost:8090"
  values.gloo: |
    agentgateway:
      enabled: true
  values.kagent: |
    controller:
      image:
        registry: us-docker.pkg.dev/solo-public
        repository: kagent-enterprise/kagent-enterprise-kagent-enterprise-controller
        tag: 0.1.5
    oidc:
      enabled: true
---
apiVersion: operator.gloo.solo.io/v1
kind: ServiceMeshController
metadata:
  name: managed-istio
  labels:
    app.kubernetes.io/name: managed-istio
spec:
  dataplaneMode: Ambient
  installNamespace: istio-system
  version: 1.27.1
---
apiVersion: operator.gloo.solo.io/v1
kind: GatewayController
metadata:
  name: gloo-gateway
spec:
  version: 2.0.0
---
apiVersion: operator.gloo.solo.io/v1
kind: KagentManagementController
metadata:
  name: kagent-enterprise
spec:
  version: 0.1.5
  repository:
    url: oci://us-docker.pkg.dev/solo-public/kagent-enterprise-helm/charts
  oidc:
    clientID: ${OIDC_BACKEND}
    clientSecret: kagent-backend-secret
    issuer: ${OIDC_ISSUER}
    authEndpoint: ${authEndpoint}
    logoutEndpoint: ${logoutEndpoint}
    tokenEndpoint: ${tokenEndpoint}
---
apiVersion: operator.gloo.solo.io/v1
kind: KagentController
metadata:
  name: kagent
spec:
  version: 0.1.5
  repository:
    url: oci://us-docker.pkg.dev/solo-public/kagent-enterprise-helm/charts
  apiKey:
    type: OpenAI
    secretRef:
      name: llm-api-keys
      namespace: kagent
  oidc:
    clientId: ${OIDC_BACKEND}
    issuer: ${OIDC_ISSUER}
    secretRef: kagent-backend-secret
    secret: ${BACKEND_CLIENT_SECRET}
  telemetry:
    logging:
      endpoint: kagent-enterprise-ui.kagent.svc.cluster.local:4317
    tracing:
      endpoint: kagent-enterprise-ui.kagent.svc.cluster.local:4317
EOF
```

## 3. Wait for Everything to Come Up (2-3 minutes)

The operator now installs Istio Ambient, Gloo Gateway, the Solo Enterprise management plane, and the kagent runtime. Watch each surface:

```bash
# Solo Istio (Ambient mode)
kubectl get pods -n istio-system

# Gloo Gateway
kubectl get pods -n gloo-system

# Solo Enterprise UI + bundled ClickHouse
kubectl get pods -n kagent | grep -E "ui|clickhouse"

# kagent-enterprise controllers / management plane
kubectl get pods -n kagent -l app.kubernetes.io/instance=kagent-enterprise

# kagent runtime controller
kubectl get pods -n kagent -l app=kagent
```

All pods should reach `Running` with all containers `Ready=1/1`.

## 4. Work Around the UI Backend Bug (port-forward)

Because `ui.frontend.uiBackendHost` is hard-coded to `http://localhost:8090`, the UI frontend (whichever load balancer or NodePort serves it) calls `localhost:8090` from your laptop expecting to hit the UI backend. Open a terminal and keep this port-forward running:

```bash
kubectl port-forward service/kagent-enterprise-ui -n kagent 8090:8090
```

Now the UI frontend works because `localhost:8090` resolves to the in-cluster backend through the port-forward.

## Verify the Install

```bash
# Check the kagent CRDs are present
kubectl api-resources --api-group=kagent.dev
kubectl api-resources --api-group=policy.kagent-enterprise.solo.io

# Check the four operator CRs are healthy
kubectl get servicemeshcontroller,gatewaycontroller,kagentmanagementcontroller,kagentcontroller -n kagent
```

Each of those CRs should report a `Ready` / `Installed` status (the exact phrasing depends on the operator version).

## Troubleshooting

### Pods stuck in `Pending` on storage

The bundled ClickHouse + PostgreSQL need a default `StorageClass`. On a fresh GKE cluster this works out of the box (`standard-rwo`); on Kind / minikube / a custom cluster, install a CSI driver and mark a `StorageClass` as default before applying the operator CRs.

### UI shows "cannot connect to backend"

You probably forgot the `port-forward` in Step 4, or it died. Re-run:

```bash
kubectl port-forward service/kagent-enterprise-ui -n kagent 8090:8090
```

### `unauthorized` on UI login

OIDC values mismatch. `OIDC_BACKEND`, `OIDC_ISSUER`, and `BACKEND_CLIENT_SECRET` must match what your IdP has for the kagent backend client. Inspect the controller logs:

```bash
kubectl logs -n kagent -l app=kagent --tail=100 | grep -i oidc
```

## Next

- [025 - Install Enterprise Agentgateway](004-install-enterprise-agentgateway.md)
- [030 - Gateway Access Logs](050-access-logs.md)
- [040 - MCP Server + Agent](010-mcp-connection-agent-config.md)
