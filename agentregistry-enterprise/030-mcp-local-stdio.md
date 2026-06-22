# Local stdio MCP Server (`demo-tools`)

Register a small in-tree MCP server with AgentRegistry. The server is a zero-dependency Python script that exposes three tools (`get_time`, `random_number`, `reverse_string`) over `stdio`. AgentRegistry clones it from this repo and treats it as a catalog asset that agents can reference.

This is the simplest MCP lab — it's catalog-only. No deployment to a runtime. To actually call these tools, you'd reference `demo-tools` from an Agent's `mcpServers:` block (the agent runtime spawns the MCP server in-process).

## Lab Objectives

- Register an `MCPServer` with `transport: stdio` and `source: repository` pointing at this repo
- Verify the catalog entry shows up in `arctl get mcps`

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) → [002a](002a-setup-oidc-keycloak.md) **or** [002b](002b-setup-oidc-entra.md) → [003](003-install-components.md)

## 1. Inspect the Files

[`assets/mcp/demo-mcp/`](assets/mcp/demo-mcp/) holds:

```
assets/mcp/demo-mcp/
├── mcpserver.yaml    # the AgentRegistry MCPServer manifest
└── server.py         # a ~120-line Python MCP server over stdio JSON-RPC
```

[`mcpserver.yaml`](assets/mcp/demo-mcp/mcpserver.yaml):

```yaml
apiVersion: ar.dev/v1alpha1
kind: MCPServer
metadata:
  name: demo-tools
  version: "1.0.0"
spec:
  description: "A minimal MCP server with simple tools: get_time, random_number, reverse_string"
  transport: stdio
  command: "python3 server.py"
  source:
    repository:
      url: "https://github.com/solo-io/field-agentic-labs"
      subfolder: "agentregistry-enterprise/assets/mcp/demo-mcp"
  tools:
    - name: get_time
      description: "Get the current UTC time"
    - name: random_number
      description: "Generate a random number between min and max"
    - name: reverse_string
      description: "Reverse a string"
```

## 2. Register the MCP Server

```bash
arctl apply -f assets/mcp/demo-mcp/mcpserver.yaml
```

## 3. Verify

```bash
arctl get mcps
```

Expected:

```
NAME         VERSION   DESCRIPTION
demo-tools   1.0.0     A minimal MCP server with simple tools: get_time, random_...
```

Inspect the full record:

```bash
arctl get mcp demo-tools --version 1.0.0 -o yaml
```

## Where the Server Actually Runs

Unlike [031](031-mcp-remote-github-copilot.md) (remote, deployed to a runtime), the stdio variant runs **inside** the agent's container. When an agent references `demo-tools` in its `mcpServers:` block, the agent runtime spawns `python3 server.py` as a subprocess and talks to it over stdin/stdout. So there's no `Deployment` resource for an stdio MCP — just the catalog entry.

To wire it into an agent, add the reference under `spec.mcpServers` (and `spec.tools`) on the Agent — see how [020 step 5](020-kagent-runtime-and-agent.md#5-register-the-agent) references `github-copilot-mcp-server` for the pattern.

## Cleanup

```bash
arctl delete mcp demo-tools --version 1.0.0
```

## Next

- [031 — Remote MCP via kagent (GitHub Copilot)](031-mcp-remote-github-copilot.md) — same MCPServer concept but pointing at a remote HTTP server
- [032 — Remote MCP through Agentgateway](032-mcp-through-agentgateway.md) — third MCP topology
- [050 — AccessPolicy](050-access-policies.md) — restrict which tools an agent (or user) can call on `demo-tools`
