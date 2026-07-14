# Agentregistry Enterprise Workshop

A hands-on lab series for **Solo.io agentregistry Enterprise** (`arctl` + `ar.dev/v1alpha1` CRDs).

The workshop is built around two ideas:

1. **Three setup labs, soup to nuts.** [001](001-baseline-setup.md) → [002a](002a-setup-oidc-keycloak.md) **or** [002b](002b-setup-oidc-entra.md) → [003](003-install-components.md) takes you from a bare Kubernetes cluster to a working baseline (OIDC + agentregistry + Enterprise Agentgateway). You only run setup once. Some unit labs (020, 031, and the kagent portion of 060) additionally require **kagent Enterprise** - install it via the [kagent-enterprise workshop](https://github.com/solo-io/field-agentic-labs/tree/main/kagent-enterprise) before running those.

2. **Independent unit-of-value labs.** Every lab numbered 010 and up is standalone - it states what it needs from the baseline, walks through one capability, and has its own `## Cleanup` section that returns the cluster to the post-baseline state. Run them in any order, run cleanup, move on.

All manifests, agent source code, and Python MCP servers are in [`assets/`](assets/) in this repo. No external private references - everything pulls from `github.com/solo-io/field-agentic-labs`.

## Prerequisites

- A running Kubernetes cluster (≥ 1.29) with a default `StorageClass` and a working `LoadBalancer` Service controller
- `kubectl`, `helm` v3, `openssl`, `envsubst`, `jq`
- An OIDC provider - Keycloak (in-cluster, [002a](002a-setup-oidc-keycloak.md)) or Microsoft Entra ID ([002b](002b-setup-oidc-entra.md))
- A Solo Enterprise for agentgateway license key (used by [003](003-install-components.md))
- An LLM provider API key - Anthropic / OpenAI / Gemini (used by kagent)
- (Optional) AWS account for the AWS Bedrock AgentCore lab ([010](010-aws-bedrock-runtime.md))
- (Optional) Azure subscription + Azure AI Foundry project for the Foundry Runtime lab ([011](011-azure-ai-foundry-runtime.md))
- (Optional) `docker buildx` + a container registry for the BYO-agent lab ([020](020-kagent-runtime-and-agent.md))

# Table of Contents

- [Setup (mandatory)](#setup-mandatory)
- [Runtimes & Agents](#runtimes--agents)
- [MCP Servers](#mcp-servers)
- [Prompts](#prompts)
- [AccessPolicy & Approvals](#accesspolicy--approvals)
- [Observability](#observability)
- [Cleanup](#cleanup)

---

## Setup (mandatory)

> Three labs. Run them in order, once. After this, every lab from 010 onwards is independent.

- [001 - Baseline Setup](001-baseline-setup.md) - cluster prereqs check + `arctl` install + namespace
- **Pick one OIDC path:**
  - [002a - Setup OIDC: Keycloak (in-cluster)](002a-setup-oidc-keycloak.md) - recommended for a self-contained POC; no cloud account needed
  - [002b - Setup OIDC: Entra ID](002b-setup-oidc-entra.md) - if you already have a Microsoft Entra tenant
- [003 - Install Components](003-install-components.md) - agentregistry + kagent + Enterprise Agentgateway

---

## Runtimes & Agents

- [010 - AWS Bedrock AgentCore Runtime + demochatbot](010-aws-bedrock-runtime.md) - registers AWS as a Runtime + deploys the `demochatbot` agent on top
- [011 - Azure AI Foundry Runtime](011-azure-ai-foundry-runtime.md) - registers a Foundry Agent Service project as a discovery runtime
- [020 - kagent Runtime + k8shelper Agent](020-kagent-runtime-and-agent.md) - register kagent as a Runtime, then deploy the prebuilt `k8shelper` BYO image (run [031](031-mcp-remote-github-copilot.md) before the Agent apply if using the checked-in MCP-enabled manifest)

---

## MCP Servers

- [030 - Local stdio MCP Server (`demo-tools`)](030-mcp-local-stdio.md) - catalog-only, in-tree Python MCP
- [031 - Remote MCP via kagent (GitHub Copilot)](031-mcp-remote-github-copilot.md) - remote streamable-HTTP MCP deployed to the kagent runtime
- [032 - Remote MCP through Agentgateway (`Virtual` runtime)](032-mcp-through-agentgateway.md) - third topology, gateway-fronted

---

## Prompts

- [040 - Prompts (Catalog Asset Quickstart)](040-prompts.md) - `Prompt` CRUD via `arctl`

---

## AccessPolicy & Approvals

- [050 - AccessPolicy for Groups + MCP Tools + Chat](050-access-policies.md)
- [051 - Approval Workflows (admin gating of catalog submissions)](051-approval-workflows.md)

---

## Observability

- [060 - Tracing + kagent Fan-Out (kagent + AWS AgentCore runtimes)](060-observability-tracing.md)
- [062 - Audit Logging (local debug + Splunk HEC)](062-audit-logging.md)

---

## Cleanup

- [099 - Cleanup](099-cleanup.md) - tear down the baseline (each unit-of-value lab has its own cleanup too)

---

## Tracks

Curated paths through subsets of the labs. See [`tracks/`](tracks/):

- [`aws-track.md`](tracks/aws-track.md) - Baseline → AWS Bedrock AgentCore + demochatbot
- [`kagent-track.md`](tracks/kagent-track.md) - Baseline → kagent runtime → k8shelper + MCP
- [`audit-track.md`](tracks/audit-track.md) - Baseline → local audit validation → Splunk HEC

---

## Use Cases

- Install agentregistry Enterprise on Kubernetes with OIDC
- Federate AWS Bedrock AgentCore and kagent under a single agent catalog and RBAC model
- Federate Azure AI Foundry Agent Service projects as discovery runtimes
- Register agents either by repo source (cloned + built by agentregistry) or by pre-built container image (BYO image)
- Register MCP servers (`stdio` local, `streamable-http` remote, `Virtual` runtime via Agentgateway) and wire them into agents
- Enforce AccessPolicy-based RBAC against Entra group object IDs, Entra app roles, or Keycloak groups
- Gate every catalog submission behind admin approval (`requireCreateApproval`)
- Surface traces from all runtimes in the agentregistry dashboard via the bundled OTel Collector + ClickHouse
- Export structured lifecycle, approval, authorization, and applied-resource audit events locally or to Splunk

## Validated On

- Core workshop baseline: Agentregistry Enterprise chart and `arctl` `v2026.6.2`
- Audit Logging lab: Agentregistry Enterprise `v2026.7.0` and OTel Collector Contrib `0.148.0`
- Kagent OSS chart `0.9.7`
- Enterprise Agentgateway `v2026.6.1`
- Keycloak `quay.io/keycloak/keycloak:26.0`
- Kubernetes 1.29+
- AWS Bedrock AgentCore (us-east-1)
- Azure AI Foundry Agent Service

## Repo Layout

```
agentregistry-enterprise/
├── README.md                            # this file
├── 001-baseline-setup.md                # cluster prereqs + arctl + namespace
├── 002a-setup-oidc-keycloak.md          # OIDC path A: in-cluster Keycloak
├── 002b-setup-oidc-entra.md             # OIDC path B: Microsoft Entra ID
├── 003-install-components.md            # agentregistry + kagent + Enterprise Agentgateway
├── 010-aws-bedrock-runtime.md           # AWS Runtime + demochatbot
├── 011-azure-ai-foundry-runtime.md      # Azure AI Foundry Runtime
├── 020-kagent-runtime-and-agent.md      # kagent Runtime + k8shelper
├── 030-mcp-local-stdio.md               # in-tree stdio MCP
├── 031-mcp-remote-github-copilot.md     # GitHub Copilot MCP via kagent
├── 032-mcp-through-agentgateway.md      # MCP via Virtual runtime + Agentgateway
├── 040-prompts.md                       # Prompt CRUD
├── 050-access-policies.md               # AccessPolicy patterns
├── 051-approval-workflows.md            # requireCreateApproval feature
├── 060-observability-tracing.md         # tracing setup + kagent collector fan-out
├── 062-audit-logging.md                 # local debug + Splunk audit export
├── 099-cleanup.md                       # full teardown
├── tracks/
│   ├── audit-track.md
│   ├── aws-track.md
│   └── kagent-track.md
└── assets/                              # YAML, agent source, Terraform, ConfigMap patches
    ├── access-policies/
    ├── demochatbot-a2a/                 # ADK agent source for the AWS lab
    ├── k8shelper-anthropic/             # BYO image source for the kagent lab (Claude)
    ├── k8shelper-gemini/                # BYO image source for the kagent lab (Gemini)
    ├── mcp/
    │   ├── demo-mcp/                    # stdio MCP server source (Python)
    │   ├── github-copilot-mcpserver.yaml
    │   ├── github-copilot-mcp-deploy.yaml
    │   └── agentgateway/                # parent Gateway + Route + Virtual runtime + MCP for lab 032
    ├── observability/                   # tracing patches + audit collector manifests
    ├── private-eks/                     # Terraform reference (not used by the main flow)
    ├── prompts/
    └── providers/kagent/                # k8shelper Agent + Deployment manifests
```
