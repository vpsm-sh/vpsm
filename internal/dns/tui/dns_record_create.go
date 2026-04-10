package tui

import (
	"fmt"
	"strconv"
	"strings"

	"nathanbeddoewebdev/vpsm/internal/dns/domain"
	"nathanbeddoewebdev/vpsm/internal/dns/services"
	"nathanbeddoewebdev/vpsm/internal/tui/components"
	"nathanbeddoewebdev/vpsm/internal/tui/styles"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type dnsCreateStep int

const (
	dnsCreateStepType dnsCreateStep = iota
	dnsCreateStepName
	dnsCreateStepContent
	dnsCreateStepTTL
	dnsCreateStepPriority
	dnsCreateStepNotes
	dnsCreateStepConfirm
)

type dnsRecordCreateModel struct {
	service      *services.Service
	providerName string
	domain       string
	opts         domain.CreateRecordOpts

	step dnsCreateStep

	// Type selection
	types  []domain.RecordType
	cursor int

	// Text inputs
	inputs map[dnsCreateStep]textinput.Model

	embedded bool
	width    int
	height   int
}

func newDNSRecordCreateModel(svc *services.Service, providerName, domainName string, prefill domain.CreateRecordOpts, embedded bool, width, height int) dnsRecordCreateModel {
	types := []domain.RecordType{"A", "AAAA", "CNAME", "MX", "TXT", "NS", "SRV", "CAA", "ALIAS"}

	inputs := make(map[dnsCreateStep]textinput.Model)

	// Name input
	nameIn := textinput.New()
	nameIn.Placeholder = "e.g. www (leave empty for root)"
	nameIn.SetValue(prefill.Name)
	nameIn.Width = 40
	inputs[dnsCreateStepName] = nameIn

	// Content input
	contentIn := textinput.New()
	contentIn.Placeholder = "e.g. 1.2.3.4"
	contentIn.SetValue(prefill.Content)
	contentIn.Width = 40
	inputs[dnsCreateStepContent] = contentIn

	// TTL input
	ttlIn := textinput.New()
	ttlIn.Placeholder = "e.g. 600"
	if prefill.TTL > 0 {
		ttlIn.SetValue(strconv.Itoa(prefill.TTL))
	}
	ttlIn.Width = 40
	inputs[dnsCreateStepTTL] = ttlIn

	// Priority input
	prioIn := textinput.New()
	prioIn.Placeholder = "e.g. 10"
	if prefill.Priority > 0 {
		prioIn.SetValue(strconv.Itoa(prefill.Priority))
	}
	prioIn.Width = 40
	inputs[dnsCreateStepPriority] = prioIn

	// Notes input
	notesIn := textinput.New()
	notesIn.Placeholder = "Optional notes"
	notesIn.SetValue(prefill.Notes)
	notesIn.Width = 40
	inputs[dnsCreateStepNotes] = notesIn

	// Preset the opts type
	opts := prefill
	if opts.Type == "" {
		opts.Type = "A" // default
	}

	cursor := 0
	for i, t := range types {
		if t == opts.Type {
			cursor = i
			break
		}
	}

	return dnsRecordCreateModel{
		service:      svc,
		providerName: providerName,
		domain:       domainName,
		opts:         opts,
		step:         dnsCreateStepType,
		types:        types,
		cursor:       cursor,
		inputs:       inputs,
		embedded:     embedded,
		width:        width,
		height:       height,
	}
}

func (m dnsRecordCreateModel) Init() tea.Cmd {
	return nil
}

func (m dnsRecordCreateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.step > dnsCreateStepType {
				m.step--
				// Skip priority if not MX or SRV
				if m.step == dnsCreateStepPriority && m.opts.Type != "MX" && m.opts.Type != "SRV" {
					m.step--
				}
				m.focusInput()
				return m, nil
			}
			if m.embedded {
				return m, func() tea.Msg { return dnsNavigateBackMsg{} }
			}
			return m, tea.Quit
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if m.step == dnsCreateStepType || m.step == dnsCreateStepConfirm {
				if m.embedded {
					return m, func() tea.Msg { return dnsNavigateBackMsg{} }
				}
				return m, tea.Quit
			}
		case "up", "k":
			if m.step == dnsCreateStepType && m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.step == dnsCreateStepType && m.cursor < len(m.types)-1 {
				m.cursor++
			}
		case "enter":
			// Process current step
			switch m.step {
			case dnsCreateStepType:
				m.opts.Type = m.types[m.cursor]
				m.updatePlaceholders()
				m.step++
				m.focusInput()
			case dnsCreateStepName:
				m.opts.Name = m.inputs[m.step].Value()
				m.step++
				m.focusInput()
			case dnsCreateStepContent:
				val := m.inputs[m.step].Value()
				if val == "" {
					return m, nil // Don't allow empty content
				}
				m.opts.Content = val
				m.step++
				m.focusInput()
			case dnsCreateStepTTL:
				val := m.inputs[m.step].Value()
				if val != "" {
					if ttl, err := strconv.Atoi(val); err == nil {
						m.opts.TTL = ttl
					}
				}
				m.step++
				if m.opts.Type != "MX" && m.opts.Type != "SRV" {
					m.step++ // skip priority
				}
				m.focusInput()
			case dnsCreateStepPriority:
				val := m.inputs[m.step].Value()
				if val != "" {
					if prio, err := strconv.Atoi(val); err == nil {
						m.opts.Priority = prio
					}
				}
				m.step++
				m.focusInput()
			case dnsCreateStepNotes:
				m.opts.Notes = m.inputs[m.step].Value()
				m.step++ // Go to confirm
			case dnsCreateStepConfirm:
				if m.embedded {
					return m, func() tea.Msg {
						return dnsCreateConfirmedMsg{domain: m.domain, opts: m.opts}
					}
				}
				return m, tea.Quit
			}
		}

	}

	// Route msg to current input if applicable
	if m.step >= dnsCreateStepName && m.step <= dnsCreateStepNotes {
		if in, ok := m.inputs[m.step]; ok {
			var cmd tea.Cmd
			in, cmd = in.Update(msg)
			m.inputs[m.step] = in
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *dnsRecordCreateModel) focusInput() {
	for k, in := range m.inputs {
		if k == m.step {
			in.Focus()
		} else {
			in.Blur()
		}
		m.inputs[k] = in
	}
}

func (m *dnsRecordCreateModel) updatePlaceholders() {
	in := m.inputs[dnsCreateStepContent]
	switch m.opts.Type {
	case "A":
		in.Placeholder = "e.g. 1.2.3.4"
	case "AAAA":
		in.Placeholder = "e.g. 2001:db8::1"
	case "CNAME":
		in.Placeholder = "e.g. example.com"
	case "MX":
		in.Placeholder = "e.g. mail.example.com"
	case "TXT":
		in.Placeholder = `e.g. "v=spf1 include:_spf.example.com ~all"`
	default:
		in.Placeholder = "Record content"
	}
	m.inputs[dnsCreateStepContent] = in
}

func (m dnsRecordCreateModel) View() string {
	header := components.Header(m.width, "dns > create", m.providerName)

	bindings := []components.KeyBinding{
		{Key: "esc", Desc: "back"},
	}
	if m.step == dnsCreateStepType {
		bindings = append(bindings, components.KeyBinding{Key: "j/k", Desc: "select"})
		bindings = append(bindings, components.KeyBinding{Key: "enter", Desc: "next"})
	} else if m.step == dnsCreateStepConfirm {
		bindings = append(bindings, components.KeyBinding{Key: "enter", Desc: "confirm"})
	} else {
		bindings = append(bindings, components.KeyBinding{Key: "enter", Desc: "next"})
	}

	footer := components.Footer(m.width, bindings)

	headerH := lipgloss.Height(header)
	footerH := lipgloss.Height(footer)
	contentH := m.height - headerH - footerH
	if contentH < 1 {
		contentH = 1
	}

	var content string
	switch m.step {
	case dnsCreateStepType:
		content = m.renderTypeStep()
	case dnsCreateStepName:
		content = m.renderInputStep("Subdomain Name")
	case dnsCreateStepContent:
		content = m.renderInputStep(fmt.Sprintf("%s Content", m.opts.Type))
	case dnsCreateStepTTL:
		content = m.renderInputStep("TTL (seconds)")
	case dnsCreateStepPriority:
		content = m.renderInputStep("Priority")
	case dnsCreateStepNotes:
		content = m.renderInputStep("Notes")
	case dnsCreateStepConfirm:
		content = m.renderConfirmStep()
	}

	content = m.renderStepper() + "\n\n" + content

	// Pad to height
	lines := lipgloss.Height(content)
	if lines < contentH {
		content += strings.Repeat("\n", contentH-lines)
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
}

func (m dnsRecordCreateModel) renderStepper() string {
	steps := []string{"Type", "Name", "Content", "TTL"}
	if m.opts.Type == "MX" || m.opts.Type == "SRV" {
		steps = append(steps, "Priority")
	}
	steps = append(steps, "Confirm")

	// Calculate which logical step we are on (accounting for skipped priority)
	logicalStep := int(m.step)
	if m.step >= dnsCreateStepPriority && m.opts.Type != "MX" && m.opts.Type != "SRV" {
		logicalStep--
	}
	if m.step >= dnsCreateStepNotes {
		// Notes doesn't get a dot
		logicalStep--
	}

	var parts []string
	for i, step := range steps {
		if i == logicalStep {
			parts = append(parts, styles.AccentText.Render("● "+step))
		} else if i < logicalStep {
			parts = append(parts, styles.SuccessText.Render("✓ ")+styles.MutedText.Render(step))
		} else {
			parts = append(parts, styles.MutedText.Render("○ "+step))
		}
	}
	return "  " + strings.Join(parts, "  ")
}

func (m dnsRecordCreateModel) renderTypeStep() string {
	var rows []string
	rows = append(rows, "  "+styles.Subtitle.Render("Select record type:"))
	rows = append(rows, "")

	for i, t := range m.types {
		cursor := " "
		style := styles.Value
		if i == m.cursor {
			cursor = styles.AccentText.Render(">")
			style = styles.AccentText
		}

		desc := ""
		switch t {
		case "A":
			desc = "IPv4 address"
		case "AAAA":
			desc = "IPv6 address"
		case "CNAME":
			desc = "Canonical name"
		case "MX":
			desc = "Mail exchange"
		case "TXT":
			desc = "Text record"
		case "NS":
			desc = "Name server"
		case "SRV":
			desc = "Service locator"
		case "CAA":
			desc = "Certificate authority"
		case "ALIAS":
			desc = "CNAME flattening"
		}

		row := fmt.Sprintf("  %s %-8s %s", cursor, style.Render(string(t)), styles.MutedText.Render(desc))
		rows = append(rows, row)
	}

	return strings.Join(rows, "\n")
}

func (m dnsRecordCreateModel) renderInputStep(title string) string {
	in := m.inputs[m.step]

	// Apply proper styling to the input
	in.PromptStyle = styles.AccentText
	in.TextStyle = styles.Value
	in.PlaceholderStyle = styles.MutedText

	return fmt.Sprintf("  %s\n\n  %s", styles.Subtitle.Render(title+":"), in.View())
}

func (m dnsRecordCreateModel) renderConfirmStep() string {
	title := styles.Title.Render("Create DNS Record")

	o := m.opts
	ttlStr := strconv.Itoa(o.TTL)
	if o.TTL == 0 {
		ttlStr = "Default"
	}

	name := o.Name
	if name == "" {
		name = "(Root)"
	}

	fields := []string{
		lipgloss.JoinHorizontal(lipgloss.Left, lipgloss.NewStyle().Width(10).Render(styles.Label.Render("Domain")), styles.Value.Render(m.domain)),
		lipgloss.JoinHorizontal(lipgloss.Left, lipgloss.NewStyle().Width(10).Render(styles.Label.Render("Type")), styles.Value.Render(string(o.Type))),
		lipgloss.JoinHorizontal(lipgloss.Left, lipgloss.NewStyle().Width(10).Render(styles.Label.Render("Name")), styles.Value.Render(name)),
		lipgloss.JoinHorizontal(lipgloss.Left, lipgloss.NewStyle().Width(10).Render(styles.Label.Render("Content")), styles.Value.Render(o.Content)),
		lipgloss.JoinHorizontal(lipgloss.Left, lipgloss.NewStyle().Width(10).Render(styles.Label.Render("TTL")), styles.Value.Render(ttlStr)),
	}

	if o.Priority > 0 {
		fields = append(fields, lipgloss.JoinHorizontal(lipgloss.Left, lipgloss.NewStyle().Width(10).Render(styles.Label.Render("Priority")), styles.Value.Render(strconv.Itoa(o.Priority))))
	}
	if o.Notes != "" {
		fields = append(fields, lipgloss.JoinHorizontal(lipgloss.Left, lipgloss.NewStyle().Width(10).Render(styles.Label.Render("Notes")), styles.Value.Render(o.Notes)))
	}

	cardContent := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		strings.Join(fields, "\n"),
	)

	return lipgloss.JoinVertical(lipgloss.Center,
		styles.CardActive.Render(cardContent),
		"",
		"  Press Enter to Create",
	)
}
