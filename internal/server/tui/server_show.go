package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nathanbeddoewebdev/vpsm/internal/server/domain"
	"nathanbeddoewebdev/vpsm/internal/tui/components"
	"nathanbeddoewebdev/vpsm/internal/tui/styles"
	"nathanbeddoewebdev/vpsm/internal/util"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Messages ---

type serverDetailLoadedMsg struct {
	server *domain.Server
}

type serverDetailErrorMsg struct {
	err error
}

type metricsLoadedMsg struct {
	metrics *domain.ServerMetrics
}

type metricsErrorMsg struct {
	err error
}

// --- Show result ---

// ShowResult holds the outcome of the server show TUI.
type ShowResult struct {
	Server *domain.Server
	Action string // "delete" or ""
}

// --- Phases ---

type showPhase int

const (
	showPhaseSelect showPhase = iota // selecting a server from the list
	showPhaseDetail                  // displaying server details
)

// --- Server show model ---

type serverShowModel struct {
	provider     domain.Provider
	providerName string

	phase showPhase

	// Select phase.
	servers   []domain.Server
	cursor    int
	listStart int

	// Detail phase.
	serverID string
	server   *domain.Server

	// Whether we came from the select phase (enables going back).
	fromSelect bool

	width  int
	height int

	loading bool
	spinner spinner.Model
	err     error

	// Status bar state.
	status        string
	statusIsError bool

	// persistentStatus holds a status message that should survive refresh cycles
	// (used for toggle results, SSH errors, delete/create confirmations).
	persistentStatus string

	// poller encapsulates the start/stop polling state machine.
	poller togglePoller

	action   string
	quitting bool

	// Metrics state (loaded independently from server detail).
	metrics        *domain.ServerMetrics
	metricsLoading bool
	metricsErr     error

	// Viewport for scrollable detail view.
	viewport viewport.Model

	// embedded is true when this model is managed by serverAppModel.
	// When true, navigation actions emit messages instead of tea.Quit.
	embedded bool
}

// detailViewportKeyMap returns a viewport KeyMap that avoids conflicts with
// the detail view's own key bindings (d=delete, s=start/stop).
func detailViewportKeyMap() viewport.KeyMap {
	return viewport.KeyMap{
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdn", "page down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "page up"),
		),
		HalfPageUp: key.NewBinding(
			key.WithKeys("ctrl+u"),
			key.WithHelp("ctrl+u", "½ page up"),
		),
		HalfPageDown: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("ctrl+d", "½ page down"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Left: key.NewBinding(
			key.WithDisabled(),
		),
		Right: key.NewBinding(
			key.WithDisabled(),
		),
	}
}

// RunServerShow starts the full-window server detail TUI.
// If serverID is empty, the TUI first shows a server selection list.
func RunServerShow(provider domain.Provider, providerName string, serverID string) (*ShowResult, error) {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(styles.Blue)

	vp := viewport.New(0, 0)
	vp.KeyMap = detailViewportKeyMap()

	m := serverShowModel{
		provider:     provider,
		providerName: providerName,
		loading:      true,
		spinner:      s,
		poller:       newTogglePoller(provider, providerName),
		viewport:     vp,
	}

	if serverID != "" {
		// Direct detail fetch.
		m.phase = showPhaseDetail
		m.serverID = serverID
	} else {
		// Select-then-detail flow.
		m.phase = showPhaseSelect
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to run server show: %w", err)
	}

	final := result.(serverShowModel)
	if final.quitting || final.server == nil {
		return nil, nil
	}
	return &ShowResult{Server: final.server, Action: final.action}, nil
}

