package tui

import (
	"context"
	"fmt"
	"strings"

	"nathanbeddoewebdev/vpsm/internal/dns/domain"
	"nathanbeddoewebdev/vpsm/internal/dns/services"
	"nathanbeddoewebdev/vpsm/internal/tui/components"
	"nathanbeddoewebdev/vpsm/internal/tui/styles"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Messages ---

type dnsRecordsLoadedMsg struct {
	records []domain.Record
}

type dnsRecordsErrorMsg struct {
	err error
}

// --- Record list model ---

type dnsRecordListModel struct {
	service      *services.Service
	providerName string
	domain       string

	records   []domain.Record
	filtered  []domain.Record
	cursor    int
	listStart int // for scrolling

	typeFilter string // e.g. "A", "CNAME", "" for all
	typeTypes  []string

	width  int
	height int

	loading          bool
	spinner          spinner.Model
	err              error
	status           string
	statusIsError    bool
	persistentStatus string

	embedded bool
}

func newDNSRecordListModel(svc *services.Service, providerName, domainName string, embedded bool, width, height int) dnsRecordListModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(styles.Blue)

	return dnsRecordListModel{
		service:      svc,
		providerName: providerName,
		domain:       domainName,
		typeTypes:    []string{"", "A", "AAAA", "CNAME", "MX", "TXT"},
		typeFilter:   "",
		embedded:     embedded,
		width:        width,
		height:       height,
		loading:      true,
		spinner:      s,
	}
}

func (m dnsRecordListModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.loadRecordsCmd())
}

func (m dnsRecordListModel) loadRecordsCmd() tea.Cmd {
	return func() tea.Msg {
		records, err := m.service.ListRecords(context.Background(), m.domain)
		if err != nil {
			return dnsRecordsErrorMsg{err}
		}
		return dnsRecordsLoadedMsg{records}
	}
}

