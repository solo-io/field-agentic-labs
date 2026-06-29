# Baseline Setup

The first of two mandatory setup labs. This lab takes you from "I have a Kubernetes cluster" to "I have everything needed to install OIDC + agentregistry Enterprise." Subsequent labs assume this baseline is in place.

## Lab Objectives

- Confirm your cluster has the prerequisites agentregistry Enterprise needs (Kubernetes ≥ 1.29, default `StorageClass`, `LoadBalancer`-capable Service controller)
- Create the `agentregistry-system` namespace
- Install the Enterprise `arctl` CLI
- Make sure your shell has the tools the rest of the workshop expects (`kubectl`, `helm`, `openssl`, `envsubst`)
- Confirm a `LoadBalancer` Service can actually get an external address (managed clusters: yes; bare-metal: install MetalLB / kube-vip first; `kind`: use `cloud-provider-kind`). The reason why is for your OIDC providers redirect to log into agentregistry with said OIDC provider.

## What This Lab Does **Not** Do

This lab is on purpose minimal. It does not install Keycloak, agentregistry, or Enterprise Agentgateway. Those come next:

- **OIDC** ([002a](002a-setup-oidc-keycloak.md) Keycloak **or** [002b](002b-setup-oidc-entra.md) Entra ID)
- **Components** ([003](003-install-components.md): agentregistry + Enterprise Agentgateway)

Some later unit labs (020, 031, 061) also need **kagent Enterprise** installed on the cluster. That's a prereq the user satisfies separately via the [kagent-enterprise workshop](https://github.com/solo-io/field-agentic-labs/tree/main/kagent-enterprise) - it isn't installed by 003.

After **001 → 002a/b → 003**, you have the baseline that every unit-of-value lab (010+) assumes.

## Prerequisites

- A running Kubernetes cluster (≥ 1.29). Any flavor - GKE, EKS, AKS, kind, k3s, Rancher Desktop. The workshop is validated on managed clusters with a default `StorageClass` and a `LoadBalancer` Service controller.
- `kubectl` configured to talk to the cluster
- `helm` v3
- `openssl` (for any TLS bootstrap your IdP needs)
- `envsubst` (ships with GNU gettext - on macOS, `brew install gettext && brew link --force gettext`)
- (Optional but recommended) `jq` for command examples that pipe JSON

## 1. Confirm the Cluster Is Ready

```bash
kubectl version
kubectl get nodes
kubectl get storageclass
```

You need at least one `StorageClass` with `(default)` in the output as agentregistry's bundled PostgreSQL and ClickHouse both request PVs. If none is marked default, mark one before continuing:

```bash
kubectl annotate storageclass <name> storageclass.kubernetes.io/is-default-class=true
```

## 2. Create the agentregistry Namespace

```bash
kubectl create namespace agentregistry-system
```

## 3. Install the Enterprise `arctl` CLI

```bash
export ARCTL_VERSION=v2026.6.2
curl -sSL https://storage.googleapis.com/agentregistry-enterprise/install.sh \
  | ARCTL_VERSION=$ARCTL_VERSION sh
export PATH=$HOME/.arctl/bin:$PATH
```

Persist the `PATH` change in your shell profile:

```bash
echo 'export PATH="$HOME/.arctl/bin:$PATH"' >> ~/.zshrc   # adjust for bash / fish
```

Verify:

```bash
arctl version --json
```

You should see `arctl_version` populated and `server_version` empty (no server installed yet - that's expected).

### OSS vs Enterprise `arctl`

If you also have the OSS `arctl` on `PATH` (e.g., `/usr/local/bin/arctl`), make sure the Enterprise one wins:

```bash
which -a arctl
# /Users/you/.arctl/bin/arctl   ← Enterprise, want this first
# /usr/local/bin/arctl          ← OSS
```

The Enterprise CLI has `arctl user login`, `arctl apply`, `arctl provider setup aws`, and the approval-workflow API surface that the OSS one lacks.

## 4. Sanity-Check Your Shell Has Everything

```bash
command -v kubectl   && kubectl   version --client | head -1
command -v helm      && helm      version --short
command -v openssl   && openssl   version
command -v envsubst  && echo "envsubst found"
command -v arctl     && arctl     version --json
command -v jq        && jq        --version
```

Every line should print a version or "found." If `envsubst` is missing on macOS:

```bash
brew install gettext
brew link --force gettext
```

## What's in Place After This Lab

| Resource | State |
|---|---|
| Kubernetes cluster | Up, has default `StorageClass`, `LoadBalancer` works |
| `agentregistry-system` namespace | Created |
| `arctl` CLI | Installed on `PATH`, Enterprise build wins resolution |
| Other tools (`kubectl`, `helm`, `openssl`, `envsubst`, `jq`) | Present |

This is the **baseline** every unit-of-value lab (010+) assumes. Don't tear it down between labs.

## Cleanup

Roll back this lab only when you're done with the whole workshop. The full teardown is in [099](099-cleanup.md). The local-only steps:

```bash
# Remove the arctl binary + PATH entry
rm -rf "$HOME/.arctl"
# (Remove the PATH export line from ~/.zshrc / ~/.bashrc by hand)

# Remove the namespace (this also nukes everything 003 installs into it)
kubectl delete namespace agentregistry-system --ignore-not-found
```

## Next

Pick one OIDC path:

- [002a - Setup OIDC: Keycloak (in-cluster)](002a-setup-oidc-keycloak.md) - recommended for a self-contained POC; no cloud account needed
- [002b - Setup OIDC: Entra ID](002b-setup-oidc-entra.md) - recommended if you already have a Microsoft Entra tenant
