# Setup OIDC: Keycloak (In-Cluster)

The second mandatory setup lab (Keycloak path). Stands up Keycloak in-cluster, configures the `agentregistry-enterprise` realm with three users (`admin` / `reader` / `writer`), creates the OIDC clients agentregistry needs, and exports the variables [003 - Install Components](003-install-components.md) will consume.

> **Pick one OIDC path.** This lab is the **Keycloak** path. If you'd rather use Microsoft Entra ID, go to [002b - Setup OIDC: Entra ID](002b-setup-oidc-entra.md) instead. Don't run both - they're alternatives, not additive.

## Lab Objectives

- Deploy Keycloak `quay.io/keycloak/keycloak:26.0` in-cluster
- Get a `LoadBalancer` IP and configure Keycloak's hostname
- Run a single script that creates the `agentregistry-enterprise` realm, three groups, three users, two OIDC clients, and the `groups` claim mapper
- Source the exported values into your shell ready for [003](003-install-components.md)

## Prerequisites

- [001 - Baseline Setup](001-baseline-setup.md) completed
- `kubectl`, `curl`, `jq`

## 1. Create the Keycloak Namespace

```bash
kubectl create namespace keycloak
```

## 2. Deploy Keycloak

The Deployment + Service manifest is at [`assets/keycloak/keycloak-deployment.yaml`](assets/keycloak/keycloak-deployment.yaml). Apply it:

```bash
kubectl apply -n keycloak -f assets/keycloak/keycloak-deployment.yaml
```

What's in the manifest:

- A `Deployment` running `quay.io/keycloak/keycloak:26.0` in `start-dev` mode (HTTP only, relaxed hostname checks) with admin credentials `admin` / `admin123`
- A `Service` of type `LoadBalancer` on port `8080`

> The default admin password `admin123` is for a POC. Rotate it (`kubectl set env deployment/keycloak -n keycloak KEYCLOAK_ADMIN_PASSWORD=<new>`) if your security team requires it.

## 3. Wait for the External IP

```bash
kubectl get svc keycloak -n keycloak -w
# Wait for EXTERNAL-IP to be set, then Ctrl-C

export KC_IP=$(kubectl get svc keycloak -n keycloak \
  -o jsonpath='{.status.loadBalancer.ingress[0].ip}{.status.loadBalancer.ingress[0].hostname}')
echo "Keycloak admin: http://${KC_IP}:8080  (admin / admin123)"
```

Pin Keycloak's hostname so its issuer URL matches what your tokens will carry:

```bash
kubectl set env deployment/keycloak -n keycloak \
  KC_HOSTNAME_URL=http://${KC_IP}:8080 \
  KC_HOSTNAME_ADMIN_URL=http://${KC_IP}:8080
kubectl rollout status deployment/keycloak -n keycloak
```

## 4. Configure the Realm

Everything that happens inside Keycloak - realm creation, groups, users, OIDC clients, the `groups` claim mapper, and pulling the `are-backend` client secret - runs from a single script at [`assets/keycloak/setup-realm.sh`](assets/keycloak/setup-realm.sh).

The script fetches a fresh admin token on every API call, so token expiry (Keycloak's default is 60 seconds for `admin-cli`) doesn't matter even on a slow connection. It's idempotent - safe to re-run if anything fails mid-stream.

What it does:

| Step | Result |
|---|---|
| 1 | Set `sslRequired=none` on the `master` realm (HTTP-only POC) |
| 2 | Create the `agentregistry-enterprise` realm |
| 3 | Create three groups: `are-admins`, `are-readers`, `are-writers`; capture each GUID |
| 4 | Create three users (`admin`, `reader`, `writer`) with password = username; add each to its group |
| 5 | Create two OIDC clients: `are-backend` (confidential) and `are-cli` (public + device-code grant, no PKCE) |
| 6 | Add a `groups` claim mapper on `are-backend` so group memberships show up in tokens |
| 7 | Pull the `are-backend` client secret |
| 8 | Write all values [003](003-install-components.md) consumes to `~/.are-keycloak-env` |

Run it:

```bash
./assets/keycloak/setup-realm.sh
```

Expected output (truncated):

```
==> Waiting for Keycloak at http://<KC_IP>:8080 ...
    Keycloak is up.
==> Configuring master realm (sslRequired=none)
==> Creating realm agentregistry-enterprise
==> Creating groups
    are-admins: created
    are-readers: created
    are-writers: created
==> Creating users
    admin: created (...)
    reader: created (...)
    writer: created (...)
==> Creating OIDC clients
    are-backend: created
    are-cli: created
==> Adding 'groups' claim mapper to are-backend
    mapper created
==> Fetching are-backend client secret
==> Writing /Users/<you>/.are-keycloak-env
==> Done. Source the env file:
        source /Users/<you>/.are-keycloak-env
```

## 5. Source the Exported Values

[003 - Install Components](003-install-components.md) reads these from your shell:

```bash
source ~/.are-keycloak-env
```

Confirm:

```bash
for V in OIDC_PROVIDER OIDC_ISSUER OIDC_BACKEND OIDC_PUBLIC_CLIENT ARE_CLI_CLIENT_ID \
         BACKEND_CLIENT_SECRET GROUP_ADMINS GROUP_READERS GROUP_WRITERS; do
  eval "VALUE=\${${V}:-}"
  printf '%-25s %s\n' "${V}=" "${VALUE}"
done
```

Every line should print a value. If any are empty, re-source the env file or re-run the script.

| Username | Password | Group |
|---|---|---|
| admin | admin | are-admins |
| reader | reader | are-readers |
| writer | writer | are-writers |

> Password = username is for the demo. **Don't do this in production.**

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

> Keycloak prefixes group names with `/` (the realm path). [050 access policies](050-access-policies.md) shows how to write policy that matches against the GUID variants (the GUIDs are stable and don't have the `/` prefix).

## Cleanup

To remove just Keycloak (you'd do this if you want to switch to the Entra path in [002b](002b-setup-oidc-entra.md) instead, or if you're done with the workshop):

```bash
kubectl delete namespace keycloak
rm -f ~/.are-keycloak-env
unset OIDC_PROVIDER OIDC_ISSUER OIDC_BACKEND OIDC_PUBLIC_CLIENT ARE_CLI_CLIENT_ID \
      BACKEND_CLIENT_SECRET GROUP_ADMINS GROUP_READERS GROUP_WRITERS KC_IP
```

Full workshop teardown is in [099 - Cleanup](099-cleanup.md).

## Next

- [003 - Install Components](003-install-components.md) (agentregistry + Enterprise Agentgateway)
