# Configure Keycloak OIDC

This lab is the Keycloak equivalent of [020](020-setup-entra.md). It walks through deploying a Keycloak instance on your cluster (if you don't already have one), configuring an `are-backend` confidential client, an `are-cli` public client with the OAuth 2.0 Device Authorization Grant enabled, and a `groups` claim mapper — then exports the values you will plug into the Helm chart in [030](030-install-agentregistry-helm.md).

## Lab Objectives

- Deploy Keycloak (`quay.io/keycloak/keycloak:24.0`) and import the `kagent-dev` realm
- Create `admin` / `reader` / `writer` users in `admins` / `readers` / `writers` groups
- Configure `are-cli` for browser **and** device-code login (no PKCE)
- Surface the `are-backend` client secret
- Export the values consumed by [030](030-install-agentregistry-helm.md)

## Prerequisites

- A Kubernetes cluster with a `LoadBalancer`-capable Service (or use port-forward)
- `kubectl` access
- The realm file `dev/keycloak/realm-data/kagent-dev-realm.json` to populate keycloak with realm information is supplied in this repo.

If you already run Keycloak externally and just need values, skip to [Step 4](#4-helm-values-snippet) and use your existing issuer / clients.

## 1. Deploy Keycloak

```bash
kubectl create namespace keycloak

# Import realm
kubectl create configmap keycloak-realm-config -n keycloak \
  --from-file=kagent-dev.json=dev/keycloak/realm-data/kagent-dev-realm.json

kubectl apply -n keycloak -f - <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: keycloak
spec:
  replicas: 1
  selector: { matchLabels: { app: keycloak } }
  template:
    metadata: { labels: { app: keycloak } }
    spec:
      containers:
      - name: keycloak
        image: quay.io/keycloak/keycloak:24.0
        args: ["start-dev", "--import-realm"]
        env:
        - { name: KEYCLOAK_ADMIN,           value: admin }
        - { name: KEYCLOAK_ADMIN_PASSWORD,  value: admin123 }
        - { name: KC_HTTP_ENABLED,          value: "true" }
        - { name: KC_HOSTNAME_STRICT,       value: "false" }
        - { name: KC_HOSTNAME_STRICT_HTTPS, value: "false" }
        ports: [{ containerPort: 8080 }]
        readinessProbe:
          httpGet: { path: /realms/master, port: 8080 }
          initialDelaySeconds: 30
          periodSeconds: 10
        volumeMounts:
        - { name: realm-config, mountPath: /opt/keycloak/data/import }
      volumes:
      - name: realm-config
        configMap: { name: keycloak-realm-config }
---
apiVersion: v1
kind: Service
metadata:
  name: keycloak
spec:
  type: LoadBalancer
  selector: { app: keycloak }
  ports: [{ port: 8080, targetPort: 8080 }]
EOF
```

Wait for the external IP and pin the hostname:

```bash
export KC_IP=$(kubectl get svc keycloak -n keycloak \
  -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
echo "Keycloak admin: http://$KC_IP:8080 (admin / admin123)"

kubectl set env deployment/keycloak -n keycloak \
  KC_HOSTNAME_URL=http://$KC_IP:8080 \
  KC_HOSTNAME_ADMIN_URL=http://$KC_IP:8080
kubectl rollout status deployment/keycloak -n keycloak
```

## 2. Configure the Realm via the Admin API

The imported realm has the clients and groups, but you still need users, an HTTP-friendly SSL setting (for the demo), and a few client tweaks so `arctl user login` works.

```bash
KC_TOKEN=$(curl -s -X POST "http://$KC_IP:8080/realms/master/protocol/openid-connect/token" \
  -d "grant_type=password" -d "client_id=admin-cli" \
  -d "username=admin" -d "password=admin123" \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")

# Allow non-SSL cookies in the realm (DEV ONLY — use HTTPS in production)
for REALM in master kagent-dev; do
  curl -s -X PUT -H "Authorization: Bearer $KC_TOKEN" -H "Content-Type: application/json" \
    "http://$KC_IP:8080/admin/realms/$REALM" \
    -d "{\"realm\":\"$REALM\",\"sslRequired\":\"none\"}"
done

# Import users (admin / reader / writer)
curl -s -X POST -H "Authorization: Bearer $KC_TOKEN" -H "Content-Type: application/json" \
  "http://$KC_IP:8080/admin/realms/kagent-dev/partialImport" \
  -d @dev/keycloak/realm-data/kagent-dev-users-0.json

# Set passwords (username = password — demo only)
for USER in admin reader writer; do
  USER_ID=$(curl -s -H "Authorization: Bearer $KC_TOKEN" \
    "http://$KC_IP:8080/admin/realms/kagent-dev/users?username=$USER" \
    | python3 -c "import sys,json; print(json.load(sys.stdin)[0]['id'])")
  curl -s -X PUT -H "Authorization: Bearer $KC_TOKEN" -H "Content-Type: application/json" \
    "http://$KC_IP:8080/admin/realms/kagent-dev/users/$USER_ID/reset-password" \
    -d "{\"type\":\"password\",\"value\":\"$USER\",\"temporary\":false}"
done

# Add the groups scope to the AR + kagent clients so the "groups" claim appears in tokens
for CLIENT_ID in $(curl -s -H "Authorization: Bearer $KC_TOKEN" \
  "http://$KC_IP:8080/admin/realms/kagent-dev/clients" \
  | python3 -c "
import sys, json
for c in json.load(sys.stdin):
    if c.get('clientId') in ('are-backend','are-cli','kagent-ui','kagent-backend'):
        print(c['id'])
"); do
  curl -s -X PUT -H "Authorization: Bearer $KC_TOKEN" \
    "http://$KC_IP:8080/admin/realms/kagent-dev/clients/$CLIENT_ID/default-client-scopes/groups-scope-kagent-dev"
done
```

## 3. Make `are-cli` Browser + Device-Code Friendly

`arctl user login` uses the OAuth 2.0 Device Authorization Grant. Two things must be true on `are-cli`:

- `oauth2.device.authorization.grant.enabled=true`
- `pkce.code.challenge.method` **must be empty** (the device-code flow does not send a PKCE challenge)

```bash
ARE_CLI_UUID=$(curl -s -H "Authorization: Bearer $KC_TOKEN" \
  "http://$KC_IP:8080/admin/realms/kagent-dev/clients" \
  | python3 -c "
import sys, json
for c in json.load(sys.stdin):
    if c.get('clientId') == 'are-cli': print(c['id'])
")

curl -s -H "Authorization: Bearer $KC_TOKEN" \
  "http://$KC_IP:8080/admin/realms/kagent-dev/clients/$ARE_CLI_UUID" \
  -o /tmp/are-cli-client.json

python3 - <<'PY'
import json
path = "/tmp/are-cli-client.json"
client = json.load(open(path))
client["publicClient"] = True
client["standardFlowEnabled"] = True
client["redirectUris"] = ["*"]
client["webOrigins"] = ["*"]
attrs = client.setdefault("attributes", {})
attrs["oauth2.device.authorization.grant.enabled"] = "true"
attrs["pkce.code.challenge.method"] = ""
json.dump(client, open(path, "w"))
PY

curl -s -X PUT -H "Authorization: Bearer $KC_TOKEN" -H "Content-Type: application/json" \
  "http://$KC_IP:8080/admin/realms/kagent-dev/clients/$ARE_CLI_UUID" \
  -d @/tmp/are-cli-client.json
```

## 4. Grab the `are-backend` Client Secret

```bash
ARE_BACKEND_UUID=$(curl -s -H "Authorization: Bearer $KC_TOKEN" \
  "http://$KC_IP:8080/admin/realms/kagent-dev/clients" \
  | python3 -c "
import sys, json
for c in json.load(sys.stdin):
    if c.get('clientId') == 'are-backend': print(c['id'])
")

ARE_SECRET=$(curl -s -X POST -H "Authorization: Bearer $KC_TOKEN" \
  "http://$KC_IP:8080/admin/realms/kagent-dev/clients/$ARE_BACKEND_UUID/client-secret" \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['value'])")

echo "are-backend client secret: $ARE_SECRET"
```

## Helm Values Snippet

When you get to [030](030-install-agentregistry-helm.md), use this OIDC block:

```yaml
oidc:
  issuer: "http://<KC_IP>:8080/realms/kagent-dev"
  clientId: "are-backend"
  publicClientId: "are-cli"
  clientSecret: "<ARE_SECRET from step 4>"
  roleClaim: "groups"     # lowercase — matches the realm's Groups mapper
  superuserRole: "admins"
  insecureSkipVerify: false
```

> If you see `"The logged in user does not have any mapped roles"` after a successful login, decode your token at [jwt.io](https://jwt.io) and check the actual claim name (`Groups` vs `groups` vs `realm_access.roles`); set `oidc.roleClaim` to match.

## Users and Groups Reference

| User | Password | Group | Role |
|------|----------|-------|------|
| admin | admin | admins | Superuser (full access) |
| reader | reader | readers | Read-only |
| writer | writer | writers | Read + write |

## Clients Reference

| Client ID | Type | Device flow | Use |
|-----------|------|-------------|-----|
| `are-backend` | Confidential | No | Server-side token validation |
| `are-cli` | Public | Yes | CLI device-code login |
| `kagent-backend` | Confidential | No | kagent integration |
| `kagent-ui` | Public | No | kagent UI |

## Troubleshooting

### `Client is not allowed to initiate OAuth 2.0 Device Authorization Grant`

Enable device flow on `are-cli`:
**Admin** > **Clients** > `are-cli` > **Capability config** > **OAuth 2.0 Device Authorization Grant** > **ON**.

### `Missing parameter: code_challenge_method`

`are-cli` requires PKCE. Clear `pkce.code.challenge.method` (the script above does this).

### `Cookie not found` in the browser on an HTTP-only install

Make sure `sslRequired=none` is set on both `master` and `kagent-dev`, and that `KC_HOSTNAME_URL` / `KC_HOSTNAME_ADMIN_URL` point at the external `http://<KC_IP>:8080`.

## Next

- [030 — Install AgentRegistry Enterprise (Helm)](030-install-agentregistry-helm.md)
- [040 — Authenticate `arctl`](040-arctl-auth.md)
