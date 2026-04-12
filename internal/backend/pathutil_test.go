package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExpandPath covers the three happy-path branches of expandPath: bare ~,
// ~/subpath, and paths without ~ (no-op). This is the in-package test so we
// can exercise the unexported helper directly without an API entry point.
func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err, "test requires a resolvable user home directory")

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"bare ~", "~", home},
		{"~ with subpath", "~/clusters.yaml", filepath.Join(home, "clusters.yaml")},
		{"~ with nested subpath", "~/.kcompass/config.yaml", filepath.Join(home, ".kcompass/config.yaml")},
		{"absolute path unchanged", "/etc/kcompass/clusters.yaml", "/etc/kcompass/clusters.yaml"},
		{"relative path unchanged", "./clusters.yaml", "./clusters.yaml"},
		{"empty path unchanged", "", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := expandPath(c.in)
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}
