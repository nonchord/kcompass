# kcompass — Specification

## What This Tool Is

`kcompass` is a CLI tool that lets engineers discover and connect to Kubernetes clusters without
manual kubeconfig management. It maintains a registry of clusters (sourced from one or more
configurable backends) and handles credential acquisition and kubeconfig merging on connect.

The target user is a developer who may be new to a company, on-call for the first time, or
switching between many clusters. The goal is that `kcompass list` works with zero setup in as
many environments as possible, and `kcompass connect <name>` handles everything needed to make
`kubectl` work against that cluster.

---

## CLI Interface

### `kcompass list`

Lists all clusters visible across configured backends.

```
$ kcompass list
NAME        DESCRIPTION
cluster1    The production cluster.
cluster2    The staging cluster.
```

Flags:
- `--backend <name>` — restrict output to a specific backend
- `--json` — emit JSON for scripting

### `kcompass connect <name>`

Acquires credentials for the named cluster, merges them into `~/.kube/config`, and sets the
current context.

```
$ kcompass connect cluster1
Setting up kubeconfig for cluster1... Done.
Context is set to cluster1.
```

Flags:
- `--no-switch` — merge credentials but do not change the current context

### `kcompass init <url>`

Explicitly registers a backend by URL. Writes to `~/.kcompass/config.yaml`.

```
$ kcompass init https://clusters.internal.company.com
Backend registered: https://clusters.internal.company.com
```

### `kcompass backends`

Lists configured backends and their status (reachable, cached, error).

```
$ kcompass backends
TYPE     SOURCE                                  STATUS
gke      project=my-project region=us-east1      ok
git      https://github.com/company/clusters     ok
local    ~/.kcompass/local.yaml                  ok
```

---

## Core Concepts

### ClusterRecord

Every backend produces a list of `ClusterRecord` values. This is the canonical schema:

```go
type ClusterRecord struct {
    Name        string            `yaml:"name"`
    Description string            `yaml:"description,omitempty"`
    Labels      map[string]string `yaml:"labels,omitempty"`
    Kubeconfig  KubeconfigSpec    `yaml:"kubeconfig"`
}

type KubeconfigSpec struct {
    // Inline is a complete kubeconfig YAML blob shipped with the record.
    Inline  string   `yaml:"inline,omitempty"`
    // Command is an argv vector kcompass runs to mint a per-user kubeconfig.
    Command []string `yaml:"command,omitempty"`
}
```

Exactly one of `Inline` or `Command` must be set on every record. This is
validated at parse time so broken inventory fails loudly when `kcompass list`
runs, not silently when `kcompass connect` is later invoked.

#### Two credential modes, one merge path

kcompass deliberately has only two ways to obtain a kubeconfig fragment:

| Mode | When to use | Example |
|------|-------------|---------|
| **`inline`** | The same kubeconfig works for everyone — OIDC with `kubelogin`, `kube-oidc-proxy`, service account tokens behind Netbird, static dev clusters. The kubeconfig is shipped in the record and merged as-is. | A kubeconfig whose user has an `exec:` block invoking `kubectl-oidc_login` |
| **`command`** | A tool mints per-user credentials on demand — Tailscale operator (`tailscale configure kubeconfig`), GKE (`gcloud container clusters get-credentials`), EKS (`aws eks update-kubeconfig`), or any custom helper. | `[tailscale, configure, kubeconfig, prod]` |

When `command` is used, kcompass runs the argv with `KUBECONFIG` set to a fresh
temp file, captures the resulting kubeconfig, and removes the temp file. This
works uniformly for any tool that respects `KUBECONFIG` (gcloud, aws, tailscale,
kubectl, helm). stdin/stdout/stderr are passed through so interactive prompts
(e.g. `gcloud auth login` opening a browser) work normally.

#### What's intentionally NOT in the schema

- **Provider** (`gke`/`eks`/`generic`) — was advisory only. The kubeconfig is
  self-describing; if a human needs to know "this is GKE" it goes in `description`.
- **Reachability mechanism** (Tailscale, Netbird, VPN) — orthogonal to credentials.
  How the API server is reachable is a network concern outside kcompass's scope.
  Netbird and kube-oidc-proxy are invisible to kcompass: they just affect the
  server URL inside an inline kubeconfig.
- **Provider-specific metadata maps** — replaced by the kubeconfig itself.

---

## Backend Interface

All backends implement this interface. No exceptions — new backends must satisfy it fully.

```go
type Backend interface {
    // Name returns the unique identifier for this backend instance.
    Name() string

    // List returns all cluster records visible to this backend.
    List(ctx context.Context) ([]ClusterRecord, error)

    // Get returns a single cluster record by name, or ErrNotFound.
    Get(ctx context.Context, name string) (*ClusterRecord, error)
}

var ErrNotFound = errors.New("cluster not found")
```

The registry layer (not individual backends) handles:
- Merging results from multiple backends (backends tried in config order)
- Deduplication (first backend to return a name wins)
- Caching (TTL-based, configurable, default 5 minutes)

---

## Backends — Implementation Priority

### 1. Cloud Provider (GKE / EKS)

