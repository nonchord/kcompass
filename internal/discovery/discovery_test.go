package discovery_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nonchord/kcompass/internal/backend"
	"github.com/nonchord/kcompass/internal/discovery"
)

// stubBackend is a minimal backend.Backend used in probe results.
type stubBackend struct{ name string }

func (s *stubBackend) Name() string                                              { return s.name }
func (s *stubBackend) List(_ context.Context) ([]backend.ClusterRecord, error)  { return nil, nil }
func (s *stubBackend) Get(_ context.Context, _ string) (*backend.ClusterRecord, error) {
	return nil, backend.ErrNotFound
}

func probeReturning(b backend.Backend) discovery.ProbeFunc {
	return func(_ context.Context) (backend.Backend, error) { return b, nil }
}

func probeReturningErr(err error) discovery.ProbeFunc {
	return func(_ context.Context) (backend.Backend, error) { return nil, err }
}

func TestRunAllSucceed(t *testing.T) {
	a := &stubBackend{"a"}
	b := &stubBackend{"b"}
	got := discovery.Run(context.Background(), []discovery.ProbeFunc{
		probeReturning(a),
		probeReturning(b),
	}, 500*time.Millisecond)
	require.Len(t, got, 2)
}

func TestRunSomeNil(t *testing.T) {
	a := &stubBackend{"a"}
	got := discovery.Run(context.Background(), []discovery.ProbeFunc{
		probeReturning(a),
		probeReturning(nil),
		func(_ context.Context) (backend.Backend, error) { return nil, nil },
	}, 500*time.Millisecond)
	require.Len(t, got, 1)
	assert.Equal(t, "a", got[0].Name())
}

func TestRunErrorsDiscarded(t *testing.T) {
	a := &stubBackend{"a"}
	got := discovery.Run(context.Background(), []discovery.ProbeFunc{
		probeReturning(a),
		probeReturningErr(errors.New("boom")),
	}, 500*time.Millisecond)
	require.Len(t, got, 1)
}

func TestRunEmptyProbes(t *testing.T) {
	got := discovery.Run(context.Background(), nil, 500*time.Millisecond)
	assert.Empty(t, got)
}

func TestRunTimeout(t *testing.T) {
	slow := func(ctx context.Context) (backend.Backend, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Second):
			return &stubBackend{"slow"}, nil
		}
	}
	fast := &stubBackend{"fast"}

	start := time.Now()
	got := discovery.Run(context.Background(), []discovery.ProbeFunc{
		slow,
		probeReturning(fast),
	}, 100*time.Millisecond)
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 2*time.Second, "Run should respect the timeout")
	require.Len(t, got, 1)
	assert.Equal(t, "fast", got[0].Name())
}

func TestRunAllParallel(t *testing.T) {
	// Verify that probes run concurrently: all three start within the timeout
	// even though each one blocks for 50ms.
	var count atomic.Int32
	blocker := func(ctx context.Context) (backend.Backend, error) {
		count.Add(1)
		select {
		case <-ctx.Done():
			return nil, nil
		case <-time.After(50 * time.Millisecond):
			return &stubBackend{"b"}, nil
		}
	}

	start := time.Now()
	got := discovery.Run(context.Background(), []discovery.ProbeFunc{blocker, blocker, blocker}, 500*time.Millisecond)
	elapsed := time.Since(start)

	assert.Equal(t, int32(3), count.Load())
	assert.Len(t, got, 3)
	// All three ran in parallel, so total time should be much less than 3×50ms = 150ms.
	assert.Less(t, elapsed, 200*time.Millisecond)
}

func TestRunZeroTimeoutUsesDefault(t *testing.T) {
	// A zero timeout should still give probes time to complete.
	fast := &stubBackend{"fast"}
	got := discovery.Run(context.Background(), []discovery.ProbeFunc{probeReturning(fast)}, 0)
	require.Len(t, got, 1)
}
