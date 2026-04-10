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
	"github.com/nonchord/kcompass/pkg/config"
)

// RegistryKey is the context key used to pass the backend registry to subcommands.
// It is exported so tests can inject a registry without going through config loading.
type RegistryKey struct{}

// NewRootCommand builds the root kcompass command with all subcommands registered.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:          "kcompass",
		Short:        "Discover and connect to Kubernetes clusters",
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// Skip building if a registry was injected (e.g. by tests).
			if cmd.Context().Value(RegistryKey{}) != nil {
				return nil
			}
			reg, err := buildRegistry()
			if err != nil {
				return err
			}
			ctx := context.WithValue(cmd.Context(), RegistryKey{}, reg)
			cmd.SetContext(ctx)
			return nil
		},
	}
	root.AddCommand(
		NewListCommand(),
		NewConnectCommand(),
		NewInitCommand(),
		NewBackendsCommand(),
	)
	return root
}

// buildRegistry loads config and constructs the backend registry.
func buildRegistry() (*backend.Registry, error) {
	cfgPath, err := config.DefaultPath()
	if err != nil {
		return nil, err
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

	return backend.NewRegistry(backends, ttl), nil
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
