package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
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
	return &cobra.Command{
		Use:   "dns <url>",
		Short: "Print DNS TXT records to advertise a backend via auto-discovery",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			printDNSRecords(cmd.OutOrStdout(), args[0])
			return nil
		},
	}
}

func printDNSRecords(out io.Writer, backendURL string) {
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
	p("  Corporate DNS  kcompass.<search-domain>")
	p("                 e.g. kcompass.internal.company.com")
	p("  Tailscale      kcompass.<tailnet-magic-dns-suffix>")
	p("                 e.g. kcompass.your-tailnet.ts.net")
	p("  Netbird        kcompass.<management-server-domain>")
	p("                 e.g. kcompass.app.netbird.io")
	p("")
	p("Full example (corporate DNS, replace search domain with yours):")
	pf("  kcompass.internal.company.com. 300 IN TXT %s\n", txtValue)
}
