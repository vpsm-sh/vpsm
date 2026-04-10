package auth

import (
	"errors"
	"fmt"
	"os"

	credproviders "nathanbeddoewebdev/vpsm/internal/platform/providers"
	providernames "nathanbeddoewebdev/vpsm/internal/platform/providers/names"
	"nathanbeddoewebdev/vpsm/internal/services/auth"
	"nathanbeddoewebdev/vpsm/internal/tui"

	"golang.org/x/term"

	"github.com/spf13/cobra"
)

func StatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show authentication status for providers",
		Long: `Show which providers have stored credentials.

Example:
  vpsm auth status`,
		Run: func(cmd *cobra.Command, args []string) {
			store := auth.DefaultStore()

			// Use TUI in interactive terminal.
			if term.IsTerminal(int(os.Stdout.Fd())) {
				if err := tui.RunAuthStatus(store); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
				}
				return
			}

			// Non-interactive fallback: check all known credential specs first,
			// then fall back to the provider name registry for any not covered.
			shown := map[string]bool{}

			for _, spec := range credproviders.All() {
				shown[spec.Provider] = true
				loggedIn := true
				for _, key := range spec.Keys {
					keychainKey := spec.KeychainKey(key)
					_, err := store.GetToken(keychainKey)
					if err != nil {
						loggedIn = false
						break
					}
				}
				if loggedIn {
					fmt.Fprintf(cmd.OutOrStdout(), "%s: logged in\n", spec.Provider)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "%s: not logged in\n", spec.Provider)
				}
			}

			// Also show any providers not covered by the credential spec registry.
			for _, providerName := range providernames.List() {
				if shown[providerName] {
					continue
				}
				_, err := store.GetToken(providerName)
				switch {
				case err == nil:
					fmt.Fprintf(cmd.OutOrStdout(), "%s: logged in\n", providerName)
				case errors.Is(err, auth.ErrTokenNotFound):
					fmt.Fprintf(cmd.OutOrStdout(), "%s: not logged in\n", providerName)
				default:
					fmt.Fprintf(cmd.OutOrStdout(), "%s: error (%v)\n", providerName, err)
				}
			}
		},
	}

	return cmd
}
