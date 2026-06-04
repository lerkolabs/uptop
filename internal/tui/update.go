package tui

import (
	"fmt"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/monitor"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleResize(msg)
	case time.Time:
		return m.handleTick(msg)
	}

	if m.state == stateConfirmDelete {
		return m.handleConfirmDelete(msg)
	}
	if m.state == stateFormSite || m.state == stateFormAlert || m.state == stateFormUser || m.state == stateFormMaint {
		return m.handleFormMsg(msg)
	}

	switch msg := msg.(type) {
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleConfirmDelete(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "y", "Y":
		switch m.deleteTab {
		case 0:
			if err := m.store.DeleteSite(m.deleteID); err != nil {
				m.engine.AddLog("Delete site failed: " + err.Error())
			}
			m.engine.RemoveSite(m.deleteID)
			m.adjustCursor(len(m.sites) - 1)
		case 1:
			if err := m.store.DeleteAlert(m.deleteID); err != nil {
				m.engine.AddLog("Delete alert failed: " + err.Error())
			}
			m.adjustCursor(len(m.alerts) - 1)
		case 4:
			if err := m.store.DeleteMaintenanceWindow(m.deleteID); err != nil {
				m.engine.AddLog("Delete maintenance window failed: " + err.Error())
			}
			m.adjustCursor(len(m.maintenanceWindows) - 1)
		case 5:
			if err := m.store.DeleteUser(m.deleteID); err != nil {
				m.engine.AddLog("Delete user failed: " + err.Error())
			}
			m.adjustCursor(len(m.users) - 1)
		}
		m.refreshData()
		m.state = stateDashboard
		if m.deleteTab == 5 {
			m.state = stateUsers
		}
	case "n", "N", "esc":
		m.state = stateDashboard
		if m.deleteTab == 5 {
			m.state = stateUsers
		}
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) handleFormMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.termWidth = wsm.Width
		m.termHeight = wsm.Height
	}
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if keyMsg.String() == "esc" {
			m.huhForm = nil
			m.state = stateDashboard
			if m.currentTab == 5 {
				m.state = stateUsers
			}
			return m, nil
		}
	}
	if m.huhForm != nil {
		form, formCmd := m.huhForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.huhForm = f
		}
		if m.huhForm.State == huh.StateCompleted {
			m.submitForm()
			m.refreshData()
			m.huhForm = nil
			return m, nil
		}
		return m, formCmd
	}
	return m, nil
}

func (m *Model) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.termWidth = msg.Width
	m.termHeight = msg.Height
	chrome := chromeBase
	if m.filterMode || m.filterText != "" {
		chrome++
	}
	m.maxTableRows = msg.Height - chrome
	if m.maxTableRows < 1 {
		m.maxTableRows = 1
	}
	m.logViewport.Width = msg.Width - chromePadH
	m.logViewport.Height = msg.Height - (chromePadV + chromeHeader + chromeFooter + 2)
	m.historyViewport.Width = msg.Width - chromePadH
	m.historyViewport.Height = msg.Height - 10
	m.slaViewport.Width = msg.Width - chromePadH
	m.slaViewport.Height = msg.Height - 16
	return m, tea.ClearScreen
}

func (m *Model) handleTick(t time.Time) (tea.Model, tea.Cmd) {
	m.refreshData()
	m.tickCount++
	target := sinApprox(float64(m.tickCount)*0.3)*0.5 + 0.5
	m.pulsePos, m.pulseVel = m.pulseSpring.Update(m.pulsePos, m.pulseVel, target)
	return m, tea.Tick(time.Second, func(t time.Time) tea.Msg { return t })
}

