// Package cli contains the cobra command definitions for kcompass.
package cli

import "github.com/spf13/cobra"

// NewRootCommand builds the root kcompass command with all subcommands registered.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:          "kcompass",
		Short:        "Discover and connect to Kubernetes clusters",
		SilenceUsage: true,
	}
	root.AddCommand(
		NewListCommand(),
		NewConnectCommand(),
		NewInitCommand(),
		NewBackendsCommand(),
	)
	return root
}
