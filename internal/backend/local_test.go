package backend_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nonchord/kcompass/internal/backend"
)

const testdataDir = "../../testdata"

func TestLocalBackendList(t *testing.T) {
	b, err := backend.NewLocalBackend("test", filepath.Join(testdataDir, "local_clusters.yaml"))
	require.NoError(t, err)
	records, err := b.List(context.Background())
	require.NoError(t, err)
	require.Len(t, records, 2)
	assert.Equal(t, "cluster1", records[0].Name)
	assert.Equal(t, "cluster2", records[1].Name)
}

func TestLocalBackendGet(t *testing.T) {
	b, err := backend.NewLocalBackend("test", filepath.Join(testdataDir, "local_clusters.yaml"))
	require.NoError(t, err)
	rec, err := b.Get(context.Background(), "cluster2")
	require.NoError(t, err)
	assert.Equal(t, "cluster2", rec.Name)
	assert.Equal(t, "The staging cluster.", rec.Description)
}

func TestLocalBackendGetNotFound(t *testing.T) {
	b, err := backend.NewLocalBackend("test", filepath.Join(testdataDir, "local_clusters.yaml"))
	require.NoError(t, err)
	_, err = b.Get(context.Background(), "nope")
	assert.True(t, errors.Is(err, backend.ErrNotFound))
}

func TestLocalBackendMissingFile(t *testing.T) {
	b, err := backend.NewLocalBackend("test", filepath.Join(t.TempDir(), "missing.yaml"))
	require.NoError(t, err)
	_, err = b.List(context.Background())
	require.Error(t, err)
}

func TestLocalBackendEmptyFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.yaml")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	b, err := backend.NewLocalBackend("test", f.Name())
	require.NoError(t, err)
	records, err := b.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, records)
}