// RunServerShowDirect starts the detail view with an already-loaded server.
func RunServerShowDirect(provider domain.Provider, providerName string, server *domain.Server) (*ShowResult, error) {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(styles.Blue)

	vp := viewport.New(0, 0)
	vp.KeyMap = detailViewportKeyMap()

	m := serverShowModel{
		provider:       provider,
		providerName:   providerName,
		phase:          showPhaseDetail,
		server:         server,
		serverID:       server.ID,
		loading:        false,
		metricsLoading: true,
		spinner:        s,
		poller:         newTogglePoller(provider, providerName),
		viewport:       vp,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to run server show: %w", err)
	}

	final := result.(serverShowModel)
	if final.quitting {
		return nil, nil
	}
	return &ShowResult{Server: final.server, Action: final.action}, nil
}

func (m serverShowModel) Init() tea.Cmd {
	if m.loading {
		switch m.phase {
		case showPhaseSelect:
			return tea.Batch(m.spinner.Tick, m.fetchServers())
		case showPhaseDetail:
			return tea.Batch(m.spinner.Tick, m.fetchServer())
		}
	}

	// When server is already loaded (RunServerShowDirect), kick off metrics.
	if !m.loading && m.server != nil && m.metricsLoading {
		return tea.Batch(m.spinner.Tick, m.fetchMetrics())
	}
	return nil
}

func (m serverShowModel) fetchServers() tea.Cmd {
	return func() tea.Msg {
		servers, err := m.provider.ListServers(context.Background())
		if err != nil {
			return serversErrorMsg{err: err}
		}
		return serversLoadedMsg{servers: servers}
	}
}

func (m serverShowModel) fetchServer() tea.Cmd {
	return func() tea.Msg {
		server, err := m.provider.GetServer(context.Background(), m.serverID)
		if err != nil {
			return serverDetailErrorMsg{err: err}
		}
		return serverDetailLoadedMsg{server: server}
	}
}

func (m serverShowModel) fetchMetrics() tea.Cmd {
	return func() tea.Msg {
		mp, ok := m.provider.(domain.MetricsProvider)
		if !ok {
			return metricsErrorMsg{err: fmt.Errorf("provider does not support metrics")}
		}

		end := time.Now()
		start := end.Add(-1 * time.Hour)
		metrics, err := mp.GetServerMetrics(context.Background(), m.serverID, []domain.MetricType{
			domain.MetricCPU,
			domain.MetricDisk,
			domain.MetricNetwork,
		}, start, end)
		if err != nil {
			return metricsErrorMsg{err: err}
		}
		return metricsLoadedMsg{metrics: metrics}
	}
}

// --- Update ---

