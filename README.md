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

**Quick install** (Linux/macOS):

```sh
curl -fsSL https://raw.githubusercontent.com/nonchord/kcompass/main/install.sh | sh
```

Options: `--dir /usr/local/bin` to change the install path (default: `~/.local/bin`),
`--version v0.2.0` to pin a version.

**With Go** (requires Go 1.26+):

```sh
go install github.com/nonchord/kcompass@latest
```

**From source:**

```sh
git clone https://github.com/nonchord/kcompass.git
cd kcompass
make build        # or: go build -o kcompass .
```

### Development

**Prerequisites:**

- [Go](https://go.dev/dl/) 1.26+
- [golangci-lint](https://golangci-lint.run/welcome/install/) (for `make lint` / `make check`)
- [OpenTofu](https://opentofu.org/docs/intro/install/) or [Terraform](https://developer.hashicorp.com/terraform/install) 1.3+ (only if working on the `terraform/` modules)

A `Makefile` provides the standard workflows:

```sh
make              # lint + test + build (default target)
make test         # go test -race ./...
make lint         # golangci-lint
make cover        # test with coverage report
make check        # fmt + vet + lint + test + tf-check
make clean        # remove build artifacts

# Terraform modules (requires tofu or terraform)
make tf-fmt       # format terraform/ in place
make tf-validate  # init + validate the example (transitively validates modules)
make tf-check     # tf-validate + fmt check (no writes)
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

# DNS zone — resolves the TXT record at kcompass.<zone> to a backend URL
kcompass init example.com

# List all clusters
kcompass list

# Connect (resolves credentials, merges kubeconfig, sets current context)
kcompass connect my-cluster
```

`kcompass init` verifies the backend is actually reachable before writing it
to the config, so a misspelled path or a private repo you don't have access
to is caught immediately instead of on the next `kcompass list`. Pass
`--skip-verify` to bypass (useful when pre-configuring a machine before it
can reach the backend).

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

kcompass shells out to the system `git` binary for clone and fetch, so it
inherits your existing git and SSH configuration — SSH agent, `~/.ssh/config`
host aliases, credential helpers, and everything else git already knows about.

| URL scheme | Auth method |
|---|---|
| `https://` | `GIT_TOKEN` env var, or your git credential helper |
| `git@` / `ssh://` | Your SSH config (agent, key files, `~/.ssh/config`) |
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

If your tailnet advertises additional DNS search paths via the admin console
(e.g. `your-org.com`), kcompass queries `tailscale dns status --json` to
attribute those paths to Tailscale — so `kcompass operator dns --verify`
reports them under the Tailscale row rather than duplicating them under
Corporate DNS.

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
| `kcompass connect <name>` | Resolve credentials and set current context. Idempotent — rerunning against the same cluster is a no-op and prints `already up to date` |
| `kcompass connect <name> --no-switch` | Merge credentials without switching context |
| `kcompass init <url-or-path-or-zone>` | Register a backend. URL schemes (`https://`, `git@`, `ssh://`, …) map to the git backend; DNS zones (e.g. `example.com`) resolve the TXT record at `kcompass.<zone>`; everything else is a local file path |
| `kcompass init --skip-verify <target>` | Register without verifying the backend is reachable (for pre-staging a machine) |
| `kcompass backends` | List configured backends and their status |
| `kcompass operator dns <url>` | Print DNS TXT records for auto-discovery |
| `kcompass operator dns <url> --verify` | Verify the expected TXT records are published on the currently detected networks |
| `kcompass operator dns <url> --verify --hostname <fqdn>` | Verify a specific FQDN instead of the auto-detected set (repeatable, useful from a machine whose resolver isn't configured for the zone) |
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

## Terraform helpers

Optional modules live in [`terraform/modules/`](terraform/) for operators
who manage cluster inventory and auto-discovery with infrastructure-as-code:

- [`kcompass_inventory`](terraform/modules/kcompass_inventory) — renders a
  `ClusterRecord` YAML document from a list of cluster entries. Pure
  value transform with no provider dependency — feed the output into
  `github_repository_file`, `gitlab_repository_file`, `local_file`,
  or any other resource that writes a file.
- [`kcompass_txt`](terraform/modules/kcompass_txt) — pure value formatter
  for the discovery TXT record. Returns `v=kc1; backend=<url>` and the
  conventional label (`kcompass`). Provider-agnostic — feed the outputs
  into `cloudflare_record`, `aws_route53_record`, `google_dns_record_set`,
  or any other DNS resource.

See [`terraform/README.md`](terraform/README.md) for usage examples and
an end-to-end [`examples/basic`](terraform/examples/basic) configuration.

kcompass itself has no dependency on these modules; they're sugar for
operators already living in Terraform.

---

## Contributing

Contributions are welcome. Please read [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) before opening an issue or pull request.
