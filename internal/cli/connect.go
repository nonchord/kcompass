package cli

import "github.com/spf13/cobra"

// NewConnectCommand creates the `kcompass connect` command.
func NewConnectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect <name>",
		Short: "Acquire credentials for a cluster and update kubeconfig",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println("not implemented")
			return nil
		},
	}
	cmd.Flags().Bool("no-switch", false, "merge credentials but do not change the current context")
	return cmd
}
