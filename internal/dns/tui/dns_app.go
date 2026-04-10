package tui

import (
	"context"
	"fmt"

	"nathanbeddoewebdev/vpsm/internal/dns/domain"
	"nathanbeddoewebdev/vpsm/internal/dns/services"
	"nathanbeddoewebdev/vpsm/internal/tui/components"
	"nathanbeddoewebdev/vpsm/internal/tui/styles"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Navigation messages ---
// Sent by child models to request view transitions.

type dnsNavigateToDomainListMsg struct{}

type dnsNavigateToRecordListMsg struct {
	domain domain.Domain
}

type dnsNavigateToRecordShowMsg struct {
	record domain.Record
	domain string
}

type dnsNavigateToRecordCreateMsg struct {
	domain string
}

type dnsNavigateToRecordEditMsg struct {
	record domain.Record
	domain string
}

type dnsNavigateToRecordDeleteMsg struct {
	record domain.Record
	domain string
}

type dnsNavigateBackMsg struct{}

// --- Action messages ---
// Sent by child models when the user confirms a destructive/creative action.

type dnsCreateConfirmedMsg struct {
	domain string
	opts   domain.CreateRecordOpts
}

type dnsUpdateConfirmedMsg struct {
	domain string
	id     string
	opts   domain.UpdateRecordOpts
}

type dnsDeleteConfirmedMsg struct {
	domain string
	record domain.Record
}

// --- Action result messages ---

type dnsCreateResultMsg struct {
	record *domain.Record
	err    error
}

type dnsUpdateResultMsg struct {
	err error
}

type dnsDeleteResultMsg struct {
	record domain.Record
	err    error
}

// --- Top-level App Model ---

type dnsAppView int

const (
	dnsAppViewDomainList dnsAppView = iota
	dnsAppViewRecordList
	dnsAppViewRecordShow
	dnsAppViewRecordCreate
	dnsAppViewRecordEdit
	dnsAppViewRecordDelete
	dnsAppViewAction // spinner while API call in progress
)

type dnsAppModel struct {
	service      *services.Service
	providerName string
	view         dnsAppView

	// Child models
	domainList   dnsDomainListModel
	recordList   dnsRecordListModel
	recordShow   dnsRecordShowModel
	recordCreate dnsRecordCreateModel
	recordEdit   dnsRecordEditModel
	recordDelete dnsRecordDeleteModel

	// Action state
	actionSpinner spinner.Model
	actionLabel   string
	actionStatus  string
	actionIsError bool

	width  int
	height int
}

// RunDNSApp starts the unified DNS TUI. If initialDomain is not empty,
// it jumps straight to the record list for that domain.
func RunDNSApp(service *services.Service, providerName string, initialDomain string) (tea.Model, error) {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(styles.Blue)

	m := dnsAppModel{
		service:       service,
		providerName:  providerName,
		view:          dnsAppViewDomainList,
		actionSpinner: s,
	}

	if initialDomain != "" {
		m.switchToRecordList(initialDomain)
	} else {
		m.switchToDomainList()
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	return p.Run()
}

func (m *dnsAppModel) switchToDomainList() {
	m.view = dnsAppViewDomainList
	m.domainList = newDNSDomainListModel(m.service, m.providerName, true, m.width, m.height)
}

func (m *dnsAppModel) switchToRecordList(domain string) {
	m.view = dnsAppViewRecordList
	m.recordList = newDNSRecordListModel(m.service, m.providerName, domain, true, m.width, m.height)
}

func (m *dnsAppModel) switchToRecordShow(rec domain.Record, domainName string) {
	m.view = dnsAppViewRecordShow
	m.recordShow = newDNSRecordShowModel(rec, domainName, m.providerName, true, m.width, m.height)
}

func (m dnsAppModel) Init() tea.Cmd {
	return m.domainList.Init()
}

func (m dnsAppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m.updateChild(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		if m.view == dnsAppViewAction {
			m.actionSpinner, cmd = m.actionSpinner.Update(msg)
			cmds = append(cmds, cmd)
		}
		// Also forward to child so loading spinners animate
		childModel, childCmd := m.updateChild(msg)
		m = childModel.(dnsAppModel)
		cmds = append(cmds, childCmd)
		return m, tea.Batch(cmds...)

	case dnsNavigateToDomainListMsg:
		m.switchToDomainList()
		return m, m.domainList.Init()

	case dnsNavigateToRecordListMsg:
		m.switchToRecordList(msg.domain.Name)
		return m, m.recordList.Init()

	case dnsNavigateToRecordShowMsg:
		m.switchToRecordShow(msg.record, msg.domain)
		return m, m.recordShow.Init()

	case dnsNavigateToRecordCreateMsg:
		m.view = dnsAppViewRecordCreate
		m.recordCreate = newDNSRecordCreateModel(m.service, m.providerName, msg.domain, domain.CreateRecordOpts{}, true, m.width, m.height)
		return m, m.recordCreate.Init()

	case dnsNavigateToRecordEditMsg:
		m.view = dnsAppViewRecordEdit
		m.recordEdit = newDNSRecordEditModel(m.service, m.providerName, msg.domain, msg.record, true, m.width, m.height)
		return m, m.recordEdit.Init()

	case dnsNavigateToRecordDeleteMsg:
		m.view = dnsAppViewRecordDelete
		m.recordDelete = newDNSRecordDeleteModel(m.service, m.providerName, msg.domain, msg.record, true, m.width, m.height)
		return m, m.recordDelete.Init()

	// Actions
	case dnsCreateConfirmedMsg:
		m.view = dnsAppViewAction
		m.actionLabel = fmt.Sprintf("Creating record for %s", msg.domain)
		m.actionIsError = false
		m.actionStatus = ""
		return m, tea.Batch(m.actionSpinner.Tick, func() tea.Msg {
			rec, err := m.service.CreateRecord(context.Background(), msg.domain, msg.opts)
			return dnsCreateResultMsg{record: rec, err: err}
		})

	case dnsUpdateConfirmedMsg:
		m.view = dnsAppViewAction
		m.actionLabel = fmt.Sprintf("Updating record %s", msg.id)
		m.actionIsError = false
		m.actionStatus = ""
		return m, tea.Batch(m.actionSpinner.Tick, func() tea.Msg {
			err := m.service.UpdateRecord(context.Background(), msg.domain, msg.id, msg.opts)
			return dnsUpdateResultMsg{err: err}
		})

	case dnsDeleteConfirmedMsg:
		m.view = dnsAppViewAction
		m.actionLabel = fmt.Sprintf("Deleting record %s", msg.record.ID)
		m.actionIsError = false
		m.actionStatus = ""
		return m, tea.Batch(m.actionSpinner.Tick, func() tea.Msg {
			err := m.service.DeleteRecord(context.Background(), msg.domain, msg.record.ID)
			return dnsDeleteResultMsg{record: msg.record, err: err}
		})

	// Results
	case dnsCreateResultMsg:
		if msg.err != nil {
			m.actionIsError = true
			m.actionStatus = msg.err.Error()
			return m, nil
		}
		// Go back to record list and refresh
		m.view = dnsAppViewRecordList
		m.recordList.persistentStatus = fmt.Sprintf("Created record %s", msg.record.ID)
		m.recordList.statusIsError = false
		m.recordList.loading = true
		return m, m.recordList.loadRecordsCmd()

	case dnsUpdateResultMsg:
		if msg.err != nil {
			m.actionIsError = true
			m.actionStatus = msg.err.Error()
			return m, nil
		}
		m.view = dnsAppViewRecordList
		m.recordList.persistentStatus = "Record updated successfully"
		m.recordList.statusIsError = false
		m.recordList.loading = true
		return m, m.recordList.loadRecordsCmd()

	case dnsDeleteResultMsg:
		if msg.err != nil {
			m.actionIsError = true
			m.actionStatus = msg.err.Error()
			return m, nil
		}
		m.view = dnsAppViewRecordList
		m.recordList.persistentStatus = fmt.Sprintf("Deleted record %s", msg.record.ID)
		m.recordList.statusIsError = false
		m.recordList.loading = true
		return m, m.recordList.loadRecordsCmd()

	case dnsNavigateBackMsg:
		if m.view == dnsAppViewRecordShow || m.view == dnsAppViewRecordCreate || m.view == dnsAppViewRecordEdit || m.view == dnsAppViewRecordDelete {
			m.view = dnsAppViewRecordList
			return m, nil
		}
		if m.view == dnsAppViewRecordList {
			// Only go back to domain list if we have it (i.e. not launched directly into record list)
			if m.domainList.service != nil {
				m.view = dnsAppViewDomainList
				return m, nil
			} else {
				return m, tea.Quit
			}
		}
	}

	childModel, cmd := m.updateChild(msg)
	m = childModel.(dnsAppModel)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m dnsAppModel) updateChild(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.view {
	case dnsAppViewDomainList:
		var updated tea.Model
		updated, cmd = m.domainList.Update(msg)
		m.domainList = updated.(dnsDomainListModel)
	case dnsAppViewRecordList:
		var updated tea.Model
		updated, cmd = m.recordList.Update(msg)
		m.recordList = updated.(dnsRecordListModel)
	case dnsAppViewRecordShow:
		var updated tea.Model
		updated, cmd = m.recordShow.Update(msg)
		m.recordShow = updated.(dnsRecordShowModel)
	case dnsAppViewRecordCreate:
		var updated tea.Model
		updated, cmd = m.recordCreate.Update(msg)
		m.recordCreate = updated.(dnsRecordCreateModel)
	case dnsAppViewRecordEdit:
		var updated tea.Model
		updated, cmd = m.recordEdit.Update(msg)
		m.recordEdit = updated.(dnsRecordEditModel)
	case dnsAppViewRecordDelete:
		var updated tea.Model
		updated, cmd = m.recordDelete.Update(msg)
		m.recordDelete = updated.(dnsRecordDeleteModel)
	}
	return m, cmd
}

func (m dnsAppModel) View() string {
	var view string

	switch m.view {
	case dnsAppViewDomainList:
		view = m.domainList.View()
	case dnsAppViewRecordList:
		view = m.recordList.View()
	case dnsAppViewRecordShow:
		view = m.recordShow.View()
	case dnsAppViewRecordCreate:
		view = m.recordCreate.View()
	case dnsAppViewRecordEdit:
		view = m.recordEdit.View()
	case dnsAppViewRecordDelete:
		view = m.recordDelete.View()
	case dnsAppViewAction:
		header := components.Header(m.width, "dns > processing", m.providerName)
		content := fmt.Sprintf("\n  %s %s\n", m.actionSpinner.View(), m.actionLabel)

		statusStyle := styles.Value
		if m.actionIsError {
			statusStyle = styles.ErrorText
		}

		if m.actionStatus != "" {
			content += fmt.Sprintf("\n  %s\n", statusStyle.Render(m.actionStatus))
		}

		view = lipgloss.JoinVertical(lipgloss.Left, header, content)
	}

	return view
}
