package tui

import (
	"context"
	"fmt"
	"strings"

	"nathanbeddoewebdev/vpsm/internal/server/domain"
	"nathanbeddoewebdev/vpsm/internal/tui/components"
	"nathanbeddoewebdev/vpsm/internal/tui/styles"
	"nathanbeddoewebdev/vpsm/internal/util"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Create wizard steps ---

type createStep int

const (
	stepLoading createStep = iota
	stepName
	stepLocation
	stepServerType
	stepImage
	stepSSHKeys
	stepConfirm
)

func (s createStep) label() string {
	switch s {
	case stepName:
		return "Name"
	case stepLocation:
		return "Location"
	case stepServerType:
		return "Type"
	case stepImage:
		return "Image"
	case stepSSHKeys:
		return "SSH Keys"
	case stepConfirm:
		return "Confirm"
	default:
		return ""
	}
}

// --- Messages ---

type catalogLoadedMsg struct {
	data catalogData
}

type catalogErrorMsg struct {
	err error
}

// --- Create item for selection lists ---

type createItem struct {
	name  string // value to store
	label string // display text
}

// --- Server create model ---

type serverCreateModel struct {
	provider     domain.CatalogProvider
	providerName string
	prefill      domain.CreateServerOpts

	step createStep
	opts domain.CreateServerOpts

	// Catalog data.
	data catalogData

	// Step: Name
	nameInput textinput.Model
	nameErr   string

	// Step: Location
	locations     []createItem
	locationIdx   int
	locationStart int

	// Step: Server Type
	serverTypes     []createItem
	serverTypeIdx   int
	serverTypeStart int

	// Step: Image
	images     []createItem
	imageIdx   int
	imageStart int

	// Step: SSH Keys
	sshKeys     []createItem
	sshSelected map[int]struct{}
	sshIdx      int
	sshStart    int

	// Step: Confirm
	confirmIdx int // 0 = create, 1 = cancel

	width  int
	height int

	loading bool
	spinner spinner.Model
	err     error

	result   *domain.CreateServerOpts
	quitting bool

	// embedded is true when this model is managed by serverAppModel.
	// When true, navigation actions emit messages instead of tea.Quit.
	embedded bool
}

// RunServerCreate starts the full-window server creation wizard.
func RunServerCreate(provider domain.CatalogProvider, providerName string, prefill domain.CreateServerOpts) (*domain.CreateServerOpts, error) {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(styles.Blue)

	ti := textinput.New()
	ti.Placeholder = "my-server"
	ti.Focus()
	ti.CharLimit = 63
	ti.Width = 40

	if prefill.Name != "" {
		ti.SetValue(prefill.Name)
	}

	m := serverCreateModel{
		provider:     provider,
		providerName: providerName,
		prefill:      prefill,
		step:         stepLoading,
		opts:         prefill,
		nameInput:    ti,
		loading:      true,
		spinner:      s,
		sshSelected:  make(map[int]struct{}),
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to run server create: %w", err)
	}

	final := result.(serverCreateModel)
	if final.quitting || final.result == nil {
		return nil, ErrAborted
	}
	return final.result, nil
}

func (m serverCreateModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		textinput.Blink,
		m.fetchCatalog(),
	)
}

func (m serverCreateModel) fetchCatalog() tea.Cmd {
	return func() tea.Msg {
		data, err := fetchCatalog(context.Background(), m.provider)
		if err != nil {
			return catalogErrorMsg{err: err}
		}
		return catalogLoadedMsg{data: data}
	}
}

