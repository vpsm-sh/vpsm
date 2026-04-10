package dns

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	dnstui "nathanbeddoewebdev/vpsm/internal/dns/tui"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// DomainsCommand returns the "dns domains" subcommand.
func DomainsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "domains",
		Short: "List domains in the provider account",
		Long: `List all domains registered in the DNS provider account.

Example:
  vpsm dns domains --provider porkbun`,
		Args: cobra.NoArgs,
		Run:  runDomains,
	}
}

func runDomains(cmd *cobra.Command, args []string) {
	svc, err := newDNSService(cmd)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
		return
	}

	providerName := cmd.Flag("provider").Value.String()

	if term.IsTerminal(int(os.Stdout.Fd())) {
		if _, err := dnstui.RunDNSApp(svc, providerName, ""); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error running TUI: %v\n", err)
		}
		return
	}

	domains, err := svc.ListDomains(context.Background())
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error listing domains: %v\n", err)
		return
	}

	if len(domains) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No domains found.")
		return
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "DOMAIN\tSTATUS\tTLD\tEXPIRES")
	fmt.Fprintln(w, "------\t------\t---\t-------")

	for _, d := range domains {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			d.Name,
			d.Status,
			d.TLD,
			d.ExpireDate,
		)
	}

	w.Flush()
}
