package cli

import (
	"context"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/nonchord/kcompass/internal/backend"
)

// NewBackendsCommand creates the `kcompass backends` command.
func NewBackendsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "backends",
		Short: "List configured backends and their status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			reg, err := registryFromContext(cmd)
			if err != nil {
				return err
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
			_, _ = fmt.Fprintln(w, "NAME\tSTATUS")
			for _, b := range reg.Backends() {
				status := backendStatus(cmd.Context(), b)
				_, _ = fmt.Fprintf(w, "%s\t%s\n", b.Name(), status)
			}
			return w.Flush()
		},
	}
}

func backendStatus(ctx context.Context, b backend.Backend) string {
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := b.List(probe)
	if err != nil {
		msg := err.Error()
		if len(msg) > 40 {
			msg = msg[:40] + "..."
		}
		return "error: " + msg
	}
	return "ok"
}
