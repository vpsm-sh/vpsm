package dns

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	dnstui "nathanbeddoewebdev/vpsm/internal/dns/tui"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// ListCommand returns the "dns list" subcommand.
func ListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <domain>",
		Short: "List DNS records for a domain",
		Long: `List all DNS records for the given domain.

Examples:
  vpsm dns list example.com
  vpsm dns list example.com --type A`,
		Args: cobra.ExactArgs(1),
		Run:  runList,
	}

	cmd.Flags().String("type", "", "Filter records by type (A, AAAA, CNAME, MX, TXT, etc.)")

	return cmd
}

func runList(cmd *cobra.Command, args []string) {
	domainName := args[0]
	typeFilter, _ := cmd.Flags().GetString("type")

	svc, err := newDNSService(cmd)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
		return
	}

	providerName := cmd.Flag("provider").Value.String()

	if term.IsTerminal(int(os.Stdout.Fd())) {
		if _, err := dnstui.RunDNSApp(svc, providerName, domainName); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error running TUI: %v\n", err)
		}
		return
	}

	records, err := svc.ListRecords(context.Background(), domainName)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error listing records: %v\n", err)
		return
	}

	// Apply optional type filter.
	if typeFilter != "" {
		filtered := records[:0]
		for _, r := range records {
			if strings.EqualFold(string(r.Type), typeFilter) {
				filtered = append(filtered, r)
			}
		}
		records = filtered
	}

	if len(records) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No records found.")
		return
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tTYPE\tCONTENT\tTTL\tPRIORITY")
	fmt.Fprintln(w, "--\t----\t----\t-------\t---\t--------")

	for _, r := range records {
		prio := ""
		if r.Priority > 0 {
			prio = fmt.Sprintf("%d", r.Priority)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\n",
			r.ID,
			r.Name,
			string(r.Type),
			r.Content,
			r.TTL,
			prio,
		)
	}

	w.Flush()
}
