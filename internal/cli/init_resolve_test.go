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
	reg := backend.NewRegistry([]backend.Backend{accessDeniedBackend{}}, 0, nil)

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
	reg := backend.NewRegistry([]backend.Backend{accessDeniedBackend{}}, 0, nil)

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

// TestInitRejectsZoneShapedTargetWithBothLookupsFailing verifies the exact
// scenario that originally motivated this error classifier: a user types
// `kcompass init nonchord.com` on a machine that can't resolve the TXT
// record, the zone-mode path misses, the fallback treats the target as a
// local file (which also doesn't exist), and the error must name BOTH
// possible interpretations so the user has something actionable to fix.
func TestInitRejectsZoneShapedTargetWithBothLookupsFailing(t *testing.T) {
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
	msg := err.Error()
	// Message must explain both the zone miss and the file miss, and
	// suggest how to fix each.
	assert.Contains(t, msg, "looks like a DNS zone")
	assert.Contains(t, msg, "kcompass.example.com")
	assert.Contains(t, msg, "v=kc1")
	assert.Contains(t, msg, "URL scheme")

	// No config file should have been created.
	_, statErr := os.Stat(cfgPath)
	assert.True(t, os.IsNotExist(statErr),
		"init must not write config when verification fails, got stat error: %v", statErr)
}

// TestInitRejectsMissingPlainPath verifies the non-zone-shaped branch of
// the error classifier. An explicit file path that doesn't exist should
// produce a clean "no file or directory at X" error — no DNS hint, no
// long multi-line message about URL schemes.
func TestInitRejectsMissingPlainPath(t *testing.T) {
	// Stub TXT lookup so it's deterministic even though looksLikeDNSZone
	// will reject this target anyway (has a slash).
	stubLookupTXT(t, func(_ context.Context, _ string) ([]string, error) {
		return nil, errors.New("no record")
	})

	cfgPath := filepath.Join(t.TempDir(), "config.yaml")

	root := NewRootCommand()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs([]string{"--config", cfgPath, "init", "/tmp/kcompass-no-such-dir/clusters.yaml"})
	err := root.Execute()

	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "no file or directory")
	// Must NOT show the DNS-zone hint for a plain path.
	assert.NotContains(t, msg, "DNS zone")
	assert.NotContains(t, msg, "v=kc1")

	_, statErr := os.Stat(cfgPath)
	assert.True(t, os.IsNotExist(statErr))
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