func (m serverCreateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case catalogLoadedMsg:
		m.loading = false
		m.data = msg.data
		m.err = nil
		m.buildCatalogItems()
		m.step = stepName
		return m, textinput.Blink

	case catalogErrorMsg:
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

	// Forward to text input when on name step.
	if m.step == stepName {
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *serverCreateModel) buildCatalogItems() {
	// Locations.
	m.locations = make([]createItem, 0, len(m.data.locations))
	for _, loc := range m.data.locations {
		value := valueOrID(loc.Name, loc.ID)
		m.locations = append(m.locations, createItem{
			name:  value,
			label: locationLabel(loc),
		})
	}

	// Pre-select prefilled location.
	if m.prefill.Location != "" {
		for i, loc := range m.locations {
			if strings.EqualFold(loc.name, m.prefill.Location) {
				m.locationIdx = i
				break
			}
		}
	}

	m.rebuildServerTypes()
	m.rebuildImages()

	// SSH keys.
	m.sshKeys = make([]createItem, 0, len(m.data.sshKeys))
	for _, key := range m.data.sshKeys {
		value := valueOrID(key.Name, key.ID)
		m.sshKeys = append(m.sshKeys, createItem{
			name:  value,
			label: sshKeyLabel(key),
		})
	}

	// Pre-select prefilled SSH keys.
	for i, key := range m.sshKeys {
		for _, prefillKey := range m.prefill.SSHKeyIdentifiers {
			if strings.EqualFold(key.name, prefillKey) {
				m.sshSelected[i] = struct{}{}
			}
		}
	}
}

func (m *serverCreateModel) rebuildServerTypes() {
	location := ""
	if m.locationIdx < len(m.locations) {
		location = m.locations[m.locationIdx].name
	}

	filtered := filterServerTypesByLocation(m.data.serverTypes, location)
	m.serverTypes = make([]createItem, 0, len(filtered))
	for _, st := range filtered {
		value := valueOrID(st.Name, st.ID)
		m.serverTypes = append(m.serverTypes, createItem{
			name:  value,
			label: serverTypeLabel(st),
		})
	}
	m.serverTypeIdx = 0
	m.serverTypeStart = 0

	// Re-select prefilled type if valid.
	if m.prefill.ServerType != "" {
		for i, st := range m.serverTypes {
			if strings.EqualFold(st.name, m.prefill.ServerType) {
				m.serverTypeIdx = i
				break
			}
		}
	}
}

func (m *serverCreateModel) rebuildImages() {
	arch := ""
	if m.serverTypeIdx < len(m.serverTypes) {
		stName := m.serverTypes[m.serverTypeIdx].name
		for _, st := range m.data.serverTypes {
			if strings.EqualFold(valueOrID(st.Name, st.ID), stName) {
				arch = st.Architecture
				break
			}
		}
	}

	filtered := filterImages(m.data.images, arch)
	m.images = make([]createItem, 0, len(filtered))
	for _, img := range filtered {
		value := valueOrID(img.Name, img.ID)
		m.images = append(m.images, createItem{
			name:  value,
			label: imageLabel(img),
		})
	}
	m.imageIdx = 0
	m.imageStart = 0

	// Re-select prefilled image if valid.
	if m.prefill.Image != "" {
		for i, img := range m.images {
			if strings.EqualFold(img.name, m.prefill.Image) {
				m.imageIdx = i
				break
			}
		}
	}
}

// listCursor returns the current cursor index for the active list step.
func (m serverCreateModel) listCursor() int {
	switch m.step {
	case stepLocation:
		return m.locationIdx
	case stepServerType:
		return m.serverTypeIdx
	case stepImage:
		return m.imageIdx
	default:
		return 0
	}
}

// listItems returns the items for the active list step.
func (m serverCreateModel) listItems() []createItem {
	switch m.step {
	case stepLocation:
		return m.locations
	case stepServerType:
		return m.serverTypes
	case stepImage:
		return m.images
	default:
		return nil
	}
}

// setListCursor updates the cursor for the active list step.
func (m *serverCreateModel) setListCursor(idx int) {
	switch m.step {
	case stepLocation:
		m.locationIdx = idx
	case stepServerType:
		m.serverTypeIdx = idx
	case stepImage:
		m.imageIdx = idx
	}
}

func (m serverCreateModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global quit.
	if msg.String() == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
	}

	if m.loading {
		return m, nil
	}

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

	switch m.step {
	case stepName:
		return m.handleNameKey(msg)
	case stepLocation, stepServerType, stepImage:
		return m.handleListKey(msg)
	case stepSSHKeys:
		return m.handleSSHKeysKey(msg)
	case stepConfirm:
		return m.handleConfirmKey(msg)
	}

	return m, nil
}

