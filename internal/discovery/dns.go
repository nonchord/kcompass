package discovery

import (
	"bufio"
	"context"
	"io"
	"net"
	"os"
	"strings"

	"github.com/nonchord/kcompass/internal/backend"
)

// DNSOptions controls how the DNS probe works.
// All fields are optional; zero values select production defaults.
type DNSOptions struct {
	// ResolvConf is the path to the resolver config file.
	// Default: /etc/resolv.conf. Ignored when SearchDomains is set.
	ResolvConf string
	// SearchDomains, when non-nil, overrides reading from ResolvConf.
	SearchDomains []string
	// LookupTXT performs a TXT lookup. If nil, net.DefaultResolver is used.
	LookupTXT func(ctx context.Context, name string) ([]string, error)
}

// DNSProbe returns a ProbeFunc that reads DNS search domains and looks up
// TXT records of the form "v=kc1; backend=<url>" at kcompass.<domain>.
// The first matching record wins.
func DNSProbe(opts DNSOptions) ProbeFunc {
	if opts.ResolvConf == "" {
		opts.ResolvConf = "/etc/resolv.conf"
	}
	if opts.LookupTXT == nil {
		opts.LookupTXT = net.DefaultResolver.LookupTXT
	}

	return func(ctx context.Context) (backend.Backend, error) {
		domains := opts.SearchDomains
		if domains == nil {
			domains = searchDomainsFromFile(opts.ResolvConf)
		}

		for _, domain := range domains {
			txts, err := opts.LookupTXT(ctx, "kcompass."+domain)
			if err != nil {
				continue
			}
			for _, txt := range txts {
				if url, ok := parseTXTRecord(txt); ok {
					return backend.NewHTTPBackend(backend.HTTPBackendConfig{
						Name: "discovery:dns:" + domain,
						URL:  url,
					}), nil
				}
			}
		}
		return nil, nil
	}
}

// searchDomainsFromFile reads DNS search domains from a resolv.conf file.
// Returns nil on any read error (best-effort).
func searchDomainsFromFile(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	return parseResolvConf(f)
}

// parseResolvConf extracts search/domain entries from a resolv.conf reader.
func parseResolvConf(r io.Reader) []string {
	var domains []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if fields[0] == "search" || fields[0] == "domain" {
			domains = append(domains, fields[1:]...)
		}
	}
	return domains
}

// parseTXTRecord parses a DNS TXT record of the form "v=kc1; backend=<url>".
// Returns the URL and true on success.
func parseTXTRecord(txt string) (string, bool) {
	if !strings.HasPrefix(strings.TrimSpace(txt), "v=kc1") {
		return "", false
	}
	for _, part := range strings.Split(txt, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "backend=") {
			url := strings.TrimSpace(strings.TrimPrefix(part, "backend="))
			if url != "" {
				return url, true
			}
		}
	}
	return "", false
}
