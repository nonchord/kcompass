# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.0] - 2026-04-13

### Changed

- **Git backend uses system `git` instead of go-git.** Clone and fetch now shell
  out to the `git` binary, inheriting the user's full SSH config (`~/.ssh/config`
  host aliases, agent forwarding, `IdentityFile` directives), credential helpers,
  and all other git configuration. This fixes silent failures on machines without
  `ssh-agent` running and resolves compatibility issues with newer SSH key formats
  (see [go-git/go-git#1673](https://github.com/go-git/go-git/issues/1673)).
- `GIT_TOKEN` for HTTPS backends still works; it is now embedded in the clone URL
  (stored only in kcompass's private cache directory).
- `GIT_TERMINAL_PROMPT=0` is set on all git operations to prevent interactive
  credential prompts from hanging the process.

### Improved

- **Registry tolerates partial backend failures.** If one backend is unreachable
  (e.g. a git repo you can't clone), kcompass now logs a warning and continues
  with the remaining backends instead of failing the entire `kcompass list`.
  The error is surfaced with `--verbose`. If all backends fail, the combined
  errors are returned.
- **Better auth error messages.** The "access denied" message now suggests
  running `git clone <url>` to verify credentials directly, and mentions
  `~/.ssh/config` and credential helpers instead of only SSH keys.

### Removed

- `go-git/go-git` dependency and its ~15 transitive dependencies. The `git`
  binary is now required at runtime.

## [0.1.0] - 2026-04-12

Initial public release.

### Added

- `kcompass list` — list all clusters across configured backends, with `--json` and `--backend` filtering.
- `kcompass connect <name>` — resolve credentials and merge into kubeconfig. Idempotent on repeat runs; preserves the user's per-context namespace across re-merges.
- `kcompass init <url-or-path-or-zone>` — register a backend by URL, local file path, or DNS zone. Zone mode resolves the TXT record at `kcompass.<zone>`. Verifies the backend is reachable before writing to config (`--skip-verify` to bypass).
- `kcompass backends` — list configured backends and their status.
- `kcompass operator dns <url>` — print DNS TXT records for auto-discovery. `--verify` checks detected domains; `--hostname <fqdn>` verifies specific FQDNs.
- `kcompass operator add` — scaffold a cluster entry into an inventory file, interactive or flag-driven.
- Auto-discovery via Tailscale, Netbird, and DNS search domain TXT probes — `kcompass list` works with zero configuration when your network publishes a `v=kc1; backend=<url>` record.
- Git backend — clone/fetch a repository over SSH or HTTPS, scan for `*.yaml` inventory files.
- Local YAML backend — read cluster records from a single file on disk.
- Terraform helper modules (`terraform/modules/`): `kcompass_inventory` (render inventory YAML) and `kcompass_txt` (render TXT record value). Both are pure value transforms with no provider dependency.

[Unreleased]: https://github.com/nonchord/kcompass/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/nonchord/kcompass/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/nonchord/kcompass/releases/tag/v0.1.0
