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

// StopCommand returns a cobra.Command that gracefully shuts down a server.
func StopCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop a running server",
		Long: `Gracefully shut down a running server instance.

The command waits for the operation to complete by polling the provider.
If the provider supports action tracking (e.g. Hetzner), progress is
reported via the action API. Otherwise, the server's status is polled
until it reaches "off".

The action is persisted locally so that if the CLI is interrupted, the
action can be resumed with "vpsm server actions --resume".

Examples:
  vpsm server stop --provider hetzner --id 12345`,
		RunE:         runStop,
		SilenceUsage: true,
	}

	cmd.Flags().String("id", "", "Server ID to stop (required)")
	cmd.MarkFlagRequired("id")

	return cmd
}

func runStop(cmd *cobra.Command, args []string) error {
	providerName := cmd.Flag("provider").Value.String()

	provider, err := providers.Get(providerName, auth.DefaultStore())
	if err != nil {
		return err
	}

	serverID, _ := cmd.Flags().GetString("id")

	fmt.Fprintf(cmd.ErrOrStderr(), "Stopping server %s...\n", serverID)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	actionStatus, err := provider.StopServer(ctx, serverID)
	if err != nil {
		return fmt.Errorf("failed to stop server: %w", err)
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
	record := svc.TrackAction(serverID, "", actionStatus, "stop_server", "off")

	if err := svc.WaitForAction(ctx, actionStatus, serverID, "off", cmd.ErrOrStderr()); err != nil {
		svc.FinalizeAction(record, domain.ActionStatusError, err.Error())
		return fmt.Errorf("failed waiting for server to stop: %w", err)
	}

	svc.FinalizeAction(record, domain.ActionStatusSuccess, "")

	cmd.SetContext(auditlog.WithMetadata(cmd.Context(), auditlog.Metadata{
		Provider:     providerName,
		ResourceType: "server",
		ResourceID:   serverID,
	}))

	fmt.Fprintf(cmd.OutOrStdout(), "Server %s stop initiated successfully.\n", serverID)
	return nil
}
