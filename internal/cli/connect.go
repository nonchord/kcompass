package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/nonchord/kcompass/internal/backend"
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
				if errors.Is(err, backend.ErrAccessDenied) {
					cmd.SilenceErrors = true
					cmd.SilenceUsage = true
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(),
						"You don't have access to this cluster inventory.")
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(),
						"Check that your SSH key or git token is configured for the backend,")
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(),
						"and that you've been granted access to the repository.")
					return err
				}
				return fmt.Errorf("connect: %w", err)
			}

			noSwitch, _ := cmd.Flags().GetBool("no-switch")
			switchCtx := !noSwitch

			kubeconfigPath, err := defaultKubeconfigPath()
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Setting up kubeconfig for %s... ", name)

			data, err := resolveKubeconfig(cmd.Context(), rec)
			if err != nil {
				return fmt.Errorf("connect: %w", err)
			}

			ctxName, err := kubeconfig.MergeStatic(kubeconfigPath, data, switchCtx)
			if err != nil {
				return fmt.Errorf("connect: %w", err)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Done.")
			if switchCtx {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Context is set to %s.\n", ctxName)
			}
			return nil
		},
	}
	cmd.Flags().Bool("no-switch", false, "merge credentials but do not change the current context")
	return cmd
}

// resolveKubeconfig produces a kubeconfig fragment ready to merge.
//
// For inline specs, returns the embedded blob unchanged.
// For command specs, runs the command with KUBECONFIG set to a temp file and
// reads the result. stdin/stdout/stderr are passed through so interactive
// prompts (e.g. gcloud's browser auth flow) work normally.
func resolveKubeconfig(ctx context.Context, rec *backend.ClusterRecord) ([]byte, error) {
	if err := rec.Validate(); err != nil {
		return nil, err
	}
	if rec.Kubeconfig.Inline != "" {
		return []byte(rec.Kubeconfig.Inline), nil
	}

	tmp, err := os.CreateTemp("", "kcompass-kubeconfig-*")
	if err != nil {
		return nil, fmt.Errorf("create temp kubeconfig: %w", err)
	}
	tmpName := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(tmpName) }()

	argv := rec.Kubeconfig.Command
	c := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // command is operator-supplied inventory, not user input
	c.Env = append(os.Environ(), "KUBECONFIG="+tmpName)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return nil, fmt.Errorf("run %s: %w", argv[0], err)
	}

	data, err := os.ReadFile(tmpName)
	if err != nil {
		return nil, fmt.Errorf("read generated kubeconfig: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%s ran successfully but produced no kubeconfig at $KUBECONFIG", argv[0])
	}
	return data, nil
}
