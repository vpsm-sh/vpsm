package tui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"nathanbeddoewebdev/vpsm/internal/server/domain"
	"nathanbeddoewebdev/vpsm/internal/serverprefs"
	prefssvc "nathanbeddoewebdev/vpsm/internal/services/serverprefs"
	"nathanbeddoewebdev/vpsm/internal/tui/components"
	"nathanbeddoewebdev/vpsm/internal/tui/styles"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Navigation messages ---
//
// These are sent by child models to request view transitions within the
// single Bubbletea program. The top-level serverAppModel handles them.

type navigateToListMsg struct{}

type navigateToShowMsg struct {
	server domain.Server
}

type navigateToDeleteMsg struct {
	server domain.Server
}

type navigateToCreateMsg struct{}

type navigateToSSHMsg struct {
	server domain.Server
}

// navigateBackMsg asks the app to return to the previous view (or the list).
type navigateBackMsg struct{}

// --- Action messages ---
//
// Sent by child models when the user confirms a destructive/creative action.
// The app model performs the API call and navigates back to the list.

type deleteConfirmedMsg struct {
	server domain.Server
}

type createConfirmedMsg struct {
	opts domain.CreateServerOpts
}

// --- Action result messages ---

type deleteResultMsg struct {
	server domain.Server
	err    error
}

type createResultMsg struct {
	server *domain.Server
	err    error
}

// --- SSH messages ---

// requestSSHMsg is emitted by the show model when the user confirms SSH.
type requestSSHMsg struct {
	server    domain.Server
	username  string
	ipAddress string
}

// sshErrKind categorizes SSH connection failures for appropriate error handling.
type sshErrKind int

const (
	sshErrNone sshErrKind = iota
	sshErrHostKeyConflict
	sshErrGeneric
)

// sshFinishedMsg is returned by the tea.ExecProcess callback.
type sshFinishedMsg struct {
	server    domain.Server
	username  string // carried forward for retry
	ipAddress string // carried forward for retry
	err       error
	errKind   sshErrKind
	errDetail string // human-readable message extracted from SSH stderr
}

// clearHostKeyMsg requests removal of a stale SSH host key and connection retry.
type clearHostKeyMsg struct {
	server    domain.Server
	username  string
	ipAddress string
}

// --- App view ---

type appView int

const (
	appViewList appView = iota
	appViewShow
	appViewDelete
	appViewCreate
	appViewSSH
	appViewAction // performing an API call (delete/create)
)

// --- App model ---

// serverAppModel is a top-level Bubbletea model that manages transitions
// between the server list, show, delete, and create views within a single
// alt-screen session. This eliminates the flicker caused by exiting and
// re-entering separate Bubbletea programs.
type serverAppModel struct {
	provider     domain.Provider
	providerName string

	view appView

	// Child models.
	list   serverListModel
	show   serverShowModel
	delete serverDeleteModel
	create serverCreateModel
	ssh    serverSSHModel

	// overlay manages concurrent start/stop operations and renders a
	// floating panel in the bottom-right corner of the screen.
	overlay opsOverlay

	// prefsSvc provides per-server user preference persistence.
	prefsSvc *prefssvc.Service

	// Action state (appViewAction).
	actionSpinner spinner.Model
	actionLabel   string
	actionStatus  string
	actionIsError bool

	width  int
	height int
}

// AppResult holds the outcome of the server app TUI session.
type AppResult struct {
	// CreatedServer is non-nil if a server was created during the session.
	// The CLI layer can use this to print the root password, etc.
	CreatedServer *domain.Server
}

// RunServerApp starts the unified server management TUI. It stays open
// until the user explicitly quits from the list view.
func RunServerApp(provider domain.Provider, providerName string) (*AppResult, error) {
	as := spinner.New()
	as.Spinner = spinner.Dot
	as.Style = lipgloss.NewStyle().Foreground(styles.Blue)

	overlay, overlayInitCmd := newOpsOverlay(provider, providerName)

	// Open preferences database (best-effort, continue if unavailable).
	var prefsSvc *prefssvc.Service
	if repo, err := serverprefs.Open(); err == nil {
		prefsSvc = prefssvc.NewService(repo)
	}

	m := serverAppModel{
		provider:      provider,
		providerName:  providerName,
		view:          appViewList,
		list:          newServerListModel(provider, providerName),
		overlay:       overlay,
		prefsSvc:      prefsSvc,
		actionSpinner: as,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())

	// Send overlay initialization command if available (loads pending actions).
	if overlayInitCmd != nil {
		go func() {
			p.Send(overlayInitCmd())
		}()
	}

	result, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to run server app: %w", err)
	}

	final := result.(serverAppModel)

	// Close database connections on exit.
	if final.overlay.svc != nil {
		final.overlay.svc.Close()
	}
	if final.prefsSvc != nil {
		final.prefsSvc.Close()
	}

	return &AppResult{}, nil
}

