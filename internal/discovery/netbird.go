package discovery

import (
	"context"
	"encoding/json"
	"fmt"
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
	// LookupTXT performs the kcompass.<mgmt-domain> TXT lookup. If nil, net.DefaultResolver is used.
	LookupTXT func(ctx context.Context, name string) ([]string, error)
	// Log, when non-nil, receives diagnostic messages during probing.
	Log func(string)
}

// netbirdStatusJSON is the subset of `netbird status --json` we need.
type netbirdStatusJSON struct {
	ManagementState struct {
		URL string `json:"URL"`
	} `json:"managementState"`
}

// NetbirdProbe returns a ProbeFunc that detects Netbird, extracts the management
// server domain, and looks up a "kcompass.<domain>" TXT record.
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
	if opts.LookupTXT == nil {
		opts.LookupTXT = net.DefaultResolver.LookupTXT
	}

	return func(ctx context.Context) (backend.Backend, error) {
		data, err := opts.RunStatus(ctx)
		if err != nil {
			if opts.Log != nil {
				opts.Log(fmt.Sprintf("discovery: netbird: not detected (%v)", err))
			}
			return nil, nil
		}
		var st netbirdStatusJSON
		if err := json.Unmarshal(data, &st); err != nil || st.ManagementState.URL == "" {
			if opts.Log != nil {
				opts.Log("discovery: netbird: no management URL in status output")
			}
			return nil, nil
		}
		domain := mgmtURLToDomain(st.ManagementState.URL)
		if domain == "" {
			if opts.Log != nil {
				opts.Log("discovery: netbird: could not parse management domain")
			}
			return nil, nil
		}
		if opts.Log != nil {
			opts.Log(fmt.Sprintf("discovery: netbird: management domain = %s", domain))
		}
		return txtBackend(ctx, "netbird", domain, opts.LookupTXT, opts.Log)
	}
}

// mgmtURLToDomain extracts the hostname from a URL like "https://app.netbird.io:443".
func mgmtURLToDomain(rawURL string) string {
	host, _, err := net.SplitHostPort(stripScheme(rawURL))
	if err != nil {
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
