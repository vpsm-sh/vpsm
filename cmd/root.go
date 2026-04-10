package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	auditcmd "nathanbeddoewebdev/vpsm/cmd/commands/audit"
	"nathanbeddoewebdev/vpsm/cmd/commands/auth"
	cfgcmd "nathanbeddoewebdev/vpsm/cmd/commands/config"
	"nathanbeddoewebdev/vpsm/cmd/commands/dns"
	"nathanbeddoewebdev/vpsm/cmd/commands/server"
	"nathanbeddoewebdev/vpsm/cmd/commands/sshkey"
	"nathanbeddoewebdev/vpsm/internal/auditlog"
	dnsproviders "nathanbeddoewebdev/vpsm/internal/dns/providers"
	serverproviders "nathanbeddoewebdev/vpsm/internal/server/providers"
	sshkeyproviders "nathanbeddoewebdev/vpsm/internal/sshkey/providers"

	"github.com/spf13/cobra"
)

// Version information, set at build time via ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
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

	cmd.Version = fmt.Sprintf("%s (commit %s, built %s)", Version, Commit, BuildTime)

	cmd.AddCommand(auth.NewCommand())
	cmd.AddCommand(auditcmd.NewCommand())
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
	cobra.EnableTraverseRunHooks = true
	dnsproviders.RegisterPorkbun()
	dnsproviders.RegisterCloudflare()

	var root = rootCmd()
	root.SilenceUsage = true
	root.SilenceErrors = true

	start := time.Now().UTC()
	cmd, err := root.ExecuteC()
	recordAuditEntry(cmd, err, start)
	if err != nil {
		fmt.Fprintf(root.ErrOrStderr(), "Error: %v\n", err)
		os.Exit(1)
	}
}

func recordAuditEntry(cmd *cobra.Command, execErr error, start time.Time) {
	if cmd == nil {
		return
	}

	repo, err := auditlog.Open()
	if err != nil {
		return
	}
	defer repo.Close()

	meta := auditlog.MetadataFromContext(cmd.Context())

	provider := meta.Provider
	if provider == "" {
		provider = flagValue(cmd, "provider")
	}

	resourceType := meta.ResourceType
	resourceID := meta.ResourceID
	resourceName := meta.ResourceName
	if resourceID == "" {
		id := flagValue(cmd, "id")
		if id != "" && strings.HasPrefix(cmd.CommandPath(), "vpsm server") {
			resourceType = "server"
			resourceID = id
		}
	}

	args := strings.Join(auditlog.SanitizeArgs(os.Args[1:]), " ")
	entry := &auditlog.AuditEntry{
		Timestamp:    start,
		Command:      cmd.CommandPath(),
		Args:         args,
		Provider:     provider,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		ResourceName: resourceName,
		DurationMs:   time.Since(start).Milliseconds(),
	}
	if execErr != nil {
		entry.Outcome = auditlog.OutcomeError
		entry.Detail = execErr.Error()
	} else {
		entry.Outcome = auditlog.OutcomeSuccess
	}

	_ = repo.Save(entry)
}

func flagValue(cmd *cobra.Command, name string) string {
	if cmd == nil {
		return ""
	}
	flag := cmd.Flag(name)
	if flag == nil {
		return ""
	}
	return flag.Value.String()
}
