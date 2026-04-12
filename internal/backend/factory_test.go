package backend_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nonchord/kcompass/internal/backend"
)

// TestNewBackendFromURL pins the URL-scheme → backend-type inference that the
// CLI `init` command and the discovery probes share. The shape of the
// returned value (GitBackend vs LocalBackend) matters because both the init
// config entry and the runtime backend walk depend on it.
func TestNewBackendFromURL(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantGit bool
	}{
		{"https", "https://github.com/org/clusters", true},
		{"http", "http://example.com/clusters.git", true},
		{"git+ssh-shorthand", "git@github.com:org/clusters", true},
		{"git-protocol", "git://github.com/org/clusters", true},
		{"ssh-scheme", "ssh://git@github.com/org/clusters", true},
		{"absolute-path", "/home/user/clusters.yaml", false},
		{"relative-path", "./clusters.yaml", false},
		{"bare-filename", "clusters.yaml", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := backend.NewBackendFromURL(c.url)
			require.NoError(t, err)
			require.NotNil(t, b)
			if c.wantGit {
				_, ok := b.(*backend.GitBackend)
				assert.True(t, ok, "expected *GitBackend for %q, got %T", c.url, b)
				// Git backends are named with a "git:" provenance prefix.
				assert.Contains(t, b.Name(), "git:")
			} else {
				_, ok := b.(*backend.LocalBackend)
				assert.True(t, ok, "expected *LocalBackend for %q, got %T", c.url, b)
				assert.Contains(t, b.Name(), "local:")
			}
		})
	}
}

// TestNewNamedBackendFromURL verifies the explicit-name variant used by the
// discovery probes (where the name encodes provenance, e.g.
// "tailscale:git@github.com:company/clusters"). The name must survive
// construction verbatim.
func TestNewNamedBackendFromURL(t *testing.T) {
	name := "tailscale:git@github.com:company/clusters"
	b, err := backend.NewNamedBackendFromURL(name, "git@github.com:company/clusters")
	require.NoError(t, err)
	assert.Equal(t, name, b.Name())
	_, ok := b.(*backend.GitBackend)
	assert.True(t, ok)
}

// TestNewNamedBackendFromURLLocal verifies the local-path path through the
// same named constructor.
func TestNewNamedBackendFromURLLocal(t *testing.T) {
	b, err := backend.NewNamedBackendFromURL("local:test", "/tmp/test.yaml")
	require.NoError(t, err)
	assert.Equal(t, "local:test", b.Name())
	_, ok := b.(*backend.LocalBackend)
	assert.True(t, ok)
}
