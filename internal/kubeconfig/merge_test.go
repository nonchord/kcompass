package kubeconfig_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nonchord/kcompass/internal/kubeconfig"
	kcmd "k8s.io/client-go/tools/clientcmd"
)

const testdataDir = "../../testdata"

var incomingKubeconfig = []byte(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://new:6443
  name: dev-cluster
contexts:
- context:
    cluster: dev-cluster
    user: dev-admin
  name: dev-cluster
current-context: dev-cluster
users:
- name: dev-admin
  user:
    token: new-token
`)

func copyFixture(t *testing.T, fixture string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testdataDir, fixture))
	require.NoError(t, err)
	dest := filepath.Join(t.TempDir(), fixture)
	require.NoError(t, os.WriteFile(dest, data, 0o600))
	return dest
}

func TestMergeIntoEmpty(t *testing.T) {
	path := copyFixture(t, "kubeconfig_empty.yaml")
	ctx, err := kubeconfig.MergeStatic(path, incomingKubeconfig, true)
	require.NoError(t, err)
	assert.Equal(t, "dev-cluster", ctx)

	cfg, err := kcmd.LoadFromFile(path)
	require.NoError(t, err)
	assert.Contains(t, cfg.Clusters, "dev-cluster")
	assert.Equal(t, "dev-cluster", cfg.CurrentContext)
}

func TestMergeNoConflict(t *testing.T) {
	path := copyFixture(t, "kubeconfig_existing.yaml")
	ctx, err := kubeconfig.MergeStatic(path, incomingKubeconfig, false)
	require.NoError(t, err)
	assert.Equal(t, "dev-cluster", ctx)

	cfg, err := kcmd.LoadFromFile(path)
	require.NoError(t, err)
	assert.Contains(t, cfg.Clusters, "existing-cluster")
	assert.Contains(t, cfg.Clusters, "dev-cluster")
	assert.Equal(t, "existing-context", cfg.CurrentContext, "context should not switch")
}

func TestMergeConflict(t *testing.T) {
	path := copyFixture(t, "kubeconfig_conflict.yaml")
	ctx, err := kubeconfig.MergeStatic(path, incomingKubeconfig, true)
	require.NoError(t, err)
	assert.Equal(t, "dev-cluster-1", ctx)

	cfg, err := kcmd.LoadFromFile(path)
	require.NoError(t, err)
	assert.Contains(t, cfg.Contexts, "dev-cluster", "original context must remain")
	assert.Contains(t, cfg.Contexts, "dev-cluster-1", "new context renamed")
	assert.Equal(t, "dev-cluster-1", cfg.CurrentContext)
}

func TestMergeSwitchContext(t *testing.T) {
	path := copyFixture(t, "kubeconfig_existing.yaml")
	_, err := kubeconfig.MergeStatic(path, incomingKubeconfig, true)
	require.NoError(t, err)
	cfg, err := kcmd.LoadFromFile(path)
	require.NoError(t, err)
	assert.Equal(t, "dev-cluster", cfg.CurrentContext)
}

func TestMergeNoSwitch(t *testing.T) {
	path := copyFixture(t, "kubeconfig_existing.yaml")
	_, err := kubeconfig.MergeStatic(path, incomingKubeconfig, false)
	require.NoError(t, err)
	cfg, err := kcmd.LoadFromFile(path)
	require.NoError(t, err)
	assert.Equal(t, "existing-context", cfg.CurrentContext)
}

func TestMergeNonExistentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subdir", "kubeconfig")
	ctx, err := kubeconfig.MergeStatic(path, incomingKubeconfig, true)
	require.NoError(t, err)
	assert.Equal(t, "dev-cluster", ctx)
	_, err = os.Stat(path)
	assert.NoError(t, err, "file should have been created")
}

func TestMergeAtomic(t *testing.T) {
	// Make the target directory read-only so the rename fails.
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	_, err := kubeconfig.MergeStatic(path, incomingKubeconfig, true)
	require.Error(t, err)
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "original must not be modified on failure")
}
