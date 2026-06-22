# Kagent Enterprise Workshop

A hands-on lab series for **Solo Enterprise for kagent** — Solo.io's enterprise distribution of kagent — packaged with `AccessPolicy` for fine-grained authz, OIDC for UI + API login, MCP server lifecycle management, and integration with Solo Istio (Ambient), Gloo Gateway, and Enterprise Agentgateway.

The workshop is built around two ideas:

1. **Four setup labs, soup to nuts.** [001](001-baseline-setup.md) → [002](002-licenses-and-secrets.md) → [003](003-install-kagent-enterprise.md) → [004](004-install-enterprise-agentgateway.md) takes you from a bare Kubernetes cluster to a working Solo Enterprise for kagent install + Enterprise Agentgateway. You only run setup once.

2. **Independent unit-of-value labs.** Every lab numbered 010 and up is standalone — it states what it needs from the baseline, walks through one capability, and has its own `## Cleanup` section that returns the cluster to the post-baseline state. Run them in any order, run cleanup, move on.

All YAML, agent code, Terraform, and ConfigMap patches referenced by the labs are in [`assets/`](assets/) — no external private references.

## Prerequisites

- A running Kubernetes cluster (≥ 1.29) with a default `StorageClass` and a working `LoadBalancer` Service controller. The workshop is validated on **GKE Standard**.
- `kubectl`, `helm` v3, `openssl`
- Solo trial license keys for **kagent-enterprise**, **gloo-gateway**, and **agentgateway**
- An LLM provider API key — `OPENAI_API_KEY` is the workshop default; Anthropic also supported
- (For [060 — Pinniped + Keycloak](060-pinniped-keycloak.md)) — that lab brings its own Keycloak; no extra prereqs
- (For [070 — Entra OBO](070-obo-entra.md)) — a Microsoft Entra tenant

# Table of Contents

