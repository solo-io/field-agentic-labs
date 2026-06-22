# Setup OIDC: Microsoft Entra ID

The second mandatory setup lab (Entra ID path). Registers the Entra app registrations + security groups agentregistry Enterprise needs, and exports the variables [003 - Install Components](003-install-components.md) will consume.

> **Pick one OIDC path.** This is the **Entra ID** path. If you'd rather run an in-cluster Keycloak (no cloud account needed), go to [002a - Setup OIDC: Keycloak](002a-setup-oidc-keycloak.md) instead. Don't run both - they're alternatives.

## Lab Objectives

- Create three Entra app registrations: `are-backend` (confidential), `are-cli` (public), `are-ui` (SPA)
- Expose a delegated API scope `api://<are-backend>/agentregistry`
- Create three security groups (`are-admins`, `are-readers`, `are-writers`) and enable the `groups` claim on all three app registrations
- Export the values [003](003-install-components.md) needs (`OIDC_ISSUER`, `OIDC_BACKEND`, `BACKEND_CLIENT_SECRET`, `GROUP_ADMINS`, `GROUP_READERS`, `GROUP_WRITERS`, `ARE_CLI_CLIENT_ID`, `ARE_UI_CLIENT_ID`, `TENANT_ID`)

## Prerequisites

- [001 - Baseline Setup](001-baseline-setup.md) completed
- `az` CLI installed and authenticated (`az login`)
- A Microsoft Entra ID tenant where you can create app registrations and security groups

## 1. Collect the Tenant ID

```bash
export TENANT_ID=$(az account show --query tenantId -o tsv)
echo "TENANT_ID=${TENANT_ID}"
```

## 2. Register the Backend App (`are-backend`)

Confidential client. Agentregistry's server uses it to validate tokens.

```bash
export ARE_BACKEND_CLIENT_ID=$(az ad app create \
 --display-name "are-backend" \
 --sign-in-audience "AzureADMyOrg" \
 --query appId -o tsv)
echo "ARE_BACKEND_CLIENT_ID=${ARE_BACKEND_CLIENT_ID}"

export BACKEND_CLIENT_SECRET=$(az ad app credential reset \
 --id "${ARE_BACKEND_CLIENT_ID}" \
 --display-name "agentregistry-enterprise" \
 --years 1 \
 --query password -o tsv)
echo "BACKEND_CLIENT_SECRET=${BACKEND_CLIENT_SECRET}"

# Set the Application ID URI so the delegated scope below has a stable name
az ad app update --id "${ARE_BACKEND_CLIENT_ID}" \
 --identifier-uris "api://${ARE_BACKEND_CLIENT_ID}"

# Expose the agentregistry delegated scope
SCOPE_ID=$(python3 -c "import uuid; print(uuid.uuid4())")
az ad app update --id "${ARE_BACKEND_CLIENT_ID}" \
 --set "api={\"oauth2PermissionScopes\":[{\"id\":\"${SCOPE_ID}\",\"adminConsentDisplayName\":\"Access agentregistry Enterprise\",\"adminConsentDescription\":\"Allows the app to access agentregistry Enterprise on behalf of the signed-in user\",\"isEnabled\":true,\"type\":\"User\",\"userConsentDisplayName\":\"Access agentregistry Enterprise\",\"userConsentDescription\":\"Allows the app to access agentregistry Enterprise on behalf of the signed-in user\",\"value\":\"agentregistry\"}]}"

# Create the service principal so admin consent works
az ad sp create --id "${ARE_BACKEND_CLIENT_ID}" 2>/dev/null || true

# Use the v2.0 access-token format - matches the v2.0 OIDC issuer URL the chart uses
az ad app update --id "${ARE_BACKEND_CLIENT_ID}" \
 --set "api.requestedAccessTokenVersion=2"
```

## 3. Register the CLI App (`are-cli`)

Public client. `arctl user login` uses this with the OAuth 2.0 Device Authorization Grant.

```bash
export ARE_CLI_CLIENT_ID=$(az ad app create \
 --display-name "are-cli" \
 --sign-in-audience "AzureADMyOrg" \
 --public-client-redirect-uris "http://localhost" \
 --is-fallback-public-client true \
 --query appId -o tsv)
echo "ARE_CLI_CLIENT_ID=${ARE_CLI_CLIENT_ID}"

# Grant are-cli access to the are-backend delegated scope
az ad app permission add \
 --id "${ARE_CLI_CLIENT_ID}" \
 --api "${ARE_BACKEND_CLIENT_ID}" \
 --api-permissions "${SCOPE_ID}=Scope"

az ad app permission admin-consent --id "${ARE_CLI_CLIENT_ID}"
```

