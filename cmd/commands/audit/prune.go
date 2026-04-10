package audit

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"nathanbeddoewebdev/vpsm/internal/auditlog"

	"github.com/spf13/cobra"
)

func PruneCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Delete audit entries older than a duration",
		Long: `Delete audit entries older than a duration.

Examples:
  vpsm audit prune --older-than 30d
  vpsm audit prune --older-than 72h`,
		RunE:         runPrune,
		SilenceUsage: true,
	}

	cmd.Flags().String("older-than", "", "Remove entries older than this duration (e.g. 30d, 72h)")

	return cmd
}

func runPrune(cmd *cobra.Command, args []string) error {
	olderThanRaw, _ := cmd.Flags().GetString("older-than")
	olderThanRaw = strings.TrimSpace(olderThanRaw)
	if olderThanRaw == "" {
		return fmt.Errorf("--older-than is required")
	}

	olderThan, err := parseDuration(olderThanRaw)
	if err != nil {
		return err
	}

	repo, err := auditlog.Open()
	if err != nil {
		return err
	}
	defer repo.Close()

	removed, err := repo.Prune(olderThan)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Removed %d audit entr(y/ies).\n", removed)
	return nil
}

func parseDuration(input string) (time.Duration, error) {
	if before, ok := strings.CutSuffix(input, "d"); ok {
		num := before
		days, err := strconv.Atoi(num)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q", input)
		}
		if days < 0 {
			return 0, fmt.Errorf("duration must be positive")
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	d, err := time.ParseDuration(input)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q", input)
	}
	if d < 0 {
		return 0, fmt.Errorf("duration must be positive")
	}
	return d, nil
}