- [Setup (mandatory)](#setup-mandatory)
- [MCP Foundations](#mcp-foundations)
- [Validate Your Install](#validate-your-install)
- [Access Policies (RBAC)](#access-policies-rbac)
- [Security & Guardrails](#security--guardrails)
- [Observability](#observability)
- [Kubernetes API Authn via Pinniped + Keycloak](#kubernetes-api-authn-via-pinniped--keycloak)
- [Microsoft Entra ID OBO](#microsoft-entra-id-obo)
- [Cleanup](#cleanup)
- [Appendix](#appendix)

---

## Setup (mandatory)

> Four labs. Run them in order, once. After this, every lab from 010 onwards is independent.

- [001 — Baseline Setup](001-baseline-setup.md) — cluster prereqs check + local tools + (optional) GKE provisioning
- [002 — Licenses, Namespace, and Secrets](002-licenses-and-secrets.md) — license env vars + `kagent` namespace + LLM/OIDC/JWT secrets
- [003 — Install Kagent Enterprise](003-install-kagent-enterprise.md) — Gloo Operator + Solo Istio Ambient + Gloo Gateway + Kagent Enterprise
- [004 — Install Enterprise Agentgateway](004-install-enterprise-agentgateway.md)

---

## MCP Foundations

- [010 — Declarative MCP Server + Agent](010-mcp-connection-agent-config.md)
- [011 — Agent A2A Skills Metadata](011-agent-skills.md)
- [012 — Build a Custom Python MCP Server](012-build-custom-mcp-server.md)

---

## Validate Your Install

- [020 — Troubleshoot a Broken Pod with the `k8s-agent`](020-troubleshoot-pod.md)

---

## Access Policies (RBAC)

- [030 — `AccessPolicy`: Agent → MCP Server](030-accesspolicy-agent-to-mcp.md)
- [031 — `AccessPolicy`: UserGroup → Agent (OIDC JWT)](031-accesspolicy-usergroup.md)

---

## Security & Guardrails

- [040 — Prompt Guards (Block Specific Prompts at the Gateway)](040-prompt-guards.md)
- [041 — Platform RBAC for kagent CRDs](041-platform-rbac.md)

---

## Observability

- [050 — Gateway Access Logs (kgateway `HTTPListenerPolicy`)](050-access-logs.md)

---

## Kubernetes API Authn via Pinniped + Keycloak

- [060 — Kubernetes OIDC Authentication with Keycloak + Pinniped](060-pinniped-keycloak.md)

> This lab is **independent of the rest of the workshop** — it sets up OIDC for `kubectl` itself, not for kagent. It brings its own Keycloak. You can run it without any of the setup labs (001–004) — it only needs a cluster.

---

## Microsoft Entra ID OBO

- [070 — Microsoft Entra ID OBO with Enterprise Agentgateway](070-obo-entra.md)

> **Different install model.** The OBO lab uses the direct-Helm install of `kagent-mgmt` + `kagent-crds` + `kagent-enterprise` at chart `0.3.12`, **not** the Gloo Operator install from [003](003-install-kagent-enterprise.md). The OBO lab is self-contained — it sets up its own kagent + Agentgateway from scratch. Don't try to run it against a cluster that already has the Gloo Operator install.

---

## Cleanup

- [099 — Cleanup](099-cleanup.md) — full baseline teardown (each unit-of-value lab has its own cleanup too)

---

## Appendix

- [Appendix — Kagent OSS + OpenShell / NemoClaw Sandbox](appendix-nemoclaw-oss.md) — different product line; included for completeness

---

## Tracks

Curated paths under [`tracks/`](tracks/):

- [`install-track.md`](tracks/install-track.md) — Baseline → first MCP-backed agent
- [`policy-track.md`](tracks/policy-track.md) — Both AccessPolicy flavors + prompt guards + platform RBAC
- [`obo-track.md`](tracks/obo-track.md) — Entra OBO end-to-end (skips the Gloo Operator install)

---

## Use Cases

- Install Solo Enterprise for kagent on Kubernetes (Gloo-Operator-driven and direct-Helm install paths)
- Register MCP servers declaratively (kagent acts as a package manager) and use them from `Declarative` and `BYO` Agents
- Surface A2A skills metadata for agent discovery
- Enforce `AccessPolicy` against MCP servers — restrict which tools an agent can call (`Agent` subject) and which users can talk to an agent (`UserGroup` subject)
- Front LLM and MCP traffic with **enterprise-agentgateway** for prompt guards, observability, and OIDC-fronted access
- Use Pinniped + Keycloak to authenticate `kubectl` itself against the cluster
- Wire end-to-end **Entra OBO** so a user logs into the kagent UI, the user's token propagates through the agent, and agentgateway exchanges it for a downstream-scoped token

## Validated Versions

| Component | Version | Used in |
|-----------|---------|---------|
| Gloo Operator | `0.4.0` | [003](003-install-kagent-enterprise.md) |
| Solo Istio (ServiceMeshController, Ambient) | `1.27.1` | [003](003-install-kagent-enterprise.md) |
| Gloo Gateway (GatewayController) | `2.0.0` | [003](003-install-kagent-enterprise.md) |
| Kagent (Enterprise) controller | `0.1.5` | [003](003-install-kagent-enterprise.md) |
| Kagent / management / CRDs charts (OBO direct-Helm path) | `0.3.12` | [070](070-obo-entra.md) |
| enterprise-agentgateway | `v2.2.0` | [004](004-install-enterprise-agentgateway.md), [070](070-obo-entra.md) |
| Kubernetes Gateway API CRDs | `v1.5.0` | [004](004-install-enterprise-agentgateway.md), [070](070-obo-entra.md) |
| Keycloak image | `quay.io/keycloak/keycloak:26.0` | [060](060-pinniped-keycloak.md) |
| Pinniped Concierge | `latest` (from `get.pinniped.dev/latest`) | [060](060-pinniped-keycloak.md) |
| Anthropic model | `claude-haiku-4-5-20251001` (OBO) / `claude-sonnet-4-6` (prompt guards) | [070](070-obo-entra.md), [040](040-prompt-guards.md) |
| Terraform google provider | `~> 5.0` | [001](001-baseline-setup.md) |

## Repo Layout

```
kagent-enterprise/
├── README.md                            # this file
├── 001-baseline-setup.md                # cluster prereqs + tools + (optional) GKE
├── 002-licenses-and-secrets.md          # licenses + namespace + Secrets + jwt key
├── 003-install-kagent-enterprise.md     # Gloo Operator install
├── 004-install-enterprise-agentgateway.md
├── 010-mcp-connection-agent-config.md   # declarative MCPServer + Agent
├── 011-agent-skills.md                  # A2A skills metadata
├── 012-build-custom-mcp-server.md       # Pharma MCP server (Python)
├── 020-troubleshoot-pod.md              # k8s-agent fixes a broken Pod
├── 030-accesspolicy-agent-to-mcp.md     # AccessPolicy w/ Agent subject
├── 031-accesspolicy-usergroup.md        # AccessPolicy w/ UserGroup subject
├── 040-prompt-guards.md                 # EnterpriseAgentgatewayPolicy.promptGuard
├── 041-platform-rbac.md                 # K8s RBAC for kagent CRDs
├── 050-access-logs.md                   # gateway access-log HTTPListenerPolicy
├── 060-pinniped-keycloak.md             # K8s OIDC via Keycloak + Pinniped (standalone)
├── 070-obo-entra.md                     # Entra OBO end-to-end (different install model)
├── 099-cleanup.md                       # full teardown
├── appendix-nemoclaw-oss.md             # OSS kagent + OpenShell/NemoClaw
├── tracks/
│   ├── install-track.md
│   ├── policy-track.md
│   └── obo-track.md
└── assets/
    ├── gke-terraform/                   # main.tf, variables.tf, terraform.tfvars.example
    ├── llm-obo-proxy/                   # FastAPI proxy for OBO (app.py, deployment.yaml)
    ├── mcp-server-example/              # pharma_mcp_server.py
    ├── obo/                             # ui-https-gateway.yaml
    └── observability/                   # access-logs HTTPListenerPolicy
```
