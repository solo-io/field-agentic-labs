# Kagent Enterprise Workshop

A hands-on lab series for **Solo Enterprise for kagent** — Solo.io's enterprise distribution of kagent. The labs walk through provisioning a cluster, installing kagent-enterprise with OIDC, building and connecting MCP servers, applying `AccessPolicy` for both agent-to-MCP and user-to-agent flows, layering enterprise-agentgateway in front of LLM and MCP traffic, and finishing with the full Microsoft Entra ID **On-Behalf-Of (OBO)** token exchange end-to-end.

All YAML, agent code, Terraform, and the OBO Python proxy referenced by the labs live under [`assets/`](assets/) in this repo, mirrored from the upstream demo repo.

## Prerequisites

Before starting this workshop, you will need:

- A Kubernetes cluster (GKE is the validated target — see [001](001-provision-gke.md))
- `kubectl` configured against the cluster
- `helm` v3 installed
- Enterprise license keys for **kagent-enterprise** and **enterprise-agentgateway** (and Solo Istio + Gloo Gateway if you take the Gloo Operator path)
- An LLM API key (Anthropic, OpenAI, etc.)
- A Microsoft Entra ID tenant if you want the OBO lab ([090](090-obo-entra.md))

# Table of Contents

- [Installation](#installation)
- [Observability](#observability)
- [MCP Foundations](#mcp-foundations)
- [Validate Your Install](#validate-your-install)
- [Access Policies (RBAC)](#access-policies-rbac)
- [Security & Guardrails](#security--guardrails)
- [Kubernetes API Authn via Pinniped + Keycloak](#kubernetes-api-authn-via-pinniped--keycloak)
- [On-Behalf-Of (OBO) Token Exchange](#on-behalf-of-obo-token-exchange)
- [Cleanup](#cleanup)
- [Appendix](#appendix)

---

## Installation

> **Start here.** Everything else depends on a working cluster + kagent install.

- [000 — Overview & Architecture](000-overview.md)
- [001 — Provision a GKE Cluster (Terraform)](001-provision-gke.md)
- [010 — Licenses, Namespace, and Secrets](010-licenses-and-secrets.md)
- [020 — Install Kagent Enterprise (Helm)](020-install-kagent-enterprise.md) — canonical direct-Helm install
- [025 — Install Enterprise Agentgateway](025-install-enterprise-agentgateway.md)

---

## Observability

- [030 — Gateway Access Logs (kgateway `HTTPListenerPolicy`)](030-access-logs.md)

---

## MCP Foundations

- [040 — Declarative MCP Server + Agent](040-mcp-connection-agent-config.md)
- [041 — Agent A2A Skills Metadata](041-agent-skills.md)
- [042 — Build a Custom Python MCP Server (Pharmaceutical Example)](042-build-custom-mcp-server.md)

---

## Validate Your Install

- [050 — Troubleshooting a Broken Pod with the `k8s-agent`](050-troubleshooting-pod.md)

---

## Access Policies (RBAC)

- [060 — `AccessPolicy`: Agent → MCP Server (Declarative + BYO)](060-accesspolicy-agent-to-mcp.md)
- [061 — `AccessPolicy`: UserGroup → Agent (OIDC JWT)](061-accesspolicy-usergroup.md)

---

## Security & Guardrails

- [070 — Prompt Guards (Block Specific Prompts at the Gateway)](070-prompt-guards.md)
- [071 — Platform RBAC for kagent CRDs](071-platform-rbac.md)

---

## Kubernetes API Authn via Pinniped + Keycloak

- [080 — Kubernetes OIDC Authentication with Keycloak + Pinniped](080-k8s-token-passthrough-pinniped.md)

---

## On-Behalf-Of (OBO) Token Exchange

- [090 — Microsoft Entra ID OBO with Enterprise Agentgateway](090-obo-entra.md)

> The OBO lab uses a **direct-Helm** install of `kagent-mgmt` / `kagent-crds` / `kagent` at chart version `0.3.12`, which is a different install model from the Gloo Operator pattern in [020](020-install-kagent-enterprise.md). The lab installs both kagent and agentgateway from scratch and is self-contained — pair it with [001](001-provision-gke.md) directly, not with [020](020-install-kagent-enterprise.md).

---

## Cleanup

- [099 — Cleanup & Common Troubleshooting](099-cleanup.md)

---

## Appendix

Related content that is not part of the main Enterprise track.

- [Appendix — Kagent OSS + OpenShell / NemoClaw Sandbox](appendix-nemoclaw-oss.md)

> The Nemoclaw lab uses **kagent OSS** (`kagent-dev/kagent`), not Solo Enterprise for kagent. It's a different install path and product line — included for completeness because it lived in the source repo, but do not run it against a cluster that already has kagent-enterprise installed.

---

## Tracks

Curated learning paths under [`tracks/`](tracks/):

- [`install-track.md`](tracks/install-track.md) — Cluster → install → first MCP-backed agent
- [`policy-track.md`](tracks/policy-track.md) — Install → both `AccessPolicy` flavors → prompt guards → platform RBAC
- [`obo-track.md`](tracks/obo-track.md) — Cluster → Entra OBO end-to-end (skips the Gloo Operator install)

---

## Use Cases

- Install Solo Enterprise for kagent on Kubernetes (both Gloo-Operator-driven and direct-Helm install paths)
- Register MCP servers declaratively (kagent acts as a package manager) and use them from `Declarative` and `BYO` Agents
- Surface A2A skills metadata for agent discovery
- Enforce `AccessPolicy` against MCP servers — restrict which tools an agent can call (`Agent` subject) and which users can talk to an agent (`UserGroup` subject)
- Front LLM and MCP traffic with **enterprise-agentgateway** for prompt guards, observability, and OIDC-fronted access
- Use Pinniped + Keycloak to authenticate `kubectl` itself against the cluster
- Wire end-to-end **Entra OBO** so a user logs into the kagent UI, the user's token propagates through the agent, and agentgateway exchanges it for a downstream-scoped token before the in-cluster LLM proxy calls Anthropic

## Validated Versions

| Component | Version | Used in |
|-----------|---------|---------|
| Gloo Operator | `0.4.0` | [020](020-install-kagent-enterprise.md) |
| Solo Istio (ServiceMeshController, Ambient) | `1.27.1` | [020](020-install-kagent-enterprise.md) |
| Gloo Gateway (GatewayController) | `2.0.0` | [020](020-install-kagent-enterprise.md) |
| Kagent (Enterprise) controller | `0.1.5` | [020](020-install-kagent-enterprise.md) |
| Kagent / management / CRDs charts (OBO) | `0.3.12` | [090](090-obo-entra.md) |
| enterprise-agentgateway | `v2.2.0` | [025](025-install-enterprise-agentgateway.md), [090](090-obo-entra.md) |
| Kubernetes Gateway API CRDs | `v1.5.0` | [025](025-install-enterprise-agentgateway.md), [090](090-obo-entra.md) |
| Keycloak image | `quay.io/keycloak/keycloak:26.0` | [080](080-k8s-token-passthrough-pinniped.md) |
| Pinniped Concierge | `latest` (from `get.pinniped.dev/latest`) | [080](080-k8s-token-passthrough-pinniped.md) |
| Anthropic model | `claude-haiku-4-5-20251001` (OBO) / `claude-sonnet-4-6` (security) | [090](090-obo-entra.md), [070](070-prompt-guards.md) |
| Terraform google provider | `~> 5.0` | [001](001-provision-gke.md) |
| Proxy Python deps | `fastapi 0.116.1`, `httpx 0.28.1`, `PyJWT 2.10.1`, `uvicorn 0.35.0` | [090](090-obo-entra.md) |

> The Gloo Operator path (`KagentController` CRs, kagent at `0.1.5`) and the direct-Helm path (`kagent-enterprise` chart at `0.3.12`) are **two different install models** for the same Solo Enterprise for kagent product. They installed from different upstream chart streams during the workshop's authoring window. Pick one based on the lab you are running — do not try to mix them in a single cluster.

## Repo Layout

```
kagent-enterprise/
├── README.md                            # this file
├── 000-overview.md                      # architecture and lab flow
├── 001-provision-gke.md                 # Terraform → GKE cluster
├── 010-licenses-and-secrets.md          # licenses, namespace, jwt key, LLM secrets
├── 020-install-kagent-enterprise.md     # Gloo Operator install (canonical)
├── 025-install-enterprise-agentgateway.md
├── 030-access-logs.md                   # gateway access-log HTTPListenerPolicy
├── 040-mcp-connection-agent-config.md   # declarative MCPServer + Agent
├── 041-agent-skills.md                  # a2aConfig.skills
├── 042-build-custom-mcp-server.md       # pharma MCP server (Python)
├── 050-troubleshooting-pod.md           # k8s-agent fixes a broken Pod
├── 060-accesspolicy-agent-to-mcp.md     # AccessPolicy w/ Agent subject
├── 061-accesspolicy-usergroup.md        # AccessPolicy w/ UserGroup subject
├── 070-prompt-guards.md                 # EnterpriseAgentgatewayPolicy.promptGuard
├── 071-platform-rbac.md                 # K8s RBAC for kagent CRDs
├── 080-k8s-token-passthrough-pinniped.md
├── 090-obo-entra.md                     # Entra OBO end-to-end
├── 099-cleanup.md
├── appendix-nemoclaw-oss.md             # OSS kagent + OpenShell/NemoClaw
├── tracks/
│   ├── install-track.md
│   ├── policy-track.md
│   └── obo-track.md
└── assets/
    ├── gke-terraform/                   # main.tf, variables.tf, terraform.tfvars.example, .gitignore
    ├── llm-obo-proxy/                   # app.py, deployment.yaml, requirements.txt
    ├── mcp-server-example/              # pharma_mcp_server.py
    ├── obo/                             # ui-https-gateway.yaml
    └── observability/                   # kagent-gateway-access-logs.yaml
```
