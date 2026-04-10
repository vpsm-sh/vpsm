package server

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"text/tabwriter"
	"time"

	"nathanbeddoewebdev/vpsm/internal/actionstore"
	"nathanbeddoewebdev/vpsm/internal/server/providers"
	"nathanbeddoewebdev/vpsm/internal/server/services/action"
	"nathanbeddoewebdev/vpsm/internal/services/auth"

	"github.com/spf13/cobra"
)

// ActionsCommand returns a cobra.Command that lists and resumes tracked actions.
func ActionsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "actions",
		Short: "List or resume tracked actions",
		Long: `Show actions that were started by previous CLI invocations.

By default, only pending (in-flight) actions are shown. Use --all to
include completed and failed actions as well.

If a previous start/stop command was interrupted (Ctrl+C), the action
remains tracked locally. Use --resume to resume polling all pending
actions until they complete.

Examples:
  vpsm server actions                     # Show pending actions
  vpsm server actions --all               # Show all recent actions
  vpsm server actions --resume            # Resume polling pending actions`,
		RunE:         runActions,
		SilenceUsage: true,
	}

	cmd.Flags().Bool("all", false, "Show all recent actions, not just pending")
	cmd.Flags().Bool("resume", false, "Resume polling all pending actions")

	return cmd
}

func runActions(cmd *cobra.Command, args []string) error {
	showAll, _ := cmd.Flags().GetBool("all")
	resume, _ := cmd.Flags().GetBool("resume")

	repo, err := actionstore.Open()
	if err != nil {
		return fmt.Errorf("failed to open action store: %w", err)
	}
	defer repo.Close()

	svc := action.NewService(nil, "", repo)

	if resume {
		return resumePendingActions(cmd, svc, repo)
	}

	var records []actionstore.ActionRecord
	if showAll {
		records, err = svc.ListRecent(20)
	} else {
		records, err = svc.ListPending()
	}
	if err != nil {
		return fmt.Errorf("failed to list actions: %w", err)
	}

	if len(records) == 0 {
		if showAll {
			fmt.Fprintln(cmd.OutOrStdout(), "No recent actions.")
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "No pending actions.")
		}
		return nil
	}

	printActions(cmd, records)

	// Hint about --resume when there are pending actions.
	if !showAll {
		fmt.Fprintf(cmd.ErrOrStderr(), "\nUse --resume to resume polling these actions.\n")
	}

	return nil
}

func printActions(cmd *cobra.Command, records []actionstore.ActionRecord) {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "ID\tPROVIDER\tSERVER\tCOMMAND\tSTATUS\tAGE\n")

	for _, record := range records {
		age := time.Since(record.CreatedAt).Truncate(time.Second)
		ageStr := formatDuration(age)

		status := record.Status
		if record.Status == "error" && record.ErrorMessage != "" {
			status = fmt.Sprintf("error: %s", truncate(record.ErrorMessage, 40))
		}

		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n",
			record.ID, record.Provider, record.ServerID, record.Command, status, ageStr)
	}

	w.Flush()
}

func resumePendingActions(cmd *cobra.Command, svc *action.Service, repo actionstore.ActionRepository) error {
	pending, err := svc.ListPending()
	if err != nil {
		return fmt.Errorf("failed to list pending actions: %w", err)
	}

	if len(pending) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No pending actions to resume.")
		return nil
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "Resuming %d pending action(s)...\n\n", len(pending))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	for i := range pending {
		record := &pending[i]
		resumeAction(ctx, cmd, repo, record)
	}

	return nil
}

func resumeAction(ctx context.Context, cmd *cobra.Command, repo actionstore.ActionRepository, record *actionstore.ActionRecord) {
	providerName := record.Provider
	provider, err := providers.Get(providerName, auth.DefaultStore())
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "[%s] Error resolving provider %q: %v\n", record.ServerID, providerName, err)
		return
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "[%s] Resuming %s (action %s)...\n", record.ServerID, record.Command, record.ActionID)

	svc := action.NewService(provider, providerName, repo)
	if err := svc.ResumeAction(ctx, record, cmd.ErrOrStderr()); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "[%s] Error: %v\n", record.ServerID, err)
		return
	}

	verb := "completed"
	if record.Command == "start_server" {
		verb = "started"
	} else if record.Command == "stop_server" {
		verb = "stopped"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Server %s %s successfully.\n", record.ServerID, verb)
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