func (m *Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.state == stateHistory {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.historyViewport.ScrollUp(3)
		case tea.MouseButtonWheelDown:
			m.historyViewport.ScrollDown(3)
		}
		return m, nil
	}
	if m.state == stateSLA {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.slaViewport.ScrollUp(3)
		case tea.MouseButtonWheelDown:
			m.slaViewport.ScrollDown(3)
		}
		return m, nil
	}
	if m.state != stateDashboard && m.state != stateLogs && m.state != stateUsers {
		return m, nil
	}
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		return m.handleClick(msg)
	}
	if msg.Button != tea.MouseButtonWheelUp && msg.Button != tea.MouseButtonWheelDown {
		return m, nil
	}

	if m.state == stateLogs {
		if msg.Button == tea.MouseButtonWheelUp {
			m.logViewport.ScrollUp(3)
		} else {
			m.logViewport.ScrollDown(3)
		}
		return m, nil
	}

	listLen := m.currentListLen()
	if msg.Button == tea.MouseButtonWheelUp {
		if m.cursor > 0 {
			m.cursor--
			if m.cursor < m.tableOffset {
				m.tableOffset = m.cursor
			}
		}
	} else {
		if m.cursor < listLen-1 {
			m.cursor++
			if m.cursor >= m.tableOffset+m.maxTableRows {
				m.tableOffset++
			}
		}
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if msg.String() == "ctrl+l" {
		return m, tea.ClearScreen
	}

	if m.filterMode {
		return m.handleFilterKey(msg)
	}

	switch m.state {
	case stateDetail:
		return m.handleDetailKey(msg)
	case stateHistory:
		return m.handleHistoryKey(msg)
	case stateSLA:
		return m.handleSLAKey(msg)
	case stateAlertDetail:
		return m.handleAlertDetailKey(msg)
	case stateDashboard, stateLogs, stateUsers:
		return m.handleDashboardKey(msg)
	}
	return m, nil
}

func (m *Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filterMode = false
		m.filterText = ""
		m.cursor = 0
		m.tableOffset = 0
		m.refreshData()
	case "enter":
		m.filterMode = false
	case "backspace":
		if len(m.filterText) > 0 {
			m.filterText = m.filterText[:len(m.filterText)-1]
			m.cursor = 0
			m.tableOffset = 0
			m.refreshData()
		}
	case "ctrl+c":
		return m, tea.Quit
	default:
		if len(msg.String()) == 1 {
			m.filterText += msg.String()
			m.cursor = 0
			m.tableOffset = 0
			m.refreshData()
		}
	}
	return m, nil
}

func (m *Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "i", "esc":
		m.state = stateDashboard
	case "e":
		return m.handleEditItem()
	case "h":
		if m.cursor < len(m.sites) {
			site := m.sites[m.cursor]
			m.historySiteName = site.Name
			m.historyChanges = m.engine.GetStateChanges(site.ID, 100)
			m.historyViewport = viewport.New(
				m.termWidth-chromePadH,
				m.termHeight-10,
			)
			m.historyViewport.SetContent(m.buildHistoryContent())
			m.historyViewport.GotoTop()
			m.state = stateHistory
		}
	case "s":
		if m.cursor < len(m.sites) {
			m.openSLAView(m.sites[m.cursor])
		}
	case "q":
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) handleSLAKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		m.state = stateDetail
	case "1", "2", "3", "4":
		idx := int(msg.String()[0]-'0') - 1
		if idx >= 0 && idx < len(slaPeriods) {
			m.slaPeriodIdx = idx
			m.recomputeSLA()
		}
	case "up", "k":
		m.slaViewport.ScrollUp(1)
	case "down", "j":
		m.slaViewport.ScrollDown(1)
	case "pgup":
		m.slaViewport.HalfPageUp()
	case "pgdown":
		m.slaViewport.HalfPageDown()
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) openSLAView(site models.Site) {
	m.slaSiteName = site.Name
	m.slaSiteID = site.ID
	m.slaPeriodIdx = 2 // default 30d
	m.recomputeSLA()
	m.state = stateSLA
}

func (m *Model) recomputeSLA() {
	period := slaPeriods[m.slaPeriodIdx]
	since := time.Now().Add(-period.duration)
	changes := m.engine.GetStateChangesSince(m.slaSiteID, since)

	var currentStatus string
	if m.cursor < len(m.sites) {
		currentStatus = m.sites[m.cursor].Status
	}

	m.slaReport = monitor.ComputeSLA(changes, currentStatus, period.duration)
	m.slaDailyBreakdown = monitor.ComputeDailyBreakdown(changes, currentStatus, period.days)

	m.slaViewport = viewport.New(
		m.termWidth-chromePadH,
		m.termHeight-16,
	)
	m.slaViewport.SetContent(m.buildSLADailyContent())
	m.slaViewport.GotoTop()
}

