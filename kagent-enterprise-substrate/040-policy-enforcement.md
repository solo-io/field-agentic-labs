# Policy Enforcement

Use three policy types to control one agent:

- **Who can use it.** An `EnterpriseKagentRBACPolicy` decides which Keycloak groups can see the agent and start conversations with it. kagent Enterprise enforces this at its API, before a request reaches Substrate.
- **Where its Actor can connect.** An `EgressPolicy` adds one origin to the destinations the egress gateway allows for the agent's Actors. Everything not inferred from the agent's models and tools, or granted explicitly, stays unreachable.
- **What it can send to its model.** An `EnterpriseAgentgatewayPolicy` on a dedicated Enterprise agentgateway rejects prompts that match a rule, after the network has already allowed the call.

```
  policy-admin / policy-writer / policy-reader / policy-norole (Keycloak)
        │ Bearer token
        ▼
  kagent controller, A2A :8083 ............ EnterpriseKagentRBACPolicy
        │
        ▼
  policy-assistant Actor on a Worker
        │
        ▼
  atenet-egress (ate-system) .............. EgressPolicy, plus origins kagent infers
        │                       │
        ▼                       ▼
  policy-demo-target     policy-llm Enterprise agentgateway .... EnterpriseAgentgatewayPolicy
  (policy-demo)          (agentgateway-system)
                                │ OpenAI key added here
                                ▼
                         api.openai.com
```

| Policy | What it controls | Who enforces it |
|---|---|---|
| `EnterpriseKagentRBACPolicy` | Viewing and managing agents and their configuration, and starting conversations | The kagent controller's API |
| `EgressPolicy` | Extra HTTP(S) origins an Agent's Actors can reach | `atenet-egress`, through the Actor policy kagent generates |
| `EnterpriseAgentgatewayPolicy` | Authentication and content rules on LLM traffic | The Enterprise agentgateway proxy |

kagent infers the model, MCP, and control-plane destinations from the agent's prepared runtime, adds the origins from the Agent's `egressPolicyRefs`, and programs the result into each Actor's Substrate policy. You write the `EgressPolicy`. kagent owns the generated Actor policy, so don't edit it.

## Lab Objectives

- Add `admins`, `writers`, and `readers` groups and four users to the Keycloak realm from 030
- Grant agent access by group with `EnterpriseKagentRBACPolicy`, and check what each user can and cannot do
- Block an Actor from an in-cluster service, then grant that origin with an `EgressPolicy`, then revoke it
- Install Enterprise agentgateway and route the agent's model calls through a dedicated LLM gateway that holds the OpenAI key
- Reject a prompt with a `promptGuard` regex
- Compare what each user sees in the Tracing view

## Prerequisites

