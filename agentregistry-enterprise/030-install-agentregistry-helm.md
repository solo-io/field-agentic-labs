# Install AgentRegistry Enterprise (Helm)

This lab installs the AgentRegistry Enterprise control plane on Kubernetes. It is OIDC-provider-agnostic — plug in the values from either [020 (Entra)](020-setup-entra.md) or [021 (Keycloak)](021-setup-keycloak.md).

## Lab Objectives

- Install the chart `oci://us-docker.pkg.dev/solo-public/agentregistry-enterprise/helm/agentregistry-enterprise`
- Verify the bundled PostgreSQL, ClickHouse, and OTel Collector are running
- Get the external IP and confirm the API responds
- (Entra only) Front the UI with an HTTPS Gateway so SPA login works

## Prerequisites

- [001 — arctl installed](001-install-arctl.md)
- [010 — cluster with a default `StorageClass`](010-cluster-prereqs.md)
- [020](020-setup-entra.md) or [021](021-setup-keycloak.md) — OIDC provider configured and values exported in your shell
- AWS IAM user credentials (long-lived, not STS) if you want the AWS Bedrock AgentCore runtime later

## 1. Create the Namespace

```bash
kubectl create namespace agentregistry-system
```

## 2. Build the Helm Values File

> **Do not commit this file** — it contains a client secret and AWS keys.

### 2a. Entra Values

If you came from [020](020-setup-entra.md):

```bash
cat > /tmp/are-values.yaml <<EOF
image:
  tag: v2026.5.3

global:
  git:
    username: ""
    token: ""
    secretRef:
      name: ""
      key: GIT_TOKEN
      usernameKey: ""

service:
  type: LoadBalancer

oidc:
  issuer: "https://login.microsoftonline.com/${TENANT_ID}/v2.0"
  clientId: "${ARE_BACKEND_CLIENT_ID}"
  publicClientId: "${ARE_UI_CLIENT_ID}"
  clientSecret: "${ARE_BACKEND_CLIENT_SECRET}"
  roleClaim: "groups"
  superuserRole: "${GROUP_ADMINS}"
  additionalScopes: "offline_access api://${ARE_BACKEND_CLIENT_ID}/agentregistry"
  insecureSkipVerify: false

aws:
  enabled: true
  accessKeyId: "<AWS_ACCESS_KEY_ID>"
  secretAccessKey: "<AWS_SECRET_ACCESS_KEY>"
  sessionToken: ""
  region: "us-east-1"

database:
  postgres:
    bundled:
      enabled: true

clickhouse:
  enabled: true

telemetry:
  enabled: true

extraEnvVars:
  - name: OTEL_EXPORTER_OTLP_ENDPOINT
    value: "http://agentregistry-enterprise-telemetry-collector:4317"
  - name: OTEL_SERVICE_NAME
    value: "agentregistry-enterprise"
EOF
```

Notes:

- `oidc.issuer` is the **v2.0** endpoint. The OIDC discovery document at that URL covers both v1.0 and v2.0 tokens.
- `oidc.roleClaim: groups` and `oidc.superuserRole: <group object ID>` because Entra emits group GUIDs in the `groups` claim. If you used app roles instead, set `roleClaim: roles` and `superuserRole: admin`.
- `additionalScopes` must include `offline_access` (for refresh tokens) and the backend API scope `api://<are-backend>/agentregistry` — without it, the access token's audience defaults to Microsoft Graph.
- Prefer long-lived IAM user credentials over STS session tokens — STS tokens expire in 1–12h.

### 2b. Keycloak Values

If you came from [021](021-setup-keycloak.md):

