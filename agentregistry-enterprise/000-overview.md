# Overview & Architecture

AgentRegistry Enterprise is a control plane for the agent ecosystem: a single catalog of `Agent`, `MCPServer`, and `Runtime` (provider) resources, with OIDC-backed RBAC enforced by `AccessPolicy`. Workloads are deployed into one of several **Runtimes** — AWS Bedrock AgentCore, kagent, others — and observability (logs, metrics, traces) flows back into a bundled OTel Collector + ClickHouse so the AgentRegistry UI can render runs, token usage, and traces across runtimes.

## High-Level Picture

```
                ┌────────────────────────┐
                │   OIDC IdP             │
                │   (Entra ID / Keycloak)│
                └──────────┬─────────────┘
                           │ groups / roles claim
                           ▼
┌──────────┐   arctl   ┌──────────────────────────────┐
│   You    │──────────▶│  AgentRegistry Enterprise    │
└──────────┘           │  (Helm release in            │
                       │   agentregistry-system)      │
                       │                              │
                       │  ┌────────────┐ ┌─────────┐  │
                       │  │ HTTP :8080 │ │ MCP     │  │
                       │  │ UI + API   │ │ :31313  │  │
                       │  └────────────┘ └─────────┘  │
                       │  ┌────────────┐ ┌─────────┐  │
                       │  │ PostgreSQL │ │ClickHouse│ │
                       │  └────────────┘ └─────────┘  │
                       │  ┌──────────────────────────┐│
                       │  │ OTel Collector :4317/4318││
                       │  └──────────────────────────┘│
                       └──────────┬──────────┬────────┘
                                  │          │
                          deploy  │          │  deploy
                                  ▼          ▼
                         ┌────────────┐  ┌───────────────────────┐
                         │  kagent    │  │  AWS Bedrock          │
                         │  Runtime   │  │  AgentCore Runtime    │
                         │  (in-cluster)│ │  (external, IAM role) │
                         └────────────┘  └───────────────────────┘
```

## Key CRDs (group `ar.dev/v1alpha1`)

| Kind | Purpose |
|---|---|
| `Runtime` (a.k.a. Provider) | A deployment target — `Kagent`, `BedrockAgentCore`, etc. |
| `Agent` | An agent catalog entry. Sourced from a Git `repository` (built by the registry) or from a pre-built container `image`. |
| `MCPServer` | An MCP server catalog entry — `stdio` command, or `remote` `streamable-http` URL. |
| `Deployment` | Binds an `Agent` or `MCPServer` to a `Runtime`. |
| `AccessPolicy` | RBAC: maps OIDC principals (group object IDs, app role values, or `Deployment` identities) to actions (`registry:read`, `registry:publish`, `runtime:invoke`, …) on catalog resources. |

## Recommended Lab Flow

1. **[001](001-install-arctl.md)** Install `arctl`.
2. **[010](010-cluster-prereqs.md)** Bring up a cluster (or use an existing one). If targeting private EKS, install the EBS CSI driver and a `gp3` default StorageClass.
3. **[020](020-setup-entra.md)** *or* **[021](021-setup-keycloak.md)** Configure your OIDC provider — Entra app registrations + group claims, or Keycloak realm + clients.
4. **[030](030-install-agentregistry-helm.md)** Install the AgentRegistry Enterprise Helm chart with your OIDC and AWS values. Verify pods and Services.
5. **[035](035-private-cluster-istio-routing.md)** *(private clusters only)* Front the `ClusterIP` Service with an Istio Gateway and HTTPRoute.
6. **[040](040-arctl-auth.md)** Point `arctl` at your install and authenticate (device-code or manual token).
7. **[050](050-aws-provider.md)** *and / or* **[051](051-kagent-provider.md)** Register a Runtime.
8. **[060](060-deploy-demochatbot-on-aws.md)** *and / or* **[061](061-deploy-k8shelper-on-kagent.md)** Deploy your first agent.
9. **[070](070-register-local-mcp.md)** → **[071](071-register-github-copilot-mcp.md)** → **[072](072-wire-mcp-to-agent.md)** Register MCP servers and wire them into an agent.
10. **[080](080-access-policies.md)** Lock down catalog + runtime access with AccessPolicy.
11. **[090](090-observability-tracing.md)** → **[091](091-trace-fanout-workaround.md)** Get traces flowing into the dashboard.
12. **[095](095-gitops-gitlab-ci.md)** *(optional)* Wire registration into CI/CD.
13. **[099](099-cleanup.md)** Cleanup.

## Asset Conventions

All YAML, agent source, Dockerfiles, Terraform, and ConfigMap patches referenced by the labs are checked in under [`assets/`](assets/) and referenced by relative path, for example:

```bash
arctl apply -f assets/demochatbot-a2a/agent.yaml
arctl apply -f assets/demochatbot-a2a/deploy.yaml
```

Where a manifest contains an environment variable like `${K8SHELPER_IMAGE}` or `${GITHUB_COPILOT_MCP_TOKEN}`, the lab uses `envsubst` to render it before `arctl apply`:

```bash
envsubst < assets/mcp/github-copilot-mcpserver.yaml | arctl apply -f -
```

## Service Ports

| Port | Purpose |
|------|---------|
| 8080 | HTTP — UI + API |
| 21212 | Agent Gateway gRPC |
| 31313 | MCP server (HTTP) |
| 4317 / 4318 | OTel Collector — gRPC / HTTP |
