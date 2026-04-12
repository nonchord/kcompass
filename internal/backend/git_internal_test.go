package backend

import (
	"os"
	"path/filepath"
	"testing"

	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGitAuthMethodHTTPS covers the HTTPS auth branch of authMethod:
// when GIT_TOKEN is set, a BasicAuth is returned; when unset, nil is
// returned (anonymous fetch). The test isolates GIT_TOKEN via t.Setenv
// so it doesn't pollute the environment for parallel tests.
func TestGitAuthMethodHTTPS(t *testing.T) {
	b := &GitBackend{url: "https://github.com/org/clusters"}

	t.Run("with GIT_TOKEN", func(t *testing.T) {
		t.Setenv("GIT_TOKEN", "gha-secret")
		auth, err := b.authMethod()
		require.NoError(t, err)
		require.NotNil(t, auth)
		basic, ok := auth.(*githttp.BasicAuth)
		require.True(t, ok, "expected *githttp.BasicAuth, got %T", auth)
		assert.Equal(t, "git", basic.Username)
		assert.Equal(t, "gha-secret", basic.Password)
	})

	t.Run("without GIT_TOKEN", func(t *testing.T) {
		t.Setenv("GIT_TOKEN", "")
		auth, err := b.authMethod()
		require.NoError(t, err)
		assert.Nil(t, auth, "anonymous fetch when no token is set")
	})
}

// TestGitAuthMethodHTTP exercises the plain HTTP branch (same GIT_TOKEN
// handling as HTTPS). HTTP is unusual in production but valid per the auth
// switch in authMethod.
func TestGitAuthMethodHTTP(t *testing.T) {
	b := &GitBackend{url: "http://internal.example/clusters"}
	t.Setenv("GIT_TOKEN", "tok")
	auth, err := b.authMethod()
	require.NoError(t, err)
	basic, ok := auth.(*githttp.BasicAuth)
	require.True(t, ok)
	assert.Equal(t, "tok", basic.Password)
}

// TestGitAuthMethodDefault covers the default branch — any URL that isn't
// HTTPS, HTTP, git@, git://, or ssh:// returns nil auth. file:// URLs
// (used in test fixtures) hit this path.
func TestGitAuthMethodDefault(t *testing.T) {
	b := &GitBackend{url: "file:///tmp/remote.git"}
	auth, err := b.authMethod()
	require.NoError(t, err)
	assert.Nil(t, auth, "file:// URLs need no auth")
}

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
// file (e.g. partial write, filesystem corruption) must count as "expired"
// so the next List triggers a fresh fetch instead of trusting invalid data.
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
	// Cache dir should start with the user's home directory, not a literal "~".
	cloneDir := b.cloneDir()
	assert.NotContains(t, cloneDir, "~",
		"~-prefixed cache dirs must be expanded before use")
}
