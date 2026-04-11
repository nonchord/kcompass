package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/nonchord/kcompass/internal/backend"
)

// NewListCommand creates the `kcompass list` command.
func NewListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all clusters across configured backends",
		RunE: func(cmd *cobra.Command, _ []string) error {
			reg, err := registryFromContext(cmd)
			if err != nil {
				return err
			}

			if len(reg.Backends()) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(),
					"No cluster registry found. Run `kcompass init <url>` to configure a backend,\nor connect to your company VPN/Tailscale network and try again.")
				return nil
			}

			records, err := reg.List(cmd.Context())
			if err != nil {
				if errors.Is(err, backend.ErrAccessDenied) {
					printAccessDenied(cmd)
					return err
				}
				return fmt.Errorf("list: %w", err)
			}

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(records)
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
			_, _ = fmt.Fprintln(w, "NAME\tDESCRIPTION")
			for _, r := range records {
				_, _ = fmt.Fprintf(w, "%s\t%s\n", r.Name, r.Description)
			}
			return w.Flush()
		},
	}
	cmd.Flags().String("backend", "", "restrict output to a specific backend")
	cmd.Flags().Bool("json", false, "emit JSON for scripting")
	return cmd
}

// registryFromContext retrieves the *backend.Registry stored by the root
// command's PersistentPreRunE.
func registryFromContext(cmd *cobra.Command) (*backend.Registry, error) {
	reg, ok := cmd.Context().Value(RegistryKey{}).(*backend.Registry)
	if !ok || reg == nil {
		return nil, fmt.Errorf("no backend registry available; run `kcompass init <path>` to configure a backend")
	}
	return reg, nil
}

// printAccessDenied writes the friendly "you don't have access" message to
// stderr and silences cobra's default error printing so the caller can return
// the error with a non-zero exit without also emitting the raw wrapped form.
// Used by list, connect, and init when they detect backend.ErrAccessDenied.
func printAccessDenied(cmd *cobra.Command) {
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	_, _ = fmt.Fprintln(cmd.ErrOrStderr(),
		"You don't have access to this cluster inventory.")
	_, _ = fmt.Fprintln(cmd.ErrOrStderr(),
		"Check that your SSH key or git token is configured for the backend,")
	_, _ = fmt.Fprintln(cmd.ErrOrStderr(),
		"and that you've been granted access to the repository.")
}
