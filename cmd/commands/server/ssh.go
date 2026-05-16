package server

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"nathanbeddoewebdev/vpsm/internal/auditlog"
	"nathanbeddoewebdev/vpsm/internal/server/providers"
	"nathanbeddoewebdev/vpsm/internal/serverprefs"
	prefssvc "nathanbeddoewebdev/vpsm/internal/services/serverprefs"
	"nathanbeddoewebdev/vpsm/internal/services/auth"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// SSHCommand returns a cobra.Command that connects to a server via SSH.
func SSHCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ssh [flags] <server-id> [-- <ssh-args...>]",
		Short: "Connect to a server via SSH",
		Long: `Connect to a running server instance via SSH.

The server must be in the 'running' state and have a public IP address.
IPv4 is preferred; if unavailable, IPv6 will be used.

The target can be specified as a bare server ID or in user@id form.
Extra SSH arguments can be passed after a bare double-dash:

  vpsm server ssh 12345
  vpsm server ssh ubuntu@12345
  vpsm server ssh --provider hetzner -U ubuntu 12345 -- -L 8080:localhost:8080

If no username is provided via --user (or -U) or user@id, the last-used
username for this server is used. If none is saved, you will be prompted
interactively (or "root" is used in non-interactive contexts).`,
		RunE:         runSSH,
		SilenceUsage: true,
	}

	// Deprecated: positional arg is preferred.
	cmd.Flags().String("id", "", "Server ID to connect to (deprecated: use positional arg)")
	cmd.Flags().MarkHidden("id")

	cmd.Flags().StringP("user", "U", "", "SSH username (optional, defaults to saved preference or prompt)")

	return cmd
}

func runSSH(cmd *cobra.Command, args []string) error {
	providerName := cmd.Flag("provider").Value.String()

	provider, err := providers.Get(providerName, auth.DefaultStore())
	if err != nil {
		return err
	}

	// Resolve server ID from positional arg or deprecated --id flag.
	var serverID string
	var sshArgs []string

	idFlag, _ := cmd.Flags().GetString("id")
	if len(args) >= 1 && args[0] != "" {
		serverID = args[0]
		sshArgs = args[1:]
	} else if idFlag != "" {
		serverID = idFlag
	} else {
		return fmt.Errorf("server ID required: vpsm server ssh <id>")
	}

	// Parse user@id syntax.
	userAtID, parsedID := parseSSHTarget(serverID)
	if parsedID != "" {
		serverID = parsedID
	}

	// --user / -U flag overrides everything.
	userFlag, _ := cmd.Flags().GetString("user")
	if userFlag != "" {
		userAtID = userFlag
	}

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

	// Determine username: explicit flag > user@id > saved pref > prompt (or root).
	var username string
	repo, err := serverprefs.Open()
	if err == nil {
		svc := prefssvc.NewService(repo)
		defer svc.Close()

		switch {
		case userAtID != "":
			username = userAtID
		default:
			username = svc.GetSSHUser(providerName, serverID)
			if username == "" {
				username = promptSSHUser(cmd)
			}
		}

		// Persist the username for future use.
		svc.SetSSHUser(providerName, serverID, username)
	} else {
		switch {
		case userAtID != "":
			username = userAtID
		default:
			username = promptSSHUser(cmd)
		}
	}

	// Attempt SSH connection with retry on host key conflict.
	if err := connectSSH(cmd, providerName, serverID, username, ipAddress, sshArgs); err != nil {
		return err
	}
	return nil
}

// parseSSHTarget parses a target string like "user@id" into its components.
// If no "@" is present, user is empty and id is the full string.
func parseSSHTarget(target string) (user, id string) {
	user, id, ok := strings.Cut(target, "@")
	if ok && id != "" {
		return user, id
	}
	return "", target
}

// promptSSHUser asks the user for an SSH username interactively.
// Falls back to "root" when stdin is not a terminal.
func promptSSHUser(cmd *cobra.Command) string {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "root"
	}
	fmt.Fprint(cmd.ErrOrStderr(), "SSH username: ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "root"
	}
	if user := strings.TrimSpace(line); user != "" {
		return user
	}
	return "root"
}

// connectSSH attempts to SSH into the server, handling host key conflicts.
func connectSSH(cmd *cobra.Command, providerName, serverID, username, ipAddress string, sshArgs []string) error {
	// Build SSH command.
	sshCmdArgs := []string{
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=10",
		"-o", "ServerAliveInterval=60",
		"-o", "ServerAliveCountMax=3",
		fmt.Sprintf("%s@%s", username, ipAddress),
	}
	sshCmdArgs = append(sshCmdArgs, sshArgs...)
	sshCmd := exec.Command("ssh", sshCmdArgs...)

	sshCmd.Stdin = os.Stdin
	sshCmd.Stdout = cmd.OutOrStdout()

	// Capture stderr for error detection while streaming it to the user in real time.
	var stderrBuf bytes.Buffer
	sshCmd.Stderr = io.MultiWriter(cmd.ErrOrStderr(), &stderrBuf)

	// Run SSH and capture exit error.
	err := sshCmd.Run()
	if err == nil {
		// SSH succeeded — exit cleanly.
		return nil
	}

	// SSH failed — analyze stderr to provide better error messages.
	stderrOutput := stderrBuf.String()

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
			return connectSSH(cmd, providerName, serverID, username, ipAddress, sshArgs)
		}
		return fmt.Errorf("ssh connection failed due to host key conflict")
	}

	// Other SSH errors — just print a generic message.
	fmt.Fprintf(cmd.ErrOrStderr(), "\nSSH connection failed.\n")
	return fmt.Errorf("ssh connection failed")
}
