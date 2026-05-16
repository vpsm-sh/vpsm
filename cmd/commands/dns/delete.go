package dns

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
)

// DeleteCommand returns the "dns delete" subcommand.
func DeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:          "delete <domain> <id>",
		Short:        "Delete a DNS record",
		Long: `Delete a DNS record by its ID.

Example:
  vpsm dns delete example.com 106926659`,
		Args:         cobra.ExactArgs(2),
		RunE:         runDelete,
		SilenceUsage: true,
	}
}

func runDelete(cmd *cobra.Command, args []string) error {
	domainName := args[0]
	recordID := args[1]
	svc, err := newDNSService(cmd)
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := svc.DeleteRecord(ctx, domainName, recordID); err != nil {
		return fmt.Errorf("failed to delete record: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Deleted record %s\n", recordID)
	return nil
}
