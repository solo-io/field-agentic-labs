# Solo Enterprise for kagent on Agent Substrate — Demo Story

## The story

Teams want to run more agents without operating a dedicated runtime for every idle conversation. They also need to understand each agent's behavior and control where its credentials live.

Solo Enterprise for kagent manages the agents and their configuration. Agent Substrate runs them as sandboxed actors on a shared worker pool. Substrate saves an idle actor's state and releases its execution slot. A later request resumes that actor on an available worker.

The story moves from platform fit to a working agent, then follows the agent's execution and credentials. Finish by showing how the team can remove the evaluation resources.

Use this guide as an engineer presenting the platform, a technical product manager explaining its value, or a customer champion taking the story internally. Complete setup before the call, then choose the jobs that answer the audience's questions.

## Story map

| The audience asks | The job | Evidence to show |
|---|---|---|
| Can we run this in our environment? | Establish platform fit | Supported cluster capabilities, healthy components, and a working login |
| Do we need a dedicated worker for every agent? | Share runtime capacity | A shared worker pool and the actor lifecycle |
| What happens before the first conversation? | Prepare the agent | A published agent with a ready golden snapshot |
| Can we see it do useful work and explain the result? | Follow one real turn | A tool-assisted answer and its execution trace |
| Where does the model key live? | Separate credentials from execution | A Secret reference and the gateway's credential-injection path |
| Can we remove the evaluation? | Close out the environment | Agent-level cleanup and platform teardown |

## Prerequisite: Establish platform fit

### User story

As a platform engineer, I want to establish that our Kubernetes environment can host the runtime before committing to an evaluation.

### What needs to be done

- Complete [001: Where Substrate Works](001-where-it-works.md) to check the cluster's certificate capabilities.
- Complete [002: Install Substrate and kagent](002-install.md), including identity material, model credentials, and telemetry.
- Confirm the platform is healthy and the UI opens. Configure the customer's identity provider when their login flow is part of the evaluation.

### What to show on the call

Show the target environment, the healthy platform, and the signed-in UI. Identify whether the evaluation uses the built-in demo login or the customer's identity provider.

### The story to tell

“We start with your existing Kubernetes environment. Kubernetes hosts the workers, while Substrate manages actor scheduling, snapshots, and routing. We establish platform compatibility before asking your team to invest in the agent demonstration.”

### What this proves

The environment can host the workshop baseline. A successful customer-IdP login also demonstrates that configured sign-in flow. Login alone does not demonstrate agent-level authorization.

## Job 1: Share runtime capacity across agents

### User story

As a platform engineer, I want idle agents to release execution capacity so I can support agents without assigning a dedicated worker to each.

### What needs to be done

- Prepare the worker pool from [002](002-install.md) and a working agent from [010](010-create-an-agent.md) or the [demo agent](assets/agentdemo/adktraceenabled/README.md).
- Identify the worker pool and its ready capacity.
- For a live lifecycle demonstration, observe an actor running, becoming suspended, and resuming on a subsequent request.

### What to show on the call

