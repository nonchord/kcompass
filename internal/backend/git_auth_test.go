package backend

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsGitAuthError(t *testing.T) {
	cases := []struct {
		name   string
		stderr string
		want   bool
	}{
		{"empty", "", false},
		{"permission denied", "fatal: Could not read from remote repository.\n\nPlease make sure you have the correct access rights\nand the repository exists.", true},
		{"Permission denied publickey", "git@github.com: Permission denied (publickey).", true},
		{"repository not found", "ERROR: Repository not found.\nfatal: Could not read from remote repository.", true},
		{"authentication required", "fatal: authentication required", true},
		{"unable to authenticate", "ssh: unable to authenticate, attempted methods [none publickey]", true},
		{"terminal prompts disabled", "fatal: could not read Username for 'https://github.com': terminal prompts disabled", true},
		{"invalid credentials", "fatal: invalid credentials", true},
		{"could not read username", "fatal: could not read Username for 'https://github.com': No such device or address", true},
		{"unrelated network error", "fatal: unable to access: Could not resolve host: github.com", false},
		{"unrelated error", "error: pathspec 'foo' did not match any file(s) known to git", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, isGitAuthError(c.stderr))
		})
	}
}

func TestFirstLine(t *testing.T) {
	assert.Equal(t, "first", firstLine("first\nsecond\nthird"))
	assert.Equal(t, "only", firstLine("only"))
	assert.Equal(t, "", firstLine(""))
}

// TestEffectiveURLEmbedsGitToken verifies that GIT_TOKEN is embedded into
// HTTPS URLs and left alone for other URL schemes.
func TestEffectiveURLEmbedsGitToken(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{"https", "https://github.com/org/repo", "https://git:tok@github.com/org/repo"},
		{"http", "http://internal.example/repo", "http://git:tok@internal.example/repo"},
		{"ssh unchanged", "git@github.com:org/repo.git", "git@github.com:org/repo.git"},
		{"file unchanged", "file:///tmp/repo.git", "file:///tmp/repo.git"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("GIT_TOKEN", "tok")
			b := &GitBackend{url: c.url}
			assert.Equal(t, c.want, b.effectiveURL())
		})
	}

	t.Run("no token", func(t *testing.T) {
		t.Setenv("GIT_TOKEN", "")
		b := &GitBackend{url: "https://github.com/org/repo"}
		assert.Equal(t, "https://github.com/org/repo", b.effectiveURL())
	})
}

// TestAccessDeniedWrappingChain verifies that ErrAccessDenied is reachable
// via errors.Is in the wrapped error form used by cloneRepo and fetchRepo.
func TestAccessDeniedWrappingChain(t *testing.T) {
	wrapped := fmt.Errorf("clone https://example/r: %w (some git error)", ErrAccessDenied)
	assert.True(t, errors.Is(wrapped, ErrAccessDenied),
		"errors.Is must find ErrAccessDenied in the chain")
}
