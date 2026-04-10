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

type dnsDomainsLoadedMsg struct {
	domains []domain.Domain
}

type dnsDomainsErrorMsg struct {
	err error
}

// --- Domain list model ---

type dnsDomainListModel struct {
	service      *services.Service
	providerName string

	domains   []domain.Domain
	cursor    int
	listStart int // for scrolling

	width  int
	height int

	loading       bool
	spinner       spinner.Model
	err           error
	status        string
	statusIsError bool

	embedded bool
}

func newDNSDomainListModel(svc *services.Service, providerName string, embedded bool, width, height int) dnsDomainListModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(styles.Blue)

	return dnsDomainListModel{
		service:      svc,
		providerName: providerName,
		embedded:     embedded,
		width:        width,
		height:       height,
		loading:      true,
		spinner:      s,
	}
}

func (m dnsDomainListModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.loadDomainsCmd())
}

func (m dnsDomainListModel) loadDomainsCmd() tea.Cmd {
	return func() tea.Msg {
		domains, err := m.service.ListDomains(context.Background())
		if err != nil {
			return dnsDomainsErrorMsg{err}
		}
		return dnsDomainsLoadedMsg{domains}
	}
}

func (m *dnsDomainListModel) updateScroll() {
	headerH, footerH, statusH := 3, 1, 1 // approximate
	contentH := max(m.height-headerH-footerH-statusH, 1)
	visibleRows := max(contentH-3, 1)

	if m.cursor < m.listStart {
		m.listStart = m.cursor
	} else if m.cursor >= m.listStart+visibleRows {
		m.listStart = m.cursor - visibleRows + 1
	}
}

func (m dnsDomainListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		case "ctrl+c", "q", "esc":
			if !m.embedded {
				return m, tea.Quit
			}
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			m.updateScroll()
		case "down", "j":
			if m.cursor < len(m.domains)-1 {
				m.cursor++
			}
			m.updateScroll()
		case "g":
			m.cursor = 0
			m.updateScroll()
		case "G":
			if len(m.domains) > 0 {
				m.cursor = len(m.domains) - 1
			}
			m.updateScroll()
		case "r":
			m.loading = true
			m.err = nil
			return m, m.loadDomainsCmd()
		case "enter":
			if len(m.domains) > 0 {
				dom := m.domains[m.cursor]
				if m.embedded {
					return m, func() tea.Msg { return dnsNavigateToRecordListMsg{domain: dom} }
				}
				return m, tea.Quit
			}
		}

	case dnsDomainsLoadedMsg:
		m.loading = false
		m.domains = msg.domains
		m.cursor = 0
		if len(m.domains) == 0 {
			m.status = "No domains found."
		} else {
			m.status = fmt.Sprintf("%d domain(s)", len(m.domains))
		}

	case dnsDomainsErrorMsg:
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

func (m dnsDomainListModel) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	header := components.Header(m.width, "dns > domains", m.providerName)

	var footerBindings []components.KeyBinding
	if m.loading {
		footerBindings = []components.KeyBinding{
			{Key: "ctrl+c", Desc: "quit"},
		}
	} else {
		footerBindings = []components.KeyBinding{
			{Key: "j/k", Desc: "navigate"},
			{Key: "enter", Desc: "records"},
			{Key: "r", Desc: "refresh"},
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

func (m dnsDomainListModel) renderContent(height int) string {
	if m.loading {
		loadingText := m.spinner.View() + "  Fetching domains…"
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

	if len(m.domains) == 0 {
		return lipgloss.Place(
			m.width, height,
			lipgloss.Center, lipgloss.Center,
			styles.MutedText.Render("No domains found."),
		)
	}

	return m.renderTable(height)
}

func (m dnsDomainListModel) renderTable(height int) string {
	type column struct {
		title string
		width int
	}

	available := m.width - 4

	cols := []column{
		{title: "DOMAIN", width: 30},
		{title: "STATUS", width: 12},
		{title: "TLD", width: 10},
		{title: "EXPIRES", width: 20},
	}

	total := 0
	for _, c := range cols {
		total += c.width
	}
	if available > total {
		extra := available - total
		for i := range cols {
			if cols[i].title == "DOMAIN" {
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

	visibleRows := max(height-3, 1) // header + sep + padding

	end := m.listStart + visibleRows
	if end > len(m.domains) {
		end = len(m.domains)
	}

	var rows []string
	rows = append(rows, headerRow, sep)

	for i := m.listStart; i < end; i++ {
		d := m.domains[i]

		status := d.Status
		if status == "ACTIVE" || status == "active" {
			status = styles.SuccessText.Render(status)
		} else {
			status = styles.MutedText.Render(status)
		}

		cells := []string{
			lipgloss.NewStyle().Width(cols[0].width).Render(d.Name),
			lipgloss.NewStyle().Width(cols[1].width).Render(status),
			lipgloss.NewStyle().Width(cols[2].width).Render(d.TLD),
			lipgloss.NewStyle().Width(cols[3].width).Render(d.ExpireDate),
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

	table := lipgloss.JoinVertical(lipgloss.Left, rows...)

	// Pad content to fill available height to push footer down
	contentLines := strings.Split(table, "\n")
	if len(contentLines) < height {
		padding := strings.Repeat("\n", height-len(contentLines))
		table += padding
	}

	return table
}