func (m serverShowModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Viewport size is set dynamically in View() based on
		// header/footer/status heights, so just store dimensions here.
		return m, nil

	case tea.KeyMsg:
		model, cmd := m.handleKey(msg)
		updated := model.(serverShowModel)
		// Forward to viewport for scrolling in detail phase.
		if updated.phase == showPhaseDetail && !updated.loading && updated.server != nil {
			updated.viewport, _ = updated.viewport.Update(msg)
		}
		return updated, cmd

	case tea.MouseMsg:
		// Forward mouse events to viewport for scroll wheel in detail phase.
		if m.phase == showPhaseDetail && !m.loading && m.server != nil {
			m.viewport, _ = m.viewport.Update(msg)
		}
		return m, nil

	case serversLoadedMsg:
		m.loading = false
		m.servers = msg.servers
		m.err = nil
		return m, nil

	case serversErrorMsg:
		m.loading = false
		m.err = msg.err
		return m, nil

	case serverDetailLoadedMsg:
		m.loading = false
		m.server = msg.server
		m.err = nil
		if m.persistentStatus != "" {
			m.status = m.persistentStatus
			m.statusIsError = false
			m.persistentStatus = ""
		} else {
			m.status = ""
			m.statusIsError = false
		}
		// Kick off async metrics fetch (non-blocking).
		m.metricsLoading = true
		m.metrics = nil
		m.metricsErr = nil
		return m, tea.Batch(m.spinner.Tick, m.fetchMetrics())

	case serverDetailErrorMsg:
		m.loading = false
		m.err = msg.err
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

	// --- Metrics lifecycle ---

	case metricsLoadedMsg:
		m.metricsLoading = false
		m.metrics = msg.metrics
		m.metricsErr = nil
		return m, nil

	case metricsErrorMsg:
		m.metricsLoading = false
		m.metricsErr = msg.err
		return m, nil

	case spinner.TickMsg:
		needsSpinner := m.loading || m.metricsLoading || (!m.embedded && m.poller.active)
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
// the model accordingly. When the outcome signals success, the server detail
// is refreshed. On error/timeout the status bar is updated.
func (m serverShowModel) applyToggleOutcome(outcome *toggleOutcome, pollerCmd tea.Cmd) (tea.Model, tea.Cmd) {
	if outcome == nil {
		// Still polling — sync status text from poller.
		m.status = m.poller.statusText
		m.statusIsError = m.poller.statusError
		return m, pollerCmd
	}

	if outcome.Success {
		m.persistentStatus = fmt.Sprintf("Server %q %s successfully", outcome.ServerName, outcome.Verb)
		if m.phase == showPhaseDetail && m.server != nil {
			m.loading = true
			m.err = nil
			m.serverID = m.server.ID
			return m, tea.Batch(m.spinner.Tick, m.fetchServer())
		}
		return m, nil
	}

	// Error or timeout.
	m.status = outcome.StatusText
	m.statusIsError = outcome.IsError
	return m, nil
}

// --- Key handling ---

func (m serverShowModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
	}

	// Block input while loading (or toggling in standalone mode).
	blocking := m.loading || (!m.embedded && m.poller.active)
	if blocking {
		return m, nil
	}

	switch m.phase {
	case showPhaseSelect:
		return m.handleSelectKey(msg)
	case showPhaseDetail:
		return m.handleDetailKey(msg)
	}

	return m, nil
}

func (m serverShowModel) handleSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.err != nil {
		if msg.String() == "q" || msg.String() == "esc" {
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil
	}

	switch msg.String() {
	case "q", "esc":
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
			selected := m.servers[m.cursor]
			m.serverID = selected.ID
			m.phase = showPhaseDetail
			m.fromSelect = true
			m.loading = true
			m.err = nil
			m.status = ""
			m.statusIsError = false
			return m, tea.Batch(m.spinner.Tick, m.fetchServer())
		}
	}

	return m, nil
}

func (m serverShowModel) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		if m.fromSelect {
			// Go back to the select phase.
			m.phase = showPhaseSelect
			m.server = nil
			m.serverID = ""
			m.err = nil
			m.status = ""
			m.statusIsError = false
			m.metrics = nil
			m.metricsLoading = false
			m.metricsErr = nil
			m.viewport.GotoTop()
			return m, nil
		}
		if m.embedded {
			return m, func() tea.Msg { return navigateBackMsg{} }
		}
		m.quitting = true
		return m, tea.Quit

	case "d":
		if m.server != nil {
			if m.embedded {
				server := *m.server
				return m, func() tea.Msg { return navigateToDeleteMsg{server: server} }
			}
			m.action = "delete"
			return m, tea.Quit
		}

	case "s":
		if m.server != nil {
			if m.embedded {
				// Delegate to the app-level overlay via message.
				switch m.server.Status {
				case "running", "off", "stopped":
					server := domain.Server{ID: m.server.ID, Name: m.server.Name, Status: m.server.Status}
					return m, func() tea.Msg { return requestToggleMsg{server: server} }
				default:
					m.status = fmt.Sprintf("Cannot start/stop server %q: status is %q", m.server.Name, m.server.Status)
					m.statusIsError = true
				}
			} else {
				// Standalone mode: use the embedded poller.
				switch m.server.Status {
				case "running":
					m.poller.active = true
					m.status = fmt.Sprintf("Stopping server %q...", m.server.Name)
					m.statusIsError = false
					return m, tea.Batch(m.spinner.Tick, m.poller.InitiateToggle(domain.Server{ID: m.server.ID, Name: m.server.Name, Status: m.server.Status}))
				case "off", "stopped":
					m.poller.active = true
					m.status = fmt.Sprintf("Starting server %q...", m.server.Name)
					m.statusIsError = false
					return m, tea.Batch(m.spinner.Tick, m.poller.InitiateToggle(domain.Server{ID: m.server.ID, Name: m.server.Name, Status: m.server.Status}))
				default:
					m.status = fmt.Sprintf("Cannot start/stop server %q: status is %q", m.server.Name, m.server.Status)
					m.statusIsError = true
				}
			}
		}

	case "r":
		if m.server != nil {
			m.loading = true
			m.serverID = m.server.ID
			m.err = nil
			m.status = ""
			m.statusIsError = false
			m.metrics = nil
			m.metricsLoading = false
			m.metricsErr = nil
			m.viewport.GotoTop()
			return m, tea.Batch(m.spinner.Tick, m.fetchServer())
		}

	case "c":
		if m.server != nil && m.embedded && m.server.Status == "running" {
			hasPublicIP := m.server.PublicIPv4 != "" || m.server.PublicIPv6 != ""
			if hasPublicIP {
				server := *m.server
				return m, func() tea.Msg { return navigateToSSHMsg{server: server} }
			}
		}
	}

	return m, nil
}

