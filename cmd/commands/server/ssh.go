package server

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"nathanbeddoewebdev/vpsm/internal/auditlog"
	"nathanbeddoewebdev/vpsm/internal/server/providers"
	"nathanbeddoewebdev/vpsm/internal/serverprefs"
	"nathanbeddoewebdev/vpsm/internal/services/auth"
	prefssvc "nathanbeddoewebdev/vpsm/internal/services/serverprefs"

	"github.com/spf13/cobra"
)

// SSHCommand returns a cobra.Command that connects to a server via SSH.
func SSHCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ssh",
		Short: "Connect to a server via SSH",
		Long: `Connect to a running server instance via SSH.

The server must be in the 'running' state and have a public IP address.
IPv4 is preferred; if unavailable, IPv6 will be used.

The username can be specified via --user, or will default to the last-used
username for this server (stored locally), or "root" if never set.

Examples:
  vpsm server ssh --provider hetzner --id 12345
  vpsm server ssh --provider hetzner --id 12345 --user ubuntu`,
		RunE:         runSSH,
		SilenceUsage: true,
	}

	cmd.Flags().String("id", "", "Server ID to connect to (required)")
	cmd.MarkFlagRequired("id")
	cmd.Flags().String("user", "", "SSH username (optional, defaults to saved preference or 'root')")

	return cmd
}

func runSSH(cmd *cobra.Command, args []string) error {
	providerName := cmd.Flag("provider").Value.String()

	provider, err := providers.Get(providerName, auth.DefaultStore())
	if err != nil {
		return err
	}

	serverID, _ := cmd.Flags().GetString("id")
	userFlag, _ := cmd.Flags().GetString("user")

	cmd.SetContext(auditlog.WithMetadata(cmd.Context(), auditlog.Metadata{
		Provider:     providerName,
		ResourceType: "server",
		ResourceID:   serverID,
	}))

	ctx := context.Background()

	// Fetch the server.
	server, err := provider.GetServer(ctx, serverID)
	if err != nil {
		return fmt.Errorf("failed to fetch server: %w", err)
	}

	// Check that the server is running.
	if server.Status != "running" {
		return fmt.Errorf("server %s is not running (status: %s); start with: vpsm server start --provider %s --id %s", serverID, server.Status, providerName, serverID)
	}

	// Resolve IP address (IPv4 preferred, IPv6 fallback).
	ipAddress := server.PublicIPv4
	if ipAddress == "" {
		ipAddress = server.PublicIPv6
	}
	if ipAddress == "" {
		return fmt.Errorf("server %s has no public IP address", serverID)
	}

	// Open serverprefs repository (best-effort, like actionstore pattern).
	var username string
	repo, err := serverprefs.Open()
	if err == nil {
		svc := prefssvc.NewService(repo)
		defer svc.Close()

		// Determine username: --user flag > saved pref > "root".
		if userFlag != "" {
			username = userFlag
		} else {
			username = svc.GetSSHUser(providerName, serverID)
			if username == "" {
				username = "root"
			}
		}

		// Persist the username for future use.
		svc.SetSSHUser(providerName, serverID, username)
	} else {
		// If prefs unavailable, use flag or default.
		if userFlag != "" {
			username = userFlag
		} else {
			username = "root"
		}
	}

	// Attempt SSH connection with retry on host key conflict.
	if err := connectSSH(cmd, providerName, serverID, username, ipAddress); err != nil {
		return err
	}
	return nil
}

// connectSSH attempts to SSH into the server, handling host key conflicts.
func connectSSH(cmd *cobra.Command, providerName, serverID, username, ipAddress string) error {
	// Build SSH command.
	sshCmd := exec.Command("ssh",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=10",
		"-o", "ServerAliveInterval=60",
		"-o", "ServerAliveCountMax=3",
		fmt.Sprintf("%s@%s", username, ipAddress),
	)

	sshCmd.Stdin = os.Stdin
	sshCmd.Stdout = os.Stdout

	// Capture stderr for error detection.
	var stderrBuf bytes.Buffer
	sshCmd.Stderr = &stderrBuf

	// Run SSH and capture exit error.
	err := sshCmd.Run()
	if err == nil {
		// SSH succeeded — exit cleanly.
		return nil
	}

	// SSH failed — analyze stderr to provide better error messages.
	stderrOutput := stderrBuf.String()

	// Always print the captured stderr so the user sees SSH's full output.
	fmt.Fprint(cmd.ErrOrStderr(), stderrOutput)

	// Detect host key conflict.
	if strings.Contains(stderrOutput, "REMOTE HOST IDENTIFICATION HAS CHANGED") {
		fmt.Fprintf(cmd.ErrOrStderr(), "\nHost key has changed (IP may have been reused by a new server).\n")
		fmt.Fprintf(cmd.ErrOrStderr(), "Clear the old key and retry? [Y/n]: ")

		// Read user response.
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response == "" || response == "y" || response == "yes" {
			// Clear the old host key.
			fmt.Fprintf(cmd.ErrOrStderr(), "Clearing old host key for %s...\n", ipAddress)
			clearCmd := exec.Command("ssh-keygen", "-R", ipAddress)
			if clearErr := clearCmd.Run(); clearErr != nil {
				return fmt.Errorf("failed to clear host key: %w", clearErr)
			}

			// Retry SSH connection.
			fmt.Fprintf(cmd.ErrOrStderr(), "Retrying SSH connection...\n")
			return connectSSH(cmd, providerName, serverID, username, ipAddress)
		}
		return fmt.Errorf("ssh connection failed due to host key conflict")
	}

	// Other SSH errors — just print a generic message.
	fmt.Fprintf(cmd.ErrOrStderr(), "\nSSH connection failed.\n")
	return fmt.Errorf("ssh connection failed")
}
