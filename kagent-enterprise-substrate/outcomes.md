# Solo Enterprise For Kagent + Agent Substrate

## Overview

Teams want to run more agents without operating a dedicated runtime for every idle conversation. They also need to understand each agent's behavior and control where its credentials live.

Solo Enterprise for kagent manages the agents and their configuration. Agent Substrate runs them as sandboxed actors on a shared worker pool. Substrate saves an idle actor's state and releases its execution slot. A later request resumes that actor on an available worker.

The story moves from platform fit to a working agent, then follows the agent's execution and credentials. Finish by showing how the team can remove the evaluation resources.

Use this guide as an engineer presenting the platform, a technical product manager explaining its value, or a customer champion taking the story internally. Complete setup before the call, then choose the jobs that answer the audience's questions.

## Prepare the demonstration

The numbered labs supply the installation and configuration steps. This plan supplies the customer questions, demonstrations, and expected results.

- Agree on the customer's priorities and select the jobs that address them. Establish platform fit before running the agent demonstrations.
- Prepare the [trace-enabled Go ADK demo](assets/agentdemo/adktraceenabled/README.md) for Jobs 1, 3, and 4. The repository supplies the agent code and tool; the presenter must build or supply a pullable image and configure the model.
- Rehearse a model-and-tool turn and locate its trace before the call. The customer does not need to write an agent during the evaluation.

Jobs 1 and 4 reuse a working agent. Job 3 requires a prepared agent from Job 2 or the demo setup. Run teardown only when the evaluation is finished. Record each numbered demonstration as observed, explained, blocked, or not selected, so an architectural walkthrough does not become a claimed test result.

## Story map

| The audience asks | The job | Evidence to show |
|---|---|---|
| Can we run this in our environment? | Establish platform fit | Supported cluster capabilities, healthy components, and a working login |
| Do we need a dedicated worker for every agent? | Share runtime capacity | A shared worker pool and the actor lifecycle |
| What happens before the first conversation? | Prepare the agent | A published agent with a ready golden snapshot |
| Can we see it do useful work and explain the result? | Follow one real turn | A tool-assisted answer and its execution trace |
| Where does the model key live? | Separate credentials from execution | A Secret reference and the gateway's credential-injection path |
| Can we remove the evaluation? | Close out the environment | Agent-level cleanup and platform teardown |

## Evaluation matrix

Use this matrix to connect the customer's requirements to specific demonstrations. Optional items apply when the customer selects them.

| Category | Requirement | Demonstrated in |
|---|---|---|
| Platform fit | The customer's Kubernetes can host the runtime | P.1 |
| Installation | Platform components become healthy and the UI opens | P.2, P.3 |
| Identity | Users can sign in through the customer's identity provider | P.4 (optional) |
| Model connectivity | A real model request succeeds through the installed platform | P.5, 3.1 |
| Runtime | Agents use a shared worker pool | 1.1, 2.1 |
| Runtime | An idle actor suspends and resumes for subsequent work | 1.2–1.4 |
| Agent lifecycle | The platform prepares a golden snapshot before the first conversation | 2.2, 2.3 |
| Agent experience | A conversation completes a model-and-tool task | 3.1 |
| Observability | The presenter can locate and explain a turn's execution trace | 3.2–3.5 |
| Credential isolation | The platform resolves model credentials outside the sandbox | 4.1–4.3 |
| Operations | Individual agents and the workshop installation have a defined cleanup path | 5.1–5.3 |

## Prerequisite: Establish platform fit

### User story

When evaluating a new runtime in our existing infrastructure, I want to establish compatibility and working access before asking application teams to adopt it.

### What needs to be done

- Complete [001: Where Substrate Works](001-where-it-works.md) to check the cluster's certificate capabilities.
- Complete [002: Install Substrate and kagent](002-install.md), including identity material, model credentials, and telemetry.
- Confirm the platform is healthy and the UI opens. Configure the customer's identity provider when their login flow is part of the evaluation.

### Demonstrations and expected results

| # | Demonstration | Expected result |
|---|---|---|
| P.1 | Check the target cluster's runtime prerequisites | The presenter establishes whether the cluster exposes the required certificate APIs and projections before installing. |
| P.2 | Install the workshop baseline | kagent, Substrate, and the worker pool become healthy after the required trust material is configured. |
| P.3 | Open the management UI and sign in | The interface is reachable and shows the signed-in user. The presenter identifies the login provider used. |
| P.4 | Sign in through the customer's identity provider (optional) | A customer user completes the configured OIDC flow and obtains access to the management interface. |
| P.5 | Run a smoke-test turn with the prepared demo agent | The agent receives a model response through the runtime and credential gateway. Ready pods alone do not satisfy this check. |

### The story to tell

“We start with your existing Kubernetes environment. Kubernetes hosts the workers, while Substrate manages actor scheduling, snapshots, and routing. We establish platform compatibility before asking your team to invest in the agent demonstration.”

### What this proves