// --- View ---

func (m serverShowModel) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	header := components.Header(m.width, "server show", m.providerName)

	var footerBindings []components.KeyBinding
	showReduced := m.loading || (!m.embedded && m.poller.active)
	switch {
	case showReduced:
		footerBindings = []components.KeyBinding{{Key: "ctrl+c", Desc: "quit"}}
	case m.phase == showPhaseSelect:
		footerBindings = []components.KeyBinding{
			{Key: "j/k", Desc: "navigate"},
			{Key: "enter", Desc: "select"},
			{Key: "q", Desc: "quit"},
		}
	case m.phase == showPhaseDetail:
		bindings := []components.KeyBinding{
			{Key: "j/k", Desc: "scroll"},
			{Key: "s", Desc: "start/stop"},
			{Key: "d", Desc: "delete"},
			{Key: "r", Desc: "refresh"},
		}
		// Show SSH keybinding if server is running and has a public IP.
		canSSH := m.server != nil && m.server.Status == "running" && (m.server.PublicIPv4 != "" || m.server.PublicIPv6 != "")
		if canSSH {
			bindings = append(bindings, components.KeyBinding{Key: "c", Desc: "ssh"})
		}
		if m.fromSelect {
			bindings = append(bindings, components.KeyBinding{Key: "esc", Desc: "back"})
		}
		bindings = append(bindings, components.KeyBinding{Key: "q", Desc: "quit"})
		footerBindings = bindings
	}
	footer := components.Footer(m.width, footerBindings)

	statusBar := ""
	if m.err != nil {
		// Errors are rendered inline in the content area, not in the status bar.
	} else if m.status != "" {
		statusBar = components.StatusBar(m.width, m.status, m.statusIsError)
	}

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

func (m serverShowModel) renderContent(height int) string {
	if m.loading {
		var loadingLabel string
		switch m.phase {
		case showPhaseSelect:
			loadingLabel = "Fetching servers…"
		default:
			loadingLabel = "Fetching server details…"
		}
		loadingText := m.spinner.View() + "  " + loadingLabel
		return lipgloss.Place(
			m.width, height,
			lipgloss.Center, lipgloss.Center,
			styles.MutedText.Render(loadingText),
		)
	}

	// In standalone mode, replace content with a centered spinner during
	// toggle. In embedded mode, the app-level overlay handles the visual.
	if !m.embedded && m.poller.active {
		toggleText := m.spinner.View() + "  " + m.status
		return lipgloss.Place(
			m.width, height,
			lipgloss.Center, lipgloss.Center,
			styles.MutedText.Render(toggleText),
		)
	}

	if m.err != nil {
		backHint := "Press q to go back."
		if m.fromSelect && m.phase == showPhaseDetail {
			backHint = "Press esc to go back."
		}
		errText := styles.ErrorText.Render("Error: "+m.err.Error()) + "\n\n" +
			styles.MutedText.Render(backHint)
		return lipgloss.Place(
			m.width, height,
			lipgloss.Center, lipgloss.Center,
			errText,
		)
	}

	switch m.phase {
	case showPhaseSelect:
		return m.renderSelectPhase(height)
	case showPhaseDetail:
		return m.renderDetailPhase(height)
	}

	return ""
}

