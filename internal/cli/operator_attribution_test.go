package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/nonchord/kcompass/internal/discovery"
)

// TestCorporateDNSDomainsExcludesTailscaleSearchPaths pins the core dedup
// invariant: anything Tailscale is pushing via its own search-path
// advertisement must be attributed to Tailscale, never Corporate DNS, even
// though it shows up in /etc/resolv.conf with no provenance metadata.
func TestCorporateDNSDomainsExcludesTailscaleSearchPaths(t *testing.T) {
	domains := discovery.NetworkDomains{
		Tailscale:            "tail77e803.ts.net",
		TailscaleSearchPaths: []string{"nonchord.com", "tail77e803.ts.net"},
		DNS:                  []string{"tail77e803.ts.net", "nonchord.com", "local"},
	}
	corp := corporateDNSDomains(domains)
	assert.Equal(t, []string{"local"}, corp)
}

func TestCorporateDNSDomainsExcludesNetbird(t *testing.T) {
	domains := discovery.NetworkDomains{
		Netbird: "app.netbird.io",
		DNS:     []string{"app.netbird.io", "internal.company.com"},
	}
	corp := corporateDNSDomains(domains)
	assert.Equal(t, []string{"internal.company.com"}, corp)
}

func TestCorporateDNSDomainsPreservesNonClaimed(t *testing.T) {
	domains := discovery.NetworkDomains{
		Tailscale:            "example.ts.net",
		TailscaleSearchPaths: []string{"example.ts.net"},
		DNS:                  []string{"example.ts.net", "corp.example.com", "another.example.org"},
	}
	corp := corporateDNSDomains(domains)
	assert.Equal(t, []string{"corp.example.com", "another.example.org"}, corp)
}

// TestTailscaleHostDomainsOrdering asserts that the MagicDNS suffix appears
// first when it's in the pushed search-path set, and that there are no
// duplicates when the suffix overlaps with search paths.
func TestTailscaleHostDomainsOrdering(t *testing.T) {
	domains := discovery.NetworkDomains{
		Tailscale:            "tail77e803.ts.net",
		TailscaleSearchPaths: []string{"nonchord.com", "tail77e803.ts.net"},
	}
	got := tailscaleHostDomains(domains)
	assert.Equal(t, []string{"tail77e803.ts.net", "nonchord.com"}, got)
}

func TestTailscaleHostDomainsSuffixOnly(t *testing.T) {
	// Older tailscale versions without `tailscale dns status --json` leave
	// TailscaleSearchPaths empty; fall back to just the MagicDNS suffix.
	domains := discovery.NetworkDomains{
		Tailscale: "tail77e803.ts.net",
	}
	got := tailscaleHostDomains(domains)
	assert.Equal(t, []string{"tail77e803.ts.net"}, got)
}

func TestTailscaleHostDomainsEmpty(t *testing.T) {
	got := tailscaleHostDomains(discovery.NetworkDomains{})
	assert.Nil(t, got)
}