func (m serverAppModel) Init() tea.Cmd {
	return m.list.Init()
}

func (m serverAppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Propagate to the active child.
		return m.updateChild(msg)

	// --- Navigation messages ---

	case navigateToListMsg:
		return m.switchToList()

	case navigateToShowMsg:
		return m.switchToShow(msg.server)

	case navigateToDeleteMsg:
		return m.switchToDelete(msg.server)

	case navigateToCreateMsg:
		return m.switchToCreate()

	case navigateToSSHMsg:
		return m.switchToSSH(msg.server)

	case navigateBackMsg:
		return m.switchToList()

	// --- Action messages ---

	case deleteConfirmedMsg:
		return m.startDeleteAction(msg.server)

	case createConfirmedMsg:
		return m.startCreateAction(msg.opts)

	// --- Action results ---

	case deleteResultMsg:
		return m.handleDeleteResult(msg)

	case createResultMsg:
		return m.handleCreateResult(msg)

	// --- Toggle overlay ---

	case requestToggleMsg:
		var cmd tea.Cmd
		m.overlay, cmd = m.overlay.StartToggle(msg.server)
		return m, cmd

	case opToggleInitiatedMsg, opToggleErrorMsg, opPollTickMsg,
		opPollResultMsg, opPollErrorMsg, opDismissMsg:
		return m.updateOverlay(msg)

	// --- SSH exec ---

	case requestSSHMsg:
		return m.handleSSHRequest(msg)

	case sshFinishedMsg:
		return m.handleSSHFinished(msg)

	case clearHostKeyMsg:
		return m.handleClearHostKey(msg)

	// --- Spinner ticks ---
	// Forward to both the overlay and the active child so both
	// spinners animate.
	case spinner.TickMsg:
		return m.updateSpinnerTick(msg)
	}

	return m.updateChild(msg)
}

// updateOverlay delegates a message to the overlay and processes any
// completed operations (e.g. refreshing the active child view).
func (m serverAppModel) updateOverlay(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var outcomes []opCompletedEvent
	m.overlay, cmd, outcomes = m.overlay.Update(msg)

	var cmds []tea.Cmd
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	for _, ev := range outcomes {
		if ev.Success {
			switch m.view {
			case appViewList:
				// Set loading state before triggering refresh to ensure
				// footer renders correctly during the transition.
				m.list.loading = true
				m.list.err = nil
				m.list.status = "" // Clear any previous status message
				cmds = append(cmds, tea.Batch(m.list.spinner.Tick, m.list.fetchServers()))
			case appViewShow:
				if m.show.server != nil {
					m.show.serverID = m.show.server.ID
					m.show.loading = true
					m.show.err = nil
					m.show.status = "" // Clear any previous status message
					cmds = append(cmds, tea.Batch(m.show.spinner.Tick, m.show.fetchServer()))
				}
			}
		}
	}

	return m, tea.Batch(cmds...)
}

// updateSpinnerTick forwards spinner ticks to both the overlay and the
// active child model so all spinners animate correctly.
func (m serverAppModel) updateSpinnerTick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Forward to overlay.
	var overlayCmd tea.Cmd
	m.overlay, overlayCmd, _ = m.overlay.Update(msg)
	if overlayCmd != nil {
		cmds = append(cmds, overlayCmd)
	}

	// Forward to active child.
	childModel, childCmd := m.updateChildDirect(msg)
	m = childModel
	if childCmd != nil {
		cmds = append(cmds, childCmd)
	}

	return m, tea.Batch(cmds...)
}

