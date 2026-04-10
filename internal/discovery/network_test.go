package discovery_test

import (
	"context"
	"strings"
	"testing"

	"github.com/nonchord/kcompass/internal/discovery"
)

// TestDetectNetworkDomainsRunsWithoutPanic verifies the function completes
// without panicking in a CI environment that has no Tailscale/Netbird.
func TestDetectNetworkDomainsRunsWithoutPanic(_ *testing.T) {
	nd := discovery.DetectNetworkDomains(context.Background())
	// Tailscale and Netbird are not expected in CI — just verify no panic.
	_ = nd.Tailscale
	_ = nd.Netbird
	_ = nd.DNS
}

func TestTailscaleProbeLogging(t *testing.T) {
	var logs []string
	logFn := func(s string) { logs = append(logs, s) }

	probe := discovery.TailscaleProbe(discovery.TailscaleOptions{
		RunStatus: tailscaleStatus("tailnet.ts.net"),
		LookupTXT: mockTXT(map[string][]string{
			"kcompass.tailnet.ts.net": {"v=kc1; backend=git@github.com:company/clusters"},
		}),
		Log: logFn,
	})
	b, err := probe(context.Background())
	if err != nil || b == nil {
		t.Fatal("expected backend to be found")
	}
	if len(logs) == 0 {
		t.Fatal("expected log messages to be emitted")
	}
	hasDomain := false
	for _, l := range logs {
		if strings.Contains(l, "tailnet.ts.net") {
			hasDomain = true
			break
		}
	}
	if !hasDomain {
		t.Errorf("expected domain in log output, got: %v", logs)
	}
}

func TestNetbirdProbeLogging(t *testing.T) {
	var logs []string
	logFn := func(s string) { logs = append(logs, s) }

	probe := discovery.NetbirdProbe(discovery.NetbirdOptions{
		DetectInterface: func() bool { return true },
		RunStatus:       netbirdStatus("https://app.netbird.io:443"),
		LookupTXT: mockTXT(map[string][]string{
			"kcompass.app.netbird.io": {"v=kc1; backend=git@github.com:company/clusters"},
		}),
		Log: logFn,
	})
	b, err := probe(context.Background())
	if err != nil || b == nil {
		t.Fatal("expected backend to be found")
	}
	if len(logs) == 0 {
		t.Fatal("expected log messages to be emitted")
	}
}

func TestDNSProbeLogging(t *testing.T) {
	var logs []string
	logFn := func(s string) { logs = append(logs, s) }

	probe := discovery.DNSProbe(discovery.DNSOptions{
		SearchDomains: []string{"corp.example.com"},
		LookupTXT: mockTXT(map[string][]string{
			"kcompass.corp.example.com": {"v=kc1; backend=git@github.com:company/clusters"},
		}),
		Log: logFn,
	})
	b, err := probe(context.Background())
	if err != nil || b == nil {
		t.Fatal("expected backend to be found")
	}
	if len(logs) == 0 {
		t.Fatal("expected log messages to be emitted")
	}
}

func TestDNSProbeLoggingNoDomains(t *testing.T) {
	var logs []string
	logFn := func(s string) { logs = append(logs, s) }

	probe := discovery.DNSProbe(discovery.DNSOptions{
		SearchDomains: []string{},
		LookupTXT:     mockTXT(map[string][]string{}),
		Log:           logFn,
	})
	b, err := probe(context.Background())
	if err != nil || b != nil {
		t.Fatal("expected nil backend")
	}
	if len(logs) == 0 {
		t.Fatal("expected at least one log message about no search domains")
	}
}

func TestProvenanceInBackendName(t *testing.T) {
	probe := discovery.TailscaleProbe(discovery.TailscaleOptions{
		RunStatus: tailscaleStatus("tailnet.ts.net"),
		LookupTXT: mockTXT(map[string][]string{
			"kcompass.tailnet.ts.net": {"v=kc1; backend=git@github.com:company/clusters"},
		}),
	})
	b, err := probe(context.Background())
	if err != nil || b == nil {
		t.Fatal("expected backend to be found")
	}
	name := b.Name()
	if !strings.HasPrefix(name, "tailscale:") {
		t.Errorf("expected backend name to have tailscale: prefix, got %q", name)
	}
	if !strings.Contains(name, "github.com") {
		t.Errorf("expected backend name to contain URL, got %q", name)
	}
}

func TestNetbirdProvenanceInBackendName(t *testing.T) {
	probe := discovery.NetbirdProbe(discovery.NetbirdOptions{
		DetectInterface: func() bool { return true },
		RunStatus:       netbirdStatus("https://app.netbird.io:443"),
		LookupTXT: mockTXT(map[string][]string{
			"kcompass.app.netbird.io": {"v=kc1; backend=git@github.com:company/clusters"},
		}),
	})
	b, err := probe(context.Background())
	if err != nil || b == nil {
		t.Fatal("expected backend to be found")
	}
	name := b.Name()
	if !strings.HasPrefix(name, "netbird:") {
		t.Errorf("expected backend name to have netbird: prefix, got %q", name)
	}
}

func TestDNSProvenanceInBackendName(t *testing.T) {
	probe := discovery.DNSProbe(discovery.DNSOptions{
		SearchDomains: []string{"corp.example.com"},
		LookupTXT: mockTXT(map[string][]string{
			"kcompass.corp.example.com": {"v=kc1; backend=git@github.com:company/clusters"},
		}),
	})
	b, err := probe(context.Background())
	if err != nil || b == nil {
		t.Fatal("expected backend to be found")
	}
	name := b.Name()
	if !strings.HasPrefix(name, "dns:") {
		t.Errorf("expected backend name to have dns: prefix, got %q", name)
	}
	if !strings.Contains(name, "corp.example.com") {
		t.Errorf("expected backend name to contain the source domain, got %q", name)
	}
}
