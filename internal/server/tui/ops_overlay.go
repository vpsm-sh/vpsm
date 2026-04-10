package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nathanbeddoewebdev/vpsm/internal/actionstore"
	"nathanbeddoewebdev/vpsm/internal/server/domain"
	"nathanbeddoewebdev/vpsm/internal/server/services/action"
	"nathanbeddoewebdev/vpsm/internal/tui/styles"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// --- Overlay configuration ---

const (
	// overlayPollInterval is the delay between successive poll requests
	// for each operation.
	overlayPollInterval = 3 * time.Second

	// overlayMaxPollAttempts caps how many times we poll a single
	// operation before giving up (~5 minutes at 3 s intervals).
	overlayMaxPollAttempts = 100

	// overlayMaxTransientErrors is the number of consecutive non-rate-limit
	// errors tolerated per operation before giving up.
	overlayMaxTransientErrors = 3

	// overlayDismissDelay is how long completed/failed items persist in the
	// overlay before being auto-removed.
	overlayDismissDelay = 5 * time.Second

	// overlayMinWidth is the minimum card width.
	overlayMinWidth = 30

	// overlayMaxWidth is the maximum card width.
	overlayMaxWidth = 38
)

// --- Operation status ---

const (
	opStatusActive    = "active"
	opStatusSucceeded = "succeeded"
	opStatusFailed    = "failed"
)

// --- Poll mode ---

const (
	opPollModeAction = "action"
	opPollModeServer = "server"
)

// --- Messages ---

// requestToggleMsg is emitted by child models (server list, server show)
// when the user presses "s" to start/stop a server. The serverAppModel
// intercepts it and delegates to the overlay.
type requestToggleMsg struct {
	server domain.Server
}

// All overlay messages carry an opID so the overlay can route them to
// the correct in-flight operation. Stale messages for already-dismissed
// operations are silently dropped.

type opToggleInitiatedMsg struct {
	opID       int
	serverID   string
	serverName string
	verb       string // "started" or "stopped"
	target     string // target server status (e.g. "running", "off")
	action     *domain.ActionStatus
}

type opToggleErrorMsg struct {
	opID int
	err  error
}

type opPollTickMsg struct {
	opID int
}

type opPollResultMsg struct {
	opID   int
	action *domain.ActionStatus
}

type opPollErrorMsg struct {
	opID int
	err  error
}

type opDismissMsg struct {
	opID int
}

// opCompletedEvent is returned to the parent model via the outcomes
// slice so it can take action (e.g. refresh server list). It is not a
// tea.Msg — it is returned synchronously from Update.
type opCompletedEvent struct {
	Success    bool
	ServerName string
	Verb       string // "started" or "stopped"
	ErrText    string
}

// --- Operation ---

// operation tracks a single in-flight start/stop polling cycle.
type operation struct {
	id         int
	dbID       int64  // database record ID (0 if not persisted)
	provider   string // provider name for database persistence
	serverID   string
	serverName string
	verb       string // "started" or "stopped"
	target     string // target server status

	pollMode  string // "action" or "server"
	actionID  string
	pollCount int

	consecutiveErrors int

	status     string // opStatusActive, opStatusSucceeded, opStatusFailed
	statusText string
	progress   int
}

// --- Ops overlay ---

// opsOverlay manages a list of concurrent start/stop operations and
// renders a floating overlay panel in the bottom-right of the screen.
//
// It is a value type — methods return a new copy plus any tea.Cmd to
// execute.
type opsOverlay struct {
	provider     domain.Provider
	providerName string
	ops          []operation
	nextID       int
	spinner      spinner.Model
	svc          *action.Service // persistence service (may be nil if DB unavailable)
}

// newOpsOverlay creates an overlay bound to the given provider and loads
// any pending actions from the database. Returns the overlay and an optional
// initialization command.
func newOpsOverlay(provider domain.Provider, providerName string) (opsOverlay, tea.Cmd) {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(styles.Blue)

	// Open database connection (best-effort, continue if unavailable).
	repo, err := actionstore.Open()
	if err != nil {
		// Log error but continue without persistence.
		repo = nil
	}

	svc := action.NewService(provider, providerName, repo)

	o := opsOverlay{
		provider:     provider,
		providerName: providerName,
		spinner:      s,
		svc:          svc,
	}

	// Load pending actions from database.
	cmd := o.loadPendingActions()

	return o, cmd
}