The environment can host the workshop baseline. A successful customer-IdP login also demonstrates that configured sign-in flow. Login alone does not demonstrate agent-level authorization.

### What to watch for

Certificate APIs and projection support determine platform fit for this workshop. Complete the cluster check before installation. Keep the controller, Substrate, and worker image on compatible versions, and verify them with a real model request.

Prepare registry access, model credentials, and the snapshot backend before the call. If customer login is in scope, rehearse their OIDC flow rather than substituting the demo login.

## Job 1: Share runtime capacity across agents

### User story

When agent usage is intermittent, I want idle agents to release execution capacity so we can share workers instead of provisioning a dedicated worker for each agent.

### What needs to be done

- Prepare the worker pool from [002](002-install.md) and a working agent from [010](010-create-an-agent.md) or the [demo agent](assets/agentdemo/adktraceenabled/README.md).
- Identify the worker pool and its ready capacity.
- For a live lifecycle demonstration, observe an actor running, becoming suspended, and resuming on a subsequent request.

### Demonstrations and expected results

| # | Demonstration | Expected result |
|---|---|---|
| 1.1 | Show `kagent-default` and the agent's worker-pool reference | The agent uses the shared pool. Publishing an agent does not require a dedicated worker Deployment for that agent. |
| 1.2 | Invoke the agent and observe its actor running | Substrate assigns the actor to an available worker while it executes the request. |
| 1.3 | Observe the actor after its idle lifecycle completes | The actor becomes suspended and releases its execution slot. The shared worker remains available. |
| 1.4 | Send another request to the same agent instance | Substrate resumes the actor and the agent responds. The [request-path explanation in 010](010-create-an-agent.md#how-a-request-reaches-an-actor) identifies the resume indicators. |

### The story to tell

“An agent is not a dedicated worker. Substrate assigns a sandboxed actor to an available worker when work arrives. When the actor becomes idle, Substrate saves its state and releases the slot. The worker stays available for another actor.”

### What this proves

The pool demonstrates the shared-capacity architecture. Observed suspension and resume demonstrate capacity reuse for that actor. Density limits and cost savings require a separate workload measurement.

### What to watch for

An idle worker pod is still provisioned capacity. The efficiency story concerns sharing execution slots across actors, not scaling every worker to zero.

Rehearse the actor's suspension timing. A pause between model or tool calls does not establish that the actor has suspended. If you only explain the lifecycle, mark 1.2–1.4 as explained rather than observed.

## Job 2: Prepare the agent before the first message

### User story

When publishing an agent for users, I want the platform to initialize it before the first conversation so users start from a prepared runtime.

### What needs to be done

- Publish a `Harness` and an `AgentTemplate` using [010](010-create-an-agent.md).
- Provide a pullable runtime image, a working model configuration, and a writable snapshot backend.
- Wait for the agent's `Ready` condition to report that its golden snapshot is ready before starting a conversation.

### Demonstrations and expected results

| # | Demonstration | Expected result |
|---|---|---|
| 2.1 | Publish an agent and its harness using the supplied manifests | The harness admits the selected AgentTemplate and references the shared worker pool and runtime image. |
| 2.2 | Observe preparation before opening a conversation | The AgentTemplate's `Ready` condition reports that its golden snapshot is ready. |
| 2.3 | Start a new conversation from the prepared agent | The platform creates an instance from the prepared runtime and the agent accepts a turn. Pair this with 3.1 when using the traced demo. |

### The story to tell

“The developer publishes what the agent does and which harness runs it. The platform initializes that runtime and prepares a golden snapshot. Each new conversation starts from that prepared state, rather than repeating the initial preparation.”

### What this proves

The platform prepared the agent before its first conversation. This demonstrates readiness, not a measured startup-latency target. Golden snapshots, idle suspension snapshots, and named conversation checkpoints serve different purposes; this job demonstrates the first.

### What to watch for

An agent appearing in the UI does not establish readiness. Preparation depends on image pulls, runtime initialization, and snapshot storage.

The image placeholders in the supplied manifests need a real pullable image. A snapshot URI alone does not select the storage backend; follow the backend configuration in [010](010-create-an-agent.md).

## Job 3: Follow one real turn from request to result

### User story

When an agent returns a result, I want to follow its model and tool calls so I can explain the result and investigate unexpected behavior.

### What needs to be done

- Deploy the [trace-enabled Go ADK demo](assets/agentdemo/adktraceenabled/README.md) and confirm its golden snapshot is ready.
- Confirm the model credential works and the telemetry pipeline receives agent traces.
- Start a conversation and ask: **“Add 17 and 25 using add_numbers.”**

### Demonstrations and expected results

| # | Demonstration | Expected result |
|---|---|---|
| 3.1 | Ask the demo agent to add 17 and 25 using `add_numbers` | The agent completes the turn and returns **42**. |
| 3.2 | Find the trace corresponding to that turn | The presenter locates the trace using the agent identity and the request's time window. |
| 3.3 | Follow the execution sequence | Distinct spans show the agent invocation, model calls, and `add_numbers` execution. |
| 3.4 | Explain the answer using the trace | The presenter connects the tool execution to the response, establishing that the agent used the tool. |
| 3.5 | Inspect the timing of model and tool spans | Span durations show where this turn spent time. The presenter can distinguish model-call latency from tool-execution latency. |

### The story to tell

“This is a real request through the platform. The agent uses a model to choose a tool, the tool performs the calculation, and the agent returns the result. The trace lets us follow those steps instead of treating the agent as a black box.”

### What this proves

The demonstrated agent can complete a model-and-tool turn, and the platform exposes its emitted execution spans. Prompt and response capture depends on the configured telemetry settings. This demo uses an in-memory session service; it does not establish durable conversation recovery across actor replacement.

### What to watch for

A correct answer does not prove that the agent called the requested tool. Confirm its execution in the trace. If the turn succeeds but no trace appears, check runtime emission and the telemetry pipeline using the [demo troubleshooting steps](assets/agentdemo/adktraceenabled/README.md#quickstart).

Ask the customer which behavior they need to explain or audit. A tool sequence or latency question gives the trace a purpose beyond showing that telemetry exists.

## Job 4: Separate credentials from agent execution

### User story

When agents need access to a model, I want the platform to supply credentials outside the sandbox so the agent process can make authorized calls without holding the real key.

### What needs to be done

- Use the Secret-backed model configuration and credential-provider namespace grant from [002](002-install.md).
- Follow the ingress and egress paths described in [020: Access Control](020-access-control.md).
- Pair the configuration walkthrough with the successful model call from Job 3.

### Demonstrations and expected results

| # | Demonstration | Expected result |
|---|---|---|
| 4.1 | Inspect the model's credential reference and the compiled runtime configuration | The ModelConfig references a Kubernetes Secret. The compiled model credential is a placeholder rather than the Secret value. |
| 4.2 | Walk the outbound request path and namespace grant | The presenter identifies actor authentication, permitted credential lookup, and destination-scoped header injection at the egress gateway. |
| 4.3 | Correlate a successful model turn with credential-provider activity | The agent receives a model response, and provider logs show resolution of the referenced Secret for the actor without displaying its value. |

### The story to tell

“The model key stays in a Kubernetes Secret. The agent runs with a placeholder, and the egress gateway resolves the permitted Secret and injects the real key into the outbound request. Actor identity and the namespace grant control that credential lookup.”

### What this proves

The walkthrough explains credential separation, supported by a successful model request. The same injection mechanism supports Secret-backed MCP headers, but this demo does not exercise an MCP call. End-user authorization to invoke an agent requires its own demonstration.

### What to watch for

Prepare a view of the credential references and placeholder without exposing Secret values. Gateway credential caching can suppress a new provider lookup, so rehearse the log correlation before the call.

Separate two customer questions: which credentials an actor can resolve, and which users can invoke that agent. This job addresses the first through actor identity and namespace grants.

## Job 5: Close out the evaluation environment

### User story

When the evaluation ends, I want to remove its agents, platform resources, and added access grants so my team can close out the POC deliberately.

### What needs to be done

- Remove the agent resources using [010's cleanup](010-create-an-agent.md#cleanup). Use the [demo's cleanup](assets/agentdemo/adktraceenabled/README.md#project-structure) if you deployed `adk-trace-enabled`.
- Remove optional cloud-storage IAM grants when the evaluation added them.
- Follow [099: Cleanup](099-cleanup.md) to remove the platform releases, workshop namespaces, trust material, and local setup files.

### Demonstrations and expected results

| # | Demonstration | Expected result |
|---|---|---|
| 5.1 | Remove the evaluation agent and its harness | The selected agent resources disappear while the shared platform remains available. |
| 5.2 | Remove the workshop platform using the documented teardown | The Helm releases, workshop namespaces, and namespace-local identity material are removed. |
| 5.3 | Review access grants and external artifacts created for the evaluation | Added cloud-storage IAM grants are removed when applicable. The team records whether snapshot data and pushed images are retained or removed. |

### The story to tell

“The evaluation has a defined exit path. We can remove an individual agent while keeping the platform for other work. When the evaluation ends, we remove the workshop installation and the access grants we added.”

### What this proves

The documented cleanup removes the identified workshop resources. Track external snapshot data and pushed images separately; removing Kubernetes resources does not establish that those external artifacts are gone.

### What to watch for

Remove agent resources before their controllers and CRDs. Agree on the teardown window with the environment owner, and distinguish workshop-owned resources from shared infrastructure.

If the customer wants to keep evaluating, explain the exit plan on the call and record the teardown demonstrations as not selected.

## Close the story

“We established that the platform fits your environment, prepared an agent on shared capacity, and followed a real task through its trace. We explained how the platform supplies credentials outside the sandbox and how to close out the evaluation.”

Close with the jobs you actually demonstrated. Agree on the next evaluation against the customer's workload, whether that means capacity measurements, durable conversation behavior, or agent-level authorization.
