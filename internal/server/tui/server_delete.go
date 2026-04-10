package tui

import (
	"context"
	"fmt"
	"strings"

	"nathanbeddoewebdev/vpsm/internal/server/domain"
	"nathanbeddoewebdev/vpsm/internal/tui/components"
	"nathanbeddoewebdev/vpsm/internal/tui/styles"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Delete model ---

type deletePhase int

const (
	deletePhaseSelect  deletePhase = iota // selecting a server from the list
	deletePhaseConfirm                    // confirmation dialog for a known server
)

type serverDeleteModel struct {
	provider     domain.Provider
	providerName string

	phase deletePhase

	// Select phase.
	servers   []domain.Server
	cursor    int
	listStart int

	// Confirm phase.
	server     *domain.Server
	confirmIdx int // 0 = delete, 1 = cancel

	width  int
	height int

	loading bool
	spinner spinner.Model
	err     error

	confirmed bool
	quitting  bool

	// embedded is true when this model is managed by serverAppModel.
	// When true, navigation actions emit messages instead of tea.Quit.
	embedded bool
}

// DeleteResult holds the outcome of the server delete TUI.
type DeleteResult struct {
	Server    *domain.Server
	Confirmed bool
}

// RunServerDelete starts the interactive server deletion TUI.
// If serverToDelete is nil, it first shows a server selection list.
func RunServerDelete(provider domain.Provider, providerName string, serverToDelete *domain.Server) (*DeleteResult, error) {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(styles.Blue)

	m := serverDeleteModel{
		provider:     provider,
		providerName: providerName,
		spinner:      s,
	}

	if serverToDelete != nil {
		m.phase = deletePhaseConfirm
		m.server = serverToDelete
		m.loading = false
	} else {
		m.phase = deletePhaseSelect
		m.loading = true
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to run server delete: %w", err)
	}

	final := result.(serverDeleteModel)
	if final.quitting || final.server == nil {
		return nil, nil
	}
	return &DeleteResult{Server: final.server, Confirmed: final.confirmed}, nil
}

func (m serverDeleteModel) Init() tea.Cmd {
	if m.loading {
		return tea.Batch(m.spinner.Tick, m.fetchServers())
	}
	return nil
}

func (m serverDeleteModel) fetchServers() tea.Cmd {
	return func() tea.Msg {
		servers, err := m.provider.ListServers(context.Background())
		if err != nil {
			return serversErrorMsg{err: err}
		}
		return serversLoadedMsg{servers: servers}
	}
}

func (m serverDeleteModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case serversLoadedMsg:
		m.loading = false
		m.servers = msg.servers
		m.err = nil
		return m, nil

	case serversErrorMsg:
		m.loading = false
		m.err = msg.err
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	return m, nil
}

func (m serverDeleteModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
	}

	if m.loading {
		return m, nil
	}

	switch m.phase {
	case deletePhaseSelect:
		return m.handleSelectKey(msg)
	case deletePhaseConfirm:
		return m.handleConfirmKey(msg)
	}

	return m, nil
}

func (m serverDeleteModel) handleSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.err != nil {
		if msg.String() == "q" || msg.String() == "esc" {
			if m.embedded {
				return m, func() tea.Msg { return navigateBackMsg{} }
			}
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil
	}

	switch msg.String() {
	case "q", "esc":
		if m.embedded {
			return m, func() tea.Msg { return navigateBackMsg{} }
		}
		m.quitting = true
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.servers)-1 {
			m.cursor++
		}
	case "g":
		m.cursor = 0
	case "G":
		if len(m.servers) > 0 {
			m.cursor = len(m.servers) - 1
		}
	case "enter":
		if len(m.servers) > 0 {
			server := m.servers[m.cursor]
			m.server = &server
			m.phase = deletePhaseConfirm
			m.confirmIdx = 1 // default to cancel for safety
			return m, nil
		}
	}

	return m, nil
}

func (m serverDeleteModel) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if len(m.servers) > 0 {
			// Go back to select phase.
			m.phase = deletePhaseSelect
			m.server = nil
			return m, nil
		}
		if m.embedded {
			return m, func() tea.Msg { return navigateBackMsg{} }
		}
		m.quitting = true
		return m, tea.Quit
	case "left", "h":
		if m.confirmIdx > 0 {
			m.confirmIdx--
		}
	case "right", "l":
		if m.confirmIdx < 1 {
			m.confirmIdx++
		}
	case "enter":
		if m.confirmIdx == 0 {
			if m.embedded && m.server != nil {
				server := *m.server
				return m, func() tea.Msg { return deleteConfirmedMsg{server: server} }
			}
			m.confirmed = true
			return m, tea.Quit
		}
		if m.embedded {
			return m, func() tea.Msg { return navigateBackMsg{} }
		}
		m.quitting = true
		return m, tea.Quit
	case "y":
		if m.embedded && m.server != nil {
			server := *m.server
			return m, func() tea.Msg { return deleteConfirmedMsg{server: server} }
		}
		m.confirmed = true
		return m, tea.Quit
	case "n":
		if m.embedded {
			return m, func() tea.Msg { return navigateBackMsg{} }
		}
		m.quitting = true
		return m, tea.Quit
	}

	return m, nil
}

