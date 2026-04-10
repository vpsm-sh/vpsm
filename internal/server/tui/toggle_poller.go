package tui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"nathanbeddoewebdev/vpsm/internal/server/domain"

	tea "github.com/charmbracelet/bubbletea"
)

// --- Polling configuration ---

const (
	// tuiPollInterval is the delay between successive poll requests.
	// 3 s keeps us well under Hetzner's 3 600 req/h rate limit while
	// still feeling responsive (most start/stop ops finish in 10-30 s).
	tuiPollInterval = 3 * time.Second

	// tuiMaxPollAttempts caps how many times we poll before giving up.
	// At 3 s intervals this gives ~5 minutes.
	tuiMaxPollAttempts = 100
)

// --- Poll mode ---

const (
	pollModeAction = "action" // poll via ActionPoller.PollAction
	pollModeServer = "server" // poll via Provider.GetServer (generic fallback)
)

// --- Messages ---

// serverToggleInitiatedMsg is sent after the initial StartServer /
// StopServer API call returns. It carries the ActionStatus so the
// model can decide which polling strategy to use.
type serverToggleInitiatedMsg struct {
	serverID   string
	serverName string
	verb       string // "started" or "stopped" (for the final success message)
	target     string // target server status for the GetServer fallback (e.g. "running", "off")
	action     *domain.ActionStatus
}

// serverToggleErrorMsg carries an error from the initial toggle API call.
type serverToggleErrorMsg struct {
	err error
}

// pollActionTickMsg tells the Update loop it is time to fire the next poll.
type pollActionTickMsg struct{}

// pollActionResultMsg carries the result of a single poll.
type pollActionResultMsg struct {
	action *domain.ActionStatus
}

// pollActionErrorMsg carries an error from a failed poll.
type pollActionErrorMsg struct {
	err error
}

// --- Toggle outcome ---

// toggleOutcome describes the terminal result of a toggle operation.
// Parent models inspect this to decide what to do next (e.g. refresh
// a server list vs. refresh a single server detail).
type toggleOutcome struct {
	// Success is true when the action completed successfully.
	Success bool
	// ServerName and Verb are used to build user-facing messages.
	ServerName string
	Verb       string // "started" or "stopped"
	// StatusText is a pre-formatted status-bar message (set on error/timeout).
	StatusText string
	// IsError controls whether StatusText renders in error style.
	IsError bool
}

// --- Toggle poller ---

// togglePoller encapsulates the state machine for polling a start/stop
// action to completion. It is embedded into TUI models (serverListModel,
// serverShowModel) so the polling logic lives in one place.
//
// The poller is a value type — methods return a new copy plus any
// tea.Cmd to execute. When the operation reaches a terminal state, the
// method returns a non-nil *toggleOutcome for the parent to act on.
type togglePoller struct {
	provider     domain.Provider
	providerName string    // used for audit log entries
	active       bool      // true while a toggle + poll cycle is in flight
	pollMode     string    // "action" or "server"
	actionID     string    // provider action ID (pollModeAction)
	pollServerID string    // server ID being toggled
	pollTarget   string    // target server status (pollModeServer)
	toggleVerb   string    // "started" or "stopped"
	toggleName   string    // server name (for messages)
	pollCount    int       // number of polls fired so far
	startedAt    time.Time // when InitiateToggle was called (for audit duration)

	// consecutiveErrors tracks transient poll failures. Rate-limit errors
	// always abort immediately; other errors are tolerated up to
	// maxTUITransientErrors consecutive failures before giving up.
	consecutiveErrors int

	// statusText / statusIsError are updated on every tick so the
	// parent model can render progress in its status bar.
	statusText  string
	statusError bool
}

// maxTUITransientErrors is the number of consecutive non-rate-limit errors
// the TUI poller tolerates before giving up.
const maxTUITransientErrors = 3

// newTogglePoller creates a poller bound to the given provider.
func newTogglePoller(provider domain.Provider, providerName string) togglePoller {
	return togglePoller{provider: provider, providerName: providerName}
}

// --- Commands ---

// InitiateToggle fires the initial StartServer or StopServer API call.
// It returns a tea.Cmd that produces a serverToggleInitiatedMsg (or
// serverToggleErrorMsg on failure).
func (tp *togglePoller) InitiateToggle(server domain.Server) tea.Cmd {
	tp.startedAt = time.Now().UTC()
	provider := tp.provider
	return func() tea.Msg {
		ctx := context.Background()
		switch server.Status {
		case "running":
			action, err := provider.StopServer(ctx, server.ID)
			if err != nil {
				return serverToggleErrorMsg{err: fmt.Errorf("failed to stop server %q: %w", server.Name, err)}
			}
			return serverToggleInitiatedMsg{
				serverID:   server.ID,
				serverName: server.Name,
				verb:       "stopped",
				target:     "off",
				action:     action,
			}
		case "off", "stopped":
			action, err := provider.StartServer(ctx, server.ID)
			if err != nil {
				return serverToggleErrorMsg{err: fmt.Errorf("failed to start server %q: %w", server.Name, err)}
			}
			return serverToggleInitiatedMsg{
				serverID:   server.ID,
				serverName: server.Name,
				verb:       "started",
				target:     "running",
				action:     action,
			}
		default:
			return serverToggleErrorMsg{
				err: fmt.Errorf("cannot start/stop server %q: current status is %q", server.Name, server.Status),
			}
		}
	}
}

