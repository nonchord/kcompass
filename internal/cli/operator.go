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
	return cmd
}

// NewOperatorDNSCommand creates the `kcompass operator dns <url>` command.
func NewOperatorDNSCommand() *cobra.Command {
	var verify bool
	cmd := &cobra.Command{
		Use:   "dns <url>",
		Short: "Print DNS TXT records to advertise a backend via auto-discovery",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			domains := discovery.DetectNetworkDomains(cmd.Context())
			printDNSRecords(cmd.OutOrStdout(), args[0], domains)
			if verify {
				verifyDNSRecords(cmd.Context(), cmd.OutOrStdout(), args[0], domains)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&verify, "verify", false, "verify that TXT records are published and match")
	return cmd
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

	if len(domains.DNS) > 0 {
		pf("  Corporate DNS  kcompass.%s\n", domains.DNS[0])
		for _, d := range domains.DNS[1:] {
			pf("                 kcompass.%s\n", d)
		}
	} else {
		p("  Corporate DNS  kcompass.<search-domain>")
		p("                 e.g. kcompass.internal.company.com")
	}

	if domains.Tailscale != "" {
		pf("  Tailscale      kcompass.%s\n", domains.Tailscale)
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
	if len(domains.DNS) > 0 {
		exampleDomain = domains.DNS[0]
	}
	pf("  kcompass.%s. 300 IN TXT %s\n", exampleDomain, txtValue)
}

// verifyDNSRecords performs live TXT lookups for each detected domain and
// prints whether the expected kcompass record is present and correct.
func verifyDNSRecords(ctx context.Context, out io.Writer, backendURL string, domains discovery.NetworkDomains) {
	p := func(s string) { _, _ = fmt.Fprintln(out, s) }
	pf := func(format string, a ...interface{}) { _, _ = fmt.Fprintf(out, format, a...) }

	type entry struct {
		label    string
		hostname string
	}

	var checks []entry
	for _, d := range domains.DNS {
		checks = append(checks, entry{"Corporate DNS", "kcompass." + d})
	}
	if domains.Tailscale != "" {
		checks = append(checks, entry{"Tailscale", "kcompass." + domains.Tailscale})
	}
	if domains.Netbird != "" {
		checks = append(checks, entry{"Netbird", "kcompass." + domains.Netbird})
	}

	p("")
	p("Verifying TXT records...")

	if len(checks) == 0 {
		p("")
		p("  No network domains detected — connect to a managed network and try again.")
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
