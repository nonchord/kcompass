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

// TestMergeIdempotent verifies that running MergeStatic twice with the same
// incoming kubeconfig leaves exactly one entry per cluster/user/context,
// instead of producing a fresh -1/-2 suffixed duplicate on every call. This
// is what users see when they run `kcompass connect <cluster>` more than
// once — before this fix, the second run produced dev-cluster-1, the third
// produced dev-cluster-2, etc.
func TestMergeIdempotent(t *testing.T) {
	path := copyFixture(t, "kubeconfig_empty.yaml")

	_, err := kubeconfig.MergeStatic(path, incomingKubeconfig, true)
	require.NoError(t, err)
	_, err = kubeconfig.MergeStatic(path, incomingKubeconfig, true)
	require.NoError(t, err)
	_, err = kubeconfig.MergeStatic(path, incomingKubeconfig, true)
	require.NoError(t, err)

	cfg, err := kcmd.LoadFromFile(path)
	require.NoError(t, err)
	assert.Len(t, cfg.Clusters, 1, "cluster must not duplicate across repeated merges")
	assert.Len(t, cfg.AuthInfos, 1, "user must not duplicate across repeated merges")
	assert.Len(t, cfg.Contexts, 1, "context must not duplicate across repeated merges")
	assert.Contains(t, cfg.Clusters, "dev-cluster")
	assert.Contains(t, cfg.AuthInfos, "dev-admin")
	assert.Contains(t, cfg.Contexts, "dev-cluster")
	assert.Equal(t, "dev-cluster", cfg.CurrentContext)
}

// TestMergeConflictRewritesContextRefs verifies that when a genuine name
// collision forces a suffix, the new context's cluster/user refs are
// rewritten to point at the renamed targets — not at the old ones that
// still occupy the original slots. This was a latent bug: the rename-only
// merge used to leave the new context pointing at the old cluster's server
// URL and the old user's credentials.
func TestMergeConflictRewritesContextRefs(t *testing.T) {
	path := copyFixture(t, "kubeconfig_conflict.yaml")

	_, err := kubeconfig.MergeStatic(path, incomingKubeconfig, true)
	require.NoError(t, err)

	cfg, err := kcmd.LoadFromFile(path)
	require.NoError(t, err)

	// The renamed context must reference the renamed cluster and user.
	renamedCtx, ok := cfg.Contexts["dev-cluster-1"]
	require.True(t, ok, "renamed context must exist")
	assert.Equal(t, "dev-cluster-1", renamedCtx.Cluster,
		"renamed context must point at renamed cluster, not the original")
	assert.Equal(t, "dev-admin-1", renamedCtx.AuthInfo,
		"renamed context must point at renamed user, not the original")

	// And the underlying cluster must be the NEW server, not the existing one.
	renamedCluster, ok := cfg.Clusters["dev-cluster-1"]
	require.True(t, ok)
	assert.Equal(t, "https://new:6443", renamedCluster.Server)

	// The original dev-cluster must still point at the original server.
	origCluster, ok := cfg.Clusters["dev-cluster"]
	require.True(t, ok)
	assert.Equal(t, "https://existing:6443", origCluster.Server)
}

// TestMergeSameNameDifferentContentStillSuffixes pins the rename behavior
// when names collide with different content, which is the existing
// TestMergeConflict case but asserted at the API-level against repeated
// invocations: the first call renames, the second is idempotent (no
// further suffixes added because the rewritten entry is now present).
func TestMergeSameNameDifferentContentStillSuffixes(t *testing.T) {
	path := copyFixture(t, "kubeconfig_conflict.yaml")

	_, err := kubeconfig.MergeStatic(path, incomingKubeconfig, true)
	require.NoError(t, err)
	_, err = kubeconfig.MergeStatic(path, incomingKubeconfig, true)
	require.NoError(t, err)

	cfg, err := kcmd.LoadFromFile(path)
	require.NoError(t, err)
	assert.Len(t, cfg.Clusters, 2, "one original + one renamed — no third suffix on rerun")
	assert.Len(t, cfg.AuthInfos, 2)
	assert.Len(t, cfg.Contexts, 2)
	assert.NotContains(t, cfg.Clusters, "dev-cluster-2",
		"second merge of the same incoming must not add dev-cluster-2")
}