// updateChildDirect delegates a message to the active child and returns
// the updated app model. Unlike updateChild it returns the concrete type
// to avoid a double type assertion in callers.
func (m serverAppModel) updateChildDirect(msg tea.Msg) (serverAppModel, tea.Cmd) {
	switch m.view {
	case appViewList:
		updated, cmd := m.list.Update(msg)
		m.list = updated.(serverListModel)
		return m, cmd
	case appViewShow:
		updated, cmd := m.show.Update(msg)
		m.show = updated.(serverShowModel)
		return m, cmd
	case appViewDelete:
		updated, cmd := m.delete.Update(msg)
		m.delete = updated.(serverDeleteModel)
		return m, cmd
	case appViewCreate:
		updated, cmd := m.create.Update(msg)
		m.create = updated.(serverCreateModel)
		return m, cmd
	case appViewSSH:
		updated, cmd := m.ssh.Update(msg)
		m.ssh = updated.(serverSSHModel)
		return m, cmd
	case appViewAction:
		return m.updateActionDirect(msg)
	}
	return m, nil
}

// updateActionDirect handles messages for the action view and returns
// the concrete serverAppModel type.
func (m serverAppModel) updateActionDirect(msg tea.Msg) (serverAppModel, tea.Cmd) {
	result, cmd := m.updateAction(msg)
	return result.(serverAppModel), cmd
}

func (m serverAppModel) View() string {
	var view string
	switch m.view {
	case appViewList:
		view = m.list.View()
	case appViewShow:
		view = m.show.View()
	case appViewDelete:
		view = m.delete.View()
	case appViewCreate:
		view = m.create.View()
	case appViewSSH:
		view = m.ssh.View()
	case appViewAction:
		view = m.renderAction()
	}

	// Composite the operations overlay on top of the child view.
	if m.overlay.HasAny() {
		overlayStr := m.overlay.View(m.width, m.height)
		view = composeOverlay(view, overlayStr, m.width, m.height)
	}

	// Pad the view to exactly m.height lines so Bubbletea's alt screen
	// renderer always repaints the full terminal. Without this, dismissing
	// the overlay (which previously padded the output) leaves ghost lines
	// from the prior frame.
	view = padToHeight(view, m.width, m.height)

	return view
}

// padToHeight ensures the view string has exactly `height` lines by
// appending blank lines if necessary. This prevents ghost rendering
// artifacts when the terminal's alt screen buffer retains content from
// previous frames.
func padToHeight(view string, width, height int) string {
	if height <= 0 {
		return view
	}
	lines := strings.Split(view, "\n")
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return strings.Join(lines, "\n")
}

// --- View transitions ---

func (m serverAppModel) switchToList() (tea.Model, tea.Cmd) {
	m.view = appViewList
	m.list = newServerListModel(m.provider, m.providerName)
	m.list.width = m.width
	m.list.height = m.height
	return m, m.list.Init()
}

func (m serverAppModel) switchToShow(server domain.Server) (tea.Model, tea.Cmd) {
	m.view = appViewShow
	m.show = newServerShowDirect(m.provider, m.providerName, &server)
	m.show.width = m.width
	m.show.height = m.height
	return m, m.show.Init()
}

func (m serverAppModel) switchToDelete(server domain.Server) (tea.Model, tea.Cmd) {
	m.view = appViewDelete
	m.delete = newServerDeleteModel(m.provider, m.providerName, &server)
	m.delete.width = m.width
	m.delete.height = m.height
	return m, m.delete.Init()
}

func (m serverAppModel) switchToCreate() (tea.Model, tea.Cmd) {
	catalogProvider, ok := m.provider.(domain.CatalogProvider)
	if !ok {
		// Provider doesn't support catalog — go back to list.
		m.view = appViewList
		m.list.status = "Interactive server creation is not supported for this provider."
		m.list.statusIsError = true
		return m, nil
	}

	m.view = appViewCreate
	m.create = newServerCreateModel(catalogProvider, m.providerName, domain.CreateServerOpts{})
	m.create.width = m.width
	m.create.height = m.height
	return m, m.create.Init()
}

func (m serverAppModel) switchToSSH(server domain.Server) (tea.Model, tea.Cmd) {
	// Resolve IP address (IPv4 preferred, IPv6 fallback).
	ipAddress := server.PublicIPv4
	if ipAddress == "" {
		ipAddress = server.PublicIPv6
	}
	if ipAddress == "" {
		// No IP available — return to show with error.
		m.view = appViewShow
		m.show.status = "No public IP address available for SSH"
		m.show.statusIsError = true
		return m, nil
	}

	// Load saved username if available.
	var defaultUsername string
	if m.prefsSvc != nil {
		defaultUsername = m.prefsSvc.GetSSHUser(m.providerName, server.ID)
	}

	m.view = appViewSSH
	m.ssh = newServerSSHModel(&server, m.providerName, ipAddress, defaultUsername)
	m.ssh.width = m.width
	m.ssh.height = m.height
	return m, m.ssh.Init()
}