func (m serverDeleteModel) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	header := components.Header(m.width, "server delete", m.providerName)

	var footerBindings []components.KeyBinding
	switch {
	case m.loading:
		footerBindings = []components.KeyBinding{{Key: "ctrl+c", Desc: "quit"}}
	case m.phase == deletePhaseSelect:
		footerBindings = []components.KeyBinding{
			{Key: "j/k", Desc: "navigate"},
			{Key: "enter", Desc: "select"},
			{Key: "q", Desc: "quit"},
		}
	case m.phase == deletePhaseConfirm:
		footerBindings = []components.KeyBinding{
			{Key: "y/n", Desc: "confirm"},
			{Key: "enter", Desc: "select"},
			{Key: "esc", Desc: "back"},
		}
	}
	footer := components.Footer(m.width, footerBindings)

	headerH := lipgloss.Height(header)
	footerH := lipgloss.Height(footer)
	contentH := max(m.height-headerH-footerH, 1)

	content := m.renderContent(contentH)

	return lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
}

func (m serverDeleteModel) renderContent(height int) string {
	if m.loading {
		loadingText := m.spinner.View() + "  Fetching servers..."
		return lipgloss.Place(
			m.width, height,
			lipgloss.Center, lipgloss.Center,
			styles.MutedText.Render(loadingText),
		)
	}

	if m.err != nil {
		errText := styles.ErrorText.Render("Error: "+m.err.Error()) + "\n\n" +
			styles.MutedText.Render("Press q to go back.")
		return lipgloss.Place(
			m.width, height,
			lipgloss.Center, lipgloss.Center,
			errText,
		)
	}

	switch m.phase {
	case deletePhaseSelect:
		return m.renderSelectPhase(height)
	case deletePhaseConfirm:
		return m.renderConfirmPhase(height)
	}

	return ""
}

func (m serverDeleteModel) renderSelectPhase(height int) string {
	if len(m.servers) == 0 {
		return lipgloss.Place(
			m.width, height,
			lipgloss.Center, lipgloss.Center,
			styles.MutedText.Render("No servers found."),
		)
	}

	title := styles.Title.Render("Select a server to delete")

	maxVisible := max(height-4, 3)

	// Scrolling.
	start := min(m.cursor, m.listStart)
	if m.cursor >= start+maxVisible {
		start = m.cursor - maxVisible + 1
	}

	end := min(start+maxVisible, len(m.servers))

	rows := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		s := m.servers[i]
		prefix := "  "
		if i == m.cursor {
			prefix = styles.AccentText.Render("> ")
		}

		label := serverOptionLabel(s)
		if i == m.cursor {
			label = styles.Value.Bold(true).Render(label)
		} else {
			label = styles.MutedText.Render(label)
		}

		rows = append(rows, prefix+label)
	}

	listContent := strings.Join(rows, "\n")

	combined := lipgloss.JoinVertical(lipgloss.Left, title, "", listContent)
	return lipgloss.Place(
		m.width, height,
		lipgloss.Center, lipgloss.Center,
		combined,
	)
}

func (m serverDeleteModel) renderConfirmPhase(height int) string {
	s := m.server

	cardWidth := 56
	labelWidth := 14

	renderField := func(label, value string) string {
		l := styles.Label.Width(labelWidth).Render(label)
		v := styles.Value.Render(value)
		return l + v
	}

	// Warning.
	warning := styles.WarningText.Render("This action cannot be undone.")

	// Server details.
	fields := []string{
		renderField("ID", s.ID),
		renderField("Name", s.Name),
		renderField("Status", s.Status),
	}
	if s.ServerType != "" {
		fields = append(fields, renderField("Type", s.ServerType))
	}
	if s.Image != "" {
		fields = append(fields, renderField("Image", s.Image))
	}
	if s.Region != "" {
		fields = append(fields, renderField("Region", s.Region))
	}
	if s.PublicIPv4 != "" {
		fields = append(fields, renderField("IPv4", s.PublicIPv4))
	}

	detailContent := strings.Join(fields, "\n")
	detail := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Red).
		Padding(1, 2).
		Width(cardWidth).
		Render(detailContent)

	// Buttons.
	deleteBtn := "  Delete  "
	cancelBtn := "  Cancel  "

	if m.confirmIdx == 0 {
		deleteBtn = lipgloss.NewStyle().
			Background(styles.Red).
			Foreground(lipgloss.Color("#000000")).
			Bold(true).
			Render(deleteBtn)
		cancelBtn = styles.MutedText.Render(cancelBtn)
	} else {
		deleteBtn = styles.MutedText.Render(deleteBtn)
		cancelBtn = lipgloss.NewStyle().
			Background(styles.DimGray).
			Foreground(styles.White).
			Bold(true).
			Render(cancelBtn)
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Center, deleteBtn, "  ", cancelBtn)

	title := styles.Title.Render("Delete server?")

	combined := lipgloss.JoinVertical(lipgloss.Center,
		title,
		"",
		warning,
		"",
		detail,
		"",
		buttons,
	)

	return lipgloss.Place(
		m.width, height,
		lipgloss.Center, lipgloss.Center,
		combined,
	)
}
