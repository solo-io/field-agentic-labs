# Licenses, Namespace, and Secrets

Before installing kagent-enterprise, the cluster needs the `kagent` namespace, the LLM provider API key, the OIDC backend client secret (used by both the management UI and the runtime controller), and a private RSA key used by the controller to mint its own JWTs for token delegation.

## Lab Objectives

- Set every license / API-key environment variable the install needs
- Create the `kagent` namespace
- Create the `llm-api-keys` Secret for your LLM provider
- Create the `kagent-backend-secret` for the OIDC backend client secret
- Generate the RSA private key and create the `jwt` Secret used by the controller for OBO/JWT minting

## Prerequisites

- [001 - Baseline Setup](001-baseline-setup.md) completed
- Solo trial license keys (kagent-enterprise, gloo-gateway, agentgateway) and an LLM API key (`OPENAI_API_KEY` is the workshop default; Anthropic also supported)

## 1. Export Required Environment Variables

The Gloo Operator install needs **three** Solo enterprise license keys (kagent-enterprise, gloo-gateway, agentgateway) plus an LLM API key. The OIDC values are required if you're enabling UI login at install time.

```bash
export SOLO_LICENSE_KEY=<solo-istio-license>
export GLOO_GATEWAY_LICENSE_KEY=<gloo-gateway-license>
export AGENTGATEWAY_LICENSE_KEY=<agentgateway-license>

export OPENAI_API_KEY=<your-openai-api-key>
# (or, depending on which model you use in ModelConfig)
# export ANTHROPIC_API_KEY=<your-anthropic-api-key>

# OIDC client + secret values (required by Gloo Operator install in 020)
export OIDC_BACKEND=<oidc-backend-client-id>
export BACKEND_CLIENT_SECRET=<oidc-backend-client-secret>
export OIDC_ISSUER=<oidc-issuer-url>
```

> If you don't yet have OIDC values, skip the OIDC parts now - you can configure OIDC later with a `helm upgrade` (Gloo Operator path) or by editing the `KagentManagementController` / `KagentController` CRs.

## 2. Create the Namespace

```bash
kubectl create ns kagent
```

## 3. Create the LLM API-Key Secret

```bash
kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
 name: llm-api-keys
 namespace: kagent
type: Opaque
stringData:
 OPENAI_API_KEY: ${OPENAI_API_KEY}
EOF
```

If you'll use Anthropic instead, swap the key - the [070 prompt guards lab](040-prompt-guards.md) and [090 OBO lab](070-obo-entra.md) both expect `kagent-anthropic` / `ANTHROPIC_API_KEY` Secrets and you can create those the same way:

```bash
kubectl create secret generic kagent-anthropic \
 -n kagent \
 --from-literal=ANTHROPIC_API_KEY="${ANTHROPIC_API_KEY}" \
 --dry-run=client -o yaml | kubectl apply -f -
```

## 4. Create the OIDC Backend Secret

The Solo Enterprise UI backend and the runtime controller both read the same OIDC client secret from this Kubernetes Secret. Two keys are written - `clientSecret` and `secret` - so both consumers work without you needing to know which one each chart looks at.

```bash
kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
 name: kagent-backend-secret
 namespace: kagent
type: Opaque
stringData:
 clientSecret: ${BACKEND_CLIENT_SECRET}
 secret: ${BACKEND_CLIENT_SECRET}
EOF
```

## 5. Generate the JWT Signing Key (for token delegation)

When a user sends a request to an Agent, the user's OIDC token authenticates the request. The Agent then needs to make downstream calls on behalf of the user - that's the **token delegation** flow. The kagent controller mints its own JWTs for the system-level part of that flow, and those JWTs must be signed by a key the controller holds.

Generate a 2048-bit RSA private key and load it into a Kubernetes Secret named `jwt`:

```bash
openssl genpkey -algorithm RSA -out /tmp/key.pem -pkeyopt rsa_keygen_bits:2048

kubectl create secret generic jwt -n kagent --from-file=jwt=/tmp/key.pem
```

The Secret must be named exactly `jwt` and the key must be named exactly `jwt` - the controller looks for `jwt.jwt` to read the private key bytes.

> **Important - OBO interaction:** in the [090 OBO lab](070-obo-entra.md), you set `oidc.skipOBO: true` in the kagent runtime values so **agentgateway** handles OBO instead of kagent. When `skipOBO: false`, the kagent controller mints its own JWT (signed with this key) and passes that to the agent instead of the raw Entra access token - and agentgateway's STS cannot validate that kagent-issued token against the Entra JWKS, so the token exchange fails. The `jwt` Secret is still useful even with `skipOBO: true` for other JWT-minting paths; create it now and you'll have it.

Clean up the key file on disk after the Secret is created:

```bash
rm -f /tmp/key.pem
```

## Verify

```bash
kubectl get secret -n kagent
```

Expected - at minimum:

```
NAME TYPE DATA AGE
jwt Opaque 1 <age>
kagent-backend-secret Opaque 2 <age>
llm-api-keys Opaque 1 <age>
```

## Next

- [020 - Install Kagent Enterprise (Helm)](003-install-kagent-enterprise.md) - canonical Gloo Operator install
- If you're going straight to the OBO lab: [090 - Entra OBO end-to-end](070-obo-entra.md) (creates its own Secrets and uses a different install model)
