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

### `kcompass init <url-or-path>`

Explicitly registers a backend by URL or local path. Writes to `~/.kcompass/config.yaml`.
The backend type is inferred from the URL: `https://`, `http://`, `git@`, `git://`, and
`ssh://` produce a git backend; anything else is treated as a local file path.

```
$ kcompass init git@github.com:company/clusters
Backend registered: git@github.com:company/clusters

To advertise via DNS auto-discovery: kcompass operator dns git@github.com:company/clusters
```

### `kcompass backends`

Lists configured backends and their status.

```
$ kcompass backends
TYPE   SOURCE                                STATUS
git    git@github.com:company/clusters       ok
local  ~/.kcompass/local.yaml                ok
```

### `kcompass operator dns <url>`

Prints the DNS TXT records that operators publish to advertise a backend via
auto-discovery. The same `v=kc1; backend=<url>` value is used at three different
hostnames depending on the network (corporate DNS search domain, Tailscale
MagicDNS suffix, Netbird management domain). When run on a machine that is already
attached to those networks, real detected hostnames are shown instead of placeholders.

```
$ kcompass operator dns git@github.com:company/clusters
$ kcompass operator dns git@github.com:company/clusters --verify
```

The `--verify` flag performs live TXT lookups against each detected domain and
reports `OK`, `mismatch`, or `not found` per hostname.

### `kcompass operator add`

Scaffolds a single cluster record into a YAML inventory. All required values can
be supplied via flags (suitable for CI/scripts) or interactively when stdin is a
terminal. Two credential modes are supported, mirroring `KubeconfigSpec` (see
[Core Concepts](#core-concepts) below):

```
$ kcompass operator add \
    --name nonchord-staging \
    --description "Staging cluster (Tailscale operator)" \
    --command "tailscale configure kubeconfig nonchord-staging" \
    --output clusters.yaml

$ kcompass operator add \
    --name dev-laptop \
    --kubeconfig ~/.kube/dev-laptop-config \
    --output clusters.yaml
```

### Global flags

- `--config <path>` — override the default config file location
- `--verbose` / `-v` — emit per-probe discovery diagnostics on stderr

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

## Backends

kcompass ships with two backends. Both produce the same `[]ClusterRecord`
output and use identical YAML file formats; they differ only in how the file is
sourced.

There is intentionally no cloud-provider backend (GKE/EKS/AKS). The original
plan included one, but with the `KubeconfigSpec` model a GKE cluster is just a
record whose `kubeconfig.command` is `[gcloud, container, clusters, get-credentials, ...]`.
The job a runtime cloud backend would do — sparing operators from authoring those
records — is instead served at inventory-generation time (e.g. via `kcompass operator add`,
or a Terraform output, or a one-shot script that walks `gcloud container clusters list`).
Cluster inventories change quarterly, not per-second; a fast local read with manual
refresh is the right tradeoff over an API call on every `kcompass list`.

### 1. Git

Clones or fetches a Git repository and reads cluster YAML files from a path within it.
Each `.yaml` file with a top-level `clusters:` key is parsed into `[]ClusterRecord`;
files without that key are silently skipped, so a repo can mix cluster inventory with
other YAML.

Supports:
- HTTPS with token auth (`GIT_TOKEN` env var)
- SSH with default key (via SSH agent, then `~/.ssh/id_*`)
- Public repos (no auth)

The local clone is cached at `~/.kcompass/cache/git/<hash-of-url>/` and refreshed
on a configurable TTL (default: always fetch).

Config example:
```yaml
backends:
  - type: git
    url: git@github.com:company/clusters
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

### 2. Local YAML

Reads a single local file. Useful for personal dev clusters, overrides, or
air-gapped environments. Uses the same file format as the Git backend.

Config example:
```yaml
backends:
  - type: local
    path: ~/.kcompass/local.yaml
```

---

## Auto-Discovery

When no backends are configured, kcompass runs three probes in parallel
(500ms timeout, configurable). All three use **DNS TXT records** with the
same value format and differ only in which hostname they query:

```
"v=kc1; backend=<git-or-local-url>"
```

### Probes

| # | Probe | Hostname queried | Detection prerequisite |
|---|---|---|---|
| 1 | **Tailscale** | `kcompass.<tailnet-magic-dns-suffix>` | tailscaled socket present, `tailscale status --json` succeeds, `MagicDNSSuffix` non-empty |
| 2 | **Netbird** | `kcompass.<management-server-domain>` | `wt0` WireGuard interface present, `netbird status --json` succeeds |
| 3 | **DNS search domains** | `kcompass.<domain>` for each domain in `/etc/resolv.conf` | none (always runs) |

The first probe to return a valid record wins. Each found backend is named with
its discovery provenance (`tailscale:<url>`, `netbird:<url>`, `dns:<domain>:<url>`)
so `kcompass list --verbose` shows where a cluster came from.

If all probes return no records, kcompass prints:
```
No cluster registry found. Run `kcompass init <url>` to configure a backend,
or connect to your company VPN/Tailscale network and try again.
```

### Why DNS TXT for all three

Originally we considered SRV records (which point at host:port) for the
Tailscale and Netbird probes. SRV is too restrictive: it can only express
HTTP backends. TXT records carry an arbitrary URL, so a single mechanism
covers git over SSH, git over HTTPS, and (in principle) anything else we
add later. Operators have one record format to learn and one
[`kcompass operator dns`](#kcompass-operator-dns-url) command to verify.

### Publishing records (for operators)

```
kcompass.internal.company.com.   300 IN TXT "v=kc1; backend=git@github.com:company/clusters"
kcompass.your-tailnet.ts.net.    300 IN TXT "v=kc1; backend=git@github.com:company/clusters"
kcompass.app.netbird.io.         300 IN TXT "v=kc1; backend=git@github.com:company/clusters"
```

The `kcompass operator dns <url>` command prints the exact records to add. Run
it on a machine that's already attached to the relevant network and it will
substitute in the real domain names.

---

## Configuration File

Location: `~/.kcompass/config.yaml`

```yaml
backends:
  - type: git
    url: git@github.com:company/clusters
    path: clusters/
  - type: local
    path: ~/.kcompass/local.yaml

cache:
  ttl: 5m                 # how long the registry caches the merged cluster list
  path: ~/.kcompass/cache/

discovery:
  enabled: true           # default; set to false to disable auto-discovery entirely
  timeout: 500ms          # per-probe timeout for network discovery
```

Discovery only runs when no `backends:` are configured. Once any backend is
present, kcompass uses it exclusively and skips probing.

---

## Out of Scope

- Creating or modifying clusters (kcompass is read/connect only)
- Managing RBAC or cluster permissions
- Any UI beyond the CLI
- Windows support in v1 (Linux and macOS only)
- Storing credentials — kcompass either embeds a kubeconfig the operator authored
  or runs an operator-supplied command that writes to `KUBECONFIG`. It never holds
  long-lived credentials of its own.
- Runtime cloud-provider backends (GKE/EKS/AKS) — superseded by inventory generation
