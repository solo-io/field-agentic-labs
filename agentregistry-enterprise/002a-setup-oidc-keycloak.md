# Setup OIDC: Keycloak (In-Cluster)

The second mandatory setup lab (Keycloak path). Stands up Keycloak in-cluster, configures the `agentregistry-enterprise` realm with three users (`admin` / `reader` / `writer`), creates the OIDC clients agentregistry needs, and exports the variables [003 - Install Components](003-install-components.md) will consume.

> **Pick one OIDC path.** This lab is the **Keycloak** path. If you'd rather use Microsoft Entra ID, go to [002b - Setup OIDC: Entra ID](002b-setup-oidc-entra.md) instead. Don't run both - they're alternatives, not additive.

## Lab Objectives

- Deploy Keycloak `quay.io/keycloak/keycloak:26.0` in-cluster
- Get a `LoadBalancer` IP and configure Keycloak's hostname
- Import the `agentregistry-enterprise` realm via the admin API (three users + three groups + two OIDC clients)
- Export the values [003](003-install-components.md) needs (`OIDC_ISSUER`, `OIDC_BACKEND`, `BACKEND_CLIENT_SECRET`, `GROUP_ADMINS`, `GROUP_READERS`, `GROUP_WRITERS`, `ARE_CLI_CLIENT_ID`)

## Prerequisites

- [001 - Baseline Setup](001-baseline-setup.md) completed
- `kubectl`, `curl`, `jq`, `python3` (for the realm-config API helpers)

## 1. Create the Keycloak Namespace

```bash
kubectl create namespace keycloak
```

## 2. Deploy Keycloak

```bash
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
 image: quay.io/keycloak/keycloak:26.0
 args: ["start-dev"]
 env:
 - { name: KEYCLOAK_ADMIN, value: admin }
 - { name: KEYCLOAK_ADMIN_PASSWORD, value: admin123 }
 - { name: KC_HTTP_ENABLED, value: "true" }
 - { name: KC_HOSTNAME_STRICT, value: "false" }
 - { name: KC_HOSTNAME_STRICT_HTTPS, value: "false" }
 ports:
 - { containerPort: 8080 }
 readinessProbe:
 httpGet: { path: /realms/master, port: 8080 }
 initialDelaySeconds: 30
 periodSeconds: 10
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

> The default admin password `admin123` is for a POC. Rotate it (`kubectl set env deployment/keycloak -n keycloak KEYCLOAK_ADMIN_PASSWORD=<new>`) before exposing this to anyone.

## 3. Wait for the External IP

```bash
kubectl get svc keycloak -n keycloak -w
# Wait for EXTERNAL-IP to be set, then Ctrl-C

export KC_IP=$(kubectl get svc keycloak -n keycloak \
 -o jsonpath='{.status.loadBalancer.ingress[0].ip}{.status.loadBalancer.ingress[0].hostname}')
echo "Keycloak admin: http://${KC_IP}:8080 (admin / admin123)"
```

Pin Keycloak's hostname so its issuer URL matches what your tokens will carry:

```bash
kubectl set env deployment/keycloak -n keycloak \
 KC_HOSTNAME_URL=http://${KC_IP}:8080 \
 KC_HOSTNAME_ADMIN_URL=http://${KC_IP}:8080
kubectl rollout status deployment/keycloak -n keycloak
```

## 4. Configure the Realm via the Admin API

You're going to script the realm-config end-to-end with `curl` + `jq` against the Keycloak admin API. Faster and more reproducible than clicking through the UI.

Get an admin token:

```bash
KC_TOKEN=$(curl -s -X POST "http://${KC_IP}:8080/realms/master/protocol/openid-connect/token" \
 -d "grant_type=password" -d "client_id=admin-cli" \
 -d "username=admin" -d "password=admin123" \
 | jq -r '.access_token')
```

Allow non-SSL cookies (HTTP-only POC; **don't do this in production** - use HTTPS):

```bash
curl -s -X PUT -H "Authorization: Bearer ${KC_TOKEN}" -H "Content-Type: application/json" \
 "http://${KC_IP}:8080/admin/realms/master" \
 -d '{"realm":"master","sslRequired":"none"}'
```

Create the `agentregistry-enterprise` realm:

```bash
curl -s -X POST -H "Authorization: Bearer ${KC_TOKEN}" -H "Content-Type: application/json" \
 "http://${KC_IP}:8080/admin/realms" \
 -d '{"realm":"agentregistry-enterprise","enabled":true,"sslRequired":"none"}'
