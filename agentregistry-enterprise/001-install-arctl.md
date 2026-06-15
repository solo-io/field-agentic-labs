# Install the Enterprise `arctl` CLI

`arctl` is the AgentRegistry Enterprise CLI. It manages registry resources (`Agent`, `MCPServer`, `Runtime`, `Deployment`, `AccessPolicy`) and is required for every other lab.

## Lab Objectives

- Install the Enterprise `arctl` binary
- Put it on your `PATH`
- Verify it works against `--json` output

## Install

```bash
export ARCTL_VERSION=v2026.5.4
curl -sSL https://storage.googleapis.com/agentregistry-enterprise/install.sh \
  | ARCTL_VERSION=$ARCTL_VERSION sh
export PATH=$HOME/.arctl/bin:$PATH
```

The install script drops the binary at `$HOME/.arctl/bin/arctl`.

## Verify

```bash
arctl version --json
```

You should see something like:

```json
{
  "arctl_version": "v2026.5.4",
  "server_version": ""
}
```

`server_version` will be empty until you point the CLI at an installed AgentRegistry Enterprise server (lab [040](040-arctl-auth.md)).

## OSS vs Enterprise CLI

The OSS `arctl` (often at `/usr/local/bin/arctl`) does **not** have `user login`, `apply`, `provider`, or other enterprise commands. If both are installed, make sure `$HOME/.arctl/bin` is **first** on your `PATH`:

```bash
which -a arctl
# /Users/you/.arctl/bin/arctl    ← enterprise, want this first
# /usr/local/bin/arctl           ← oss
```

If the OSS one is winning, prepend the enterprise path in your shell profile:

```bash
echo 'export PATH="$HOME/.arctl/bin:$PATH"' >> ~/.zshrc
```

## Pinning Versions

The labs were validated with the versions listed in the [README](README.md#validated-on). Pin `ARCTL_VERSION` rather than re-running the install script unpinned — the CLI is tightly coupled to the server chart version.

## Next

- [010 — Cluster Prerequisites](010-cluster-prereqs.md) — if you don't already have a cluster
- [020 — Microsoft Entra ID OIDC](020-setup-entra.md) — or [021 — Keycloak OIDC](021-setup-keycloak.md)
