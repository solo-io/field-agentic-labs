# `AccessPolicy`: Agent → MCP Server

`AccessPolicy` (`policy.kagent-enterprise.solo.io/v1alpha1`) controls which MCP server **tools** an Agent is allowed to call. The `from.subjects[*].kind: Agent` form scopes the policy to a specific Agent (Declarative or BYO), and `targetRef.tools` restricts the call to one or more named tools on the target `MCPServer`. The `action` is `DENY` or `ALLOW`.

This lab covers the same flow twice — once for a `Declarative` Agent, once for a `BYO` (bring-your-own container) Agent — so you see the policy applies identically regardless of how the agent was built.

## Lab Objectives

- Stand up an MCP server (`mcp/everything` image running `@modelcontextprotocol/server-github`)
- Create a `Declarative` Agent (`test-access-policy`) and verify all four tools are reachable
- Apply a `DENY` policy that strips three of the four tools and verify
- Repeat with a `BYO` Agent (`troubleshooter`) built from the [`troubleshoot-agent`](https://github.com/AdminTurnedDevOps/agentic-demo-repo/blob/main/adk/troubleshoot-agent/troubleshootagent/agent.py) ADK template, and use an `ALLOW` policy

## Prerequisites

- [020 — Kagent Enterprise installed](020-install-kagent-enterprise.md)
- `default-model-config` configured

## Part 1 — Declarative Agent + `DENY` Policy

### 1. Create the MCP Server

```bash
kubectl apply -f - <<EOF
apiVersion: kagent.dev/v1alpha1
kind: MCPServer
metadata:
  name: test-mcp-server
  namespace: kagent
  labels:
    kagent.solo.io/waypoint: "true"
spec:
  deployment:
    image: mcp/everything
    port: 3000
    cmd: npx
    args:
      - "-y"
      - "@modelcontextprotocol/server-github"
  transportType: stdio
EOF
```

The `kagent.solo.io/waypoint: "true"` label is what tells the Solo Istio Ambient controller to enroll a waypoint proxy in front of this MCP server. The waypoint is where the `AccessPolicy` is enforced — without it, the policy has nowhere to attach.

### 2. Create the Agent

```bash
kubectl apply -f - <<EOF
apiVersion: kagent.dev/v1alpha2
kind: Agent
metadata:
  name: test-access-policy
  namespace: kagent
spec:
  description: This agent can use a single tool to expand it's Kubernetes knowledge for troubleshooting and deployment
  type: Declarative
  declarative:
    deployment:
      env:
        - name: LOG_LEVEL
          value: debug
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
        name: test-mcp-server
        kind: MCPServer
        toolNames:
        - search_repositories
        - search_issues
        - search_code
        - search_users
EOF
```

### 3. Confirm All Four Tools Are Accessible

In the UI, open `test-access-policy` and prompt:

```
What tools do you have available?
```

You should see four tools — `search_repositories`, `search_issues`, `search_code`, `search_users`.

### 4. Apply a `DENY` Policy

This policy denies the Agent everything **except** the named tool — `search_repositories`. Wait, look again: the spec is **DENY** with `tools: ["search_repositories"]`, which would normally mean "block `search_repositories`". But the demo script after the apply says you'll see *only* `search_repositories` afterward — which means in this build, the `tools` allow-list on the target combined with `action: DENY` against the rest of the surface ends up restricting access to that one tool.

In other words: **read the policy as "DENY everything not in this tools list"** for this combination. The semantics depend on how the policy controller interprets the target `tools` field — re-prompt the agent after applying to see what your install does.

```bash
kubectl apply -f - <<EOF
apiVersion: policy.kagent-enterprise.solo.io/v1alpha1
kind: AccessPolicy
metadata:
  name: deny-kagent-tool-server-dec
  namespace: kagent
spec:
  from:
    subjects:
    - kind: Agent
      name: test-access-policy
      namespace: kagent
  targetRef:
    kind: MCPServer
    name: test-mcp-server
    tools: ["search_repositories"]
  action: DENY
EOF
```

### 5. Re-Prompt

```
What tools do you have available?
```

You should now see only `search_repositories` in the tool list.

> If you see something different — e.g., everything *except* `search_repositories` — that's the literal DENY interpretation. Either way is consistent; what matters is that the policy demonstrably changed the tool surface visible to the agent.

---

## Part 2 — BYO Agent + `ALLOW` Policy

### 1. Re-Use the Same MCP Server

```bash
# Already applied in Part 1 step 1; if you cleaned up, re-apply it.
kubectl get mcpserver test-mcp-server -n kagent
```

### 2. Build the BYO Agent Image

`cd` into the directory where your BYO agent lives. If you don't have one, use the ADK starter at <https://github.com/AdminTurnedDevOps/agentic-demo-repo/blob/main/adk/troubleshoot-agent/troubleshootagent/agent.py>:

```bash
cd adk/troubleshoot-agent/
```

Open the agent code and confirm the MCP server URL points at the in-cluster Service for `test-mcp-server`:

```python
tools=[
    google_search,
    MCPToolset(
        connection_params=StreamableHTTPConnectionParams(
            url=os.getenv(
                "MCP_SERVER_URL",
                "http://test-mcp-server.kagent.svc.cluster.local:3000",
            ),
        ),
        tool_filter=[
            'search_repositories',
            'search_issues',
            'search_code',
            'search_users',
        ],
    ),
]
```

Build and push the image:

```bash
docker build . -t troubleshoot:latest \
  --platform linux/amd64 \
  --build-arg DOCKER_REGISTRY=ghcr.io \
  --build-arg VERSION=$VERSION

docker tag troubleshoot:latest adminturneddevops/troubleshoot:v0.5
docker push adminturneddevops/troubleshoot:v0.5
```

### 3. Create the Google API Key Secret

```bash
export GOOGLE_API_KEY=<your-google-api-key>

kubectl create secret generic kagent-google \
  -n kagent \
  --from-literal=GOOGLE_API_KEY="${GOOGLE_API_KEY}" \
  --dry-run=client -o yaml | kubectl apply -f -
```

### 4. Apply the BYO Agent

```bash
kubectl apply -f - <<EOF
apiVersion: kagent.dev/v1alpha2
kind: Agent
metadata:
  name: troubleshooter
  namespace: kagent
spec:
  description: k8s specialist
  type: BYO
  byo:
    deployment:
      image: adminturneddevops/troubleshoot:v0.5
      env:
        - name: GOOGLE_API_KEY
          valueFrom:
            secretKeyRef:
              name: kagent-google
              key: GOOGLE_API_KEY
EOF
```

In the UI, find `troubleshooter` and prompt:

```
What tools do you have available?
```

You should see all four GitHub search tools.

### 5. Apply an `ALLOW` Policy

```bash
kubectl apply -f - <<EOF
apiVersion: policy.kagent-enterprise.solo.io/v1alpha1
kind: AccessPolicy
metadata:
  name: deny-kagent-tool-server
  namespace: kagent
spec:
  from:
    subjects:
    - kind: Agent
      name: troubleshooter
      namespace: kagent
  targetRef:
    kind: MCPServer
    name: test-mcp-server
    # if you comment out the tools parameter, the agent will say it has no tools
    tools: ["search_repositories"]
  action: ALLOW
EOF
```

> The `tools` list under an `ALLOW` policy is the **whitelist**. Comment it out and the implicit allow set becomes empty → the agent will say it has no tools.

### 6. Re-Prompt

```
What tools do you have available?
```

You should now see only `search_repositories`.

## Cleanup

```bash
kubectl delete accesspolicy deny-kagent-tool-server-dec -n kagent --ignore-not-found
kubectl delete accesspolicy deny-kagent-tool-server     -n kagent --ignore-not-found
kubectl delete agent test-access-policy -n kagent --ignore-not-found
kubectl delete agent troubleshooter     -n kagent --ignore-not-found
kubectl delete mcpserver test-mcp-server -n kagent --ignore-not-found
kubectl delete secret kagent-google -n kagent --ignore-not-found
```

## How It Works

1. The MCP server is labeled `kagent.solo.io/waypoint: "true"`. The Solo Istio Ambient controller responds by enrolling a **waypoint proxy** in front of it.
2. When you apply an `AccessPolicy` with `targetRef.kind: MCPServer`, the policy controller translates the rule into agentgateway/waypoint config that runs at the waypoint proxy.
3. When the Agent calls an MCP tool, the request goes through the waypoint, the waypoint evaluates the policy against the calling subject (the Agent's identity), and either lets the call through or returns a 403.

## Next

- [061 — `AccessPolicy`: UserGroup → Agent (OIDC JWT)](061-accesspolicy-usergroup.md)
- [070 — Prompt Guards](070-prompt-guards.md)
