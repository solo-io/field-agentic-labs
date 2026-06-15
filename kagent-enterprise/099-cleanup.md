# Cleanup & Common Troubleshooting

Tear down everything created across the workshop. Run in order — child resources first, then Helm releases, then namespaces, then cluster.

## Cleanup

### 1. Per-lab Demo Resources

```bash
# 050: broken Pod
kubectl delete pod ngi14 --ignore-not-found

# 060: agent-to-MCP AccessPolicy demo
kubectl delete accesspolicy deny-kagent-tool-server-dec -n kagent --ignore-not-found
kubectl delete accesspolicy deny-kagent-tool-server     -n kagent --ignore-not-found
kubectl delete agent test-access-policy -n kagent --ignore-not-found
kubectl delete agent troubleshooter     -n kagent --ignore-not-found
kubectl delete mcpserver test-mcp-server -n kagent --ignore-not-found
kubectl delete secret kagent-google -n kagent --ignore-not-found

# 061: UserGroup AccessPolicy demo
kubectl delete accesspolicy deny-reader-agent-access -n policies --ignore-not-found
kubectl delete agent platform-agent      -n policies --ignore-not-found
kubectl delete modelconfig model-config  -n policies --ignore-not-found
kubectl delete secret kagent-anthropic   -n policies --ignore-not-found
kubectl delete namespace policies --ignore-not-found

# 070: prompt guard
kubectl delete enterpriseagentgatewaypolicy credit-guard-prompt-guard -n agentgateway-system --ignore-not-found

# 071: platform RBAC demo
kubectl delete clusterrolebinding kagent-viewer-binding --ignore-not-found
kubectl delete clusterrole        kagent-crd-viewer     --ignore-not-found
kubectl delete serviceaccount     test-reader -n kagent --ignore-not-found
```

### 2. OBO Stack (Lab 090)

```bash
# Policy + secrets + route + proxy
kubectl delete enterpriseagentgatewaypolicy entra-obo-token-exchange -n agentgateway-system --ignore-not-found
kubectl delete secret entra-obo-client-secret -n agentgateway-system --ignore-not-found
kubectl delete httproute llm-obo-proxy        -n agentgateway-system --ignore-not-found
kubectl delete service   llm-obo-proxy        -n agentgateway-system --ignore-not-found
kubectl delete deployment llm-obo-proxy       -n agentgateway-system --ignore-not-found
kubectl delete configmap llm-obo-proxy-code   -n agentgateway-system --ignore-not-found

# UI HTTPS plumbing
kubectl delete httproute       kagent-ui-https            -n agentgateway-system --ignore-not-found
kubectl delete referencegrant  allow-agentgateway-ui-route -n kagent              --ignore-not-found
kubectl delete secret          kagent-ui-https-tls        -n agentgateway-system --ignore-not-found

# Gateway + dataplane params + provider keys
kubectl delete gateway agentgateway-entra-testing -n agentgateway-system --ignore-not-found
kubectl delete enterpriseagentgatewayparameters agentgateway-entra-testing-enterprise -n agentgateway-system --ignore-not-found
kubectl delete secret anthropic-secret -n agentgateway-system --ignore-not-found

# OBO-specific kagent ModelConfig + Agent
kubectl delete agent       obo-demo-agent       -n kagent --ignore-not-found
kubectl delete modelconfig anthropic-model-config -n kagent --ignore-not-found
```

### 3. Enterprise Agentgateway

```bash
helm uninstall agentgateway      -n agentgateway-system 2>/dev/null || true
helm uninstall agentgateway-crds -n agentgateway-system 2>/dev/null || true
kubectl delete secret enterprise-agentgateway-license -n agentgateway-system --ignore-not-found
kubectl delete namespace agentgateway-system 2>/dev/null || true
```

### 4. Kagent Enterprise — Direct-Helm Path (Lab 090)

If you installed via the direct-Helm path from the OBO lab:

```bash
helm uninstall kagent       -n kagent 2>/dev/null || true
helm uninstall kagent-crds  -n kagent 2>/dev/null || true
helm uninstall kagent-mgmt  -n kagent 2>/dev/null || true

kubectl delete secret enterprise-kagent-license       -n kagent --ignore-not-found
kubectl delete secret kagent-enterprise-oidc-secret   -n kagent --ignore-not-found
kubectl delete secret kagent-anthropic                -n kagent --ignore-not-found
kubectl delete secret llm-api-keys                    -n kagent --ignore-not-found
kubectl delete secret kagent-backend-secret           -n kagent --ignore-not-found
kubectl delete secret jwt                             -n kagent --ignore-not-found

kubectl delete namespace kagent 2>/dev/null || true
```

### 5. Kagent Enterprise — Gloo Operator Path (Lab 020)

If you installed via the Gloo Operator path:

```bash
# Operator CRs — removes Solo Istio, Gloo Gateway, kagent-enterprise mgmt + runtime
kubectl delete kagentcontroller                   kagent             -n kagent --ignore-not-found
kubectl delete kagentmanagementcontroller         kagent-enterprise  -n kagent --ignore-not-found
kubectl delete gatewaycontroller                  gloo-gateway       -n kagent --ignore-not-found
kubectl delete servicemeshcontroller              managed-istio      -n kagent --ignore-not-found
kubectl delete configmap                           gloo-extensions-config -n kagent --ignore-not-found

# Operator + namespaces
helm uninstall gloo-operator -n kagent 2>/dev/null || true
kubectl delete namespace kagent       2>/dev/null || true
kubectl delete namespace gloo-system  2>/dev/null || true
kubectl delete namespace istio-system 2>/dev/null || true
```

