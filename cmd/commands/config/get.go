package config

import (
	"fmt"
	"os"
	"strings"

	"nathanbeddoewebdev/vpsm/internal/config"
	"nathanbeddoewebdev/vpsm/internal/tui"
	"nathanbeddoewebdev/vpsm/internal/util"

	"golang.org/x/term"

	"github.com/spf13/cobra"
)

// GetCommand returns the "config get" command.
func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a configuration value",
		Long: "Get a persistent configuration value.\n\n" +
			"If no key is provided and running in a terminal, opens an interactive\n" +
			"config viewer where you can browse and edit all settings.\n\n" +
			config.KeysHelp() +
			"\nExamples:\n" +
			"  vpsm config get                   # interactive viewer\n" +
			"  vpsm config get --key default-provider   # print a single value",
		Args:         cobra.ExactArgs(0),
		RunE:         runGet,
		SilenceUsage: true,
	}

	cmd.Flags().String("key", "", "Configuration key to fetch (prints a single value)")

	return cmd
}

func runGet(cmd *cobra.Command, args []string) error {
	keyFlag, _ := cmd.Flags().GetString("key")
	keyFlag = strings.TrimSpace(keyFlag)

	// No key flag: open interactive config viewer.
	if keyFlag == "" {
		if term.IsTerminal(int(os.Stdout.Fd())) {
			if err := tui.RunConfigView(); err != nil {
				return fmt.Errorf("config view failed: %w", err)
			}
			return nil
		}

		// Non-interactive: list all values.
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		for _, spec := range config.Keys {
			value := spec.Get(cfg)
			if value == "" {
				value = "(not set)"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", spec.Name, value)
		}
		return nil
	}

	key := util.NormalizeKey(keyFlag)

	spec := config.Lookup(key)
	if spec == nil {
		return fmt.Errorf("unknown configuration key %q (valid: %s)", keyFlag, strings.Join(config.KeyNames(), ", "))
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	value := spec.Get(cfg)
	if value == "" {
		fmt.Fprintln(cmd.OutOrStdout(), "not set")
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), value)
	}
	return nil
}