func (m *Model) handleHistoryKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		m.state = stateDetail
	case "up", "k":
		m.historyViewport.ScrollUp(1)
	case "down", "j":
		m.historyViewport.ScrollDown(1)
	case "pgup":
		m.historyViewport.HalfPageUp()
	case "pgdown":
		m.historyViewport.HalfPageDown()
	case "home", "g":
		m.historyViewport.GotoTop()
	case "end", "G":
		m.historyViewport.GotoBottom()
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) handleAlertDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "i", "esc":
		m.state = stateDashboard
	case "q":
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) handleDashboardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "/":
		if m.currentTab == 0 {
			m.filterMode = true
			return m, nil
		}
	case "f":
		if m.state == stateLogs {
			m.logFilterImportant = !m.logFilterImportant
			return m, nil
		}
	case "tab":
		m.switchTab(m.currentTab + 1)
	case "pgup", "pgdown":
		if m.state == stateLogs {
			m.logViewport, cmd = m.logViewport.Update(msg)
			return m, cmd
		}
	case "up", "k":
		if m.state == stateLogs {
			m.logViewport.ScrollUp(1)
		} else if m.cursor > 0 {
			m.cursor--
			if m.cursor < m.tableOffset {
				m.tableOffset = m.cursor
			}
		}
	case "down", "j":
		if m.state == stateLogs {
			m.logViewport.ScrollDown(1)
		} else {
			max := m.currentListLen() - 1
			if m.cursor < max {
				m.cursor++
				if m.cursor >= m.tableOffset+m.maxTableRows {
					m.tableOffset++
				}
			}
		}
	case "n":
		return m.handleNewItem()
	case "e", "enter":
		return m.handleEditItem()
	case "t":
		if m.currentTab == 1 && len(m.alerts) > 0 {
			a := m.alerts[m.cursor]
			go func() {
				if err := m.engine.TestAlert(a.ID); err != nil {
					m.engine.AddLog(fmt.Sprintf("Test alert failed (%s): %v", a.Name, err))
				}
			}()
			return m, nil
		}
	case " ":
		if m.currentTab == 0 && len(m.sites) > 0 && m.sites[m.cursor].Type == "group" {
			gid := m.sites[m.cursor].ID
			m.collapsed[gid] = !m.collapsed[gid]
			saveCollapsed(m.store, m.collapsed)
			m.refreshData()
		}
	case "p":
		if m.currentTab == 0 && len(m.sites) > 0 {
			site := m.sites[m.cursor]
			m.engine.ToggleSitePause(site.ID)
			site.Paused = !site.Paused
			_ = m.store.UpdateSitePaused(site.ID, site.Paused)
			m.refreshData()
		}
	case "i":
		if m.currentTab == 0 && len(m.sites) > 0 {
			m.state = stateDetail
		} else if m.currentTab == 1 && len(m.alerts) > 0 {
			m.state = stateAlertDetail
		}
	case "x":
		if m.currentTab == 4 && len(m.maintenanceWindows) > 0 {
			mw := m.maintenanceWindows[m.cursor]
			now := time.Now()
			isActive := !mw.StartTime.After(now) && (mw.EndTime.IsZero() || mw.EndTime.After(now))
			if isActive {
				if err := m.store.EndMaintenanceWindow(mw.ID); err != nil {
					m.engine.AddLog("End maintenance failed: " + err.Error())
				}
				m.refreshData()
			}
		}
	case "T":
		m.themeIndex = (m.themeIndex + 1) % len(themes)
		m.theme = themes[m.themeIndex]
		applyTheme(m.theme)
		_ = m.store.SetPreference("theme", m.theme.Name)
	case "d", "backspace":
		return m.handleDeleteItem()
	}
	return m, nil
}

func (m *Model) handleNewItem() (tea.Model, tea.Cmd) {
	m.editID = 0
	m.editToken = ""
	switch m.currentTab {
	case 0:
		m.state = stateFormSite
		return m, m.initSiteHuhForm()
	case 1:
		m.state = stateFormAlert
		return m, m.initAlertHuhForm()
	case 4:
		m.state = stateFormMaint
		return m, m.initMaintHuhForm()
	case 5:
		if m.isAdmin {
			m.state = stateFormUser
			return m, m.initUserHuhForm()
		}
	}
	return m, nil
}

