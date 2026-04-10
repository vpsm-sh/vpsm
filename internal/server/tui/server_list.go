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

// --- Messages ---

type serversLoadedMsg struct {
	servers []domain.Server
}

type serversErrorMsg struct {
	err error
}

// --- Server list model ---

type serverListModel struct {
	provider     domain.Provider
	providerName string

	servers   []domain.Server
	cursor    int
	listStart int // for scrolling

	width  int
	height int

	loading bool
	spinner spinner.Model
	err     error
	status  string

	// persistentStatus holds a status message that should survive refresh cycles
	// (used for toggle results, delete/create confirmations).
	persistentStatus string
	// statusIsError controls whether the status bar renders in error style.
	statusIsError bool

	// poller encapsulates the start/stop polling state machine.
	poller togglePoller

	// Set when the user selects a server for detail/delete.
	selectedServer *domain.Server
	action         string // "show", "delete", or ""
	quitting       bool

	// embedded is true when this model is managed by serverAppModel.
	// When true, navigation actions emit messages instead of tea.Quit.
	embedded bool
}

// RunServerList starts the full-window interactive server list TUI.
// It returns the selected server (if any), the action to take, and any error.
func RunServerList(provider domain.Provider, providerName string) (*domain.Server, string, error) {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(styles.Blue)

	m := serverListModel{
		provider:     provider,
		providerName: providerName,
		loading:      true,
		spinner:      s,
		poller:       newTogglePoller(provider),
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return nil, "", fmt.Errorf("failed to run server list: %w", err)
	}

	final := result.(serverListModel)
	return final.selectedServer, final.action, nil
}

func (m serverListModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.fetchServers(),
	)
}

func (m serverListModel) fetchServers() tea.Cmd {
	return func() tea.Msg {
		servers, err := m.provider.ListServers(context.Background())
		if err != nil {
			return serversErrorMsg{err: err}
		}
		return serversLoadedMsg{servers: servers}
	}
}

// --- Update ---

