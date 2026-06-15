# Track — Entra OBO End-to-End

A focused path for someone who only wants the Microsoft Entra OBO scenario: user logs in to the kagent UI, the user's token is propagated through the agent, agentgateway exchanges it for a downstream-scoped token, the in-cluster proxy validates it, and Anthropic gets called.

> **Different install model:** this track uses the direct-Helm install (`kagent-mgmt` + `kagent-crds` + `kagent-enterprise` at `0.3.12`, `enterprise-agentgateway` at `v2.2.0`), **not** the Gloo Operator install in [020](../020-install-kagent-enterprise.md). They install from different chart streams — don't mix them on the same cluster.

## Estimated Time

- ~2 hours end-to-end on a fresh cluster

## Prerequisites

- A Kubernetes cluster (GKE recommended — see [001](../001-provision-gke.md))
- Helm v3
- Enterprise license keys for **kagent-enterprise** and **enterprise-agentgateway**
- A Microsoft Entra ID tenant with admin access (you'll create three things: two app registrations + a security group)
- An Anthropic API key
- `openssl`, `envsubst`

## Order

1. [001 — Provision a GKE Cluster](../001-provision-gke.md) *(skip if you have a cluster)*
2. [090 — Microsoft Entra ID OBO end-to-end](../090-obo-entra.md)
3. [099 — Cleanup](../099-cleanup.md) — specifically the **OBO Stack**, **Enterprise Agentgateway**, and **Kagent Enterprise — Direct-Helm Path** sections

## Key Things the Lab Walks You Through

| Step | What it does |
|---|---|
| 1 | Two Entra app registrations: `kagent-backend` (confidential) + `kagent-ui` (SPA) with a delegated scope |
| 2 | Export the dozen-ish env vars the rest of the lab consumes |
| 3 | `kagent-enterprise-oidc-secret` + `kagent-anthropic` + `enterprise-kagent-license` Secrets |
| 4 | Direct Helm install of `kagent-mgmt` + `kagent-crds` + `kagent-enterprise` at `0.3.12` with `skipOBO: true` so agentgateway handles OBO instead of kagent |
| 5 | Locate `solo-enterprise-ui` (HTTP-only) and confirm the callback URI you'll register on the SPA app |
| 6 | Install `enterprise-agentgateway` at `v2.2.0` with `tokenExchange.enabled: true` and Entra JWKS as the `subjectValidator` |
| 7 | Gateway + HTTPS listener (self-signed) for SPA login → in-cluster Python `llm-obo-proxy` Deployment + Service → `EnterpriseAgentgatewayPolicy` with `entra` block in `ExchangeOnly` mode |
| 8 | Apply a `ModelConfig` with `apiKeyPassthrough: true` and a `KAGENT_PROPAGATE_TOKEN=true` Agent |

## Why `skipOBO: true` Matters

When `skipOBO: false`, the kagent controller mints its **own** JWT (signed with the `jwt` Secret) and passes that to the agent instead of the raw Entra access token. agentgateway's STS can't validate that kagent-issued token against the Entra JWKS — the exchange fails. **Always set `skipOBO: true`** when agentgateway is doing the OBO.

## Why an In-Cluster Proxy Instead of `api.anthropic.com`

The public Anthropic API expects a provider-native `x-api-key` header — it doesn't know what to do with an Entra bearer token. So the OBO target is an in-cluster Python `Service` that:

1. Validates the exchanged Entra token against `login.microsoftonline.com/{TENANT_ID}/discovery/v2.0/keys`.
2. Checks the `iss` claim is one of `{login.microsoftonline.com/{TENANT_ID}/v2.0, sts.windows.net/{TENANT_ID}/}`.
3. Optionally checks `aud` against `EXPECTED_AUDIENCES`.
4. Translates the OpenAI `/v1/chat/completions` request to an Anthropic `/v1/messages` call with the provider API key.
5. Translates the response back to the OpenAI shape so the agent doesn't have to know it was redirected.

The proxy source is in [`assets/llm-obo-proxy/app.py`](../assets/llm-obo-proxy/app.py) — 227 lines of FastAPI, `PyJWKClient`, and `httpx`.

## What You Will Have at the End

- A Solo Enterprise for kagent install (direct-Helm) with Entra OIDC for both the UI and the runtime controller
- Enterprise Agentgateway with the token-exchange service running on port 7777
- A Gateway with HTTPS termination (self-signed) routing browser SPA login to the UI
- A `Service` + `Deployment` + `HTTPRoute` for `llm-obo-proxy`, fronted by an `EnterpriseAgentgatewayPolicy` in `ExchangeOnly` mode pointed at the Entra OBO endpoint
- A `Declarative` Agent (`obo-demo-agent`) that propagates the user's token via `KAGENT_PROPAGATE_TOKEN=true` and uses a `ModelConfig` with `apiKeyPassthrough: true` pointing at the gateway's `/llm` route
- End-to-end proof — controller logs showing token exchange POSTs and the proxy logs showing validated tokens and successful Anthropic calls

## Next

- [policy-track](policy-track.md) — Layer `AccessPolicy` on top of the OBO setup for runtime-level RBAC
- [070 — Prompt Guards](../070-prompt-guards.md) — Add prompt guards on the same gateway
