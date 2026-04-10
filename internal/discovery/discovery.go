// Package discovery implements auto-discovery of kcompass backends.
// It runs a set of probes in parallel and collects the backends they find.
// Each probe is best-effort: errors are silently discarded.
package discovery

import (
	"context"
	"sync"
	"time"

	"github.com/nonchord/kcompass/internal/backend"
)

// ProbeFunc attempts to discover a single backend.
// Return (nil, nil) when nothing was found (this is not an error).
// Errors from probes are discarded; they exist only to satisfy the type.
type ProbeFunc func(ctx context.Context) (backend.Backend, error)

// Run executes all probes in parallel, each bound by timeout. Non-nil
// backends are collected and returned; errors are silently dropped.
func Run(ctx context.Context, probes []ProbeFunc, timeout time.Duration) []backend.Backend {
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var (
		mu      sync.Mutex
		results []backend.Backend
		wg      sync.WaitGroup
	)

	for _, p := range probes {
		wg.Add(1)
		go func(probe ProbeFunc) {
			defer wg.Done()
			b, _ := probe(ctx) // errors are best-effort
			if b != nil {
				mu.Lock()
				results = append(results, b)
				mu.Unlock()
			}
		}(p)
	}
	wg.Wait()
	return results
}

// DefaultProbes returns the standard discovery probe set for production use.
func DefaultProbes() []ProbeFunc {
	return []ProbeFunc{
		TailscaleProbe(TailscaleOptions{}),
		NetbirdProbe(NetbirdOptions{}),
		DNSProbe(DNSOptions{}),
		GCloudProbe(),
		AWSProbe(),
	}
}

// txtBackend looks up "kcompass.<domain>" as a DNS TXT record and returns a
// backend for the first valid "v=kc1; backend=<url>" value found.
// Returns (nil, nil) when no matching record exists.
func txtBackend(
	ctx context.Context,
	domain string,
	lookupTXT func(context.Context, string) ([]string, error),
) (backend.Backend, error) {
	txts, err := lookupTXT(ctx, "kcompass."+domain)
	if err != nil {
		return nil, nil
	}
	for _, txt := range txts {
		if url, ok := parseTXTRecord(txt); ok {
			b, err := backend.NewBackendFromURL(url)
			if err != nil {
				continue
			}
			return b, nil
		}
	}
	return nil, nil
}
