package tui

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"nathanbeddoewebdev/vpsm/internal/sshkeys"
	"nathanbeddoewebdev/vpsm/internal/tui/components"
	"nathanbeddoewebdev/vpsm/internal/tui/styles"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// ErrAborted is returned when a user cancels the interactive flow.
var ErrAborted = errors.New("ssh key add aborted by user")

// SSHKeySource describes how the public key is provided.
type SSHKeySource int

const (
	SSHKeySourceFile SSHKeySource = iota
	SSHKeySourcePaste
)

func (s SSHKeySource) label() string {
	switch s {
	case SSHKeySourcePaste:
		return "Paste public key"
	default:
		return "Use local .pub file"
	}
}

// SSHKeyAddPrefill provides initial values for the SSH key add flow.
type SSHKeyAddPrefill struct {
	Source    SSHKeySource
	Path      string
	PublicKey string
	Name      string
}

// SSHKeyAddResult contains the resolved SSH key name and public key.
type SSHKeyAddResult struct {
	Name      string
	PublicKey string
}

type sshKeyAddStep int

const (
	sshStepSource sshKeyAddStep = iota
	sshStepPath
	sshStepPaste
	sshStepName
	sshStepConfirm
)

func (s sshKeyAddStep) label() string {
	switch s {
	case sshStepSource:
		return "Source"
	case sshStepPath:
		return "Key file"
	case sshStepPaste:
		return "Paste"
	case sshStepName:
		return "Name"
	case sshStepConfirm:
		return "Confirm"
	default:
		return ""
	}
}

type sshKeyAddModel struct {
	providerName string
	step         sshKeyAddStep
	source       SSHKeySource

	pathInput textinput.Model
	keyInput  textinput.Model
	nameInput textinput.Model

	keyPath   string
	publicKey string

	sourceIdx  int
	confirmIdx int

	width  int
	height int

	err      string
	quitting bool
	result   *SSHKeyAddResult
}

// RunSSHKeyAdd runs the full-screen SSH key add wizard.
func RunSSHKeyAdd(providerName string, prefill SSHKeyAddPrefill) (*SSHKeyAddResult, error) {
	source := resolveSource(prefill)
	defaultPath := sshkeys.DefaultPath()
	if prefill.Path == "" {
		prefill.Path = defaultPath
	}

	pathInput := textinput.New()
	pathInput.Placeholder = defaultPath
	pathInput.CharLimit = 300
	pathInput.Width = 60
	pathInput.SetValue(prefill.Path)

	keyInput := textinput.New()
	keyInput.Placeholder = "ssh-ed25519 AAAA..."
	keyInput.CharLimit = 4096
	keyInput.Width = 70
	keyInput.SetValue(prefill.PublicKey)

	nameInput := textinput.New()
	nameInput.Placeholder = sshkeys.DefaultKeyName()
	nameInput.CharLimit = 64
	nameInput.Width = 40
	if prefill.Name != "" {
		nameInput.SetValue(prefill.Name)
	}

	m := sshKeyAddModel{
		providerName: providerName,
		step:         sshStepSource,
		source:       source,
		pathInput:    pathInput,
		keyInput:     keyInput,
		nameInput:    nameInput,
	}
	if source == SSHKeySourcePaste {
		m.sourceIdx = 1
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to run ssh key add: %w", err)
	}

	final := result.(sshKeyAddModel)
	if final.quitting || final.result == nil {
		return nil, ErrAborted
	}

	return final.result, nil
}

