# kcompass — Claude Code Instructions

This file is read automatically by Claude Code on every invocation. All decisions in here
are final unless you are explicitly told otherwise in the current prompt.

Read `SPEC.md` for the full specification. This file contains working instructions only.

---

## Language and Tooling

- **Language:** Go (latest stable)
- **Module path:** `github.com/nonchord/kcompass`
- **CLI framework:** `cobra`
- **YAML:** `gopkg.in/yaml.v3`
- **Git backend:** `go-git/go-git`
- **DNS / TXT lookups:** standard library `net` package only
- **Kubeconfig merge:** `k8s.io/client-go/tools/clientcmd`
- **Terminal detection:** `golang.org/x/term`
- **Tests:** standard library `testing` + `github.com/stretchr/testify`

Do not introduce dependencies without a clear reason. Prefer stdlib.

## Code Quality

- golangci-lint must be used.
- All linting issues must be resolved before completion.
- Code coverage target is 90%.

---

## Project Structure

```
cmd/
  kcompass/
    main.go           # entrypoint only, no logic
internal/
  backend/
    backend.go        # Backend interface, ClusterRecord, KubeconfigSpec
    registry.go       # merges backends, handles dedup and caching
    factory.go        # URL → Backend inference (NewBackendFromURL)
    git.go            # go-git backend
    local.go          # local YAML backend
    pathutil.go       # ~ expansion helper
  discovery/
    discovery.go      # parallel probe runner; txtBackend helper
    tailscale.go      # Tailscale daemon → MagicDNS suffix → TXT lookup
    netbird.go        # Netbird daemon → mgmt domain → TXT lookup
    dns.go            # /etc/resolv.conf search domains → TXT lookup
    network.go        # DetectNetworkDomains for `operator dns` real-hostname output
    cloud.go          # placeholder GCloud/AWS probe stubs (return nil, nil)
  kubeconfig/
    merge.go          # atomic kubeconfig merge with collision renaming
  cli/
    root.go           # root cobra command, --config / --verbose, registry injection
    list.go
    connect.go        # resolveKubeconfig: handles inline and command modes
    init.go           # writes ~/.kcompass/config.yaml
    backends.go
    operator.go       # `operator dns` and registration of operator subcommand
    operator_add.go   # `operator add` interactive + flag-driven inventory scaffolder
pkg/
  config/
    config.go         # reads ~/.kcompass/config.yaml
```

---

## Implementation Status

The original 8-phase plan has been superseded. Current state:

1. **Skeleton** — done
2. **Local backend** — done
3. **Git backend** — done
4. **kubeconfig merge** — done (`MergeStatic` handles both inline and
   command-mode output uniformly)
5. **Auto-discovery** — done (TXT records, not SRV; no cloud probes)
6. **`kcompass operator dns`** — done (publish/verify TXT records)
7. **`kcompass operator add`** — done (interactive + flag-driven inventory scaffolder)
8. **`KubeconfigSpec` refactor** — done (`auth` enum collapsed into
   `kubeconfig.inline` / `kubeconfig.command`; `Provider`/`Metadata` dropped)

**Explicitly not built and not planned:**
- GKE / EKS / AKS runtime backends. These were in the original plan but are
  superseded by `kubeconfig.command`. A GKE cluster is now an inventory record
  with `kubeconfig.command: [gcloud, container, clusters, get-credentials, ...]`.
  Inventory generation happens at operator time (Terraform output, `kcompass
  operator add`, or a one-shot script), not on every `kcompass list`.
- HTTP backend. Was in the original plan; dropped because it required a
  kcompass-specific server nobody has. Git covers all the same use cases.

---

## Invariants — Never Violate These

- **Backends are read-only.** No backend implementation may write to any external system.
- **kcompass never stores credentials.** Either the operator embeds a kubeconfig in the
  inventory record (`kubeconfig.inline`) or kcompass runs an operator-supplied command
  (`kubeconfig.command`) that writes to `KUBECONFIG`. kcompass only merges the result.
- **Discovery is best-effort.** A failed probe must never surface an error to the user
  unless `--verbose` is set. Log it via the supplied `Log` function and continue.
- **First backend wins on name collision.** Backends are tried in config order. If two
  backends return a cluster with the same name, the first result is used silently.
- **Inventory validation runs at parse time.** `readClusterFile` calls
  `ClusterRecord.Validate` on every record, so a malformed `kubeconfig:` block fails
  loudly during `kcompass list` rather than silently waiting for someone to try `connect`.
- **All network and subprocess calls respect context cancellation.** No goroutine may
  block indefinitely. Pass `cmd.Context()` through to `exec.CommandContext`, `net.Resolver`
  lookups, and any future I/O.

---

## Error Handling

- User-facing errors: plain English, no stack traces, actionable where possible.
- Internal errors: wrap with `fmt.Errorf("component: %w", err)` for context.
- `ErrNotFound` must be returned (not a generic error) when a cluster name doesn't exist,
  so the registry can handle it correctly across backends.

---

## Testing Expectations

- Every backend must have unit tests using fixtures or mocks. No test may make real network
  calls unless gated behind an opt-in env var.
- The discovery sequence must be fully testable without real Tailscale/Netbird/DNS — use
  injectable probe functions rather than hardcoded calls to system tools.
- The kubeconfig merge logic must be tested with fixture kubeconfig files covering:
  - Empty kubeconfig
  - Existing kubeconfig with no conflicts
  - Existing kubeconfig with a conflicting context name

---

## What Not To Build

- No Windows support
- No UI of any kind
- No cluster creation, deletion, or modification
- No credential storage — embed kubeconfigs at inventory time or delegate to a command
- No plugin system — backends are compiled in
- No runtime cloud-provider backends — express cloud clusters as `kubeconfig.command`
  records generated at operator time
- No HTTP backend — git covers the same use cases without requiring a custom server
