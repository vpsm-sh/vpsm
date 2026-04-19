package dns

import (
	"fmt"

	"nathanbeddoewebdev/vpsm/internal/config"
	dnsproviders "nathanbeddoewebdev/vpsm/internal/dns/providers"
	"nathanbeddoewebdev/vpsm/internal/dns/services"
	"nathanbeddoewebdev/vpsm/internal/services/auth"

	"github.com/spf13/cobra"
)

// NewCommand returns the top-level "dns" Cobra command with all subcommands attached.
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "dns",
		Short:             "Manage DNS records across providers",
		Long:              `Create, list, update, and delete DNS records. List domains in your account.`,
		PersistentPreRunE: resolveDNSProvider,
	}

	cmd.AddCommand(DomainsCommand())
	cmd.AddCommand(ListCommand())
	cmd.AddCommand(CreateCommand())
	cmd.AddCommand(UpdateCommand())
	cmd.AddCommand(DeleteCommand())
	cmd.AddCommand(SearchCommand())

	cmd.PersistentFlags().String("provider", "", "DNS provider to use (overrides default)")

	return cmd
}

// resolveDNSProvider ensures the --provider flag has a value, falling back to
// the dns-provider config key when the flag was not explicitly set.
func resolveDNSProvider(cmd *cobra.Command, args []string) error {
	if cmd.Flag("provider").Changed {
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.DNSProvider != "" {
		if err := cmd.Flag("provider").Value.Set(cfg.DNSProvider); err != nil {
			return fmt.Errorf("failed to set provider flag: %w", err)
		}
		return nil
	}

	return fmt.Errorf("no DNS provider specified: use --provider flag or set a default with 'vpsm config set dns-provider <name>'")
}

func newDNSService(cmd *cobra.Command) (*services.Service, error) {
	providerName := cmd.Flag("provider").Value.String()
	provider, err := dnsproviders.Get(providerName, auth.DefaultStore())
	if err != nil {
		return nil, err
	}

	return services.New(provider), nil
}
