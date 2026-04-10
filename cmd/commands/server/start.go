package server

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"nathanbeddoewebdev/vpsm/internal/actionstore"
	"nathanbeddoewebdev/vpsm/internal/auditlog"
	"nathanbeddoewebdev/vpsm/internal/server/domain"
	"nathanbeddoewebdev/vpsm/internal/server/providers"
	"nathanbeddoewebdev/vpsm/internal/server/services/action"
	"nathanbeddoewebdev/vpsm/internal/services/auth"

	"github.com/spf13/cobra"
)

// StartCommand returns a cobra.Command that powers on a server.
func StartCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a server",
		Long: `Power on a stopped server instance from the specified provider.

The command waits for the operation to complete by polling the provider
for action progress (or falling back to server-status polling).

The action is persisted locally so that if the CLI is interrupted, the
action can be resumed with "vpsm server actions --resume".

Examples:
  vpsm server start --provider hetzner --id 12345`,
		RunE:         runStart,
		SilenceUsage: true,
	}

	cmd.Flags().String("id", "", "Server ID to start (required)")
	cmd.MarkFlagRequired("id")

	return cmd
}

func runStart(cmd *cobra.Command, args []string) error {
	providerName := cmd.Flag("provider").Value.String()

	provider, err := providers.Get(providerName, auth.DefaultStore())
	if err != nil {
		return err
	}

	serverID, _ := cmd.Flags().GetString("id")

	fmt.Fprintf(cmd.ErrOrStderr(), "Starting server %s...\n", serverID)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	actionStatus, err := provider.StartServer(ctx, serverID)
	if err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	// Open the action repository. If unavailable, repo is set to nil
	// and the service degrades gracefully (no persistence, but operation continues).
	repo, err := actionstore.Open()
	if err != nil {
		repo = nil
	}
	svc := action.NewService(provider, providerName, repo)
	defer svc.Close()

	// Persist the action so it can be resumed if the CLI is interrupted.
	record := svc.TrackAction(serverID, "", actionStatus, "start_server", "running")

	if err := svc.WaitForAction(ctx, actionStatus, serverID, "running", cmd.ErrOrStderr()); err != nil {
		svc.FinalizeAction(record, domain.ActionStatusError, err.Error())
		return err
	}

	svc.FinalizeAction(record, domain.ActionStatusSuccess, "")

	cmd.SetContext(auditlog.WithMetadata(cmd.Context(), auditlog.Metadata{
		Provider:     providerName,
		ResourceType: "server",
		ResourceID:   serverID,
	}))

	fmt.Fprintf(cmd.OutOrStdout(), "Server %s started successfully.\n", serverID)
	return nil
}
