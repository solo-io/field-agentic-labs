# Register Agents and MCP Servers from a GitLab Pipeline

This lab walks through a GitLab CI/CD pipeline that registers an MCP server (GitHub Copilot, exposed through an in-cluster Agentgateway), an agent (the `demochatbot` from [010 - AWS Bedrock Runtime](010-aws-bedrock-runtime.md)), and an agentregistry `Deployment` of the agent to AWS Bedrock AgentCore - all driven by `arctl apply`.

It is the GitOps version of [010 - AWS Bedrock Runtime](010-aws-bedrock-runtime.md) + [031 - Remote MCP via kagent](031-mcp-remote-github-copilot.md). Run those labs interactively first to understand what the pipeline is doing - then come back here to put it on rails.

## Lab Objectives

- Define CI/CD variables (`ARCTL_API_TOKEN`, `ARCTL_API_BASE_URL`, AWS creds, EKS cluster name)
- Use `arctl` from an `alpine:3.21` image to install the CLI in the pipeline
- Deploy an in-cluster Agentgateway + `AgentgatewayBackend` + `HTTPRoute` for the remote MCP
- Register the MCP and agent in agentregistry and deploy to AWS

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) → [002a](002a-setup-oidc-keycloak.md) **or** [002b](002b-setup-oidc-entra.md) → [003](003-install-components.md)
- An AWS account + IAM role registered as a Runtime ([010](010-aws-bedrock-runtime.md) steps 1-3)
- A GitLab project with a runner that can reach (a) the Kubernetes API server, (b) `ARCTL_API_BASE_URL` from outside the cluster, and (c) any container registries the pipeline pulls from
- A long-lived `ARCTL_API_TOKEN` (the same bearer token `arctl` uses). See [003 step 4](003-install-components.md#4-authenticate-arctl).

## Pipeline Variables

Set these in **GitLab > Settings > CI/CD > Variables** (masked + protected):

| Variable | Description |
|----------|-------------|
| `ARCTL_API_BASE_URL` | Agentregistry endpoint (e.g., `http://agentregistry.internal.example.com`) |
| `ARCTL_API_TOKEN` | Bearer token for `arctl` (see [003 step 4](003-install-components.md#4-authenticate-arctl)) |
| `AWS_ACCESS_KEY_ID` | AWS credentials for `kubectl` EKS access |
| `AWS_SECRET_ACCESS_KEY` | AWS secret key |
| `AWS_DEFAULT_REGION` | AWS region (e.g., `us-east-1`) |
| `EKS_CLUSTER_NAME` | EKS cluster name |
| `GITHUB_PAT` | GitHub PAT for the GitHub MCP backend (used by the in-cluster Agentgateway) |
| `KUBECONFIG_B64` | (Alternative) Base64-encoded kubeconfig if you can't use `aws eks update-kubeconfig` |

## `.gitlab-ci.yml`

```yaml
stages:
  - setup
  - deploy-gateway
  - register

variables:
  ARCTL_VERSION: "v2026.05.0"

# ---------------------------------------------------------------------
# 1. Install tools (arctl, kubectl, awscli)
# ---------------------------------------------------------------------
install-tools:
  stage: setup
  image: alpine:3.21
  script:
    - apk add --no-cache curl bash python3
    - curl -sSL https://storage.googleapis.com/agentregistry-enterprise/install.sh | ARCTL_VERSION=$ARCTL_VERSION sh
    - export PATH=$HOME/.arctl/bin:$PATH
    - arctl version --json
    - curl -LO "https://dl.k8s.io/release/$(curl -Ls https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
    - chmod +x kubectl && mv kubectl /usr/local/bin/
    - kubectl version --client
  artifacts:
    paths:
      - $HOME/.arctl/bin/arctl

# ---------------------------------------------------------------------
# 2. Deploy MCP Gateway + Backend + Route on the cluster
# ---------------------------------------------------------------------
deploy-mcp-gateway:
  stage: deploy-gateway
  image: alpine:3.21
  before_script:
    - apk add --no-cache curl bash python3
    - curl -LO "https://dl.k8s.io/release/$(curl -Ls https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
    - chmod +x kubectl && mv kubectl /usr/local/bin/
    - curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o awscli.zip
    - python3 -m zipfile -e awscli.zip /tmp/awscli && /tmp/awscli/aws/install
    - aws eks update-kubeconfig --name $EKS_CLUSTER_NAME --region $AWS_DEFAULT_REGION
  script:
    - |
      kubectl apply -f - <<EOF
      apiVersion: gateway.networking.k8s.io/v1
      kind: Gateway
      metadata:
        name: mcp-gateway
        namespace: agentgateway-system
        labels: { app: github-mcp-server }
      spec:
        gatewayClassName: istio
        listeners:
          - name: mcp
            port: 3000
            protocol: HTTP
            allowedRoutes: { namespaces: { from: Same } }
      ---
      apiVersion: v1
      kind: Secret
      metadata:
        name: github-pat
        namespace: agentgateway-system
      type: Opaque
      stringData:
        Authorization: "Bearer ${GITHUB_PAT}"
      ---
      apiVersion: agentgateway.dev/v1alpha1
      kind: AgentgatewayBackend
      metadata:
        name: github-mcp-server
        namespace: agentgateway-system
      spec:
        mcp:
          targets:
            - name: github-copilot
              static:
                host: api.githubcopilot.com
                port: 443
                path: /mcp/
                protocol: StreamableHTTP
                policies:
                  tls: {}
                  auth:
                    secretRef: { name: github-pat }
      ---
      apiVersion: gateway.networking.k8s.io/v1
      kind: HTTPRoute
      metadata:
        name: mcp-route
        namespace: agentgateway-system
        labels: { app: github-mcp-server }
      spec:
        parentRefs: [{ name: mcp-gateway }]
        rules:
          - matches: [{ path: { type: PathPrefix, value: /mcp } }]
            backendRefs:
              - { name: github-mcp-server, namespace: agentgateway-system, group: agentgateway.dev, kind: AgentgatewayBackend }
      EOF
    - sleep 10
    - export GATEWAY_IP=$(kubectl get svc mcp-gateway -n agentgateway-system -o jsonpath='{.status.loadBalancer.ingress[0].hostname}')
    - echo "GATEWAY_IP=$GATEWAY_IP" >> deploy.env
    - echo "MCP Gateway endpoint:$GATEWAY_IP"
  artifacts:
    reports:
      dotenv: deploy.env

# ---------------------------------------------------------------------
# 3. Register the MCP Server and Agent in agentregistry
# ---------------------------------------------------------------------
register-agent:
  stage: register
  image: alpine:3.21
  dependencies: [deploy-mcp-gateway]
  before_script:
    - apk add --no-cache curl bash
    - curl -sSL https://storage.googleapis.com/agentregistry-enterprise/install.sh | ARCTL_VERSION=$ARCTL_VERSION sh
    - export PATH=$HOME/.arctl/bin:$PATH
  script:
    - |
      arctl apply -f - <<EOF
      apiVersion: ar.dev/v1alpha1
      kind: RemoteMCPServer
      metadata:
        name: github/copilot-mcp-server
        version: "0.1.0"
      spec:
        title: github-copilot
        description: Remote MCP server exposing GitHub tools via agentgateway.
        remote:
          type: streamable-http
          url: http://$GATEWAY_IP:3000/mcp
      EOF
    - |
      arctl apply -f - <<EOF
      apiVersion: ar.dev/v1alpha1
      kind: Agent
      metadata:
        name: demochatbot
        version: "1.0.4"
      spec:
        description: "A deterministic A2A/ADK-compatible chatbot for AWS Bedrock AgentCore"
        source:
          repository:
            url: "https://github.com/solo-io/field-agentic-labs"
            subfolder: "agentregistry-enterprise/assets/demochatbot-a2a"
        mcpServers:
          - { kind: RemoteMCPServer, name: github/copilot-mcp-server, version: "0.1.0" }
      EOF
    - |
      arctl apply -f - <<EOF
      apiVersion: ar.dev/v1alpha1
      kind: Deployment
      metadata:
        name: demochatbot
      spec:
        providerRef: { kind: Provider, name: AWS }
        targetRef:   { kind: Agent,    name: demochatbot, version: "1.0.4" }
      EOF
    - arctl get agents
    - arctl get mcps
    - arctl get deployments
```

## GitHub Actions vs GitLab CI/CD

The `arctl` commands are identical between CI systems - only the surrounding syntax differs:

| GitHub Actions | GitLab CI/CD |
|----------------|--------------|
| `${{ secrets.ARCTL_API_TOKEN }}` | `$ARCTL_API_TOKEN` (CI/CD variable) |
| `${{ secrets.GITHUB_PAT }}` | `$GITHUB_PAT` (CI/CD variable) |
| `runs-on: ubuntu-latest` | `image: alpine:3.21` |
| `uses: actions/checkout@v3` | Built-in checkout |
| Separate `run:` steps | `script:` block |
| `env:` at job level | `variables:` at job or pipeline level |

## GitLab Runner Networking on Private EKS

The runner must reach:

1. **The EKS API server** - for `kubectl`. Options:
   - Deploy the runner inside the VPC (recommended)
   - SSM or VPN from an external runner
   - `KUBECONFIG_B64` with a kubeconfig that goes through a bastion proxy
2. **The agentregistry endpoint** - for `arctl`. The internal NLB is only reachable from inside the VPC, so the runner must be in the same VPC or a peered one.
3. **External registries** - `us-docker.pkg.dev` (Helm chart OCI), `storage.googleapis.com` (arctl CLI download). NAT gateway or VPC endpoints.

A common setup is to deploy the GitLab runner as a pod in the same EKS cluster, or as an EC2 instance in the same VPC:

```bash
helm repo add gitlab https://charts.gitlab.io
helm install gitlab-runner gitlab/gitlab-runner \
  --namespace gitlab-runner --create-namespace \
  --set gitlabUrl="https://gitlab.example.com" \
  --set runnerToken="<RUNNER_TOKEN>" \
  --set runners.executor="kubernetes" \
  --set runners.kubernetes.namespace="gitlab-runner"
```

## Cleanup

Tear down everything the pipeline created:

```bash
# agentregistry side
arctl delete deployment demochatbot                          2>/dev/null || true
arctl delete agent      demochatbot --version 1.0.4          2>/dev/null || true
arctl delete remotemcpserver github/copilot-mcp-server --version 0.1.0 2>/dev/null || true

# Kubernetes side: the gateway + backend + route + secret the pipeline applied
kubectl delete httproute           mcp-route          -n agentgateway-system --ignore-not-found
kubectl delete agentgatewaybackend github-mcp-server  -n agentgateway-system --ignore-not-found
kubectl delete gateway             mcp-gateway        -n agentgateway-system --ignore-not-found
kubectl delete secret              github-pat         -n agentgateway-system --ignore-not-found
```

If you also want to drop the AWS Runtime + CloudFormation stack the pipeline depends on, see the Cleanup section of [010](010-aws-bedrock-runtime.md#cleanup).

If you want to remove the GitLab runner, follow your GitLab runner uninstall path (it's outside the scope of this lab).

## Next

- [099 - Cleanup](099-cleanup.md) - full workshop teardown
