package auth

import (
	"bufio"
	"fmt"
	"os"
	"slices"
	"strings"

	dnsproviders "nathanbeddoewebdev/vpsm/internal/dns/providers"
	credproviders "nathanbeddoewebdev/vpsm/internal/platform/providers"
	providernames "nathanbeddoewebdev/vpsm/internal/platform/providers/names"
	"nathanbeddoewebdev/vpsm/internal/services/auth"
	"nathanbeddoewebdev/vpsm/internal/tui"
	"nathanbeddoewebdev/vpsm/internal/util"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func LoginCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login <provider>",
		Short: "Store credentials for a provider",
		Long: `Store credentials for a provider using the local keychain.

For single-token providers (e.g. Hetzner, Cloudflare), you will be prompted for an API token.
For multi-credential providers (e.g. Porkbun), you will be prompted for each key.

Cloudflare requires a scoped Account API Token (not a Global API Key)
with Zone:Read and DNS:Edit permissions.

Examples:
  vpsm auth login hetzner
  vpsm auth login porkbun
  vpsm auth login cloudflare`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			provider := strings.TrimSpace(args[0])
			if provider == "" {
				fmt.Fprintln(cmd.ErrOrStderr(), "provider is required")
				return
			}

			if !isKnownProvider(provider) {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error: unknown provider %q\n", provider)
				fmt.Fprintln(cmd.ErrOrStderr(), "Known providers: hetzner, porkbun, cloudflare")
				return
			}

			store := auth.DefaultStore()
			spec := credproviders.Lookup(provider)

			// Multi-credential providers (e.g. Porkbun with apikey + secretapikey).
			// These are always handled via stdin prompts regardless of TTY state
			// because the TUI auth login only handles single-token providers.
			if spec != nil && len(spec.Keys) > 1 {
				runMultiKeyLogin(cmd, provider, spec, store)
				return
			}

			token, err := cmd.Flags().GetString("token")
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), err)
				return
			}
			token = strings.TrimSpace(token)

			if token == "" {
				// Interactive mode: use TUI if running in a terminal.
				if term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())) {
					result, err := tui.RunAuthLogin(provider, store)
					if err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
						return
					}
					if result != nil && result.Saved {
						fmt.Fprintf(cmd.OutOrStdout(), "Saved credentials for %s\n", provider)
					} else {
						fmt.Fprintln(cmd.ErrOrStderr(), "Login cancelled.")
					}
					return
				}

				fmt.Fprintln(cmd.ErrOrStderr(), "Error: non-interactive login requires --token")
				return
			}

			if err := store.SetToken(provider, token); err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), err)
				return
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Saved credentials for %s\n", provider)
		},
	}

	cmd.Flags().String("token", "", "API token (optional, overrides prompt; not used for multi-credential providers)")

	return cmd
}

// runMultiKeyLogin handles the login flow for providers that require multiple
// credentials (e.g. Porkbun API key + secret key). Each credential is prompted
// for in sequence, reading from stdin with terminal echo suppressed for secrets.
// isKnownProvider checks if a provider name is registered in any of the known
// provider registries (credential specs, server providers, or DNS providers).
func isKnownProvider(name string) bool {
	normalized := util.NormalizeKey(name)

	// Check credential spec registry (covers Hetzner, Porkbun, Cloudflare).
	if credproviders.Lookup(normalized) != nil {
		return true
	}

	// Check server provider registry.
	if slices.Contains(providernames.List(), normalized) {
		return true
	}

	// Check DNS provider registry.
	return slices.Contains(dnsproviders.List(), normalized)
}

func runMultiKeyLogin(cmd *cobra.Command, provider string, spec *credproviders.CredentialSpec, store auth.Store) {
	fmt.Fprintf(cmd.OutOrStdout(), "Logging in to %s\n", spec.DisplayName)

	reader := bufio.NewReader(os.Stdin)
	isTTY := term.IsTerminal(int(os.Stdin.Fd()))

	for _, key := range spec.Keys {
		keychainKey := spec.KeychainKey(key)
		fmt.Fprintf(cmd.OutOrStdout(), "%s: ", key.Prompt)

		var value string
		var err error

		if key.Secret && isTTY {
			// Suppress echo for secret fields in a real terminal.
			raw, readErr := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(cmd.OutOrStdout()) // newline after the hidden input
			if readErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error reading %s: %v\n", key.Prompt, readErr)
				return
			}
			value = strings.TrimSpace(string(raw))
		} else {
			value, err = reader.ReadString('\n')
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error reading %s: %v\n", key.Prompt, err)
				return
			}
			value = strings.TrimSpace(value)
		}

		if value == "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "%s cannot be empty\n", key.Prompt)
			return
		}

		if err := store.SetToken(keychainKey, value); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error saving %s: %v\n", key.Prompt, err)
			return
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Saved credentials for %s\n", provider)
}
