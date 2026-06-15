# Track — Install + First MCP-Backed Agent

A focused path for someone who wants the canonical Solo Enterprise for kagent install (Gloo Operator) and a working MCP-backed Agent — no auth deep-dives, no OBO.

## Estimated Time

- ~90 minutes end-to-end on a fresh cluster (add ~10 minutes if you provision GKE)

## Prerequisites

- A Kubernetes cluster (GKE recommended — see [001](../001-provision-gke.md))
- Solo trial keys for Istio, Gloo Gateway, and Agentgateway
- An LLM provider API key (OpenAI for the default; swap to Anthropic if you prefer)

## Order

1. [001 — Provision a GKE Cluster](../001-provision-gke.md) *(skip if you have a cluster)*
2. [010 — Licenses, Namespace, and Secrets](../010-licenses-and-secrets.md)
3. [020 — Install Kagent Enterprise (Gloo Operator)](../020-install-kagent-enterprise.md)
4. [030 — Gateway Access Logs](../030-access-logs.md) *(optional but recommended)*
5. [040 — Declarative MCP Server + Agent](../040-mcp-connection-agent-config.md)
6. [041 — Agent A2A Skills Metadata](../041-agent-skills.md)
7. [050 — Validate Your Install (fix a broken Pod)](../050-troubleshooting-pod.md)
8. [099 — Cleanup](../099-cleanup.md)

## What You Will Have at the End

- Solo Enterprise for kagent installed via the Gloo Operator (kagent `0.1.5`, Solo Istio Ambient `1.27.1`, Gloo Gateway `2.0.0`)
- Structured JSON access logs flowing out of the `kagent-gateway`
- `mcp-kubernetes-server` MCP server running, wired into `kubernetes-mcp-agent`
- A2A skills surfacing on the agent card
- The pre-built `k8s-agent` successfully diagnosing a broken Pod end-to-end

## Next

- [policy-track](policy-track.md) — Layer `AccessPolicy`, prompt guards, and platform RBAC on top
- [obo-track](obo-track.md) — Replace this install with the OBO end-to-end (different install model)
