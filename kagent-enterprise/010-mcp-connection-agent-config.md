# Declarative MCP Server + Agent

This lab introduces kagent's **declarative MCP** pattern. You apply an `MCPServer` resource that tells kagent which MCP server you want; kagent acts like a package manager — it deploys a Pod that runs the server (here, `kubernetes-mcp-server` via `npx`) and exposes it over `stdio`. You then create a `Declarative` Agent that references the MCP server in its `tools` block with the specific tool names the agent is allowed to call.

## Lab Objectives

- Apply an `MCPServer` (`mcp-kubernetes-server`) that runs `npx kubernetes-mcp-server@latest` with stdio transport
- Apply a `Declarative` Agent (`kubernetes-mcp-agent`) that references the MCP server and a long allow-list of `kubernetes-mcp-server` tools
- Confirm the Agent reaches `READY=True` / `ACCEPTED=True` and inspect what kagent built under the hood

## Mental Model

> Think of an `MCPServer` resource as a dependency declaration — like a line in `package.json` or `requirements.txt`. kagent handles "package resolution" and deployment automatically. You declare what MCP server you want; kagent fetches, installs, and runs it in the cluster.

When you apply an `MCPServer` with `transportType: stdio`:

1. kagent's controller sees the new `MCPServer` CR.
2. It creates a Kubernetes `Deployment` that runs the `cmd` + `args` you specified (here `npx kubernetes-mcp-server@latest`).
3. `npx` fetches the latest `kubernetes-mcp-server` package from npm at startup.
4. The MCP server process runs in a Pod and is reachable over the stdio transport that kagent injects.

Because stdio transport doesn't speak HTTP, **do not specify a `port`** for stdio MCP servers.

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) → [002](002-licenses-and-secrets.md) → [003](003-install-kagent-enterprise.md)
## 1. Create the MCP Server

```bash
kubectl apply -f - <<EOF
apiVersion: kagent.dev/v1alpha2
kind: MCPServer
metadata:
  name: mcp-kubernetes-server
  namespace: kagent
spec:
  deployment:
    args:
    - kubernetes-mcp-server@latest
    cmd: npx
  stdioTransport: {}
  transportType: stdio
EOF
```

A few seconds later kagent will have created the underlying `Deployment`:

```bash
kubectl get deploy -n kagent | grep mcp-kubernetes-server
kubectl logs -n kagent -l mcpserver=mcp-kubernetes-server -f
```

The first time, `npx` fetches `kubernetes-mcp-server@latest` from npm — give it a minute.

## 2. Create the Agent

The Agent is `type: Declarative` (no custom container — kagent builds the agent process from the declarative spec). The `tools` block binds the agent to `mcp-kubernetes-server` and lists exactly which tools from that server the agent is allowed to call.

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
        - pods_log
        - pods_exec
        - pods_run
        - resources_list
        - resources_get
        - resources_create_or_update
        - resources_delete
EOF
```

## 3. Wait for the Agent to Be Ready

```bash
kubectl get agents -n kagent
```

Wait until both `READY` and `ACCEPTED` are `True`:

```
NAME                    READY   ACCEPTED   AGE
kubernetes-mcp-agent    True    True       45s
```

## 4. Peek Under the Hood

```bash
kubectl describe agent kubernetes-mcp-agent -n kagent
```

You should see:

- A `Status` block with conditions `Accepted=True` and `Ready=True`
- The generated Kubernetes `Deployment`, `Service`, and (if Istio Ambient is enabled) `Waypoint` that the controller produced

```bash
kubectl get all -n kagent -l kagent=kubernetes-mcp-agent
```

## 5. Test It

Open the kagent UI (port-forward from [020 step 4](003-install-kagent-enterprise.md#4-work-around-the-ui-backend-bug-port-forward) if you took the Gloo Operator path), find `kubernetes-mcp-agent`, and ask:

```
What tools do you have available?
```

The agent should list each tool name from your `toolNames` allow-list. Then try:

```
List all pods in the kagent namespace.
```

It should call `pods_list_in_namespace` and return a Markdown table.

## Cleanup

```bash
# Agent + MCP server created in steps 1-2
kubectl delete agent     kubernetes-mcp-agent  -n kagent --ignore-not-found
kubectl delete mcpserver mcp-kubernetes-server -n kagent --ignore-not-found
```

## What's Next

You can repeat this pattern with **any** MCP server that has a published npm/PyPI package or a public container image. The MCP server doesn't have to be in npm — `spec.deployment.image` lets you point at a pre-built image instead of using `cmd: npx`. See [060 step 1](030-accesspolicy-agent-to-mcp.md#1-declarative-agent--access-policy) for the `mcp/everything` image example.

## Next

- [041 — Agent A2A Skills Metadata](011-agent-skills.md) — surface skills metadata on this Agent
- [060 — `AccessPolicy`: Agent → MCP](030-accesspolicy-agent-to-mcp.md) — restrict which tools the agent can call
