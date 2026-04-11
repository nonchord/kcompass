package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
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
// not the zone name the user typed. --skip-verify is passed so the
// resolved URL (which isn't a real repo) doesn't get cloned.
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
	root.SetArgs([]string{"--config", cfgPath, "init", "--skip-verify", "example.com"})
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
// local file path (preserving the pre-zone-mode behavior). --skip-verify
// is used because "example.com" is not a real file.
func TestInitFallsBackToLocalOnTXTMiss(t *testing.T) {
	stubLookupTXT(t, func(_ context.Context, _ string) ([]string, error) {
		return nil, errors.New("no record")
	})

	cfgPath := filepath.Join(t.TempDir(), "config.yaml")

	root := NewRootCommand()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--config", cfgPath, "init", "--skip-verify", "example.com"})
	require.NoError(t, root.Execute())

	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	require.Len(t, cfg.Backends, 1)
	// No TXT hit → treated as a local path, same as "example.com" would
	// have been before zone mode existed.
	assert.Equal(t, "local", cfg.Backends[0].Type)
	assert.Equal(t, "example.com", cfg.Backends[0].Options["path"])
}

// TestInitRejectsMissingLocalPath verifies that without --skip-verify, init
// refuses to register a local backend whose path does not exist, and does
// not write to the config file. This is the scenario the user hit running
// `kcompass init nonchord.com` on a machine that couldn't TXT-resolve the
// zone and then silently got a broken local entry.
func TestInitRejectsMissingLocalPath(t *testing.T) {
	// Stub TXT lookup so that the zone-resolution path misses and the
	// fallback treats "example.com" as a local file path — same flow the
	// user saw.
	stubLookupTXT(t, func(_ context.Context, _ string) ([]string, error) {
		return nil, errors.New("no record")
	})

	cfgPath := filepath.Join(t.TempDir(), "config.yaml")

	root := NewRootCommand()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs([]string{"--config", cfgPath, "init", "example.com"})
	err := root.Execute()

	require.Error(t, err, "init must reject an unreadable local path")
	assert.Contains(t, err.Error(), "cannot access")

	// No config file should have been created.
	_, statErr := os.Stat(cfgPath)
	assert.True(t, os.IsNotExist(statErr),
		"init must not write config when verification fails, got stat error: %v", statErr)
}

// TestInitVerifiesLocalHappyPath exercises the happy path: init against a
// real, parseable local inventory file must succeed and write the config.
func TestInitVerifiesLocalHappyPath(t *testing.T) {
	// Write a minimal valid inventory file into a temp dir.
	dir := t.TempDir()
	inventoryPath := filepath.Join(dir, "clusters.yaml")
	require.NoError(t, os.WriteFile(inventoryPath, []byte(
		"clusters:\n  - name: dev\n    kubeconfig:\n      command: [echo, placeholder]\n",
	), 0o600))

	cfgPath := filepath.Join(dir, "config.yaml")

	root := NewRootCommand()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--config", cfgPath, "init", inventoryPath})
	require.NoError(t, root.Execute())

	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	require.Len(t, cfg.Backends, 1)
	assert.Equal(t, "local", cfg.Backends[0].Type)
	assert.Equal(t, inventoryPath, cfg.Backends[0].Options["path"])
}

// TestInitSkipVerifyBypass proves that --skip-verify bypasses the access
// check, so operators can pre-configure a machine before it can actually
// reach the backend.
func TestInitSkipVerifyBypass(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")

	root := NewRootCommand()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--config", cfgPath, "init", "--skip-verify", "/definitely/not/here.yaml"})
	require.NoError(t, root.Execute())

	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	require.Len(t, cfg.Backends, 1)
	assert.Equal(t, "/definitely/not/here.yaml", cfg.Backends[0].Options["path"])
}
