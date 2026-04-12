// Package backend defines the Backend interface and shared types used by all backends.
package backend

import (
	"context"
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
	mu       sync.Mutex
	cache    *cachedList
}

// NewRegistry creates a Registry from the given backends and TTL.
// A zero TTL disables caching.
func NewRegistry(backends []Backend, ttl time.Duration) *Registry {
	return &Registry{backends: backends, ttl: ttl}
}

// Name implements Backend.
func (r *Registry) Name() string { return "registry" }

// Backends returns the individual backends held by this registry.
func (r *Registry) Backends() []Backend { return r.backends }

// List implements Backend. Results are cached for the configured TTL.
//
// The whole operation runs under r.mu so concurrent callers collapse to a
// single backend walk rather than racing to recompute and overwrite each
// other's cache entry. kcompass is a CLI with only occasional concurrent
// List callers, so the serialization cost is negligible and correctness
// (one consistent view of the cache) wins.
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

	for _, b := range r.backends {
		records, err := b.List(ctx)
		if err != nil {
			return nil, fmt.Errorf("registry: backend %q: %w", b.Name(), err)
		}
		for _, rec := range records {
			if !seen[rec.Name] {
				seen[rec.Name] = true
				merged = append(merged, rec)
			}
		}
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