// HandleInitiated processes a serverToggleInitiatedMsg. It returns the
// updated poller, a tea.Cmd to execute, and a *toggleOutcome if the
// action already reached a terminal state (nil otherwise).
func (tp togglePoller) HandleInitiated(msg serverToggleInitiatedMsg) (togglePoller, tea.Cmd, *toggleOutcome) {
	tp.toggleName = msg.serverName
	tp.toggleVerb = msg.verb
	tp.pollServerID = msg.serverID
	tp.pollTarget = msg.target
	tp.pollCount = 0

	action := msg.action

	// Fast path: action completed synchronously. We still need to verify
	// the server has actually reached the target status, because some
	// operations (e.g. Hetzner Shutdown) report the action as "success"
	// when the signal is sent, not when the server is fully stopped.
	if action != nil && action.Status == domain.ActionStatusSuccess {
		tp.pollMode = pollModeServer
		tp.statusText = fmt.Sprintf("Waiting for server %q to reach %q status...", msg.serverName, msg.target)
		tp.statusError = false
		return tp, schedulePollTick(), nil
	}

	// Fast path: action failed immediately.
	if action != nil && action.Status == domain.ActionStatusError {
		tp.active = false
		errMsg := "action failed"
		if action.ErrorMessage != "" {
			errMsg = action.ErrorMessage
		}
		fastErr := fmt.Errorf("%s", errMsg)
		recordAudit(tp.providerName, opAuditCommand(msg.verb), "server", msg.serverID, msg.serverName, fastErr, tp.startedAt)
		return tp, nil, &toggleOutcome{
			StatusText: fmt.Sprintf("Failed to %s server %q: %s", verbToInfinitive(msg.verb), msg.serverName, errMsg),
			IsError:    true,
		}
	}

	// Action is still running -- choose polling strategy.
	if _, ok := tp.provider.(domain.ActionPoller); ok && action != nil && action.ID != "" {
		tp.pollMode = pollModeAction
		tp.actionID = action.ID
	} else {
		tp.pollMode = pollModeServer
	}

	tp.statusText = fmt.Sprintf("%s server %q...", verbToGerund(msg.verb), msg.serverName)
	tp.statusError = false

	return tp, schedulePollTick(), nil
}

// HandlePollTick fires a single poll request. Call this when the model
// receives a pollActionTickMsg.
func (tp togglePoller) HandlePollTick() (togglePoller, tea.Cmd) {
	if !tp.active {
		return tp, nil // stale tick after toggle completed
	}
	return tp, tp.doPoll()
}

// HandlePollResult processes a pollActionResultMsg and returns the updated
// poller, a tea.Cmd, and a *toggleOutcome if the action reached a terminal
// state.
func (tp togglePoller) HandlePollResult(msg pollActionResultMsg) (togglePoller, tea.Cmd, *toggleOutcome) {
	tp.consecutiveErrors = 0 // successful response — reset transient error counter
	status := msg.action

	switch status.Status {
	case domain.ActionStatusSuccess:
		if tp.pollMode == pollModeAction {
			// The provider action completed, but the server may not have
			// reached the target status yet (e.g. Hetzner Shutdown reports
			// success when the ACPI signal is sent, not when the server is
			// off). Transition to server-status polling to confirm.
			tp.pollMode = pollModeServer
			tp.consecutiveErrors = 0
			tp.statusText = fmt.Sprintf("Waiting for server %q to reach %q status...", tp.toggleName, tp.pollTarget)
			tp.statusError = false
			return tp, schedulePollTick(), nil
		}
		// pollModeServer: the server has reached the target status.
		tp.active = false
		tp.pollCount = 0
		recordAudit(tp.providerName, opAuditCommand(tp.toggleVerb), "server", tp.pollServerID, tp.toggleName, nil, tp.startedAt)
		return tp, nil, &toggleOutcome{
			Success:    true,
			ServerName: tp.toggleName,
			Verb:       tp.toggleVerb,
		}

	case domain.ActionStatusError:
		tp.active = false
		tp.pollCount = 0
		errMsg := "action failed"
		if status.ErrorMessage != "" {
			errMsg = status.ErrorMessage
		}
		pollErrStatus := fmt.Errorf("%s", errMsg)
		recordAudit(tp.providerName, opAuditCommand(tp.toggleVerb), "server", tp.pollServerID, tp.toggleName, pollErrStatus, tp.startedAt)
		return tp, nil, &toggleOutcome{
			StatusText: fmt.Sprintf("Failed to %s server %q: %s", verbToInfinitive(tp.toggleVerb), tp.toggleName, errMsg),
			IsError:    true,
		}

	default:
		// Still running -- update progress and schedule next tick.
		tp.pollCount++
		if tp.pollCount >= tuiMaxPollAttempts {
			tp.active = false
			tp.pollCount = 0
			timeoutErr := fmt.Errorf("timed out waiting for server %q to %s", tp.toggleName, verbToInfinitive(tp.toggleVerb))
			recordAudit(tp.providerName, opAuditCommand(tp.toggleVerb), "server", tp.pollServerID, tp.toggleName, timeoutErr, tp.startedAt)
			return tp, nil, &toggleOutcome{
				StatusText: timeoutErr.Error(),
				IsError:    true,
			}
		}

		label := verbToGerund(tp.toggleVerb)
		if status.Progress > 0 {
			tp.statusText = fmt.Sprintf("%s server %q... (%d%%)", label, tp.toggleName, status.Progress)
		} else {
			tp.statusText = fmt.Sprintf("%s server %q...", label, tp.toggleName)
		}
		tp.statusError = false

		return tp, schedulePollTick(), nil
	}
}