```bash
cat > /tmp/are-values.yaml <<EOF
image:
  tag: v2026.05.0

service:
  type: LoadBalancer

oidc:
  issuer: "http://${KC_IP}:8080/realms/kagent-dev"
  clientId: "are-backend"
  publicClientId: "are-cli"
  clientSecret: "${ARE_SECRET}"
  roleClaim: "groups"
  superuserRole: "admins"
  insecureSkipVerify: false

aws:
  enabled: true
  accessKeyId: "<AWS_ACCESS_KEY_ID>"
  secretAccessKey: "<AWS_SECRET_ACCESS_KEY>"
  sessionToken: ""
  region: "us-east-1"

database:
  postgres:
    bundled:
      enabled: true

clickhouse:
  enabled: true

telemetry:
  enabled: true

extraEnvVars:
  - name: OTEL_EXPORTER_OTLP_ENDPOINT
    value: "http://agentregistry-enterprise-telemetry-collector:4317"
  - name: OTEL_SERVICE_NAME
    value: "agentregistry-enterprise"
EOF
```

> **`roleClaim` mismatch is the #1 source of `"The logged in user does not have any mapped roles"`.** Decode your token at [jwt.io](https://jwt.io) and confirm the claim name matches — Keycloak realms often emit `Groups` (capital G), `groups`, or `realm_access.roles`.

### 2c. Private-Cluster Variant

If your service has to sit behind an Istio Gateway instead of a `LoadBalancer`, change `service.type` to `ClusterIP` and skip ahead to [035](035-private-cluster-istio-routing.md) for the Gateway + HTTPRoute.

## 3. Install the Chart

```bash
helm upgrade --install agentregistry-enterprise \
  oci://us-docker.pkg.dev/solo-public/agentregistry-enterprise/helm/agentregistry-enterprise \
  --version 2026.5.3 \
  --namespace agentregistry-system \
  -f /tmp/are-values.yaml \
  --wait --timeout 5m
```

> For the Keycloak track use `--version 2026.05.0` and `image.tag: v2026.05.0` to match the validated combination in [021](021-setup-keycloak.md).

## 4. Verify

```bash
kubectl get pods -n agentregistry-system
```

Expected (all 1/1 Running):

```
agentregistry-enterprise-<hash>                       1/1 Running
agentregistry-enterprise-clickhouse-shard0-0          1/1 Running
agentregistry-enterprise-postgresql-<hash>            1/1 Running
agentregistry-enterprise-telemetry-collector-<hash>   1/1 Running
```

```bash
kubectl get svc -n agentregistry-system
```

The `agentregistry-enterprise` Service exposes:

| Port | Purpose |
|------|---------|
| 8080 | HTTP — UI + API |
| 21212 | Agent Gateway gRPC |
| 31313 | MCP server |

Server logs should show `using OIDC authentication`, `HTTP server starting on :8080`, and `all migrations applied successfully`:

```bash
kubectl logs -n agentregistry-system deployment/agentregistry-enterprise --tail=30
```

Grab the external IP for the UI:

```bash
export AR_IP=$(kubectl get svc agentregistry-enterprise -n agentregistry-system \
  -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
echo "UI:        http://$AR_IP:8080"
echo "API docs:  http://$AR_IP:8080/docs"
```

## 5. Expose the UI over HTTPS for Entra SPA Login

Skip this section if you are using Keycloak — Keycloak SPA login over HTTP works on non-localhost addresses.

Microsoft Entra requires **HTTPS** for SPA redirect URIs on non-localhost. The fastest way to get there is an HTTPS Gateway with a self-signed certificate that terminates TLS in front of the AgentRegistry Service. This requires a Gateway API controller with TLS support (Enterprise Agentgateway, Istio, etc.) on your cluster.

### 5a. Create the HTTPS Gateway

```bash
kubectl apply -f - <<'EOF'
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: are-https-gateway
  namespace: agentregistry-system
  labels: { app: agentregistry-enterprise }
spec:
  gatewayClassName: enterprise-agentgateway
  listeners:
    - name: https
      port: 443
      protocol: HTTPS
      tls:
        mode: Terminate
        certificateRefs:
          - group: ""
            kind: Secret
            name: are-https-tls
      allowedRoutes:
        namespaces: { from: Same }
EOF
```

Wait for an address:

