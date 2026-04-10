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
	// Netbird is the management server domain (e.g. "app.netbird.io").
	Netbird string
	// DNS holds the search domains from /etc/resolv.conf.
	DNS []string
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
