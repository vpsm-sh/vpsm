package config

import (
	"fmt"
	"slices"
	"strings"

	"nathanbeddoewebdev/vpsm/internal/config"
	dnsproviders "nathanbeddoewebdev/vpsm/internal/dns/providers"
	providernames "nathanbeddoewebdev/vpsm/internal/platform/providers/names"
	"nathanbeddoewebdev/vpsm/internal/util"

	"github.com/spf13/cobra"
)

// SetCommand returns the "config set" command.
func SetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Long: "Set a persistent configuration value.\n\n" +
			config.KeysHelp() +
			"\nExamples:\n" +
			"  vpsm config set default-provider hetzner",
		Args:         cobra.ExactArgs(2),
		RunE:         runSet,
		SilenceUsage: true,
	}

	return cmd
}

// validators maps key names to optional pre-save validation functions.
// Keys not present in this map have no extra validation.
var validators = map[string]func(cmd *cobra.Command, value string) error{
	"default-provider": validateProvider,
	"dns-provider":     validateDNSProvider,
}

func runSet(cmd *cobra.Command, args []string) error {
	key := util.NormalizeKey(args[0])
	value := args[1]

	spec := config.Lookup(key)
	if spec == nil {
		return fmt.Errorf("unknown configuration key %q (valid: %s)", args[0], strings.Join(config.KeyNames(), ", "))
	}

	if validate, ok := validators[spec.Name]; ok {
		if err := validate(cmd, value); err != nil {
			return err
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	normalized := util.NormalizeKey(value)
	spec.Set(cfg, normalized)
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "%s set to %q\n", spec.Name, normalized)
	return nil
}

// validateProvider checks that the given name is a registered server provider.
func validateProvider(cmd *cobra.Command, name string) error {
	normalized := util.NormalizeKey(name)
	known := providernames.List()
	if slices.Contains(known, normalized) {
		return nil
	}
	return fmt.Errorf("unknown provider %q (registered: %s)", name, strings.Join(known, ", "))
}

// validateDNSProvider checks that the given name is a registered DNS provider.
func validateDNSProvider(cmd *cobra.Command, name string) error {
	normalized := util.NormalizeKey(name)
	known := dnsproviders.List()
	if slices.Contains(known, normalized) {
		return nil
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Error: unknown DNS provider %q\n", name)
	fmt.Fprintf(cmd.ErrOrStderr(), "Registered DNS providers: %v\n", known)
	return fmt.Errorf("unknown DNS provider %q", name)
}
