// Package discovery implements auto-discovery of kcompass backends.
// It runs a set of probes in parallel and collects the backends they find.
// Each probe is best-effort: errors are silently discarded.
package discovery

import (
	"context"
	"fmt"
	"net"
	"strings"
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

// srvToHTTPBackend performs an SRV lookup for _kcompass._tcp.<domain> and
// returns an HTTP backend pointed at the highest-priority result.
// Returns (nil, nil) when the lookup finds nothing.
func srvToHTTPBackend(
	ctx context.Context,
	domain string,
	lookupSRV func(ctx context.Context, service, proto, name string) (string, []*net.SRV, error),
) (backend.Backend, error) {
	_, addrs, err := lookupSRV(ctx, "kcompass", "tcp", domain)
	if err != nil || len(addrs) == 0 {
		return nil, nil
	}
	srv := addrs[0]
	target := strings.TrimSuffix(srv.Target, ".")
	url := fmt.Sprintf("https://%s:%d", target, srv.Port)
	return backend.NewHTTPBackend(backend.HTTPBackendConfig{
		Name: "discovery:srv:" + domain,
		URL:  url,
	}), nil
}