### 6. Pinniped + Keycloak (Lab 080)

```bash
kubectl delete clusterrolebinding keycloak-cluster-admins --ignore-not-found
kubectl delete clusterrolebinding keycloak-developers     --ignore-not-found
kubectl delete clusterrolebinding keycloak-viewers        --ignore-not-found
kubectl delete jwtauthenticator   keycloak                --ignore-not-found
kubectl delete -f https://get.pinniped.dev/latest/install-pinniped-concierge.yaml --ignore-not-found
kubectl delete namespace keycloak --ignore-not-found

rm -f keycloak-tls.crt keycloak-tls.key pinniped-kubeconfig.yaml
```

### 7. GKE Cluster (Lab 001)

```bash
cd assets/gke-terraform
terraform destroy
```

### 8. Local Temp Files

```bash
rm -f /tmp/key.pem \
      /tmp/agw-values.rendered.yaml \
      /tmp/management.rendered.yaml \
      /tmp/kagent-values.rendered.yaml \
      /tmp/kagent-ui-https.crt \
      /tmp/kagent-ui-https.key
```

## Common Troubleshooting

### Pods stuck in `Pending` on storage

The bundled ClickHouse + PostgreSQL need a default `StorageClass`. On GKE this works out of the box (`standard-rwo`). On Kind/minikube/custom clusters, install a CSI driver and mark a `StorageClass` default *before* installing kagent.

### UI shows "cannot connect to backend" (Gloo Operator path)

The UI hard-codes `localhost:8090` for its backend. Re-run the port-forward from [020 step 4](020-install-kagent-enterprise.md#4-work-around-the-ui-backend-bug-port-forward):

```bash
kubectl port-forward service/kagent-enterprise-ui -n kagent 8090:8090
```

### `unauthorized` on UI login

OIDC values mismatch. Inspect the controller logs:

```bash
kubectl logs -n kagent -l app=kagent --tail=100 | grep -i oidc
```

### Entra: `AADSTS50011: The redirect URI ... does not match`

You registered the wrong callback URI. For the OBO lab the SPA callback must be `https://<AGW_HTTPS_EXTERNAL_IP>/callback`, **not** `/auth`. See [090 step 7a](090-obo-entra.md#7a--add-https-for-the-ui-login-flow).

### Entra: `AADSTS500117: The reply uri specified ... isn't using a secure scheme`

Entra requires HTTPS for SPA redirect URIs on non-localhost. The HTTPS Gateway in 7a is the fix.

### Token exchange returns 401 / "issuer mismatch"

The `subjectValidator.remoteConfig.url` on the agentgateway controller must point at the **Entra** JWKS endpoint, not at the Kubernetes API server JWKS. See [025 step 4](025-install-enterprise-agentgateway.md#4-install-the-controller).

If you used `oidc.skipOBO: false` on kagent, kagent mints its own JWT signed by the `jwt` Secret — agentgateway's STS can't validate that against the Entra JWKS, so the exchange fails. **Set `skipOBO: true`** in the kagent values for the OBO scenario.

### `obo-demo-agent` reaches the proxy but the proxy 401s

Decode the bearer token the proxy received and check `aud` and `iss`:

```bash
kubectl logs deployment/llm-obo-proxy -n agentgateway-system | tail -50
```

`EXPECTED_AUDIENCES` in [`assets/llm-obo-proxy/deployment.yaml`](assets/llm-obo-proxy/deployment.yaml) must include the value that agentgateway is exchanging the token for. By default the lab sets it to `${KAGENT_BACKEND_CLIENT_ID},api://${KAGENT_BACKEND_CLIENT_ID}` — either form should match the `aud` claim.

### CloudFormation / Terraform fails

For GKE Terraform, run:

```bash
cd assets/gke-terraform
terraform plan
```

…and look at the diff. The most common cause is that `project_id` in `terraform.tfvars` is wrong, or the GKE / Service Networking / Compute APIs aren't enabled on the project (`gcloud services enable container.googleapis.com servicenetworking.googleapis.com compute.googleapis.com`).

## Reference Card

| Component | Value |
|-----------|-------|
| Gloo Operator chart | `oci://us-docker.pkg.dev/solo-public/gloo-operator-helm/gloo-operator` `0.4.0` |
| Kagent (Gloo Operator) controller | `0.1.5` |
| Solo Istio (Ambient) | `1.27.1` |
| Gloo Gateway | `2.0.0` |
| Kagent (direct-Helm) charts | `oci://us-docker.pkg.dev/solo-public/solo-enterprise-helm/charts/management`, `kagent-enterprise-helm/charts/kagent-enterprise-crds`, `kagent-enterprise-helm/charts/kagent-enterprise` — `0.3.12` |
| Enterprise Agentgateway | `oci://us-docker.pkg.dev/solo-public/enterprise-agentgateway/charts/enterprise-agentgateway[-crds]` — `v2.2.0` |
| Gateway API CRDs | `v1.5.0` |
| Anthropic OBO model | `claude-haiku-4-5-20251001` |
| Keycloak image | `quay.io/keycloak/keycloak:26.0` |
| Pinniped Concierge | `latest` (`get.pinniped.dev/latest`) |
| Proxy port | 8080 |
| Token-exchange port | 7777 |
| UI HTTPS gateway | 443 (self-signed) |
| UI HTTP service | 8080 / 8090 (Gloo Operator path) |
