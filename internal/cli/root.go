// Package cli contains the cobra command definitions for kcompass.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/nonchord/kcompass/internal/backend"
	"github.com/nonchord/kcompass/internal/discovery"
	"github.com/nonchord/kcompass/pkg/config"
)

// RegistryKey is the context key used to pass the backend registry to subcommands.
// It is exported so tests can inject a registry without going through config loading.
type RegistryKey struct{}

// NewRootCommand builds the root kcompass command with all subcommands registered.
func NewRootCommand() *cobra.Command {
	var cfgPath string

	root := &cobra.Command{
		Use:          "kcompass",
		Short:        "Discover and connect to Kubernetes clusters",
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// Skip building if a registry was injected (e.g. by tests).
			if cmd.Context().Value(RegistryKey{}) != nil {
				return nil
			}
			reg, err := buildRegistry(cmd.Context(), cfgPath)
			if err != nil {
				return err
			}
			ctx := context.WithValue(cmd.Context(), RegistryKey{}, reg)
			cmd.SetContext(ctx)
			return nil
		},
	}
	root.PersistentFlags().StringVar(&cfgPath, "config", "", "path to config file (default: ~/.kcompass/config.yaml)")
	root.AddCommand(
		NewListCommand(),
		NewConnectCommand(),
		NewInitCommand(),
		NewBackendsCommand(),
	)
	return root
}

// buildRegistry loads config and constructs the backend registry.
// When no backends are configured and discovery is not disabled, auto-discovery
// is attempted with the configured (or default) timeout.
func buildRegistry(ctx context.Context, cfgPath string) (*backend.Registry, error) {
	if cfgPath == "" {
		var err error
		cfgPath, err = config.DefaultPath()
		if err != nil {
			return nil, err
		}
	}

	cfg, err := config.Load(cfgPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if cfg == nil {
		cfg = &config.Config{}
	}

	ttl := cfg.Cache.TTL.Duration
	if ttl == 0 {
		ttl = 5 * time.Minute
	}

	var backends []backend.Backend
	for _, bc := range cfg.Backends {
		b, buildErr := buildBackend(bc)
		if buildErr != nil {
			return nil, buildErr
		}
		backends = append(backends, b)
	}

	// When no explicit backends are configured, attempt auto-discovery unless
	// the user has explicitly disabled it.
	if len(backends) == 0 && discoveryEnabled(cfg) {
		timeout := cfg.Discovery.Timeout.Duration
		if timeout == 0 {
			timeout = 500 * time.Millisecond
		}
		backends = discovery.Run(ctx, discovery.DefaultProbes(), timeout)
	}

	return backend.NewRegistry(backends, ttl), nil
}

// discoveryEnabled reports whether auto-discovery should run.
// Discovery is enabled by default (nil Enabled field); set Enabled: false to disable.
func discoveryEnabled(cfg *config.Config) bool {
	return cfg.Discovery.Enabled == nil || *cfg.Discovery.Enabled
}

// buildBackend constructs a single Backend from a BackendConfig.
func buildBackend(bc config.BackendConfig) (backend.Backend, error) {
	switch bc.Type {
	case "local":
		path, _ := bc.Options["path"].(string)
		if path == "" {
			return nil, fmt.Errorf("local backend: missing required field 'path'")
		}
		return backend.NewLocalBackend("local:"+path, path)
	case "git":
		url, _ := bc.Options["url"].(string)
		if url == "" {
			return nil, fmt.Errorf("git backend: missing required field 'url'")
		}
		repoPath, _ := bc.Options["path"].(string)
		ref, _ := bc.Options["ref"].(string)
		return backend.NewGitBackend(backend.GitBackendConfig{
			Name:     "git:" + url,
			URL:      url,
			RepoPath: repoPath,
			Ref:      ref,
		})
	case "http":
		url, _ := bc.Options["url"].(string)
		if url == "" {
			return nil, fmt.Errorf("http backend: missing required field 'url'")
		}
		tokenEnv, _ := bc.Options["token_env"].(string)
		return backend.NewHTTPBackend(backend.HTTPBackendConfig{
			Name:     "http:" + url,
			URL:      url,
			TokenEnv: tokenEnv,
		}), nil
	default:
		return nil, fmt.Errorf("unknown backend type %q", bc.Type)
	}
}

// defaultKubeconfigPath returns the path to the user's kubeconfig file,
// honouring the KUBECONFIG env var if set.
func defaultKubeconfigPath() (string, error) {
	if kc := os.Getenv("KUBECONFIG"); kc != "" {
		return kc, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".kube", "config"), nil
}
