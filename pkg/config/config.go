// Package config handles loading and parsing the kcompass configuration file.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a time.Duration that unmarshals from YAML strings like "5m" or "500ms".
type Duration struct {
	time.Duration
}

// MarshalYAML implements yaml.Marshaler for Duration, writing the string
// representation (e.g. "5m0s") so the value roundtrips correctly.
func (d Duration) MarshalYAML() (interface{}, error) {
	return d.String(), nil
}

// UnmarshalYAML implements yaml.Unmarshaler for Duration.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = dur
	return nil
}

// BackendConfig holds the configuration for a single backend instance.
// Type-specific fields are captured inline so config.go never needs to
// change when new backends are added.
type BackendConfig struct {
	Type    string                 `yaml:"type"`
	Options map[string]interface{} `yaml:",inline"`
}

// CacheConfig holds cache settings.
type CacheConfig struct {
	TTL  Duration `yaml:"ttl"`
	Path string   `yaml:"path"`
}

// DiscoveryConfig holds auto-discovery settings.
type DiscoveryConfig struct {
	// Enabled controls auto-discovery when no backends are configured.
	// Nil (absent from config) means enabled; false explicitly disables it.
	Enabled *bool    `yaml:"enabled,omitempty"`
	Timeout Duration `yaml:"timeout,omitempty"`
}

// Config is the top-level kcompass configuration.
type Config struct {
	Backends  []BackendConfig `yaml:"backends"`
	Cache     CacheConfig     `yaml:"cache"`
	Discovery DiscoveryConfig `yaml:"discovery"`
}

// DefaultPath returns the default config file path (~/.kcompass/config.yaml).
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config: resolve home dir: %w", err)
	}
	return filepath.Join(home, ".kcompass", "config.yaml"), nil
}

// Load reads and parses the config file at path. Returns an error wrapping
// os.ErrNotExist if the file is absent; callers may choose to treat that as
// a benign "no config yet" condition.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return &cfg, nil
}
