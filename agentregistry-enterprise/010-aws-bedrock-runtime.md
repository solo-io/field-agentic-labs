# AWS Bedrock AgentCore Runtime

Register AWS Bedrock AgentCore as an agentregistry **Runtime** and deploy the included `demochatbot` agent on top. Agentregistry will clone the agent source from this repo, build the AgentCore wrapper, hand it to AgentCore, and you'll see the deployment go from `deploying` → `deployed`.

## Lab Objectives

- Generate the IAM CloudFormation template with `arctl provider setup aws`
- Deploy the stack and capture `RoleArn` + `ExternalId`
- Register the AWS Runtime in agentregistry
- Register and deploy the `demochatbot` Agent
- Verify the deployment lands in AgentCore + locate the CloudWatch log group

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) → [002a](002a-setup-oidc-keycloak.md) **or** [002b](002b-setup-oidc-entra.md) → [003](003-install-components.md)
- `arctl` authenticated against the running agentregistry (verified in 003)
- An AWS account with permissions to create IAM roles + CloudFormation stacks
- `aws` CLI installed and authenticated (`aws sts get-caller-identity` succeeds)

```bash
export AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
export AWS_REGION=us-east-1   # adjust if you want a different region
```

## 1. Generate the IAM CloudFormation Template

```bash
arctl provider setup aws --aws-account-id "${AWS_ACCOUNT_ID}" > /tmp/agentregistry-cf.yaml
```

The template creates an IAM role with the permissions agentregistry needs to drive AgentCore: Bedrock AgentCore, IAM (to create per-agent execution roles), S3 (agent code artifacts), CloudWatch Logs, AppConfig, Cognito, EC2.

Note the **External ID** and **Role Name** printed at the bottom of the template output.

## 2. Deploy the CloudFormation Stack

```bash
aws cloudformation create-stack \
  --stack-name agentregistry-access-role \
  --template-body file:///tmp/agentregistry-cf.yaml \
  --capabilities CAPABILITY_NAMED_IAM \
  --region "${AWS_REGION}"

aws cloudformation wait stack-create-complete \
  --stack-name agentregistry-access-role \
  --region "${AWS_REGION}"
```

Capture the outputs:

```bash
export AWS_ROLE_ARN=$(aws cloudformation describe-stacks \
  --stack-name agentregistry-access-role --region "${AWS_REGION}" \
  --query "Stacks[0].Outputs[?OutputKey=='RoleArn'].OutputValue" --output text)

export AWS_EXTERNAL_ID=$(aws cloudformation describe-stacks \
  --stack-name agentregistry-access-role --region "${AWS_REGION}" \
  --query "Stacks[0].Outputs[?OutputKey=='ExternalId'].OutputValue" --output text)

echo "AWS_ROLE_ARN=${AWS_ROLE_ARN}"
echo "AWS_EXTERNAL_ID=${AWS_EXTERNAL_ID}"
```

## 3. Register the AWS Runtime

```bash
cat > /tmp/aws-runtime.yaml <<EOF
apiVersion: ar.dev/v1alpha1
kind: Runtime
metadata:
  name: AWS
spec:
  type: BedrockAgentCore
  config:
    roleArn: "${AWS_ROLE_ARN}"
    externalId: "${AWS_EXTERNAL_ID}"
    region: "${AWS_REGION}"
EOF

arctl apply -f /tmp/aws-runtime.yaml
arctl get runtimes
```

You should see `AWS` with `type: BedrockAgentCore`.

## 4. Register and Deploy `demochatbot`

The agent + deployment manifests are checked in:

- [`assets/demochatbot-a2a/agent.yaml`](assets/demochatbot-a2a/agent.yaml) - sourced from this repo at `agentregistry-enterprise/assets/demochatbot-a2a/`
- [`assets/demochatbot-a2a/deploy.yaml`](assets/demochatbot-a2a/deploy.yaml) - targets `Runtime: AWS`

```bash
arctl apply -f assets/demochatbot-a2a/agent.yaml
arctl apply -f assets/demochatbot-a2a/deploy.yaml
```

## 5. Watch the Deployment Reach `deployed`

```bash
arctl get deployments
arctl get deployment demochatbot -o yaml
```

The Deployment moves through `deploying` → `deployed`. If `status.conditions` shows a failure, common causes:

| Failure | Fix |
|---|---|
| `IAM role not assumable` | Re-check `External ID` matches what's in the role's trust policy (step 2) |
| `image build failed` | Check the agentregistry server logs: `kubectl logs -n agentregistry-system deploy/agentregistry-enterprise --tail=100` |

## 6. Locate the CloudWatch Log Group

AgentCore writes to a log group named `/aws/bedrock-agentcore/runtimes/<runtime-id>-DEFAULT`. The `<runtime-id>` is in `arctl get deployment demochatbot -o yaml` under `status.runtime`.

```bash
aws logs describe-log-groups \
  --region "${AWS_REGION}" \
  --log-group-name-prefix /aws/bedrock-agentcore/runtimes/

# Tail the active group:
aws logs tail "/aws/bedrock-agentcore/runtimes/<runtime-id>-DEFAULT" \
  --region "${AWS_REGION}" --follow
```

## Cleanup

Return the cluster + AWS account to the baseline:

```bash
# agentregistry side: delete the deployment + agent + runtime
arctl delete deployment demochatbot
arctl delete agent      demochatbot --version 1.0.4
arctl delete runtime    AWS

# AWS side: delete the CloudFormation stack (removes the IAM role)
aws cloudformation delete-stack \
  --stack-name agentregistry-access-role \
  --region "${AWS_REGION}"

aws cloudformation wait stack-delete-complete \
  --stack-name agentregistry-access-role \
  --region "${AWS_REGION}"

# Local temp files
rm -f /tmp/agentregistry-cf.yaml /tmp/aws-runtime.yaml

unset AWS_ACCOUNT_ID AWS_REGION AWS_ROLE_ARN AWS_EXTERNAL_ID
```

## Next

- [020 - kagent Runtime + Agent](020-kagent-runtime-and-agent.md) - the in-cluster runtime
- [030 - Local stdio MCP](030-mcp-local-stdio.md)
- [060 - Observability / Tracing](060-observability-tracing.md) - wire AgentCore traces into the AR dashboard
