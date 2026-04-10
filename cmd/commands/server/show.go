package server

import (
	"context"
	"fmt"
	"os"

	"nathanbeddoewebdev/vpsm/internal/server/providers"
	"nathanbeddoewebdev/vpsm/internal/server/tui"
	"nathanbeddoewebdev/vpsm/internal/services/auth"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// ShowCommand returns a cobra.Command that displays details for a single server.
func ShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show details for a server",
		Long: `Display detailed information about a single server.

If --id is not provided, this behaves like vpsm server list: an interactive
TUI in a terminal, or table/json output in non-interactive mode.

Examples:
  # Interactive mode (TUI)
  vpsm server show --provider hetzner

  # Non-interactive with table output
  vpsm server show --provider hetzner --id 12345

  # JSON output for scripting
  vpsm server show --provider hetzner --id 12345 -o json`,
		RunE:         runShow,
		SilenceUsage: true,
	}

	cmd.Flags().String("id", "", "Server ID to show (skips interactive selection)")
	cmd.Flags().StringP("output", "o", "table", "Output format: table or json")

	return cmd
}

func runShow(cmd *cobra.Command, args []string) error {
	providerName := cmd.Flag("provider").Value.String()

	provider, err := providers.Get(providerName, auth.DefaultStore())
	if err != nil {
		return err
	}

	serverID, _ := cmd.Flags().GetString("id")

	if serverID == "" {
		output, _ := cmd.Flags().GetString("output")
		outputChanged := cmd.Flags().Changed("output")
		if outputChanged || !term.IsTerminal(int(os.Stdout.Fd())) {
			if output == "" {
				output = "table"
			}
			return runListNonInteractive(cmd, provider, output)
		}

		// Interactive full-window TUI with seamless view transitions.
		if _, err := tui.RunServerApp(provider, providerName); err != nil {
			return fmt.Errorf("server show failed: %w", err)
		}
		return nil
	}

	// Non-interactive mode: fetch and display directly.
	ctx := context.Background()
	server, err := provider.GetServer(ctx, serverID)
	if err != nil {
		return fmt.Errorf("failed to fetch server: %w", err)
	}

	output, _ := cmd.Flags().GetString("output")
	switch output {
	case "json":
		printServerJSON(cmd, server)
	default:
		printServerDetail(cmd, server)
	}

	return nil
}
