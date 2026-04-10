package discovery

import (
	"context"
	"encoding/json"
	"net"
	"os/exec"

	"github.com/nonchord/kcompass/internal/backend"
)

// NetbirdOptions controls how the Netbird probe works.
// All fields are optional; zero values select production defaults.
type NetbirdOptions struct {
	// DetectInterface reports whether the Netbird WireGuard interface (wt0)
	// is present. If nil, net.InterfaceByName("wt0") is used.
	DetectInterface func() bool
	// RunStatus runs "netbird status --json". If nil, exec.CommandContext is used.
	// In production this is only attempted when DetectInterface returns true.
	RunStatus func(ctx context.Context) ([]byte, error)
	// LookupSRV performs the _kcompass._tcp SRV lookup. If nil, net.DefaultResolver is used.
	LookupSRV func(ctx context.Context, service, proto, name string) (string, []*net.SRV, error)
}

// netbirdStatusJSON is the subset of `netbird status --json` we need.
// The Management URL contains the domain used for the SRV lookup.
type netbirdStatusJSON struct {
	ManagementState struct {
		URL string `json:"URL"`
	} `json:"managementState"`
}

// NetbirdProbe returns a ProbeFunc that detects Netbird and performs an
// SRV lookup for _kcompass._tcp.<management-domain>.
func NetbirdProbe(opts NetbirdOptions) ProbeFunc {
	if opts.DetectInterface == nil {
		opts.DetectInterface = func() bool {
			_, err := net.InterfaceByName("wt0")
			return err == nil
		}
	}

	detect := opts.DetectInterface
	if opts.RunStatus == nil {
		opts.RunStatus = func(ctx context.Context) ([]byte, error) {
			if !detect() {
				return nil, errNotFound
			}
			return exec.CommandContext(ctx, "netbird", "status", "--json").Output()
		}
	}
	if opts.LookupSRV == nil {
		opts.LookupSRV = net.DefaultResolver.LookupSRV
	}

	return func(ctx context.Context) (backend.Backend, error) {
		data, err := opts.RunStatus(ctx)
		if err != nil {
			return nil, nil
		}
		var st netbirdStatusJSON
		if err := json.Unmarshal(data, &st); err != nil || st.ManagementState.URL == "" {
			return nil, nil
		}
		// Extract the hostname from the management URL.
		domain := mgmtURLToDomain(st.ManagementState.URL)
		if domain == "" {
			return nil, nil
		}
		return srvToHTTPBackend(ctx, domain, opts.LookupSRV)
	}
}

// mgmtURLToDomain extracts the hostname from a URL like "https://app.netbird.io:443".
func mgmtURLToDomain(rawURL string) string {
	host, _, err := net.SplitHostPort(stripScheme(rawURL))
	if err != nil {
		// No port present; treat whole stripped value as host.
		host = stripScheme(rawURL)
	}
	return host
}

func stripScheme(u string) string {
	for _, prefix := range []string{"https://", "http://"} {
		if len(u) > len(prefix) && u[:len(prefix)] == prefix {
			return u[len(prefix):]
		}
	}
	return u
}

// errNotFound is a sentinel used internally to signal "not present".
var errNotFound = net.UnknownNetworkError("not found")