Show `kagent-default` as shared runtime capacity. If you observe suspension and resume, show those transitions alongside the agent request. Otherwise, use the [request-path explanation in 010](010-create-an-agent.md#how-a-request-reaches-an-actor) to explain the lifecycle.

### The story to tell

“An agent is not a dedicated worker. Substrate assigns a sandboxed actor to an available worker when work arrives. When the actor becomes idle, Substrate saves its state and releases the slot. The worker stays available for another actor.”

### What this proves

The pool demonstrates the shared-capacity architecture. Observed suspension and resume demonstrate capacity reuse for that actor. Density limits and cost savings require a separate workload measurement.

## Job 2: Prepare the agent before the first message

### User story

As an agent developer, I want to prepare my agent before users start conversations so each new instance starts from an initialized runtime.

### What needs to be done

- Publish a `Harness` and an `AgentTemplate` using [010](010-create-an-agent.md).
- Provide a pullable runtime image, a working model configuration, and a writable snapshot backend.
- Wait for the agent's `Ready` condition to report that its golden snapshot is ready before starting a conversation.

### What to show on the call

Show the agent definition, its relationship to the shared pool, and the ready golden snapshot. Connect that prepared runtime to the new conversation in Job 3.

### The story to tell

“The developer publishes what the agent does and which harness runs it. The platform initializes that runtime and prepares a golden snapshot. Each new conversation starts from that prepared state, rather than repeating the initial preparation.”

### What this proves

The platform prepared the agent before its first conversation. This demonstrates readiness, not a measured startup-latency target. Golden snapshots, idle suspension snapshots, and named conversation checkpoints serve different purposes; this job demonstrates the first.

## Job 3: Follow one real turn from request to result

### User story

As an application owner, I want to see an agent complete a task and inspect its execution so I can explain how it reached the result.

### What needs to be done

- Deploy the [trace-enabled Go ADK demo](assets/agentdemo/adktraceenabled/README.md) and confirm its golden snapshot is ready.
- Confirm the model credential works and the telemetry pipeline receives agent traces.
- Start a conversation and ask: **“Add 17 and 25 using add_numbers.”**

### What to show on the call

Show the answer, **42**, then open the corresponding trace. Identify the agent invocation, model calls, and `add_numbers` tool execution. Use the tool span to establish that the agent called the tool, rather than relying on the answer alone.

### The story to tell

“This is a real request through the platform. The agent uses a model to choose a tool, the tool performs the calculation, and the agent returns the result. The trace lets us follow those steps instead of treating the agent as a black box.”

### What this proves

The demonstrated agent can complete a model-and-tool turn, and the platform exposes its emitted execution spans. Prompt and response capture depends on the configured telemetry settings. This demo uses an in-memory session service; it does not establish durable conversation recovery across actor replacement.

## Job 4: Separate credentials from agent execution

### User story

As a security engineer, I want the platform to supply model credentials outside the agent sandbox so the agent can make authorized calls without holding the real key.

### What needs to be done

- Use the Secret-backed model configuration and credential-provider namespace grant from [002](002-install.md).
- Follow the ingress and egress paths described in [020: Access Control](020-access-control.md).
- Pair the configuration walkthrough with the successful model call from Job 3.

### What to show on the call

Show the model's Secret reference without displaying its value. Explain where actor identity is authenticated and where the egress gateway adds the destination-scoped credential. Distinguish the agent process inside the sandbox from the components handling mTLS.

### The story to tell

“The model key stays in a Kubernetes Secret. The agent runs with a placeholder, and the egress gateway resolves the permitted Secret and injects the real key into the outbound request. Actor identity and the namespace grant control that credential lookup.”

### What this proves

The walkthrough explains credential separation, supported by a successful model request. The same injection mechanism supports Secret-backed MCP headers, but this demo does not exercise an MCP call. End-user authorization to invoke an agent requires its own demonstration.

## Job 5: Close out the evaluation environment

### User story

As the environment owner, I want a defined cleanup path so I can remove the evaluation resources when the POC ends.

### What needs to be done

- Remove the agent resources using [010's cleanup](010-create-an-agent.md#cleanup). Use the [demo's cleanup](assets/agentdemo/adktraceenabled/README.md#project-structure) if you deployed `adk-trace-enabled`.
- Remove optional cloud-storage IAM grants when the evaluation added them.
- Follow [099: Cleanup](099-cleanup.md) to remove the platform releases, workshop namespaces, trust material, and local setup files.

### What to show on the call

Explain the cleanup plan before teardown. When ending the evaluation, show that removing an agent leaves the shared platform available. Then confirm removal of the workshop platform resources.

### The story to tell

“The evaluation has a defined exit path. We can remove an individual agent while keeping the platform for other work. When the evaluation ends, we remove the workshop installation and the access grants we added.”

### What this proves

The documented cleanup removes the identified workshop resources. Track external snapshot data and pushed images separately; removing Kubernetes resources does not establish that those external artifacts are gone.

## Close the story

“We established that the platform fits your environment, prepared an agent on shared capacity, and followed a real task through its trace. We explained how the platform supplies credentials outside the sandbox and how to close out the evaluation.”

Close with the jobs you actually demonstrated. Agree on the next evaluation against the customer's workload, whether that means capacity measurements, durable conversation behavior, or agent-level authorization.
