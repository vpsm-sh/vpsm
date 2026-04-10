package audit

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"nathanbeddoewebdev/vpsm/internal/auditlog"

	"github.com/spf13/cobra"
)

func ListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recent audit entries",
		Long: `List recent audit entries stored locally.

Examples:
  vpsm audit list
  vpsm audit list --limit 50
  vpsm audit list --command "vpsm server create"
  vpsm audit list -o json`,
		RunE:         runList,
		SilenceUsage: true,
	}

	cmd.Flags().Int("limit", 25, "Number of entries to display")
	cmd.Flags().String("command", "", "Filter by exact command path")
	cmd.Flags().StringP("output", "o", "table", "Output format: table or json")

	return cmd
}

func runList(cmd *cobra.Command, args []string) error {
	limit, _ := cmd.Flags().GetInt("limit")
	if limit <= 0 {
		return fmt.Errorf("limit must be greater than 0")
	}

	filter, _ := cmd.Flags().GetString("command")
	output, _ := cmd.Flags().GetString("output")
	if output == "" {
		output = "table"
	}

	repo, err := auditlog.Open()
	if err != nil {
		return err
	}
	defer repo.Close()

	var entries []auditlog.AuditEntry
	if filter != "" {
		entries, err = repo.ListByCommand(filter, limit)
	} else {
		entries, err = repo.List(limit)
	}
	if err != nil {
		return err
	}

	if output == "json" {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(entries)
	}
	if output != "table" {
		return fmt.Errorf("unsupported output format %q", output)
	}

	if len(entries) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No audit entries found.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TIME\tCOMMAND\tOUTCOME\tDURATION\tRESOURCE\tDETAIL")
	fmt.Fprintln(w, "----\t-------\t-------\t--------\t--------\t------")
	for _, entry := range entries {
		timeStr := entry.Timestamp.Local().Format("2006-01-02 15:04:05")
		resource := formatResource(entry)
		detail := entry.Detail
		if detail == "" {
			detail = "-"
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			timeStr,
			entry.Command,
			entry.Outcome,
			formatDuration(entry.DurationMs),
			resource,
			detail,
		)
	}
	w.Flush()
	return nil
}

func formatDuration(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	d := time.Duration(ms) * time.Millisecond
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

func formatResource(entry auditlog.AuditEntry) string {
	if entry.ResourceType == "" && entry.ResourceID == "" && entry.ResourceName == "" {
		return "-"
	}

	resource := entry.ResourceType
	if entry.ResourceID != "" {
		if resource != "" {
			resource += ":" + entry.ResourceID
		} else {
			resource = entry.ResourceID
		}
	}
	if entry.ResourceName != "" {
		if resource != "" {
			resource += " (" + entry.ResourceName + ")"
		} else {
			resource = entry.ResourceName
		}
	}
	return resource
}
