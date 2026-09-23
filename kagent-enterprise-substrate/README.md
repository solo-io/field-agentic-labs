# Kagent Enterprise Substrate Workshop

A hands-on lab series for running **Solo Enterprise for kagent** on **Agent Substrate**. Substrate is the sandbox runtime. kagent agents run there as Actors.

The workshop is built around two ideas:

1. **Two setup labs, soup to nuts.** [001](001-where-it-works.md) → [002](002-install.md) tells you whether your cluster can run Substrate, then installs Substrate and kagent Enterprise. You only run setup once.

2. **Independent unit-of-value labs.** Every lab numbered 010 and up states what it needs from the baseline. [010](010-create-an-agent.md) creates one `Harness` and one `AgentTemplate`, and has its own `## Cleanup` that removes those objects. [020](020-access-control.md) walks the mTLS path the install already uses. It applies nothing.

Every Helm value, manifest, and command is inline in the lab that uses it.

## Prerequisites

- A Kubernetes cluster at v1.36 or above, with `PodCertificateRequest`, `ClusterTrustBundle`, and projected-volume support. See [001](001-where-it-works.md) before installing. On v1.37 and above the `PodCertificateRequest` gate is on by default.
- `kubectl`, `helm` v3, `curl`, `jq`, `openssl`
- A Solo kagent license key
- An LLM provider API key. `OPENAI_API_KEY` is the workshop default. Anthropic is the documented alternative.
- (GKE snapshots) Workload Identity turned on, plus `gcloud`, so Actor snapshots can be written to Cloud Storage

# Table of Contents

- [Setup (mandatory)](#setup-mandatory)
- [Agents](#agents)
- [Access Control](#access-control)
- [Cleanup](#cleanup)

---

## Setup (mandatory)

> Two labs. Run them in order, once. After this, every lab from 010 onwards is independent.

- [001 - Where Substrate Works](001-where-it-works.md) - which clusters can enable `PodCertificateRequest`
- [002 - Install Substrate and kagent](002-install.md) - CRDs, Substrate, cryptographic pools, kagent Enterprise, UI

---

## Agents

- [010 - Create an Agent](010-create-an-agent.md) - `Harness` + `AgentTemplate` on the `kagent-default` worker pool, plus the GCS snapshot binding

---

## Access Control

- [020 - Access Control](020-access-control.md) - the three mTLS hops, and where model and MCP credentials are injected

---

## Cleanup

- [099 - Cleanup](099-cleanup.md) - tear down the baseline (010 has its own cleanup too)

---

## Tracks

Curated paths through subsets of the labs. See [`tracks/`](tracks/):

- [`install-track.md`](tracks/install-track.md) - cluster check → Substrate + kagent → one agent → access-control walkthrough

---

## Use Cases

- Decide whether a cluster can run Substrate before installing it
- Install Agent Substrate and point Solo Enterprise for kagent at it
- Bootstrap the CA and JWT pools Substrate will not create for you
- Run an agent as an Actor on the gVisor worker pool
- See how Actor traffic is authenticated, and where credentials are injected on the way out

## Validated On

| Component | Version |
|---|---|
| Kubernetes | 1.36 or newer. 1.37 and above does not need the `PodCertificateRequest` gate. |
| kagent Enterprise chart and CRDs | `1.0.0-alpha3` |
| Agent Substrate chart | `0.2.0-beta5` |
| `kubectl-ate` | `v0.2.0-beta5` |
| Worker image | `ghcr.io/kagent-dev/substrate/ateom-gvisor:v0.2.0-beta5` |

## Repo Layout

```
kagent-enterprise-substrate/
├── README.md
├── 001-where-it-works.md          # cluster compatibility
├── 002-install.md                 # Substrate + kagent Enterprise
├── 010-create-an-agent.md         # Harness + AgentTemplate
├── 020-access-control.md          # mTLS hops
├── 099-cleanup.md                 # full teardown
└── tracks/
    └── install-track.md
```