func (m serverListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		if m.persistentStatus != "" {
			m.status = m.persistentStatus
			m.statusIsError = false
			m.persistentStatus = ""
		} else if len(m.servers) == 0 {
			m.status = "No servers found."
			m.statusIsError = false
		} else {
			m.status = fmt.Sprintf("%d server(s)", len(m.servers))
			m.statusIsError = false
		}
		return m, nil

	case serversErrorMsg:
		m.loading = false
		m.err = msg.err
		m.status = ""
		m.statusIsError = false
		return m, nil

	// --- Toggle lifecycle (delegated to togglePoller) ---

	case serverToggleInitiatedMsg:
		var cmd tea.Cmd
		var outcome *toggleOutcome
		m.poller, cmd, outcome = m.poller.HandleInitiated(msg)
		return m.applyToggleOutcome(outcome, cmd)

	case serverToggleErrorMsg:
		m.poller.active = false
		m.status = msg.err.Error()
		m.statusIsError = true
		return m, nil

	case pollActionTickMsg:
		var cmd tea.Cmd
		m.poller, cmd = m.poller.HandlePollTick()
		return m, cmd

	case pollActionResultMsg:
		var cmd tea.Cmd
		var outcome *toggleOutcome
		m.poller, cmd, outcome = m.poller.HandlePollResult(msg)
		return m.applyToggleOutcome(outcome, cmd)

	case pollActionErrorMsg:
		var cmd tea.Cmd
		var outcome *toggleOutcome
		m.poller, cmd, outcome = m.poller.HandlePollError(msg)
		return m.applyToggleOutcome(outcome, cmd)

	case spinner.TickMsg:
		needsSpinner := m.loading || (!m.embedded && m.poller.active)
		if needsSpinner {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	return m, nil
}

// applyToggleOutcome interprets a toggleOutcome from the poller and updates
// the model accordingly. When the outcome signals success, the server list
// is refreshed. On error/timeout the status bar is updated.
func (m serverListModel) applyToggleOutcome(outcome *toggleOutcome, pollerCmd tea.Cmd) (tea.Model, tea.Cmd) {
	if outcome == nil {
		// Still polling — sync status text from poller.
		m.status = m.poller.statusText
		m.statusIsError = m.poller.statusError
		return m, pollerCmd
	}

	if outcome.Success {
		m.loading = true
		m.persistentStatus = fmt.Sprintf("Server %q %s successfully", outcome.ServerName, outcome.Verb)
		return m, tea.Batch(m.spinner.Tick, m.fetchServers())
	}

	// Error or timeout.
	m.status = outcome.StatusText
	m.statusIsError = outcome.IsError
	return m, nil
}

// updateScroll ensures the cursor is always visible within the listStart bounds.
func (m *serverListModel) updateScroll() {
	headerH, footerH, statusH := 3, 1, 1 // approximate
	contentH := max(m.height-headerH-footerH-statusH, 1)
	visibleRows := max(contentH-3, 1)

	if m.cursor < m.listStart {
		m.listStart = m.cursor
	} else if m.cursor >= m.listStart+visibleRows {
		m.listStart = m.cursor - visibleRows + 1
	}
}

// --- Key handling ---

func (m serverListModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Block input while loading (except ctrl+c).
	// When embedded, toggle polling is handled by the app-level overlay
	// so input is not blocked during operations.
	blocking := m.loading || (!m.embedded && m.poller.active)
	if blocking {
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c", "q", "esc":
		m.quitting = true
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.updateScroll()
		}

	case "down", "j":
		if m.cursor < len(m.servers)-1 {
			m.cursor++
			m.updateScroll()
		}

	case "g":
		m.cursor = 0
		m.updateScroll()

	case "G":
		if len(m.servers) > 0 {
			m.cursor = len(m.servers) - 1
			m.updateScroll()
		}

	case "enter":
		if len(m.servers) > 0 {
			server := m.servers[m.cursor]
			if m.embedded {
				return m, func() tea.Msg { return navigateToShowMsg{server: server} }
			}
			m.selectedServer = &server
			m.action = "show"
			return m, tea.Quit
		}

	case "d":
		if len(m.servers) > 0 {
			server := m.servers[m.cursor]
			if m.embedded {
				return m, func() tea.Msg { return navigateToDeleteMsg{server: server} }
			}
			m.selectedServer = &server
			m.action = "delete"
			return m, tea.Quit
		}

	case "s":
		if len(m.servers) > 0 {
			server := m.servers[m.cursor]
			if m.embedded {
				// Delegate to the app-level overlay via message.
				switch server.Status {
				case "running", "off", "stopped":
					return m, func() tea.Msg { return requestToggleMsg{server: server} }
				default:
					m.status = fmt.Sprintf("Cannot start/stop server %q: status is %q", server.Name, server.Status)
					m.statusIsError = true
				}
			} else {
				// Standalone mode: use the embedded poller.
				switch server.Status {
				case "running":
					m.poller.active = true
					m.status = fmt.Sprintf("Stopping server %q...", server.Name)
					m.statusIsError = false
					return m, tea.Batch(m.spinner.Tick, m.poller.InitiateToggle(server))
				case "off", "stopped":
					m.poller.active = true
					m.status = fmt.Sprintf("Starting server %q...", server.Name)
					m.statusIsError = false
					return m, tea.Batch(m.spinner.Tick, m.poller.InitiateToggle(server))
				default:
					m.status = fmt.Sprintf("Cannot start/stop server %q: status is %q", server.Name, server.Status)
					m.statusIsError = true
				}
			}
		}

	case "c":
		if m.embedded {
			return m, func() tea.Msg { return navigateToCreateMsg{} }
		}
		m.action = "create"
		return m, tea.Quit

	case "r":
		m.loading = true
		m.err = nil
		m.status = ""
		m.statusIsError = false
		return m, tea.Batch(m.spinner.Tick, m.fetchServers())
	}

	return m, nil
}

// --- View ---

func (m serverListModel) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	header := components.Header(m.width, "server list", m.providerName)

	var footerBindings []components.KeyBinding
	showReduced := m.loading || (!m.embedded && m.poller.active)
	if showReduced {
		footerBindings = []components.KeyBinding{
			{Key: "ctrl+c", Desc: "quit"},
		}
	} else {
		footerBindings = []components.KeyBinding{
			{Key: "j/k", Desc: "navigate"},
			{Key: "enter", Desc: "show"},
			{Key: "s", Desc: "start/stop"},
			{Key: "d", Desc: "delete"},
			{Key: "c", Desc: "create"},
			{Key: "r", Desc: "refresh"},
			{Key: "q", Desc: "quit"},
		}
	}
	footer := components.Footer(m.width, footerBindings)

	statusBar := ""
	if m.err != nil {
		statusBar = components.StatusBar(m.width, "Error: "+m.err.Error(), true)
	} else if m.status != "" {
		statusBar = components.StatusBar(m.width, m.status, m.statusIsError)
	}

	// Calculate available height for content.
	headerH := lipgloss.Height(header)
	footerH := lipgloss.Height(footer)
	statusH := lipgloss.Height(statusBar)
	contentH := max(m.height-headerH-footerH-statusH, 1)

	content := m.renderContent(contentH)

	// Assemble the full layout.
	sections := []string{header, content}
	if statusBar != "" {
		sections = append(sections, statusBar)
	}
	sections = append(sections, footer)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m serverListModel) renderContent(height int) string {
	if m.loading {
		loadingText := m.spinner.View() + "  Fetching servers…"
		return lipgloss.Place(
			m.width, height,
			lipgloss.Center, lipgloss.Center,
			styles.MutedText.Render(loadingText),
		)
	}

	if m.err != nil {
		return lipgloss.Place(
			m.width, height,
			lipgloss.Center, lipgloss.Center,
			styles.ErrorText.Render("Failed to load servers"),
		)
	}

	if len(m.servers) == 0 {
		empty := styles.MutedText.Render("No servers found. Press ") +
			styles.KeyStyle.Render("c") +
			styles.MutedText.Render(" to create one.")
		return lipgloss.Place(
			m.width, height,
			lipgloss.Center, lipgloss.Center,
			empty,
		)
	}

	return m.renderTable(height)
}