func (m *Model) handleEditItem() (tea.Model, tea.Cmd) {
	switch m.currentTab {
	case 0:
		if len(m.sites) > 0 {
			m.editID = m.sites[m.cursor].ID
			m.editToken = m.sites[m.cursor].Token
			m.state = stateFormSite
			return m, m.initSiteHuhForm()
		}
	case 1:
		if len(m.alerts) > 0 {
			m.editID = m.alerts[m.cursor].ID
			m.state = stateFormAlert
			return m, m.initAlertHuhForm()
		}
	case 5:
		if m.isAdmin && len(m.users) > 0 {
			m.editID = m.users[m.cursor].ID
			m.state = stateFormUser
			return m, m.initUserHuhForm()
		}
	}
	return m, nil
}

func (m *Model) handleDeleteItem() (tea.Model, tea.Cmd) {
	switch m.currentTab {
	case 0:
		if len(m.sites) > 0 {
			m.deleteID = m.sites[m.cursor].ID
			m.deleteName = m.sites[m.cursor].Name
			m.deleteTab = 0
			m.state = stateConfirmDelete
		}
	case 1:
		if len(m.alerts) > 0 {
			m.deleteID = m.alerts[m.cursor].ID
			m.deleteName = m.alerts[m.cursor].Name
			m.deleteTab = 1
			m.state = stateConfirmDelete
		}
	case 4:
		if len(m.maintenanceWindows) > 0 {
			m.deleteID = m.maintenanceWindows[m.cursor].ID
			m.deleteName = m.maintenanceWindows[m.cursor].Title
			m.deleteTab = 4
			m.state = stateConfirmDelete
		}
	case 5:
		if m.isAdmin && len(m.users) > 0 {
			m.deleteID = m.users[m.cursor].ID
			m.deleteName = m.users[m.cursor].Username
			m.deleteTab = 5
			m.state = stateConfirmDelete
		}
	}
	return m, nil
}

func (m *Model) handleClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	tabCount := 5
	if m.isAdmin {
		tabCount = 6
	}
	for i := 0; i < tabCount; i++ {
		if m.zones.Get(fmt.Sprintf("tab-%d", i)).InBounds(msg) {
			m.switchTab(i)
			return m, nil
		}
	}

	prefix, listLen := m.currentZonePrefix()
	end := m.tableOffset + m.maxTableRows
	if end > listLen {
		end = listLen
	}
	for i := m.tableOffset; i < end; i++ {
		if m.zones.Get(fmt.Sprintf("%s-%d", prefix, i)).InBounds(msg) {
			m.cursor = i
			return m, nil
		}
	}

	return m, nil
}

func (m *Model) switchTab(idx int) {
	maxTabs := 4
	if m.isAdmin {
		maxTabs = 5
	}
	if idx > maxTabs {
		idx = 0
	}
	m.currentTab = idx
	m.cursor = 0
	m.tableOffset = 0
	switch idx {
	case 2:
		m.state = stateLogs
	case 5:
		m.state = stateUsers
	default:
		m.state = stateDashboard
	}
}

func (m *Model) adjustCursor(newLen int) {
	if m.cursor >= newLen && m.cursor > 0 {
		m.cursor--
	}
	if m.cursor < m.tableOffset {
		m.tableOffset = m.cursor
		if m.tableOffset < 0 {
			m.tableOffset = 0
		}
	}
}

func (m *Model) submitForm() {
	switch m.state {
	case stateFormSite:
		if m.siteFormData != nil {
			m.submitSiteForm()
		}
	case stateFormAlert:
		if m.alertFormData != nil {
			m.submitAlertForm()
		}
	case stateFormUser:
		if m.userFormData != nil {
			m.submitUserForm()
		}
	case stateFormMaint:
		if m.maintFormData != nil {
			m.submitMaintForm()
		}
	}
}

func (m Model) currentListLen() int {
	switch m.currentTab {
	case 1:
		return len(m.alerts)
	case 3:
		return len(m.nodes)
	case 4:
		return len(m.maintenanceWindows)
	case 5:
		return len(m.users)
	default:
		return len(m.sites)
	}
}

func (m Model) currentZonePrefix() (string, int) {
	switch m.currentTab {
	case 0:
		return "site", len(m.sites)
	case 1:
		return "alert", len(m.alerts)
	case 4:
		return "maint", len(m.maintenanceWindows)
	case 5:
		return "user", len(m.users)
	default:
		return "site", 0
	}
}
