package backend_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nonchord/kcompass/internal/backend"
)

// TestBackendNames covers the trivial Name() getters for all three backends
// and the Registry. These are small but keep pre-0.1.0 coverage honest —
// they're part of the Backend interface contract.
func TestBackendNames(t *testing.T) {
	t.Run("LocalBackend", func(t *testing.T) {
		b, err := backend.NewLocalBackend("my-local", "/tmp/clusters.yaml")
		require.NoError(t, err)
		assert.Equal(t, "my-local", b.Name())
	})

	t.Run("GitBackend", func(t *testing.T) {
		b, err := backend.NewGitBackend(backend.GitBackendConfig{
			Name: "my-git",
			URL:  "file:///tmp/not-real.git",
		})
		require.NoError(t, err)
		assert.Equal(t, "my-git", b.Name())
	})

	t.Run("Registry", func(t *testing.T) {
		reg := backend.NewRegistry(nil, 0, nil)
		assert.Equal(t, "registry", reg.Name())
	})
}

// TestRegistryBackendsGetter pins the Backends() accessor used by the CLI
// `list` command to detect an empty-registry state.
func TestRegistryBackendsGetter(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		reg := backend.NewRegistry(nil, 0, nil)
		assert.Empty(t, reg.Backends())
	})

	t.Run("populated", func(t *testing.T) {
		lb, err := backend.NewLocalBackend("a", "/tmp/a.yaml")
		require.NoError(t, err)
		reg := backend.NewRegistry([]backend.Backend{lb}, 0, nil)
		got := reg.Backends()
		require.Len(t, got, 1)
		assert.Equal(t, "a", got[0].Name())
	})
}

// errBackend is a minimal Backend that always fails List, used to cover
// the error-propagation path in Registry.Get.
type errBackend struct{ name string }

func (e *errBackend) Name() string { return e.name }
func (e *errBackend) List(_ context.Context) ([]backend.ClusterRecord, error) {
	return nil, errListFailed
}
func (e *errBackend) Get(_ context.Context, _ string) (*backend.ClusterRecord, error) {
	return nil, errListFailed
}

var errListFailed = errors.New("list failed")

// TestRegistryGetPropagatesListError exercises the branch of Registry.Get
// where List returns an error. When all backends fail, Get must propagate
// the combined error, not silently return ErrNotFound.
func TestRegistryGetPropagatesListError(t *testing.T) {
	reg := backend.NewRegistry([]backend.Backend{&errBackend{name: "broken"}}, 0, nil)
	_, err := reg.Get(context.Background(), "anything")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all backends failed",
		"Get must propagate the underlying List error, not swallow it")
	assert.Contains(t, err.Error(), errListFailed.Error())
}

// TestLocalBackendGetMissingFile exercises LocalBackend.Get's error path
// when the inventory file does not exist. List returns an os-level error,
// which Get must propagate rather than transform into ErrNotFound.
func TestLocalBackendGetMissingFile(t *testing.T) {
	b, err := backend.NewLocalBackend("local", "/tmp/kcompass-definitely-not-here.yaml")
	require.NoError(t, err)

	_, err = b.Get(context.Background(), "anything")
	require.Error(t, err)
	assert.NotErrorIs(t, err, backend.ErrNotFound,
		"missing file is an I/O error, not a not-found cluster")
}

// TestGitBackendUnclonableURL exercises the cloneRepo error path. A file://
// URL pointing at a nonexistent local path is cheap to set up, deterministic,
// and avoids touching the network. go-git will fail at PlainCloneContext,
// the error should bubble up from List.
func TestGitBackendUnclonableURL(t *testing.T) {
	b, err := backend.NewGitBackend(backend.GitBackendConfig{
		Name:     "unclonable",
		URL:      "file:///tmp/kcompass-definitely-not-a-repo",
		CacheDir: t.TempDir(),
	})
	require.NoError(t, err)

	_, err = b.List(context.Background())
	require.Error(t, err, "List on an unclonable URL must return an error")
	assert.Contains(t, err.Error(), "clone")
}
