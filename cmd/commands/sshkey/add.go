package sshkey

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"nathanbeddoewebdev/vpsm/internal/auditlog"
	platformsshkey "nathanbeddoewebdev/vpsm/internal/platform/sshkey"
	"nathanbeddoewebdev/vpsm/internal/services/auth"
	"nathanbeddoewebdev/vpsm/internal/sshkey/providers"
	"nathanbeddoewebdev/vpsm/internal/sshkey/tui"
	"nathanbeddoewebdev/vpsm/internal/sshkeys"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func AddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [path]",
		Short: "Upload an SSH key to the cloud provider",
		Long: `Upload a local SSH public key to the cloud provider's account.

Provide a path argument or use --public-key to paste the key directly.
If no path argument is provided, you will be prompted with the default path (~/.ssh/id_ed25519.pub) prefilled.
If the selected file does not exist, you will be asked to provide another path.

The key name will be prompted interactively unless --name is specified.

Examples:
  # Upload default key (interactive name prompt)
  vpsm ssh-key add

  # Upload specific key with explicit name
  vpsm ssh-key add ~/.ssh/work_laptop.pub --name work-laptop

  # Paste public key directly
  vpsm ssh-key add --public-key "ssh-ed25519 AAAA..." --name laptop

  # Upload with provider override
  vpsm ssh-key add --provider hetzner --name my-key`,
		RunE:         runAdd,
		SilenceUsage: true,
	}

	cmd.Flags().String("name", "", "Name for the SSH key (interactive prompt if not provided)")
	cmd.Flags().String("public-key", "", "Public SSH key content (paste instead of providing a path)")

	return cmd
}

func runAdd(cmd *cobra.Command, args []string) error {
	providerName := cmd.Flag("provider").Value.String()

	provider, err := providers.Get(providerName, auth.DefaultStore())
	if err != nil {
		return err
	}

	publicKeyInput, _ := cmd.Flags().GetString("public-key")
	publicKeyProvided := cmd.Flags().Changed("public-key")
	if publicKeyProvided && len(args) > 0 {
		return fmt.Errorf("provide a path or --public-key, not both")
	}

	keyName, _ := cmd.Flags().GetString("name")
	keyName = strings.TrimSpace(keyName)

	needsInteractive := keyName == "" || (!publicKeyProvided && len(args) == 0)
	var publicKey string
	var keyPath string

	if needsInteractive {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("interactive mode requires a terminal; provide --name and a key input to run non-interactively")
		}

		prefill := tui.SSHKeyAddPrefill{Name: keyName}
		if publicKeyProvided {
			prefill.Source = tui.SSHKeySourcePaste
			prefill.PublicKey = publicKeyInput
		} else if len(args) > 0 {
			prefill.Source = tui.SSHKeySourceFile
			prefill.Path = args[0]
		}

		var result *tui.SSHKeyAddResult
		var err error
		if os.Getenv("ACCESSIBLE") != "" {
			result, err = tui.RunSSHKeyAddAccessible(providerName, prefill)
		} else {
			result, err = tui.RunSSHKeyAdd(providerName, prefill)
		}
		if err != nil {
			if errors.Is(err, tui.ErrAborted) {
				fmt.Fprintln(cmd.ErrOrStderr(), "SSH key add cancelled.")
				return nil
			}
			return err
		}
		if result == nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "SSH key add cancelled.")
			return nil
		}

		publicKey = result.PublicKey
		keyName = result.Name
	} else {
		if publicKeyProvided {
			publicKey, err = sshkeys.ValidatePublicKey(publicKeyInput)
			if err != nil {
				return err
			}
		} else {
			keyPath = args[0]
			keyPath, err = sshkeys.ExpandHomePath(keyPath)
			if err != nil {
				return err
			}
			if _, err := os.Stat(keyPath); os.IsNotExist(err) {
				printCommonSSHKeyPaths(cmd)
				return fmt.Errorf("SSH key file not found: %s", keyPath)
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "Reading key from %s\n", keyPath)

			publicKey, err = sshkeys.ReadAndValidatePublicKey(keyPath)
			if err != nil {
				return err
			}
		}
	}

	// Upload the key
	fmt.Fprintf(cmd.ErrOrStderr(), "Uploading SSH key %q to %s...", keyName, provider.GetDisplayName())

	ctx := context.Background()
	keySpec, err := provider.CreateSSHKey(ctx, keyName, publicKey)
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr())
		return fmt.Errorf("failed to upload ssh key: %w", err)
	}

	fmt.Fprintln(cmd.ErrOrStderr(), " done")
	fmt.Fprintln(cmd.ErrOrStderr())

	cmd.SetContext(auditlog.WithMetadata(cmd.Context(), auditlog.Metadata{
		Provider:     providerName,
		ResourceType: "ssh-key",
		ResourceID:   keySpec.ID,
		ResourceName: keySpec.Name,
	}))

	// Print the result
	printKeyDetails(cmd, keySpec)
	return nil
}

func printKeyDetails(cmd *cobra.Command, key *platformsshkey.Spec) {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	defer w.Flush()

	fmt.Fprintln(w, "SSH key added:")
	fmt.Fprintf(w, "  Name:\t%s\n", key.Name)
	fmt.Fprintf(w, "  Fingerprint:\t%s\n", key.Fingerprint)
	fmt.Fprintf(w, "  ID:\t%s\n", key.ID)
}

func printCommonSSHKeyPaths(cmd *cobra.Command) {
	fmt.Fprintln(cmd.ErrOrStderr(), "\nCommon SSH key paths:")
	for _, path := range sshkeys.CommonPaths() {
		fmt.Fprintf(cmd.ErrOrStderr(), "  %s\n", path)
	}
}
