# Build a Custom Python MCP Server (Pharmaceutical Example)

The two MCP labs before this one ([040](010-mcp-connection-agent-config.md), [041](011-agent-skills.md)) use an MCP server someone else published. This lab walks through writing one from scratch: a pharmaceutical-themed MCP server in Python with three tools (`check_drug_interactions`, `get_medication_info`, `search_clinical_trials`) backed by a mock in-memory database.

The example is **reference code** — it uses the `mcp` Python SDK and exposes the tools over `stdio_server`. You can run it locally with an MCP client like Claude Desktop, or package it as a container image and use it as the `spec.deployment.image` of an `MCPServer` CR.

## Lab Objectives

- Inspect the structure of an MCP server (`list_tools`, `call_tool`, `list_resources`)
- Run the pharma server locally and connect it to Claude Desktop
- Understand how to adapt this pattern into a deployable kagent `MCPServer`

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) → [002](002-licenses-and-secrets.md) → [003](003-install-kagent-enterprise.md)
## Source

The full source is checked in at [`assets/mcp-server-example/pharma_mcp_server.py`](assets/mcp-server-example/pharma_mcp_server.py). It's 258 lines and compiles cleanly against Python 3.13.

What's inside:

- `DRUG_DATABASE` — a mock dictionary of 4 drugs (aspirin, warfarin, metformin, lisinopril), each with `name`, `class`, `interactions`, `uses`, `dosage`, `contraindications`.
- `CLINICAL_TRIALS` — a mock dictionary of trials keyed by condition (`diabetes`, `hypertension`, `oncology`).
- `app = Server("pharma-mcp-server")` — the `mcp.server.Server` instance.
- `@app.list_tools()` — returns three `Tool` definitions with JSON-schema `inputSchema`.
- `@app.call_tool()` — handles `check_drug_interactions`, `get_medication_info`, `search_clinical_trials`; returns Markdown-formatted `TextContent`.
- `@app.list_resources()` — returns a `pharma://formulary` `Resource` entry.
- `main()` — wraps the app in `mcp.server.stdio.stdio_server()` so it can be driven by any MCP client over stdin/stdout.

## 1. Install Dependencies

```bash
pip install mcp anthropic-mcp-server
```

> The `anthropic-mcp-server` dependency is what the source doc lists. The `mcp` package is what the code actually imports. If `anthropic-mcp-server` doesn't resolve in your environment, just `pip install mcp` is sufficient to run the example.

## 2. Run Locally with Claude Desktop

```bash
cd assets/mcp-server-example
python pharma_mcp_server.py
```

The process will block on stdin waiting for MCP JSON-RPC traffic — that's expected for a stdio MCP server.

Configure Claude Desktop to use it. On macOS edit `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "pharma-server": {
      "command": "python",
      "args": ["/absolute/path/to/assets/mcp-server-example/pharma_mcp_server.py"]
    }
  }
}
```

Restart Claude Desktop and ask:

```
Check if there are any interactions between aspirin and warfarin.
```

```
Tell me about metformin - what's it used for and what are the contraindications?
```

```
Are there any clinical trials for diabetes currently recruiting?
```

Sample output for the interaction check:

```
**Drug Interaction Alert**

**Interaction 1:**
- **Drugs**: Aspirin - Warfarin
- **Severity**: Moderate to High
- **Mechanism**: NSAID interaction with Anticoagulant
- **Recommendation**: Consult prescriber. May require dose adjustment or monitoring.
```

## 3. Deploy as a kagent `MCPServer`

Two paths:

### Option A — `cmd: python` pointing at a code-mounted ConfigMap

Quick to demo, brittle for production. Mount the `.py` and `requirements.txt` from a ConfigMap and have a `python:3.13-slim` container `pip install` at startup and exec the script.

This is the same pattern the OBO lab uses for [`llm-obo-proxy/deployment.yaml`](assets/llm-obo-proxy/deployment.yaml) — see [090 step 7b](070-obo-entra.md#7b-deploy-the-in-cluster-llm-proxy-service) for the exact pattern. The shape ends up like:

```yaml
apiVersion: kagent.dev/v1alpha2
kind: MCPServer
metadata:
  name: pharma-mcp-server
  namespace: kagent
spec:
  deployment:
    image: python:3.13-slim
    cmd: /bin/sh
    args:
      - -c
      - "cd /app && pip install --no-cache-dir mcp && python pharma_mcp_server.py"
    volumes:
      - name: code
        configMap: { name: pharma-mcp-code }
    volumeMounts:
      - { name: code, mountPath: /app }
  stdioTransport: {}
  transportType: stdio
```

…and you'd `kubectl create configmap pharma-mcp-code --from-file=...` ahead of time.

### Option B — Package into a container image (recommended for production)

Write a tiny `Dockerfile`:

```dockerfile
FROM python:3.13-slim
WORKDIR /app
COPY pharma_mcp_server.py .
RUN pip install --no-cache-dir mcp
CMD ["python", "pharma_mcp_server.py"]
```

Build, push, and reference it:

```bash
docker buildx build --platform linux/amd64 \
  -t <your-registry>/pharma-mcp:0.1.0 \
  --push assets/mcp-server-example/

kubectl apply -f - <<EOF
apiVersion: kagent.dev/v1alpha2
kind: MCPServer
metadata:
  name: pharma-mcp-server
  namespace: kagent
spec:
  deployment:
    image: <your-registry>/pharma-mcp:0.1.0
  stdioTransport: {}
  transportType: stdio
EOF
```

Then point any `Declarative` Agent at it the same way [040](010-mcp-connection-agent-config.md) does:

```yaml
tools:
- type: McpServer
  mcpServer:
    name: pharma-mcp-server
    kind: MCPServer
    toolNames:
    - check_drug_interactions
    - get_medication_info
    - search_clinical_trials
```

## Cleanup

If you deployed the pharma server as an MCPServer (step 3, Option A or B):

```bash
kubectl delete mcpserver pharma-mcp-server -n kagent --ignore-not-found
# If you used Option A (ConfigMap), also:
kubectl delete configmap pharma-mcp-code   -n kagent --ignore-not-found
```

If you only ran the local Claude Desktop integration in step 2, there's nothing to clean up on the cluster. Just remove the `pharma-server` entry from `~/Library/Application Support/Claude/claude_desktop_config.json` and restart Claude Desktop.

If you built a container image and pushed it to a registry, use your registry's tooling to remove it.

## Productionizing

The mock database is fine for a demo. For real use, swap `DRUG_DATABASE` / `CLINICAL_TRIALS` for live integrations:

- **FDA Drug Database APIs**
- **RxNorm / RxNav** for standardized drug naming
- **ClinicalTrials.gov API** for real trial data
- **DrugBank** or **FirstDataBank** for comprehensive interaction checking
- **HL7 FHIR** for EHR integration
- **Audit logging** for HIPAA / 21 CFR Part 11

## Safety Disclaimer

This is a demonstration system. In production:

- Always validate against authoritative pharmaceutical databases
- Implement proper clinical decision-support safeguards
- Ensure compliance with HIPAA, FDA
- Require healthcare-professional oversight

## Next

- [060 — `AccessPolicy`: Agent → MCP](030-accesspolicy-agent-to-mcp.md) — restrict which of these three tools an agent is allowed to call
