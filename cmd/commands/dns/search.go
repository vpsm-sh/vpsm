package dns

import (
	"context"
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// SearchCommand returns the "dns search" subcommand.
func SearchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <domain>",
		Short: "Check domain availability",
		Long: `Check if a domain name is available for registration.

Example:
  vpsm dns search example.com --provider porkbun`,
		Args: cobra.ExactArgs(1),
		Run:  runSearch,
	}

	cmd.Flags().StringP("output", "o", "", "Output format: table or json")

	return cmd
}

func runSearch(cmd *cobra.Command, args []string) {
	svc, err := newDNSService(cmd)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
		return
	}

	result, err := svc.SearchDomain(context.Background(), args[0])
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
		return
	}

	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		enc.Encode(result)
		return
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
}