## 4. Register the SPA App (`are-ui`)

Browser SPA for the agentregistry UI. You'll add the HTTPS callback URI in [003](003-install-components.md) once the UI has a reachable address - Entra requires HTTPS for non-localhost SPA redirects.

```bash
export ARE_UI_CLIENT_ID=$(az ad app create \
 --display-name "are-ui" \
 --sign-in-audience "AzureADMyOrg" \
 --query appId -o tsv)
echo "ARE_UI_CLIENT_ID=${ARE_UI_CLIENT_ID}"

az ad app permission add \
 --id "${ARE_UI_CLIENT_ID}" \
 --api "${ARE_BACKEND_CLIENT_ID}" \
 --api-permissions "${SCOPE_ID}=Scope"

az ad app permission admin-consent --id "${ARE_UI_CLIENT_ID}"
```

## 5. Create the Three Security Groups

```bash
export GROUP_ADMINS=$(az ad group create \
 --display-name "are-admins" --mail-nickname "are-admins" \
 --description "agentregistry Enterprise superuser access" --query id -o tsv)

export GROUP_READERS=$(az ad group create \
 --display-name "are-readers" --mail-nickname "are-readers" \
 --description "agentregistry Enterprise read-only access" --query id -o tsv)

export GROUP_WRITERS=$(az ad group create \
 --display-name "are-writers" --mail-nickname "are-writers" \
 --description "agentregistry Enterprise read + write access" --query id -o tsv)

echo "GROUP_ADMINS=${GROUP_ADMINS}"
echo "GROUP_READERS=${GROUP_READERS}"
echo "GROUP_WRITERS=${GROUP_WRITERS}"
```

Add yourself to `are-admins`:

```bash
MY_USER_ID=$(az ad signed-in-user show --query id -o tsv)
az ad group member add --group "${GROUP_ADMINS}" --member-id "${MY_USER_ID}"
```

Add other users by UPN:

```bash
USER_ID=$(az ad user show --id "writer@example.com" --query id -o tsv)
az ad group member add --group "${GROUP_WRITERS}" --member-id "${USER_ID}"
```

## 6. Enable the `groups` Claim on All Three Apps

Entra doesn't put group memberships in tokens by default. Turn it on for `are-backend`, `are-cli`, and `are-ui`:

```bash
for APP_ID in "${ARE_BACKEND_CLIENT_ID}" "${ARE_CLI_CLIENT_ID}" "${ARE_UI_CLIENT_ID}"; do
 az ad app update --id "${APP_ID}" \
 --set "groupMembershipClaims=\"SecurityGroup\"" \
 --set "optionalClaims={\"accessToken\":[{\"name\":\"groups\",\"source\":null,\"essential\":false,\"additionalProperties\":[]}],\"idToken\":[{\"name\":\"groups\",\"source\":null,\"essential\":false,\"additionalProperties\":[]}]}"
done
```

> **Entra emits group object IDs (GUIDs), not display names.** [050 access policies](050-access-policies.md) references the GUIDs you exported in step 5 as policy principals.

> **Groups overage:** if a user belongs to more than ~200 groups, Entra omits the `groups` claim entirely and returns `_claim_names` / `_claim_sources` pointing to Microsoft Graph. Agentregistry does not resolve the Graph overage endpoint. Limit group membership in this tenant, or switch to **app roles** instead of groups (the chart accepts `oidc.roleClaim: roles` - same plumbing).

## 7. Export Everything 003 Needs

[003 - Install Components](003-install-components.md) consumes these env vars. Persist them now:

```bash
export OIDC_PROVIDER=entra
export OIDC_ISSUER="https://login.microsoftonline.com/${TENANT_ID}/v2.0"
export OIDC_BACKEND="${ARE_BACKEND_CLIENT_ID}"
export OIDC_PUBLIC_CLIENT="${ARE_UI_CLIENT_ID}"
# (BACKEND_CLIENT_SECRET, ARE_CLI_CLIENT_ID, ARE_UI_CLIENT_ID, GROUP_* already exported above)

for V in OIDC_PROVIDER OIDC_ISSUER OIDC_BACKEND OIDC_PUBLIC_CLIENT \
 TENANT_ID ARE_BACKEND_CLIENT_ID ARE_CLI_CLIENT_ID ARE_UI_CLIENT_ID \
 BACKEND_CLIENT_SECRET SCOPE_ID \
 GROUP_ADMINS GROUP_READERS GROUP_WRITERS; do
 printf '%-25s %s\n' "${V}=" "${!V}"
done
```

