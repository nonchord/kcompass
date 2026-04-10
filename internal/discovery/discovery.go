// Package discovery implements auto-discovery of kcompass backends.
// It runs a set of probes in parallel and collects the backends they find.
// Each probe is best-effort: errors are silently discarded.
package discovery

import (
	"context"
	"fmt"
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
// Pass an optional log function to receive per-probe diagnostic messages.
func DefaultProbes(log ...func(string)) []ProbeFunc {
	var logFn func(string)
	if len(log) > 0 {
		logFn = log[0]
	}
	return []ProbeFunc{
		TailscaleProbe(TailscaleOptions{Log: logFn}),
		NetbirdProbe(NetbirdOptions{Log: logFn}),
		DNSProbe(DNSOptions{Log: logFn}),
		GCloudProbe(),
		AWSProbe(),
	}
}

// txtBackend looks up "kcompass.<domain>" as a DNS TXT record and returns a
// backend for the first valid "v=kc1; backend=<url>" value found.
// sourceName is used as the backend name prefix (e.g. "tailscale", "dns:corp.example.com")
// and as the subject of any log messages.
// Returns (nil, nil) when no matching record exists.
func txtBackend(
	ctx context.Context,
	sourceName string,
	domain string,
	lookupTXT func(context.Context, string) ([]string, error),
	log func(string),
) (backend.Backend, error) {
	hostname := "kcompass." + domain
	txts, err := lookupTXT(ctx, hostname)
	if err != nil {
		if log != nil {
			log(fmt.Sprintf("discovery: %s: TXT lookup for %s failed: %v", sourceName, hostname, err))
		}
		return nil, nil
	}
	for _, txt := range txts {
		if url, ok := ParseTXTRecord(txt); ok {
			name := sourceName + ":" + url
			b, buildErr := backend.NewNamedBackendFromURL(name, url)
			if buildErr != nil {
				continue
			}
			if log != nil {
				log(fmt.Sprintf("discovery: %s: found backend %q via %s", sourceName, url, hostname))
			}
			return b, nil
		}
	}
	if log != nil {
		log(fmt.Sprintf("discovery: %s: no kcompass TXT record at %s", sourceName, hostname))
	}
	return nil, nil
}
