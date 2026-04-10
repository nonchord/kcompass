package backend_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nonchord/kcompass/internal/backend"
)

// mockBackend is a simple in-memory Backend for testing.
type mockBackend struct {
	name    string
	records []backend.ClusterRecord
	calls   int
}

func (m *mockBackend) Name() string { return m.name }

func (m *mockBackend) List(_ context.Context) ([]backend.ClusterRecord, error) {
	m.calls++
	return m.records, nil
}

func (m *mockBackend) Get(_ context.Context, name string) (*backend.ClusterRecord, error) {
	for i := range m.records {
		if m.records[i].Name == name {
			return &m.records[i], nil
		}
	}
	return nil, backend.ErrNotFound
}

func rec(name string) backend.ClusterRecord {
	return backend.ClusterRecord{Name: name}
}

func TestRegistryMerge(t *testing.T) {
	a := &mockBackend{name: "a", records: []backend.ClusterRecord{rec("c1"), rec("c2")}}
	b := &mockBackend{name: "b", records: []backend.ClusterRecord{rec("c3")}}
	r := backend.NewRegistry([]backend.Backend{a, b}, time.Minute)
	records, err := r.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, records, 3)
}

func TestRegistryDedup(t *testing.T) {
	a := &mockBackend{name: "a", records: []backend.ClusterRecord{rec("shared"), rec("only-a")}}
	b := &mockBackend{name: "b", records: []backend.ClusterRecord{rec("shared"), rec("only-b")}}
	r := backend.NewRegistry([]backend.Backend{a, b}, time.Minute)
	records, err := r.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, records, 3)
	// first backend wins on collision
	names := make([]string, len(records))
	for i, rec := range records {
		names[i] = rec.Name
	}
	assert.Contains(t, names, "shared")
	assert.Contains(t, names, "only-a")
	assert.Contains(t, names, "only-b")
}

func TestRegistryCacheTTL(t *testing.T) {
	m := &mockBackend{name: "a", records: []backend.ClusterRecord{rec("c1")}}
	r := backend.NewRegistry([]backend.Backend{m}, time.Minute)
	_, err := r.List(context.Background())
	require.NoError(t, err)
	_, err = r.List(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, m.calls, "second call should hit cache")
}

func TestRegistryCacheExpiry(t *testing.T) {
	m := &mockBackend{name: "a", records: []backend.ClusterRecord{rec("c1")}}
	r := backend.NewRegistry([]backend.Backend{m}, time.Millisecond)
	_, err := r.List(context.Background())
	require.NoError(t, err)
	time.Sleep(5 * time.Millisecond)
	_, err = r.List(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, m.calls, "second call after expiry should re-query backend")
}

func TestRegistryGetNotFound(t *testing.T) {
	m := &mockBackend{name: "a", records: []backend.ClusterRecord{rec("c1")}}
	r := backend.NewRegistry([]backend.Backend{m}, time.Minute)
	_, err := r.Get(context.Background(), "nope")
	assert.True(t, errors.Is(err, backend.ErrNotFound))
}

func TestRegistryContextCancelled(t *testing.T) {
	m := &mockBackend{name: "a", records: []backend.ClusterRecord{rec("c1")}}
	r := backend.NewRegistry([]backend.Backend{m}, time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := r.List(ctx)
	require.Error(t, err)
	assert.Equal(t, 0, m.calls)
}
