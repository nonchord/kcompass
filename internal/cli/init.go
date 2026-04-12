package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/nonchord/kcompass/internal/backend"
	"github.com/nonchord/kcompass/internal/discovery"
	"github.com/nonchord/kcompass/pkg/config"
)

// lookupTXT is swappable in tests.
var lookupTXT = net.DefaultResolver.LookupTXT

// NewInitCommand creates the `kcompass init` command.
func NewInitCommand() *cobra.Command {
	var skipVerify bool
	cmd := &cobra.Command{
		Use:   "init <url-or-path-or-zone>",
		Short: "Register a backend",
		Long: `Register a backend by URL, local file path, or DNS zone.

If the argument looks like a DNS zone (e.g. "example.com"), kcompass will look
up the TXT record at kcompass.<zone> and register the resolved backend URL.
This is the same record format the auto-discovery probes consume, but triggered
explicitly — useful when your machine's resolver isn't configured for the zone
yet, or when you want to bootstrap against an organization you don't yet share
a network with.

By default, init verifies it can actually read the backend before writing it
to the config. This catches common mistakes like misspelled paths or private
repositories you haven't been granted access to, so you find out immediately
instead of on the next kcompass list. Pass --skip-verify to bypass (e.g. when
pre-configuring a machine before joining the network).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]
			displayTarget := target

			// If the argument looks like a zone name, try resolving via TXT
			// before falling through to URL/path inference. On success, the
			// resolved URL is what we actually register.
			if looksLikeDNSZone(target) {
				if resolved, ok := resolveZoneToBackendURL(cmd.Context(), target); ok {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(),
						"Resolved kcompass.%s → %s\n", target, resolved)
					target = resolved
				}
			}

			// Verify the target is actually reachable and parseable before we
			// persist it. This prevents silently registering a backend that
			// will fail on every subsequent `kcompass list`.
			if !skipVerify {
				if err := verifyBackend(cmd, target); err != nil {
					return err
				}
			}

			cfgPath, _ := cmd.Root().PersistentFlags().GetString("config")
			if cfgPath == "" {
				var err error
				cfgPath, err = config.DefaultPath()
				if err != nil {
					return err
				}
			}

			cfg, err := config.Load(cfgPath)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if cfg == nil {
				cfg = &config.Config{}
			}

			entry := inferBackendConfig(target)
			cfg.Backends = append(cfg.Backends, entry)

			if err := writeConfig(cfgPath, cfg); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Backend registered: %s\n", target)
			if displayTarget != target {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(),
					"(via kcompass.%s TXT record)\n", displayTarget)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"To advertise via DNS auto-discovery: kcompass operator dns %s\n", target)
			return nil
		},
	}
	cmd.Flags().BoolVar(&skipVerify, "skip-verify", false,
		"register the backend without verifying it can be read (for pre-configuring a machine before it can reach the backend)")
	return cmd
}

// verifyBackend constructs a backend from the target string and calls List
// once, so init can fail loudly when the backend is unreachable, misspelled,
// or inaccessible instead of silently writing a broken entry to config.
// On backend.ErrAccessDenied the friendly message is printed to stderr;
// other errors are classified by classifyInitError to surface actionable
// hints ("did you mean a DNS zone?", "no file at X", etc.) rather than a
// generic wrap.
func verifyBackend(cmd *cobra.Command, target string) error {
	b, err := backend.NewBackendFromURL(target)
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}
	if _, err := b.List(cmd.Context()); err != nil {
		if errors.Is(err, backend.ErrAccessDenied) {
			printAccessDenied(cmd)
			return err
		}
		cmd.SilenceUsage = true
		return classifyInitError(target, err)
	}
	return nil
}

// classifyInitError turns a raw backend.List error into a user-facing
// message that says what went wrong and — where possible — hints at how
// to recover. The most common confusion: a user types `kcompass init
// nonchord.com` on a machine that can't TXT-resolve the zone, kcompass
// falls through to treating it as a local path, and the user gets a
// cryptic stat error. This function catches that case and explains both
// possibilities.
func classifyInitError(target string, err error) error {
	if errors.Is(err, os.ErrNotExist) {
		if looksLikeDNSZone(target) {
			return fmt.Errorf(
				"init: %q looks like a DNS zone, but no TXT record was found at kcompass.%s, "+
					"and no file exists at that path either.\n"+
					"  - To use DNS discovery, ensure a `v=kc1; backend=<url>` record is published at kcompass.%s.\n"+
					"  - To register a git backend directly, include the URL scheme (`https://`, `git@`, etc).\n"+
					"  - To register a local file, use an explicit path like `./%s` or an absolute path",
				target, target, target, target)
		}
		return fmt.Errorf("init: no file or directory at %q", target)
	}
	if errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("init: permission denied reading %q — check the file permissions", target)
	}
	return fmt.Errorf("init: cannot access %s: %w", target, err)
}

// looksLikeDNSZone reports whether target is plausibly a DNS zone (as opposed
// to a URL or a file path). The heuristic is conservative: it must contain a
// dot, no path separators, no leading dot/slash/tilde, and no URL scheme.
// Ambiguous strings like "nonchord.com" are resolved as zones first; on TXT
// miss, init falls back to treating them as local paths (current behavior).
func looksLikeDNSZone(target string) bool {
	if target == "" {
		return false
	}
	if strings.ContainsAny(target, `/\:`) {
		return false
	}
	if strings.HasPrefix(target, ".") || strings.HasPrefix(target, "~") {
		return false
	}
	if !strings.Contains(target, ".") {
		return false
	}
	// URL schemes are handled by inferBackendConfig, never by TXT lookup.
	for _, scheme := range []string{"https://", "http://", "git@", "git://", "ssh://"} {
		if strings.HasPrefix(target, scheme) {
			return false
		}
	}
	return true
}

// resolveZoneToBackendURL performs a TXT lookup at kcompass.<zone> and returns
// the first valid "v=kc1; backend=<url>" value found. Returns (_, false) on
// any lookup error or when no matching record exists.
func resolveZoneToBackendURL(ctx context.Context, zone string) (string, bool) {
	txts, err := lookupTXT(ctx, "kcompass."+zone)
	if err != nil {
		return "", false
	}
	for _, txt := range txts {
		if url, ok := discovery.ParseTXTRecord(txt); ok {
			return url, true
		}
	}
	return "", false
}

// inferBackendConfig picks a backend type from the argument string.
// All URL schemes (HTTPS, SSH, git://) map to the git backend; plain paths map to local.
func inferBackendConfig(target string) config.BackendConfig {
	switch {
	case strings.HasPrefix(target, "https://"),
		strings.HasPrefix(target, "http://"),
		strings.HasPrefix(target, "git@"),
		strings.HasPrefix(target, "git://"),
		strings.HasPrefix(target, "ssh://"):
		return config.BackendConfig{Type: "git", Options: map[string]interface{}{"url": target}}
	default:
		return config.BackendConfig{Type: "local", Options: map[string]interface{}{"path": target}}
	}
}

// writeConfig serialises cfg to path, creating parent directories as needed.
func writeConfig(path string, cfg *config.Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("init: create config dir: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("init: marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("init: write config: %w", err)
	}
	return nil
}