// HasActive reports whether any operations are still polling.
func (o opsOverlay) HasActive() bool {
	for _, op := range o.ops {
		if op.status == opStatusActive {
			return true
		}
	}
	return false
}

// HasAny reports whether any operations exist (active or completed
// awaiting dismiss).
func (o opsOverlay) HasAny() bool {
	return len(o.ops) > 0
}

// --- Database persistence ---

// loadPendingActions retrieves pending actions from the database and
// reconstructs in-memory operations. Returns a tea.Cmd that immediately
// polls each loaded action to check current status. Only loads actions
// that are less than 5 minutes old to avoid showing stale operations.
func (o *opsOverlay) loadPendingActions() tea.Cmd {
	if o.svc == nil {
		return nil
	}

	records, err := o.svc.ListPending()
	if err != nil || len(records) == 0 {
		return nil
	}

	var cmds []tea.Cmd
	now := time.Now()

	for _, record := range records {
		// Filter to current provider only.
		if record.Provider != o.providerName {
			continue
		}

		// Skip stale operations (older than 5 minutes).
		if now.Sub(record.UpdatedAt) > 5*time.Minute {
			continue
		}

		// Reconstruct operation from database record.
		opID := o.nextID
		o.nextID++

		verb := "started"
		if record.Command == "stop_server" {
			verb = "stopped"
		}

		op := operation{
			id:         opID,
			dbID:       record.ID,
			provider:   record.Provider,
			serverID:   record.ServerID,
			serverName: record.ServerName,
			verb:       verb,
			target:     record.TargetStatus,
			pollMode:   opPollModeAction, // Prefer action polling
			actionID:   record.ActionID,
			status:     opStatusActive,
			statusText: fmt.Sprintf("Resuming %q...", record.ServerName),
			progress:   record.Progress,
		}

		o.ops = append(o.ops, op)

		// Immediately poll to get current status.
		cmds = append(cmds, scheduleOpPollTick(op.id))
	}

	if len(cmds) > 0 {
		return tea.Batch(cmds...)
	}
	return nil
}

// saveOp persists an operation to the database. Errors are logged but
// don't fail the operation (TUI should continue if DB is unavailable).
func (o *opsOverlay) saveOp(op operation) {
	if o.svc == nil {
		return
	}

	record := &actionstore.ActionRecord{
		ID:           op.dbID, // 0 for new, or existing DB ID
		ActionID:     op.actionID,
		Provider:     o.providerName,
		ServerID:     op.serverID,
		ServerName:   op.serverName,
		Command:      inferCommand(op.verb),
		TargetStatus: op.target,
		Status:       mapOpStatusToDomain(op.status),
		Progress:     op.progress,
	}

	if err := o.svc.SaveRecord(record); err == nil {
		// Update the operation's DB ID if this was an insert.
		if op.dbID == 0 {
			op.dbID = record.ID
			// Find and update the operation in the slice.
			for i := range o.ops {
				if o.ops[i].id == op.id {
					o.ops[i].dbID = op.dbID
					break
				}
			}
		}
	}
}

// inferCommand converts a verb to a command name for database storage.
func inferCommand(verb string) string {
	if verb == "started" {
		return "start_server"
	}
	return "stop_server"
}

// mapOpStatusToDomain converts overlay status to domain status.
func mapOpStatusToDomain(opStatus string) string {
	switch opStatus {
	case opStatusActive:
		return domain.ActionStatusRunning
	case opStatusSucceeded:
		return domain.ActionStatusSuccess
	case opStatusFailed:
		return domain.ActionStatusError
	default:
		return domain.ActionStatusRunning
	}
}

// --- Commands ---

