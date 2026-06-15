# Configure Microsoft Entra ID (Azure AD) OIDC

AgentRegistry Enterprise authenticates users via OIDC and maps a token claim (`groups` or `roles`) to RBAC principals consumed by [AccessPolicy](080-access-policies.md). With Entra ID, the recommended setup is **security groups** with the `groups` claim — Entra emits the group **object IDs (GUIDs)**, which become the principals you reference in policy.

This lab creates three Entra app registrations (`are-backend`, `are-cli`, `are-ui`), three security groups (`are-admins`, `are-readers`, `are-writers`), and exports the values you will plug into the Helm chart in [030](030-install-agentregistry-helm.md).

## Lab Objectives

- Register three Entra ID apps (confidential backend, public CLI, public SPA)
- Expose an API scope (`api://<are-backend>/agentregistry`)
- Create security groups and enable the `groups` claim on all three apps
- Export the variables consumed by [030](030-install-agentregistry-helm.md)

## Prerequisites

- `az` CLI installed and authenticated (`az login`)
- A Microsoft Entra ID tenant with permissions to create app registrations and groups

## 1. Collect the Tenant ID

**Portal**: [Microsoft Entra ID](https://portal.azure.com) > **Overview** > copy the **Tenant ID**.

**CLI**:

```bash
export TENANT_ID=$(az account show --query tenantId -o tsv)
echo "Tenant ID: $TENANT_ID"
```

## 2. Register the Backend App (`are-backend`)

This is the confidential client the AgentRegistry server uses to validate tokens.

**Portal**:
1. **App registrations** > **New registration**
2. Name: `are-backend`, Single tenant, no redirect URI > **Register**
3. Copy the **Application (client) ID**
4. **Certificates & secrets** > **New client secret** > copy the **Value**
5. **Expose an API** > Set Application ID URI > **Add a scope** named `agentregistry` (Admins and users, enabled)

**CLI**:

```bash
ARE_BACKEND_CLIENT_ID=$(az ad app create \
  --display-name "are-backend" \
  --sign-in-audience "AzureADMyOrg" \
  --query appId -o tsv)
echo "ARE_BACKEND_CLIENT_ID: $ARE_BACKEND_CLIENT_ID"

ARE_BACKEND_CLIENT_SECRET=$(az ad app credential reset \
  --id "$ARE_BACKEND_CLIENT_ID" \
  --display-name "agentregistry-enterprise" \
  --years 1 \
  --query password -o tsv)
echo "ARE_BACKEND_CLIENT_SECRET: $ARE_BACKEND_CLIENT_SECRET"

az ad app update --id "$ARE_BACKEND_CLIENT_ID" \
  --identifier-uris "api://$ARE_BACKEND_CLIENT_ID"

SCOPE_ID=$(python3 -c "import uuid; print(uuid.uuid4())")
az ad app update --id "$ARE_BACKEND_CLIENT_ID" \
  --set "api={\"oauth2PermissionScopes\":[{\"id\":\"$SCOPE_ID\",\"adminConsentDisplayName\":\"Access AgentRegistry Enterprise\",\"adminConsentDescription\":\"Allows the app to access AgentRegistry Enterprise on behalf of the signed-in user\",\"isEnabled\":true,\"type\":\"User\",\"userConsentDisplayName\":\"Access AgentRegistry Enterprise\",\"userConsentDescription\":\"Allows the app to access AgentRegistry Enterprise on behalf of the signed-in user\",\"value\":\"agentregistry\"}]}"

az ad sp create --id "$ARE_BACKEND_CLIENT_ID" 2>/dev/null || true

# v2.0 issuer
az ad app update --id "$ARE_BACKEND_CLIENT_ID" \
  --set "api.requestedAccessTokenVersion=2"
```

## 3. Register the CLI App (`are-cli`)

Public client. Uses the OAuth 2.0 device authorization grant so you can authenticate from a terminal.

**Portal**:
1. **New registration** > Name: `are-cli`, Single tenant, Redirect URI: Public client `http://localhost` > **Register**
2. **Authentication** > **Allow public client flows** > **Yes** > **Save**
3. **API permissions** > **Add a permission** > **My APIs** > `are-backend` > select `agentregistry` > **Add permissions**
4. **Grant admin consent**

**CLI**:

```bash
ARE_CLI_CLIENT_ID=$(az ad app create \
  --display-name "are-cli" \
  --sign-in-audience "AzureADMyOrg" \
  --public-client-redirect-uris "http://localhost" \
  --is-fallback-public-client true \
  --query appId -o tsv)
echo "ARE_CLI_CLIENT_ID: $ARE_CLI_CLIENT_ID"

az ad app permission add \
  --id "$ARE_CLI_CLIENT_ID" \
  --api "$ARE_BACKEND_CLIENT_ID" \
  --api-permissions "$SCOPE_ID=Scope"

az ad app permission admin-consent --id "$ARE_CLI_CLIENT_ID"
```

## 4. Register the UI App (`are-ui`)

SPA client for the AgentRegistry web UI. The redirect URI is set **after** the chart is installed and the HTTPS Gateway has an external IP (see [030](030-install-agentregistry-helm.md#expose-the-ui-over-https-for-entra-spa-login)). Entra requires HTTPS for SPA redirect URIs on non-localhost addresses.

**Portal**:
1. **New registration** > Name: `are-ui`, Single tenant, Redirect URI: SPA (blank for now) > **Register**
2. **API permissions** > **My APIs** > `are-backend` > `agentregistry` > **Add permissions**
3. **Grant admin consent**

**CLI**:

```bash
ARE_UI_CLIENT_ID=$(az ad app create \
  --display-name "are-ui" \
  --sign-in-audience "AzureADMyOrg" \
  --query appId -o tsv)
echo "ARE_UI_CLIENT_ID: $ARE_UI_CLIENT_ID"

az ad app permission add \
  --id "$ARE_UI_CLIENT_ID" \
  --api "$ARE_BACKEND_CLIENT_ID" \
  --api-permissions "$SCOPE_ID=Scope"

az ad app permission admin-consent --id "$ARE_UI_CLIENT_ID"
```

## 5. Create Security Groups

```bash
MY_USER_ID=$(az ad signed-in-user show --query id -o tsv)

GROUP_ADMINS=$(az ad group create \
  --display-name "are-admins" --mail-nickname "are-admins" \
  --description "AgentRegistry Enterprise superuser access" \
  --query id -o tsv)

GROUP_READERS=$(az ad group create \
  --display-name "are-readers" --mail-nickname "are-readers" \
  --description "AgentRegistry Enterprise read-only access" \
  --query id -o tsv)

GROUP_WRITERS=$(az ad group create \
  --display-name "are-writers" --mail-nickname "are-writers" \
  --description "AgentRegistry Enterprise read + write access" \
  --query id -o tsv)

az ad group member add --group "$GROUP_ADMINS" --member-id "$MY_USER_ID"

echo "GROUP_ADMINS:  $GROUP_ADMINS"
echo "GROUP_READERS: $GROUP_READERS"
echo "GROUP_WRITERS: $GROUP_WRITERS"
```

| Group | Purpose |
|-------|---------|
| `are-admins` | Superuser access (full control) |
| `are-readers` | Read-only access |
| `are-writers` | Read + write access |

Add other users:

```bash
USER_ID=$(az ad user show --id "user@example.com" --query id -o tsv)
az ad group member add --group "$GROUP_READERS" --member-id "$USER_ID"
```

## 6. Enable the `groups` Claim on All Three Apps

Entra must be told to put the `groups` claim into tokens.

**Portal**: For each of `are-backend`, `are-cli`, `are-ui`:
1. **Token configuration** > **Add groups claim**
2. Select **Security groups**, **Group ID** for both ID and Access tokens > **Add**

**CLI**:

```bash
for APP_ID in "$ARE_BACKEND_CLIENT_ID" "$ARE_CLI_CLIENT_ID" "$ARE_UI_CLIENT_ID"; do
  az ad app update --id "$APP_ID" \
    --set "groupMembershipClaims=\"SecurityGroup\"" \
    --set "optionalClaims={\"accessToken\":[{\"name\":\"groups\",\"source\":null,\"essential\":false,\"additionalProperties\":[]}],\"idToken\":[{\"name\":\"groups\",\"source\":null,\"essential\":false,\"additionalProperties\":[]}]}"
done
```

> **Entra emits group object IDs (GUIDs), not display names.** Your AccessPolicies must reference the GUIDs you exported above. See [080](080-access-policies.md).

> **Groups overage:** if a user belongs to more than ~200 groups, Entra omits the `groups` claim entirely and returns `_claim_names` / `_claim_sources` pointing to Microsoft Graph. AgentRegistry does not resolve the Graph overage endpoint. Use app roles instead — see [Appendix: App Roles](#appendix-use-app-roles-instead-of-groups).

## 7. Export Values for the Helm Chart

You should now have all the values [030](030-install-agentregistry-helm.md) needs. Confirm they are set in your shell:

```bash
for V in TENANT_ID ARE_BACKEND_CLIENT_ID ARE_BACKEND_CLIENT_SECRET \
         ARE_CLI_CLIENT_ID ARE_UI_CLIENT_ID \
         GROUP_ADMINS GROUP_READERS GROUP_WRITERS; do
  printf '%-30s %s\n' "$V" "${!V}"
done
```

## Troubleshooting

### AADSTS900144 — `The request body must contain the following parameter: 'scope'`

The legacy `arctl user login` device-code flow does not pass a scope, which Entra requires. Use the manual device-code login in [040](040-arctl-auth.md#manual-device-code-login-entra).

### AADSTS7000218 — `'client_assertion' or 'client_secret' required`

`are-cli` is a confidential client; the device-code flow needs a public client.

```bash
az ad app update --id "$ARE_CLI_CLIENT_ID" --is-fallback-public-client true
```

### AADSTS65001 — `The user or administrator has not consented to use the application`

```bash
az ad app permission admin-consent --id "$ARE_CLI_CLIENT_ID"
az ad app permission admin-consent --id "$ARE_UI_CLIENT_ID"
```

### v1.0 vs v2.0 Issuer Mismatch

If token validation fails with an issuer mismatch, confirm `accessTokenAcceptedVersion=2` and that your Helm `oidc.issuer` uses the v2.0 endpoint `https://login.microsoftonline.com/<TENANT_ID>/v2.0`.

```bash
az ad app show --id "$ARE_BACKEND_CLIENT_ID" --query "api.requestedAccessTokenVersion"
```

---

## Appendix: Use App Roles Instead of Groups

Prefer this if you want human-readable role names, or to dodge the groups overage limit.

```bash
ADMIN_ROLE_ID=$(python3 -c "import uuid; print(uuid.uuid4())")
READER_ROLE_ID=$(python3 -c "import uuid; print(uuid.uuid4())")
WRITER_ROLE_ID=$(python3 -c "import uuid; print(uuid.uuid4())")

az ad app update --id "$ARE_BACKEND_CLIENT_ID" \
  --app-roles "[
    {\"id\":\"$ADMIN_ROLE_ID\",\"displayName\":\"Admin\",\"description\":\"Full access\",\"value\":\"admin\",\"isEnabled\":true,\"allowedMemberTypes\":[\"User\"]},
    {\"id\":\"$READER_ROLE_ID\",\"displayName\":\"Reader\",\"description\":\"Read-only access\",\"value\":\"reader\",\"isEnabled\":true,\"allowedMemberTypes\":[\"User\"]},
    {\"id\":\"$WRITER_ROLE_ID\",\"displayName\":\"Writer\",\"description\":\"Read and write access\",\"value\":\"writer\",\"isEnabled\":true,\"allowedMemberTypes\":[\"User\"]}
  ]"

ARE_BACKEND_SP_ID=$(az ad sp show --id "$ARE_BACKEND_CLIENT_ID" --query id -o tsv)
MY_USER_ID=$(az ad signed-in-user show --query id -o tsv)

az rest --method POST \
  --uri "https://graph.microsoft.com/v1.0/servicePrincipals/$ARE_BACKEND_SP_ID/appRoleAssignments" \
  --body "{\"principalId\":\"$MY_USER_ID\",\"resourceId\":\"$ARE_BACKEND_SP_ID\",\"appRoleId\":\"$ADMIN_ROLE_ID\"}"
```

Then set `oidc.roleClaim: roles` and `oidc.superuserRole: admin` in your Helm values (see [030](030-install-agentregistry-helm.md)). AccessPolicy principals become the role values (`admin`, `reader`, `writer`) instead of GUIDs.

## Next

- [030 — Install AgentRegistry Enterprise (Helm)](030-install-agentregistry-helm.md)
- [040 — Authenticate `arctl`](040-arctl-auth.md)
