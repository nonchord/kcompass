package backend

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/stretchr/testify/assert"
)

func TestIsAuthError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"transport.ErrAuthenticationRequired wrapped", fmt.Errorf("x: %w", transport.ErrAuthenticationRequired), true},
		{"transport.ErrAuthorizationFailed wrapped", fmt.Errorf("x: %w", transport.ErrAuthorizationFailed), true},
		{"transport.ErrRepositoryNotFound wrapped", fmt.Errorf("x: %w", transport.ErrRepositoryNotFound), true},
		{"ssh handshake permission denied", errors.New("ssh: handshake failed: Permission denied (publickey)"), true},
		{"https repo not found", errors.New("authentication required"), true},
		{"substring: unable to authenticate", errors.New("ssh: unable to authenticate, attempted methods [none publickey]"), true},
		{"unrelated network error", errors.New("dial tcp: connection refused"), false},
		{"not found but unrelated", errors.New("file not found"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, isAuthError(c.err))
		})
	}
}

// TestAccessDeniedWrappingChain verifies that the single-%w wrap used by
// cloneRepo and fetchRepo makes ErrAccessDenied reachable via errors.Is,
// while the underlying transport error is present in the message text
// (via %v) for debug purposes but NOT in the error chain. This matches
// how the CLI consumes the error: printAccessDenied checks for
// ErrAccessDenied only, and the raw form reads cleanly without two
// colon-joined error phrases.
func TestAccessDeniedWrappingChain(t *testing.T) {
	original := transport.ErrAuthenticationRequired
	wrapped := fmt.Errorf("clone https://example/r: %w (%v)", ErrAccessDenied, original)
	assert.True(t, errors.Is(wrapped, ErrAccessDenied),
		"errors.Is must find ErrAccessDenied in the chain")
	assert.False(t, errors.Is(wrapped, original),
		"the underlying transport error is in the message text, not the chain")
	assert.Contains(t, wrapped.Error(), original.Error(),
		"the underlying error's message should still appear for debug")
}
