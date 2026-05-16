package dns

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// SearchCommand returns the "dns search" subcommand.
func SearchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "search <domain>",
		Short:        "Check domain availability",
		Long: `Check if a domain name is available for registration.

Example:
  vpsm dns search example.com --provider porkbun`,
		Args:         cobra.ExactArgs(1),
		RunE:         runSearch,
		SilenceUsage: true,
	}

	cmd.Flags().StringP("output", "o", "", "Output format: table or json")

	return cmd
}

func runSearch(cmd *cobra.Command, args []string) error {
	svc, err := newDNSService(cmd)
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	result, err := svc.SearchDomain(ctx, args[0])
	if err != nil {
		return fmt.Errorf("failed to search domain: %w", err)
	}

	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			return fmt.Errorf("failed to encode JSON: %w", err)
		}
		return nil
	}

	avail := "No"
	if result.Available {
		avail = "Yes"
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "DOMAIN\tAVAILABLE\tPRICE\tRENEWAL\tCURRENCY")
	fmt.Fprintln(w, "------\t---------\t-----\t-------\t--------")

	price := result.Price
	if price == "" {
		price = "-"
	}
	renewal := result.Renewal
	if renewal == "" {
		renewal = "-"
	}
	currency := result.Currency
	if currency == "" {
		currency = "-"
	}

	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
		result.Domain,
		avail,
		price,
		renewal,
		currency,
	)

	w.Flush()
	return nil
}
