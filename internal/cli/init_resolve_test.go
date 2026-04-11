package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nonchord/kcompass/internal/backend"
	"github.com/nonchord/kcompass/pkg/config"
)

func TestLooksLikeDNSZone(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"nonchord.com", true},
		{"internal.example.com", true},
		{"example.com", true},
		{"", false},
		{"nothingtosee", false},
		{"./relative.yaml", false},
		{"/absolute/path.yaml", false},
		{"~/home-relative.yaml", false},
		{"git@github.com:org/clusters", false},
		{"https://github.com/org/clusters", false},
		{"git://github.com/org/clusters", false},
		{"ssh://git@github.com/org/clusters", false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			assert.Equal(t, c.want, looksLikeDNSZone(c.in))
		})
	}
}

// stubLookupTXT swaps the package-level TXT resolver for the duration of a
// test. The returned function restores the original.
func stubLookupTXT(t *testing.T, fn func(ctx context.Context, name string) ([]string, error)) {
	t.Helper()
	old := lookupTXT
	lookupTXT = fn
	t.Cleanup(func() { lookupTXT = old })
}

func TestResolveZoneToBackendURLHit(t *testing.T) {
	stubLookupTXT(t, func(_ context.Context, name string) ([]string, error) {
		if name == "kcompass.example.com" {
			return []string{"v=kc1; backend=git@github.com:example/clusters"}, nil
		}
		return nil, errors.New("no record")
	})
	url, ok := resolveZoneToBackendURL(context.Background(), "example.com")
	require.True(t, ok)
	assert.Equal(t, "git@github.com:example/clusters", url)
}

func TestResolveZoneToBackendURLMiss(t *testing.T) {
	stubLookupTXT(t, func(_ context.Context, _ string) ([]string, error) {
		return nil, errors.New("no record")
	})
	_, ok := resolveZoneToBackendURL(context.Background(), "example.com")
	assert.False(t, ok)
}

func TestResolveZoneToBackendURLIgnoresUnrelatedTXT(t *testing.T) {
	stubLookupTXT(t, func(_ context.Context, _ string) ([]string, error) {
		return []string{
			"v=spf1 include:_spf.google.com ~all",
			"google-site-verification=foo",
		}, nil
	})
	_, ok := resolveZoneToBackendURL(context.Background(), "example.com")
	assert.False(t, ok, "unrelated TXT records must not be mistaken for kcompass records")
}

// TestInitResolvesZoneViaTXT drives the init command end-to-end with a
// stubbed TXT resolver. The config must contain the resolved git URL,
// not the zone name the user typed.
func TestInitResolvesZoneViaTXT(t *testing.T) {
	stubLookupTXT(t, func(_ context.Context, name string) ([]string, error) {
		if name == "kcompass.example.com" {
			return []string{"v=kc1; backend=git@github.com:example/clusters"}, nil
		}
		return nil, errors.New("no record")
	})

	cfgPath := filepath.Join(t.TempDir(), "config.yaml")

	root := NewRootCommand()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--config", cfgPath, "init", "example.com"})
	require.NoError(t, root.Execute())

	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	require.Len(t, cfg.Backends, 1)
	assert.Equal(t, "git", cfg.Backends[0].Type)
	assert.Equal(t, "git@github.com:example/clusters", cfg.Backends[0].Options["url"])

	out := buf.String()
	assert.Contains(t, out, "Resolved kcompass.example.com")
	assert.Contains(t, out, "git@github.com:example/clusters")
	assert.Contains(t, out, "via kcompass.example.com TXT record")
}

// accessDeniedBackend is a minimal Backend implementation that always
// returns ErrAccessDenied wrapped in a realistic error chain. Used to
// verify that the list and connect commands detect the sentinel and
// print the friendly message without the raw git error bubbling up.
type accessDeniedBackend struct{}

func (accessDeniedBackend) Name() string { return "access-denied-stub" }
func (accessDeniedBackend) List(_ context.Context) ([]backend.ClusterRecord, error) {
	return nil, fmt.Errorf("clone git@github.com:private/repo: %w: ssh: handshake failed", backend.ErrAccessDenied)
}
func (accessDeniedBackend) Get(ctx context.Context, _ string) (*backend.ClusterRecord, error) {
	_, err := accessDeniedBackend{}.List(ctx)
	return nil, err
}

func TestListSurfacesAccessDenied(t *testing.T) {
	reg := backend.NewRegistry([]backend.Backend{accessDeniedBackend{}}, 0)

	root := NewRootCommand()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs([]string{"list"})
	ctx := context.WithValue(context.Background(), RegistryKey{}, reg)
	root.SetContext(ctx)

	err := root.Execute()
	require.Error(t, err, "expected list to return a non-nil error for non-zero exit")
	assert.True(t, errors.Is(err, backend.ErrAccessDenied))

	msg := stderr.String()
	assert.Contains(t, msg, "You don't have access to this cluster inventory.")
	assert.Contains(t, msg, "SSH key")
}

func TestConnectSurfacesAccessDenied(t *testing.T) {
	reg := backend.NewRegistry([]backend.Backend{accessDeniedBackend{}}, 0)

	root := NewRootCommand()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs([]string{"connect", "any-cluster"})
	ctx := context.WithValue(context.Background(), RegistryKey{}, reg)
	root.SetContext(ctx)

	err := root.Execute()
	require.Error(t, err)
	assert.True(t, errors.Is(err, backend.ErrAccessDenied))
	assert.Contains(t, stderr.String(), "You don't have access to this cluster inventory.")
}

// TestInitFallsBackToLocalOnTXTMiss verifies that when the argument looks
// like a zone but no TXT record resolves, the argument is treated as a
// local file path (preserving the pre-zone-mode behavior).
func TestInitFallsBackToLocalOnTXTMiss(t *testing.T) {
	stubLookupTXT(t, func(_ context.Context, _ string) ([]string, error) {
		return nil, errors.New("no record")
	})

	cfgPath := filepath.Join(t.TempDir(), "config.yaml")

	root := NewRootCommand()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--config", cfgPath, "init", "example.com"})
	require.NoError(t, root.Execute())

	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	require.Len(t, cfg.Backends, 1)
	// No TXT hit → treated as a local path, same as "example.com" would
	// have been before zone mode existed.
	assert.Equal(t, "local", cfg.Backends[0].Type)
	assert.Equal(t, "example.com", cfg.Backends[0].Options["path"])
}
