package tui

import (
	"fmt"
	"strconv"

	"nathanbeddoewebdev/vpsm/internal/dns/domain"
	"nathanbeddoewebdev/vpsm/internal/tui/components"
	"nathanbeddoewebdev/vpsm/internal/tui/styles"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type dnsRecordShowModel struct {
	record       domain.Record
	domain       string
	providerName string
	embedded     bool
	width        int
	height       int
}

func newDNSRecordShowModel(record domain.Record, domainName string, providerName string, embedded bool, width, height int) dnsRecordShowModel {
	return dnsRecordShowModel{
		record:       record,
		domain:       domainName,
		providerName: providerName,
		embedded:     embedded,
		width:        width,
		height:       height,
	}
}

func (m dnsRecordShowModel) Init() tea.Cmd {
	return nil
}

func (m dnsRecordShowModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "backspace", "left", "h":
			if m.embedded {
				return m, func() tea.Msg { return dnsNavigateBackMsg{} }
			}
			return m, tea.Quit
		case "q":
			if !m.embedded {
				return m, tea.Quit
			}
		case "e":
			if m.embedded {
				return m, func() tea.Msg { return dnsNavigateToRecordEditMsg{record: m.record, domain: m.domain} }
			}
		case "d":
			if m.embedded {
				return m, func() tea.Msg { return dnsNavigateToRecordDeleteMsg{record: m.record, domain: m.domain} }
			}
		}
	}

	return m, nil
}

func (m dnsRecordShowModel) View() string {
	breadcrumb := fmt.Sprintf("dns > %s > %s", m.domain, m.record.Name)
	header := components.Header(m.width, breadcrumb, m.providerName)

	bindings := []components.KeyBinding{
		{Key: "e", Desc: "edit"},
		{Key: "d", Desc: "delete"},
		{Key: "esc", Desc: "back"},
	}
	if !m.embedded {
		bindings = append(bindings, components.KeyBinding{Key: "q", Desc: "quit"})
	}
	footer := components.Footer(m.width, bindings)

	headerH := lipgloss.Height(header)
	footerH := lipgloss.Height(footer)
	contentH := m.height - headerH - footerH
	if contentH < 1 {
		contentH = 1
	}

	content := m.renderCard()

	// Center horizontally and vertically
	content = lipgloss.Place(m.width, contentH, lipgloss.Center, lipgloss.Center, content)

	return lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
}

func (m dnsRecordShowModel) renderCard() string {
	r := m.record

	// Header row: Name + Type badge
	typeColor := styles.Value
	switch r.Type {
	case "A", "AAAA":
		typeColor = lipgloss.NewStyle().Foreground(styles.Green)
	case "CNAME":
		typeColor = lipgloss.NewStyle().Foreground(styles.Yellow)
	case "MX":
		typeColor = lipgloss.NewStyle().Foreground(styles.Blue)
	case "TXT":
		typeColor = styles.MutedText
	}

	titleRow := lipgloss.JoinHorizontal(lipgloss.Center,
		styles.Title.Render(r.Name),
		"  ",
		typeColor.Render(string(r.Type)),
	)

	// Detail grid
	fields := []struct {
		label string
		val   string
	}{
		{"ID", r.ID},
		{"Name", r.Name},
		{"Type", string(r.Type)},
		{"Content", r.Content},
		{"TTL", strconv.Itoa(r.TTL)},
		{"Priority", "—"},
	}

	if r.Priority > 0 {
		fields[5].val = strconv.Itoa(r.Priority)
	}

	if r.Notes != "" {
		fields = append(fields, struct {
			label string
			val   string
		}{"Notes", r.Notes})
	}

	var gridRows []string
	gridRows = append(gridRows, titleRow, "") // Title + empty line

	for _, f := range fields {
		row := lipgloss.JoinHorizontal(lipgloss.Left,
			lipgloss.NewStyle().Width(12).Render(styles.Label.Render(f.label)),
			styles.Value.Render(f.val),
		)
		gridRows = append(gridRows, row)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, gridRows...)
	return styles.Card.Render(content)
}
