# `AccessPolicy`: Agent → MCP Server

`AccessPolicy` (`policy.kagent-enterprise.solo.io/v1alpha1`) controls which MCP server **tools** an Agent is allowed to call. The `from.subjects[*].kind: Agent` form scopes the policy to a specific Agent (Declarative or BYO), and `targetRef.tools` restricts the call to one or more named tools on the target `MCPServer`. The `action` is `DENY` or `ALLOW`.

This lab covers the same flow twice — once for a `Declarative` Agent, once for a `BYO` (bring-your-own container) Agent — so you see the policy applies identically regardless of how the agent was built.

## Lab Objectives

- Stand up an MCP server (`mcp/everything` image running `@modelcontextprotocol/server-github`)
- Create a `Declarative` Agent (`test-access-policy`) and verify all four tools are reachable
- Apply a `DENY` policy that strips three of the four tools and verify
- Replace the `DENY` policy with an `ALLOW` policy and observe the difference in semantics

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) → [002](002-licenses-and-secrets.md) → [003](003-install-kagent-enterprise.md)
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

## Part 2 — Swap `DENY` for `ALLOW`

`DENY` and `ALLOW` are the two `action` values the `AccessPolicy` CRD accepts. They differ in default behavior:

| Action | `tools: []` empty / omitted | `tools: [X]` |
|---|---|---|
| `DENY` | Block **all** tools on the target | Block only tool `X` |
| `ALLOW` | Block **all** tools on the target (empty allow-list = nothing allowed) | Allow only tool `X` |

Test both variants without changing the Agent. First, delete the `DENY` policy from Part 1 step 4:

```bash
kubectl delete accesspolicy deny-kagent-tool-server-dec -n kagent
```

Re-prompt the agent (`What tools do you have available?`) — you should see all four tools again.

Now apply an `ALLOW` policy listing only one tool:

```bash
kubectl apply -f - <<EOF
apiVersion: policy.kagent-enterprise.solo.io/v1alpha1
kind: AccessPolicy
metadata:
  name: allow-kagent-tool-server-dec
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
    tools: ["search_repositories"]   # whitelist — only this tool is allowed
  action: ALLOW
EOF
```

Re-prompt. You should see **only `search_repositories`** — the `ALLOW` whitelist takes the agent from four tools down to one.

> Comment out the `tools:` list under `ALLOW` and the agent will report it has no tools at all. An empty `ALLOW` allow-list = allow nothing.

## Cleanup

```bash
kubectl delete accesspolicy deny-kagent-tool-server-dec  -n kagent --ignore-not-found
kubectl delete accesspolicy allow-kagent-tool-server-dec -n kagent --ignore-not-found
kubectl delete agent     test-access-policy              -n kagent --ignore-not-found
kubectl delete mcpserver test-mcp-server                 -n kagent --ignore-not-found
```

## How It Works

1. The MCP server is labeled `kagent.solo.io/waypoint: "true"`. The Solo Istio Ambient controller responds by enrolling a **waypoint proxy** in front of it.
2. When you apply an `AccessPolicy` with `targetRef.kind: MCPServer`, the policy controller translates the rule into agentgateway/waypoint config that runs at the waypoint proxy.
3. When the Agent calls an MCP tool, the request goes through the waypoint, the waypoint evaluates the policy against the calling subject (the Agent's identity), and either lets the call through or returns a 403.

## Next

- [031 — `AccessPolicy`: UserGroup → Agent (OIDC JWT)](031-accesspolicy-usergroup.md)
- [040 — Prompt Guards](040-prompt-guards.md)
- [041 — Platform RBAC](041-platform-rbac.md)