// StartToggle creates a new operation and fires the initial
// StartServer/StopServer API call. Returns the updated overlay and
// a tea.Cmd.
func (o opsOverlay) StartToggle(server domain.Server) (opsOverlay, tea.Cmd) {
	opID := o.nextID
	o.nextID++

	var verb, target string
	switch server.Status {
	case "running":
		verb = "stopped"
		target = "off"
	case "off", "stopped":
		verb = "started"
		target = "running"
	default:
		// Cannot toggle — return without creating an operation.
		return o, nil
	}

	op := operation{
		id:         opID,
		provider:   o.providerName,
		serverID:   server.ID,
		serverName: server.Name,
		verb:       verb,
		target:     target,
		status:     opStatusActive,
		statusText: fmt.Sprintf("%s %q...", verbToGerund(verb), server.Name),
	}
	o.ops = append(o.ops, op)

	// Persist initial operation state to database.
	o.saveOp(op)

	provider := o.provider
	cmd := func() tea.Msg {
		ctx := context.Background()
		switch server.Status {
		case "running":
			action, err := provider.StopServer(ctx, server.ID)
			if err != nil {
				return opToggleErrorMsg{opID: opID, err: fmt.Errorf("failed to stop server %q: %w", server.Name, err)}
			}
			return opToggleInitiatedMsg{
				opID:       opID,
				serverID:   server.ID,
				serverName: server.Name,
				verb:       "stopped",
				target:     "off",
				action:     action,
			}
		default:
			action, err := provider.StartServer(ctx, server.ID)
			if err != nil {
				return opToggleErrorMsg{opID: opID, err: fmt.Errorf("failed to start server %q: %w", server.Name, err)}
			}
			return opToggleInitiatedMsg{
				opID:       opID,
				serverID:   server.ID,
				serverName: server.Name,
				verb:       "started",
				target:     "running",
				action:     action,
			}
		}
	}

	return o, tea.Batch(o.spinner.Tick, cmd)
}

// --- Update ---

// Update processes overlay-related messages and returns the updated
// overlay, a tea.Cmd, and a slice of completed outcomes for the parent
// model to act on (e.g. refresh server list).
func (o opsOverlay) Update(msg tea.Msg) (opsOverlay, tea.Cmd, []opCompletedEvent) {
	switch msg := msg.(type) {
	case opToggleInitiatedMsg:
		return o.handleInitiated(msg)
	case opToggleErrorMsg:
		return o.handleToggleError(msg)
	case opPollTickMsg:
		return o.handlePollTick(msg)
	case opPollResultMsg:
		return o.handlePollResult(msg)
	case opPollErrorMsg:
		return o.handlePollError(msg)
	case opDismissMsg:
		return o.handleDismiss(msg)
	case spinner.TickMsg:
		if o.HasActive() {
			var cmd tea.Cmd
			o.spinner, cmd = o.spinner.Update(msg)
			return o, cmd, nil
		}
		return o, nil, nil
	}
	return o, nil, nil
}

// --- Internal handlers ---

func (o opsOverlay) handleInitiated(msg opToggleInitiatedMsg) (opsOverlay, tea.Cmd, []opCompletedEvent) {
	idx := o.findOp(msg.opID)
	if idx < 0 {
		return o, nil, nil
	}
	op := o.ops[idx]
	action := msg.action

	// Update action ID if available.
	if action != nil && action.ID != "" {
		op.actionID = action.ID
	}

	// Fast path: action completed synchronously — verify server status.
	if action != nil && action.Status == domain.ActionStatusSuccess {
		op.pollMode = opPollModeServer
		op.statusText = fmt.Sprintf("Verifying %q...", op.serverName)
		o.ops[idx] = op
		o.saveOp(op)
		return o, scheduleOpPollTick(op.id), nil
	}

	// Fast path: action failed immediately.
	if action != nil && action.Status == domain.ActionStatusError {
		errMsg := "action failed"
		if action.ErrorMessage != "" {
			errMsg = action.ErrorMessage
		}
		op.status = opStatusFailed
		op.statusText = fmt.Sprintf("Failed: %s", errMsg)
		o.ops[idx] = op
		o.saveOp(op)
		return o, scheduleDismiss(op.id), []opCompletedEvent{{
			ErrText: fmt.Sprintf("Failed to %s server %q: %s", verbToInfinitive(op.verb), op.serverName, errMsg),
		}}
	}

	// Action is still running — choose polling strategy.
	if _, ok := o.provider.(domain.ActionPoller); ok && action != nil && action.ID != "" {
		op.pollMode = opPollModeAction
		op.actionID = action.ID
	} else {
		op.pollMode = opPollModeServer
	}
	op.statusText = fmt.Sprintf("%s %q...", verbToGerund(op.verb), op.serverName)
	o.ops[idx] = op
	o.saveOp(op)
	return o, scheduleOpPollTick(op.id), nil
}