// --- API actions ---

func (m serverAppModel) startDeleteAction(server domain.Server) (tea.Model, tea.Cmd) {
	m.view = appViewAction
	m.actionLabel = fmt.Sprintf("Deleting server %q...", server.Name)
	m.actionStatus = ""
	m.actionIsError = false

	provider := m.provider
	return m, tea.Batch(m.actionSpinner.Tick, func() tea.Msg {
		err := provider.DeleteServer(context.Background(), server.ID)
		return deleteResultMsg{server: server, err: err}
	})
}

func (m serverAppModel) startCreateAction(opts domain.CreateServerOpts) (tea.Model, tea.Cmd) {
	m.view = appViewAction
	m.actionLabel = fmt.Sprintf("Creating server %q...", opts.Name)
	m.actionStatus = ""
	m.actionIsError = false

	provider := m.provider
	return m, tea.Batch(m.actionSpinner.Tick, func() tea.Msg {
		server, err := provider.CreateServer(context.Background(), opts)
		return createResultMsg{server: server, err: err}
	})
}

func (m serverAppModel) handleDeleteResult(msg deleteResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		// Show error, then return to list on any key.
		m.actionLabel = ""
		m.actionStatus = fmt.Sprintf("Error deleting server %q: %v", msg.server.Name, msg.err)
		m.actionIsError = true
		return m, nil
	}

	// Go straight back to the list with a success status.
	m.view = appViewList
	m.list = newServerListModel(m.provider, m.providerName)
	m.list.width = m.width
	m.list.height = m.height
	m.list.persistentStatus = fmt.Sprintf("Server %q deleted successfully", msg.server.Name)
	return m, m.list.Init()
}

func (m serverAppModel) handleCreateResult(msg createResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.actionLabel = ""
		m.actionStatus = fmt.Sprintf("Error creating server: %v", msg.err)
		m.actionIsError = true
		return m, nil
	}

	// Go back to the list with a success status.
	m.view = appViewList
	m.list = newServerListModel(m.provider, m.providerName)
	m.list.width = m.width
	m.list.height = m.height

	name := "server"
	if msg.server != nil {
		name = fmt.Sprintf("%q", msg.server.Name)
	}
	m.list.persistentStatus = fmt.Sprintf("Server %s created successfully", name)
	return m, m.list.Init()
}

// --- SSH handlers ---

func (m serverAppModel) handleSSHRequest(msg requestSSHMsg) (tea.Model, tea.Cmd) {
	// Persist username for this server.
	if m.prefsSvc != nil {
		m.prefsSvc.SetSSHUser(m.providerName, msg.server.ID, msg.username)
	}

	// Build SSH command with secure options.
	sshCmd := exec.Command("ssh",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=10",
		"-o", "ServerAliveInterval=60",
		"-o", "ServerAliveCountMax=3",
		fmt.Sprintf("%s@%s", msg.username, msg.ipAddress),
	)
	sshCmd.Stdin = os.Stdin
	sshCmd.Stdout = os.Stdout

	// Capture stderr for error detection while also showing it to the user.
	var stderrBuf bytes.Buffer
	sshCmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	// Capture context for navigation and error handling after SSH exits.
	server := msg.server
	username := msg.username
	ipAddress := msg.ipAddress

	return m, tea.ExecProcess(sshCmd, func(err error) tea.Msg {
		if err == nil {
			// SSH succeeded — no error to report.
			return sshFinishedMsg{
				server:    server,
				username:  username,
				ipAddress: ipAddress,
				err:       nil,
				errKind:   sshErrNone,
			}
		}

		// SSH failed — inspect stderr to categorize the failure.
		stderrOutput := stderrBuf.String()
		errKind := sshErrGeneric
		errDetail := "SSH connection failed"

		if strings.Contains(stderrOutput, "REMOTE HOST IDENTIFICATION HAS CHANGED") {
			errKind = sshErrHostKeyConflict
			errDetail = "Host key has changed (IP may have been reused by a new server)"
		} else if strings.Contains(stderrOutput, "Connection refused") {
			errDetail = "Connection refused (server may not be running SSH)"
		} else if strings.Contains(stderrOutput, "Connection timed out") || strings.Contains(stderrOutput, "No route to host") {
			errDetail = "Connection timed out (network issue or firewall blocking access)"
		} else if strings.Contains(stderrOutput, "Permission denied") {
			errDetail = "Permission denied (check username and SSH keys)"
		}

		return sshFinishedMsg{
			server:    server,
			username:  username,
			ipAddress: ipAddress,
			err:       err,
			errKind:   errKind,
			errDetail: errDetail,
		}
	})
}

