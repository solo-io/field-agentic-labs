# Azure AI Foundry Runtime

Register an **Azure AI Foundry Agent Service** project as an agentregistry **Runtime**. Agentregistry uses an Entra app registration and client secret to discover Foundry agents from a project endpoint and show them as unmanaged discovered instances.

This lab is discovery-focused. You create the Entra application, grant Azure RBAC, store the client secret as an agentregistry `Secret`, then create a `Runtime` of type `MicrosoftFoundry`.

## Lab Objectives

- Resolve the Azure AI Foundry project endpoint to its Azure resource scope
- Create an Entra app registration + service principal for agentregistry
- Grant Azure RBAC so the service principal can read Foundry agents
- Store the Entra client secret in agentregistry as a `Secret`
- Register the Foundry project as a `MicrosoftFoundry` Runtime
- Verify sync status and troubleshoot common Entra / Azure RBAC failures

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) → [002a](002a-setup-oidc-keycloak.md) **or** [002b](002b-setup-oidc-entra.md) → [003](003-install-components.md)
- Agentregistry Enterprise `v2026.6.2` or newer
- `arctl` authenticated against the running agentregistry
- `az` CLI installed and authenticated (`az account show` succeeds)
- An Azure AI Foundry project endpoint, for example:

  ```text
  https://<foundry-account>.services.ai.azure.com/api/projects/<project-name>
  ```

- Azure permissions to create Entra app registrations. 

If you don't have this, you will see an error:

`Identity(object id: some_id_here) does not have permissions for Microsoft.MachineLearningServices/workspaces/agents/read`

You can retry the RBAC assignment, but you may see Azure reject it:

`some person does not have authorization to perform Microsoft.Authorization/roleAssignments/write`

An Azure Owner or User Access Administrator needs to run:

```
az role assignment create \
  --assignee-object-id YOUR_OBJECT_ID \
  --assignee-principal-type ServicePrincipal \
  --role "Azure AI Developer" \
  --scope /subscriptions/YOUR_SUBSCRIPTION/resourceGroups/YOUR_RG
```

After that propagates, the Foundry runtime should sync. The client ID/secret and AgentRegistry runtime are already created and wired correctly.

- Azure **Owner** or **User Access Administrator** at the Foundry resource group or subscription scope, so you can create role assignments
- `jq`, `curl`, `base64`

