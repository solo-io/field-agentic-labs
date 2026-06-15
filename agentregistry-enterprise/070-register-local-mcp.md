# Register a Local stdio MCP Server (`demo-tools`)

This lab registers a tiny in-tree MCP server with AgentRegistry. The server is a zero-dependency Python script that exposes three tools (`get_time`, `random_number`, `reverse_string`) over `stdio`.

## Lab Objectives

- Register an `MCPServer` with `transport: stdio` and `source: repository`
- Verify with `arctl get mcps`

## Prerequisites

- [040 — `arctl` authenticated](040-arctl-auth.md)

## 1. Inspect the Files

Under [`assets/mcp/demo-mcp/`](assets/mcp/demo-mcp/):

```
assets/mcp/demo-mcp/
├── mcpserver.yaml    # ar.dev MCPServer
└── server.py         # ~120-line Python MCP server, stdio JSON-RPC
```

[`assets/mcp/demo-mcp/mcpserver.yaml`](assets/mcp/demo-mcp/mcpserver.yaml):

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
      url: "https://github.com/AdminTurnedDevOps/agentic-demo-repo"
      subfolder: "agentregistry-enterprise/demo-mcp"
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

## What This MCP Server Does

The Python source ([`assets/mcp/demo-mcp/server.py`](assets/mcp/demo-mcp/server.py)) implements the JSON-RPC framing of the MCP protocol over stdin/stdout. It exists for two purposes:

- A reference implementation small enough to read in one sitting
- A sanity-check tool surface for runtimes that haven't been wired to a real MCP backend yet

## Deploying This MCP to a Runtime

Unlike the GitHub Copilot MCP in [071](071-register-github-copilot-mcp.md), the stdio variant runs **inside** the agent's container (the agent process spawns it via `command:`). So there is no separate `MCPServer` deployment — you just reference the `MCPServer` from the `Agent`'s `mcpServers` list, and AgentRegistry injects the runtime MCP config.

## Next

- [071 — Register a Remote MCP Server (GitHub Copilot)](071-register-github-copilot-mcp.md)
- [072 — Wire an MCP Server to an Agent](072-wire-mcp-to-agent.md)
