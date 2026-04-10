package tui

import (
	"fmt"
	"strings"

	"nathanbeddoewebdev/vpsm/internal/config"
	"nathanbeddoewebdev/vpsm/internal/tui/components"
	"nathanbeddoewebdev/vpsm/internal/tui/styles"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Config messages ---

type configSavedMsg struct{}

type configSaveErrorMsg struct {
	err error
}

// --- Config model ---

type configViewModel struct {
	cfg  *config.Config
	keys []config.KeySpec

	cursor  int
	editing bool
	editor  textinput.Model

	width  int
	height int

	status  string
	isError bool
}

// RunConfigView starts the interactive config viewer/editor TUI.
func RunConfigView() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	m := configViewModel{
		cfg:  cfg,
		keys: config.Keys,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func (m configViewModel) Init() tea.Cmd {
	return nil
}

func (m configViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case configSavedMsg:
		m.editing = false
		m.status = "Configuration saved"
		m.isError = false
		return m, nil

	case configSaveErrorMsg:
		m.status = "Error: " + msg.err.Error()
		m.isError = true
		return m, nil
	}

	if m.editing {
		var cmd tea.Cmd
		m.editor, cmd = m.editor.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m configViewModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.editing {
		return m.handleEditKey(msg)
	}

	switch msg.String() {
	case "ctrl+c", "q", "esc":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.keys)-1 {
			m.cursor++
		}
	case "enter", "e":
		// Enter edit mode for the selected key.
		spec := m.keys[m.cursor]
		ti := textinput.New()
		ti.SetValue(spec.Get(m.cfg))
		ti.Focus()
		ti.Width = 40
		ti.Placeholder = "enter value"
		m.editor = ti
		m.editing = true
		m.status = ""
		return m, textinput.Blink
	}

	return m, nil
}

func (m configViewModel) handleEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.editing = false
		return m, nil
	case "enter":
		value := strings.TrimSpace(m.editor.Value())
		spec := m.keys[m.cursor]
		spec.Set(m.cfg, value)
		return m, m.saveConfig()
	}

	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	return m, cmd
}

func (m configViewModel) saveConfig() tea.Cmd {
	return func() tea.Msg {
		if err := m.cfg.Save(); err != nil {
			return configSaveErrorMsg{err: err}
		}
		return configSavedMsg{}
	}
}

func (m configViewModel) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	header := components.Header(m.width, "config", "")

	var footerBindings []components.KeyBinding
	if m.editing {
		footerBindings = []components.KeyBinding{
			{Key: "enter", Desc: "save"},
			{Key: "esc", Desc: "cancel"},
		}
	} else {
		footerBindings = []components.KeyBinding{
			{Key: "j/k", Desc: "navigate"},
			{Key: "e", Desc: "edit"},
			{Key: "q", Desc: "quit"},
		}
	}
	footer := components.Footer(m.width, footerBindings)

	statusBar := ""
	if m.status != "" {
		statusBar = components.StatusBar(m.width, m.status, m.isError)
	}

	headerH := lipgloss.Height(header)
	footerH := lipgloss.Height(footer)
	statusH := lipgloss.Height(statusBar)
	contentH := max(m.height-headerH-footerH-statusH, 1)

	content := m.renderContent(contentH)

	sections := []string{header, content}
	if statusBar != "" {
		sections = append(sections, statusBar)
	}
	sections = append(sections, footer)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m configViewModel) renderContent(height int) string {
	title := styles.Title.Render("Configuration")

	if len(m.keys) == 0 {
		combined := lipgloss.JoinVertical(lipgloss.Center,
			title,
			"",
			styles.MutedText.Render("No configuration keys defined."),
		)
		return lipgloss.Place(
			m.width, height,
			lipgloss.Center, lipgloss.Center,
			combined,
		)
	}

	cardWidth := 56
	labelWidth := 20

	rows := make([]string, 0, len(m.keys))
	for i, spec := range m.keys {
		isSelected := i == m.cursor

		prefix := "  "
		if isSelected {
			prefix = styles.AccentText.Render("> ")
		}

		name := spec.Name
		value := spec.Get(m.cfg)
		if value == "" {
			value = "(not set)"
		}

		var row string
		if isSelected {
			if m.editing {
				nameText := styles.Label.Width(labelWidth).Render(name)
				row = prefix + nameText + m.editor.View()
			} else {
				nameText := styles.Label.Width(labelWidth).Render(name)
				valueText := styles.Value.Bold(true).Render(value)
				row = prefix + nameText + valueText
			}
		} else {
			nameText := styles.MutedText.Width(labelWidth).Render(name)
			valueText := styles.MutedText.Render(value)
			row = prefix + nameText + valueText
		}

		rows = append(rows, row)

		// Show description for selected item.
		if isSelected && !m.editing {
			descLine := strings.Repeat(" ", 4) + styles.MutedText.Italic(true).Render(spec.Description)
			rows = append(rows, descLine)
		}
	}

	content := strings.Join(rows, "\n")
	card := styles.Card.Width(cardWidth).Render(content)

	combined := lipgloss.JoinVertical(lipgloss.Center, title, "", card)

	return lipgloss.Place(
		m.width, height,
		lipgloss.Center, lipgloss.Center,
		combined,
	)
}
