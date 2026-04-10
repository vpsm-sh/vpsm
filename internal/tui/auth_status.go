package tui

import (
	"errors"
	"fmt"
	"strings"

	providernames "nathanbeddoewebdev/vpsm/internal/platform/providers/names"
	"nathanbeddoewebdev/vpsm/internal/services/auth"
	"nathanbeddoewebdev/vpsm/internal/tui/components"
	"nathanbeddoewebdev/vpsm/internal/tui/styles"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Provider status ---

type providerStatus struct {
	name   string
	status string // "authenticated", "not authenticated", or error message
	ok     bool
}

// --- Auth status model ---

type authStatusModel struct {
	store auth.Store

	statuses []providerStatus

	width  int
	height int
}

// RunAuthStatus starts the full-window auth status TUI.
func RunAuthStatus(store auth.Store) error {
	providerNames := providernames.List()

	statuses := make([]providerStatus, 0, len(providerNames))
	for _, name := range providerNames {
		_, err := store.GetToken(name)
		switch {
		case err == nil:
			statuses = append(statuses, providerStatus{name: name, status: "authenticated", ok: true})
		case errors.Is(err, auth.ErrTokenNotFound):
			statuses = append(statuses, providerStatus{name: name, status: "not authenticated", ok: false})
		default:
			statuses = append(statuses, providerStatus{name: name, status: fmt.Sprintf("error: %v", err), ok: false})
		}
	}

	m := authStatusModel{
		store:    store,
		statuses: statuses,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m authStatusModel) Init() tea.Cmd {
	return nil
}

func (m authStatusModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m authStatusModel) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	header := components.Header(m.width, "auth status", "")
	footerBindings := []components.KeyBinding{
		{Key: "q", Desc: "quit"},
	}
	footer := components.Footer(m.width, footerBindings)

	headerH := lipgloss.Height(header)
	footerH := lipgloss.Height(footer)
	contentH := max(m.height-headerH-footerH, 1)

	content := m.renderContent(contentH)

	return lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
}

func (m authStatusModel) renderContent(height int) string {
	if len(m.statuses) == 0 {
		return lipgloss.Place(
			m.width, height,
			lipgloss.Center, lipgloss.Center,
			styles.MutedText.Render("No providers registered."),
		)
	}

	title := styles.Title.Render("Provider Authentication")

	cardWidth := 48
	labelWidth := 16

	rows := make([]string, 0, len(m.statuses))
	for _, ps := range m.statuses {
		nameStyle := styles.Label.Width(labelWidth)
		name := nameStyle.Render(ps.name)

		var statusText string
		if ps.ok {
			statusText = styles.SuccessText.Render("authenticated")
		} else {
			statusText = styles.MutedText.Render(ps.status)
		}

		rows = append(rows, name+statusText)
	}

	var content strings.Builder
	for i, row := range rows {
		content.WriteString(row)
		if i < len(rows)-1 {
			content.WriteString("\n")
		}
	}

	card := styles.Card.Width(cardWidth).Render(content.String())

	combined := lipgloss.JoinVertical(lipgloss.Center, title, "", card)

	return lipgloss.Place(
		m.width, height,
		lipgloss.Center, lipgloss.Center,
		combined,
	)
}
