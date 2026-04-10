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

type dnsRecordDeleteModel struct {
	service      *services.Service
	providerName string
	domain       string
	record       domain.Record

	confirmIdx int // 0 = Delete, 1 = Cancel
	confirmed  bool
	loading    bool
	spinner    spinner.Model
	err        error

	embedded bool
	width    int
	height   int
}

func newDNSRecordDeleteModel(svc *services.Service, providerName, domainName string, rec domain.Record, embedded bool, width, height int) dnsRecordDeleteModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(styles.Red)

	return dnsRecordDeleteModel{
		service:      svc,
		providerName: providerName,
		domain:       domainName,
		record:       rec,
		confirmIdx:   1, // Default to Cancel for safety
		embedded:     embedded,
		width:        width,
		height:       height,
		spinner:      s,
	}
}

func (m dnsRecordDeleteModel) Init() tea.Cmd {
	return nil
}

func (m dnsRecordDeleteModel) deleteCmd() tea.Cmd {
	return func() tea.Msg {
		err := m.service.DeleteRecord(context.Background(), m.domain, m.record.ID)
		return dnsDeleteResultMsg{record: m.record, err: err}
	}
}

func (m dnsRecordDeleteModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		if m.loading {
			break
		}
		switch msg.String() {
		case "esc", "q":
			if m.embedded {
				return m, func() tea.Msg { return dnsNavigateBackMsg{} }
			}
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
			if m.confirmIdx == 1 {
				// Cancel
				if m.embedded {
					return m, func() tea.Msg { return dnsNavigateBackMsg{} }
				}
				return m, tea.Quit
			}
			// Delete
			m.confirmed = true
			if m.embedded {
				return m, func() tea.Msg { return dnsDeleteConfirmedMsg{domain: m.domain, record: m.record} }
			}
			m.loading = true
			return m, tea.Batch(m.spinner.Tick, m.deleteCmd())
		}

	case dnsDeleteResultMsg:
		m.loading = false
		m.err = msg.err
		if m.err == nil && !m.embedded {
			return m, tea.Quit
		}

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m dnsRecordDeleteModel) View() string {
	header := components.Header(m.width, "dns > delete", m.providerName)

	bindings := []components.KeyBinding{
		{Key: "←/→", Desc: "select"},
		{Key: "enter", Desc: "confirm"},
		{Key: "esc", Desc: "cancel"},
	}
	footer := components.Footer(m.width, bindings)

	headerH := lipgloss.Height(header)
	footerH := lipgloss.Height(footer)
	contentH := m.height - headerH - footerH
	if contentH < 1 {
		contentH = 1
	}

	var content string
	if m.loading {
		content = fmt.Sprintf("\n  %s Deleting record...", m.spinner.View())
	} else if m.err != nil {
		content = fmt.Sprintf("\n  %s", styles.ErrorText.Render(m.err.Error()))
	} else {
		content = m.renderCard()
	}

	content = lipgloss.Place(m.width, contentH, lipgloss.Center, lipgloss.Center, content)

	return lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
}

func (m dnsRecordDeleteModel) renderCard() string {
	title := lipgloss.NewStyle().Foreground(styles.Red).Bold(true).Render("Delete DNS Record")

	r := m.record

	fields := []string{
		lipgloss.JoinHorizontal(lipgloss.Left, lipgloss.NewStyle().Width(10).Render(styles.Label.Render("Name")), styles.Value.Render(r.Name)),
		lipgloss.JoinHorizontal(lipgloss.Left, lipgloss.NewStyle().Width(10).Render(styles.Label.Render("Type")), styles.Value.Render(string(r.Type))),
		lipgloss.JoinHorizontal(lipgloss.Left, lipgloss.NewStyle().Width(10).Render(styles.Label.Render("Content")), styles.Value.Render(r.Content)),
		lipgloss.JoinHorizontal(lipgloss.Left, lipgloss.NewStyle().Width(10).Render(styles.Label.Render("TTL")), styles.Value.Render(fmt.Sprintf("%d", r.TTL))),
	}

	warning := styles.ErrorText.Render("This action cannot be undone.")

	// Buttons
	delBtn := "[ Delete ]"
	canBtn := "[ Cancel ]"

	if m.confirmIdx == 0 {
		delBtn = lipgloss.NewStyle().Foreground(styles.White).Background(styles.Red).Render(delBtn)
		canBtn = styles.MutedText.Render(canBtn)
	} else {
		delBtn = lipgloss.NewStyle().Foreground(styles.Red).Render(delBtn)
		canBtn = lipgloss.NewStyle().Foreground(styles.White).Background(styles.Gray).Render(canBtn)
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Center, delBtn, "  ", canBtn)

	cardContent := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		strings.Join(fields, "\n"),
		"",
		warning,
	)

	// Make the card border red
	cardStyle := styles.Card.Copy().BorderForeground(styles.Red)

	return lipgloss.JoinVertical(lipgloss.Center, cardStyle.Render(cardContent), "", buttons)
}
