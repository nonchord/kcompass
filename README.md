# kcompass

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Contributor Covenant](https://img.shields.io/badge/Contributor%20Covenant-2.1-4baaaa.svg)](CODE_OF_CONDUCT.md)

`kcompass` is a CLI tool that lets engineers discover and connect to Kubernetes clusters without manual kubeconfig management. It maintains a registry of clusters sourced from one or more configurable backends, and handles credential acquisition and kubeconfig merging on connect.

```
$ kcompass list
NAME        DESCRIPTION
cluster1    The production cluster.
cluster2    The staging cluster.

$ kcompass connect cluster1
Setting up kubeconfig for cluster1... Done.
Context is set to cluster1.
```

---

## Installation

**Requires Go 1.26 or later.**

```sh
go install github.com/nonchord/kcompass/cmd/kcompass@latest
```

Or build from source:

```sh
git clone https://github.com/nonchord/kcompass.git
cd kcompass
go build -o kcompass ./cmd/kcompass
```

---

## Quick start

If your network publishes a kcompass DNS TXT record (corporate DNS, Tailscale,
or Netbird), kcompass works with **zero configuration**:

```sh
kcompass list
kcompass connect my-cluster
```

Otherwise register a backend explicitly:

```sh
# Local file
kcompass init ~/my-clusters.yaml

# Git repository (https or ssh URL)
kcompass init git@github.com:your-org/clusters

# List all clusters
kcompass list

# Connect (resolves credentials, merges kubeconfig, sets current context)
kcompass connect my-cluster
```

If you're an operator wanting other engineers to discover your registry without
manual setup, see [`kcompass operator dns`](#auto-discovery) below.

---

## Configuration

kcompass reads `~/.kcompass/config.yaml`. Running `kcompass init` writes entries to this file automatically, but you can also edit it by hand.

```yaml
backends:
  - type: local
    path: ~/.kcompass/clusters.yaml

  - type: git
    url: git@github.com:your-org/clusters
    path: clusters/       # subdirectory to scan, default: repo root
    ref: main             # branch/tag/SHA, default: default branch

cache:
  ttl: 5m                 # how long to cache the merged cluster list in memory
  path: ~/.kcompass/cache/

discovery:
  enabled: true           # default; set false to disable auto-discovery entirely
  timeout: 500ms          # per-probe network timeout, default 500ms
```

Auto-discovery only runs when `backends:` is empty. Once any backend is configured,
kcompass uses it exclusively and skips discovery probes.

---

## Backends

### Local

Reads cluster records from a YAML file on disk. Useful for personal dev clusters, overrides, or air-gapped environments.

```yaml
backends:
  - type: local
    path: ~/.kcompass/clusters.yaml
```

**File format:**

```yaml
clusters:
  - name: dev
    description: Local dev cluster.
    kubeconfig:
      inline: |
        apiVersion: v1
        kind: Config
        # ... full kubeconfig blob
```

Register via CLI:

```sh
kcompass init /path/to/clusters.yaml
```

### Git

Clones or pulls a Git repository and reads cluster YAML files from a configurable path within it. The local clone is cached at `~/.kcompass/cache/git/<hash>/` and refreshed on a configurable TTL.

```yaml
backends:
  - type: git
    url: https://github.com/your-org/clusters
    path: clusters/   # subdirectory to scan, default: repo root
    ref: main         # branch/tag/SHA, default: default branch
```

**Authentication:**

| URL scheme | Auth method |
|---|---|
| `https://` | `GIT_TOKEN` env var (if set), otherwise unauthenticated |
| `git@` / `ssh://` | SSH agent, then `~/.ssh/id_*` default keys |
| `file://` | None required |

```sh
# HTTPS with token
GIT_TOKEN=ghp_... kcompass list

# Register via CLI
kcompass init https://github.com/your-org/clusters
kcompass init git@github.com:your-org/clusters
```

**Cluster file format** (any `.yaml` file in the scanned path):

```yaml
clusters:
  # Per-user credentials minted by a command (Tailscale operator, gcloud, aws, ...)
  - name: nonchord-staging
    description: Staging cluster (Tailscale operator)
    labels:
      env: staging
      team: platform
    kubeconfig:
      command: [tailscale, configure, kubeconfig, nonchord-staging]

  - name: production
    description: Production GKE cluster
    kubeconfig:
      command:
        - gcloud
        - container
        - clusters
        - get-credentials
        - production
        - --region=us-east1
        - --project=my-gcp-project

  # Embedded kubeconfig (the same kubeconfig works for everyone)
  - name: dev-laptop
    description: Local k3s
    kubeconfig:
      inline: |
        apiVersion: v1
        kind: Config
        # ... full kubeconfig blob
```

Files without a top-level `clusters:` key are silently skipped, so a repo can contain other YAML without causing errors.

---

## Auto-discovery

When no backends are configured, kcompass automatically probes for a registry using three mechanisms in parallel (500ms timeout):

All three mechanisms use the same DNS TXT record format. The record value is always:

```
"v=kc1; backend=<git-url>"
```

The hostname where you publish it depends on your network.

### 1. Corporate DNS

kcompass reads the DNS search domains from `/etc/resolv.conf` and performs a TXT lookup for `kcompass.<domain>` on each one:

```
kcompass.internal.company.com. 300 IN TXT "v=kc1; backend=git@github.com:company/clusters"
```

### 2. Tailscale

If tailscaled is running, kcompass looks up `kcompass.<tailnet-magic-dns-suffix>` as a TXT record:

```
kcompass.your-tailnet.ts.net. 300 IN TXT "v=kc1; backend=git@github.com:company/clusters"
```

### 3. Netbird

If the Netbird WireGuard interface (`wt0`) is present, kcompass looks up `kcompass.<management-server-domain>` as a TXT record:

```
kcompass.app.netbird.io. 300 IN TXT "v=kc1; backend=git@github.com:company/clusters"
```

### Generating the record

`kcompass operator dns` prints the exact record to add for any backend URL:

```sh
$ kcompass operator dns git@github.com:company/clusters
```

### Disabling discovery

```yaml
discovery:
  enabled: false
```

---

## Commands

| Command | Description |
|---|---|
| `kcompass list` | List all clusters across configured backends |
| `kcompass list --json` | JSON output for scripting |
| `kcompass list --backend <name>` | Restrict to a specific backend |
| `kcompass connect <name>` | Resolve credentials and set current context |
| `kcompass connect <name> --no-switch` | Merge credentials without switching context |
| `kcompass init <url-or-path>` | Register a backend (git or local, inferred from URL) |
| `kcompass backends` | List configured backends and their status |
| `kcompass operator dns <url>` | Print DNS TXT records for auto-discovery |
| `kcompass operator dns <url> --verify` | Verify TXT records are published correctly |
| `kcompass operator add` | Scaffold a cluster entry into an inventory file (interactive or flag-driven) |
| `--verbose` / `-v` | Global flag: emit per-probe discovery diagnostics on stderr |

---

## ClusterRecord schema

Every backend produces a list of `ClusterRecord` values. Each record needs a name and **exactly one** of `kubeconfig.inline` or `kubeconfig.command`:

```yaml
name: my-cluster              # required, unique identifier
description: "..."            # optional, human-readable
labels:                       # optional, free-form key/value tags
  env: production
  team: platform
kubeconfig:
  # Choose ONE of these two:

  # 1. Inline — embed a kubeconfig that works for every user.
  #    Use for OIDC + kubelogin, kube-oidc-proxy, service account tokens
  #    behind Netbird, static dev clusters.
  inline: |
    apiVersion: v1
    kind: Config
    # ...

  # 2. Command — argv kcompass runs to mint per-user credentials.
  #    Use for tools that bind credentials to the running user's identity:
  #    Tailscale operator, gcloud, aws, or any custom helper.
  command: [tailscale, configure, kubeconfig, my-cluster]
```

When `command` is used, kcompass runs the argv with `KUBECONFIG` set to a fresh
temp file, captures the resulting kubeconfig, and merges it. This works uniformly
for any tool that respects `KUBECONFIG` (gcloud, aws, tailscale, kubectl, helm).
stdin/stdout/stderr are passed through so interactive prompts (e.g. `gcloud auth login`
opening a browser) work normally.

Inventory is validated at parse time, so a malformed record fails loudly during
`kcompass list` rather than silently waiting for someone to try `connect`.

---

## Contributing

Contributions are welcome. Please read [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) before opening an issue or pull request.
