package dns

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"text/tabwriter"

	dnstui "nathanbeddoewebdev/vpsm/internal/dns/tui"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// DomainsCommand returns the "dns domains" subcommand.
func DomainsCommand() *cobra.Command {
	return &cobra.Command{
		Use:          "domains",
		Short:        "List domains in the provider account",
		Long: `List all domains registered in the DNS provider account.

Example:
  vpsm dns domains --provider porkbun`,
		Args:         cobra.NoArgs,
		RunE:         runDomains,
		SilenceUsage: true,
	}
}

func runDomains(cmd *cobra.Command, args []string) error {
	svc, err := newDNSService(cmd)
	if err != nil {
		return err
	}

	providerName := cmd.Flag("provider").Value.String()

	if term.IsTerminal(int(os.Stdout.Fd())) {
		_, err = dnstui.RunDNSApp(svc, providerName, "")
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	domains, err := svc.ListDomains(ctx)
	if err != nil {
		return fmt.Errorf("failed to list domains: %w", err)
	}

	if len(domains) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No domains found.")
		return nil
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
	return nil
}