func (m serverAppModel) handleSSHFinished(msg sshFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.err == nil {
		// SSH succeeded — navigate back to show view with refresh.
		m.view = appViewShow
		m.show = newServerShowDirect(m.provider, m.providerName, &msg.server)
		m.show.width = m.width
		m.show.height = m.height
		m.show.loading = true
		m.show.serverID = msg.server.ID
		m.show.err = nil
		return m, tea.Batch(m.show.spinner.Tick, m.show.fetchServer())
	}

	// SSH failed — branch on error kind.
	switch msg.errKind {
	case sshErrHostKeyConflict:
		// Host key conflict — return to SSH view with error + retry option.
		m.view = appViewSSH
		m.ssh = newServerSSHModelWithError(
			&msg.server,
			m.providerName,
			msg.ipAddress,
			msg.username,
			msg.errDetail,
			true, // hostKeyConflict
		)
		m.ssh.width = m.width
		m.ssh.height = m.height
		return m, m.ssh.Init()

	default:
		// Generic SSH error — navigate to show view with persistent error status.
		m.view = appViewShow
		m.show = newServerShowDirect(m.provider, m.providerName, &msg.server)
		m.show.width = m.width
		m.show.height = m.height
		m.show.persistentStatus = msg.errDetail
		m.show.statusIsError = true
		m.show.loading = true
		m.show.serverID = msg.server.ID
		m.show.err = nil
		return m, tea.Batch(m.show.spinner.Tick, m.show.fetchServer())
	}
}

func (m serverAppModel) handleClearHostKey(msg clearHostKeyMsg) (tea.Model, tea.Cmd) {
	// Remove the stale SSH host key for this IP address.
	cmd := exec.Command("ssh-keygen", "-R", msg.ipAddress)
	// Run synchronously (fast operation).
	if err := cmd.Run(); err != nil {
		// If ssh-keygen fails, show error and return to SSH view without retry.
		m.view = appViewSSH
		m.ssh = newServerSSHModelWithError(
			&msg.server,
			m.providerName,
			msg.ipAddress,
			msg.username,
			fmt.Sprintf("Failed to clear host key: %v", err),
			false, // not a host key conflict anymore, just an error
		)
		m.ssh.width = m.width
		m.ssh.height = m.height
		return m, m.ssh.Init()
	}

	// Host key cleared — immediately retry SSH connection.
	return m, func() tea.Msg {
		return requestSSHMsg{
			server:    msg.server,
			username:  msg.username,
			ipAddress: msg.ipAddress,
		}
	}
}

// --- Delegate to active child ---

func (m serverAppModel) updateChild(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.view {
	case appViewList:
		updated, cmd := m.list.Update(msg)
		m.list = updated.(serverListModel)
		return m, cmd

	case appViewShow:
		updated, cmd := m.show.Update(msg)
		m.show = updated.(serverShowModel)
		return m, cmd

	case appViewDelete:
		updated, cmd := m.delete.Update(msg)
		m.delete = updated.(serverDeleteModel)
		return m, cmd

	case appViewCreate:
		updated, cmd := m.create.Update(msg)
		m.create = updated.(serverCreateModel)
		return m, cmd

	case appViewSSH:
		updated, cmd := m.ssh.Update(msg)
		m.ssh = updated.(serverSSHModel)
		return m, cmd

	case appViewAction:
		return m.updateAction(msg)
	}

	return m, nil
}

// --- Action view ---