## Verify the Setup

Run the manual device-code flow once to confirm the apps + scope + permissions line up before you install anything:

```bash
DEVICE=$(curl -s -X POST \
 "https://login.microsoftonline.com/${TENANT_ID}/oauth2/v2.0/devicecode" \
 -H "Content-Type: application/x-www-form-urlencoded" \
 -d "client_id=${ARE_CLI_CLIENT_ID}&scope=openid+api://${ARE_BACKEND_CLIENT_ID}/agentregistry")

echo "${DEVICE}" | jq
```

Follow the printed URL + code in a browser. After you sign in, poll for the token:

```bash
DEVICE_CODE=$(echo "${DEVICE}" | jq -r .device_code)

while true; do
 RESP=$(curl -s -X POST \
 "https://login.microsoftonline.com/${TENANT_ID}/oauth2/v2.0/token" \
 -H "Content-Type: application/x-www-form-urlencoded" \
 -d "grant_type=urn:ietf:params:oauth:grant-type:device_code&client_id=${ARE_CLI_CLIENT_ID}&device_code=${DEVICE_CODE}")
 ERR=$(echo "${RESP}" | jq -r '.error // "none"')
 if [ "${ERR}" = "none" ]; then
 echo "${RESP}" | jq -r '.access_token' | cut -d. -f2 | base64 -d 2>/dev/null | jq '{preferred_username,groups,iss,aud}'
 break
 elif [ "${ERR}" = "authorization_pending" ]; then
 sleep 5
 else
 echo "${RESP}" | jq; break
 fi
done
```

Expected - a non-empty `groups` array containing the GUID of `are-admins` (if you added yourself to it), `iss` matching `https://login.microsoftonline.com/<TENANT_ID>/v2.0`, and `aud` matching `${ARE_BACKEND_CLIENT_ID}`.

## Troubleshooting

| Error | Fix |
|---|---|
| `AADSTS900144: request body must contain 'scope'` | The CLI didn't pass a `scope`. The manual device-code script above always passes one - use it. |
| `AADSTS7000218: client_assertion / client_secret required` | `are-cli` is misconfigured as confidential. Run `az ad app update --id "${ARE_CLI_CLIENT_ID}" --is-fallback-public-client true`. |
| `AADSTS65001: not consented` | Admin consent missing. `az ad app permission admin-consent --id "${ARE_CLI_CLIENT_ID}"`. |
| `groups` claim missing from the token | The token-config step 6 didn't apply. Re-run; confirm with `az ad app show --id "${ARE_BACKEND_CLIENT_ID}" --query 'groupMembershipClaims'`. |
| Token shows `_claim_names` instead of `groups` | Groups overage - user is in too many groups. Use Entra app roles instead. |

## Cleanup

Tear down just the Entra app registrations + groups (you'd do this to switch to the Keycloak path in [002a](002a-setup-oidc-keycloak.md), or when you're done with the workshop):

```bash
az ad app delete --id "${ARE_BACKEND_CLIENT_ID}"
az ad app delete --id "${ARE_CLI_CLIENT_ID}"
az ad app delete --id "${ARE_UI_CLIENT_ID}"

az ad group delete --group "${GROUP_ADMINS}"
az ad group delete --group "${GROUP_READERS}"
az ad group delete --group "${GROUP_WRITERS}"

unset OIDC_PROVIDER OIDC_ISSUER OIDC_BACKEND OIDC_PUBLIC_CLIENT \
 TENANT_ID ARE_BACKEND_CLIENT_ID ARE_CLI_CLIENT_ID ARE_UI_CLIENT_ID \
 BACKEND_CLIENT_SECRET SCOPE_ID \
 GROUP_ADMINS GROUP_READERS GROUP_WRITERS MY_USER_ID
```

Full workshop teardown is in [099 - Cleanup](099-cleanup.md).

## Next

- [003 - Install Components](003-install-components.md) (agentregistry + Enterprise Agentgateway)