func (m serverCreateModel) handleNameKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.embedded {
			return m, func() tea.Msg { return navigateBackMsg{} }
		}
		m.quitting = true
		return m, tea.Quit
	case "enter":
		name := strings.TrimSpace(m.nameInput.Value())
		if name == "" {
			m.nameErr = "Name is required"
			return m, nil
		}
		if err := util.ValidateServerName(name); err != nil {
			m.nameErr = err.Error()
			return m, nil
		}
		m.nameErr = ""
		m.opts.Name = name
		m.step = stepLocation
		return m, nil
	}

	// Forward to text input.
	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)
	m.nameErr = "" // Clear error on typing.
	return m, cmd
}

func (m serverCreateModel) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.listItems()
	cursor := m.listCursor()

	// Step navigation mapping.
	var prevStep, nextStep createStep
	switch m.step {
	case stepLocation:
		prevStep, nextStep = stepName, stepServerType
	case stepServerType:
		prevStep, nextStep = stepLocation, stepImage
	case stepImage:
		prevStep, nextStep = stepServerType, stepSSHKeys
	}

	switch msg.String() {
	case "esc":
		m.step = prevStep
		if prevStep == stepName {
			m.nameInput.Focus()
			return m, textinput.Blink
		}
		return m, nil
	case "q":
		if m.embedded {
			return m, func() tea.Msg { return navigateBackMsg{} }
		}
		m.quitting = true
		return m, tea.Quit
	case "up", "k":
		if cursor > 0 {
			m.setListCursor(cursor - 1)
		}
	case "down", "j":
		if cursor < len(items)-1 {
			m.setListCursor(cursor + 1)
		}
	case "g":
		m.setListCursor(0)
	case "G":
		if len(items) > 0 {
			m.setListCursor(len(items) - 1)
		}
	case "enter":
		if len(items) > 0 {
			selected := items[cursor].name
			switch m.step {
			case stepLocation:
				m.opts.Location = selected
				m.locationIdx = cursor
				m.rebuildServerTypes()
			case stepServerType:
				m.opts.ServerType = selected
				m.serverTypeIdx = cursor
				m.rebuildImages()
			case stepImage:
				m.opts.Image = selected
			}
			m.step = nextStep
			return m, nil
		}
	}

	return m, nil
}

func (m serverCreateModel) handleSSHKeysKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.step = stepImage
		return m, nil
	case "q":
		if m.embedded {
			return m, func() tea.Msg { return navigateBackMsg{} }
		}
		m.quitting = true
		return m, tea.Quit
	case "up", "k":
		if m.sshIdx > 0 {
			m.sshIdx--
		}
	case "down", "j":
		if m.sshIdx < len(m.sshKeys)-1 {
			m.sshIdx++
		}
	case " ":
		// Toggle selection.
		if _, ok := m.sshSelected[m.sshIdx]; ok {
			delete(m.sshSelected, m.sshIdx)
		} else {
			m.sshSelected[m.sshIdx] = struct{}{}
		}
	case "enter":
		// Collect selected keys.
		m.opts.SSHKeyIdentifiers = nil
		for i := range m.sshKeys {
			if _, ok := m.sshSelected[i]; ok {
				m.opts.SSHKeyIdentifiers = append(m.opts.SSHKeyIdentifiers, m.sshKeys[i].name)
			}
		}
		m.confirmIdx = 0
		m.step = stepConfirm
		return m, nil
	}

	return m, nil
}

func (m serverCreateModel) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if len(m.sshKeys) > 0 {
			m.step = stepSSHKeys
		} else {
			m.step = stepImage
		}
		return m, nil
	case "q":
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
			// Create!
			opts := m.opts
			opts.Name = strings.TrimSpace(opts.Name)
			if len(opts.SSHKeyIdentifiers) == 0 {
				opts.SSHKeyIdentifiers = nil
			}
			if m.embedded {
				return m, func() tea.Msg { return createConfirmedMsg{opts: opts} }
			}
			m.result = &opts
			return m, tea.Quit
		}
		// Cancel.
		if m.embedded {
			return m, func() tea.Msg { return navigateBackMsg{} }
		}
		m.quitting = true
		return m, tea.Quit
	case "y":
		opts := m.opts
		opts.Name = strings.TrimSpace(opts.Name)
		if len(opts.SSHKeyIdentifiers) == 0 {
			opts.SSHKeyIdentifiers = nil
		}
		if m.embedded {
			return m, func() tea.Msg { return createConfirmedMsg{opts: opts} }
		}
		m.result = &opts
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

