package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/nonchord/kcompass/pkg/config"
)

// NewInitCommand creates the `kcompass init` command.
func NewInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init <url-or-path>",
		Short: "Register a backend",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]

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
			return nil
		},
	}
}

// inferBackendConfig picks a backend type from the argument string.
func inferBackendConfig(target string) config.BackendConfig {
	switch {
	case strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://"):
		return config.BackendConfig{Type: "http", Options: map[string]interface{}{"url": target}}
	case strings.HasPrefix(target, "git@") || strings.HasPrefix(target, "git://") || strings.HasPrefix(target, "ssh://"):
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
