package server

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"nathanbeddoewebdev/vpsm/internal/server/domain"
	"nathanbeddoewebdev/vpsm/internal/server/providers"
	"nathanbeddoewebdev/vpsm/internal/server/tui"
	"nathanbeddoewebdev/vpsm/internal/services/auth"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func ListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all servers",
		Long: `List all servers from the specified provider.

In interactive mode (default), opens a full-window TUI with keyboard
navigation. Use --output json or --output table for non-interactive output.

Examples:
  # Interactive TUI
  vpsm server list

  # Non-interactive table
  vpsm server list -o table

  # JSON output for scripting
  vpsm server list -o json`,
		RunE:         runList,
		SilenceUsage: true,
	}

	cmd.Flags().StringP("output", "o", "", "Output format: table or json (omit for interactive TUI)")

	return cmd
}

func runList(cmd *cobra.Command, args []string) error {
	providerName := cmd.Flag("provider").Value.String()

	provider, err := providers.Get(providerName, auth.DefaultStore())
	if err != nil {
		return err
	}

	output, _ := cmd.Flags().GetString("output")

	// Non-interactive mode for scripting, or when no TTY is available.
	if output == "json" || output == "table" || !term.IsTerminal(int(os.Stdout.Fd())) {
		if output == "" {
			output = "table"
		}
		return runListNonInteractive(cmd, provider, output)
	}

	// Interactive full-window TUI. Runs a single Bubbletea program that
	// manages all view transitions (list, show, delete, create) internally,
	// eliminating screen flicker between views.
	if _, err := tui.RunServerApp(provider, providerName); err != nil {
		return fmt.Errorf("server list failed: %w", err)
	}
	return nil
}

func runListNonInteractive(cmd *cobra.Command, provider domain.Provider, output string) error {
	ctx := context.Background()
	servers, err := provider.ListServers(ctx)
	if err != nil {
		return fmt.Errorf("failed to list servers: %w", err)
	}

	if output == "json" {
		printServersJSON(cmd, servers)
		return nil
	}

	if len(servers) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No servers found.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tSTATUS\tREGION\tTYPE\tPUBLIC IPv4\tIMAGE")
	fmt.Fprintln(w, "--\t----\t------\t------\t----\t-----------\t-----")

	for _, server := range servers {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			server.ID,
			server.Name,
			server.Status,
			server.Region,
			server.ServerType,
			server.PublicIPv4,
			server.Image,
		)
	}

	w.Flush()
	return nil
}