func (o opsOverlay) handleToggleError(msg opToggleErrorMsg) (opsOverlay, tea.Cmd, []opCompletedEvent) {
	idx := o.findOp(msg.opID)
	if idx < 0 {
		return o, nil, nil
	}
	op := o.ops[idx]
	op.status = opStatusFailed
	op.statusText = "Failed: " + msg.err.Error()
	o.ops[idx] = op
	o.saveOp(op)
	return o, scheduleDismiss(op.id), []opCompletedEvent{{
		ErrText: msg.err.Error(),
	}}
}

func (o opsOverlay) handlePollTick(msg opPollTickMsg) (opsOverlay, tea.Cmd, []opCompletedEvent) {
	idx := o.findOp(msg.opID)
	if idx < 0 {
		return o, nil, nil // stale tick for dismissed op
	}
	op := o.ops[idx]
	if op.status != opStatusActive {
		return o, nil, nil
	}
	return o, o.doPoll(op), nil
}

func (o opsOverlay) handlePollResult(msg opPollResultMsg) (opsOverlay, tea.Cmd, []opCompletedEvent) {
	idx := o.findOp(msg.opID)
	if idx < 0 {
		return o, nil, nil
	}
	op := o.ops[idx]
	op.consecutiveErrors = 0
	status := msg.action

	switch status.Status {
	case domain.ActionStatusSuccess:
		if op.pollMode == opPollModeAction {
			// Action completed — verify server reached target status.
			op.pollMode = opPollModeServer
			op.consecutiveErrors = 0
			op.statusText = fmt.Sprintf("Verifying %q...", op.serverName)
			o.ops[idx] = op
			o.saveOp(op)
			return o, scheduleOpPollTick(op.id), nil
		}
		// Server reached target status — success.
		op.status = opStatusSucceeded
		op.statusText = fmt.Sprintf("%q %s", op.serverName, op.verb)
		op.progress = 100
		o.ops[idx] = op
		o.saveOp(op)
		return o, scheduleDismiss(op.id), []opCompletedEvent{{
			Success:    true,
			ServerName: op.serverName,
			Verb:       op.verb,
		}}

	case domain.ActionStatusError:
		errMsg := "action failed"
		if status.ErrorMessage != "" {
			errMsg = status.ErrorMessage
		}
		op.status = opStatusFailed
		op.statusText = fmt.Sprintf("Failed: %s", errMsg)
		o.ops[idx] = op
		o.saveOp(op)
		return o, scheduleDismiss(op.id), []opCompletedEvent{{
			ErrText: fmt.Sprintf("Failed to %s server %q: %s", verbToInfinitive(op.verb), op.serverName, errMsg),
		}}

	default:
		// Still running — update progress.
		op.pollCount++
		if op.pollCount >= overlayMaxPollAttempts {
			op.status = opStatusFailed
			op.statusText = fmt.Sprintf("Timed out %s %q", verbToGerund(op.verb), op.serverName)
			o.ops[idx] = op
			o.saveOp(op)
			return o, scheduleDismiss(op.id), []opCompletedEvent{{
				ErrText: fmt.Sprintf("Timed out waiting for server %q to %s", op.serverName, verbToInfinitive(op.verb)),
			}}
		}

		if status.Progress > 0 {
			op.progress = status.Progress
			op.statusText = fmt.Sprintf("%s %q (%d%%)", verbToGerund(op.verb), op.serverName, status.Progress)
		} else {
			op.statusText = fmt.Sprintf("%s %q...", verbToGerund(op.verb), op.serverName)
		}
		o.ops[idx] = op
		o.saveOp(op)
		return o, scheduleOpPollTick(op.id), nil
	}
}

