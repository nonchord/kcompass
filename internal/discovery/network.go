package discovery

import (
	"context"
	"encoding/json"
	"net"
	"os/exec"
)

// NetworkDomains holds the domain names detected from each network integration.
// Empty fields indicate the corresponding network was not detected.
type NetworkDomains struct {
	// Tailscale is the MagicDNS suffix (e.g. "your-tailnet.ts.net").
	Tailscale string
	// TailscaleSearchPaths are the DNS search domains that Tailscale itself
	// is pushing to the OS resolver. May include the MagicDNS suffix and
	// any extra paths configured via the admin console's DNS settings.
	// Populated from `tailscale dns status --json`.
	TailscaleSearchPaths []string
	// Netbird is the management server domain (e.g. "app.netbird.io").
	Netbird string
	// DNS holds the search domains from /etc/resolv.conf — a flat list with
	// no provenance, so entries here may have been injected by Tailscale,
	// Netbird, DHCP, or manual configuration. Use TailscaleSearchPaths /
	// Netbird to attribute a given entry to a specific source.
	DNS []string
}

// tailscaleDNSStatusJSON is the subset of `tailscale dns status --json` we need
// to attribute search domains to Tailscale. The command is marked preliminary
// upstream; we parse best-effort and fall back to an empty list on any error.
type tailscaleDNSStatusJSON struct {
	SearchDomains []string `json:"SearchDomains"`
}

// DetectNetworkDomains queries Tailscale, Netbird, and resolv.conf to discover
// which domains are in use. All detection is best-effort; failures produce empty fields.
func DetectNetworkDomains(ctx context.Context) NetworkDomains {
	var nd NetworkDomains

	// Tailscale: query via the CLI (same guard as TailscaleProbe).
	if data, err := exec.CommandContext(ctx, "tailscale", "status", "--json").Output(); err == nil {
		var st tailscaleStatusJSON
		if json.Unmarshal(data, &st) == nil {
			nd.Tailscale = st.MagicDNSSuffix
		}
	}

	// Tailscale pushed search paths: only meaningful when Tailscale is running.
	// On older tailscale versions without `tailscale dns status --json`, the
	// command fails and TailscaleSearchPaths stays nil — the template will
	// then fall back to just the MagicDNS suffix.
	if nd.Tailscale != "" {
		if data, err := exec.CommandContext(ctx, "tailscale", "dns", "status", "--json").Output(); err == nil {
			var st tailscaleDNSStatusJSON
			if json.Unmarshal(data, &st) == nil {
				nd.TailscaleSearchPaths = st.SearchDomains
			}
		}
	}

	// Netbird: only attempt when the wt0 interface is present.
	if _, err := net.InterfaceByName("wt0"); err == nil {
		if data, err := exec.CommandContext(ctx, "netbird", "status", "--json").Output(); err == nil {
			var st netbirdStatusJSON
			if json.Unmarshal(data, &st) == nil {
				nd.Netbird = mgmtURLToDomain(st.ManagementState.URL)
			}
		}
	}

	// DNS search domains from resolv.conf.
	nd.DNS = searchDomainsFromFile("/etc/resolv.conf")

	return nd
}
