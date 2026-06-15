# Prompt Guards (Block Specific Prompts at the Gateway)

Enterprise Agentgateway can short-circuit a request before it ever reaches the LLM provider, based on regex matches against the request body. This lab attaches an `EnterpriseAgentgatewayPolicy` with a `promptGuard.request.regex` rule to an `HTTPRoute` (the `claude` route — Anthropic backend) and demonstrates a 403 response on a matching prompt.

## Lab Objectives

- Apply an `EnterpriseAgentgatewayPolicy` that rejects any prompt containing `"credit card"`
- Confirm matching prompts get **403 Forbidden** instead of being proxied
- Confirm non-matching prompts pass through normally

## Prerequisites

- [025 — Enterprise Agentgateway installed](025-install-enterprise-agentgateway.md)
- A working `Gateway` + `HTTPRoute` named `claude` that proxies to an Anthropic backend (`AgentgatewayBackend`). The full setup is documented in the sister workshop `agentgateway-enterprise/security/prompt-guard/setup.md` in <https://github.com/AdminTurnedDevOps/agentic-demo-repo>. If you've completed [090 step 7c](090-obo-entra.md#7c-reference-direct-to-provider-path-not-suitable-for-obo) (the "reference only" direct-to-Anthropic section), you already have the `claude` `HTTPRoute`.
- `$INGRESS_GW_ADDRESS` — the external IP/hostname of the agentgateway Gateway

## 1. Apply the Prompt-Guard Policy

```bash
kubectl apply -f - <<EOF
apiVersion: enterpriseagentgateway.solo.io/v1alpha1
kind: EnterpriseAgentgatewayPolicy
metadata:
  name: credit-guard-prompt-guard
  namespace: agentgateway-system
  labels:
    app: agentgateway-route
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
- `backend.ai.promptGuard.request[0].regex.action: Reject` — short-circuit before the backend call.
- `matches: ["credit card"]` — case-insensitive regex; matches anywhere in the request body.
- `response.message` — what the client sees in the 403 body.

## 2. Send a Matching Prompt and Confirm the 403

```bash
curl "$INGRESS_GW_ADDRESS:8080/anthropic" \
  -v \
  -H "content-type:application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
        "messages": [
          { "role": "system",  "content": "You are a skilled cloud-native network engineer." },
          { "role": "user",    "content": "What is a credit card?" }
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

## 3. Send a Non-Matching Prompt to Confirm Pass-Through

```bash
curl "$INGRESS_GW_ADDRESS:8080/anthropic" \
  -H "content-type:application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
        "messages": [
          { "role": "system",  "content": "You are a skilled cloud-native network engineer." },
          { "role": "user",    "content": "Explain CNI plugins in 3 sentences." }
        ]
      }' | jq
```

You should see a normal 200 response with the model's answer.

## 4. Clean Up

```bash
kubectl delete enterpriseagentgatewaypolicy credit-guard-prompt-guard -n agentgateway-system
```

## Variations

The `promptGuard.request` block supports more than `regex`. Common extensions:

- **`webhook`** — call an external moderation service and let it decide whether to allow the request.
- **`openai`** — use the OpenAI moderation API as a built-in classifier.
- **`pii`** — detect and (optionally) redact PII fields.

Each can be combined with the regex matcher — they're evaluated in order, and the first rejection wins.

The same `promptGuard` block also has a `response` side that scans the LLM's reply on the way back to the client, not just the request on the way in.

## What This Is *Not*

This is not a substitute for proper authentication or per-user access control. It blocks specific text patterns from reaching the backend, but a determined caller can simply rephrase. Combine prompt guards with:

- [061 — `UserGroup` `AccessPolicy`](061-accesspolicy-usergroup.md) — to limit who can call the agent at all
- [060 — `Agent` `AccessPolicy`](060-accesspolicy-agent-to-mcp.md) — to limit what tools the agent can call when called
- [090 — Entra OBO](090-obo-entra.md) — to preserve user identity all the way through to the LLM

## Next

- [071 — Platform RBAC for kagent CRDs](071-platform-rbac.md)
- [090 — Microsoft Entra ID OBO](090-obo-entra.md)