> Foundry authentication always uses a Microsoft Entra app registration. Your agentregistry UI/API auth can still be either Keycloak or Entra; use the `ARCTL_API_BASE_URL` and `ARCTL_API_TOKEN` flow from [003](003-install-components.md#3-authenticate-arctl) for the raw API verification commands below.

## 1. Set the Foundry Variables

Set your Foundry endpoint and Azure context:

```bash
export FOUNDRY_PROJECT_ENDPOINT="https://<foundry-account>.services.ai.azure.com/api/projects/<project-name>"

export AZURE_TENANT_ID=$(az account show --query tenantId -o tsv)
export AZURE_SUBSCRIPTION_ID=$(az account show --query id -o tsv)

echo "AZURE_TENANT_ID=${AZURE_TENANT_ID}"
echo "AZURE_SUBSCRIPTION_ID=${AZURE_SUBSCRIPTION_ID}"
echo "FOUNDRY_PROJECT_ENDPOINT=${FOUNDRY_PROJECT_ENDPOINT}"
```

If you already know the resource group, Foundry account, and project name, export them directly:

```bash
export FOUNDRY_RESOURCE_GROUP="<resource-group>"
export FOUNDRY_ACCOUNT="<foundry-account>"
export FOUNDRY_PROJECT="<project-name>"
```

If not, derive them from Azure resource inventory:

```bash
az resource list \
  --query "[?type=='Microsoft.CognitiveServices/accounts/projects'].{name:name,resourceGroup:resourceGroup,id:id,location:location}" \
  -o table
```

Pick the row that matches your endpoint. Foundry project resource names are shaped like:

```text
<foundry-account>/<project-name>
```

Export the project resource ID:

```bash
export FOUNDRY_PROJECT_RESOURCE_ID="/subscriptions/${AZURE_SUBSCRIPTION_ID}/resourceGroups/${FOUNDRY_RESOURCE_GROUP}/providers/Microsoft.CognitiveServices/accounts/${FOUNDRY_ACCOUNT}/projects/${FOUNDRY_PROJECT}"

az resource show --ids "${FOUNDRY_PROJECT_RESOURCE_ID}" \
  --query "{name:name,type:type,resourceGroup:resourceGroup,endpoint:properties.endpoints.\"AI Foundry API\",provisioningState:properties.provisioningState}" \
  -o yaml
```

Expected:

```yaml
type: Microsoft.CognitiveServices/accounts/projects
provisioningState: Succeeded
```

## 2. Create the Entra App Registration

Create a single-tenant confidential app registration and service principal:

```bash
export FOUNDRY_APP_NAME="agentregistry-foundry-runtime"

export FOUNDRY_CLIENT_ID=$(az ad app create \
  --display-name "${FOUNDRY_APP_NAME}" \
  --sign-in-audience "AzureADMyOrg" \
  --query appId -o tsv)

export FOUNDRY_APP_OBJECT_ID=$(az ad app show \
  --id "${FOUNDRY_CLIENT_ID}" \
  --query id -o tsv)

export FOUNDRY_SP_OBJECT_ID=$(az ad sp create \
  --id "${FOUNDRY_CLIENT_ID}" \
  --query id -o tsv)

echo "FOUNDRY_CLIENT_ID=${FOUNDRY_CLIENT_ID}"
echo "FOUNDRY_APP_OBJECT_ID=${FOUNDRY_APP_OBJECT_ID}"
echo "FOUNDRY_SP_OBJECT_ID=${FOUNDRY_SP_OBJECT_ID}"
```

## 3. Grant Azure RBAC

Agentregistry's Foundry sync needs to list Foundry agents. In current Azure APIs, the service principal must be able to perform:

```text
Microsoft.MachineLearningServices/workspaces/agents/read
```

Assign `Azure AI Developer` at the resource group scope:

```bash
az role assignment create \
  --assignee-object-id "${FOUNDRY_SP_OBJECT_ID}" \
  --assignee-principal-type ServicePrincipal \
  --role "Azure AI Developer" \
  --scope "/subscriptions/${AZURE_SUBSCRIPTION_ID}/resourceGroups/${FOUNDRY_RESOURCE_GROUP}"
```

Optionally also assign `Foundry User` at the Foundry project scope:

```bash
az role assignment create \
  --assignee-object-id "${FOUNDRY_SP_OBJECT_ID}" \
  --assignee-principal-type ServicePrincipal \
  --role "Foundry User" \
  --scope "${FOUNDRY_PROJECT_RESOURCE_ID}"
```

Verify assignments:

```bash
az role assignment list \
  --assignee "${FOUNDRY_SP_OBJECT_ID}" \
  --include-inherited \
  --query "[].{role:roleDefinitionName,scope:scope}" \
  -o table
```

If role assignment fails with `AuthorizationFailed`, your signed-in Azure user can create app registrations but cannot assign Azure RBAC roles. Ask an Azure Owner or User Access Administrator to run the commands above.

## 4. Create and Verify the Client Secret

Create a one-year client secret. Do not echo or commit the value:

```bash
export FOUNDRY_CLIENT_SECRET=$(az ad app credential reset \
  --id "${FOUNDRY_CLIENT_ID}" \
  --display-name "${FOUNDRY_APP_NAME}" \
  --years 1 \
  --query password -o tsv 2>/dev/null)
```

Entra client secrets can take a few seconds to propagate. Wait until the secret can mint a token for Azure AI:

```bash
for i in {1..12}; do
  TOKEN_TEST=$(curl -sS -X POST "https://login.microsoftonline.com/${AZURE_TENANT_ID}/oauth2/v2.0/token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    --data-urlencode "client_id=${FOUNDRY_CLIENT_ID}" \
    --data-urlencode "client_secret=${FOUNDRY_CLIENT_SECRET}" \
    --data-urlencode "grant_type=client_credentials" \
    --data-urlencode "scope=https://ai.azure.com/.default")

  if [ "$(printf "%s" "${TOKEN_TEST}" | jq -r '.access_token != null')" = "true" ]; then
    echo "Foundry client secret is valid"
    break
  fi

  sleep 5
done
```

If validation fails, inspect the non-secret error:

```bash
printf "%s" "${TOKEN_TEST}" | jq '{error,error_description}'
```

## 5. Store the Secret in agentregistry

Create an agentregistry `Secret` that stores the Entra client secret. The Foundry runtime will reference this by name/key.

```bash
export FOUNDRY_REGISTRY_SECRET_NAME="foundry-runtime-client-secret"
export FOUNDRY_REGISTRY_SECRET_KEY="clientSecret"
```

```bash
ENCODED_SECRET=$(printf "%s" "${FOUNDRY_CLIENT_SECRET}" | base64 | tr -d "\n")

cat > /tmp/foundry-client-secret.yaml <<EOF
apiVersion: ar.dev/v1alpha1
kind: Secret
metadata:
  name: ${FOUNDRY_REGISTRY_SECRET_NAME}
spec:
  type: Opaque
  data:
    ${FOUNDRY_REGISTRY_SECRET_KEY}: ${ENCODED_SECRET}
EOF

arctl apply -f /tmp/foundry-client-secret.yaml
```

> The temp file contains a base64-encoded secret value. Remove it in cleanup or immediately after applying if you prefer.

Verify without exposing the secret:

```bash
if [ -z "${ARCTL_API_TOKEN:-}" ]; then
  export ARCTL_API_TOKEN=$(arctl user info --show-tokens | jq -r .access_token)
fi

curl -sS \
  -H "Authorization: Bearer ${ARCTL_API_TOKEN}" \
  "${ARCTL_API_BASE_URL}/v0/secrets/${FOUNDRY_REGISTRY_SECRET_NAME}" | jq '.status'
```

Expected:

```json
{
  "dataKeys": [
    "clientSecret"
  ]
}
```

If `ARCTL_API_TOKEN` is still not set, repeat the authentication flow from [003](003-install-components.md#3-authenticate-arctl).

## 6. Register the Foundry Runtime

Choose a runtime connection name. It must be a valid agentregistry resource name:

```bash
export FOUNDRY_RUNTIME_NAME="foundry-${FOUNDRY_ACCOUNT}"
```

Create the runtime manifest:

```bash
cat > /tmp/foundry-runtime.yaml <<EOF
apiVersion: ar.dev/v1alpha1
kind: Runtime
metadata:
  name: ${FOUNDRY_RUNTIME_NAME}
spec:
  type: MicrosoftFoundry
  config:
    projectEndpoint: ${FOUNDRY_PROJECT_ENDPOINT}
    tenantId: ${AZURE_TENANT_ID}
    clientId: ${FOUNDRY_CLIENT_ID}
    subscriptionId: ${AZURE_SUBSCRIPTION_ID}
    resourceGroup: ${FOUNDRY_RESOURCE_GROUP}
    auth:
      clientSecretRef:
        name: ${FOUNDRY_REGISTRY_SECRET_NAME}
        key: ${FOUNDRY_REGISTRY_SECRET_KEY}
EOF

arctl apply -f /tmp/foundry-runtime.yaml
```

Verify it appears:

```bash
arctl get runtimes
arctl get runtime "${FOUNDRY_RUNTIME_NAME}" -o yaml
```

## 7. Verify Sync

The runtime should move to `Synced=True` after agentregistry polls Foundry:

```bash
arctl get runtime "${FOUNDRY_RUNTIME_NAME}" -o yaml
```

If you prefer the raw API status:

```bash
if [ -z "${ARCTL_API_TOKEN:-}" ]; then
  export ARCTL_API_TOKEN=$(arctl user info --show-tokens | jq -r .access_token)
fi

curl -sS \
  -H "Authorization: Bearer ${ARCTL_API_TOKEN}" \
  "${ARCTL_API_BASE_URL}/v0/runtimes/${FOUNDRY_RUNTIME_NAME}" | jq '.status'
```

Expected if the project has no agents:

- `Synced=True`, with zero discovered instances in the UI.

Expected if the project has agents:

- `Synced=True`, and discovered Foundry agents appear under the runtime as unmanaged instances.

## Troubleshooting

| Error | Cause | Fix |
|---|---|---|
| `AADSTS7000215: Invalid client secret provided` | Entra has not propagated the new secret yet, or the runtime points at the wrong Secret key | Re-run step 4 and step 5. Wait until `TOKEN_TEST` returns an access token before applying the Secret. |
| `does not have permissions for Microsoft.MachineLearningServices/workspaces/agents/read` | Service principal lacks the Azure role needed by the Foundry agent list API | Assign `Azure AI Developer` at the resource-group scope in step 3. |
| `AuthorizationFailed` when assigning a role | Your Azure user can create apps but cannot write RBAC role assignments | Ask an Azure Owner or User Access Administrator to run step 3. |
| Runtime does not appear in UI Secret dropdown | The agentregistry Secret was not created, or was created in another namespace | Re-run step 5 and verify `/v0/secrets/<name>` reports `dataKeys`. |
| `arctl apply` returns `401` | Your local `arctl` token expired | Run `arctl user login` again, then retry. |

## Cleanup

Return agentregistry and Azure to the post-baseline state:

```bash
# agentregistry side
arctl delete -f /tmp/foundry-runtime.yaml
arctl delete -f /tmp/foundry-client-secret.yaml
```

Remove Azure RBAC assignments:

```bash
az role assignment delete \
  --assignee "${FOUNDRY_SP_OBJECT_ID}" \
  --role "Azure AI Developer" \
  --scope "/subscriptions/${AZURE_SUBSCRIPTION_ID}/resourceGroups/${FOUNDRY_RESOURCE_GROUP}"

az role assignment delete \
  --assignee "${FOUNDRY_SP_OBJECT_ID}" \
  --role "Foundry User" \
  --scope "${FOUNDRY_PROJECT_RESOURCE_ID}" 2>/dev/null || true
```

Delete the Entra app registration:

```bash
az ad app delete --id "${FOUNDRY_CLIENT_ID}"
```

Remove local temp files and variables:

```bash
rm -f /tmp/foundry-runtime.yaml /tmp/foundry-client-secret.yaml

unset FOUNDRY_PROJECT_ENDPOINT FOUNDRY_RESOURCE_GROUP FOUNDRY_ACCOUNT FOUNDRY_PROJECT \
  FOUNDRY_PROJECT_RESOURCE_ID FOUNDRY_APP_NAME FOUNDRY_CLIENT_ID FOUNDRY_APP_OBJECT_ID \
  FOUNDRY_SP_OBJECT_ID FOUNDRY_CLIENT_SECRET FOUNDRY_REGISTRY_SECRET_NAME \
  FOUNDRY_REGISTRY_SECRET_KEY FOUNDRY_RUNTIME_NAME AZURE_TENANT_ID AZURE_SUBSCRIPTION_ID
```

## Next

- [010 - AWS Bedrock AgentCore Runtime](010-aws-bedrock-runtime.md) - add an AWS-hosted runtime
- [020 - kagent Runtime + Agent](020-kagent-runtime-and-agent.md) - add an in-cluster runtime
- [050 - AccessPolicy](050-access-policies.md) - control who can read and invoke runtime-backed assets
