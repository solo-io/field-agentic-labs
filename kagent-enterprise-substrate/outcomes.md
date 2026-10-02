# What these labs show

This page is separate from the labs. The numbered labs stay the steps. Read **The story** if you own the product or the engineering and you need the claim these labs are making. The sections after it are the scope, the user story for each lab, and what a walkthrough can show.

[tracks/install-track.md](tracks/install-track.md) is the five labs in order.

## Evaluation Matrix

Solo Enterprise for kagent does not run an agent as a Deployment. It hands the agent to Agent Substrate. Substrate keeps a small pool of warm worker pods. Each pod runs one Actor, the agent process inside a gVisor sandbox. When that Actor is idle, Substrate snapshots the process and frees the pod. The next message restores the snapshot onto a free worker, often a different one. Kubernetes scheduled the pod. Substrate scheduled the agent. That split is the workshop.

The labs follow the order a customer hits that claim.

**The cluster comes first.** Substrate does not ship TLS secrets. Each of its pods asks Kubernetes for a short-lived certificate, and that requires `PodCertificateRequest` on the API server. Lab 001 exists so a platform that cannot serve the gate stops before an install. A failed install on EKS 1.36 is a control-plane gap, not a broken chart. Kubernetes 1.37 and newer leave the gate on.

**The two control planes stay separate.** Lab 002 installs Substrate as its own release, named `substrate`, in `ate-system`. kagent does not embed it. kagent dials ate-api and the router, and it creates the `WorkerPool` the Actors will use. The Substrate chart also refuses to create the CA and JWT pools. Pods that stay unready until an operator creates those pools are the intended state: the trust material belongs to the operator, and the chart will not invent it.

**An agent is two objects, and neither of them is a conversation.** Lab 010 adds a `Harness` and an `AgentTemplate`. The harness names the image, the worker pool, and the snapshot location. The template names the model and the prompt, and it is the only template that harness may run. Substrate boots that template once and stores a golden snapshot when the template becomes ready, before anyone sends a message. Every later session starts from that snapshot. The lab stops at those objects.

Two later snapshots are easy to brief as if this lab included them. An idle snapshot is the resume state of one session, and the next suspend replaces it. A checkpoint is an explicit freeze of one finished conversation, kept so a fork can start from it. This workshop shows the golden snapshot, and it shows that a request can wake an Actor from a snapshot. It does not show a chat, a checkpoint, or a fork.

**The process in the sandbox is not the security principal.** Lab 020 is the claim that follows from the split. On the way in, the router authenticates to `atunnel` on the worker, and `atunnel` forwards into the sandbox. On the way out, the Actor's traffic reaches the egress gateway under the Actor's own certificate, and that gateway attaches the model and MCP credentials. The key used to call the model is injected outside the process that makes the call. The lab does not show who is allowed to talk to the agent. That authorization story is unfinished.

**Teardown follows the same split.** Lab 099 removes the harness before the CRDs. The harness is a custom object. Removing the CRD release first leaves it behind with nothing to reconcile it. The order is the install boundary run backwards.

Anything past that list is outside the story these labs can defend. A live conversation, checkpoints and forks, microVM isolation, an external identity provider, and an authorization policy are real product surfaces, and none of them is what a person has seen by finishing this workshop.

## What's covered

| In this workshop | Lab |
|---|---|
| Whether a cluster can serve `PodCertificateRequest`, and which providers can turn that gate on | [001](001-where-it-works.md) |
| Substrate `0.2.0-beta5` as its own release in `ate-system`, plus the five CA and JWT pools the chart does not create | [002](002-install.md) |
| kagent Enterprise `1.0.0-alpha3` pointed at ate-api and the atenet router | [002](002-install.md) |
| A gVisor `WorkerPool` named `kagent-default`. One running Actor per worker. An idle Actor is a snapshot | [002](002-install.md) |
| One `Harness` and one `AgentTemplate` on that pool, the golden snapshot, Actor naming, and where snapshots are stored | [010](010-create-an-agent.md) |
| The three mTLS hops, and model and MCP credential injection at the egress gateway | [020](020-access-control.md) |
| Teardown order so a harness is not left behind after its CRDs are gone | [099](099-cleanup.md) |

Not in this workshop:

