# AWS Bedrock AgentCore Runtime

Register AWS Bedrock AgentCore as an agentregistry **Runtime** and deploy the included `demochatbot` agent on top. Agentregistry will clone the agent source from this repo, build the AgentCore wrapper, hand it to AgentCore, and you'll see the deployment go from `deploying` → `deployed`.

## Lab Objectives

- Generate the IAM CloudFormation template with `arctl runtime setup bedrock-agent-core`
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
: "${ARCTL_API_BASE_URL:?ARCTL_API_BASE_URL is not set; repeat the API setup from lab 003}"

# Make the stored login token explicit for enterprise-only helper commands.
export ARCTL_API_TOKEN="$(arctl user info --show-tokens | jq -er '.access_token')"

# Login success only confirms the IdP flow. This call confirms that
# AgentRegistry accepts the token. Do not continue if it returns 401/403.
arctl user whoami

export AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
export AWS_REGION=us-east-1   # adjust if you want a different region
```

## IAM permissions

AgentRegistry never calls Bedrock with the long-lived IAM user keys you put in the Helm values. It calls `sts:AssumeRole` into the access role from the CloudFormation stack, then uses those temporary credentials. Put Bedrock, S3, and IAM permissions on **that role**. The user only needs permission to assume it.

Use your own account, role name, and External ID. Do not copy values from another install. The Runtime `spec.config.externalId` and the role trust policy must be the same string. A mismatch returns `AccessDenied` on `sts:AssumeRole` even when the user is an account admin.

### Caller (the IAM user or role whose keys AgentRegistry stores)

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AssumeAgentRegistryAccessRole",
      "Effect": "Allow",
      "Action": [
        "sts:AssumeRole",
        "sts:TagSession"
      ],
      "Resource": "arn:aws:iam::<account-id>:role/AgentRegistryAccessRole-<suffix>"
    }
  ]
}
```

`sts:TagSession` is required because AgentRegistry names the STS session and the role trust allows `TagSession` as its own statement.

### Role trust policy

