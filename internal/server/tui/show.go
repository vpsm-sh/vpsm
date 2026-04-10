package tui

import (
	"context"
	"errors"
	"fmt"
	"os"

	"nathanbeddoewebdev/vpsm/internal/server/domain"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
)

// ErrShowAborted is returned when a user cancels the show flow.
var ErrShowAborted = errors.New("server selection aborted by user")

// ShowServerForm runs an interactive wizard that lets the user select a server
// to inspect. It fetches the current server list, presents a selection, and
// returns the chosen server.
func ShowServerForm(provider domain.Provider) (*domain.Server, error) {
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
			return nil, ErrShowAborted
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

	// --- Form: Select server ---

	var selectedID string
	serverOpts := buildServerOptions(servers)

	height := max(len(serverOpts), 5)
	if height > 12 {
		height = 12
	}

	selectField := huh.NewSelect[string]().
		Title("Select a server").
		Options(serverOpts...).
		Value(&selectedID).
		Height(height)

	if err := runForm(accessible, huh.NewGroup(selectField)); err != nil {
		if errors.Is(err, ErrAborted) {
			return nil, ErrShowAborted
		}
		return nil, err
	}

	server := serverByID[selectedID]
	return &server, nil
}
