# Prompt Guards (Block Specific Prompts at the Gateway)

Enterprise Agentgateway can short-circuit a request before it ever reaches the LLM provider, based on regex matches against the request body. This lab attaches an `EnterpriseAgentgatewayPolicy` with a `promptGuard.request.regex` rule to an `HTTPRoute` (the `claude` route - Anthropic backend) and demonstrates a 403 response on a matching prompt.

## Lab Objectives

- Apply an `EnterpriseAgentgatewayPolicy` that rejects any prompt containing `"credit card"`
- Confirm matching prompts get **403 Forbidden** instead of being proxied
- Confirm non-matching prompts pass through normally

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) → [002](002-licenses-and-secrets.md) → [003](003-install-kagent-enterprise.md) → [004](004-install-enterprise-agentgateway.md)
- An **Anthropic API key** (the gateway proxies to Anthropic so you can prove the guard intercepts on the way out):

 ```bash
 export ANTHROPIC_API_KEY=<your-anthropic-api-key>
 ```

## 1. Stand Up an Anthropic Backend + Route

The prompt-guard policy in step 2 targets an `HTTPRoute` named `claude`. This step creates that route + the Anthropic `AgentgatewayBackend` it points at, so the lab is self-contained.

```bash
# Anthropic API key Secret
kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
 name: anthropic-secret
 namespace: agentgateway-system
type: Opaque
stringData:
 Authorization: "${ANTHROPIC_API_KEY}"
EOF

# Anthropic AgentgatewayBackend + HTTPRoute named `claude`
kubectl apply -f - <<EOF
apiVersion: agentgateway.dev/v1alpha1
kind: AgentgatewayBackend
metadata:
 name: anthropic
 namespace: agentgateway-system
spec:
 ai:
 provider:
 anthropic:
 model: "claude-sonnet-4-6"
 policies:
 auth:
 secretRef:
 name: anthropic-secret
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
 name: claude
 namespace: agentgateway-system
spec:
 parentRefs:
 - name: agentgateway-proxy
 namespace: agentgateway-system
 rules:
 - matches:
 - path:
 type: PathPrefix
 value: /anthropic
 filters:
 - type: URLRewrite
 urlRewrite:
 path:
 type: ReplaceFullPath
 replaceFullPath: /v1/chat/completions
 backendRefs:
 - name: anthropic
 namespace: agentgateway-system
 group: agentgateway.dev
 kind: AgentgatewayBackend
EOF
```

> Adjust the `parentRefs.name` (`agentgateway-proxy`) to match the actual `Gateway` resource that the install in [004](004-install-enterprise-agentgateway.md) creates. Confirm with `kubectl get gateway -n agentgateway-system`.

Get the gateway address:

```bash
export INGRESS_GW_ADDRESS=$(kubectl get gateway agentgateway-proxy -n agentgateway-system \
 -o jsonpath='{.status.addresses[0].value}')
echo "Gateway: ${INGRESS_GW_ADDRESS}"
```

## 2. Apply the Prompt-Guard Policy

```bash
kubectl apply -f - <<EOF
apiVersion: enterpriseagentgateway.solo.io/v1alpha1
kind: EnterpriseAgentgatewayPolicy
metadata:
 name: credit-guard-prompt-guard
 namespace: agentgateway-system
spec:
 targetRefs:
 - group: gateway.networking.k8s.io
 kind: HTTPRoute
 name: claude
 backend:
 ai:
 promptGuard:
 request:
 - response:
 message: "Rejected due to inappropriate content"
 regex:
 action: Reject
 matches:
 - "credit card"
EOF
```

What this says:

- `targetRefs` binds the policy to the `claude` `HTTPRoute`.
- `backend.ai.promptGuard.request[0].regex.action: Reject` - short-circuit before the backend call.
- `matches: ["credit card"]` - case-insensitive regex; matches anywhere in the request body.
- `response.message` - what the client sees in the 403 body.

## 3. Send a Matching Prompt and Confirm the 403

```bash
curl "$INGRESS_GW_ADDRESS:8080/anthropic" \
 -v \
 -H "content-type:application/json" \
 -H "anthropic-version: 2023-06-01" \
 -d '{
 "messages": [
 { "role": "system", "content": "You are a skilled cloud-native network engineer." },
 { "role": "user", "content": "What is a credit card?" }
 ]
 }' | jq
```

Expected:

```
< HTTP/1.1 403 Forbidden
< content-length: 37
< date: ...
```

with the body `Rejected due to inappropriate content`.

## 4. Send a Non-Matching Prompt to Confirm Pass-Through

```bash
curl "$INGRESS_GW_ADDRESS:8080/anthropic" \
 -H "content-type:application/json" \
 -H "anthropic-version: 2023-06-01" \
 -d '{
 "messages": [
 { "role": "system", "content": "You are a skilled cloud-native network engineer." },
 { "role": "user", "content": "Explain CNI plugins in 3 sentences." }
 ]
 }' | jq
```

You should see a normal 200 response with the model's answer.

## Cleanup

```bash
kubectl delete enterpriseagentgatewaypolicy credit-guard-prompt-guard -n agentgateway-system
```

## Variations

The `promptGuard.request` block supports more than `regex`. Common extensions:

- **`webhook`** - call an external moderation service and let it decide whether to allow the request.
- **`openai`** - use the OpenAI moderation API as a built-in classifier.
- **`pii`** - detect and (optionally) redact PII fields.

Each can be combined with the regex matcher - they're evaluated in order, and the first rejection wins.

The same `promptGuard` block also has a `response` side that scans the LLM's reply on the way back to the client, not just the request on the way in.

## What This Is *Not*

This is not a substitute for proper authentication or per-user access control. It blocks specific text patterns from reaching the backend, but a determined caller can simply rephrase. Combine prompt guards with:

- [031 - `UserGroup` `AccessPolicy`](031-accesspolicy-usergroup.md) - to limit who can call the agent at all
- [030 - `Agent` `AccessPolicy`](030-accesspolicy-agent-to-mcp.md) - to limit what tools the agent can call when called
- [070 - Entra OBO](070-obo-entra.md) - to preserve user identity all the way through to the LLM

## Next

- [071 - Platform RBAC for kagent CRDs](041-platform-rbac.md)
- [090 - Microsoft Entra ID OBO](070-obo-entra.md)
