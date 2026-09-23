# Access Control

How Substrate authenticates its own calls, and how an Actor's outbound calls prove which Actor is calling. The agent process inside the sandbox is not the mTLS endpoint.

This lab applies nothing. The certificates it names are the pools created in [002](002-install.md).

## Lab Objectives

- Understand each of the three mTLS hops
- See where model and MCP credentials are injected

## Prerequisites

- Baseline setup complete: [001](001-where-it-works.md) → [002](002-install.md)

## 1. Control Plane

The atenet router calls ate-api on port 443 with a pod-identity client certificate, and ate-api checks that certificate against the pod-identity CA. ate-api uses the same kind of client certificate to connect to Postgres. Postgres is configured with `ssl = on` and `hostssl ... clientcert=verify-ca`, so a client without a certificate is rejected.

That CA is `pod-identity-ca-pool` from [002 step 4](002-install.md#4-create-the-cryptographic-pools).

## 2. Path to an Actor

A request hits the router, which selects the actor with the `ate-target-actor: <atespace>/<actor>` header. The router then dials `atunnel` on the worker pod's port 443 over mTLS. `atunnel` terminates that connection and forwards the request into the sandbox. The agent process itself sits behind `atunnel`.

This is ingress and egress to an Actor, not mTLS from actor to actor.

## 3. Path out of an Actor

Traffic leaving the sandbox is intercepted and carried to the egress gateway as an mTLS CONNECT. That certificate is the actor-identity certificate, checked against the actor-id CA, so the gateway knows which actor is making the call before it opens the outbound connection. This is also the hop where the model and MCP credentials are injected, rather than being placed inside the sandbox.

That CA is `actor-id-ca-pool`, published to `ate-api-server` as `actor-id-ca-certs` in [002](002-install.md).

## openFGA

WIP

## Cleanup

This lab creates nothing.

## Next

- [010 - Create an Agent](010-create-an-agent.md) - if you have not applied the harness yet
- [099 - Cleanup](099-cleanup.md)