func (m serverListModel) renderTable(height int) string {
	// Define columns.
	type column struct {
		title string
		width int
	}

	// Calculate dynamic column widths based on terminal width.
	// Reserve some width for padding/borders.
	available := m.width - 4 // 2 padding on each side

	// Minimum column widths.
	cols := []column{
		{title: "NAME", width: 18},
		{title: "STATUS", width: 12},
		{title: "TYPE", width: 10},
		{title: "IPv4", width: 16},
		{title: "REGION", width: 8},
		{title: "IMAGE", width: 16},
	}

	// If terminal is wide enough, add ID column.
	totalMin := 0
	for _, c := range cols {
		totalMin += c.width
	}

	showID := available >= totalMin+14
	if showID {
		cols = append([]column{{title: "ID", width: 14}}, cols...)
	}

	// Distribute remaining width to the NAME column.
	total := 0
	for _, c := range cols {
		total += c.width
	}
	if available > total {
		extra := available - total
		for i := range cols {
			if cols[i].title == "NAME" {
				cols[i].width += extra
				break
			}
		}
	}

	// Render header row.
	headerCells := make([]string, len(cols))
	for i, col := range cols {
		headerCells[i] = styles.TableHeader.
			Width(col.width).
			Render(col.title)
	}
	headerRow := lipgloss.JoinHorizontal(lipgloss.Top, headerCells...)

	// Separator.
	sep := styles.MutedText.Render(strings.Repeat("─", available))

	// Render data rows.
	visibleRows := max(
		// header + sep + bottom padding
		height-3, 1)

	endIdx := m.listStart + visibleRows
	if endIdx > len(m.servers) {
		endIdx = len(m.servers)
	}

	rows := make([]string, 0, visibleRows)
	for i := m.listStart; i < endIdx; i++ {
		s := m.servers[i]
		isSelected := i == m.cursor

		cells := make([]string, 0, len(cols))
		for _, col := range cols {
			var value string
			switch col.title {
			case "ID":
				value = truncate(s.ID, col.width-2)
			case "NAME":
				value = truncate(s.Name, col.width-2)
			case "STATUS":
				if isSelected {
					value = truncate(s.Status, col.width-2)
				} else {
					// Use color-coded status for non-selected rows.
					cells = append(cells, styles.StatusStyle(s.Status).
						Width(col.width).
						Padding(0, 1).
						Render(s.Status))
					continue
				}
			case "TYPE":
				value = truncate(s.ServerType, col.width-2)
			case "IPv4":
				value = truncate(s.PublicIPv4, col.width-2)
			case "REGION":
				value = truncate(s.Region, col.width-2)
			case "IMAGE":
				value = truncate(s.Image, col.width-2)
			}

			cellStyle := styles.TableCell.Width(col.width)
			if isSelected {
				cellStyle = styles.TableSelectedRow.Width(col.width)
			}
			cells = append(cells, cellStyle.Render(value))
		}

		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}

	// Pad remaining space with empty rows.
	for len(rows) < visibleRows {
		rows = append(rows, "")
	}

	table := lipgloss.JoinVertical(lipgloss.Left,
		append([]string{headerRow, sep}, rows...)...,
	)

	return lipgloss.NewStyle().
		Padding(0, 2).
		Render(table)
}

// truncate shortens a string to fit the given width with an ellipsis.
func truncate(s string, maxWidth int) string {
	if maxWidth < 1 {
		return ""
	}
	if len(s) <= maxWidth {
		return s
	}
	if maxWidth <= 3 {
		return s[:maxWidth]
	}
	return s[:maxWidth-1] + "…"
}
