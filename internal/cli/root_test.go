package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nonchord/kcompass/internal/backend"
	"github.com/nonchord/kcompass/internal/cli"
)

const testdataDir = "../../testdata"

// executeWithConfig runs a command with a custom config file path, bypassing
// the default ~/.kcompass/config.yaml. The registry is built from that config.
func executeWithConfig(t *testing.T, cfgPath string, args ...string) (string, error) {
	t.Helper()
	buf := &bytes.Buffer{}
	root := cli.NewRootCommand()
	root.SetOut(buf)
	root.SetArgs(append([]string{"--config", cfgPath}, args...))
	err := root.Execute()
	return buf.String(), err
}

// executeWithRegistry runs a kcompass command with a pre-built registry injected,
// bypassing config-file loading. It returns captured stdout.
func executeWithRegistry(t *testing.T, reg *backend.Registry, args ...string) (string, error) {
	t.Helper()
	buf := &bytes.Buffer{}
	root := cli.NewRootCommand()
	root.SetOut(buf)
	root.SetArgs(args)

	// Inject the registry via context before Execute so PersistentPreRunE
	// can still set it (our override is replaced, so we inject it ourselves
	// by wrapping the command context).
	ctx := context.WithValue(context.Background(), cli.RegistryKey{}, reg)
	root.SetContext(ctx)

	err := root.Execute()
	return buf.String(), err
}

func makeLocalRegistry(t *testing.T, fixture string) *backend.Registry {
	t.Helper()
	b, err := backend.NewLocalBackend("local", filepath.Join(testdataDir, fixture))
	require.NoError(t, err)
	return backend.NewRegistry([]backend.Backend{b}, 0)
}

func TestListCommand(t *testing.T) {
	reg := makeLocalRegistry(t, "local_clusters.yaml")
	out, err := executeWithRegistry(t, reg, "list")
	require.NoError(t, err)
	assert.Contains(t, out, "cluster1")
	assert.Contains(t, out, "cluster2")
	assert.Contains(t, out, "The production cluster.")
}

func TestListCommandJSON(t *testing.T) {
	reg := makeLocalRegistry(t, "local_clusters.yaml")
	out, err := executeWithRegistry(t, reg, "list", "--json")
	require.NoError(t, err)
	var records []backend.ClusterRecord
	require.NoError(t, json.Unmarshal([]byte(out), &records))
	assert.Len(t, records, 2)
}

func TestListNoBackendsMessage(t *testing.T) {
	// An empty config (no backends, discovery disabled) should print the
	// "no cluster registry" guidance instead of crashing or silently doing nothing.
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("discovery:\n  enabled: false\n"), 0o600))

	out, err := executeWithConfig(t, cfgPath, "list")
	require.NoError(t, err)
	assert.Contains(t, out, "No cluster registry found")
	assert.Contains(t, out, "kcompass init")
}

func TestDiscoveryDisabledByConfig(t *testing.T) {
	// With discovery disabled and no backends, registry should have zero backends.
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("discovery:\n  enabled: false\n"), 0o600))

	out, err := executeWithConfig(t, cfgPath, "list")
	require.NoError(t, err)
	assert.Contains(t, out, "No cluster registry found")
}

func TestConnectStaticAuth(t *testing.T) {
	reg := makeLocalRegistry(t, "local_static.yaml")
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	t.Setenv("KUBECONFIG", kubeconfigPath)

	out, err := executeWithRegistry(t, reg, "connect", "dev-cluster")
	require.NoError(t, err)
	assert.Contains(t, out, "Done.")
	assert.Contains(t, out, "dev-cluster")

	_, statErr := os.Stat(kubeconfigPath)
	assert.NoError(t, statErr, "kubeconfig should have been created")
}
