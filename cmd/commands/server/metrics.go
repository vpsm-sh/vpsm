package server

import (
	"context"
	"fmt"
	"time"

	"nathanbeddoewebdev/vpsm/internal/server/domain"
	"nathanbeddoewebdev/vpsm/internal/server/providers"
	"nathanbeddoewebdev/vpsm/internal/services/auth"

	"github.com/spf13/cobra"
)

func MetricsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Show server metrics",
		Long: `Display CPU, disk IOPS, and network bandwidth metrics for a server.

Fetches metrics from the last hour and prints a summary with current,
minimum, maximum, and average values for each time series.

Examples:
  # Table output (default)
  vpsm server metrics --provider hetzner --id 12345

  # JSON output for scripting
  vpsm server metrics --provider hetzner --id 12345 -o json`,
		RunE:         runMetrics,
		SilenceUsage: true,
	}

	cmd.Flags().String("id", "", "Server ID (required)")
	cmd.MarkFlagRequired("id")
	cmd.Flags().StringP("output", "o", "table", "Output format: table or json")

	return cmd
}

func runMetrics(cmd *cobra.Command, args []string) error {
	providerName := cmd.Flag("provider").Value.String()

	provider, err := providers.Get(providerName, auth.DefaultStore())
	if err != nil {
		return err
	}

	mp, ok := provider.(domain.MetricsProvider)
	if !ok {
		return fmt.Errorf("provider %q does not support metrics", providerName)
	}

	serverID, _ := cmd.Flags().GetString("id")

	ctx := context.Background()
	end := time.Now()
	start := end.Add(-1 * time.Hour)

	metrics, err := mp.GetServerMetrics(ctx, serverID, []domain.MetricType{
		domain.MetricCPU,
		domain.MetricDisk,
		domain.MetricNetwork,
	}, start, end)
	if err != nil {
		return fmt.Errorf("failed to fetch metrics: %w", err)
	}

	output, _ := cmd.Flags().GetString("output")
	switch output {
	case "json":
		printMetricsJSON(cmd, metrics)
	default:
		printMetricsSummary(cmd, metrics)
	}

	return nil
}
