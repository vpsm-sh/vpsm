package server

import (
	"context"
	"fmt"
	"os"

	"nathanbeddoewebdev/vpsm/internal/auditlog"
	"nathanbeddoewebdev/vpsm/internal/server/providers"
	"nathanbeddoewebdev/vpsm/internal/server/tui"
	"nathanbeddoewebdev/vpsm/internal/services/auth"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func DeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a server",
		Long: `Delete a server instance from the specified provider.

If --id is not provided, an interactive TUI will let you select a server
from the current list. The TUI shows a summary and asks for confirmation
before deleting. Requires a terminal; use --id for scripting.

Examples:
  # Interactive mode (TUI)
  vpsm server delete --provider hetzner

  # Non-interactive (scripting)
  vpsm server delete --provider hetzner --id 12345`,
		RunE:         runDelete,
		SilenceUsage: true,
	}

	cmd.Flags().String("id", "", "Server ID to delete (skips interactive selection)")

	return cmd
}

func runDelete(cmd *cobra.Command, args []string) error {
	providerName := cmd.Flag("provider").Value.String()

	provider, err := providers.Get(providerName, auth.DefaultStore())
	if err != nil {
		return err
	}

	serverID, _ := cmd.Flags().GetString("id")

	if serverID == "" {
		// Interactive mode requires a terminal.
		if !term.IsTerminal(int(os.Stdout.Fd())) {
			return fmt.Errorf("--id is required when not running in a terminal")
		}

		result, err := tui.RunServerDelete(provider, providerName, nil)
		if err != nil {
			return err
		}
		if result == nil || !result.Confirmed {
			fmt.Fprintln(cmd.ErrOrStderr(), "Server deletion cancelled.")
			return nil
		}

		serverID = result.Server.ID
		cmd.SetContext(auditlog.WithMetadata(cmd.Context(), auditlog.Metadata{
			Provider:     providerName,
			ResourceType: "server",
			ResourceID:   serverID,
			ResourceName: result.Server.Name,
		}))
		fmt.Fprintf(cmd.ErrOrStderr(), "Deleting server %q (ID: %s)...\n", result.Server.Name, serverID)
	} else {
		cmd.SetContext(auditlog.WithMetadata(cmd.Context(), auditlog.Metadata{
			Provider:     providerName,
			ResourceType: "server",
			ResourceID:   serverID,
		}))
		fmt.Fprintf(cmd.ErrOrStderr(), "Deleting server %s...\n", serverID)
	}

	ctx := context.Background()
	if err := provider.DeleteServer(ctx, serverID); err != nil {
		return fmt.Errorf("failed to delete server: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Server %s deleted successfully.\n", serverID)
	return nil
}
