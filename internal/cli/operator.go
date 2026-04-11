package cli

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/spf13/cobra"

	"github.com/nonchord/kcompass/internal/discovery"
)

// NewOperatorCommand creates the `kcompass operator` subcommand group.
func NewOperatorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "operator",
		Short: "Tools for operators publishing a kcompass registry",
	}
	cmd.AddCommand(NewOperatorDNSCommand())
	cmd.AddCommand(NewOperatorAddCommand())
	return cmd
}

// NewOperatorDNSCommand creates the `kcompass operator dns <url>` command.
func NewOperatorDNSCommand() *cobra.Command {
	var (
		verify    bool
		hostnames []string
	)
	cmd := &cobra.Command{
		Use:   "dns <url>",
		Short: "Print DNS TXT records to advertise a backend via auto-discovery",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			domains := discovery.DetectNetworkDomains(cmd.Context())
			printDNSRecords(cmd.OutOrStdout(), args[0], domains)
			if verify || len(hostnames) > 0 {
				verifyDNSRecords(cmd.Context(), cmd.OutOrStdout(), args[0], domains, hostnames)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&verify, "verify", false, "verify that TXT records are published and match")
	cmd.Flags().StringSliceVar(&hostnames, "hostname", nil,
		"verify these FQDNs instead of the auto-detected ones (repeatable, implies --verify)")
	return cmd
}

// corporateDNSDomains returns the subset of domains.DNS that is not claimed by
// a more specific source (Tailscale or Netbird). When Tailscale pushes search
// paths to the OS resolver (visible via `tailscale dns status --json`), those
// entries appear in /etc/resolv.conf with no provenance; attributing them back
// to Tailscale reads cleaner and avoids the same hostname showing up twice in
// the output.
func corporateDNSDomains(domains discovery.NetworkDomains) []string {
	claimed := map[string]bool{}
	if domains.Tailscale != "" {
		claimed[domains.Tailscale] = true
	}
	for _, d := range domains.TailscaleSearchPaths {
		claimed[d] = true
	}
	if domains.Netbird != "" {
		claimed[domains.Netbird] = true
	}
	var out []string
	for _, d := range domains.DNS {
		if claimed[d] {
			continue
		}
		out = append(out, d)
	}
	return out
}

// tailscaleHostDomains returns the list of domains that should be printed
// under the "Tailscale" row — the MagicDNS suffix plus any extra search
// paths Tailscale is pushing, deduplicated and stable-ordered (MagicDNS
// suffix first when it's in the set).
func tailscaleHostDomains(domains discovery.NetworkDomains) []string {
	if domains.Tailscale == "" && len(domains.TailscaleSearchPaths) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(d string) {
		if d == "" || seen[d] {
			return
		}
		seen[d] = true
		out = append(out, d)
	}
	add(domains.Tailscale)
	for _, d := range domains.TailscaleSearchPaths {
		add(d)
	}
	return out
}

func printDNSRecords(out io.Writer, backendURL string, domains discovery.NetworkDomains) {
	txtValue := fmt.Sprintf(`"v=kc1; backend=%s"`, backendURL)

	p := func(s string) { _, _ = fmt.Fprintln(out, s) }
	pf := func(format string, a ...interface{}) { _, _ = fmt.Fprintf(out, format, a...) }

	p("Add a DNS TXT record so users on your network can run `kcompass list`")
	p("with zero configuration. The record value is the same for all network types;")
	p("only the hostname differs:")
	p("")
	pf("  TXT value:  %s\n", txtValue)
	p("")
	p("  Network        Hostname")
	p("  ─────────────────────────────────────────────────────────────────")

	corpDomains := corporateDNSDomains(domains)
	if len(corpDomains) > 0 {
		pf("  Corporate DNS  kcompass.%s\n", corpDomains[0])
		for _, d := range corpDomains[1:] {
			pf("                 kcompass.%s\n", d)
		}
	} else {
		p("  Corporate DNS  kcompass.<search-domain>")
		p("                 e.g. kcompass.internal.company.com")
	}

	if tsDomains := tailscaleHostDomains(domains); len(tsDomains) > 0 {
		pf("  Tailscale      kcompass.%s\n", tsDomains[0])
		for _, d := range tsDomains[1:] {
			pf("                 kcompass.%s\n", d)
		}
	} else {
		p("  Tailscale      kcompass.<tailnet-magic-dns-suffix>")
		p("                 e.g. kcompass.your-tailnet.ts.net")
	}

	if domains.Netbird != "" {
		pf("  Netbird        kcompass.%s\n", domains.Netbird)
	} else {
		p("  Netbird        kcompass.<management-server-domain>")
		p("                 e.g. kcompass.app.netbird.io")
	}

	p("")
	p("Full example (corporate DNS, replace search domain with yours):")
	exampleDomain := "internal.company.com"
	if len(corpDomains) > 0 {
		exampleDomain = corpDomains[0]
	}
	pf("  kcompass.%s. 300 IN TXT %s\n", exampleDomain, txtValue)
}

// verifyDNSRecords performs live TXT lookups and prints whether the expected
// kcompass record is present and correct. When explicitHostnames is non-empty,
// only those FQDNs are checked (and the detected domain list is ignored);
// otherwise the detected domain list is used, with duplicates removed so the
// same hostname is never queried twice when multiple probes happen to agree.
func verifyDNSRecords(
	ctx context.Context,
	out io.Writer,
	backendURL string,
	domains discovery.NetworkDomains,
	explicitHostnames []string,
) {
	p := func(s string) { _, _ = fmt.Fprintln(out, s) }
	pf := func(format string, a ...interface{}) { _, _ = fmt.Fprintf(out, format, a...) }

	type entry struct {
		label    string
		hostname string
	}

	var (
		checks []entry
		seen   = map[string]bool{}
	)
	add := func(label, hostname string) {
		if seen[hostname] {
			return
		}
		seen[hostname] = true
		checks = append(checks, entry{label, hostname})
	}

	if len(explicitHostnames) > 0 {
		for _, h := range explicitHostnames {
			add("Explicit", h)
		}
	} else {
		// Attribute Tailscale-pushed search paths to Tailscale, not Corporate
		// DNS, even though they're present in /etc/resolv.conf. Order matters:
		// adding Tailscale first means any subsequent Corporate DNS entry with
		// the same hostname is silently skipped by the dedup in `add`.
		for _, d := range tailscaleHostDomains(domains) {
			add("Tailscale", "kcompass."+d)
		}
		if domains.Netbird != "" {
			add("Netbird", "kcompass."+domains.Netbird)
		}
		for _, d := range corporateDNSDomains(domains) {
			add("Corporate DNS", "kcompass."+d)
		}
	}

	p("")
	p("Verifying TXT records...")

	if len(checks) == 0 {
		p("")
		p("  No network domains detected — connect to a managed network and try again,")
		p("  or pass --hostname <fqdn> to verify a specific record.")
		return
	}

	p("")
	for _, c := range checks {
		txts, err := net.DefaultResolver.LookupTXT(ctx, c.hostname)
		if err != nil {
			pf("  %-15s %s\n", c.label, c.hostname+" — not found")
			continue
		}
		matched := false
		for _, txt := range txts {
			url, ok := discovery.ParseTXTRecord(txt)
			if !ok {
				continue
			}
			if url == backendURL {
				pf("  %-15s %s — OK\n", c.label, c.hostname)
			} else {
				pf("  %-15s %s — mismatch (found %q, expected %q)\n", c.label, c.hostname, url, backendURL)
			}
			matched = true
			break
		}
		if !matched {
			pf("  %-15s %s — no kcompass record\n", c.label, c.hostname)
		}
	}
}