func (m serverShowModel) renderSelectPhase(height int) string {
	if len(m.servers) == 0 {
		return lipgloss.Place(
			m.width, height,
			lipgloss.Center, lipgloss.Center,
			styles.MutedText.Render("No servers found."),
		)
	}

	title := styles.Title.Render("Select a server")

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

func (m serverShowModel) renderDetailPhase(height int) string {
	if m.server == nil {
		return lipgloss.Place(
			m.width, height,
			lipgloss.Center, lipgloss.Center,
			styles.MutedText.Render("No server data."),
		)
	}

	// Render the full detail content (unconstrained by height).
	detail := m.renderDetail()

	// Use a local copy of the viewport for rendering. The persistent
	// viewport (with YOffset) lives on the model and is updated in Update().
	vp := m.viewport
	vp.Width = m.width
	vp.Height = height
	vp.SetContent(detail)

	return vp.View()
}

func (m serverShowModel) renderDetail() string {
	s := m.server

	// Two-column layout: info on left, metrics on right.
	// Horizontal padding of 2 on each side matches header/footer.
	const hPad = 2
	const columnGap = 2

	// Usable width after horizontal padding on both sides.
	usableWidth := max(m.width-(hPad*2), 60)

	// Left column: ~45% of width, giving room for long values.
	leftWidth := min(usableWidth*45/100, 52)
	if leftWidth < 34 {
		leftWidth = 34
	}

	// Right column gets the rest. Needs enough room for Y-axis labels + chart.
	rightWidth := max(usableWidth-leftWidth-columnGap, 36)

	labelWidth := 14
	valueWidth := max(
		// padding + border
		leftWidth-labelWidth-8, 6)

	renderField := func(label, value string) string {
		l := styles.Label.Width(labelWidth).Render(label)
		v := styles.Value.Width(valueWidth).Render(value)
		return l + v
	}

	// Server name + status header.
	nameTitle := styles.Title.Render(s.Name)
	statusBadge := styles.StatusIndicator(s.Status)
	titleLine := nameTitle + "  " + statusBadge

	// --- Overview section ---
	overviewFields := []string{
		renderField("ID", s.ID),
		renderField("Provider", s.Provider),
		renderField("Type", s.ServerType),
		renderField("Region", s.Region),
	}
	if s.Image != "" {
		overviewFields = append(overviewFields, renderField("Image", s.Image))
	}
	if !s.CreatedAt.IsZero() {
		overviewFields = append(overviewFields, renderField("Created", s.CreatedAt.UTC().Format("2006-01-02 15:04:05 UTC")))
	}

	overviewContent := strings.Join(overviewFields, "\n")

	// --- Network section ---
	var networkFields []string
	if s.PublicIPv4 != "" {
		networkFields = append(networkFields, renderField("IPv4", s.PublicIPv4))
	}
	if s.PublicIPv6 != "" {
		networkFields = append(networkFields, renderField("IPv6", s.PublicIPv6))
	}
	if s.PrivateIPv4 != "" {
		networkFields = append(networkFields, renderField("Private IP", s.PrivateIPv4))
	}

	// Build left column (info cards).
	leftStyle := styles.Card.Width(leftWidth)

	leftSections := []string{
		titleLine,
		"",
		leftStyle.Render(
			styles.Subtitle.Render("Overview") + "\n\n" + overviewContent,
		),
	}

	if len(networkFields) > 0 {
		networkContent := strings.Join(networkFields, "\n")
		leftSections = append(leftSections, leftStyle.Render(
			styles.Subtitle.Render("Network")+"\n\n"+networkContent,
		))
	}

	leftColumn := lipgloss.JoinVertical(lipgloss.Left, leftSections...)

	// Build right column (metrics).
	rightStyle := styles.Card.Width(rightWidth)
	rightColumn := m.renderMetricsSection(rightWidth, rightStyle)

	// Join columns horizontally with a gap.
	gap := strings.Repeat(" ", columnGap)
	var detail string
	if rightColumn != "" {
		detail = lipgloss.JoinHorizontal(lipgloss.Top, leftColumn, gap, rightColumn)
	} else {
		detail = leftColumn
	}

	// Pad left to align with header/footer.
	return lipgloss.NewStyle().PaddingLeft(hPad).Render(detail)
}

// renderMetricsSection renders the metrics card with loading/error/chart states.
func (m serverShowModel) renderMetricsSection(cardWidth int, sectionStyle lipgloss.Style) string {
	if m.metricsLoading {
		metricsContent := m.spinner.View() + "  Loading metrics…"
		return sectionStyle.Render(
			styles.Subtitle.Render("Metrics") + "\n\n" + styles.MutedText.Render(metricsContent),
		)
	}

	if m.metricsErr != nil {
		return sectionStyle.Render(
			styles.Subtitle.Render("Metrics") + "\n\n" + styles.MutedText.Render("Failed to load metrics"),
		)
	}

	if m.metrics == nil {
		return ""
	}

	// Chart width = card inner content width.
	// Card has Border(1 each side) + Padding(2 each side) = 6 horizontal overhead.
	chartWidth := max(cardWidth-6, 20)

	var charts []string

	// CPU chart (single series, percentage).
	cpuData := extractMetricValues(m.metrics, "cpu")
	if len(cpuData) > 0 {
		charts = append(charts, components.MetricsChart("CPU", cpuData, chartWidth, "%"))
	}

	// Disk IOPS chart (dual: read + write). Blue/Yellow.
	diskRead := extractMetricValues(m.metrics, "disk.0.iops.read")
	diskWrite := extractMetricValues(m.metrics, "disk.0.iops.write")
	if len(diskRead) > 0 || len(diskWrite) > 0 {
		charts = append(charts, components.MetricsDualChart(
			"Disk IOPS", diskRead, diskWrite, "read", "write", chartWidth, "",
			components.DualChartColors{Color1: styles.Blue, Color2: styles.Yellow},
		))
	}

	// Network bandwidth chart (dual: in + out). Green/Red.
	netIn := extractMetricValues(m.metrics, "network.0.bandwidth.in")
	netOut := extractMetricValues(m.metrics, "network.0.bandwidth.out")
	if len(netIn) > 0 || len(netOut) > 0 {
		charts = append(charts, components.MetricsDualChart(
			"Network", netIn, netOut, "in", "out", chartWidth, "B/s",
			components.DualChartColors{Color1: styles.Green, Color2: styles.Red},
		))
	}

	if len(charts) == 0 {
		return sectionStyle.Render(
			styles.Subtitle.Render("Metrics") + "\n\n" + styles.MutedText.Render("No metric data available"),
		)
	}

	metricsContent := strings.Join(charts, "\n\n")
	return sectionStyle.Render(
		styles.Subtitle.Render("Metrics (last hour)") + "\n\n" + metricsContent,
	)
}

// extractMetricValues extracts the float64 values from a named time series.
func extractMetricValues(m *domain.ServerMetrics, key string) []float64 {
	key = util.NormalizeKey(key)
	ts, ok := m.TimeSeries[key]
	if !ok {
		return nil
	}
	vals := make([]float64, len(ts.Values))
	for i, p := range ts.Values {
		vals[i] = p.Value
	}
	return vals
}