```bash
kubectl get gateway are-https-gateway -n agentregistry-system -w
export ARE_HTTPS_IP=$(kubectl get gateway are-https-gateway -n agentregistry-system \
  -o jsonpath='{.status.addresses[0].value}')
echo "HTTPS Gateway IP: $ARE_HTTPS_IP"
```

### 5b. Generate a Self-Signed Cert

```bash
openssl req -x509 -nodes -newkey rsa:2048 -days 365 \
  -keyout /tmp/are-https.key \
  -out /tmp/are-https.crt \
  -subj "/CN=${ARE_HTTPS_IP}" \
  -addext "subjectAltName = IP:${ARE_HTTPS_IP}"

kubectl create secret tls are-https-tls \
  -n agentregistry-system \
  --cert=/tmp/are-https.crt --key=/tmp/are-https.key \
  --dry-run=client -o yaml | kubectl apply -f -
```

### 5c. Route HTTPS Traffic to the Service

```bash
kubectl apply -f - <<'EOF'
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: are-https-route
  namespace: agentregistry-system
  labels: { app: agentregistry-enterprise }
spec:
  parentRefs:
    - name: are-https-gateway
      namespace: agentregistry-system
      sectionName: https
  rules:
    - matches:
        - path: { type: PathPrefix, value: / }
      backendRefs:
        - { group: "", kind: Service, name: agentregistry-enterprise, port: 8080 }
EOF
```

Smoke test:

```bash
curl -k -I "https://${ARE_HTTPS_IP}/"
curl -k -I "https://${ARE_HTTPS_IP}/callback"
```

Both should return `HTTP/2 200`.

### 5d. Add the HTTPS Callback to the `are-ui` App Registration

**Portal**: `are-ui` > **Authentication** > **Single-page application** > Add `https://<ARE_HTTPS_IP>/callback`.

**CLI**:

```bash
az ad app update --id "$ARE_UI_CLIENT_ID" \
  --set "spa={\"redirectUris\":[\"https://${ARE_HTTPS_IP}/callback\"]}"
```

The UI is now at `https://<ARE_HTTPS_IP>` (accept the self-signed cert warning).

## 6. Updating Credentials

To rotate AWS keys or change OIDC values, edit `/tmp/are-values.yaml` and re-run:

```bash
helm upgrade agentregistry-enterprise \
  oci://us-docker.pkg.dev/solo-public/agentregistry-enterprise/helm/agentregistry-enterprise \
  --version 2026.5.3 \
  --namespace agentregistry-system \
  -f /tmp/are-values.yaml \
  --wait --timeout 5m
```

The server pod rolls automatically when the AWS Secret changes.

## Troubleshooting

### Pods not starting

```bash
kubectl describe pod -n agentregistry-system -l app.kubernetes.io/name=agentregistry-enterprise
kubectl logs -n agentregistry-system deployment/agentregistry-enterprise -c wait-for-postgres
kubectl logs -n agentregistry-system deployment/agentregistry-enterprise
```

Most common cause: the cluster has no default `StorageClass`. See [010](010-cluster-prereqs.md).

### `Token provider: disabled (encryption key not set)`

Informational, not an error. Agent token minting requires `config.jwtPrivateKey` (a 64-char hex string). Only needed if you want to issue tokens to deployed agents:

```yaml
config:
  jwtPrivateKey: "<openssl rand -hex 32>"
```

### Issuer mismatch

Decode your token at [jwt.ms](https://jwt.ms) (Entra) or [jwt.io](https://jwt.io). The `iss` claim must match `oidc.issuer` in your Helm values exactly. For Entra v1.0 tokens this means `https://sts.windows.net/<TENANT_ID>/`; for v2.0 tokens it's `https://login.microsoftonline.com/<TENANT_ID>/v2.0`. See [020 troubleshooting](020-setup-entra.md#v10-vs-v20-issuer-mismatch).

## Next

- [035 — Private-Cluster Istio Routing](035-private-cluster-istio-routing.md) — if you set `service.type: ClusterIP`
- [040 — Authenticate `arctl`](040-arctl-auth.md)
