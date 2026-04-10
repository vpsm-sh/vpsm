package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"nathanbeddoewebdev/vpsm/internal/server/domain"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
)

// ErrDeleteAborted is returned when a user cancels the delete flow.
var ErrDeleteAborted = errors.New("server deletion aborted by user")

// DeleteServerForm runs an interactive wizard that lets the user select a server
// to delete. It fetches the current server list, presents a selection, shows a
// summary, and asks for confirmation before returning the chosen server.
func DeleteServerForm(provider domain.Provider) (*domain.Server, error) {
	accessible := os.Getenv("ACCESSIBLE") != ""

	// Fetch existing servers with a spinner.
	var servers []domain.Server
	fetchErr := spinner.New().
		Title("Fetching servers...").
		Accessible(accessible).
		Output(os.Stderr).
		ActionWithErr(func(ctx context.Context) error {
			var err error
			servers, err = provider.ListServers(ctx)
			return err
		}).
		Run()
	if fetchErr != nil {
		if errors.Is(fetchErr, huh.ErrUserAborted) || errors.Is(fetchErr, context.Canceled) {
			return nil, ErrDeleteAborted
		}
		return nil, fetchErr
	}

	if len(servers) == 0 {
		return nil, fmt.Errorf("no servers found")
	}

	// Build a lookup so we can map the selected ID back to a full Server.
	serverByID := make(map[string]domain.Server, len(servers))
	for _, s := range servers {
		serverByID[s.ID] = s
	}

	// --- Form: Select server + Summary + Confirm ---

	var selectedID string
	serverOpts := buildServerOptions(servers)

	height := max(len(serverOpts), 5)
	if height > 12 {
		height = 12
	}

	selectField := huh.NewSelect[string]().
		Title("Select server to delete").
		Options(serverOpts...).
		Value(&selectedID).
		Height(height)

	summaryNote := huh.NewNote().
		Title("Server details").
		DescriptionFunc(func() string {
			if s, ok := serverByID[selectedID]; ok {
				return buildDeleteSummary(s)
			}
			return ""
		}, &selectedID)

	confirm := false
	confirmField := huh.NewConfirm().
		Title("Delete this server? This action cannot be undone.").
		Affirmative("Yes, delete").
		Negative("Cancel").
		Value(&confirm)

	if err := runForm(accessible,
		huh.NewGroup(selectField),
		huh.NewGroup(summaryNote, confirmField),
	); err != nil {
		if errors.Is(err, ErrAborted) {
			return nil, ErrDeleteAborted
		}
		return nil, err
	}

	if !confirm {
		return nil, ErrDeleteAborted
	}

	server := serverByID[selectedID]
	return &server, nil
}

// buildServerOptions builds huh select options from a slice of servers.
func buildServerOptions(servers []domain.Server) []huh.Option[string] {
	options := make([]huh.Option[string], 0, len(servers))
	for _, s := range servers {
		label := serverOptionLabel(s)
		options = append(options, huh.NewOption(label, s.ID))
	}
	return options
}

// serverOptionLabel formats a server for display in the selection list.
func serverOptionLabel(s domain.Server) string {
	parts := []string{s.Name}

	if s.Status != "" {
		parts = append(parts, s.Status)
	}
	if s.ServerType != "" {
		parts = append(parts, s.ServerType)
	}
	if s.PublicIPv4 != "" {
		parts = append(parts, s.PublicIPv4)
	}
	if s.Region != "" {
		parts = append(parts, s.Region)
	}

	return strings.Join(parts, " - ")
}

// buildDeleteSummary formats a server's details for the confirmation summary.
func buildDeleteSummary(s domain.Server) string {
	var b strings.Builder

	fmt.Fprintf(&b, "ID: %s\n", s.ID)
	fmt.Fprintf(&b, "Name: %s\n", s.Name)
	fmt.Fprintf(&b, "Status: %s\n", s.Status)

	if s.ServerType != "" {
		fmt.Fprintf(&b, "Type: %s\n", s.ServerType)
	}
	if s.Image != "" {
		fmt.Fprintf(&b, "Image: %s\n", s.Image)
	}
	if s.Region != "" {
		fmt.Fprintf(&b, "Region: %s\n", s.Region)
	}
	if s.PublicIPv4 != "" {
		fmt.Fprintf(&b, "IPv4: %s\n", s.PublicIPv4)
	}
	if s.PublicIPv6 != "" {
		fmt.Fprintf(&b, "IPv6: %s\n", s.PublicIPv6)
	}

	return strings.TrimSpace(b.String())
}