// RunSSHKeyAddAccessible runs a single-form accessible SSH key add flow.
func RunSSHKeyAddAccessible(providerName string, prefill SSHKeyAddPrefill) (*SSHKeyAddResult, error) {
	_ = providerName

	sourceValue := "file"
	if resolveSource(prefill) == SSHKeySourcePaste {
		sourceValue = "paste"
	}

	keyPath := prefill.Path
	if keyPath == "" {
		keyPath = sshkeys.DefaultPath()
	}

	publicKey := strings.TrimSpace(prefill.PublicKey)
	name := strings.TrimSpace(prefill.Name)
	if name == "" {
		if sourceValue == "file" {
			name = sshkeys.SuggestKeyName(keyPath)
		} else {
			name = sshkeys.DefaultKeyName()
		}
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("SSH key source").
				Options(
					huh.NewOption("Use local .pub file", "file"),
					huh.NewOption("Paste public key", "paste"),
				).
				Value(&sourceValue),
			huh.NewInput().
				Title("Path to public key").
				Description("Used when source is file").
				Value(&keyPath).
				Validate(func(s string) error {
					if sourceValue != "file" {
						return nil
					}
					trimmed := strings.TrimSpace(s)
					if trimmed == "" {
						return fmt.Errorf("path cannot be empty")
					}
					expanded, err := sshkeys.ExpandHomePath(trimmed)
					if err != nil {
						return err
					}
					if _, err := os.Stat(expanded); os.IsNotExist(err) {
						return fmt.Errorf("SSH key file not found: %s", trimmed)
					}
					if _, err := sshkeys.ReadAndValidatePublicKey(expanded); err != nil {
						return err
					}
					return nil
				}),
			huh.NewInput().
				Title("Public key").
				Description("Used when source is paste").
				Value(&publicKey).
				Validate(func(s string) error {
					if sourceValue != "paste" {
						return nil
					}
					_, err := sshkeys.ValidatePublicKey(s)
					return err
				}),
			huh.NewInput().
				Title("Key name").
				Value(&name).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("name cannot be empty")
					}
					return nil
				}),
		),
	).WithAccessible(true)

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, ErrAborted
		}
		return nil, err
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name cannot be empty")
	}

	var err error
	if sourceValue == "file" {
		expanded, err := sshkeys.ExpandHomePath(strings.TrimSpace(keyPath))
		if err != nil {
			return nil, err
		}
		publicKey, err = sshkeys.ReadAndValidatePublicKey(expanded)
		if err != nil {
			return nil, err
		}
	} else {
		publicKey, err = sshkeys.ValidatePublicKey(publicKey)
		if err != nil {
			return nil, err
		}
	}

	return &SSHKeyAddResult{Name: name, PublicKey: publicKey}, nil
}

func resolveSource(prefill SSHKeyAddPrefill) SSHKeySource {
	if prefill.Source == SSHKeySourcePaste {
		return SSHKeySourcePaste
	}
	if strings.TrimSpace(prefill.PublicKey) != "" && strings.TrimSpace(prefill.Path) == "" {
		return SSHKeySourcePaste
	}
	return SSHKeySourceFile
}

func (m sshKeyAddModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m sshKeyAddModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		available := max(msg.Width-10, 20)
		m.pathInput.Width = minInt(60, available)
		m.keyInput.Width = minInt(70, available)
		m.nameInput.Width = minInt(40, available)
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m sshKeyAddModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
	}

	switch m.step {
	case sshStepSource:
		return m.handleSourceKey(msg)
	case sshStepPath:
		return m.handlePathKey(msg)
	case sshStepPaste:
		return m.handlePasteKey(msg)
	case sshStepName:
		return m.handleNameKey(msg)
	case sshStepConfirm:
		return m.handleConfirmKey(msg)
	}

	return m, nil
}

func (m sshKeyAddModel) handleSourceKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.quitting = true
		return m, tea.Quit
	case "up", "k", "left", "h":
		if m.sourceIdx > 0 {
			m.sourceIdx--
		}
	case "down", "j", "right", "l":
		if m.sourceIdx < 1 {
			m.sourceIdx++
		}
	case "enter":
		if m.sourceIdx == 0 {
			m.source = SSHKeySourceFile
			m.step = sshStepPath
			m.pathInput.Focus()
		} else {
			m.source = SSHKeySourcePaste
			m.step = sshStepPaste
			m.keyInput.Focus()
		}
		m.err = ""
		return m, textinput.Blink
	}

	return m, nil
}

