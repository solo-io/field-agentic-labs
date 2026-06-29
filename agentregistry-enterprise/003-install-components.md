# Install Components: agentregistry + Enterprise Agentgateway

The third (and last) mandatory setup lab for agentregistry. Installs two components every unit-of-value lab in this workshop depends on:

1. **Agentregistry Enterprise** - the catalog + control plane (lab subject)
2. **Enterprise Agentgateway** - the LLM / MCP gateway (lab 032 exposes MCPs through here)

It also authenticates `arctl` against the running agentregistry server.

After this lab + [001](001-baseline-setup.md) + ([002a](002a-setup-oidc-keycloak.md) **or** [002b](002b-setup-oidc-entra.md)), the cluster has the full **baseline** that the rest of the workshop assumes.

> **kagent Enterprise is a prerequisite, not part of this lab.** The kagent-runtime lab ([020](020-kagent-runtime-and-agent.md)) and the MCP-via-kagent lab ([031](031-mcp-remote-github-copilot.md)) assume kagent Enterprise is already installed on the cluster. Install it from the [kagent-enterprise workshop](https://github.com/solo-io/field-agentic-labs/tree/main/kagent-enterprise) (labs 001 - 003) before running 020 or 031. The AWS Bedrock AgentCore lab ([010](010-aws-bedrock-runtime.md)), the local-stdio MCP lab ([030](030-mcp-local-stdio.md)), and most other unit labs do **not** need kagent.

## Lab Objectives

- Install agentregistry Enterprise (Helm OCI) wired to your OIDC provider from 002
- Install Enterprise Agentgateway in the `agentgateway-system` namespace with your license key
- Authenticate `arctl` against the running agentregistry server
- Confirm both components are healthy

## Prerequisites

- [001 - Baseline Setup](001-baseline-setup.md) completed (cluster, `arctl`, namespace, tools)
- One of:
  - [002a - Setup OIDC: Keycloak](002a-setup-oidc-keycloak.md), with `OIDC_PROVIDER=keycloak` + the variables exported, **OR**
  - [002b - Setup OIDC: Entra ID](002b-setup-oidc-entra.md), with `OIDC_PROVIDER=entra` + the variables exported

The shell variables you need to have set going in:

```bash
# From 002a or 002b
$OIDC_PROVIDER          # "keycloak" or "entra"
$OIDC_ISSUER
$OIDC_BACKEND           # client ID (Entra GUID, or "are-backend" for Keycloak)
$BACKEND_CLIENT_SECRET
$OIDC_PUBLIC_CLIENT     # client ID used by the UI for browser login
$ARE_CLI_CLIENT_ID      # client ID used by arctl user login
$GROUP_ADMINS           # admins group object ID / GUID
$AGENTGATEWAY_LICENSE_KEY # Solo Enterprise for agentgateway license key
```

Sanity check:

```bash
for V in OIDC_PROVIDER OIDC_ISSUER OIDC_BACKEND BACKEND_CLIENT_SECRET \
         OIDC_PUBLIC_CLIENT ARE_CLI_CLIENT_ID GROUP_ADMINS AGENTGATEWAY_LICENSE_KEY; do
  eval "VALUE=\${${V}:-}"
  if [ -z "${VALUE}" ]; then
    echo "MISSING: ${V}"
  elif [ "${V}" = "AGENTGATEWAY_LICENSE_KEY" ]; then
    printf '  OK  %-25s %s\n' "${V}" "set"
  else
    printf '  OK  %-25s %.20s...\n' "${V}" "${VALUE}"
  fi
done
```

Every line should print `OK`. If any OIDC variable prints `MISSING`, go back to 002a or 002b. If `AGENTGATEWAY_LICENSE_KEY` prints `MISSING`, export your Solo Enterprise for agentgateway license key before continuing.

## 1. Install agentregistry Enterprise

Build the Helm values from your OIDC variables. The script appends the Entra-only `additionalScopes` setting when `OIDC_PROVIDER=entra`. **Do not commit this file** - it contains secrets:

```bash
if [ "${OIDC_PROVIDER}" = "keycloak" ]; then
  export OIDC_SUPERUSER_ROLE="are-admins"
else
  export OIDC_SUPERUSER_ROLE="${GROUP_ADMINS}"
fi

cat > /tmp/are-values.yaml.tpl <<'EOF'
image:
  tag: v2026.6.2

service:
  type: LoadBalancer

oidc:
  issuer: "${OIDC_ISSUER}"
  clientId: "${OIDC_BACKEND}"
  publicClientId: "${OIDC_PUBLIC_CLIENT}"
  clientSecret: "${BACKEND_CLIENT_SECRET}"
  roleClaim: "groups"
  superuserRole: "${OIDC_SUPERUSER_ROLE}"
  insecureSkipVerify: false
EOF

if [ "${OIDC_PROVIDER}" = "entra" ]; then
  cat >> /tmp/are-values.yaml.tpl <<'EOF'
  additionalScopes: "offline_access api://${OIDC_BACKEND}/agentregistry"
EOF
fi

cat >> /tmp/are-values.yaml.tpl <<'EOF'

database:
  postgres:
    type: bundled

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

envsubst < /tmp/are-values.yaml.tpl > /tmp/are-values.yaml
rm -f /tmp/are-values.yaml.tpl
```

Keycloak emits group names such as `are-admins` in the `groups` claim. Entra emits group object IDs, so the Entra path uses `${GROUP_ADMINS}`.

Install:

```bash
helm upgrade --install agentregistry-enterprise \
  oci://us-docker.pkg.dev/solo-public/agentregistry-enterprise/helm/agentregistry-enterprise \
  --version 2026.6.2 \
  --namespace agentregistry-system \
  -f /tmp/are-values.yaml \
  --wait --timeout 5m
```

Verify (all pods 1/1 Running):

```bash
kubectl get pods -n agentregistry-system -w
```

Expected:

```
agentregistry-enterprise-server-<hash>                1/1 Running
agentregistry-enterprise-clickhouse-shard0-0          1/1 Running
agentregistry-enterprise-postgresql-<hash>            1/1 Running
agentregistry-enterprise-telemetry-collector-<hash>   1/1 Running
```

Grab the external IP and confirm the API responds:

```bash
export AR_IP=$(kubectl get svc agentregistry-enterprise-server -n agentregistry-system \
  -o jsonpath='{.status.loadBalancer.ingress[0].ip}{.status.loadBalancer.ingress[0].hostname}')
export ARCTL_API_BASE_URL="http://${AR_IP}:12121"
echo "agentregistry API: ${ARCTL_API_BASE_URL}"
echo "agentregistry UI:  http://${AR_IP}:12121"

arctl version --json   # arctl_version + server_version should both populate
```

## 2. Install Enterprise Agentgateway

Required for lab 032 (remote MCP through agentgateway). Installing it now keeps the baseline complete - every subsequent unit-of-value lab can assume it's there.

```bash
export AGENTGATEWAY_LICENSE_KEY=<agentgateway-license-key>
```

```bash
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.5.0/standard-install.yaml
```

```bash
# Agentgateway CRDs
helm upgrade --install agentgateway-crds \
  oci://us-docker.pkg.dev/solo-public/enterprise-agentgateway/charts/enterprise-agentgateway-crds \
  --version v2026.6.1 \
  --namespace agentgateway-system \
  --create-namespace
```

```bash
# Agentgateway controller
helm upgrade --install enterprise-agentgateway \
  oci://us-docker.pkg.dev/solo-public/enterprise-agentgateway/charts/enterprise-agentgateway \
  --version v2026.6.1 \
  --namespace agentgateway-system \
  --set-string licensing.licenseKey="${AGENTGATEWAY_LICENSE_KEY}"
```

> Enterprise Agentgateway requires a Solo Enterprise for agentgateway license key. If you do not have one, contact your Solo account representative before running this step.

Verify:

```bash
kubectl get pods -n agentgateway-system
```

You should see the `enterprise-agentgateway` controller pod Ready.

## 3. Authenticate `arctl`

### Keycloak path

```bash
arctl user login \
  --oidc-issuer-url "${OIDC_ISSUER}" \
  --oidc-client-id "${ARE_CLI_CLIENT_ID}"
```

This opens a browser to Keycloak; sign in as `admin` / `admin`. The token is cached in your OS keychain and `arctl` refreshes it automatically.

### Entra path

The current `arctl user login` doesn't pass a `scope` parameter, which Entra requires (`AADSTS900144`). Use the manual device-code flow you already validated in [002b step 7](002b-setup-oidc-entra.md#verify-the-setup), then export the access token:

```bash
DEVICE=$(curl -s -X POST \
  "https://login.microsoftonline.com/${TENANT_ID}/oauth2/v2.0/devicecode" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "client_id=${ARE_CLI_CLIENT_ID}&scope=openid+api://${ARE_BACKEND_CLIENT_ID}/agentregistry")
echo "${DEVICE}" | jq

# Open the URL + enter the code in your browser, then:
DEVICE_CODE=$(echo "${DEVICE}" | jq -r .device_code)

while true; do
  RESP=$(curl -s -X POST \
    "https://login.microsoftonline.com/${TENANT_ID}/oauth2/v2.0/token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "grant_type=urn:ietf:params:oauth:grant-type:device_code&client_id=${ARE_CLI_CLIENT_ID}&device_code=${DEVICE_CODE}")
  ERR=$(echo "${RESP}" | jq -r '.error // "none"')
  if [ "${ERR}" = "none" ]; then
    export ARCTL_API_TOKEN=$(echo "${RESP}" | jq -r '.access_token')
    echo "Token obtained"
    break
  elif [ "${ERR}" = "authorization_pending" ]; then
    sleep 5
  else
    echo "${RESP}" | jq
    break
  fi
done
```

### Verify either path

```bash
# Re-export in case you opened a new shell or still have an old :8080 value.
export AR_IP=$(kubectl get svc agentregistry-enterprise-server -n agentregistry-system \
  -o jsonpath='{.status.loadBalancer.ingress[0].ip}{.status.loadBalancer.ingress[0].hostname}')
export ARCTL_API_BASE_URL="http://${AR_IP}:12121"

arctl version --json
arctl get providers   # empty list is fine - runtimes are registered in 010 / 020
arctl user whoami
```

`whoami` should print the user you signed in as, with the mapped role(s) containing your admin group's identifier.

## 4. Confirm the Baseline is Complete

```bash
# agentregistry
kubectl get pods -n agentregistry-system
arctl version --json   # both versions populated

# Enterprise Agentgateway
kubectl get pods -n agentgateway-system
kubectl get crd | grep agentgateway
```

If both blocks look healthy, the baseline is in place. Any unit-of-value lab that doesn't require kagent (010, 030, 032, 040, 050, 051, 060, 070) should now work.

For labs that **do** require kagent (020, 031, 061), install it first from the [kagent-enterprise workshop](https://github.com/solo-io/field-agentic-labs/tree/main/kagent-enterprise) - run that workshop's labs 001 through 003.

## What's in Place After 001 + 002 + 003

| Component | Namespace | Role |
|---|---|---|
| `arctl` CLI | local | Authenticated against your agentregistry server |
| Agentregistry Enterprise | `agentregistry-system` | Catalog + control plane |
| Enterprise Agentgateway | `agentgateway-system` | LLM / MCP gateway |
| Keycloak **or** Entra ID | `keycloak` namespace / Microsoft cloud | OIDC backend |
| kagent Enterprise (**user-installed prereq**) | `kagent` | In-cluster agent runtime, only required by labs 020 / 031 / 061 |

Each unit-of-value lab assumes this baseline. None of them re-install these components.

## Cleanup

This lab installs the baseline that every unit-of-value lab relies on. Don't clean this up until you're done with the entire workshop. When you are, see [099 - Cleanup](099-cleanup.md) for the full teardown.

Component-level rollback (in case the install partially failed and you want to redo):

```bash
helm uninstall enterprise-agentgateway -n agentgateway-system 2>/dev/null || true
helm uninstall agentgateway-crds       -n agentgateway-system 2>/dev/null || true

helm uninstall agentregistry-enterprise -n agentregistry-system 2>/dev/null || true

rm -f /tmp/are-values.yaml
unset AGENTGATEWAY_LICENSE_KEY
```

The namespaces from 001 / 002 stay - re-run this lab to reinstall.

## Troubleshooting

| Symptom | Fix |
|---|---|
| Agentregistry pods stuck in `Pending` on storage | No default `StorageClass`. Go back to [001 step 1](001-baseline-setup.md#1-confirm-the-cluster-is-ready). |
| `arctl version --json` shows empty `server_version` | `ARCTL_API_BASE_URL` is unset or wrong. `export ARCTL_API_BASE_URL=http://${AR_IP}:12121`. |
| `arctl user whoami` shows local token info only and `registry unreachable` | Your OIDC login worked, but `ARCTL_API_BASE_URL` is stale or wrong. Re-export it with `export ARCTL_API_BASE_URL=http://${AR_IP}:12121`; do not use `:8080` for the agentregistry Service. |
| `arctl user login` hangs on Entra | The current CLI doesn't pass `scope`. Use the manual device-code flow above. |
| UI shows "no mapped roles" after login | The `groups` claim is missing or the GUID doesn't match `${GROUP_ADMINS}`. Decode your token at <https://jwt.io> and confirm. Re-run 002a/002b if the realm/app-reg config drifted. |
| Agentgateway controller `CrashLoopBackOff` | Check that `AGENTGATEWAY_LICENSE_KEY` was set and passed with `--set-string licensing.licenseKey=...`. |

## Next

Every unit-of-value lab from here on is self-contained. Pick one:

- [010 - AWS Bedrock AgentCore Runtime](010-aws-bedrock-runtime.md) - register AWS + deploy the demochatbot
- [020 - kagent Runtime + Agent](020-kagent-runtime-and-agent.md) - register the kagent runtime + deploy `k8shelper` (**requires kagent Enterprise already installed**)
- [030 - Local stdio MCP Server](030-mcp-local-stdio.md) - `demo-tools` (in-tree)
- [031 - Remote MCP via kagent (GitHub Copilot)](031-mcp-remote-github-copilot.md) (**requires kagent Enterprise already installed**)
- [032 - Remote MCP through Agentgateway](032-mcp-through-agentgateway.md)
- [040 - Prompts](040-prompts.md)
- [050 - AccessPolicy](050-access-policies.md)
- [051 - Approval Workflows](051-approval-workflows.md)
- [060 - Observability / Tracing](060-observability-tracing.md)
- [061 - Trace Fan-Out (kagent)](061-trace-fanout.md) (**requires kagent Enterprise already installed**)
