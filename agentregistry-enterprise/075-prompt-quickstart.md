# Prompt Quickstart

`Prompt` is a catalog asset, same family as `Agent` and `MCPServer`. It's an immutable, versioned blob of prompt text (typically a system prompt) that other agents can reference. Manage prompts with `arctl`, **not** `kubectl` — they live in the AgentRegistry catalog, not in `etcd` as CRDs.

This is the shortest catalog-asset lab in the workshop: list → create → verify → delete. Five minutes end-to-end.

## Lab Objectives

- List prompts in the catalog
- Apply a `Prompt` (`kubernetes-triage-system-prompt` v1.0.0)
- Inspect it with `arctl get`
- Delete it

## Prerequisites

- [040 — `arctl` authenticated](040-arctl-auth.md) against your AgentRegistry install

## 1. List Prompts

```bash
arctl get prompts
```

On a fresh install:

```
No prompts found.
```

## 2. Create a Prompt

The manifest is at [`assets/prompts/kubernetes-triage-system-prompt.yaml`](assets/prompts/kubernetes-triage-system-prompt.yaml):

```yaml
apiVersion: ar.dev/v1alpha1
kind: Prompt
metadata:
  name: kubernetes-triage-system-prompt
  tag: "1.0.0"
spec:
  description: "System prompt for Kubernetes troubleshooting agents"
  content: |
    You are a Kubernetes troubleshooting assistant.
    Be concise, ask for missing context, and prioritize evidence from kubectl output.
    When diagnosing failures, check resource status, events, logs, and recent changes before recommending fixes.
```

Apply it:

```bash
arctl apply -f assets/prompts/kubernetes-triage-system-prompt.yaml
```

Expected:

```
Prompt/kubernetes-triage-system-prompt (1.0.0) created
```

> **If [approval workflows](081-approval-workflows.md) are enabled and you are not an admin**, the expected result is `staged` rather than `created` — the prompt is parked in the approval queue. See [081 step 5](081-approval-workflows.md#5-list-pending-approval-requests-cli) for how to list and approve pending requests. `Prompt` is one of the four approval-gated catalog kinds (`Agent`, `MCPServer`, `Skill`, `Prompt`).

## 3. Verify the Prompt

List again:

```bash
arctl get prompts
```

Inspect the full record:

```bash
arctl get prompt kubernetes-triage-system-prompt --tag "1.0.0" -o yaml
```

You should see the description, content, and metadata you applied.

## 4. Delete the Prompt

```bash
arctl delete prompt kubernetes-triage-system-prompt --tag "1.0.0"
```

## Why Prompts Are a Catalog Asset

Centralizing prompt text as catalog assets gives you a few things the inline `systemMessage` on an `Agent` doesn't:

| Concern | Inline `systemMessage` on `Agent` | `Prompt` catalog asset |
|---|---|---|
| Version pinning | Tied to the agent version | Independent `tag`, multiple agents can pin different versions |
| Reuse | Copy/paste between agents | Reference by `name` + `tag` |
| Access control | Implicit via the agent's policies | Standalone — `AccessPolicy` can grant `registry:read` on `prompt` separately ([080](080-access-policies.md)) |
| Approval gating | Gated as part of the agent submission | Gated independently when `requireCreateApproval=true` ([081](081-approval-workflows.md)) |
| Auditability | Buried in the agent spec | Top-level catalog entry — appears in `arctl get prompts` and in the UI catalog |

Treat prompts the same way you'd treat a shared library in code: version it, name it, and have callers pin to a specific tag.

## Next

- [080 — AccessPolicy](080-access-policies.md) — grant `registry:read` on `prompt` resources to specific groups
- [081 — Approval Workflows](081-approval-workflows.md) — see how the `staged` vs `created` flow works for prompts