- A chat, a checkpoint, or a fork. Lab 010 applies the two objects and stops. A traced turn with a tool call is the image under `assets/agentdemo/adktraceenabled/`.
- microVM. The installed pool is gVisor.
- Authorization. The openFGA section of lab 020 is unfinished, and the lab applies nothing.
- Your own IdP, beyond the optional OIDC values in lab 002. The install uses the built-in demo IdP.
- Agent evals, beyond an optional values snippet in lab 002.
- Moving existing kagent v0 objects to v1. That note is `prereqs.md`, and it is not a lab.

## User stories

| Lab | User story |
|---|---|
| [001](001-where-it-works.md) | As a platform owner, I want to know if this cluster can issue pod certificates, so I do not start an install that cannot become Ready. |
| [002](002-install.md) | As an operator, I want Substrate and kagent Enterprise installed and the UI open, so later labs start from a working baseline. |
| [010](010-create-an-agent.md) | As someone with an agent image, I want that image admitted on the worker pool, so a session can start from its golden snapshot. |
| [020](020-access-control.md) | As a security reviewer, I want the path a request takes and the place the model key is injected, so I can see that the key is not inside the sandbox. |
| [099](099-cleanup.md) | As that operator, I want the cluster returned to empty, so the harness, the releases, and the identity pools are all gone. |

## What you show

### 001 — Where Substrate works

**You show.** `PodCertificateRequest` is an API-server feature, not a kubelet flag. GKE and a cluster whose API server you administer can enable it. Kubernetes 1.37 and newer leave it on. EKS on 1.36 cannot enable it, because the managed control plane does not expose the gate. AKS depends on its 1.37 release.

**You do not show.** Substrate, kagent, or any certificate. The cluster is unchanged.

### 002 — Install Substrate and kagent

**You show.**

- Substrate is its own Helm release, named `substrate`, in `ate-system`. Kubernetes owns the worker pods. Substrate owns Actor scheduling, snapshots, and routing.
- A worker hosts at most one running Actor. An idle Actor is a snapshot, and the next request resumes it onto a free worker.
- The chart does not create its certificate and JWT pools. The pods stay unready until the five pools, the actor-id CA, and `ate-api-authentication` exist.
- kagent Enterprise dials `ate-api` and the atenet router, and creates the gVisor pool `kagent-default`. The Substrate subchart inside kagent stays off.
- The UI is open at the port-forward. Agent traces reach the collector only when the OTLP endpoint keeps its `http://` prefix.

**You do not show.** An agent, a conversation, or a checkpoint. Those start at 010, and a conversation is a later step in the UI.

### 010 — Create an agent

**You show.**

- Three objects, and which one this lab creates. `ModelConfig` is the LLM, already installed. `Harness` is where the agent runs. `AgentTemplate` is the definition the harness is allowed to run.
- The harness is pinned to `kagent-default` and to a snapshot location. A new session starts from the golden snapshot Substrate takes when that template is ready, before anyone sends a message.
- An Actor's identity is its atespace plus its name. In kagent the atespace is the namespace, and the name is `ai-` plus the lowercased `AgentInstance` id.
- A request names the Actor with `ate-target-actor`. A suspended Actor is restored onto a free worker. The access log then says `triggered`, `none`, or `joined`.
- Snapshots land in the bucket from the Substrate install. The `gs://` location in the manifest does not switch the backend to Cloud Storage. That switch is `atelet.storageBackend=gcs`, plus the IAM binding for the `ate-api-server` service account.

**You do not show.** A chat, a checkpoint, or a fork. A traced turn, including a tool call, is the image under `assets/agentdemo/adktraceenabled/`.

### 020 — Access control

**You show.**

- Control plane. The router calls ate-api with a pod-identity certificate. ate-api checks it against `pod-identity-ca-pool`. ate-api uses the same kind of certificate to reach Postgres.
- Path to an Actor. The router dials `atunnel` on the worker over mTLS. `atunnel` terminates that connection and forwards into the sandbox. The agent process is not the mTLS endpoint.
- Path out of an Actor. Traffic leaving the sandbox is an mTLS CONNECT to the egress gateway, authenticated with the actor-identity certificate from `actor-id-ca-pool`. The gateway injects the model and MCP credentials on that hop. They are not placed in the sandbox.

**You do not show.** An authorization policy. The openFGA section of the lab is still unfinished, and this lab applies nothing.
