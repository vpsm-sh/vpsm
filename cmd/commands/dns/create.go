package dns

import (
	"context"
	"fmt"

	dnsdomain "nathanbeddoewebdev/vpsm/internal/dns/domain"

	"github.com/spf13/cobra"
)

// CreateCommand returns the "dns create" subcommand.
func CreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <domain>",
		Short: "Create a DNS record",
		Long: `Create a new DNS record for the given domain.

Examples:
  vpsm dns create example.com --type A --name www --content 1.2.3.4
  vpsm dns create example.com --type MX --content mail.example.com --priority 10
  vpsm dns create example.com --type TXT --name _dmarc --content "v=DMARC1; p=none"`,
		Args: cobra.ExactArgs(1),
		Run:  runCreate,
	}

	cmd.Flags().String("type", "", "Record type (A, AAAA, CNAME, MX, TXT, etc.) [required]")
	cmd.Flags().String("name", "", "Subdomain name (leave empty for root domain, use * for wildcard)")
	cmd.Flags().String("content", "", "Record content (IP address, hostname, text value, etc.) [required]")
	cmd.Flags().Int("ttl", 0, "Time-to-live in seconds (default: 600)")
	cmd.Flags().Int("priority", 0, "Record priority (for MX, SRV, etc.)")
	cmd.Flags().String("notes", "", "Optional notes for the record")

	cmd.MarkFlagRequired("type")
	cmd.MarkFlagRequired("content")

	return cmd
}

func runCreate(cmd *cobra.Command, args []string) {
	domainName := args[0]
	recordType, _ := cmd.Flags().GetString("type")
	name, _ := cmd.Flags().GetString("name")
	content, _ := cmd.Flags().GetString("content")
	ttl, _ := cmd.Flags().GetInt("ttl")
	priority, _ := cmd.Flags().GetInt("priority")
	notes, _ := cmd.Flags().GetString("notes")

	svc, err := newDNSService(cmd)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
		return
	}
	rec, err := svc.CreateRecord(context.Background(), domainName, dnsdomain.CreateRecordOpts{
		Name:     name,
		Type:     dnsdomain.RecordType(recordType),
		Content:  content,
		TTL:      ttl,
		Priority: priority,
		Notes:    notes,
	})
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error creating record: %v\n", err)
		return
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Created record %s (%s %s -> %s)\n",
		rec.ID, rec.Type, rec.Name, rec.Content)
}
