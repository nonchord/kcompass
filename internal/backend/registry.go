// Package backend defines the Backend interface and shared types used by all backends.
package backend

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type cachedList struct {
	records []ClusterRecord
	expiry  time.Time
}

// Registry merges results from multiple backends, deduplicating by name
// (first backend wins) and caching the combined list for a configurable TTL.
// Registry itself implements Backend so the CLI never needs to hold individual
// backend references.
type Registry struct {
	backends []Backend
	ttl      time.Duration
	log      func(string)
	mu       sync.Mutex
	cache    *cachedList
}

// NewRegistry creates a Registry from the given backends, TTL, and optional
// log function. When log is non-nil, individual backend failures are logged
// as warnings rather than failing the entire list.
func NewRegistry(backends []Backend, ttl time.Duration, log func(string)) *Registry {
	return &Registry{backends: backends, ttl: ttl, log: log}
}

// Name implements Backend.
func (r *Registry) Name() string { return "registry" }

// Backends returns the individual backends held by this registry.
func (r *Registry) Backends() []Backend { return r.backends }

// List implements Backend. Results are cached for the configured TTL.
//
// Individual backend failures are logged and skipped so that one broken
// backend doesn't block access to clusters from the others. If ALL backends
// fail, the combined errors are returned.
//
// The whole operation runs under r.mu so concurrent callers collapse to a
// single backend walk rather than racing to recompute and overwrite each
// other's cache entry.
func (r *Registry) List(ctx context.Context) ([]ClusterRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cache != nil && r.ttl > 0 && time.Now().Before(r.cache.expiry) {
		return r.cache.records, nil
	}

	seen := make(map[string]bool)
	var merged []ClusterRecord
	var backendErrs []error
	succeeded := 0

	for _, b := range r.backends {
		records, err := b.List(ctx)
		if err != nil {
			backendErrs = append(backendErrs, fmt.Errorf("backend %q: %w", b.Name(), err))
			if r.log != nil {
				r.log(fmt.Sprintf("warning: backend %q: %v", b.Name(), err))
			}
			continue
		}
		succeeded++
		for _, rec := range records {
			if !seen[rec.Name] {
				seen[rec.Name] = true
				merged = append(merged, rec)
			}
		}
	}

	// If every backend failed, surface the errors so the user knows why
	// the list is empty. errors.Join preserves the error chain so callers
	// can match sentinels like ErrAccessDenied via errors.Is.
	if succeeded == 0 && len(backendErrs) > 0 {
		return nil, fmt.Errorf("registry: all backends failed: %w", errors.Join(backendErrs...))
	}

	r.cache = &cachedList{records: merged, expiry: time.Now().Add(r.ttl)}
	return merged, nil
}

// Get implements Backend.
func (r *Registry) Get(ctx context.Context, name string) (*ClusterRecord, error) {
	records, err := r.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range records {
		if records[i].Name == name {
			return &records[i], nil
		}
	}
	return nil, ErrNotFound
}