func (m sshKeyAddModel) handlePathKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.step = sshStepSource
		return m, nil
	case "enter":
		path := strings.TrimSpace(m.pathInput.Value())
		if path == "" {
			m.err = "Path cannot be empty"
			return m, nil
		}
		expanded, err := sshkeys.ExpandHomePath(path)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		if _, err := os.Stat(expanded); os.IsNotExist(err) {
			m.err = fmt.Sprintf("SSH key file not found: %s", path)
			return m, nil
		}
		key, err := sshkeys.ReadAndValidatePublicKey(expanded)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.publicKey = key
		m.keyPath = path
		m.setNameDefaultIfEmpty(sshkeys.SuggestKeyName(path))
		m.step = sshStepName
		m.nameInput.Focus()
		m.err = ""
		return m, textinput.Blink
	}

	var cmd tea.Cmd
	m.pathInput, cmd = m.pathInput.Update(msg)
	m.err = ""
	return m, cmd
}

func (m sshKeyAddModel) handlePasteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.step = sshStepSource
		return m, nil
	case "enter":
		key, err := sshkeys.ValidatePublicKey(m.keyInput.Value())
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.publicKey = key
		m.keyPath = ""
		m.setNameDefaultIfEmpty(sshkeys.DefaultKeyName())
		m.step = sshStepName
		m.nameInput.Focus()
		m.err = ""
		return m, textinput.Blink
	}

	var cmd tea.Cmd
	m.keyInput, cmd = m.keyInput.Update(msg)
	m.err = ""
	return m, cmd
}

func (m sshKeyAddModel) handleNameKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.source == SSHKeySourcePaste {
			m.step = sshStepPaste
		} else {
			m.step = sshStepPath
		}
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.nameInput.Value())
		if name == "" {
			m.err = "Name cannot be empty"
			return m, nil
		}
		m.nameInput.SetValue(name)
		m.step = sshStepConfirm
		m.confirmIdx = 0
		m.err = ""
		return m, nil
	}

	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)
	m.err = ""
	return m, cmd
}

func (m sshKeyAddModel) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.step = sshStepName
		return m, nil
	case "q":
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
			m.result = &SSHKeyAddResult{
				Name:      strings.TrimSpace(m.nameInput.Value()),
				PublicKey: m.publicKey,
			}
			return m, tea.Quit
		}
		m.quitting = true
		return m, tea.Quit
	case "y":
		m.result = &SSHKeyAddResult{
			Name:      strings.TrimSpace(m.nameInput.Value()),
			PublicKey: m.publicKey,
		}
		return m, tea.Quit
	case "n":
		m.quitting = true
		return m, tea.Quit
	}

	return m, nil
}