func (m *dnsRecordListModel) applyFilter() {
	m.filtered = make([]domain.Record, 0)
	for _, r := range m.records {
		if m.typeFilter == "" || strings.EqualFold(string(r.Type), m.typeFilter) {
			m.filtered = append(m.filtered, r)
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
		if m.cursor < 0 {
			m.cursor = 0
		}
	}
	m.updateScroll()
}

func (m *dnsRecordListModel) updateScroll() {
	headerH, footerH, statusH := 3, 1, 1 // approximate
	contentH := max(m.height-headerH-footerH-statusH, 1)
	filterBarH := 1
	tableH := max(contentH-filterBarH-1, 1)
	visibleRows := max(tableH-3, 1)

	if m.cursor < m.listStart {
		m.listStart = m.cursor
	} else if m.cursor >= m.listStart+visibleRows {
		m.listStart = m.cursor - visibleRows + 1
	}
}

func (m dnsRecordListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		if m.loading {
			if msg.String() == "ctrl+c" {
				if !m.embedded {
					return m, tea.Quit
				}
			}
			return m, nil
		}

		switch msg.String() {
		case "esc", "backspace":
			if m.embedded {
				return m, func() tea.Msg { return dnsNavigateBackMsg{} }
			}
			return m, tea.Quit
		case "ctrl+c", "q":
			if !m.embedded {
				return m, tea.Quit
			}
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			m.updateScroll()
		case "down", "j":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
			m.updateScroll()
		case "g":
			m.cursor = 0
			m.updateScroll()
		case "G":
			if len(m.filtered) > 0 {
				m.cursor = len(m.filtered) - 1
			}
			m.updateScroll()
		case "f":
			idx := 0
			for i, t := range m.typeTypes {
				if t == m.typeFilter {
					idx = i
					break
				}
			}
			idx = (idx + 1) % len(m.typeTypes)
			m.typeFilter = m.typeTypes[idx]
			m.applyFilter()
		case "r":
			m.loading = true
			m.err = nil
			return m, m.loadRecordsCmd()
		case "enter":
			if len(m.filtered) > 0 {
				rec := m.filtered[m.cursor]
				if m.embedded {
					return m, func() tea.Msg { return dnsNavigateToRecordShowMsg{record: rec, domain: m.domain} }
				}
			}
		case "c":
			if m.embedded {
				return m, func() tea.Msg { return dnsNavigateToRecordCreateMsg{domain: m.domain} }
			}
		case "d":
			if len(m.filtered) > 0 {
				rec := m.filtered[m.cursor]
				if m.embedded {
					return m, func() tea.Msg { return dnsNavigateToRecordDeleteMsg{record: rec, domain: m.domain} }
				}
			}
		case "e":
			if len(m.filtered) > 0 {
				rec := m.filtered[m.cursor]
				if m.embedded {
					return m, func() tea.Msg { return dnsNavigateToRecordEditMsg{record: rec, domain: m.domain} }
				}
			}
		}

	case dnsRecordsLoadedMsg:
		m.loading = false
		m.records = msg.records
		m.applyFilter()

		status := fmt.Sprintf("%d record(s)", len(m.records))
		if m.persistentStatus != "" {
			status = m.persistentStatus + " | " + status
		}
		m.status = status

	case dnsRecordsErrorMsg:
		m.loading = false
		m.err = msg.err
		m.status = msg.err.Error()
		m.statusIsError = true

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m dnsRecordListModel) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	header := components.Header(m.width, "dns > "+m.domain, m.providerName)

	var footerBindings []components.KeyBinding
	if m.loading {
		footerBindings = []components.KeyBinding{
			{Key: "ctrl+c", Desc: "quit"},
		}
	} else {
		footerBindings = []components.KeyBinding{
			{Key: "j/k", Desc: "nav"},
			{Key: "enter", Desc: "show"},
			{Key: "c", Desc: "create"},
			{Key: "d", Desc: "delete"},
			{Key: "e", Desc: "edit"},
			{Key: "f", Desc: "filter"},
			{Key: "esc", Desc: "back"},
		}
		if !m.embedded {
			footerBindings = append(footerBindings, components.KeyBinding{Key: "q", Desc: "quit"})
		}
	}
	footer := components.Footer(m.width, footerBindings)

	statusBar := ""
	if m.err != nil {
		statusBar = components.StatusBar(m.width, "Error: "+m.err.Error(), true)
	} else if m.status != "" {
		statusBar = components.StatusBar(m.width, m.status, m.statusIsError)
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

func (m dnsRecordListModel) renderContent(height int) string {
	if m.loading {
		loadingText := m.spinner.View() + "  Fetching records…"
		return lipgloss.Place(
			m.width, height,
			lipgloss.Center, lipgloss.Center,
			styles.MutedText.Render(loadingText),
		)
	}

	if m.err != nil {
		return lipgloss.Place(
			m.width, height,
			lipgloss.Center, lipgloss.Center,
			styles.ErrorText.Render(fmt.Sprintf("Error: %v", m.err)),
		)
	}

	if len(m.records) == 0 {
		return lipgloss.Place(
			m.width, height,
			lipgloss.Center, lipgloss.Center,
			styles.MutedText.Render("No records found for this domain."),
		)
	}

	filterBar := m.renderFilterBar()
	tableH := max(height-lipgloss.Height(filterBar)-1, 1) // -1 for margin
	table := m.renderTable(tableH)

	// Create table + filter with space, pad remaining if needed
	content := lipgloss.JoinVertical(lipgloss.Left, filterBar, "", table)

	contentLines := strings.Split(content, "\n")
	if len(contentLines) < height {
		padding := strings.Repeat("\n", height-len(contentLines))
		content += padding
	}

	return content
}

func (m dnsRecordListModel) renderFilterBar() string {
	var parts []string
	parts = append(parts, "  Filter: ")

	for _, t := range m.typeTypes {
		label := t
		if t == "" {
			label = "All"
		}

		if t == m.typeFilter {
			parts = append(parts, fmt.Sprintf("[%s]", styles.AccentText.Render(label)))
		} else {
			parts = append(parts, fmt.Sprintf(" %s ", styles.MutedText.Render(label)))
		}
	}

	return strings.Join(parts, "")
}

func (m dnsRecordListModel) renderTable(height int) string {
	if len(m.filtered) == 0 {
		return lipgloss.Place(
			m.width, height,
			lipgloss.Center, lipgloss.Top,
			styles.MutedText.Render("\nNo records match the current filter."),
		)
	}

	type column struct {
		title string
		width int
	}

	available := m.width - 4

	cols := []column{
		{title: "NAME", width: 20},
		{title: "TYPE", width: 8},
		{title: "CONTENT", width: 30},
		{title: "TTL", width: 8},
	}

	// Distribute remaining width to the CONTENT column
	total := 0
	for _, c := range cols {
		total += c.width
	}
	if available > total {
		extra := available - total
		for i := range cols {
			if cols[i].title == "CONTENT" {
				cols[i].width += extra
				break
			}
		}
	}

	headerCells := make([]string, len(cols))
	for i, col := range cols {
		headerCells[i] = styles.TableHeader.
			Width(col.width).
			Render(col.title)
	}
	headerRow := lipgloss.JoinHorizontal(lipgloss.Top, headerCells...)

	sep := styles.MutedText.Render(strings.Repeat("─", available))

	visibleRows := max(height-3, 1)

	end := m.listStart + visibleRows
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	var rows []string
	rows = append(rows, headerRow, sep)

	for i := m.listStart; i < end; i++ {
		r := m.filtered[i]

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

		contentStr := r.Content
		if len(contentStr) > cols[2].width-2 {
			contentStr = contentStr[:cols[2].width-5] + "..."
		}

		cells := []string{
			lipgloss.NewStyle().Width(cols[0].width).Render(r.Name),
			lipgloss.NewStyle().Width(cols[1].width).Render(typeColor.Render(string(r.Type))),
			lipgloss.NewStyle().Width(cols[2].width).Render(contentStr),
			lipgloss.NewStyle().Width(cols[3].width).Render(fmt.Sprintf("%d", r.TTL)),
		}

		rowContent := lipgloss.JoinHorizontal(lipgloss.Top, cells...)

		cursor := "  "
		rowStyle := styles.TableCell
		if i == m.cursor {
			cursor = styles.AccentText.Render("> ")
			rowStyle = styles.TableSelectedRow
		}

		renderedRow := lipgloss.JoinHorizontal(lipgloss.Top, cursor, rowStyle.Render(rowContent))
		rows = append(rows, renderedRow)
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}
