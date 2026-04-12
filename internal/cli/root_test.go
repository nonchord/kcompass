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

func TestConnectInlineKubeconfig(t *testing.T) {
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

// TestConnectReportsAlreadyConnected verifies that a second connect against
// the same cluster produces a distinct message ("already up to date") instead
// of the standard "Done." — so users get feedback that the command was a
// no-op rather than silently re-running the whole merge pipeline.
func TestConnectReportsAlreadyConnected(t *testing.T) {
	reg := makeLocalRegistry(t, "local_static.yaml")
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	t.Setenv("KUBECONFIG", kubeconfigPath)

	// First connect: fresh merge, should say "Done."
	firstOut, err := executeWithRegistry(t, reg, "connect", "dev-cluster")
	require.NoError(t, err)
	assert.Contains(t, firstOut, "Done.")
	assert.NotContains(t, firstOut, "already up to date")

	// Second connect: same cluster, same credentials. Should report that
	// the kubeconfig is already up to date and NOT print "Done.".
	secondOut, err := executeWithRegistry(t, reg, "connect", "dev-cluster")
	require.NoError(t, err)
	assert.Contains(t, secondOut, "already up to date")
	assert.NotContains(t, secondOut, "Done.",
		"second connect must not claim it set up anything new")
	assert.Contains(t, secondOut, "Context is set to dev-cluster",
		"context switch message still prints on reuse")
}

// TestConnectCommandKubeconfig exercises the command-mode credential path:
// the cluster record specifies a small shell command that writes a valid
// kubeconfig to $KUBECONFIG, and connect must capture and merge it.
func TestConnectCommandKubeconfig(t *testing.T) {
	// A YAML inventory with one cluster whose kubeconfig.command writes a
	// minimal kubeconfig to $KUBECONFIG via /bin/sh.
	const kubeconfigBlob = `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://10.0.0.1:6443
  name: cmd-cluster
contexts:
- context:
    cluster: cmd-cluster
    user: cmd-user
  name: cmd-cluster
current-context: cmd-cluster
users:
- name: cmd-user
  user:
    token: tok
`
	dir := t.TempDir()
	inventoryPath := filepath.Join(dir, "clusters.yaml")
	scriptPath := filepath.Join(dir, "fake-cred-tool.sh")

	// Script writes a kubeconfig blob to wherever $KUBECONFIG points.
	script := "#!/bin/sh\ncat > \"$KUBECONFIG\" <<'EOF'\n" + kubeconfigBlob + "EOF\n"
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	inventory := "clusters:\n" +
		"  - name: cmd-cluster\n" +
		"    description: Per-user credentials via command\n" +
		"    kubeconfig:\n" +
		"      command: [/bin/sh, " + scriptPath + "]\n"
	require.NoError(t, os.WriteFile(inventoryPath, []byte(inventory), 0o600))

	b, err := backend.NewLocalBackend("local", inventoryPath)
	require.NoError(t, err)
	reg := backend.NewRegistry([]backend.Backend{b}, 0)

	kubeconfigPath := filepath.Join(dir, "kubeconfig")
	t.Setenv("KUBECONFIG", kubeconfigPath)

	out, err := executeWithRegistry(t, reg, "connect", "cmd-cluster")
	require.NoError(t, err)
	assert.Contains(t, out, "Done.")

	merged, err := os.ReadFile(kubeconfigPath)
	require.NoError(t, err)
	assert.Contains(t, string(merged), "cmd-cluster")
	assert.Contains(t, string(merged), "10.0.0.1")
}

// TestConnectInvalidKubeconfigSpec verifies that a record with neither inline
// nor command produces an error at parse time, not at connect time. (We assert
// the parse-time failure path through the local backend.)
func TestConnectInvalidKubeconfigSpec(t *testing.T) {
	dir := t.TempDir()
	inventoryPath := filepath.Join(dir, "clusters.yaml")
	require.NoError(t, os.WriteFile(inventoryPath, []byte(
		"clusters:\n  - name: broken\n    kubeconfig: {}\n",
	), 0o600))

	b, err := backend.NewLocalBackend("local", inventoryPath)
	require.NoError(t, err)
	_, err = b.List(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kubeconfig")
}
