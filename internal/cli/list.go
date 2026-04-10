package cli

import (
	"encoding/json"
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

			records, err := reg.List(cmd.Context())
			if err != nil {
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
