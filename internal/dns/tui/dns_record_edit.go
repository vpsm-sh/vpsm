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

type dnsRecordEditModel struct {
	service      *services.Service
	providerName string
	domain       string
	record       domain.Record
	opts         domain.UpdateRecordOpts

	step dnsCreateStep // Reuse the same steps as create

	// Text inputs
	inputs map[dnsCreateStep]textinput.Model

	embedded bool
	width    int
	height   int
}

func newDNSRecordEditModel(svc *services.Service, providerName, domainName string, record domain.Record, embedded bool, width, height int) dnsRecordEditModel {
	inputs := make(map[dnsCreateStep]textinput.Model)

	// Name input
	nameIn := textinput.New()
	nameIn.Placeholder = "e.g. www (leave empty for root)"
	nameIn.SetValue(record.Name)
	inputs[dnsCreateStepName] = nameIn

	// Content input
	contentIn := textinput.New()
	contentIn.Placeholder = "e.g. 1.2.3.4"
	contentIn.SetValue(record.Content)
	inputs[dnsCreateStepContent] = contentIn

	// TTL input
	ttlIn := textinput.New()
	ttlIn.Placeholder = "e.g. 600"
	if record.TTL > 0 {
		ttlIn.SetValue(strconv.Itoa(record.TTL))
	}
	inputs[dnsCreateStepTTL] = ttlIn

	// Priority input
	prioIn := textinput.New()
	prioIn.Placeholder = "e.g. 10"
	if record.Priority > 0 {
		prioIn.SetValue(strconv.Itoa(record.Priority))
	}
	inputs[dnsCreateStepPriority] = prioIn

	// Notes input
	notesIn := textinput.New()
	notesIn.Placeholder = "Optional notes"
	notesIn.SetValue(record.Notes)
	inputs[dnsCreateStepNotes] = notesIn

	opts := domain.UpdateRecordOpts{
		Type: record.Type,
	}

	m := dnsRecordEditModel{
		service:      svc,
		providerName: providerName,
		domain:       domainName,
		record:       record,
		opts:         opts,
		step:         dnsCreateStepName, // Skip type step
		inputs:       inputs,
		embedded:     embedded,
		width:        width,
		height:       height,
	}
	m.focusInput()
	return m
}

func (m dnsRecordEditModel) Init() tea.Cmd {
	return nil
}

func (m dnsRecordEditModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.step > dnsCreateStepName {
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
			if m.step == dnsCreateStepConfirm {
				if m.embedded {
					return m, func() tea.Msg { return dnsNavigateBackMsg{} }
				}
				return m, tea.Quit
			}
		case "enter":
			// Process current step
			switch m.step {
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
				val := m.inputs[m.step].Value()
				m.opts.Notes = &val
				m.step++ // Go to confirm
			case dnsCreateStepConfirm:
				if m.embedded {
					return m, func() tea.Msg {
						return dnsUpdateConfirmedMsg{domain: m.domain, id: m.record.ID, opts: m.opts}
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

func (m *dnsRecordEditModel) focusInput() {
	for k, in := range m.inputs {
		if k == m.step {
			in.Focus()
		} else {
			in.Blur()
		}
		m.inputs[k] = in
	}
}

func (m dnsRecordEditModel) View() string {
	header := components.Header(m.width, "dns > edit", m.providerName)

	bindings := []components.KeyBinding{
		{Key: "esc", Desc: "back"},
	}
	if m.step == dnsCreateStepConfirm {
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

func (m dnsRecordEditModel) renderStepper() string {
	steps := []string{"Name", "Content", "TTL"}
	if m.opts.Type == "MX" || m.opts.Type == "SRV" {
		steps = append(steps, "Priority")
	}
	steps = append(steps, "Confirm")

	logicalStep := int(m.step) - 1 // skip type
	if m.step >= dnsCreateStepPriority && m.opts.Type != "MX" && m.opts.Type != "SRV" {
		logicalStep--
	}
	if m.step >= dnsCreateStepNotes {
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

func (m dnsRecordEditModel) renderInputStep(title string) string {
	in := m.inputs[m.step]

	in.PromptStyle = styles.AccentText
	in.TextStyle = styles.Value
	in.PlaceholderStyle = styles.MutedText

	return fmt.Sprintf("  %s\n\n  %s", styles.Subtitle.Render(title+":"), in.View())
}

func (m dnsRecordEditModel) renderConfirmStep() string {
	title := styles.Title.Render("Update DNS Record")

	o := m.opts
	r := m.record

	ttlStr := strconv.Itoa(o.TTL)
	if o.TTL == 0 {
		ttlStr = "Default"
	}

	name := o.Name
	if name == "" {
		name = "(Root)"
	}

	// Helper to highlight changed fields
	val := func(old string, newStr string) string {
		if old != newStr && newStr != "" {
			return styles.AccentText.Render(newStr)
		}
		return styles.Value.Render(old)
	}

	notesStr := r.Notes
	if o.Notes != nil {
		notesStr = *o.Notes
	}

	fields := []string{
		lipgloss.JoinHorizontal(lipgloss.Left, lipgloss.NewStyle().Width(10).Render(styles.Label.Render("Domain")), styles.Value.Render(m.domain)),
		lipgloss.JoinHorizontal(lipgloss.Left, lipgloss.NewStyle().Width(10).Render(styles.Label.Render("Type")), styles.Value.Render(string(o.Type))),
		lipgloss.JoinHorizontal(lipgloss.Left, lipgloss.NewStyle().Width(10).Render(styles.Label.Render("Name")), val(r.Name, name)),
		lipgloss.JoinHorizontal(lipgloss.Left, lipgloss.NewStyle().Width(10).Render(styles.Label.Render("Content")), val(r.Content, o.Content)),
		lipgloss.JoinHorizontal(lipgloss.Left, lipgloss.NewStyle().Width(10).Render(styles.Label.Render("TTL")), val(strconv.Itoa(r.TTL), ttlStr)),
	}

	if r.Priority > 0 || o.Priority > 0 {
		fields = append(fields, lipgloss.JoinHorizontal(lipgloss.Left, lipgloss.NewStyle().Width(10).Render(styles.Label.Render("Priority")), val(strconv.Itoa(r.Priority), strconv.Itoa(o.Priority))))
	}
	if r.Notes != "" || (o.Notes != nil && *o.Notes != "") {
		fields = append(fields, lipgloss.JoinHorizontal(lipgloss.Left, lipgloss.NewStyle().Width(10).Render(styles.Label.Render("Notes")), val(r.Notes, notesStr)))
	}

	cardContent := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		strings.Join(fields, "\n"),
	)

	return lipgloss.JoinVertical(lipgloss.Center,
		styles.CardActive.Render(cardContent),
		"",
		"  Press Enter to Update",
	)
}
