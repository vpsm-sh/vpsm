package dns

import (
	"context"
	"fmt"

	dnsdomain "nathanbeddoewebdev/vpsm/internal/dns/domain"

	"github.com/spf13/cobra"
)

// UpdateCommand returns the "dns update" subcommand.
func UpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <domain> <id>",
		Short: "Update a DNS record",
		Long: `Update an existing DNS record by its ID.

Examples:
  vpsm dns update example.com 106926659 --content 5.6.7.8
  vpsm dns update example.com 106926659 --content 5.6.7.8 --ttl 3600`,
		Args: cobra.ExactArgs(2),
		Run:  runUpdate,
	}

	cmd.Flags().String("type", "", "New record type")
	cmd.Flags().String("name", "", "New subdomain name")
	cmd.Flags().String("content", "", "New record content [required]")
	cmd.Flags().Int("ttl", 0, "New time-to-live in seconds (default: 600)")
	cmd.Flags().Int("priority", 0, "New record priority")
	cmd.Flags().String("notes", "", "New notes (use empty string to clear)")

	cmd.MarkFlagRequired("content")

	return cmd
}

func runUpdate(cmd *cobra.Command, args []string) {
	domainName := args[0]
	recordID := args[1]
	recordType, _ := cmd.Flags().GetString("type")
	name, _ := cmd.Flags().GetString("name")
	content, _ := cmd.Flags().GetString("content")
	ttl, _ := cmd.Flags().GetInt("ttl")
	priority, _ := cmd.Flags().GetInt("priority")

	// Notes: nil means no change, pointer to empty string clears it.
	var notesPtr *string
	if cmd.Flags().Changed("notes") {
		v, _ := cmd.Flags().GetString("notes")
		notesPtr = &v
	}

	svc, err := newDNSService(cmd)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
		return
	}
	err = svc.UpdateRecord(context.Background(), domainName, recordID, dnsdomain.UpdateRecordOpts{
		Name:     name,
		Type:     dnsdomain.RecordType(recordType),
		Content:  content,
		TTL:      ttl,
		Priority: priority,
		Notes:    notesPtr,
	})
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error updating record: %v\n", err)
		return
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Updated record %s\n", recordID)
}
