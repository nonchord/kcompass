package cli

import "github.com/spf13/cobra"

// NewListCommand creates the `kcompass list` command.
func NewListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all clusters across configured backends",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println("not implemented")
			return nil
		},
	}
	cmd.Flags().String("backend", "", "restrict output to a specific backend")
	cmd.Flags().Bool("json", false, "emit JSON for scripting")
	return cmd
}
