package tui

import (
	"fmt"
	"strings"

	"nathanbeddoewebdev/vpsm/internal/services/auth"
	"nathanbeddoewebdev/vpsm/internal/tui/components"
	"nathanbeddoewebdev/vpsm/internal/tui/styles"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Messages ---

type tokenSavedMsg struct{}

type tokenSaveErrorMsg struct {
	err error
}

// --- Auth login model ---

type authLoginModel struct {
	provider string
	store    auth.Store

	tokenInput textinput.Model

	width  int
	height int

	err      error
	saved    bool
	quitting bool
}

// AuthLoginResult holds the outcome of the login TUI.
type AuthLoginResult struct {
	Saved bool
}

// RunAuthLogin starts the interactive auth login TUI.
func RunAuthLogin(provider string, store auth.Store) (*AuthLoginResult, error) {
	ti := textinput.New()
	ti.Placeholder = "paste your API token here"
	ti.Focus()
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '*'
	ti.Width = 50

	m := authLoginModel{
		provider:   provider,
		store:      store,
		tokenInput: ti,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to run auth login: %w", err)
	}

	final := result.(authLoginModel)
	if final.quitting && !final.saved {
		return nil, nil
	}
	return &AuthLoginResult{Saved: final.saved}, nil
}

func (m authLoginModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m authLoginModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tokenSavedMsg:
		m.saved = true
		return m, tea.Quit

	case tokenSaveErrorMsg:
		m.err = msg.err
		return m, nil
	}

	var cmd tea.Cmd
	m.tokenInput, cmd = m.tokenInput.Update(msg)
	return m, cmd
}

func (m authLoginModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.quitting = true
		return m, tea.Quit
	case "enter":
		token := strings.TrimSpace(m.tokenInput.Value())
		if token == "" {
			m.err = fmt.Errorf("token cannot be empty")
			return m, nil
		}
		m.err = nil
		return m, m.saveToken(token)
	}

	var cmd tea.Cmd
	m.tokenInput, cmd = m.tokenInput.Update(msg)
	m.err = nil
	return m, cmd
}

func (m authLoginModel) saveToken(token string) tea.Cmd {
	return func() tea.Msg {
		if err := m.store.SetToken(m.provider, token); err != nil {
			return tokenSaveErrorMsg{err: err}
		}
		return tokenSavedMsg{}
	}
}

func (m authLoginModel) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	header := components.Header(m.width, "auth login", m.provider)
	footerBindings := []components.KeyBinding{
		{Key: "enter", Desc: "save"},
		{Key: "esc", Desc: "cancel"},
	}
	footer := components.Footer(m.width, footerBindings)

	headerH := lipgloss.Height(header)
	footerH := lipgloss.Height(footer)
	contentH := max(m.height-headerH-footerH, 1)

	content := m.renderContent(contentH)

	return lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
}

func (m authLoginModel) renderContent(height int) string {
	title := styles.Title.Render("API Token")
	hint := styles.MutedText.Render("Enter your " + m.provider + " API token")

	inputView := m.tokenInput.View()

	var errLine string
	if m.err != nil {
		errLine = "\n" + styles.ErrorText.Render(m.err.Error())
	}

	card := lipgloss.JoinVertical(lipgloss.Left,
		title,
		hint,
		"",
		inputView,
		errLine,
	)

	return lipgloss.Place(
		m.width, height,
		lipgloss.Center, lipgloss.Center,
		card,
	)
}