The stack writes this. If you edit the role by hand, keep both statements. Only `AssumeRole` is conditioned on the External ID.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": { "AWS": "arn:aws:iam::<account-id>:root" },
      "Action": "sts:AssumeRole",
      "Condition": {
        "StringEquals": { "sts:ExternalId": "<runtime spec.config.externalId>" }
      }
    },
    {
      "Effect": "Allow",
      "Principal": { "AWS": "arn:aws:iam::<account-id>:root" },
      "Action": "sts:TagSession"
    }
  ]
}
```

### Access role — sync

Attach the AWS managed policy:

`arn:aws:iam::aws:policy/BedrockAgentCoreFullAccess`

That policy grants `bedrock-agentcore:*` on `arn:aws:bedrock-agentcore:*:*:*`, which covers discovery (`ListAgentRuntimes`, `GetAgentRuntime`) and invoke. It also includes the IAM pass-role, Secrets Manager, KMS, and log-read actions AWS bundles with it. A Runtime can show **Synced** with only this managed policy, plus a matching External ID.

### Access role — deploy

Sync does not upload agent artifacts. Catalog deploys also need the inline policy `BedrockAgentCoreSupplementalAccess` from the same template. An older stack that predates the `are-*` S3 statement will sync and then fail `s3:CreateBucket` or `s3:PutBucketTagging` / `s3:GetBucketTagging`. Update the stack or add the missing statement. The template does not grant `s3:DeleteBucket`.

| Sid | What it is for | Actions | Resource |
|---|---|---|---|
| `IAMCreateAndManageExecutionRoles` | Per-agent AgentCore execution roles | `iam:CreateRole`, `DeleteRole`, `GetRole`, `PutRolePolicy`, `DeleteRolePolicy`, `AttachRolePolicy`, `DetachRolePolicy`, `TagRole`, `ListRolePolicies`, `ListAttachedRolePolicies`, `GetRolePolicy`, `UpdateRole`, `UpdateAssumeRolePolicy` | `arn:aws:iam::*:role/*BedrockAgentCore*`, `arn:aws:iam::*:role/service-role/*BedrockAgentCore*`, `arn:aws:iam::*:role/AmazonBedrockAgentCoreSDKRuntime-*`, `arn:aws:iam::*:role/aws-service-role/bedrock-agentcore.amazonaws.com/*` |
| `IAMCreatePolicy` | Execution-role customer managed policies | `iam:CreatePolicy`, `GetPolicy`, `GetPolicyVersion`, `ListPolicyVersions`, `DeletePolicy`, `DeletePolicyVersion`, `CreatePolicyVersion` | `arn:aws:iam::*:policy/service-role/AmazonBedrockAgentCoreRuntimeExecutionPolicy_*` |
| `IAMServiceLinkedRole` | AgentCore service-linked role | `iam:CreateServiceLinkedRole`, `GetServiceLinkedRoleDeletionStatus`, `DeleteServiceLinkedRole` | `arn:aws:iam::*:role/aws-service-role/bedrock-agentcore.amazonaws.com/*`, only when `iam:AWSServiceName` is `bedrock-agentcore.amazonaws.com` |
| `CloudWatchLogsFullAccess` | AgentCore runtime logs | `logs:CreateLogGroup`, `CreateLogStream`, `PutLogEvents`, `DescribeLogGroups`, `DescribeLogStreams`, `DeleteLogGroup`, `PutDeliverySource`, `PutResourcePolicy`, `DeleteResourcePolicy` | `arn:aws:logs:*:*:log-group:/aws/bedrock-agentcore/*`, `arn:aws:logs:*:*:log-group:/aws/vendedlogs/bedrock-agentcore/*`, `arn:aws:logs:*:*:delivery-source:*`, `arn:aws:logs:*:*:delivery-destination:*` |
| `CloudWatchLogsResourcePolicy` | Account log-resource policies | `logs:PutResourcePolicy`, `DeleteResourcePolicy`, `DescribeResourcePolicies` | `*` |
| `S3CodeBuildArtifacts` | Source and install buckets (`are-*`, `agentcore-*`, `bedrock-agentcore-codebuild-sources-*`) | `s3:CreateBucket`, `PutObject`, `GetObject`, `ListBucket`, `ListBucketVersions`, `GetBucketLocation`, `PutBucketPublicAccessBlock`, `PutBucketVersioning`, `PutBucketPolicy`, `GetBucketPolicy`, `DeleteObject`, `DeleteObjectVersion`, `PutLifecycleConfiguration`, `PutObjectTagging`, `GetObjectTagging`, `PutBucketTagging`, `GetBucketTagging` | `arn:aws:s3:::are-*`, `arn:aws:s3:::are-*/*`, and the same pair for `agentcore-*` and `bedrock-agentcore-codebuild-sources-*` |
| `CognitoUserPoolManagement` | Optional AgentCore auth pools | `cognito-idp:CreateUserPool`, `DeleteUserPool`, `DescribeUserPool`, `CreateUserPoolClient`, `DeleteUserPoolClient`, `DescribeUserPoolClient`, `AdminCreateUser`, `AdminDeleteUser`, `AdminSetUserPassword`, `AdminGetUser`, `InitiateAuth` | `*` |
| `OutboundIdentityFederationDescribe` | Workload identity token setup | `iam:GetOutboundWebIdentityFederationInfo` | `*` |

Source of truth: `internal/runtime/agentcore/setup.go` in the AgentRegistry Enterprise repo (`BedrockAgentCoreSupplementalAccess`). Re-apply that template when upgrading AgentRegistry rather than hand-editing a subset of the S3 actions.

## 1. Generate the IAM CloudFormation Template

```bash
arctl runtime setup bedrock-agent-core \
  --aws-account-id "${AWS_ACCOUNT_ID}" \
  --registry-token "${ARCTL_API_TOKEN}" > /tmp/agentregistry-cf.yaml
```

The template creates one IAM role. That role — not the IAM user whose keys AgentRegistry stores — is what lists, deploys, and invokes AgentCore. The full permission split is in [IAM permissions](#iam-permissions) above.

Note the **External ID** and **Role Name** printed to the terminal. They are written
to stderr so the redirected file contains only valid CloudFormation YAML. The
External ID is also included in the template's stack outputs. The role name is
`AgentRegistryAccessRole-` plus the first 8 characters of that External ID.

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
| `API returned status 401: Unauthorized` | The IdP issued a token, but AgentRegistry did not accept it. Re-export `ARCTL_API_BASE_URL` from lab 003, export `ARCTL_API_TOKEN` with `arctl user info --show-tokens`, and require `arctl user whoami` to succeed before retrying. |
| `forbidden: unauthenticated` or HTTP `403` | AgentRegistry accepted the bearer token but did not map it to a role allowed to publish a Runtime. Confirm `arctl user whoami` shows the configured superuser role. For Keycloak, re-run 002a's setup script so the `are-cli` token includes the `groups` claim, then log in again. |
| `IAM role not assumable` | Re-check `External ID` matches what's in the role's trust policy (step 2) |
| `image build failed` | Check the agentregistry server logs: `kubectl logs -n agentregistry-system deploy/agentregistry-enterprise-server --tail=100` |

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
arctl delete agent      demochatbot --tag 1.0.4
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