func (m sshKeyAddModel) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	header := components.Header(m.width, "ssh-key add", m.providerName)

	var footerBindings []components.KeyBinding
	switch m.step {
	case sshStepSource:
		footerBindings = []components.KeyBinding{
			{Key: "j/k", Desc: "select"},
			{Key: "enter", Desc: "next"},
			{Key: "esc", Desc: "cancel"},
		}
	case sshStepConfirm:
		footerBindings = []components.KeyBinding{
			{Key: "y/n", Desc: "confirm"},
			{Key: "enter", Desc: "select"},
			{Key: "esc", Desc: "back"},
		}
	default:
		footerBindings = []components.KeyBinding{
			{Key: "enter", Desc: "next"},
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

func (m sshKeyAddModel) renderContent(height int) string {
	progress := m.renderProgress()

	var stepContent string
	switch m.step {
	case sshStepSource:
		stepContent = m.renderSourceStep()
	case sshStepPath:
		stepContent = m.renderPathStep()
	case sshStepPaste:
		stepContent = m.renderPasteStep()
	case sshStepName:
		stepContent = m.renderNameStep()
	case sshStepConfirm:
		stepContent = m.renderConfirmStep()
	}

	combined := lipgloss.JoinVertical(lipgloss.Center, progress, "", stepContent)
	return lipgloss.Place(
		m.width, height,
		lipgloss.Center, lipgloss.Center,
		combined,
	)
}

func (m sshKeyAddModel) renderProgress() string {
	steps := []sshKeyAddStep{sshStepSource, sshStepName, sshStepConfirm}
	if m.source == SSHKeySourcePaste {
		steps = []sshKeyAddStep{sshStepSource, sshStepPaste, sshStepName, sshStepConfirm}
	} else if m.source == SSHKeySourceFile {
		steps = []sshKeyAddStep{sshStepSource, sshStepPath, sshStepName, sshStepConfirm}
	}

	parts := make([]string, len(steps))
	for i, s := range steps {
		label := s.label()
		if s == m.step {
			parts[i] = styles.AccentText.Bold(true).Render(label)
		} else if s < m.step {
			parts[i] = styles.SuccessText.Render(label)
		} else {
			parts[i] = styles.MutedText.Render(label)
		}
	}

	return strings.Join(parts, styles.MutedText.Render(" > "))
}

func (m sshKeyAddModel) renderSourceStep() string {
	title := styles.Title.Render("Choose key source")
	hint := styles.MutedText.Render("Select how you'd like to provide the public key")

	options := []SSHKeySource{SSHKeySourceFile, SSHKeySourcePaste}
	rows := make([]string, 0, len(options))
	for i, option := range options {
		prefix := "  "
		label := option.label()
		if i == m.sourceIdx {
			prefix = styles.AccentText.Render("> ")
			label = styles.Value.Bold(true).Render(label)
		} else {
			label = styles.MutedText.Render(label)
		}
		rows = append(rows, prefix+label)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		hint,
		"",
		strings.Join(rows, "\n"),
	)
}

func (m sshKeyAddModel) renderPathStep() string {
	title := styles.Title.Render("SSH public key file")
	hint := styles.MutedText.Render("Provide the path to your .pub file")

	inputView := m.pathInput.View()

	var errLine string
	if m.err != "" {
		errLine = "\n" + styles.ErrorText.Render(m.err)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		hint,
		"",
		inputView,
		errLine,
	)
}

func (m sshKeyAddModel) renderPasteStep() string {
	title := styles.Title.Render("Paste SSH public key")
	hint := styles.MutedText.Render("Paste a single-line SSH public key")

	inputView := m.keyInput.View()

	var errLine string
	if m.err != "" {
		errLine = "\n" + styles.ErrorText.Render(m.err)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		hint,
		"",
		inputView,
		errLine,
	)
}

func (m sshKeyAddModel) renderNameStep() string {
	title := styles.Title.Render("Key name")
	hint := styles.MutedText.Render("Give this key a friendly name")

	inputView := m.nameInput.View()

	var errLine string
	if m.err != "" {
		errLine = "\n" + styles.ErrorText.Render(m.err)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		hint,
		"",
		inputView,
		errLine,
	)
}

func (m sshKeyAddModel) renderConfirmStep() string {
	title := styles.Title.Render("Review & Confirm")

	labelWidth := 14
	renderField := func(label, value string) string {
		l := styles.Label.Width(labelWidth).Render(label)
		v := styles.Value.Render(value)
		return l + v
	}

	source := "File"
	if m.source == SSHKeySourcePaste {
		source = "Paste"
	}

	fields := []string{
		renderField("Source", source),
	}

	if m.source == SSHKeySourceFile {
		fields = append(fields, renderField("Path", m.keyPath))
	} else {
		fields = append(fields, renderField("Key", truncateValue(m.publicKey, 48)))
	}

	fields = append(fields, renderField("Name", strings.TrimSpace(m.nameInput.Value())))

	summary := styles.Card.Width(60).Render(strings.Join(fields, "\n"))

	createBtn := "  Upload  "
	cancelBtn := "  Cancel  "

	if m.confirmIdx == 0 {
		createBtn = lipgloss.NewStyle().
			Background(styles.Green).
			Foreground(lipgloss.Color("#000000")).
			Bold(true).
			Render(createBtn)
		cancelBtn = styles.MutedText.Render(cancelBtn)
	} else {
		createBtn = styles.MutedText.Render(createBtn)
		cancelBtn = lipgloss.NewStyle().
			Background(styles.Red).
			Foreground(lipgloss.Color("#000000")).
			Bold(true).
			Render(cancelBtn)
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Center, createBtn, "  ", cancelBtn)

	return lipgloss.JoinVertical(lipgloss.Center,
		title,
		"",
		summary,
		"",
		buttons,
	)
}

func (m *sshKeyAddModel) setNameDefaultIfEmpty(name string) {
	if strings.TrimSpace(m.nameInput.Value()) == "" {
		m.nameInput.SetValue(name)
	}
}

func truncateValue(value string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
