package cli_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nonchord/kcompass/pkg/config"
)

func runInit(t *testing.T, cfgPath string, args ...string) {
	t.Helper()
	_, err := executeWithConfig(t, cfgPath, append([]string{"init"}, args...)...)
	require.NoError(t, err)
}

func loadConfig(t *testing.T, path string) *config.Config {
	t.Helper()
	cfg, err := config.Load(path)
	require.NoError(t, err)
	return cfg
}

func TestInitHTTPS(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	runInit(t, cfgPath, "https://github.com/org/clusters")
	cfg := loadConfig(t, cfgPath)
	require.Len(t, cfg.Backends, 1)
	// https:// is a git backend — there is no HTTP backend type
	assert.Equal(t, "git", cfg.Backends[0].Type)
	assert.Equal(t, "https://github.com/org/clusters", cfg.Backends[0].Options["url"])
}

func TestInitGitSSH(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	runInit(t, cfgPath, "git@github.com:org/clusters")
	cfg := loadConfig(t, cfgPath)
	require.Len(t, cfg.Backends, 1)
	assert.Equal(t, "git", cfg.Backends[0].Type)
	assert.Equal(t, "git@github.com:org/clusters", cfg.Backends[0].Options["url"])
}

func TestInitGitScheme(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	runInit(t, cfgPath, "git://github.com/org/clusters")
	cfg := loadConfig(t, cfgPath)
	require.Len(t, cfg.Backends, 1)
	assert.Equal(t, "git", cfg.Backends[0].Type)
}

func TestInitLocal(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	runInit(t, cfgPath, "/home/user/clusters.yaml")
	cfg := loadConfig(t, cfgPath)
	require.Len(t, cfg.Backends, 1)
	assert.Equal(t, "local", cfg.Backends[0].Type)
	assert.Equal(t, "/home/user/clusters.yaml", cfg.Backends[0].Options["path"])
}

func TestInitAppendsToExisting(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	runInit(t, cfgPath, "/path/one.yaml")
	runInit(t, cfgPath, "git@github.com:org/clusters")
	cfg := loadConfig(t, cfgPath)
	assert.Len(t, cfg.Backends, 2)
}

func TestInitCreatesConfigDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "subdir", "nested")
	cfgPath := filepath.Join(dir, "config.yaml")
	runInit(t, cfgPath, "/path/clusters.yaml")
	_, err := os.Stat(cfgPath)
	assert.NoError(t, err, "config file should have been created")
}

func TestInitOutput(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	out, err := executeWithConfig(t, cfgPath, "init", "https://example.com/clusters")
	require.NoError(t, err)
	assert.Contains(t, out, "Backend registered")
	assert.Contains(t, out, "https://example.com/clusters")
}

func TestInitPreservesExistingFields(t *testing.T) {
	// Write a config with cache settings; init should not clobber them.
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	initial := `cache:
  ttl: 10m
  path: /tmp/cache
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(initial), 0o600))
	runInit(t, cfgPath, "/path/clusters.yaml")

	cfg := loadConfig(t, cfgPath)
	assert.Equal(t, 10*time.Minute, cfg.Cache.TTL.Duration, "cache TTL should be preserved")
	assert.Equal(t, "/tmp/cache", cfg.Cache.Path, "cache path should be preserved")
}
