# Agent A2A Skills Metadata

A `Declarative` Agent can publish **A2A skills** in its `spec.declarative.a2aConfig.skills` block. Skills are first-class metadata about what an agent can do: an `id`, a human-readable `name`, a `description`, `tags`, and concrete `examples` of prompts the agent handles. Other agents (and the kagent UI) read this metadata for discovery and routing.

## Lab Objectives

- Add an `a2aConfig.skills` block to a `Declarative` Agent (`kubernetes-mcp-agent`)
- Confirm the skills surface in the kagent UI / agent card
- Understand the relationship between skills (capabilities) and tools (concrete MCP calls)

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) → [002](002-licenses-and-secrets.md) → [003](003-install-kagent-enterprise.md)
## Apply the Agent with Skills

This version of the `kubernetes-mcp-agent` declares three skills (`cluster-diagnostics`, `resource-management`, `security-audit`) on top of the MCP tool list. Skills are a superset of tools - they describe *capabilities* an agent advertises, not the individual tool calls it actually makes.

```bash
kubectl apply -f - <<EOF
apiVersion: kagent.dev/v1alpha2
kind: Agent
metadata:
 name: kubernetes-mcp-agent
 namespace: kagent
spec:
 description: This agent can use a single tool to expand it's Kubernetes knowledge for troubleshooting and deployment
 type: Declarative
 declarative:
 modelConfig: default-model-config
 systemMessage: |-
 You're a friendly and helpful agent that uses the Kubernetes tool to help troubleshooting and deploy environments

 # Instructions

 - If user question is unclear, ask for clarification before running any tools
 - Always be helpful and friendly
 - If you don't know how to answer the question DO NOT make things up
 respond with "Sorry, I don't know how to answer that" and ask the user to further clarify the question

 # Response format
 - ALWAYS format your response as Markdown
 - Your response will include a summary of actions you took and an explanation of the result
 tools:
 - type: McpServer
 mcpServer:
 name: mcp-kubernetes-server
 kind: MCPServer
 toolNames:
 - events_list
 - namespaces_list
 - pods_list
 - pods_list_in_namespace
 - pods_get
 - pods_delete
 a2aConfig:
 skills:
 - id: cluster-diagnostics
 name: Cluster Diagnostics
 description: The ability to analyze and diagnose Kubernetes Cluster issues.
 tags:
 - cluster
 - diagnostics
 examples:
 - "What is the status of my cluster?"
 - "How can I troubleshoot a failing pod?"
 - "What are the resource limits for my nodes?"
 - id: resource-management
 name: Resource Management
 description: The ability to manage and optimize Kubernetes resources.
 tags:
 - resource
 - management
 examples:
 - "Scale my deployment X to 3 replicas."
 - "Optimize resource requests for my pods."
 - "Reserve more CPU for my nodes."
 - id: security-audit
 name: Security Audit
 description: The ability to audit and enhance Kubernetes security.
 tags:
 - security
 - audit
 examples:
 - "Check for RBAC misconfigurations."
 - "Audit my network policies."
 - "Identify potential security vulnerabilities in my cluster."
EOF
```

## Verify

```bash
kubectl get agent kubernetes-mcp-agent -n kagent -o yaml \
 | grep -A 100 a2aConfig
```

In the UI, the agent's card should now show the three skills with their descriptions, tags, and example prompts.

## Cleanup

If this lab was the only thing you applied (no leftover MCPServer / model-config from [010](010-mcp-connection-agent-config.md)), tear down with:

```bash
kubectl delete agent kubernetes-mcp-agent -n kagent --ignore-not-found
kubectl delete mcpserver mcp-kubernetes-server -n kagent --ignore-not-found
```

If you ran [010](010-mcp-connection-agent-config.md) and this lab just *re-applied* the same Agent with skills metadata added, the lab's only contribution to delete is the `a2aConfig` block - `kubectl edit agent kubernetes-mcp-agent -n kagent` and remove it, or re-apply 010's manifest (which has no skills) on top.

## Skills vs Tools

| Field | Layer | Audience |
|---|---|---|
| `spec.declarative.tools[*]` | Wiring | The agent's runtime - these are the actual MCP calls it can make |
| `spec.declarative.a2aConfig.skills[*]` | Catalog metadata | Other agents, the UI, A2A clients - used for discovery and routing |

A skill is an advertised capability. The tools are how the agent fulfills it. The two don't have to be 1:1 - a single skill (e.g., `cluster-diagnostics`) typically uses several tools (`pods_list`, `pods_log`, `events_list`).

## Next

- [042 - Build a Custom MCP Server (Pharmaceutical Example)](012-build-custom-mcp-server.md) - go from "use someone else's MCP server" to "ship your own"
- [060 - `AccessPolicy`: Agent → MCP](030-accesspolicy-agent-to-mcp.md)
