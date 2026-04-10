# kcompass — Claude Code Instructions

This file is read automatically by Claude Code on every invocation. All decisions in here
are final unless you are explicitly told otherwise in the current prompt.

Read `SPEC.md` for the full specification. This file contains working instructions only.

---

## Language and Tooling

- **Language:** Go (latest stable)
- **Module path:** `github.com/mdwn/kcompass`
- **CLI framework:** `cobra`
- **YAML:** `gopkg.in/yaml.v3`
- **Git backend:** `go-git/go-git`
- **DNS / SRV:** standard library `net` package only
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
    backend.go        # Backend interface and ErrNotFound
    registry.go       # merges backends, handles dedup and caching
    gke.go
    eks.go
    git.go
    http.go
    local.go
  discovery/
    discovery.go      # auto-discovery sequence, returns []Backend
    tailscale.go
    netbird.go
    dns.go
    cloud.go
  kubeconfig/
    merge.go          # kubeconfig merge and context switching
  cli/
    list.go
    connect.go
    init.go
    backends.go
pkg/
  config/
    config.go         # reads ~/.kcompass/config.yaml
```

---

## Implementation Order

Always implement in this order. Do not skip ahead.

1. **Skeleton** — CLI entrypoints (cobra commands), Backend interface, ClusterRecord struct,
   config loading. Commands should print "not implemented" stubs. Tests for config parsing.

2. **Local backend** — simplest backend, no network. Validate the full list→connect flow
   end-to-end with a local fixture file before touching any other backend.

3. **GKE backend** — uses `google.golang.org/api/container/v1`. Requires `gcloud` auth
   in the environment. Integration test is opt-in via `TEST_GKE=1` env var.

4. **EKS backend** — uses `github.com/aws/aws-sdk-go-v2/service/eks`. Integration test
   is opt-in via `TEST_EKS=1`.

5. **Git backend** — uses `go-git`. Clone to cache dir, parse cluster files. Test with
   a local bare repo fixture.

6. **HTTP backend** — straightforward. Test with `httptest.NewServer`.

7. **Auto-discovery** — implement last, after all backends exist. Each probe is a function
   returning `(Backend, error)`. Run probes in parallel with `errgroup` and 500ms context
   deadline.

8. **kubeconfig merge** — implement alongside connect command in step 2, but only the
   static/local auth path. Add gcloud/aws auth execution in steps 3 and 4 respectively.

---

## Invariants — Never Violate These

- **Backends are read-only.** No backend implementation may write to any external system.
- **kcompass never stores credentials.** It invokes the appropriate auth tool (gcloud, aws)
  and lets that tool write to kubeconfig. kcompass only merges the result.
- **Discovery is best-effort.** A failed probe must never surface an error to the user.
  Log it at debug level and continue.
- **First backend wins on name collision.** Backends are tried in config order. If two
  backends return a cluster with the same name, the first result is used silently.
- **All network calls respect context cancellation.** No goroutine may block indefinitely.

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
- No credential storage — delegate entirely to gcloud/aws/OIDC tooling
- No plugin system — backends are compiled in
