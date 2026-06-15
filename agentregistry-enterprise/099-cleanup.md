# Cleanup & Common Troubleshooting

This lab tears down everything created across the workshop. Run it in order — child resources first (HTTPRoute → Gateway → Secret), then the Helm release, then the namespace, then external resources (CloudFormation stack, Entra app registrations, Keycloak deployment).

## Cleanup

### 1. HTTPS Gateway and route (Entra path)

```bash
kubectl delete httproute  are-https-route   -n agentregistry-system 2>/dev/null || true
kubectl delete gateway    are-https-gateway -n agentregistry-system 2>/dev/null || true
kubectl delete secret     are-https-tls     -n agentregistry-system 2>/dev/null || true
```

### 2. AgentRegistry Helm release + namespace

```bash
helm uninstall agentregistry-enterprise -n agentregistry-system 2>/dev/null || true
kubectl delete namespace agentregistry-system 2>/dev/null || true
```

### 3. kagent extras (if you applied lab 091)

```bash
# Restore the original kagent collector config
kubectl apply -f assets/observability/backups/solo-enterprise-telemetry-collector-config.backup.yaml
kubectl -n kagent rollout restart statefulset solo-enterprise-telemetry-collector
```

### 4. MCP gateway resources (if you used lab 095)

```bash
kubectl delete httproute            mcp-route          -n agentgateway-system 2>/dev/null || true
kubectl delete agentgatewaybackend  github-mcp-server  -n agentgateway-system 2>/dev/null || true
kubectl delete secret               github-pat         -n agentgateway-system 2>/dev/null || true
kubectl delete gateway              mcp-gateway        -n agentgateway-system 2>/dev/null || true
```

### 5. AWS CloudFormation stack

```bash
aws cloudformation delete-stack \
  --stack-name agentregistry-access-role \
  --region us-east-1

aws cloudformation wait stack-delete-complete \
  --stack-name agentregistry-access-role \
  --region us-east-1
```

### 6. Temp files

```bash
rm -f /tmp/are-values.yaml /tmp/are-values-private.yaml \
      /tmp/aws-runtime.yaml /tmp/aws-provider.yaml \
      /tmp/agentregistry-cf.yaml \
      /tmp/kagent-runtime.yaml \
      /tmp/are-https.key /tmp/are-https.crt \
      /tmp/mcp-servers.json /tmp/mcp-servers.rendered.json \
      /tmp/ebs-csi-trust.json
```

### 7. Entra app registrations + groups (optional)

```bash
az ad app delete --id "$ARE_BACKEND_CLIENT_ID"
az ad app delete --id "$ARE_CLI_CLIENT_ID"
az ad app delete --id "$ARE_UI_CLIENT_ID"

az ad group delete --group "$GROUP_ADMINS"
az ad group delete --group "$GROUP_READERS"
az ad group delete --group "$GROUP_WRITERS"
```

### 8. Keycloak (optional)

```bash
kubectl delete namespace keycloak
```

### 9. Private EKS cluster (optional)

```bash
cd assets/private-eks
terraform destroy -var "cluster_name=are-private" -var "region=us-east-1"
```

## Common Troubleshooting

### `The logged in user does not have any mapped roles`

The OIDC token doesn't contain the expected role claim.

1. Decode your token at [jwt.ms](https://jwt.ms) (Entra) or [jwt.io](https://jwt.io).
2. Find the claim with your group / role memberships (`groups`, `Groups`, `roles`, `realm_access.roles`).
3. Update `oidc.roleClaim` in your Helm values to match.
4. `helm upgrade` and re-login.

If Entra returns `_claim_names` / `_claim_sources` instead of `groups`, you hit the groups overage limit — switch to [Entra app roles](020-setup-entra.md#appendix-use-app-roles-instead-of-groups).

### `AADSTS900144: The request body must contain the following parameter: 'scope'`

The current `arctl user login` doesn't pass `scope`, which Entra requires. Use the manual device-code flow in [040](040-arctl-auth.md#3-entra--manual-device-code-login).

### `AADSTS7000218: client_assertion or client_secret required`

The `are-cli` app is a confidential client; device-code requires a public client:

```bash
az ad app update --id "$ARE_CLI_CLIENT_ID" --is-fallback-public-client true
```

### `AADSTS50011: redirect URI ... does not match`

- `are-cli`: must have `http://localhost` under **Mobile and desktop applications**.
- `are-ui`: must have `https://<ARE_HTTPS_IP>/callback` under **Single-page application**.

### `AADSTS500117: reply uri specified in the request isn't using a secure scheme`

Entra requires HTTPS for SPA redirect URIs on non-localhost. Set up the HTTPS Gateway in [030](030-install-agentregistry-helm.md#5-expose-the-ui-over-https-for-entra-spa-login).

### `Token provider: disabled (encryption key not set)`

Informational. Only matters if you want to issue tokens to deployed agents. Generate a key:

```bash
openssl rand -hex 32
```

And add to your values:

```yaml
config:
  jwtPrivateKey: "<generated-hex-string>"
```

### Pods not starting

```bash
kubectl describe pod -n agentregistry-system -l app.kubernetes.io/name=agentregistry-enterprise
kubectl logs -n agentregistry-system deployment/agentregistry-enterprise -c wait-for-postgres
kubectl logs -n agentregistry-system deployment/agentregistry-enterprise
```

Most common cause: the cluster has no default `StorageClass`. See [010](010-cluster-prereqs.md).

### CloudFormation stack fails

```bash
aws cloudformation describe-stack-events \
  --stack-name agentregistry-access-role \
  --region us-east-1 \
  --query 'StackEvents[?ResourceStatus==`CREATE_FAILED`]'
```

Common: IAM user lacks `iam:CreateRole` or `cloudformation:CreateStack`; role name collision (`--role-name` with a unique name in `arctl provider setup aws`).

### kagent: `authentication token expired during deployment; please retry`

AgentRegistry maps every kagent API 401 to this. Usually the kagent controller is rejecting the forwarded identity. Enable `INSECURE_MODE` — see [051 step 2](051-kagent-provider.md#2-enable-insecure_mode-on-the-kagent-controller).

### Tracing UI is empty

See [090 troubleshooting](090-observability-tracing.md#troubleshooting) and the fan-out alternative in [091](091-trace-fanout-workaround.md).

## Reference Card

| Component | Value |
|-----------|-------|
| Chart | `oci://us-docker.pkg.dev/solo-public/agentregistry-enterprise/helm/agentregistry-enterprise` |
| Chart Version | `2026.5.3` (Entra track) / `2026.05.0` (Keycloak track) |
| Image | `us-docker.pkg.dev/solo-public/agentregistry-enterprise/server:v2026.5.3` |
| CLI install | `curl -sSL https://storage.googleapis.com/agentregistry-enterprise/install.sh \| ARCTL_VERSION=v2026.5.4 sh` |
| HTTP | 8080 |
| gRPC | 21212 |
| MCP | 31313 |
| OTel Collector | 4317 (gRPC) / 4318 (HTTP) |
| HTTPS Gateway | 443 via `are-https-gateway` (self-signed TLS) |
| Entra issuer | `https://login.microsoftonline.com/<TENANT_ID>/v2.0` |
| Entra device login | `https://microsoft.com/devicelogin` |
| Token decoders | [jwt.ms](https://jwt.ms) (Entra) / [jwt.io](https://jwt.io) |