```

## 5. Create the Three Groups

```bash
for GROUP in are-admins are-readers are-writers; do
 curl -s -X POST -H "Authorization: Bearer ${KC_TOKEN}" -H "Content-Type: application/json" \
 "http://${KC_IP}:8080/admin/realms/agentregistry-enterprise/groups" \
 -d "{\"name\":\"${GROUP}\"}"
done

# Capture each group's GUID for later use in AccessPolicy (050)
GROUPS_JSON=$(curl -s -H "Authorization: Bearer ${KC_TOKEN}" \
 "http://${KC_IP}:8080/admin/realms/agentregistry-enterprise/groups")
export GROUP_ADMINS=$(echo "${GROUPS_JSON}" | jq -r '.[] | select(.name=="are-admins") | .id')
export GROUP_READERS=$(echo "${GROUPS_JSON}" | jq -r '.[] | select(.name=="are-readers") | .id')
export GROUP_WRITERS=$(echo "${GROUPS_JSON}" | jq -r '.[] | select(.name=="are-writers") | .id')

echo "GROUP_ADMINS=${GROUP_ADMINS}"
echo "GROUP_READERS=${GROUP_READERS}"
echo "GROUP_WRITERS=${GROUP_WRITERS}"
```

## 6. Create Three Users (admin / reader / writer)

```bash
# Helper: create user + set password + join group
create_user() {
 local USERNAME=$1
 local GROUP_ID=$2
 curl -s -X POST -H "Authorization: Bearer ${KC_TOKEN}" -H "Content-Type: application/json" \
 "http://${KC_IP}:8080/admin/realms/agentregistry-enterprise/users" \
 -d "{\"username\":\"${USERNAME}\",\"enabled\":true,\"email\":\"${USERNAME}@example.com\",\"emailVerified\":true,\"firstName\":\"${USERNAME}\",\"lastName\":\"user\"}"

 local USER_ID=$(curl -s -H "Authorization: Bearer ${KC_TOKEN}" \
 "http://${KC_IP}:8080/admin/realms/agentregistry-enterprise/users?username=${USERNAME}" \
 | jq -r '.[0].id')

 curl -s -X PUT -H "Authorization: Bearer ${KC_TOKEN}" -H "Content-Type: application/json" \
 "http://${KC_IP}:8080/admin/realms/agentregistry-enterprise/users/${USER_ID}/reset-password" \
 -d "{\"type\":\"password\",\"value\":\"${USERNAME}\",\"temporary\":false}"

 curl -s -X PUT -H "Authorization: Bearer ${KC_TOKEN}" \
 "http://${KC_IP}:8080/admin/realms/agentregistry-enterprise/users/${USER_ID}/groups/${GROUP_ID}"
}

create_user admin "${GROUP_ADMINS}"
create_user reader "${GROUP_READERS}"
create_user writer "${GROUP_WRITERS}"
```

| Username | Password | Group |
|---|---|---|
| admin | admin | are-admins |
| reader | reader | are-readers |
| writer | writer | are-writers |

> Password = username is for the demo. **Don't do this in production.**

## 7. Create the OIDC Clients

Two clients:

- **`are-backend`** - confidential, for the agentregistry server to validate tokens
- **`are-cli`** - public, for `arctl user login` via the OAuth 2.0 Device Authorization Grant

```bash
# are-backend (confidential)
curl -s -X POST -H "Authorization: Bearer ${KC_TOKEN}" -H "Content-Type: application/json" \
 "http://${KC_IP}:8080/admin/realms/agentregistry-enterprise/clients" \
 -d '{
 "clientId":"are-backend",
 "enabled":true,
 "publicClient":false,
 "standardFlowEnabled":true,
 "directAccessGrantsEnabled":true,
 "serviceAccountsEnabled":true,
 "redirectUris":["*"],
 "webOrigins":["*"]
 }'

# are-cli (public + device-code grant + no PKCE)
curl -s -X POST -H "Authorization: Bearer ${KC_TOKEN}" -H "Content-Type: application/json" \
 "http://${KC_IP}:8080/admin/realms/agentregistry-enterprise/clients" \
 -d '{
 "clientId":"are-cli",
 "enabled":true,
 "publicClient":true,
 "standardFlowEnabled":true,
 "directAccessGrantsEnabled":true,
 "redirectUris":["*"],
 "webOrigins":["*"],
 "attributes":{
 "oauth2.device.authorization.grant.enabled":"true",
 "pkce.code.challenge.method":""
 }
 }'
