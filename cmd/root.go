package cmd

import (
	"os"

	"nathanbeddoewebdev/vpsm/cmd/commands/auth"
	cfgcmd "nathanbeddoewebdev/vpsm/cmd/commands/config"
	"nathanbeddoewebdev/vpsm/cmd/commands/dns"
	"nathanbeddoewebdev/vpsm/cmd/commands/server"
	"nathanbeddoewebdev/vpsm/cmd/commands/sshkey"
	dnsproviders "nathanbeddoewebdev/vpsm/internal/dns/providers"
	serverproviders "nathanbeddoewebdev/vpsm/internal/server/providers"
	sshkeyproviders "nathanbeddoewebdev/vpsm/internal/sshkey/providers"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands.
func rootCmd() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "vpsm",
		Short: "A CLI tool for managing VPS instances across cloud providers",
		Long: `vpsm is a command-line tool for managing virtual private servers across
multiple cloud providers. It supports creating, listing, and deleting
servers, with interactive TUI wizards for guided workflows.

Supported providers: Hetzner (more coming soon).

Quick start:
  vpsm auth login hetzner          # Store your API token
  vpsm server list                 # List all servers
  vpsm server create               # Interactive server creation
  vpsm server delete               # Interactive server deletion`,
	}

	cmd.AddCommand(auth.NewCommand())
	cmd.AddCommand(cfgcmd.NewCommand())
	cmd.AddCommand(dns.NewCommand())
	cmd.AddCommand(server.NewCommand())
	cmd.AddCommand(sshkey.NewCommand())

	return cmd
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	serverproviders.RegisterHetzner()
	sshkeyproviders.RegisterHetzner()
	dnsproviders.RegisterPorkbun()
	dnsproviders.RegisterCloudflare()

	var root = rootCmd()
	err := root.Execute()
	if err != nil {
		os.Exit(1)
	}
}
