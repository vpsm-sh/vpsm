package audit

import "github.com/spf13/cobra"

// NewCommand returns the "audit" parent command.
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "View and manage audit history",
		Long: "View a local audit trail of vpsm commands and prune old entries.\n\n" +
			"Audit history is stored locally in ~/.config/vpsm/vpsm.db.",
		SilenceUsage: true,
	}

	cmd.AddCommand(ListCommand())
	cmd.AddCommand(PruneCommand())

	return cmd
}