func (o opsOverlay) handlePollError(msg opPollErrorMsg) (opsOverlay, tea.Cmd, []opCompletedEvent) {
	idx := o.findOp(msg.opID)
	if idx < 0 {
		return o, nil, nil
	}
	op := o.ops[idx]

	// Rate-limit errors abort immediately.
	if isRateLimited(msg.err) {
		op.status = opStatusFailed
		op.statusText = "Rate limited"
		o.ops[idx] = op
		o.saveOp(op)
		return o, scheduleDismiss(op.id), []opCompletedEvent{{
			ErrText: "Polling stopped (rate limited)",
		}}
	}

	op.consecutiveErrors++
	if op.consecutiveErrors >= overlayMaxTransientErrors {
		op.status = opStatusFailed
		op.statusText = fmt.Sprintf("Failed after %d errors", op.consecutiveErrors)
		o.ops[idx] = op
		o.saveOp(op)
		return o, scheduleDismiss(op.id), []opCompletedEvent{{
			ErrText: fmt.Sprintf("Error polling (after %d consecutive failures): %v", op.consecutiveErrors, msg.err),
		}}
	}

	// Transient error within budget — schedule another poll.
	op.statusText = fmt.Sprintf("Retrying... (%d/%d)", op.consecutiveErrors, overlayMaxTransientErrors)
	o.ops[idx] = op
	o.saveOp(op)
	return o, scheduleOpPollTick(op.id), nil
}

func (o opsOverlay) handleDismiss(msg opDismissMsg) (opsOverlay, tea.Cmd, []opCompletedEvent) {
	idx := o.findOp(msg.opID)
	if idx < 0 {
		return o, nil, nil
	}
	// Remove the operation from the in-memory list.
	// Note: We keep the record in the database permanently per user request.
	// The operation will not be reloaded on next TUI start because
	// loadPendingActions only loads operations less than 5 minutes old.
	o.ops = append(o.ops[:idx], o.ops[idx+1:]...)
	return o, nil, nil
}

// --- Poll dispatch ---

func (o opsOverlay) doPoll(op operation) tea.Cmd {
	opID := op.id
	switch op.pollMode {
	case opPollModeAction:
		provider := o.provider
		actionID := op.actionID
		return func() tea.Msg {
			poller, ok := provider.(domain.ActionPoller)
			if !ok {
				return opPollErrorMsg{opID: opID, err: fmt.Errorf("provider lost ActionPoller capability")}
			}
			status, err := poller.PollAction(context.Background(), actionID)
			if err != nil {
				return opPollErrorMsg{opID: opID, err: err}
			}
			return opPollResultMsg{opID: opID, action: status}
		}
	case opPollModeServer:
		provider := o.provider
		serverID := op.serverID
		target := op.target
		return func() tea.Msg {
			server, err := provider.GetServer(context.Background(), serverID)
			if err != nil {
				return opPollErrorMsg{opID: opID, err: err}
			}
			if server == nil {
				return opPollErrorMsg{opID: opID, err: fmt.Errorf("server %q disappeared while polling", serverID)}
			}
			if server.Status == target {
				return opPollResultMsg{opID: opID, action: &domain.ActionStatus{Status: domain.ActionStatusSuccess, Progress: 100}}
			}
			return opPollResultMsg{opID: opID, action: &domain.ActionStatus{Status: domain.ActionStatusRunning, Progress: 0}}
		}
	default:
		return nil
	}
}

// --- Helpers ---

func (o opsOverlay) findOp(opID int) int {
	for i, op := range o.ops {
		if op.id == opID {
			return i
		}
	}
	return -1
}

func scheduleOpPollTick(opID int) tea.Cmd {
	return tea.Tick(overlayPollInterval, func(_ time.Time) tea.Msg {
		return opPollTickMsg{opID: opID}
	})
}

