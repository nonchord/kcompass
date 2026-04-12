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

// TestAccessDeniedWrappingChain verifies that fmt.Errorf with two %w verbs
// correctly chains both ErrAccessDenied and the original error, so that
// errors.Is walks the chain and finds the sentinel.
func TestAccessDeniedWrappingChain(t *testing.T) {
	original := transport.ErrAuthenticationRequired
	wrapped := fmt.Errorf("clone https://example/r: %w: %w", ErrAccessDenied, original)
	assert.True(t, errors.Is(wrapped, ErrAccessDenied), "errors.Is should find ErrAccessDenied")
	assert.True(t, errors.Is(wrapped, original), "errors.Is should find the underlying auth error")
}
