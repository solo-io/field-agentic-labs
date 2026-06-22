# `AccessPolicy`: UserGroup → Agent (OIDC JWT)

The `UserGroup` subject form of `AccessPolicy` controls **which users can invoke an Agent** based on OIDC JWT claims. It's the user-facing complement to the `Agent` subject form in [060](030-accesspolicy-agent-to-mcp.md): that lab restricted what an Agent could do; this one restricts who can talk to an Agent.

The policy checks a specific claim (`sub`, `groups`, `email`, `preferred_username`, …) against an expected value, using JWKS for token signature verification. Validation happens at the Agent's waypoint proxy (Solo Istio Ambient), which means the target namespace must be mesh-enrolled.

This lab uses `preferred_username` so you don't need a `groups` mapper on your Keycloak client (`preferred_username` is in tokens by default).

## Lab Objectives

- Create a dedicated mesh-enrolled namespace (`policies`)
- Deploy a `platform-agent` Declarative Agent into it
- Apply a `UserGroup` `AccessPolicy` that **denies** the user `reader` from invoking the Agent
- Confirm `reader` is rejected (403) while everyone else still has access

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) → [002](002-licenses-and-secrets.md) → [003](003-install-kagent-enterprise.md)
## 1. Verify the Token Carries `preferred_username`

Adapt the URL, client, and credentials to your IdP:

```bash
TOKEN=$(curl -s -X POST \
  "https://demo-keycloak-907026730415.us-east4.run.app/realms/kagent-dev/protocol/openid-connect/token" \
  -d "grant_type=password" \
  -d "client_id=kagent-backend" \
  -d "client_secret=<your-client-secret>" \
  -d "username=<username>" \
  -d "password=<password>" | jq -r '.access_token')

echo $TOKEN | cut -d. -f2 | base64 -d 2>/dev/null | jq '{preferred_username, aud, iss}'
```

You should see `preferred_username`, `aud` (e.g., `account`), and `iss` (your realm URL).

## 2. Create a Mesh-Enrolled Namespace

The Agent's waypoint proxy is what enforces the JWT check. It only gets installed if the namespace is enrolled into Solo Istio Ambient.

```bash
kubectl create ns policies
kubectl label namespaces policies istio.io/dataplane-mode=ambient
```

## 3. Deploy the Agent

```bash
kubectl apply -f - <<EOF
apiVersion: kagent.dev/v1alpha2
kind: Agent
metadata:
  name: platform-agent
  namespace: policies
  labels:
    kagent.solo.io/waypoint: "true"
spec:
  description: Platform engineering agent for cluster operations
  type: Declarative
  declarative:
    modelConfig: model-config
    systemMessage: |-
      You are a platform engineering assistant that helps with Kubernetes cluster operations.

      # Instructions

      - If user question is unclear, ask for clarification before running any tools
      - Always be helpful and friendly
      - If you don't know how to answer the question DO NOT make things up
        respond with "Sorry, I don't know how to answer that" and ask the user to further clarify the question

      # Response format
      - ALWAYS format your response as Markdown
      - Your response will include a summary of actions you took and an explanation of the result
EOF
```

> `kagent.solo.io/waypoint: "true"` is required — the policy is enforced by the waypoint proxy, not by the agent process.

Open the kagent UI, find `platform-agent` in the `policies` namespace, and prompt `"what can you do?"` to confirm baseline access works.

## 4. Apply a `DENY` Policy That Blocks the `reader` User

```bash
kubectl apply -f - <<EOF
apiVersion: policy.kagent-enterprise.solo.io/v1alpha1
kind: AccessPolicy
metadata:
  name: deny-reader-agent-access
  namespace: policies
spec:
  action: DENY
  from:
    subjects:
      - kind: UserGroup
        name: reader-user
        userGroup:
          claimName: "preferred_username"
          claimValue: "reader"
          issuer: "https://demo-keycloak-907026730415.us-east4.run.app/realms/kagent-dev"
          audiences:
            - "account"
          jwksKey:
            inline: |
              {
                "keys": [
                  {
                    "alg": "RS256",
                    "e": "AQAB",
                    "kid": "JWxVLtipR-Q6wF2zmQKEoxbFhqwibK2aKNLyRqNxdj4",
                    "kty": "RSA",
                    "use": "sig",
                    "n": "5ApthhEwr6U00Coa0_572OytJXbVZKgl-myirM2m4GSrVfaKus41GEPHHXMzyGDPgHU7Rb4o0yzB-obkgz0zo2jnjv1zSx88BgdhhdE0BX2ULFDj67jVYdFZdCOoBr1_xJ5LEjQArHxfywZxW4a0egc3JaIwo-3qSSlRnD1KV2uzTG9FoDpvJLn1ZzdMgoTHuxIMla6WdgPDswVD8nrQM0I_1VGyGC0l2dICUEiqN0QrZen--U70J6EU6hd8vi_9qmALhjoSEASH2Z2sHco4Shv_aVx0BM-zN5UJWz4VF51Ag_KgcePS5Co7iVM0FUwMNWauWhPDPLWiXoUJvUWVPw"
                  }
                ]
              }
  targetRef:
    kind: Agent
    name: platform-agent
EOF
```

The inline JWKS is what the waypoint uses to verify the token signature. Production setups use `jwksKey.remote` against the IdP's JWKS URL instead — see your IdP docs.

## 5. Verify

Log into the UI as `reader` and try to prompt `platform-agent`. You should get a 403.

Log in as another user (one whose `preferred_username` is not `reader`) and the prompt should work normally.

## How It Works

1. Applying the `AccessPolicy` triggers the policy controller, which writes a JWT authentication requirement onto the Agent's waypoint proxy.
2. Every incoming request to the Agent must carry a valid JWT matching the configured `issuer`, `audiences`, and `jwksKey`.
3. The proxy evaluates a CEL expression like `jwt.preferred_username == "reader"` against the token's claims.
4. With `action: DENY`, matching tokens are rejected with **403 Forbidden**. With `action: ALLOW`, only matching tokens are permitted. Invalid or missing tokens always return **401 Unauthorized**.

Common claim choices:

| Claim | When to use it |
|---|---|
| `preferred_username` | Username-based RBAC against Keycloak. No mapper required. |
| `groups` | Group-based RBAC. Requires a group mapper on the OIDC client (see [080](060-pinniped-keycloak.md#26-configure-group-mapper) for the Keycloak procedure). |
| `email` | Email-based RBAC. Available by default in most IdPs. |
| `sub` | Stable subject ID. Best for service-account-like principals. |

## Cleanup

```bash
kubectl delete accesspolicy deny-reader-agent-access -n policies --ignore-not-found
kubectl delete agent platform-agent -n policies --ignore-not-found
kubectl delete modelconfig model-config -n policies --ignore-not-found
kubectl delete secret kagent-anthropic -n policies --ignore-not-found
kubectl delete namespace policies --ignore-not-found
```

## Next

- [070 — Prompt Guards](040-prompt-guards.md) — block prompts at the gateway based on regex
- [080 — Kubernetes OIDC Auth with Pinniped + Keycloak](060-pinniped-keycloak.md) — same Keycloak, different purpose: authenticating `kubectl`
