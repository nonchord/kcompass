package cli

import "github.com/spf13/cobra"

// NewBackendsCommand creates the `kcompass backends` command.
func NewBackendsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "backends",
		Short: "List configured backends and their status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println("not implemented")
			return nil
		},
	}
}
