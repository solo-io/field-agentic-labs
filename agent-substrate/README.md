# Agent Substrate Workshop

A hands-on lab series for **[Agent Substrate](https://github.com/agent-substrate/substrate)** - the Kubernetes-native runtime that multiplexes many idle "actors" (agent-like workloads) onto a small pool of warm "worker" pods.

The workshop is built around two ideas:

1. **Three setup labs, soup to nuts.** [001](001-baseline-setup.md) → [002](002-gcp-iam-and-bucket.md) → [003](003-install-substrate.md) takes you from a bare Kubernetes cluster to a working Substrate install + `kubectl-ate` CLI. You only run setup once.

2. **Independent unit-of-value labs.** Every lab numbered 010 and up is standalone - it states what it needs from the baseline, walks through one capability, and has its own `## Cleanup` section that returns the cluster to the post-baseline state. Run them in any order, run cleanup, move on.

This workshop **links out to the upstream Substrate repo** (`github.com/agent-substrate/substrate`) for the source tree - `assets/` here holds only small lab-specific artifacts (the env-file template, parameterized `AgentHarness` manifests for the kagent integration, and the two screenshots used by the kagent lab).

## Prerequisites

- A **GKE Standard** cluster (Autopilot is unsupported - see [appendix-why-gke](appendix-why-gke.md))
- A GCP project with billing on
- Go ≥ 1.26.3 (matches the Substrate `go.mod` toolchain)
- `gcloud`, `kubectl`, `helm`, `git`, `openssl`, `curl`
- An LLM provider API key for the kagent integration ([020](020-kagent-integration.md))

# Table of Contents

- [Setup (mandatory)](#setup-mandatory)
- [Substrate Demos](#substrate-demos)
- [Kagent Integration](#kagent-integration)
- [Operations & Observability](#operations--observability)
- [Cleanup](#cleanup)
- [Appendix](#appendix)

---

## Setup (mandatory)

> Three labs. Run them in order, once. After this, every lab from 010 onwards is independent.

- [001 - Baseline Setup](001-baseline-setup.md) - cluster prereqs check + tool install + clone upstream + env file
- [002 - GCP IAM, Snapshot Bucket, and `kubectl` Context](002-gcp-iam-and-bucket.md)
- [003 - Install Substrate (Helm + `kubectl-ate`)](003-install-substrate.md)

---

## Substrate Demos

Pick any combination. Each is self-contained, each has its own Cleanup.

- [010 - Counter Demo (stateful HTTP, suspend/resume)](010-counter-demo.md) ← canonical first demo
- [011 - Sandbox Demo (Alpine shell + REPL client)](011-sandbox-demo.md)
- [012 - Agent-Secret Demo (Zero-Idle self-suspend + RAM persistence)](012-agent-secret-demo.md)
- [013 - Claude Code Multiplex (3 agents on 2 pods)](013-claude-code-multiplex.md) - **upstream DRAFT**

---

## Kagent Integration

- [020 - kagent Integration (install + substrate-backed `AgentHarness`)](020-kagent-integration.md) - install kagent with substrate enabled + walk through a real `AgentHarness` end-to-end

---

## Operations & Observability

- [030 - Suspend / Resume Operations](030-operations.md)
- [040 - Logs, Metrics, and Tracing](040-observability.md)

---

## Cleanup

- [099 - Cleanup](099-cleanup.md) - full baseline teardown

---

## Appendix

- [Appendix - Why GKE (the Pod Certificate requirement)](appendix-why-gke.md)
- [Appendix - Local `kind` Quickstart](appendix-kind-quickstart.md) - for laptop dev without GCP
- [Appendix - Install Substrate from Source (`install-ate.sh`)](appendix-install-script-alternative.md)
- [Appendix - Benchmarking with Locust](appendix-benchmarking.md)

---

## Tracks

Curated paths under [`tracks/`](tracks/):

- [`install-track.md`](tracks/install-track.md) - Cluster → Substrate install → counter demo
- [`demos-track.md`](tracks/demos-track.md) - All four demos in sequence
- [`kagent-track.md`](tracks/kagent-track.md) - Substrate + kagent end-to-end

---

## Use Cases

- Multiplex many stateful agent sessions onto a small pool of warm worker pods
- Preserve in-memory + disk state across **suspend → snapshot → resume** cycles, even when the actor lands on a different worker
- Use gVisor sandboxes (`runsc`) to checkpoint and restore real process state, not just container restarts
- Run framework-agnostic agents - ADK, LangChain, Claude Code, MCP servers - as Substrate Actors
- Stand up substrate-backed `AgentHarness` resources from kagent, with kagent generating one `ActorTemplate` per harness
- Operate suspended actors via `kubectl-ate` and the `ateapi.Control` gRPC API

## Validated On

| Component | Version | Used in |
|-----------|---------|---------|
| Go | ≥ 1.26.3 | [001](001-baseline-setup.md) |
| Substrate Helm charts (`oci://ghcr.io/kagent-dev/substrate/helm/{substrate,substrate-crds}`) | floating tags (pin for reproducibility) | [003](003-install-substrate.md) |
| `ateom-gvisor` image (kagent integration) | `ghcr.io/kagent-dev/substrate/ateom-gvisor:v0.0.6` | [020](020-kagent-integration.md) |
| kagent Helm chart | `0.9.7` | [020](020-kagent-integration.md) |
| `runsc` (gVisor) | nightly with `--allow-connected-on-save` (pinned per demo template) | [003](003-install-substrate.md), [010](010-counter-demo.md) |
| Google Cloud SDK | verified at `484.0.0` | [002](002-gcp-iam-and-bucket.md) |
| GKE cluster / node-pool | `1.35.0-gke.2398000` (example default) | [001](001-baseline-setup.md) |
| Node machine type | `c3-standard-4` (any 4-vCPU works) | [001](001-baseline-setup.md) |
| Valkey / Redis (state store) | `valkey/valkey:8.0` | bundled by [003](003-install-substrate.md) |
| Envoy (router) | `envoyproxy/envoy:v1.30-latest` | bundled by [003](003-install-substrate.md) |

> Substrate is in **VERY early development**. Per the upstream README, APIs are almost guaranteed to change. Treat this workshop as a snapshot.

## Repo Layout

```
agent-substrate/
├── README.md # this file
├── 001-baseline-setup.md # cluster prereqs + clone + env file
├── 002-gcp-iam-and-bucket.md # GCP IAM + snapshot bucket
├── 003-install-substrate.md # Helm install + kubectl-ate
├── 010-counter-demo.md
├── 011-sandbox-demo.md
├── 012-agent-secret-demo.md
├── 013-claude-code-multiplex.md # DRAFT upstream
├── 020-kagent-integration.md # install kagent + AgentHarness walkthrough
├── 030-operations.md
├── 040-observability.md
├── 099-cleanup.md
├── appendix-why-gke.md
├── appendix-kind-quickstart.md
├── appendix-install-script-alternative.md
├── appendix-benchmarking.md
├── tracks/
│ ├── install-track.md
│ ├── demos-track.md
│ └── kagent-track.md
└── assets/
 ├── env/ate-dev-env.sh.example # mirror of upstream env template
 ├── images/{kagent.png,suberror.png} # screenshots used by 020
 └── agentharness/ # parameterized AgentHarness + gateway-token
```
