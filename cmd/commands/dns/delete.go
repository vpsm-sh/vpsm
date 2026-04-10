package dns

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

// DeleteCommand returns the "dns delete" subcommand.
func DeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <domain> <id>",
		Short: "Delete a DNS record",
		Long: `Delete a DNS record by its ID.

Example:
  vpsm dns delete example.com 106926659`,
		Args: cobra.ExactArgs(2),
		Run:  runDelete,
	}
}

func runDelete(cmd *cobra.Command, args []string) {
	domainName := args[0]
	recordID := args[1]
	svc, err := newDNSService(cmd)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
		return
	}
	if err := svc.DeleteRecord(context.Background(), domainName, recordID); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error deleting record: %v\n", err)
		return
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Deleted record %s\n", recordID)
}
