# Deploy the Demo Chatbot on AWS Bedrock AgentCore

This lab deploys the `demochatbot` agent to AWS Bedrock AgentCore through the AgentRegistry **AWS Runtime** you registered in [050](050-aws-provider.md). The agent is built from source — AgentRegistry clones the repo, builds the A2A/kagent-adk AgentCore wrapper, and hands it to AgentCore for deployment.

## Lab Objectives

- Register the `demochatbot` Agent (source = Git repository subfolder)
- Deploy it to the `AWS` runtime
- Watch the deployment go from `deploying` → `deployed`
- Locate CloudWatch logs for the running AgentCore runtime

## Prerequisites

- [040 — `arctl` authenticated](040-arctl-auth.md)
- [050 — AWS Runtime registered](050-aws-provider.md)

## 1. Inspect the Manifests

The agent lives at [`assets/demochatbot-a2a/`](assets/demochatbot-a2a/):

```
assets/demochatbot-a2a/
├── agent.yaml                       # ar.dev Agent — sourced from this repo
├── deploy.yaml                      # ar.dev Deployment targeting AWS
└── demochatbot/
    ├── __init__.py
    ├── agent.py                     # ADK BaseAgent implementation
    └── agent-card.json              # A2A agent card
```

[`assets/demochatbot-a2a/agent.yaml`](assets/demochatbot-a2a/agent.yaml):

```yaml
apiVersion: ar.dev/v1alpha1
kind: Agent
metadata:
  name: demochatbot
  version: "1.0.4"
spec:
  description: "A deterministic A2A/ADK-compatible chatbot for AWS Bedrock AgentCore"
  source:
    repository:
      url: "https://github.com/AdminTurnedDevOps/agentic-demo-repo"
      subfolder: "agentregistry-enterprise/demochatbot-a2a"
```

[`assets/demochatbot-a2a/deploy.yaml`](assets/demochatbot-a2a/deploy.yaml):

```yaml
apiVersion: ar.dev/v1alpha1
kind: Deployment
metadata:
  name: demochatbot
spec:
  providerRef:
    kind: Provider
    name: AWS
  targetRef:
    kind: Agent
    name: demochatbot
    version: "1.0.4"
```

> `providerRef` and `runtimeRef` are interchangeable in the API — older docs use `Provider`, newer ones use `Runtime`. The chart accepts both.

## 2. Register and Deploy

```bash
# Register the agent in the catalog
arctl apply -f assets/demochatbot-a2a/agent.yaml

# Deploy it to AWS Bedrock AgentCore
arctl apply -f assets/demochatbot-a2a/deploy.yaml
```

## 3. Watch the Deployment

```bash
arctl get deployments
```

The Deployment moves through `deploying` → `deployed`. Inspect the full record (status conditions, runtime metadata) with:

```bash
arctl get deployment demochatbot -o yaml
```

A failed deployment surfaces the reason in `status.conditions`. Common ones:

- `IAM role not assumable` — the External ID on the Runtime doesn't match what was registered in CloudFormation; re-check the values from [050 step 2](050-aws-provider.md#2-deploy-the-stack).
- `image build failed` — clone or build of the repo subfolder failed; check the server logs.

## 4. Locate the CloudWatch Logs

AgentCore writes to a log group named `/aws/bedrock-agentcore/runtimes/<runtime-id>-DEFAULT`. The `<runtime-id>` is visible in the `arctl get deployment demochatbot -o yaml` output under `status.runtime`.

```bash
aws logs describe-log-groups \
  --region us-east-1 \
  --log-group-name-prefix /aws/bedrock-agentcore/runtimes/

aws logs tail /aws/bedrock-agentcore/runtimes/<runtime-id>-DEFAULT \
  --region us-east-1 \
  --follow
```

## 5. Add Telemetry (Optional)

To get the demochatbot's traces into the AgentRegistry dashboard, the `AWS` runtime needs `spec.telemetryEndpoint` set to the **external** address of the OTel Collector and the deployment needs to be re-applied so the AgentCore workload picks up `OTEL_EXPORTER_OTLP_ENDPOINT`. See [090 step 2: AWS Bedrock AgentCore Runtime](090-observability-tracing.md#aws-bedrock-agentcore-runtime).

## Next

- [061 — Deploy `k8shelper` on kagent](061-deploy-k8shelper-on-kagent.md)
- [080 — AccessPolicy / RBAC](080-access-policies.md)
- [090 — Tracing setup](090-observability-tracing.md)