// HandlePollError processes a pollActionErrorMsg. Rate-limit errors always
// produce a terminal outcome. Other errors are tolerated up to
// maxTUITransientErrors consecutive failures — within that budget a new
// poll tick is scheduled so the operation can recover from brief network
// blips.
func (tp togglePoller) HandlePollError(msg pollActionErrorMsg) (togglePoller, tea.Cmd, *toggleOutcome) {
	// Rate-limit errors abort immediately.
	if isRateLimited(msg.err) {
		tp.active = false
		tp.pollCount = 0
		tp.consecutiveErrors = 0
		rateLimitErr := fmt.Errorf("polling stopped (rate limited)")
		recordAudit(tp.providerName, opAuditCommand(tp.toggleVerb), "server", tp.pollServerID, tp.toggleName, rateLimitErr, tp.startedAt)
		return tp, nil, &toggleOutcome{
			StatusText: "Polling stopped (rate limited)",
			IsError:    true,
		}
	}

	tp.consecutiveErrors++
	if tp.consecutiveErrors >= maxTUITransientErrors {
		tp.active = false
		tp.pollCount = 0
		tp.consecutiveErrors = 0
		pollErr := fmt.Errorf("error polling (after %d consecutive failures): %v", tp.consecutiveErrors, msg.err)
		recordAudit(tp.providerName, opAuditCommand(tp.toggleVerb), "server", tp.pollServerID, tp.toggleName, pollErr, tp.startedAt)
		return tp, nil, &toggleOutcome{
			StatusText: pollErr.Error(),
			IsError:    true,
		}
	}

	// Transient error within budget — schedule another poll tick.
	tp.statusText = fmt.Sprintf("Transient error, retrying... (%d/%d)", tp.consecutiveErrors, maxTUITransientErrors)
	tp.statusError = false
	return tp, schedulePollTick(), nil
}

// --- Internal helpers ---

// doPoll fires a single poll request using the appropriate strategy.
func (tp togglePoller) doPoll() tea.Cmd {
	switch tp.pollMode {
	case pollModeAction:
		provider := tp.provider
		actionID := tp.actionID
		return func() tea.Msg {
			poller, ok := provider.(domain.ActionPoller)
			if !ok {
				return pollActionErrorMsg{err: fmt.Errorf("provider lost ActionPoller capability")}
			}
			status, err := poller.PollAction(context.Background(), actionID)
			if err != nil {
				return pollActionErrorMsg{err: err}
			}
			return pollActionResultMsg{action: status}
		}
	case pollModeServer:
		provider := tp.provider
		serverID := tp.pollServerID
		target := tp.pollTarget
		return func() tea.Msg {
			server, err := provider.GetServer(context.Background(), serverID)
			if err != nil {
				return pollActionErrorMsg{err: err}
			}
			if server == nil {
				return pollActionErrorMsg{err: fmt.Errorf("server %q disappeared while polling", serverID)}
			}
			if server.Status == target {
				return pollActionResultMsg{action: &domain.ActionStatus{Status: domain.ActionStatusSuccess, Progress: 100}}
			}
			// Synthesize a "running" ActionStatus with 0 progress.
			return pollActionResultMsg{action: &domain.ActionStatus{Status: domain.ActionStatusRunning, Progress: 0}}
		}
	default:
		return nil
	}
}

// schedulePollTick returns a tea.Cmd that sends a pollActionTickMsg after
// the configured interval.
func schedulePollTick() tea.Cmd {
	return tea.Tick(tuiPollInterval, func(_ time.Time) tea.Msg {
		return pollActionTickMsg{}
	})
}

// isRateLimited checks whether err wraps domain.ErrRateLimited.
func isRateLimited(err error) bool {
	return errors.Is(err, domain.ErrRateLimited)
}

// --- Verb helpers ---

// verbToInfinitive converts a past-tense toggle verb to its infinitive form
// for use in progress / error messages.
func verbToInfinitive(verb string) string {
	switch verb {
	case "started":
		return "start"
	case "stopped":
		return "stop"
	default:
		return verb
	}
}

// verbToGerund converts a past-tense toggle verb to its gerund form
// for use in progress messages.
func verbToGerund(verb string) string {
	switch verb {
	case "started":
		return "Starting"
	case "stopped":
		return "Stopping"
	default:
		return verb
	}
}
