# Install Enterprise Agentgateway

Enterprise Agentgateway sits in front of LLM and MCP traffic and adds prompt guards, OBO token exchange, OIDC-fronted access, and per-route observability. The Gloo Operator install in [020](003-install-kagent-enterprise.md) sets `agentgateway.enabled: true` in the Gloo Gateway values, which gives you the **agentgateway dataplane** as part of Gloo Gateway. For the OBO scenario and the prompt-guard lab, you'll also need the **enterprise-agentgateway controller** running as its own Helm release in `agentgateway-system`.

This lab installs the enterprise-agentgateway controller chart at `v2.2.0` with `tokenExchange.enabled: true` (so the same install supports both the prompt-guard lab and the OBO lab). It's the same install path the OBO lab uses; doing it here once means [090](070-obo-entra.md) just needs the OBO-specific config on top.

## Lab Objectives

- Install the Kubernetes Gateway API CRDs (`v1.5.0`) if not already present
- Install the enterprise-agentgateway CRDs chart
- Install the enterprise-agentgateway controller with `tokenExchange.enabled: true`
- Apply `EnterpriseAgentgatewayParameters` so the dataplane pods get `STS_URI` and `STS_AUTH_TOKEN`

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) → [002](002-licenses-and-secrets.md) → [003](003-install-kagent-enterprise.md)
- `AGW_LICENSE_KEY` exported (validated only against agentgateway - separate from `AGENTGATEWAY_LICENSE_KEY` used by the Gloo Operator in 003)
- For Entra OBO ([070](070-obo-entra.md)): `TENANT_ID` exported

## 1. Install the Gateway API CRDs

```bash
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.5.0/standard-install.yaml
```

If your cluster already has the standard Gateway API CRDs (the Gloo Operator install in [020](003-install-kagent-enterprise.md) usually pulls them in via Gloo Gateway), skip this step.

## 2. Install the Enterprise Agentgateway CRDs

```bash
helm install agentgateway-crds \
 oci://us-docker.pkg.dev/solo-public/enterprise-agentgateway/charts/enterprise-agentgateway-crds \
 --version v2.2.0 \
 --namespace agentgateway-system \
 --create-namespace
```

## 3. License Secret

```bash
kubectl create secret generic enterprise-agentgateway-license \
 -n agentgateway-system \
 --from-literal=enterprise-agentgateway-license-key="${AGW_LICENSE_KEY}"
```

## 4. Install the Controller

Create `agw-values.yaml`:

```yaml
tokenExchange:
 enabled: true
 issuer: "http://enterprise-agentgateway.agentgateway-system.svc.cluster.local:7777"
 subjectValidator:
 validatorType: "remote"
 remoteConfig:
 url: "https://login.microsoftonline.com/${TENANT_ID}/discovery/v2.0/keys"
 apiValidator:
 validatorType: "k8s"
 actorValidator:
 validatorType: "k8s"

controller:
 service:
 ports:
 tokenExchange: 7777

licensing:
 createSecret: false
 secretName: "enterprise-agentgateway-license"
```

For the OBO scenario, the **subject** token is the user's Entra access token, so the subject validator must use the Entra JWKS endpoint (`login.microsoftonline.com/${TENANT_ID}/discovery/v2.0/keys`) rather than the Kubernetes API server JWKS. If you don't intend to use Entra OBO at all, you can drop the `subjectValidator` block now and add it later before running [090](070-obo-entra.md).

```bash
envsubst < agw-values.yaml > /tmp/agw-values.rendered.yaml

helm install agentgateway \
 oci://us-docker.pkg.dev/solo-public/enterprise-agentgateway/charts/enterprise-agentgateway \
 --version v2.2.0 \
 --namespace agentgateway-system \
 --create-namespace \
 -f /tmp/agw-values.rendered.yaml
```

## 5. Wire the Dataplane to the STS Endpoint

The dataplane pods (the agentgateway proxy itself) need to know where the token-exchange service lives and how to authenticate to it. Apply an `EnterpriseAgentgatewayParameters` that sets `STS_URI` and `STS_AUTH_TOKEN` env vars:

```bash
kubectl apply -f - <<EOF
apiVersion: enterpriseagentgateway.solo.io/v1alpha1
kind: EnterpriseAgentgatewayParameters
metadata:
 name: agentgateway-entra-testing-enterprise
 namespace: agentgateway-system
spec:
 logging:
 level: debug
 env:
 - name: STS_URI
 value: "http://enterprise-agentgateway.agentgateway-system.svc.cluster.local:7777/token"
 - name: STS_AUTH_TOKEN
 value: "/var/run/secrets/xds-tokens/xds-token"
EOF
```

When you create the `Gateway` resource in [090 step 7](070-obo-entra.md#step-7--create-the-gateway-deploy-an-in-cluster-llm-proxy-and-attach-the-entra-obo-policy) (or in [070](040-prompt-guards.md) for prompt guards), point its `spec.infrastructure.parametersRef` at this `EnterpriseAgentgatewayParameters` object so the dataplane pods inherit `STS_URI` and `STS_AUTH_TOKEN`.

## Verify

```bash
kubectl get pods -n agentgateway-system
kubectl get svc enterprise-agentgateway -n agentgateway-system
kubectl logs deployment/enterprise-agentgateway -n agentgateway-system | grep -Ei "token exchange|AGW server"
```

You should see a healthy controller pod, a Service exposing port 7777 (token exchange), and log lines confirming the token-exchange server started.

## Next

- [030 - Gateway Access Logs](050-access-logs.md)
- [070 - Prompt Guards](040-prompt-guards.md)
- [090 - Microsoft Entra ID OBO end-to-end](070-obo-entra.md)
