package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/nonchord/kcompass/pkg/config"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "config*.yaml")
	require.NoError(t, err)
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

func TestLoadEmptyFile(t *testing.T) {
	path := writeTempConfig(t, "")
	cfg, err := config.Load(path)
	require.NoError(t, err)
	assert.Empty(t, cfg.Backends)
}

func TestLoadFullConfig(t *testing.T) {
	content := `
backends:
  - type: gke
    project: my-project
  - type: local
    path: ~/.kcompass/local.yaml
cache:
  ttl: 5m
  path: ~/.kcompass/cache/
discovery:
  enabled: true
  timeout: 500ms
`
	path := writeTempConfig(t, content)
	cfg, err := config.Load(path)
	require.NoError(t, err)
	require.Len(t, cfg.Backends, 2)
	assert.Equal(t, "gke", cfg.Backends[0].Type)
	assert.Equal(t, "local", cfg.Backends[1].Type)
	assert.Equal(t, 5*time.Minute, cfg.Cache.TTL.Duration)
	assert.Equal(t, 500*time.Millisecond, cfg.Discovery.Timeout.Duration)
	assert.True(t, cfg.Discovery.Enabled)
}

func TestLoadMissingFile(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, os.ErrNotExist))
}

func TestDefaultPath(t *testing.T) {
	path, err := config.DefaultPath()
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(path, filepath.Join(".kcompass", "config.yaml")))
}

func TestDurationUnmarshal(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"5m", 5 * time.Minute, false},
		{"500ms", 500 * time.Millisecond, false},
		{"1h30m", 90 * time.Minute, false},
		{"invalid", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var d config.Duration
			err := yaml.Unmarshal([]byte(tt.input), &d)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, d.Duration)
			}
		})
	}
}
