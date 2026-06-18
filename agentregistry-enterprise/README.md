# AgentRegistry Enterprise Workshop

A hands-on lab series for **Solo.io AgentRegistry Enterprise** (`arctl` + `ar.dev/v1alpha1` CRDs). The labs cover installing the registry on Kubernetes, wiring it to an OIDC identity provider (Microsoft Entra ID or Keycloak), registering AWS Bedrock AgentCore and kagent runtimes, deploying agents and MCP servers, applying AccessPolicy-based RBAC, and surfacing telemetry in the AgentRegistry UI.

All manifests, agent code, Terraform, and ConfigMap patches referenced by the labs live under [`assets/`](assets/) in this repo, mirrored from the upstream demo repo.

## Prerequisites

Before starting this workshop, you will need:

- A running Kubernetes cluster (GKE, EKS, AKS, or Kind) with `kubectl` access
- `helm` v3 installed
- The Enterprise `arctl` CLI (installed in [001](001-install-arctl.md))
- An OIDC provider — Microsoft Entra ID ([020](020-setup-entra.md)) or Keycloak ([021](021-setup-keycloak.md))
- (Optional) AWS account + `aws` CLI for the AWS Bedrock AgentCore runtime ([050](050-aws-provider.md))
- (Optional) An existing kagent installation for the kagent runtime ([051](051-kagent-provider.md))

# Table of Contents

- [Installation](#installation)
- [Identity Provider Setup](#identity-provider-setup)
- [Cluster Access](#cluster-access)
- [Runtimes (Providers)](#runtimes-providers)
- [Agents](#agents)
- [MCP Servers](#mcp-servers)
- [AccessPolicy / RBAC](#accesspolicy--rbac)
- [Observability](#observability)
- [GitOps / CI-CD](#gitops--ci-cd)
- [Cleanup](#cleanup)

---

## Installation

> **Start here.** All other labs depend on the CLI and the AgentRegistry Enterprise install.

- [000 — Overview & Architecture](000-overview.md)
- [001 — Install the `arctl` Enterprise CLI](001-install-arctl.md)
- [030 — Install AgentRegistry Enterprise (Helm)](030-install-agentregistry-helm.md)

---

## Identity Provider Setup

Pick one OIDC backend before installing. Both expose the same `groups` (or `roles`) claim model that AccessPolicy consumes.

- [020 — Microsoft Entra ID (Azure AD) OIDC](020-setup-entra.md)
- [021 — Keycloak OIDC](021-setup-keycloak.md)
- [040 — Authenticate `arctl` (device-code, manual token)](040-arctl-auth.md)

---

## Cluster Access

- [010 — Cluster Prerequisites (Private EKS, EBS CSI, Terraform)](010-cluster-prereqs.md)
- [035 — Private-Cluster Routing via Istio Gateway + NLB](035-private-cluster-istio-routing.md)

---

## Runtimes (Providers)

A **Runtime** (sometimes called a Provider) is where AgentRegistry actually deploys an agent: AWS Bedrock AgentCore, kagent, etc.

- [050 — AWS Bedrock AgentCore Provider](050-aws-provider.md)
- [051 — kagent Runtime](051-kagent-provider.md)

---

## Agents

- [060 — Deploy a Demo Chatbot on AWS Bedrock AgentCore](060-deploy-demochatbot-on-aws.md)
- [061 — Deploy `k8shelper` on kagent (Anthropic + Gemini variants)](061-deploy-k8shelper-on-kagent.md)

---

## MCP Servers

- [070 — Register a Local stdio MCP Server (`demo-tools`)](070-register-local-mcp.md)
- [071 — Register a Remote Streamable-HTTP MCP Server (GitHub Copilot)](071-register-github-copilot-mcp.md)
- [072 — Wire an MCP Server to an Agent](072-wire-mcp-to-agent.md)

---

## AccessPolicy / RBAC

- [080 — AccessPolicy for Entra Groups, kagent fan-out, and MCP tools](080-access-policies.md)
- [081 — Approval Workflows (admin gating of catalog submissions)](081-approval-workflows.md)

---

## Observability

- [090 — Tracing Setup (kagent + AWS AgentCore runtimes)](090-observability-tracing.md)
- [091 — Trace Fan-Out Workaround for kagent](091-trace-fanout-workaround.md)

---

## GitOps / CI-CD

- [095 — Register Agents and MCP Servers from a GitLab Pipeline](095-gitops-gitlab-ci.md)

---

## Cleanup

- [099 — Cleanup & Common Troubleshooting](099-cleanup.md)

---

## Tracks

Curated learning paths that walk through a subset of the labs in a recommended order. See [`tracks/`](tracks/):

- [`aws-track.md`](tracks/aws-track.md) — Install → Entra → AWS provider → demochatbot
- [`kagent-track.md`](tracks/kagent-track.md) — Install → Keycloak → kagent runtime → k8shelper + MCP

---

## Use Cases

- Install AgentRegistry Enterprise on Kubernetes with OIDC
- Federate AWS Bedrock AgentCore and kagent under a single agent catalog and RBAC model
- Register agents either by repo source (cloned + built by AgentRegistry) or by pre-built container image (BYO image)
- Register MCP servers (`stdio` local, `streamable-http` remote) and wire them into agents
- Enforce AccessPolicy-based RBAC against Entra group object IDs, Entra app roles, or Keycloak groups
- Surface traces from all runtimes in the AgentRegistry dashboard via the bundled OTel Collector + ClickHouse
- Drive registration through `arctl apply` in GitLab CI/CD against private EKS clusters

## Validated On

- AgentRegistry Enterprise chart `2026.5.3` / `2026.05.0` (`2026.6.0` for [081 approval workflows](081-approval-workflows.md))
- `arctl` v2026.5.3 / v2026.5.4
- Kubernetes 1.29+
- AWS Bedrock AgentCore (us-east-1)
- kagent management chart with the `solo-enterprise-telemetry-collector`

## Repo Layout

```
agentregistry-enterprise/
├── README.md                     # this file
├── 000-overview.md               # architecture and lab flow
├── 001-install-arctl.md          # CLI install
├── 010-cluster-prereqs.md        # EKS + EBS CSI + StorageClass (+ Terraform under assets/private-eks)
├── 020-setup-entra.md            # Entra ID OIDC
├── 021-setup-keycloak.md         # Keycloak OIDC
├── 030-install-agentregistry-helm.md
├── 035-private-cluster-istio-routing.md
├── 040-arctl-auth.md             # device-code / manual token
├── 050-aws-provider.md
├── 051-kagent-provider.md
├── 060-deploy-demochatbot-on-aws.md
├── 061-deploy-k8shelper-on-kagent.md
├── 070-register-local-mcp.md
├── 071-register-github-copilot-mcp.md
├── 072-wire-mcp-to-agent.md
├── 080-access-policies.md
├── 081-approval-workflows.md
├── 090-observability-tracing.md
├── 091-trace-fanout-workaround.md
├── 095-gitops-gitlab-ci.md
├── 099-cleanup.md
├── tracks/
│   ├── aws-track.md
│   └── kagent-track.md
└── assets/                       # YAML manifests, agent source, Terraform, ConfigMap patches
    ├── access-policies/          # parameterized AccessPolicy templates
    ├── demochatbot-a2a/
    ├── k8shelper-anthropic/
    ├── k8shelper-gemini/
    ├── mcp/
    ├── observability/
    ├── private-eks/              # Terraform for a private EKS cluster (no tfstate committed)
    └── providers/kagent/
```
