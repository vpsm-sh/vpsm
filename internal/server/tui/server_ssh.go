package tui

import (
	"regexp"
	"strings"

	"nathanbeddoewebdev/vpsm/internal/server/domain"
	"nathanbeddoewebdev/vpsm/internal/tui/components"
	"nathanbeddoewebdev/vpsm/internal/tui/styles"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// validUsernameRegex matches valid SSH usernames (alphanumeric, dot, underscore, hyphen).
var validUsernameRegex = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// --- SSH connect model ---

type serverSSHModel struct {
	server       *domain.Server
	providerName string
	ipAddress    string

	usernameInput   textinput.Model
	validationErr   string
	hostKeyConflict bool   // true when showing host key conflict error
	errorMsg        string // error message to display

	width  int
	height int

	embedded bool
}

func (m serverSSHModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m serverSSHModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Forward to text input.
	var cmd tea.Cmd
	m.usernameInput, cmd = m.usernameInput.Update(msg)
	return m, cmd
}

func (m serverSSHModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "esc":
		if m.embedded {
			server := *m.server
			return m, func() tea.Msg { return navigateToShowMsg{server: server} }
		}
		return m, tea.Quit

	case "k":
		// 'k' key: clear host key and retry (only available when hostKeyConflict is true).
		if m.hostKeyConflict && m.embedded {
			username := strings.TrimSpace(m.usernameInput.Value())
			if username == "" {
				username = "root"
			}
			return m, func() tea.Msg {
				return clearHostKeyMsg{
					server:    *m.server,
					username:  username,
					ipAddress: m.ipAddress,
				}
			}
		}
		// If not in conflict mode, fall through to default (textinput handles 'k').
		var cmd tea.Cmd
		m.usernameInput, cmd = m.usernameInput.Update(msg)
		return m, cmd

	case "enter":
		username := strings.TrimSpace(m.usernameInput.Value())
		if username == "" {
			username = "root"
		}
		// Validate username contains only valid SSH username characters.
		if !validUsernameRegex.MatchString(username) {
			m.validationErr = "Username must contain only letters, numbers, dots, underscores, and hyphens"
			return m, nil
		}
		m.validationErr = ""
		if m.embedded {
			return m, func() tea.Msg {
				return requestSSHMsg{
					server:    *m.server,
					username:  username,
					ipAddress: m.ipAddress,
				}
			}
		}
		return m, tea.Quit

	default:
		var cmd tea.Cmd
		m.usernameInput, cmd = m.usernameInput.Update(msg)
		return m, cmd
	}
}

func (m serverSSHModel) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	header := components.Header(m.width, "ssh connect", m.providerName)

	footerBindings := []components.KeyBinding{
		{Key: "enter", Desc: "connect"},
	}
	if m.hostKeyConflict {
		footerBindings = append(footerBindings, components.KeyBinding{Key: "k", Desc: "clear key & retry"})
	}
	footerBindings = append(footerBindings, components.KeyBinding{Key: "esc", Desc: "back"})
	footer := components.Footer(m.width, footerBindings)

	headerH := lipgloss.Height(header)
	footerH := lipgloss.Height(footer)
	contentH := max(m.height-headerH-footerH, 1)

	content := m.renderContent(contentH)

	return lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
}

func (m serverSSHModel) renderContent(height int) string {
	cardWidth := 48
	labelWidth := 10

	renderField := func(label, value string) string {
		l := styles.Label.Width(labelWidth).Render(label)
		v := styles.Value.Render(value)
		return l + v
	}

	title := styles.Title.Render("SSH Connect")

	fields := []string{
		renderField("Server", m.server.Name),
		renderField("Target", m.ipAddress),
		"",
		styles.Subtitle.Render("Username"),
		"",
		m.usernameInput.View(),
	}

	// Show error messages (validation or SSH connection errors).
	if m.validationErr != "" {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
		fields = append(fields, "", errStyle.Render(m.validationErr))
	} else if m.errorMsg != "" {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
		fields = append(fields, "", errStyle.Render(m.errorMsg))
		if m.hostKeyConflict {
			hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Italic(true)
			fields = append(fields, "", hintStyle.Render("Press 'k' to clear the old key and retry"))
		}
	}

	cardContent := strings.Join(fields, "\n")
	card := styles.Card.Width(cardWidth).Render(cardContent)

	combined := lipgloss.JoinVertical(lipgloss.Center, title, "", card)

	return lipgloss.Place(
		m.width, height,
		lipgloss.Center, lipgloss.Center,
		combined,
	)
}