func (m serverCreateModel) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	header := components.Header(m.width, "server create", m.providerName)

	var footerBindings []components.KeyBinding
	switch m.step {
	case stepLoading:
		footerBindings = []components.KeyBinding{{Key: "ctrl+c", Desc: "quit"}}
	case stepName:
		footerBindings = []components.KeyBinding{
			{Key: "enter", Desc: "next"},
			{Key: "esc", Desc: "cancel"},
		}
	case stepSSHKeys:
		footerBindings = []components.KeyBinding{
			{Key: "space", Desc: "toggle"},
			{Key: "enter", Desc: "next"},
			{Key: "esc", Desc: "back"},
		}
	case stepConfirm:
		footerBindings = []components.KeyBinding{
			{Key: "y/n", Desc: "confirm"},
			{Key: "enter", Desc: "select"},
			{Key: "esc", Desc: "back"},
		}
	default:
		footerBindings = []components.KeyBinding{
			{Key: "j/k", Desc: "navigate"},
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

func (m serverCreateModel) renderContent(height int) string {
	if m.loading {
		loadingText := m.spinner.View() + "  Fetching server options..."
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

	// Progress indicator.
	progress := m.renderProgress()

	// Step content.
	var stepContent string
	switch m.step {
	case stepName:
		stepContent = m.renderNameStep()
	case stepLocation:
		stepContent = m.renderListStep("Select a location", m.locations, m.locationIdx, m.locationStart, height-6)
	case stepServerType:
		stepContent = m.renderListStep("Select a server type", m.serverTypes, m.serverTypeIdx, m.serverTypeStart, height-6)
	case stepImage:
		stepContent = m.renderListStep("Select an image", m.images, m.imageIdx, m.imageStart, height-6)
	case stepSSHKeys:
		stepContent = m.renderSSHKeysStep(height - 6)
	case stepConfirm:
		stepContent = m.renderConfirmStep()
	}

	combined := lipgloss.JoinVertical(lipgloss.Center, progress, "", stepContent)

	return lipgloss.Place(
		m.width, height,
		lipgloss.Center, lipgloss.Center,
		combined,
	)
}

func (m serverCreateModel) renderProgress() string {
	allSteps := []createStep{stepName, stepLocation, stepServerType, stepImage, stepSSHKeys, stepConfirm}
	if len(m.sshKeys) == 0 {
		// Remove SSH keys step if none available.
		allSteps = []createStep{stepName, stepLocation, stepServerType, stepImage, stepConfirm}
	}

	parts := make([]string, len(allSteps))
	for i, s := range allSteps {
		label := s.label()
		if s == m.step {
			parts[i] = styles.AccentText.Bold(true).Render(label)
		} else if s < m.step {
			parts[i] = styles.SuccessText.Render(label)
		} else {
			parts[i] = styles.MutedText.Render(label)
		}
	}

	sep := styles.MutedText.Render(" > ")
	return strings.Join(parts, sep)
}

func (m serverCreateModel) renderNameStep() string {
	title := styles.Title.Render("Server name")
	hint := styles.MutedText.Render("Enter a valid hostname for your server")

	inputView := m.nameInput.View()

	var errLine string
	if m.nameErr != "" {
		errLine = "\n" + styles.ErrorText.Render(m.nameErr)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		hint,
		"",
		inputView,
		errLine,
	)
}

func (m serverCreateModel) renderListStep(title string, items []createItem, cursor int, start int, maxVisible int) string {
	if maxVisible < 3 {
		maxVisible = 3
	}

	titleLine := styles.Title.Render(title)

	// Compute scroll window from cursor position.
	if cursor < start {
		start = cursor
	}
	if cursor >= start+maxVisible {
		start = cursor - maxVisible + 1
	}

	end := min(start+maxVisible, len(items))

	rows := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		item := items[i]
		prefix := "  "
		if i == cursor {
			prefix = styles.AccentText.Render("> ")
		}

		label := item.label
		if i == cursor {
			label = styles.Value.Bold(true).Render(label)
		} else {
			label = styles.MutedText.Render(label)
		}

		rows = append(rows, prefix+label)
	}

	// Scroll indicators.
	if start > 0 {
		rows = append([]string{styles.MutedText.Render("  ... " + fmt.Sprintf("%d more above", start))}, rows...)
	}
	remaining := len(items) - end
	if remaining > 0 {
		rows = append(rows, styles.MutedText.Render("  ... "+fmt.Sprintf("%d more below", remaining)))
	}

	listContent := strings.Join(rows, "\n")

	return lipgloss.JoinVertical(lipgloss.Left,
		titleLine,
		"",
		listContent,
	)
}

func (m serverCreateModel) renderSSHKeysStep(maxVisible int) string {
	if len(m.sshKeys) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left,
			styles.Title.Render("SSH Keys"),
			"",
			styles.MutedText.Render("No SSH keys found for this account."),
			styles.MutedText.Render("Press enter to continue without SSH keys."),
		)
	}

	title := styles.Title.Render("SSH Keys")
	hint := styles.MutedText.Render("Press space to toggle, enter to continue")

	if maxVisible < 3 {
		maxVisible = 3
	}

	// Scrolling.
	if m.sshIdx < m.sshStart {
		m.sshStart = m.sshIdx
	}
	if m.sshIdx >= m.sshStart+maxVisible {
		m.sshStart = m.sshIdx - maxVisible + 1
	}

	end := min(m.sshStart+maxVisible, len(m.sshKeys))

	rows := make([]string, 0, end-m.sshStart)
	for i := m.sshStart; i < end; i++ {
		item := m.sshKeys[i]

		// Checkbox.
		check := "[ ]"
		if _, ok := m.sshSelected[i]; ok {
			check = styles.SuccessText.Render("[x]")
		}

		prefix := "  "
		if i == m.sshIdx {
			prefix = styles.AccentText.Render("> ")
		}

		label := item.label
		if i == m.sshIdx {
			label = styles.Value.Render(label)
		} else {
			label = styles.MutedText.Render(label)
		}

		rows = append(rows, prefix+check+" "+label)
	}

	listContent := strings.Join(rows, "\n")

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		hint,
		"",
		listContent,
	)
}

