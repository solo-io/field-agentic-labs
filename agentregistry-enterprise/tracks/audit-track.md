# Track - Audit Logging

A focused path for validating Agentregistry control-plane audit events locally, then forwarding them to Splunk through a persistent OpenTelemetry Collector bridge.

## Estimated Time

- ~30 minutes for the no-Splunk path
- ~50 minutes for both paths when a Splunk HEC endpoint already exists

## Prerequisites

- A Kubernetes cluster with a default `StorageClass`
- An OIDC provider: Keycloak or Microsoft Entra ID
- Agentregistry Enterprise `2026.7.0` or newer
- For the Splunk portion: an enabled HEC endpoint, token, and writable index

## Order

1. [001 - Baseline Setup](../001-baseline-setup.md)
2. **Pick one OIDC path:** [002a - Keycloak](../002a-setup-oidc-keycloak.md) or [002b - Entra ID](../002b-setup-oidc-entra.md)
3. [003 - Install Components](../003-install-components.md)
4. [050 - AccessPolicy](../050-access-policies.md) - understand the permissions represented in authorization events
5. [051 - Approval Workflows](../051-approval-workflows.md) *(optional, for approval audit events)*
6. [062 - Audit Logging](../062-audit-logging.md) - run the no-Splunk path, then the Splunk HEC path
7. [099 - Cleanup](../099-cleanup.md)

## What You Will Have at the End

- Structured lifecycle and authorization records from Agentregistry
- A local workflow for inspecting events without a SIEM
- A chart-managed audit relay with a persistent WAL
- A persistent OTLP-to-Splunk HEC bridge
- Splunk searches for lifecycle, authorization failures, and approvals

## Next

- [060 - Tracing](../060-observability-tracing.md) - add runtime execution traces alongside control-plane audit logs
- [Kagent track](kagent-track.md) - deploy an in-cluster agent runtime
- [AWS track](aws-track.md) - deploy an agent to Bedrock AgentCore
