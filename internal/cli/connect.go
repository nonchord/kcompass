package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nonchord/kcompass/internal/kubeconfig"
)

// NewConnectCommand creates the `kcompass connect` command.
func NewConnectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect <name>",
		Short: "Acquire credentials for a cluster and update kubeconfig",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, err := registryFromContext(cmd)
			if err != nil {
				return err
			}

			name := args[0]
			rec, err := reg.Get(cmd.Context(), name)
			if err != nil {
				return fmt.Errorf("connect: %w", err)
			}

			noSwitch, _ := cmd.Flags().GetBool("no-switch")
			switchCtx := !noSwitch

			kubeconfigPath, err := defaultKubeconfigPath()
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Setting up kubeconfig for %s... ", name)

			switch rec.Auth {
			case "static":
				blob, ok := rec.Metadata["kubeconfig"]
				if !ok {
					return fmt.Errorf("connect: cluster %q has auth=static but no kubeconfig metadata field", name)
				}
				ctx, mergeErr := kubeconfig.MergeStatic(kubeconfigPath, []byte(blob), switchCtx)
				if mergeErr != nil {
					return fmt.Errorf("connect: %w", mergeErr)
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Done.")
				if switchCtx {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Context is set to %s.\n", ctx)
				}
			default:
				return fmt.Errorf("connect: auth method %q is not yet supported", rec.Auth)
			}
			return nil
		},
	}
	cmd.Flags().Bool("no-switch", false, "merge credentials but do not change the current context")
	return cmd
}