func (m serverCreateModel) renderConfirmStep() string {
	title := styles.Title.Render("Review & Confirm")

	// Summary card.
	cardWidth := 56

	labelWidth := 14
	renderField := func(label, value string) string {
		l := styles.Label.Width(labelWidth).Render(label)
		v := styles.Value.Render(value)
		return l + v
	}

	location := m.opts.Location
	if location == "" {
		location = "(auto)"
	}

	fields := []string{
		renderField("Name", m.opts.Name),
		renderField("Location", m.findLabel(m.locations, location)),
		renderField("Server type", m.findLabel(m.serverTypes, m.opts.ServerType)),
		renderField("Image", m.findLabel(m.images, m.opts.Image)),
	}

	if len(m.opts.SSHKeyIdentifiers) > 0 {
		keyLabels := make([]string, len(m.opts.SSHKeyIdentifiers))
		for i, k := range m.opts.SSHKeyIdentifiers {
			keyLabels[i] = m.findLabel(m.sshKeys, k)
		}
		fields = append(fields, renderField("SSH keys", strings.Join(keyLabels, ", ")))
	} else {
		fields = append(fields, renderField("SSH keys", "None"))
	}

	if labels := formatLabels(m.opts.Labels); labels != "" {
		fields = append(fields, renderField("Labels", labels))
	}
	if m.opts.UserData != "" {
		fields = append(fields, renderField("User data", fmt.Sprintf("%d bytes", len(m.opts.UserData))))
	}

	summaryContent := strings.Join(fields, "\n")
	summary := styles.Card.Width(cardWidth).Render(summaryContent)

	// Confirm/Cancel buttons.
	createBtn := "  Create  "
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

func (m serverCreateModel) findLabel(items []createItem, name string) string {
	for _, item := range items {
		if strings.EqualFold(item.name, name) {
			return item.label
		}
	}
	return name
}
