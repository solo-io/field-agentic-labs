# Agent Substrate Workshop

A hands-on lab series for **[Agent Substrate](https://github.com/agent-substrate/substrate)** — the Kubernetes-native runtime that multiplexes many idle "actors" (agent-like workloads) onto a small pool of warm "worker" pods. The labs walk through standing it up on GKE, deploying the four demo actors (counter, sandbox, agent-secret, claude-code-multiplex), and integrating it with **kagent** so kagent `AgentHarness` resources can run on top of Substrate.

This workshop **links out to the upstream repo** rather than mirroring it. You will `git clone https://github.com/agent-substrate/substrate` early on, then every lab references paths inside that clone (`hack/install-ate.sh`, `demos/counter/`, `cmd/kubectl-ate/`, etc.). The only assets checked in here are a small number of lab-specific YAML files and the two screenshots referenced from the kagent install lab.

## Prerequisites

Before starting this workshop, you will need:

- A **GKE Standard** cluster (Autopilot is unsupported — see [appendix-why-gke](appendix-why-gke.md))
- A GCP project with billing enabled and the relevant APIs on
- Go ≥ 1.26.3 (matches the Substrate `go.mod` toolchain)
- `gcloud`, `kubectl`, `helm`, `git`, `openssl`, `curl`
- `ko` (Substrate's image builder — installed via Go)
- An LLM API key (Anthropic for most labs; the kagent integration also accepts OpenAI/Gemini)
- For [060 kagent integration](060-install-kagent-with-substrate.md) onwards: a Helm v3 install

# Table of Contents

- [Foundations](#foundations)
- [GCP Cluster Setup](#gcp-cluster-setup)
- [Install Substrate](#install-substrate)
- [Substrate Demos](#substrate-demos)
- [Kagent Integration](#kagent-integration)
- [Operations](#operations)
- [Observability](#observability)
- [Cleanup](#cleanup)
- [Appendix](#appendix)

---

## Foundations

> **Start here.** Read the overview and clone the source.

- [000 — Overview & Architecture](000-overview.md)
- [001 — Clone the Upstream Substrate Repo](001-clone-upstream.md)

---

## GCP Cluster Setup

- [010 — GKE Cluster Prerequisites (Standard, beta APIs, Workload Identity)](010-gke-cluster-prereqs.md)
- [020 — Configure Your Environment (`ate-dev-env.sh`)](020-configure-env.md)
- [030 — GCP IAM, Snapshot Bucket, and `kubectl` Context](030-gcp-iam-and-bucket.md)

---

## Install Substrate

- [040 — Install Agent Substrate (Helm OCI)](040-install-substrate-helm.md)
- [045 — Install the `kubectl-ate` CLI](045-install-kubectl-ate.md)

---

## Substrate Demos

> Run any of these once Substrate is installed and `kubectl-ate` is on your `PATH`.

- [050 — Counter Demo (stateful HTTP, suspend/resume)](050-counter-demo.md)
- [051 — Sandbox Demo (Alpine shell + REPL client)](051-sandbox-demo.md)
- [052 — Agent-Secret Demo (Zero-Idle self-suspend + RAM persistence)](052-agent-secret-demo.md)
- [053 — Claude Code Multiplex (3 agents on 2 pods)](053-claude-code-multiplex.md) — **upstream DRAFT, includes workarounds**

---

## Kagent Integration

> The kagent integration is independent of Substrate's own demos — it gives you a way to run kagent `AgentHarness` resources on top of Substrate workers.

- [060 — Install kagent with Substrate Enabled](060-install-kagent-with-substrate.md)
- [070 — Substrate-Backed `AgentHarness` Walkthrough](070-kagent-agentharness.md)

---

## Operations

- [080 — Suspend / Resume Operations](080-operations-suspend-resume.md)

---

## Observability

- [090 — Logs, Metrics, and Tracing](090-observability.md)

---

## Cleanup

- [099 — Cleanup & Common Troubleshooting](099-cleanup.md)

---

## Appendix

- [Appendix — Why GKE (the Pod Certificate requirement)](appendix-why-gke.md)
- [Appendix — Local `kind` Quickstart](appendix-kind-quickstart.md) — for laptop dev without GCP
- [Appendix — Install Substrate from Source (`install-ate.sh`)](appendix-install-script-alternative.md) — the alternative to the Helm path in [040](040-install-substrate-helm.md)
- [Appendix — Benchmarking with Locust](appendix-benchmarking.md)

---

## Tracks

Curated learning paths under [`tracks/`](tracks/):

- [`install-track.md`](tracks/install-track.md) — Cluster → Substrate install → counter demo
- [`demos-track.md`](tracks/demos-track.md) — All four demos in sequence
- [`kagent-track.md`](tracks/kagent-track.md) — Install Substrate → install kagent with Substrate → first `AgentHarness`

---

## Use Cases

- Multiplex many stateful agent sessions onto a small pool of warm worker pods
- Preserve in-memory + disk state across **suspend → snapshot → resume** cycles, even when the actor lands on a different worker
- Use gVisor sandboxes (`runsc`) to checkpoint and restore real process state, not just container restarts
- Run framework-agnostic agents — ADK, LangChain, Claude Code, MCP servers — as Substrate Actors
- Stand up Substrate-backed `AgentHarness` resources from kagent, with kagent generating one `ActorTemplate` per harness
- Operate suspended actors via `kubectl-ate` and the `ateapi.Control` gRPC API (`ResumeActor`, `GetActor`, `SuspendActor`)

## Validated Versions

| Component | Version | Used in |
|-----------|---------|---------|
| Go | ≥ 1.26.3 | [010](010-gke-cluster-prereqs.md) |
| Substrate Helm charts (`oci://ghcr.io/kagent-dev/substrate/helm/{substrate,substrate-crds}`) | floating tags (recommend pinning) | [040](040-install-substrate-helm.md) |
| `ateom-gvisor` image (kagent integration) | `ghcr.io/kagent-dev/substrate/ateom-gvisor:v0.0.6` | [060](060-install-kagent-with-substrate.md) |
| kagent Helm chart | `0.9.7` | [060](060-install-kagent-with-substrate.md) |
| `runsc` (gVisor) | nightly with `--allow-connected-on-save` (pinned per demo template) | [040](040-install-substrate-helm.md), [050](050-counter-demo.md) |
| Google Cloud SDK | verified at `484.0.0` | [030](030-gcp-iam-and-bucket.md) |
| GKE cluster / node-pool | `1.35.0-gke.2398000` (example default) | [010](010-gke-cluster-prereqs.md) |
| Node machine type | `c3-standard-4` (any 4-vCPU works) | [010](010-gke-cluster-prereqs.md) |
| Valkey / Redis (state store) | `valkey/valkey:8.0` | bundled by [040](040-install-substrate-helm.md) |
| Envoy (router) | `envoyproxy/envoy:v1.30-latest` | bundled by [040](040-install-substrate-helm.md) |

> Substrate is in **VERY early development**. Per the upstream README, APIs are almost guaranteed to change and there are no backward-compatibility guarantees. Treat this workshop as a snapshot.

## Repo Layout

```
agent-substrate/
├── README.md                                # this file
├── 000-overview.md
├── 001-clone-upstream.md
├── 010-gke-cluster-prereqs.md
├── 020-configure-env.md
├── 030-gcp-iam-and-bucket.md
├── 040-install-substrate-helm.md            # CANONICAL install (Helm OCI)
├── 045-install-kubectl-ate.md
├── 050-counter-demo.md                      # first demo
├── 051-sandbox-demo.md
├── 052-agent-secret-demo.md
├── 053-claude-code-multiplex.md             # DRAFT upstream
├── 060-install-kagent-with-substrate.md
├── 070-kagent-agentharness.md
├── 080-operations-suspend-resume.md
├── 090-observability.md
├── 099-cleanup.md
├── appendix-why-gke.md
├── appendix-kind-quickstart.md
├── appendix-install-script-alternative.md   # ./hack/install-ate.sh path
├── appendix-benchmarking.md
├── tracks/
│   ├── install-track.md
│   ├── demos-track.md
│   └── kagent-track.md
└── assets/
    ├── env/ate-dev-env.sh.example           # mirror of upstream env template
    ├── images/{kagent.png,suberror.png}     # referenced by 060
    └── agentharness/{openclaw-substrate-demo.yaml,gateway-token.yaml}
```
