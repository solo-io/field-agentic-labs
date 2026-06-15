# Authenticate `arctl`

This lab points `arctl` at your AgentRegistry Enterprise install and walks through the two authentication flows: Keycloak (built-in device-code via `arctl user login`) and Entra (manual device-code flow with explicit scope).

## Lab Objectives

- Set `ARCTL_API_BASE_URL` to your server
- Verify the CLI can talk to the server (`arctl version --json` returns both client + server versions)
- Authenticate with the appropriate device-code flow
- Smoke test with `arctl get providers` / `arctl get runtimes`

## Prerequisites

- [001 — arctl installed](001-install-arctl.md)
- [030 — AgentRegistry Enterprise installed](030-install-agentregistry-helm.md)
- One of [020 (Entra)](020-setup-entra.md) or [021 (Keycloak)](021-setup-keycloak.md)

## 1. Point `arctl` at the Server

```bash
export AR_IP=$(kubectl get svc agentregistry-enterprise -n agentregistry-system \
  -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
export ARCTL_API_BASE_URL=http://$AR_IP:8080
arctl version --json
```

Both `arctl_version` and `server_version` should be populated.

> The CLI does **not** support `--insecure-skip-verify` for registry API calls, so use the **direct HTTP Service** (`http://$AR_IP:8080`), not the self-signed HTTPS Gateway from [030](030-install-agentregistry-helm.md#5-expose-the-ui-over-https-for-entra-spa-login). The HTTPS Gateway is for browser SPA login only.

For a private-cluster install where the Service is `ClusterIP`, point at your Istio Gateway address from [035](035-private-cluster-istio-routing.md) instead, or `kubectl port-forward` and use `http://localhost:8080`.

## 2. Keycloak — `arctl user login`

The built-in device-code flow works directly against Keycloak as long as `are-cli` has the OAuth 2.0 Device Authorization Grant enabled and PKCE cleared (see [021](021-setup-keycloak.md#3-make-are-cli-browser--device-code-friendly)).

```bash
arctl user login \
  --oidc-issuer-url "http://$KC_IP:8080/realms/kagent-dev" \
  --oidc-client-id are-cli
```

A browser opens to Keycloak; sign in. The token is stored in your OS keychain and refreshed automatically.

```bash
arctl get providers
arctl get runtimes
```

## 3. Entra — Manual Device-Code Login

The current `arctl user login` does **not** pass a `scope` parameter, which Entra requires (`AADSTS900144`). Until that ships, do the device-code flow by hand and export the access token as `ARCTL_API_TOKEN`.

The three Entra values from [020](020-setup-entra.md) must be set:

```bash
echo "TENANT_ID:             $TENANT_ID"
echo "ARE_CLI_CLIENT_ID:     $ARE_CLI_CLIENT_ID"
echo "ARE_BACKEND_CLIENT_ID: $ARE_BACKEND_CLIENT_ID"
```

### 3a. Kick Off the Device Code Flow

```bash
DEVICE_RESPONSE=$(curl -s -X POST \
  "https://login.microsoftonline.com/$TENANT_ID/oauth2/v2.0/devicecode" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "client_id=$ARE_CLI_CLIENT_ID&scope=openid+api://$ARE_BACKEND_CLIENT_ID/agentregistry")

echo "$DEVICE_RESPONSE" | python3 -m json.tool
```

This prints something like:

```
To sign in, use a web browser to open the page https://microsoft.com/devicelogin
and enter the code XXXXXXX to authenticate.
```

Open the URL, enter the code, sign in.

### 3b. Poll for the Token

```bash
DEVICE_CODE=$(echo "$DEVICE_RESPONSE" \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['device_code'])")

while true; do
  TOKEN_RESPONSE=$(curl -s -X POST \
    "https://login.microsoftonline.com/$TENANT_ID/oauth2/v2.0/token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "grant_type=urn:ietf:params:oauth:grant-type:device_code&client_id=$ARE_CLI_CLIENT_ID&device_code=$DEVICE_CODE")

  ERROR=$(echo "$TOKEN_RESPONSE" \
    | python3 -c "import sys,json; print(json.load(sys.stdin).get('error','none'))")

  if [ "$ERROR" = "none" ]; then
    export ARCTL_API_TOKEN=$(echo "$TOKEN_RESPONSE" \
      | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")
    echo "Token obtained"
    break
  elif [ "$ERROR" = "authorization_pending" ]; then
    sleep 5
  else
    echo "Error: $ERROR"
    echo "$TOKEN_RESPONSE" | python3 -m json.tool
    break
  fi
done
```

### 3c. Verify

```bash
arctl get providers

# Or pass the token explicitly:
arctl get providers --registry-token "$ARCTL_API_TOKEN"
```

You can also see your mapped roles:

```bash
arctl user whoami
```

The roles should contain the **object IDs** of the security groups you belong to. If they don't, your token doesn't carry the `groups` claim — go back to [020 step 6](020-setup-entra.md#6-enable-the-groups-claim-on-all-three-apps).

## 4. Quick-Recipe Variables

Once authenticated, all the later labs assume:

```bash
export PATH="$HOME/.arctl/bin:$PATH"
export ARCTL_API_BASE_URL="http://$AR_IP:8080"
# Entra path:
export ARCTL_API_TOKEN="<bearer token from step 3>"
```

## Troubleshooting

### `AADSTS900144` — request body must contain `scope`

You ran `arctl user login` against Entra. Use the manual device-code flow above.

### `AADSTS7000218` — `client_assertion` or `client_secret` required

`are-cli` is registered as a confidential client. Make it public:

```bash
az ad app update --id "$ARE_CLI_CLIENT_ID" --is-fallback-public-client true
```

### `The logged in user does not have any mapped roles`

Decode your token at [jwt.ms](https://jwt.ms) / [jwt.io](https://jwt.io):

- Entra: the `groups` claim should contain GUIDs that match `oidc.superuserRole` and your AccessPolicy principals. If `_claim_names`/`_claim_sources` is there instead, you hit the groups overage limit — switch to app roles.
- Keycloak: the actual claim name may be `Groups` (capital), `groups`, or `realm_access.roles`. Update `oidc.roleClaim` in your Helm values to match and `helm upgrade`.

### CLI hits a different `arctl` than expected

```bash
which -a arctl
```

The enterprise binary at `$HOME/.arctl/bin/arctl` must come first. The OSS one (`/usr/local/bin/arctl`) is missing `user login`, `apply`, `provider`, etc.

## Next

- [050 — AWS Bedrock AgentCore Provider](050-aws-provider.md), or
- [051 — kagent Runtime](051-kagent-provider.md)
