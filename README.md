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

**Requires Go 1.21 or later.**

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

Register a backend and list your clusters:

```sh
# Local file
kcompass init ~/my-clusters.yaml

# Git repository
kcompass init https://github.com/your-org/clusters

# List all clusters
kcompass list

# Connect (merges kubeconfig and sets current context)
kcompass connect my-cluster
```

---

## Configuration

kcompass reads `~/.kcompass/config.yaml`. Running `kcompass init` writes entries to this file automatically, but you can also edit it by hand.

```yaml
backends:
  - type: local
    path: ~/.kcompass/clusters.yaml

  - type: git
    url: https://github.com/your-org/clusters
    path: clusters/       # subdirectory to scan, default: repo root
    ref: main             # branch/tag/SHA, default: default branch

cache:
  ttl: 5m                 # how long to cache the merged cluster list in memory
  path: ~/.kcompass/cache/

discovery:
  enabled: true           # attempt auto-discovery when no backends are configured
  timeout: 500ms
```

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
    provider: generic
    auth: static
    metadata:
      kubeconfig: |
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
  - name: production
    description: Production GKE cluster.
    provider: gke
    auth: gcloud
    metadata:
      project: my-gcp-project
      region: us-east1
      cluster_id: production
```

Files without a top-level `clusters:` key are silently skipped, so a repo can contain other YAML without causing errors.

---

## Commands

| Command | Description |
|---|---|
| `kcompass list` | List all clusters across configured backends |
| `kcompass list --json` | JSON output for scripting |
| `kcompass list --backend <name>` | Restrict to a specific backend |
| `kcompass connect <name>` | Merge credentials and set current context |
| `kcompass connect <name> --no-switch` | Merge credentials without switching context |
| `kcompass init <url-or-path>` | Register a backend |
| `kcompass backends` | List configured backends and their status |

---

## ClusterRecord schema

Every backend produces a list of `ClusterRecord` values:

```yaml
name: my-cluster           # unique identifier
description: "..."         # human-readable description
provider: gke              # gke | eks | aks | generic
auth: gcloud               # gcloud | aws | oidc | static
metadata:                  # provider-specific fields
  project: my-project
  region: us-east1
  cluster_id: my-cluster
```

---

## Contributing

Contributions are welcome. Please read [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) before opening an issue or pull request.