func (m serverAppModel) updateAction(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// While performing the action (spinner), only allow ctrl+c.
		if m.actionStatus == "" {
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}
		// After an error, any key returns to the list.
		return m.switchToList()

	case spinner.TickMsg:
		if m.actionStatus == "" {
			var cmd tea.Cmd
			m.actionSpinner, cmd = m.actionSpinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	return m, nil
}

func (m serverAppModel) renderAction() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	header := components.Header(m.width, "server", m.providerName)
	footer := components.Footer(m.width, []components.KeyBinding{
		{Key: "ctrl+c", Desc: "quit"},
	})

	headerH := lipgloss.Height(header)
	footerH := lipgloss.Height(footer)
	contentH := max(m.height-headerH-footerH, 1)

	var content string
	if m.actionStatus != "" {
		// Error or result display.
		style := styles.ErrorText
		if !m.actionIsError {
			style = styles.SuccessText
		}
		text := style.Render(m.actionStatus) + "\n\n" +
			styles.MutedText.Render("Press any key to continue.")
		content = lipgloss.Place(m.width, contentH, lipgloss.Center, lipgloss.Center, text)
	} else {
		// Spinner while performing action.
		text := m.actionSpinner.View() + "  " + m.actionLabel
		content = lipgloss.Place(m.width, contentH, lipgloss.Center, lipgloss.Center,
			styles.MutedText.Render(text))
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
}

// --- Child model constructors (for use by the app model) ---

func newServerListModel(provider domain.Provider, providerName string) serverListModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(styles.Blue)

	return serverListModel{
		provider:     provider,
		providerName: providerName,
		loading:      true,
		spinner:      s,
		embedded:     true,
	}
}

func newServerShowDirect(provider domain.Provider, providerName string, server *domain.Server) serverShowModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(styles.Blue)

	vp := viewport.New(0, 0)
	vp.KeyMap = detailViewportKeyMap()

	return serverShowModel{
		provider:       provider,
		providerName:   providerName,
		phase:          showPhaseDetail,
		server:         server,
		serverID:       server.ID,
		loading:        false,
		metricsLoading: true,
		spinner:        s,
		embedded:       true,
		viewport:       vp,
	}
}

func newServerDeleteModel(provider domain.Provider, providerName string, server *domain.Server) serverDeleteModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(styles.Blue)

	m := serverDeleteModel{
		provider:     provider,
		providerName: providerName,
		spinner:      s,
		embedded:     true,
	}

	if server != nil {
		m.phase = deletePhaseConfirm
		m.server = server
		m.loading = false
		m.confirmIdx = 1 // default to cancel for safety
	} else {
		m.phase = deletePhaseSelect
		m.loading = true
	}

	return m
}

func newServerCreateModel(provider domain.CatalogProvider, providerName string, prefill domain.CreateServerOpts) serverCreateModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(styles.Blue)

	ti := newCreateTextInput(prefill.Name)

	return serverCreateModel{
		provider:     provider,
		providerName: providerName,
		prefill:      prefill,
		step:         stepLoading,
		opts:         prefill,
		nameInput:    ti,
		loading:      true,
		spinner:      s,
		sshSelected:  make(map[int]struct{}),
		embedded:     true,
	}
}

func newServerSSHModel(server *domain.Server, providerName string, ipAddress string, defaultUsername string) serverSSHModel {
	ti := textinput.New()
	ti.Placeholder = "root"
	ti.Focus()
	ti.CharLimit = 64
	ti.Width = 40

	if defaultUsername != "" {
		ti.SetValue(defaultUsername)
	}

	return serverSSHModel{
		server:        server,
		providerName:  providerName,
		ipAddress:     ipAddress,
		usernameInput: ti,
		embedded:      true,
	}
}

func newServerSSHModelWithError(server *domain.Server, providerName string, ipAddress string, defaultUsername string, errorMsg string, hostKeyConflict bool) serverSSHModel {
	ti := textinput.New()
	ti.Placeholder = "root"
	ti.Focus()
	ti.CharLimit = 64
	ti.Width = 40

	if defaultUsername != "" {
		ti.SetValue(defaultUsername)
	}

	return serverSSHModel{
		server:          server,
		providerName:    providerName,
		ipAddress:       ipAddress,
		usernameInput:   ti,
		errorMsg:        errorMsg,
		hostKeyConflict: hostKeyConflict,
		embedded:        true,
	}
}

func newCreateTextInput(prefillName string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "my-server"
	ti.Focus()
	ti.CharLimit = 63
	ti.Width = 40

	if prefillName != "" {
		ti.SetValue(prefillName)
	}

	return ti
}
