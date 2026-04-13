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
	r := backend.NewRegistry([]backend.Backend{a, b}, time.Minute, nil)
	records, err := r.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, records, 3)
}

func TestRegistryDedup(t *testing.T) {
	a := &mockBackend{name: "a", records: []backend.ClusterRecord{rec("shared"), rec("only-a")}}
	b := &mockBackend{name: "b", records: []backend.ClusterRecord{rec("shared"), rec("only-b")}}
	r := backend.NewRegistry([]backend.Backend{a, b}, time.Minute, nil)
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
	r := backend.NewRegistry([]backend.Backend{m}, time.Minute, nil)
	_, err := r.List(context.Background())
	require.NoError(t, err)
	_, err = r.List(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, m.calls, "second call should hit cache")
}

func TestRegistryCacheExpiry(t *testing.T) {
	m := &mockBackend{name: "a", records: []backend.ClusterRecord{rec("c1")}}
	r := backend.NewRegistry([]backend.Backend{m}, time.Millisecond, nil)
	_, err := r.List(context.Background())
	require.NoError(t, err)
	time.Sleep(5 * time.Millisecond)
	_, err = r.List(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, m.calls, "second call after expiry should re-query backend")
}

func TestRegistryGetNotFound(t *testing.T) {
	m := &mockBackend{name: "a", records: []backend.ClusterRecord{rec("c1")}}
	r := backend.NewRegistry([]backend.Backend{m}, time.Minute, nil)
	_, err := r.Get(context.Background(), "nope")
	assert.True(t, errors.Is(err, backend.ErrNotFound))
}

func TestRegistryContextCancelled(t *testing.T) {
	m := &mockBackend{name: "a", records: []backend.ClusterRecord{rec("c1")}}
	r := backend.NewRegistry([]backend.Backend{m}, time.Minute, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := r.List(ctx)
	require.Error(t, err)
	assert.Equal(t, 0, m.calls)
}

// failingBackend always returns an error from List.
type failingBackend struct {
	name string
	err  error
}

func (f *failingBackend) Name() string { return f.name }
func (f *failingBackend) List(_ context.Context) ([]backend.ClusterRecord, error) {
	return nil, f.err
}
func (f *failingBackend) Get(_ context.Context, _ string) (*backend.ClusterRecord, error) {
	return nil, f.err
}

func TestRegistryPartialFailure(t *testing.T) {
	good := &mockBackend{name: "good", records: []backend.ClusterRecord{rec("c1")}}
	bad := &failingBackend{name: "bad", err: errors.New("network error")}

	var logged []string
	logFn := func(s string) { logged = append(logged, s) }

	r := backend.NewRegistry([]backend.Backend{bad, good}, time.Minute, logFn)
	records, err := r.List(context.Background())
	require.NoError(t, err, "one failing backend must not fail the whole list")
	assert.Len(t, records, 1)
	assert.Equal(t, "c1", records[0].Name)

	require.Len(t, logged, 1)
	assert.Contains(t, logged[0], "bad")
	assert.Contains(t, logged[0], "network error")
}

func TestRegistryAllBackendsFail(t *testing.T) {
	bad1 := &failingBackend{name: "bad1", err: errors.New("error1")}
	bad2 := &failingBackend{name: "bad2", err: errors.New("error2")}

	r := backend.NewRegistry([]backend.Backend{bad1, bad2}, time.Minute, nil)
	_, err := r.List(context.Background())
	require.Error(t, err, "when all backends fail, List must return an error")
	assert.Contains(t, err.Error(), "all backends failed")
	assert.Contains(t, err.Error(), "error1")
	assert.Contains(t, err.Error(), "error2")
}