- [030](030-oauth-models-mcp-agents.md) completed through [step 4](030-oauth-models-mcp-agents.md#4-turn-on-oidc-and-token-propagation), on the rc2 baseline that lab describes: Keycloak runs in `keycloak`, and kagent Enterprise validates its tokens. The rest of 030 is optional.
- The 030 lab shell, with `KEYCLOAK_ADMIN_PASSWORD`, `DEMO_USER`, and `DEMO_PASSWORD` still set, and the `kc_token` and `jwt_claims` helpers from [030 step 3](030-oauth-models-mcp-agents.md#3-get-a-user-token) defined
- The Keycloak port-forward from 030 step 3 running: `kubectl port-forward -n keycloak svc/keycloak 8180:8180`
- `AGENTGATEWAY_LICENSE_KEY` is not needed again. This lab copies the `enterprise-agentgateway-license` Secret from [002 step 9](002-install.md#9-create-the-license-and-provider-secrets).
- The `kagent-openai` Secret from 002 step 9
- `kubectl`, `helm` v3, `curl`, `jq`, `openssl`, `uuidgen`
- Pods can pull `python:3.12-alpine` and the Enterprise agentgateway images from `us-docker.pkg.dev/solo-public`
- Local ports `8080`, `8083`, `8180`, and `18081` are free

> **Policies apply to the whole kagent API.** Once any `EnterpriseKagentRBACPolicy` exists, policy grants decide access to every agent, template, Harness, and ModelConfig, not only the ones in this lab. `alice` from 030 belongs to none of the groups below, so she loses access to 030's agents until the cleanup removes the policies. `kubectl` is not affected: it goes through Kubernetes RBAC, not the kagent API.

## What Gets Installed

| Object | Namespace | Purpose |
|---|---|---|
| Groups `admins`, `writers`, `readers`, and users `policy-admin`, `policy-writer`, `policy-reader`, `policy-norole` | Keycloak realm `kagent` | The principals the RBAC policies match |
| Harness `kagent-policy`, AgentTemplate and Agent `policy-assistant` | `kagent` | The agent under test |
| EnterpriseKagentRBACPolicies `policy-lab-admins`, `policy-lab-viewers`, `policy-lab-writers` | `kagent` | Who can see the agent and start conversations |
| Namespace `policy-demo`, with `policy-demo-target` and `policy-demo-probe` | `policy-demo` | An HTTP service the Actor is not allowed to reach by default |
| EgressPolicy `policy-demo-target` | `kagent` | Grants that service's origin |
| Gateway API CRDs `v1.6.1` | cluster | Required by Enterprise agentgateway |
| `enterprise-agentgateway-crds` and `enterprise-agentgateway` releases, license Secret | `agentgateway-system` | Enterprise agentgateway controller and the `enterprise-agentgateway` GatewayClass |
| Gateway `policy-llm`, its parameters, backend, route, provider Secret, and policy | `agentgateway-system` | The dedicated LLM gateway and its prompt guard |
| ModelConfig `policy-gateway-model` | `kagent` | Sends the agent's model calls to `policy-llm` |

## 1. Set the Lab Variables

```bash
export POLICY_PASSWORD=$(openssl rand -hex 16)
export AGW_VERSION=v2026.10.2
export GATEWAY_API_VERSION=v1.6.1
```

All four lab users share `POLICY_PASSWORD`. Keep this shell open: later steps use these variables and the helpers you define along the way.

## 2. Add Groups and Users to Keycloak

030's realm has one group, `kagent-users`. Add the three groups the policies match and one user per role through Keycloak's admin API. `master` realm admin tokens last one minute, so `kc_admin_token` gets a new one for each call. `kc_admin` sends one admin request and prints its HTTP status:

```bash
kc_admin_token() {
  curl -fsS http://127.0.0.1:8180/realms/master/protocol/openid-connect/token \
    -d grant_type=password -d client_id=admin-cli --data-urlencode username=admin \
    --data-urlencode password="${KEYCLOAK_ADMIN_PASSWORD:?run 030 step 1}" | jq -r .access_token
}

kc_admin() {
  curl -sS -o /dev/null -w "%{http_code}\n" -X "$1" "http://127.0.0.1:8180/admin/realms/kagent$2" \
    -H "Authorization: Bearer $(kc_admin_token)" -H 'Content-Type: application/json' ${3:+-d "$3"}
}

for g in admins writers readers; do
  kc_admin POST /groups "{\"name\": \"$g\"}"
done

for role in admin writer reader norole; do
  if [ "$role" = norole ]; then groups='[]'; else groups="[\"/${role}s\"]"; fi
  kc_admin POST /users "$(jq -n --arg u "policy-$role" --arg p "$POLICY_PASSWORD" --argjson g "$groups" \
    '{username: $u, enabled: true, email: ($u + "@example.com"), emailVerified: true,
      firstName: "Policy", lastName: $u, groups: $g,
      credentials: [{type: "password", value: $p, temporary: false}]}')"
done
```

Expected: seven `201` lines. `409` means the group or user already exists, which is fine on a rerun.

| User | Group |
|---|---|
| `policy-admin` | `admins` |
| `policy-writer` | `writers` |
| `policy-reader` | `readers` |
| `policy-norole` | none |

The `groups` protocol mapper from 030 already puts group names in the `Groups` claim of `kagent-ui` tokens, and `Groups` is the claim kagent Enterprise reads for RBAC by default (`enterprise.rbac.groupsClaim`).

> **Keycloak keeps these users in memory.** 030 runs Keycloak in `start-dev` mode. If the pod restarts, it re-imports the realm with `alice` only. Rerun this step, then step 3.

## 3. Get a Token for Each User

```bash
user_token() {
  curl -fsS http://127.0.0.1:8180/realms/kagent/protocol/openid-connect/token \
    -d grant_type=password -d client_id=kagent-ui -d scope=openid \
    --data-urlencode username="$1" --data-urlencode password="$POLICY_PASSWORD" \
    | jq -r .access_token
}

refresh_tokens() {
  T_ADMIN=$(user_token policy-admin)
  T_WRITER=$(user_token policy-writer)
  T_READER=$(user_token policy-reader)
  T_NOROLE=$(user_token policy-norole)
  T_ALICE=$(kc_token)
}

refresh_tokens
for t in "$T_ADMIN" "$T_WRITER" "$T_READER" "$T_NOROLE" "$T_ALICE"; do
  echo "$t" | jwt_claims | jq -c '{preferred_username, Groups}'
done
```

Expected:

```json
{"preferred_username":"policy-admin","Groups":["admins"]}
{"preferred_username":"policy-writer","Groups":["writers"]}
{"preferred_username":"policy-reader","Groups":["readers"]}
{"preferred_username":"policy-norole","Groups":[]}
{"preferred_username":"alice","Groups":["kagent-users"]}
```

A user with no groups can also come back with no `Groups` claim at all. kagent treats that the same as an empty list. Tokens last 30 minutes. Run `refresh_tokens` whenever a later call returns `401`.

## 4. Create the Lab Agent

The agent gets its own Harness so this lab does not depend on 010's or 030's objects. The Harness omits `workload.image`, so it runs the release's built-in kagent runtime. That image includes BusyBox `wget`, which the egress checks use through the agent's built-in `bash` tool.

```bash
kubectl apply -f - <<'EOF'
apiVersion: api.kagent.dev/v1alpha3
kind: Harness
metadata:
  name: kagent-policy
  namespace: kagent
spec:
  kagent: {}
  substrate:
    workerPoolRef:
      name: kagent-default
    snapshotPolicy:
      location: gs://ate-snapshots/kagent/
---
apiVersion: api.kagent.dev/v1alpha3
kind: AgentTemplate
metadata:
  name: policy-assistant
  namespace: kagent
spec:
  description: Runs commands for policy checks and reports their exact output.
  systemPrompt: |
    You are a test assistant for policy checks. When asked to run a command, run it
    with the bash tool exactly as given, without installing anything or changing the
    destination. Report the command's stdout and stderr verbatim.
  modelConfig:
    name: default-model-config
---
apiVersion: api.kagent.dev/v1alpha3
kind: Agent
metadata:
  name: policy-assistant
  namespace: kagent
spec:
  template:
    ref:
      name: policy-assistant
  harnessRef:
    name: kagent-policy
EOF

kubectl wait agent/policy-assistant -n kagent --for=condition=Ready --timeout=600s
```

In another terminal, forward the controller's A2A port, unless 030's port-forward is still running:

```bash
kubectl port-forward -n kagent svc/kagent-controller 8083:8083
```

Define two helpers. `card_as` prints the HTTP status of an agent card read. `a2a_as` sends one `SendMessage` as the given token, optionally into an existing conversation, prints the reply, and saves the conversation's context ID in `LAST_CONTEXT`:

```bash
card_as() {
  curl -sS -o /dev/null -w "%{http_code}\n" -H "Authorization: Bearer $1" \
    "http://127.0.0.1:8083/agents/kagent/$2/.well-known/agent-card.json"
}

a2a_as() {
  local body resp
  body=$(jq -n --arg id "$(uuidgen | tr '[:upper:]' '[:lower:]')" --arg text "$3" --arg ctx "${4:-}" \
    '{jsonrpc: "2.0", id: 1, method: "SendMessage",
      params: {message: ({messageId: $id, role: "ROLE_USER", parts: [{text: $text}]}
        + (if $ctx == "" then {} else {contextId: $ctx} end))}}')
  resp=$(curl -sS --max-time 300 "http://127.0.0.1:8083/agents/kagent/$2" \
    -H "Authorization: Bearer $1" -H "A2A-Version: 1.0" -H "Content-Type: application/json" -d "$body")
  LAST_CONTEXT=$(echo "$resp" | jq -r '(.result.task // .result.message // {}).contextId // empty' 2>/dev/null)
  echo "$resp" | jq '.error // (.result | .task // .message | {state: .status.state, context: .contextId,
      text: ([.artifacts[]?.parts[]?.text, .status.message.parts[]?.text, .parts[]?.text]
             | map(select(. != null)) | join("\n"))})' 2>/dev/null || echo "$resp"
}
```

Before any policy exists, every signed-in user can use the agent. Try the user with no groups:

```bash
card_as "$T_NOROLE" policy-assistant
a2a_as "$T_NOROLE" policy-assistant "Reply with exactly: BEFORE_POLICY_OK"
```

Expected: `200`, then `"state": "TASK_STATE_COMPLETED"` with `BEFORE_POLICY_OK`. The first call can take a minute while the conversation's Actor starts. With no policy objects, kagent opens ordinary resources to any authenticated caller.

## 5. Apply the RBAC Policies

Three policies, applied admins first. While any policy exists, admins also need an explicit grant, so create theirs before the others:

```bash
kubectl apply -f - <<'EOF'
apiVersion: kagent-enterprise.solo.io/v1alpha1
kind: EnterpriseKagentRBACPolicy
metadata:
  name: policy-lab-admins
  namespace: kagent
spec:
  principals:
  - all:
    - attribute: groups
      values: [admins]
  grants:
  - name: manage-kagent
    resource: kagent:*
    accessLevel: manage
EOF

kubectl wait enterprisekagentrbacpolicy/policy-lab-admins -n kagent --for=condition=Accepted --timeout=60s
```

Readers and writers can both see the agent and what it is built from. Only writers get `manage` on Sessions, which is what starting a conversation takes:

```bash
kubectl apply -f - <<'EOF'
apiVersion: kagent-enterprise.solo.io/v1alpha1
kind: EnterpriseKagentRBACPolicy
metadata:
  name: policy-lab-viewers
  namespace: kagent
spec:
  principals:
  - all:
    - attribute: groups
      values: [readers, writers]
  grants:
  - name: view-namespaces
    resource: kagent:system-namespaces
    accessLevel: view
  - name: view-agent
    resource: kagent:agents
    accessLevel: view
    constraints:
    - all:
      - attribute: namespace
        values: [kagent]
      - attribute: name
        values: [policy-assistant]
  - name: view-agent-template
    resource: kagent:agent-templates
    accessLevel: view
    constraints:
    - all:
      - attribute: namespace
        values: [kagent]
      - attribute: name
        values: [policy-assistant]
  - name: view-harness
    resource: kagent:harnesses
    accessLevel: view
    constraints:
    - all:
      - attribute: namespace
        values: [kagent]
      - attribute: name
        values: [kagent-policy]
  - name: view-models
    resource: kagent:model-configs
    accessLevel: view
    constraints:
    - all:
      - attribute: namespace
        values: [kagent]
      - attribute: name
        values: [default-model-config, policy-gateway-model]
---
apiVersion: kagent-enterprise.solo.io/v1alpha1
kind: EnterpriseKagentRBACPolicy
metadata:
  name: policy-lab-writers
  namespace: kagent
spec:
  principals:
  - all:
    - attribute: groups
      values: [writers]
  grants:
  - name: start-conversations
    resource: kagent:sessions
    accessLevel: manage
EOF

kubectl wait enterprisekagentrbacpolicy/policy-lab-viewers enterprisekagentrbacpolicy/policy-lab-writers \
  -n kagent --for=condition=Accepted --timeout=60s
```

How the policies read:

- **`principals`** match the caller. Entries are ORed, and the conditions inside one `all` are ANDed. `groups` comes from the token's `Groups` claim.
- **`grants`** name a catalog resource and an access level. `view` reads, and `manage` writes and includes `view`. Grants from every matching policy add up.
- **`constraints`** narrow a grant to objects by `namespace` and `name`. Without them, the grant covers every object of that resource.
- **`kagent:system-namespaces`** lets the UI list namespaces. It grants nothing inside them.
- **`kagent:sessions`** has no `namespace` or `name` facts. The Session service still checks who created each conversation, so this grant never opens another user's conversations.

A policy is `Accepted` only if every grant matches the catalog. `kubectl describe enterprisekagentrbacpolicy -n kagent` shows `UnknownResource`, `UnknownAccessLevel`, or `UnknownAttribute` when one does not. An invalid policy still counts as a policy, so it still turns enforcement on.

## 6. Check Who Can Do What

Read the agent card as each user:

```bash
for t in "$T_ADMIN" "$T_WRITER" "$T_READER" "$T_NOROLE" "$T_ALICE"; do
  printf '%-14s %s\n' "$(echo "$t" | jwt_claims | jq -r .preferred_username)" "$(card_as "$t" policy-assistant)"
done
```

Expected:

```
policy-admin   200
policy-writer  200
policy-reader  200
policy-norole  403
alice          403
```

Now try to start a conversation as each user:

```bash
a2a_as "$T_WRITER" policy-assistant "Reply with exactly: WRITER_OK"
WRITER_CTX=$LAST_CONTEXT
a2a_as "$T_ADMIN"  policy-assistant "Reply with exactly: ADMIN_OK"
a2a_as "$T_READER" policy-assistant "Reply with exactly: READER_OK"
a2a_as "$T_NOROLE" policy-assistant "Reply with exactly: NOROLE_OK"
```

| User | Result | Why |
|---|---|---|
| `policy-writer` | Completed, `WRITER_OK` | Agent `view` plus Session `manage` |
| `policy-admin` | Completed, `ADMIN_OK` | `kagent:*` `manage` covers both |
| `policy-reader` | An `error` object, no task | Agent `view` without Session `manage` |
| `policy-norole` | An `error` object, no task | No grant on the agent at all |

Conversations belong to whoever created them. Try to add a message to the writer's conversation as the admin:

```bash
a2a_as "$T_ADMIN" policy-assistant "Reply with exactly: NOT_MY_CONVERSATION" "$WRITER_CTX"
```

Expected: an `error` object. `kagent:*` lets the admin manage agents and configuration, and start conversations of their own. It does not let them write into another user's conversation, because the Session service checks the creator separately from the policy grants.

> **Existing conversations keep their access.** Messages sent into a conversation that already exists are checked against Session authorization and ownership. Agent `view` is checked again only when a conversation is created. Removing a user's agent grant stops new conversations, but not one that user already has open.

## 7. Deploy the Egress Target

Deploy an HTTP service the agent has no reason to reach, and a probe pod that shows the service works on its own:

```bash
kubectl apply -f - <<'EOF'
apiVersion: v1
kind: Namespace
metadata:
  name: policy-demo
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: policy-demo-target
  namespace: policy-demo
data:
  server.py: |
    from http.server import BaseHTTPRequestHandler, HTTPServer

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            body = b"POLICY_DEMO_ALLOWED\n"
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    HTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: policy-demo-target
  namespace: policy-demo
spec:
  replicas: 1
  selector:
    matchLabels:
      app: policy-demo-target
  template:
    metadata:
      labels:
        app: policy-demo-target
    spec:
      containers:
      - name: http
        image: python:3.12-alpine
        command: [python3, -u, /demo/server.py]
        ports:
        - containerPort: 8080
        readinessProbe:
          httpGet:
            path: /
            port: 8080
        volumeMounts:
        - name: server
          mountPath: /demo
          readOnly: true
      volumes:
      - name: server
        configMap:
          name: policy-demo-target
---
apiVersion: v1
kind: Service
metadata:
  name: policy-demo-target
  namespace: policy-demo
spec:
  selector:
    app: policy-demo-target
  ports:
  - port: 8080
    targetPort: 8080
---
apiVersion: v1
kind: Pod
metadata:
  name: policy-demo-probe
  namespace: policy-demo
spec:
  automountServiceAccountToken: false
  restartPolicy: Never
  containers:
  - name: probe
    image: python:3.12-alpine
    command: [sleep, "86400"]
EOF

kubectl rollout status deploy/policy-demo-target -n policy-demo --timeout=300s
kubectl wait pod/policy-demo-probe -n policy-demo --for=condition=Ready --timeout=300s

kubectl exec -n policy-demo policy-demo-probe -- python3 -c \
  'import urllib.request; print(urllib.request.urlopen("http://policy-demo-target.policy-demo.svc.cluster.local:8080", timeout=5).read().decode())'
```

Expected: `POLICY_DEMO_ALLOWED`. The service is healthy, so any failure from the Actor in the next step comes from egress policy, not from the target.

## 8. Grant and Revoke an Origin

Start one writer conversation and use it for every probe in this step. `EGRESS_PROMPT` asks the agent to run BusyBox `wget` against the target:

```bash
EGRESS_PROMPT='Use the bash tool to run exactly this command, and report its stdout and stderr verbatim.

busybox wget -S -T 5 -O - http://policy-demo-target.policy-demo.svc.cluster.local:8080 2>&1; echo "probe_exit=$?"'

a2a_as "$T_WRITER" policy-assistant "$EGRESS_PROMPT"
EGRESS_CTX=$LAST_CONTEXT
```

Expected: the command fails and the reply ends with `probe_exit=1`. The Actor cannot resolve or reach `policy-demo-target`, because no model, tool, or grant puts that origin in its policy. The exact `wget` message depends on where the request is stopped, for example `bad address`.

Create the grant, and reference it from the Agent:

```bash
kubectl apply -f - <<'EOF'
apiVersion: api.kagent.dev/v1alpha3
kind: EgressPolicy
metadata:
  name: policy-demo-target
  namespace: kagent
spec:
  destinations:
  - origin: http://policy-demo-target.policy-demo.svc.cluster.local:8080
EOF

kubectl patch agent.api.kagent.dev policy-assistant -n kagent --type=merge \
  -p '{"spec":{"egressPolicyRefs":[{"name":"policy-demo-target"}]}}'

kubectl get agent policy-assistant -n kagent \
  -o jsonpath='{range .status.conditions[?(@.type=="EgressPoliciesApplied")]}{.status} {.reason}{"\n"}{end}'
```

Expected: `True Applied`. Send the same prompt into the same conversation:

```bash
a2a_as "$T_WRITER" policy-assistant "$EGRESS_PROMPT" "$EGRESS_CTX"
```

Expected: `HTTP/1.1 200 OK`, `POLICY_DEMO_ALLOWED`, and `probe_exit=0`. The Actor did not restart: kagent updated the policy of the running Actor.

Rules for `EgressPolicy`:

- The reference lives on `Agent.spec.egressPolicyRefs`, not on the AgentTemplate or Harness, and the policy must be in the Agent's namespace.
- An origin is a scheme, DNS hostname, and optional port. It grants every path on that origin. A wildcard can replace only the leftmost label, for example `https://*.example.com`.
- A destination can also carry `authentication`, which makes the egress gateway replace a placeholder header with a Secret value or an Actor JWT. The runtime never sees the credential.
- If a reference is missing or two grants conflict, `EgressPoliciesApplied` turns `False` with `EgressPolicyNotFound`, `InvalidEgressPolicy`, or `EgressGrantConflict`. The agent's `Ready` condition is not affected.

Revoke the grant, and probe once more in the same conversation:

```bash
kubectl patch agent.api.kagent.dev policy-assistant -n kagent --type=json \
  -p '[{"op":"remove","path":"/spec/egressPolicyRefs"}]'

a2a_as "$T_WRITER" policy-assistant "$EGRESS_PROMPT" "$EGRESS_CTX"
```

Expected: the probe fails again with `probe_exit=1`. Revoking a reference takes effect on running Actors too.

## 9. Install Enterprise agentgateway

Substrate's router and egress gateway are agentgateway instances that the Substrate chart configures directly. A Gateway API controller is a separate install: Enterprise agentgateway reconciles `Gateway`, `HTTPRoute`, and its own backend and policy resources into proxies.

Install the Gateway API CRDs:

```bash
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}/standard-install.yaml
```

> **Clusters that already manage Gateway API CRDs.** GKE with its Gateway controller enabled, and some other platforms, install and own these CRDs. Check `kubectl get crd gateways.gateway.networking.k8s.io -o jsonpath='{.metadata.labels}'` first. If the platform owns them, follow its upgrade path to a version that is at least `v1.6.1` instead of applying over it.

Copy the license Secret from `ate-system` without printing it, then install the CRDs and the controller:

```bash
kubectl create namespace agentgateway-system

kubectl get secret enterprise-agentgateway-license -n ate-system \
  -o jsonpath='{.data.enterprise-agentgateway-license-key}' | base64 -d \
  | kubectl create secret generic enterprise-agentgateway-license -n agentgateway-system \
      --from-file=enterprise-agentgateway-license-key=/dev/stdin

helm upgrade -i enterprise-agentgateway-crds \
  oci://us-docker.pkg.dev/solo-public/enterprise-agentgateway/charts/enterprise-agentgateway-crds \
  --version $AGW_VERSION --namespace agentgateway-system

helm upgrade -i enterprise-agentgateway \
  oci://us-docker.pkg.dev/solo-public/enterprise-agentgateway/charts/enterprise-agentgateway \
  --version $AGW_VERSION --namespace agentgateway-system \
  --set licensing.createSecret=false --set licensing.secretName=enterprise-agentgateway-license

kubectl rollout status deploy/enterprise-agentgateway -n agentgateway-system --timeout=300s
kubectl get gatewayclass enterprise-agentgateway
```

Expected: `ACCEPTED` is `True` for the `enterprise-agentgateway` GatewayClass. The chart reads the key from the Secret, so it never appears in the Helm release.

## 10. Create the LLM Gateway

Copy the OpenAI key into the gateway namespace without printing it. The backend reads the `Authorization` key of this Secret:

```bash
kubectl get secret kagent-openai -n kagent -o jsonpath='{.data.OPENAI_API_KEY}' | base64 -d \
  | kubectl create secret generic policy-llm-openai -n agentgateway-system --from-file=Authorization=/dev/stdin
```

Create the gateway, its OpenAI backend, and the route between them:

```bash
kubectl apply -f - <<'EOF'
apiVersion: enterpriseagentgateway.solo.io/v1alpha1
kind: EnterpriseAgentgatewayParameters
metadata:
  name: policy-llm
  namespace: agentgateway-system
spec:
  service:
    spec:
      type: ClusterIP
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: policy-llm
  namespace: agentgateway-system
spec:
  gatewayClassName: enterprise-agentgateway
  infrastructure:
    parametersRef:
      group: enterpriseagentgateway.solo.io
      kind: EnterpriseAgentgatewayParameters
      name: policy-llm
  listeners:
  - name: http
    port: 8080
    protocol: HTTP
    allowedRoutes:
      namespaces:
        from: Same
---
apiVersion: enterpriseagentgateway.solo.io/v1alpha1
kind: EnterpriseAgentgatewayBackend
metadata:
  name: policy-llm-openai
  namespace: agentgateway-system
spec:
  ai:
    provider:
      openai: {}
  policies:
    auth:
      secretRef:
        name: policy-llm-openai
    ai:
      routes:
        /v1/chat/completions: Completions
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: policy-llm-openai
  namespace: agentgateway-system
spec:
  parentRefs:
  - name: policy-llm
    sectionName: http
  rules:
  - matches:
    - path:
        type: Exact
        value: /v1/chat/completions
    backendRefs:
    - group: enterpriseagentgateway.solo.io
      kind: EnterpriseAgentgatewayBackend
      name: policy-llm-openai
EOF

kubectl wait gateway/policy-llm -n agentgateway-system --for=condition=Programmed --timeout=300s
```

What each object does:

- **`EnterpriseAgentgatewayParameters`** makes the proxy's Service a `ClusterIP`, reachable only inside the cluster.
- **`Gateway`** has the controller deploy a proxy named `policy-llm` with one HTTP listener on `8080`.
- **`EnterpriseAgentgatewayBackend`** defines OpenAI as the upstream, adds the key from `policy-llm-openai`, and parses `/v1/chat/completions` as chat completions, so LLM policies and token accounting apply.
- **`HTTPRoute`** sends only `/v1/chat/completions` to that backend.

In another terminal, forward the proxy:

```bash
kubectl port-forward -n agentgateway-system svc/policy-llm 18081:8080
```

Send a request with no credential. The gateway adds the key:

```bash
curl -sS http://127.0.0.1:18081/v1/chat/completions -H 'Content-Type: application/json' \
  -d '{"model":"gpt-4.1-mini","messages":[{"role":"user","content":"Say hello in three words."}]}' \
  | jq -r '.choices[0].message.content // .'
```

The model's reply prints. Keep the port-forward running for step 12.

> **This gateway has no client authentication.** Anything that can reach the `policy-llm` Service can use the key. For Actors, the egress gateway decides whether they can reach it at all. To require a token at the gateway too, validate Actor JWTs or user tokens as [030](030-oauth-models-mcp-agents.md) does.

## 11. Route the Agent Through the Gateway

Create a ModelConfig that points at the gateway, and switch the agent's template to it. `default-model-config` stays as it is for other agents:

```bash
kubectl apply -f - <<'EOF'
apiVersion: api.kagent.dev/v1alpha3
kind: ModelConfig
metadata:
  name: policy-gateway-model
  namespace: kagent
spec:
  provider: OpenAI
  model: gpt-4.1-mini
  apiKeySecret: kagent-openai
  apiKeySecretKey: OPENAI_API_KEY
  openAI:
    baseUrl: http://policy-llm.agentgateway-system.svc.cluster.local:8080/v1
    apiFormat: chatCompletions
EOF

kubectl patch agenttemplate.api.kagent.dev policy-assistant -n kagent --type=merge \
  -p '{"spec":{"modelConfig":{"name":"policy-gateway-model"}}}'

kubectl wait agent/policy-assistant -n kagent --for=condition=Ready --timeout=600s
```

The runtime still holds only a placeholder. The egress gateway attaches the `kagent-openai` key on the way to the gateway, as it does for `default-model-config`, and the gateway replaces `Authorization` with its own Secret before calling OpenAI. kagent adds the gateway's origin to the Actor's policy because the ModelConfig names it, so no `EgressPolicy` is needed.

`apiFormat: chatCompletions` matches the only path the route accepts.

Start a new conversation, so it uses the new template revision:

```bash
a2a_as "$T_WRITER" policy-assistant "Reply with exactly: VIA_ENTERPRISE_GATEWAY"
```

Expected: completed, with `VIA_ENTERPRISE_GATEWAY`. Conversations created before the patch keep the revision they started with.

Read the gateway's access log:

```bash
kubectl logs -n agentgateway-system -l gateway.networking.k8s.io/gateway-name=policy-llm --tail=20 \
  | grep 'route=agentgateway-system/policy-llm-openai'
```

Each model call is one line, with `endpoint=api.openai.com:443`, `http.status=200`, `protocol=llm`, `gen_ai.request.model=gpt-4.1-mini`, the token counts, and a cost estimate.

Then ask the agent to call OpenAI directly:

```bash
a2a_as "$T_WRITER" policy-assistant 'Use the bash tool to run exactly this command, and report its stdout and stderr verbatim.

busybox wget -S -T 5 -O /dev/null https://api.openai.com/v1/models 2>&1; echo "probe_exit=$?"'
```

Expected: `probe_exit=1`. Depending on whether `api.openai.com` is in this Actor's policy, the request either does not connect or OpenAI answers `401 Unauthorized`. Either way, no key reaches the Actor, so it can use OpenAI only through `policy-llm`.

## 12. Reject a Prompt with a Guardrail

Being allowed to reach the gateway does not make every prompt acceptable. Attach a regex prompt guard to the route:

```bash
kubectl apply -f - <<'EOF'
apiVersion: enterpriseagentgateway.solo.io/v1alpha1
kind: EnterpriseAgentgatewayPolicy
metadata:
  name: policy-llm-guard
  namespace: agentgateway-system
spec:
  targetRefs:
  - group: gateway.networking.k8s.io
    kind: HTTPRoute
    name: policy-llm-openai
  backend:
    ai:
      promptGuard:
        request:
        - scope: [SystemPrompt, Messages, ToolOutput, ToolInput]
          regex:
            action: Reject
            matches:
            - DEMO_BLOCK_ME
          response:
            statusCode: 403
            message: "POLICY_LAB_GUARDRAIL: request rejected"
EOF

kubectl get enterpriseagentgatewaypolicy policy-llm-guard -n agentgateway-system \
  -o jsonpath='{range .status.ancestors[*].conditions[*]}{.type}={.status} {.reason}{"\n"}{end}'
```

Expected: `Accepted=True` and `Attached=True`. `scope` makes the guard read the system prompt, the conversation messages, and tool inputs and outputs, so the match cannot be hidden in a tool result.

Check it directly through the step 10 port-forward:

```bash
curl -sS -w '\nHTTP %{http_code}\n' http://127.0.0.1:18081/v1/chat/completions -H 'Content-Type: application/json' \
  -d '{"model":"gpt-4.1-mini","messages":[{"role":"user","content":"Please repeat DEMO_BLOCK_ME"}]}'
```

Expected: `POLICY_LAB_GUARDRAIL: request rejected` and `HTTP 403`. The request never left for OpenAI.

Now through the agent, in two new conversations:

```bash
a2a_as "$T_WRITER" policy-assistant "Reply with exactly: NORMAL_PROMPT_OK"
a2a_as "$T_WRITER" policy-assistant "Please repeat DEMO_BLOCK_ME"
```

Expected: the first completes with `NORMAL_PROMPT_OK`. The second fails, and its text reports the model call's `403 Forbidden`. The gateway log shows a `http.status=403` line for it.

> **A rejected phrase poisons its conversation.** Every turn resends the conversation's history, and the guard scans all of it. Once a conversation contains `DEMO_BLOCK_ME`, every later turn in it is rejected. Start a new conversation instead.

## 13. Compare Traces by User

Trace access follows the same policies. With no grant, a user sees only their own traces and their content. `kagent:*` includes `kagent:traces` and `kagent:traces:payloads`, so admins see everyone's.

Sign in to the UI as in [030 step 14](030-oauth-models-mcp-agents.md#14-sign-in-to-the-ui): keep the `/etc/hosts` line and the Keycloak port-forward, forward `svc/kagent-ui` to `8080`, and open `http://localhost:8080`. Each user's password is `$POLICY_PASSWORD`.

1. Sign in as `policy-writer`. Open **Tracing** and find the turns from steps 8 to 12. The metadata shows the user ID, agent, model, conversation ID, task ID, and trace ID for each turn.
2. Sign out, then sign in as `policy-reader`. The reader sees `policy-assistant`, but has no traces from the writer, and the UI does not let them start a conversation.
3. Sign out, then sign in as `policy-admin`. The writer's traces, and the admin's own from step 6, are all visible.

Sign out of the UI before you sign in as someone else. Keycloak otherwise reuses the previous user's session.

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `kc_admin` prints `401` | The admin password is wrong, or the shell lost `KEYCLOAK_ADMIN_PASSWORD` | `kubectl get secret keycloak-admin -n keycloak -o jsonpath='{.data.password}' \| base64 -d` shows it. Export it again |
| A user's token has no `Groups` claim | The user was created without groups | Add the group in the Keycloak admin console (**Users** → user → **Groups**), or delete the user and rerun step 2 |
| Every user gets `403` after step 5, including `policy-admin` | The admin policy is not `Accepted`, or the token's groups do not match | `kubectl describe enterprisekagentrbacpolicy policy-lab-admins -n kagent`, and decode the token with `jwt_claims` |
| A policy shows `UnknownResource` or `UnknownAttribute` | A grant names a resource or attribute the catalog does not have | Fix the grant. The policy still turns enforcement on while it is invalid |
| `policy-writer` gets an `error` when starting a conversation | `policy-lab-writers` is missing or not `Accepted` | `kubectl get enterprisekagentrbacpolicy -n kagent` |
| A2A calls return `401` | A token expired | `refresh_tokens` |
| The allowed probe in step 8 still fails | The reference did not apply | Check `EgressPoliciesApplied` on the Agent. `EgressPolicyNotFound` means the policy is not in `kagent` or the name differs |
| The first probe in step 8 succeeds | An earlier run left `egressPolicyRefs` on the Agent | `kubectl get agent policy-assistant -n kagent -o jsonpath='{.spec.egressPolicyRefs}'` |
| The agent answers without running the command | The model skipped the tool | Send the prompt again. The system prompt asks for verbatim output, but the model decides |
| `enterprise-agentgateway` does not start | The license Secret is missing or has the wrong key | `kubectl get secret enterprise-agentgateway-license -n agentgateway-system -o jsonpath='{.data}' \| jq keys` lists `enterprise-agentgateway-license-key` |
| `gateway/policy-llm` never reaches `Programmed` | The GatewayClass is not accepted, or the proxy cannot pull its image | `kubectl get gatewayclass`, and `kubectl get pods -n agentgateway-system` |
| Agent turns fail after step 11 and the gateway logs nothing | The route path does not match | Keep `apiFormat: chatCompletions`. The route only accepts `/v1/chat/completions` |
| The guard policy shows `PartiallyValid` | A guard in the policy did not compile | Read the message in the policy status, and fix that guard |

## Cleanup

Removes everything this lab created. Keycloak and the rest of 030 stay.

Delete the lab's kagent objects:

```bash
kubectl delete agent policy-assistant -n kagent --ignore-not-found
kubectl delete agenttemplate policy-assistant -n kagent --ignore-not-found
kubectl delete harness kagent-policy -n kagent --ignore-not-found
kubectl delete modelconfig policy-gateway-model -n kagent --ignore-not-found
kubectl delete egresspolicy policy-demo-target -n kagent --ignore-not-found
```

Delete the gateway objects, then the controller, its CRDs, and its namespace:

```bash
kubectl delete enterpriseagentgatewaypolicy policy-llm-guard -n agentgateway-system --ignore-not-found
kubectl delete httproute policy-llm-openai -n agentgateway-system --ignore-not-found
kubectl delete enterpriseagentgatewaybackend policy-llm-openai -n agentgateway-system --ignore-not-found
kubectl delete gateway policy-llm -n agentgateway-system --ignore-not-found
kubectl delete enterpriseagentgatewayparameters policy-llm -n agentgateway-system --ignore-not-found

helm uninstall enterprise-agentgateway -n agentgateway-system --wait 2>/dev/null || true
helm uninstall enterprise-agentgateway-crds -n agentgateway-system --wait 2>/dev/null || true
kubectl delete namespace agentgateway-system policy-demo --ignore-not-found
```

The Gateway API CRDs stay. Remove them only if you installed them in step 9 and nothing else in the cluster uses them:

```bash
kubectl delete -f https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}/standard-install.yaml
```

Remove the user grants before the admin grant, so admins keep access until the end. Deleting the last policy opens ordinary access to every signed-in user again, `alice` included:

```bash
kubectl delete enterprisekagentrbacpolicy policy-lab-viewers policy-lab-writers -n kagent --ignore-not-found
kubectl delete enterprisekagentrbacpolicy policy-lab-admins -n kagent --ignore-not-found
```

Remove the Keycloak users and groups, if you are keeping Keycloak. 030's cleanup deletes the whole `keycloak` namespace, which also removes them:

```bash
for role in admin writer reader norole; do
  id=$(curl -fsS "http://127.0.0.1:8180/admin/realms/kagent/users?username=policy-$role&exact=true" \
    -H "Authorization: Bearer $(kc_admin_token)" | jq -r '.[0].id // empty')
  [ -n "$id" ] && kc_admin DELETE "/users/$id"
done

for g in admins writers readers; do
  id=$(curl -fsS "http://127.0.0.1:8180/admin/realms/kagent/groups?search=$g&exact=true" \
    -H "Authorization: Bearer $(kc_admin_token)" | jq -r --arg g "$g" '.[] | select(.name == $g) | .id')
  [ -n "$id" ] && kc_admin DELETE "/groups/$id"
done
```

Each delete prints `204`.

Stop the port-forwards on `18081` and, if you are done with 030 too, `8083`. Then clear the shell:

```bash
unset POLICY_PASSWORD AGW_VERSION GATEWAY_API_VERSION T_ADMIN T_WRITER T_READER T_NOROLE T_ALICE \
  LAST_CONTEXT WRITER_CTX EGRESS_CTX EGRESS_PROMPT
unset -f kc_admin_token kc_admin user_token refresh_tokens card_as a2a_as
```

## Next

- [030 Cleanup](030-oauth-models-mcp-agents.md#cleanup) - removes Keycloak and returns kagent to the bundled demo login
- [099 - Cleanup](099-cleanup.md) - when you are done with the workshop
