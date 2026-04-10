package cli

import "github.com/spf13/cobra"

// NewInitCommand creates the `kcompass init` command.
func NewInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init <url>",
		Short: "Register a backend by URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println("not implemented")
			return nil
		},
	}
}
