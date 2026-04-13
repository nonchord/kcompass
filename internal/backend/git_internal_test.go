package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewGitBackendCustomCacheDir verifies that a caller-supplied CacheDir
// is honored (and ~-expanded) instead of the default under ~/.kcompass.
func TestNewGitBackendCustomCacheDir(t *testing.T) {
	dir := t.TempDir()
	b, err := NewGitBackend(GitBackendConfig{
		Name:     "g",
		URL:      "https://example/repo",
		CacheDir: dir,
	})
	require.NoError(t, err)
	// cloneDir is a hash under the cache root, so assert it's inside dir.
	cloneDir := b.cloneDir()
	assert.Equal(t, dir, filepath.Dir(cloneDir),
		"clone dir should be an immediate child of the configured cache dir")
}

// TestFetchExpiredMalformedTimestamp covers the "timestamp file exists but
// doesn't parse" branch of fetchExpired. A corrupted .kcompass-last-fetch
// file must count as "expired" so the next List triggers a fresh fetch.
func TestFetchExpiredMalformedTimestamp(t *testing.T) {
	cloneDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(cloneDir, lastFetchFile),
		[]byte("this is not a valid RFC3339 timestamp"),
		0o600,
	))

	b := &GitBackend{fetchTTL: 1 * 1e9} // 1 second
	assert.True(t, b.fetchExpired(cloneDir),
		"unparseable timestamp must be treated as expired")
}

// TestNewGitBackendTildeExpansion exercises the ~-expansion path in
// NewGitBackend's cache dir handling.
func TestNewGitBackendTildeExpansion(t *testing.T) {
	b, err := NewGitBackend(GitBackendConfig{
		Name:     "g",
		URL:      "https://example/repo",
		CacheDir: "~/.custom-kcompass-cache",
	})
	require.NoError(t, err)
	cloneDir := b.cloneDir()
	assert.NotContains(t, cloneDir, "~",
		"~-prefixed cache dirs must be expanded before use")
}