func scheduleDismiss(opID int) tea.Cmd {
	return tea.Tick(overlayDismissDelay, func(_ time.Time) tea.Msg {
		return opDismissMsg{opID: opID}
	})
}

// --- View ---

// View renders the floating overlay panel. Returns an empty string if
// there are no operations to show.
func (o opsOverlay) View(width, height int) string {
	if len(o.ops) == 0 || width < overlayMinWidth || height < 5 {
		return ""
	}

	// Build operation lines.
	lines := make([]string, 0, len(o.ops))
	for _, op := range o.ops {
		lines = append(lines, o.renderOpLine(op))
	}
	content := strings.Join(lines, "\n")

	// Build card.
	cardWidth := min(overlayMaxWidth, width-4)
	if cardWidth < overlayMinWidth {
		cardWidth = overlayMinWidth
	}

	titleStyle := lipgloss.NewStyle().
		Foreground(styles.Gray).
		Bold(true)

	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.DimGray).
		Padding(0, 1).
		Width(cardWidth).
		Render(titleStyle.Render("Operations") + "\n" + content)

	return card
}

// renderOpLine renders a single operation line with an appropriate
// icon/spinner prefix.
func (o opsOverlay) renderOpLine(op operation) string {
	// Truncate status text to fit the card.
	maxTextWidth := overlayMaxWidth - 6 // icon + spacing + border/padding
	text := op.statusText
	if lipgloss.Width(text) > maxTextWidth {
		text = ansi.Truncate(text, maxTextWidth-1, "…")
	}

	switch op.status {
	case opStatusSucceeded:
		icon := lipgloss.NewStyle().Foreground(styles.Green).Render("✓")
		return icon + " " + lipgloss.NewStyle().Foreground(styles.Green).Render(text)
	case opStatusFailed:
		icon := lipgloss.NewStyle().Foreground(styles.Red).Render("✗")
		return icon + " " + lipgloss.NewStyle().Foreground(styles.Red).Render(text)
	default:
		return o.spinner.View() + " " + lipgloss.NewStyle().Foreground(styles.White).Render(text)
	}
}

// --- Overlay compositing ---

// composeOverlay composites the overlay panel onto a base view string,
// placing it in the bottom-right corner. The base string is expected to
// be a full-screen render (width x height).
//
// It uses ANSI-safe truncation so styled text (with escape sequences)
// is not corrupted.
func composeOverlay(base string, overlay string, width, height int) string {
	if overlay == "" {
		return base
	}

	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	// Pad base lines to ensure we have enough rows.
	for len(baseLines) < height {
		baseLines = append(baseLines, strings.Repeat(" ", width))
	}

	overlayH := len(overlayLines)
	overlayW := overlayVisualWidth(overlayLines)

	// Position: bottom-right with 2 rows above the bottom (above footer)
	// and 1 col margin from the right edge.
	startRow := max(height-overlayH-2, 1)
	startCol := max(width-overlayW-1, 0)

	for i, oLine := range overlayLines {
		row := startRow + i
		if row < 0 || row >= len(baseLines) {
			continue
		}

		baseLine := baseLines[row]

		// ANSI-safe: truncate the base line to startCol visual cells.
		left := ansi.Truncate(baseLine, startCol, "")

		// Pad left to exactly startCol visual width in case the base
		// line was shorter.
		leftW := lipgloss.Width(left)
		if leftW < startCol {
			left += strings.Repeat(" ", startCol-leftW)
		}

		// Overlay line + right padding to fill remaining width.
		oLineW := lipgloss.Width(oLine)
		rightPad := max(width-startCol-oLineW, 0)

		baseLines[row] = left + oLine + strings.Repeat(" ", rightPad)
	}

	return strings.Join(baseLines, "\n")
}

// overlayVisualWidth returns the visual width of the widest line.
func overlayVisualWidth(lines []string) int {
	w := 0
	for _, l := range lines {
		lw := lipgloss.Width(l)
		if lw > w {
			w = lw
		}
	}
	return w
}