```

> `pkce.code.challenge.method` must be **empty** on `are-cli`. The OAuth 2.0 Device Authorization Grant doesn't send a PKCE challenge - if PKCE is required, `arctl user login` will fail with `Missing parameter: code_challenge_method`.

## 8. Add a Groups Claim Mapper to `are-backend`

By default Keycloak doesn't put group memberships in tokens. Add a mapper that surfaces them in a `groups` claim on both access and ID tokens:

```bash
ARE_BACKEND_ID=$(curl -s -H "Authorization: Bearer ${KC_TOKEN}" \
 "http://${KC_IP}:8080/admin/realms/agentregistry-enterprise/clients?clientId=are-backend" \
 | jq -r '.[0].id')

curl -s -X POST -H "Authorization: Bearer ${KC_TOKEN}" -H "Content-Type: application/json" \
 "http://${KC_IP}:8080/admin/realms/agentregistry-enterprise/clients/${ARE_BACKEND_ID}/protocol-mappers/models" \
 -d '{
 "name":"groups",
 "protocol":"openid-connect",
 "protocolMapper":"oidc-group-membership-mapper",
 "config":{
 "claim.name":"groups",
 "full.path":"false",
 "id.token.claim":"true",
 "access.token.claim":"true",
 "userinfo.token.claim":"true"
 }
 }'
```

## 9. Grab the `are-backend` Client Secret

```bash
export BACKEND_CLIENT_SECRET=$(curl -s -X POST -H "Authorization: Bearer ${KC_TOKEN}" \
 "http://${KC_IP}:8080/admin/realms/agentregistry-enterprise/clients/${ARE_BACKEND_ID}/client-secret" \
 | jq -r '.value')
echo "BACKEND_CLIENT_SECRET=${BACKEND_CLIENT_SECRET}"
```

## 10. Export Everything 003 Needs

[003 - Install Components](003-install-components.md) consumes these env vars. Keep this shell open or persist them somewhere (`.envrc`, your shell's `~/.zprofile`, etc.) before continuing:

```bash
export OIDC_PROVIDER=keycloak
export OIDC_ISSUER="http://${KC_IP}:8080/realms/agentregistry-enterprise"
export OIDC_BACKEND=are-backend
export OIDC_PUBLIC_CLIENT=are-cli # used by arctl user login
export ARE_CLI_CLIENT_ID=are-cli
export BACKEND_CLIENT_SECRET="${BACKEND_CLIENT_SECRET}"
export GROUP_ADMINS="${GROUP_ADMINS}"
export GROUP_READERS="${GROUP_READERS}"
export GROUP_WRITERS="${GROUP_WRITERS}"

# Print everything so you can paste into a notes file:
for V in OIDC_PROVIDER OIDC_ISSUER OIDC_BACKEND OIDC_PUBLIC_CLIENT ARE_CLI_CLIENT_ID \
 BACKEND_CLIENT_SECRET GROUP_ADMINS GROUP_READERS GROUP_WRITERS; do
 printf '%-25s %s\n' "${V}=" "${!V}"
done
```

## Verify the Realm

Decode a real token to confirm the `groups` claim shows up:

```bash
curl -s -X POST "http://${KC_IP}:8080/realms/agentregistry-enterprise/protocol/openid-connect/token" \
 -d "grant_type=password" \
 -d "client_id=are-backend" \
 -d "client_secret=${BACKEND_CLIENT_SECRET}" \
 -d "username=admin" \
 -d "password=admin" \
 -d "scope=openid" \
 | jq -r '.access_token' \
 | cut -d. -f2 | base64 -d 2>/dev/null | jq '{preferred_username, groups, iss, aud}'
```

Expected:

```json
{
 "preferred_username": "admin",
 "groups": ["/are-admins"],
 "iss": "http://<KC_IP>:8080/realms/agentregistry-enterprise",
 "aud": ["account"]
}
```

> Keycloak prefixes group names with `/` (the realm path). [050 access policies](050-access-policies.md) shows how to write policy that matches against the GUID variants you exported in step 5 - the GUIDs are stable and don't have the `/` prefix.

## Cleanup

To remove just Keycloak (you'd do this if you want to switch to the Entra path in [002b](002b-setup-oidc-entra.md) instead, or if you're done with the workshop):

```bash
kubectl delete namespace keycloak
unset OIDC_PROVIDER OIDC_ISSUER OIDC_BACKEND OIDC_PUBLIC_CLIENT ARE_CLI_CLIENT_ID \
 BACKEND_CLIENT_SECRET GROUP_ADMINS GROUP_READERS GROUP_WRITERS KC_TOKEN KC_IP ARE_BACKEND_ID
```

Full workshop teardown is in [099 - Cleanup](099-cleanup.md).

## Next

- [003 - Install Components](003-install-components.md) (agentregistry + kagent + Enterprise Agentgateway)