Queries cloud provider APIs directly using ambient credentials (`gcloud`, `aws` CLIs or
their underlying credential chains). Requires no registry configuration.

**GKE:** Uses the GKE API (`container.googleapis.com`) to list clusters across a project.
Project is inferred from `gcloud config get-value project` if not specified.

**EKS:** Uses the EKS API to list clusters in a region. Region inferred from AWS config.

This is the only backend that can operate with zero `kcompass` configuration. It should be
attempted automatically if no backends are configured and cloud credentials are detected.

Config example:
```yaml
backends:
  - type: gke
    project: my-gcp-project     # optional, inferred if omitted
    region: us-east1            # optional, queries all regions if omitted
  - type: eks
    region: us-east-1
```

### 2. Git

Clones or fetches a Git repository and reads cluster YAML files from a path within it.
Each file (or a single `clusters.yaml` listing) is parsed into `[]ClusterRecord`.

Supports:
- HTTPS with token auth (`GIT_TOKEN` env var or keychain)
- SSH with default key
- Public repos (no auth)

The local clone is cached at `~/.kcompass/cache/git/<hash-of-url>/` and refreshed on TTL.

Config example:
```yaml
backends:
  - type: git
    url: https://github.com/company/clusters
    path: clusters/             # subdirectory within repo, default: repo root
    ref: main                   # branch/tag/sha, default: default branch
```

Cluster file format (each `.yaml` file in `path`, or a single `clusters.yaml`):
```yaml
clusters:
  # Per-user credentials minted by a command
  - name: nonchord-staging
    description: Staging cluster (Tailscale operator)
    labels:
      env: staging
      team: platform
    kubeconfig:
      command: [tailscale, configure, kubeconfig, nonchord-staging]

  # Embedded kubeconfig for everyone
  - name: dev-laptop
    description: Local k3s on the dev laptop
    kubeconfig:
      inline: |
        apiVersion: v1
        kind: Config
        clusters:
          - name: dev-laptop
            cluster: { server: https://192.168.1.50:6443 }
        # ...
```

### 3. HTTP / REST API

Fetches cluster records from an HTTP endpoint. Expects a JSON response:

```json
{
  "clusters": [
    {
      "name": "cluster1",
      "description": "The production cluster.",
      "provider": "gke",
      "auth": "gcloud",
      "metadata": { "project": "my-project", "region": "us-east1", "cluster_id": "cluster1" }
    }
  ]
}
```

Supports bearer token auth (`Authorization: Bearer <token>`) via env var or config.

Config example:
```yaml
backends:
  - type: http
    url: https://clusters.internal.company.com/api/clusters
    token_env: KCLUSTER_TOKEN   # env var name, optional
```

### 4. Local YAML

Reads a local file. Useful for personal dev clusters, overrides, or airgapped environments.
Uses the same file format as the Git backend.

Config example:
```yaml
backends:
  - type: local
    path: ~/.kcompass/local.yaml
```

---

## Auto-Discovery Sequence

When no backends are configured, kcompass attempts discovery in this order. All network
attempts are parallel with a 500ms timeout. Results are cached to avoid repeated lookups.

```
1. Detect Tailscale daemon (tailscaled socket or `tailscale status`)
      → SRV lookup: _kcompass._tcp.<tailnet-domain>
      → If found: use returned host as HTTP backend URL

2. Detect Netbird daemon (netbird service or WireGuard interface wt0)
      → SRV lookup: _kcompass._tcp.<netbird-domain>
      → If found: use returned host as HTTP backend URL

3. Read DNS search domains from /etc/resolv.conf (or OS resolver on macOS)
      → For each domain: TXT lookup kcompass.<domain>
      → If TXT value begins with "v=kc1": parse and use as backend URL

4. Detect gcloud credentials → attempt GKE backend (see above)

5. Detect AWS credentials → attempt EKS backend (see above)

6. All failed → print:
   No cluster registry found. Run `kcompass init <url>` to configure a backend,
   or connect to your company VPN/Tailscale network and try again.
```

Daemon detection details:
- **Tailscale:** check for socket at `/var/run/tailscale/tailscaled.sock` or run `tailscale status --json`
- **Netbird:** check for WireGuard interface `wt0` via netlink, or `netbird status`

DNS TXT record format (for step 3):
```
kcompass.internal.company.com TXT "v=kc1; backend=https://clusters.internal.company.com"
```

---

## Configuration File

Location: `~/.kcompass/config.yaml`

```yaml
backends:
  - type: gke
    project: my-project
  - type: git
    url: https://github.com/company/clusters
    path: clusters/
  - type: local
    path: ~/.kcompass/local.yaml

cache:
  ttl: 5m
  path: ~/.kcompass/cache/

discovery:
  enabled: true       # whether to attempt auto-discovery when no backends configured
  timeout: 500ms      # per-probe timeout for network discovery
```

---

## Out of Scope

- Creating or modifying clusters (kcompass is read/connect only)
- Managing RBAC or cluster permissions
- Any UI beyond the CLI
- Windows support in v1 (Linux and macOS only)
- Storing credentials — kcompass delegates entirely to the auth method (gcloud, aws, OIDC)
  and writes only what those tools produce into kubeconfig
