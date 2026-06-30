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
	case tickMsg:
		return m.handleTick(time.Time(msg))
	case tabDataMsg:
		return m.handleTabData(msg)
	case detailDataMsg:
		// Drop replies for a site the user has already navigated away from,
		// so a slow load can't clobber the panel currently on screen.
		if m.state == stateDetail && m.cursor < len(m.sites) && m.sites[m.cursor].ID != msg.siteID {
			return m, nil
		}
		m.detailChanges = msg.changes
		m.detailChangesSiteID = msg.siteID
		m.detailDailyDays = msg.dailyDays
		return m, nil
	case historyDataMsg:
		if msg.siteID != m.historySiteID {
			return m, nil // stale reply for a previously opened history
		}
		m.historyChanges = msg.changes
		m.historyViewport.SetContent(m.buildHistoryContent())
		m.historyViewport.GotoTop()
		return m, nil
	case slaDataMsg:
		return m.handleSLAData(msg)
	case writeDoneMsg:
		if msg.err != nil {
			m.engine.AddLog(msg.op + " failed: " + msg.err.Error())
		}
		m.refreshLive()
		return m, m.loadTabDataCmd()
	}

	if m.state == stateConfirmDelete {
		return m.handleConfirmDelete(msg)
	}
	if m.state == stateFormSite || m.state == stateFormAlert || m.state == stateFormUser || m.state == stateFormMaint {
		return m.handleFormMsg(msg)
	}
	if m.state == stateLogs {
		return m.handleLogsFullscreen(msg)
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
		// The store delete runs in a Cmd; the in-memory engine/model updates
		// stay here so the row vanishes immediately. If the delete fails, the
		// writeDoneMsg reload converges the UI back to the DB state (and the
		// engine poll loop re-adds a site that is still in the DB).
		st := m.store
		ctx := m.ctx
		id := m.deleteID
		var cmd tea.Cmd
		switch m.deleteKind {
		case "site":
			cmd = writeCmd("Delete site", func() error { return st.DeleteSite(ctx, id) })
			m.engine.RemoveSite(id)
			m.adjustCursor(len(m.sites) - 1)
		case "maint":
			cmd = writeCmd("Delete maintenance window", func() error { return st.DeleteMaintenanceWindow(ctx, id) })
		case "alert":
			cmd = writeCmd("Delete alert", func() error { return st.DeleteAlert(ctx, id) })
		case "user":
			cmd = writeCmd("Delete user", func() error { return st.DeleteUser(ctx, id) })
		}
		m.refreshLive()
		if m.returnState == stateSettings {
			m.state = stateSettings
		} else {
			m.state = stateDashboard
		}
		m.returnState = 0
		return m, cmd
	case "n", "N", "esc":
		if m.returnState == stateSettings {
			m.state = stateSettings
		} else {
			m.state = stateDashboard
		}
		m.returnState = 0
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) handleFormMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if keyMsg.String() == "esc" {
			m.huhForm = nil
			if m.returnState == stateSettings {
				m.state = stateSettings
			} else {
				m.state = stateDashboard
			}
			m.returnState = 0
			return m, nil
		}
	}
	if m.huhForm != nil {
		form, formCmd := m.huhForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.huhForm = f
		}
		if m.state == stateFormSite && m.siteFormData != nil &&
			m.siteFormData.SiteType != m.lastSiteType {
			rebuildCmd := m.rebuildSiteForm()
			// Advance to Type select — user just changed it.
			skipName := m.huhForm.NextField()
			return m, tea.Batch(rebuildCmd, skipName)
		}
		if m.huhForm.State == huh.StateCompleted {
			// The store write runs in the returned Cmd; its writeDoneMsg
			// triggers the tab-data reload once the row actually exists.
			cmd := m.submitForm()
			m.refreshLive()
			m.huhForm = nil
			return m, cmd
		}
		return m, formCmd
	}
	return m, nil
}

const logsStripHeight = 6

func (m *Model) recalcLayout() {
	chrome := chromeBase
	if m.filterMode || m.filterText != "" {
		chrome++
	}
	if m.logsOpen {
		chrome += logsStripHeight
	}
	m.maxTableRows = m.termHeight - chrome
	if m.maxTableRows < 1 {
		m.maxTableRows = 1
	}
}

func (m *Model) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.termWidth = msg.Width
	m.termHeight = msg.Height
	m.recalcLayout()
	m.logViewport.Width = msg.Width - chromePadH
	m.logViewport.Height = msg.Height - (chromePadV + chromeHeader + chromeFooter + 2)
	m.historyViewport.Width = msg.Width - chromePadH
	m.historyViewport.Height = msg.Height - 10
	m.slaViewport.Width = msg.Width - chromePadH
	m.slaViewport.Height = msg.Height - 16
	if m.huhForm != nil {
		formHeight := msg.Height - 7
		if formHeight < 5 {
			formHeight = 5
		}
		m.huhForm.WithHeight(formHeight)
	}
	return m, nil
}

func (m *Model) handleTick(t time.Time) (tea.Model, tea.Cmd) {
	m.refreshLive()
	m.tickCount++
	target := sinApprox(float64(m.tickCount)*0.3)*0.5 + 0.5
	m.pulsePos, m.pulseVel = m.pulseSpring.Update(m.pulsePos, m.pulseVel, target)

	cmds := []tea.Cmd{tickCmd()}
	if t.Sub(m.lastTabLoad) > tabRefreshTTL {
		m.lastTabLoad = t
		cmds = append(cmds, m.loadTabDataCmd())
		if dc := m.detailRefreshCmd(); dc != nil {
			cmds = append(cmds, dc)
		}
	}
	return m, tea.Batch(cmds...)
}

// detailRefreshCmd reloads the open detail panel's state-change list on the
// tab-data cadence, so a flap that happens while the panel is on screen shows
// up without leaving and re-entering. Nil when no detail panel is open.
func (m *Model) detailRefreshCmd() tea.Cmd {
	if m.state != stateDetail || m.cursor >= len(m.sites) {
		return nil
	}
	return m.loadDetailCmd(m.sites[m.cursor].ID)
}

// handleTabData folds an async tab-data load into the model. Replies older
// than the newest issued load are dropped so out-of-order completions can't
// overwrite fresher data. On error the previous data is kept and the failure
// logged, so a transient store error never blanks the view.
func (m *Model) handleTabData(msg tabDataMsg) (tea.Model, tea.Cmd) {
	if msg.seq != m.tabSeq {
		return m, nil
	}
	if msg.err != nil {
		m.engine.AddLog("Tab data refresh failed: " + msg.err.Error())
		return m, nil
	}
	m.alerts = msg.alerts
	if m.isAdmin {
		m.users = msg.users
	}
	m.nodes = msg.nodes
	m.maintenanceWindows = msg.maint
	m.clampCursor()
	return m, nil
}

// testAlertCmd sends a test notification off the UI goroutine; the outcome
// surfaces through the engine log (picked up by the next refreshLive).
func (m *Model) testAlertCmd(id int, name string) tea.Cmd {
	eng := m.engine
	ctx := m.ctx
	return func() tea.Msg {
		if err := eng.TestAlert(ctx, id); err != nil {
			eng.AddLog(fmt.Sprintf("Test alert failed (%s): %v", name, err))
		}
		return nil
	}
}

func (m *Model) handleLogsFullscreen(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			m.state = stateDashboard
			m.focusedPanel = panelMonitors
		case "ctrl+c":
			return m, tea.Quit
		case "f":
			m.logFilterImportant = !m.logFilterImportant
			m.refreshLogContent()
		case "up", "k":
			m.logViewport.ScrollUp(1)
		case "down", "j":
			m.logViewport.ScrollDown(1)
		case "pgup":
			m.logViewport.ScrollUp(m.logViewport.Height)
		case "pgdown":
			m.logViewport.ScrollDown(m.logViewport.Height)
		}
	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.logViewport.ScrollUp(3)
		case tea.MouseButtonWheelDown:
			m.logViewport.ScrollDown(3)
		}
	case tickMsg:
		m.refreshLogContent()
	}
	return m, nil
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
	if m.state == stateDetail {
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			return m.handleSparklineClick(msg)
		}
		return m, nil
	}
	if m.state != stateDashboard {
		return m, nil
	}
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		return m.handleClick(msg)
	}
	if msg.Button != tea.MouseButtonWheelUp && msg.Button != tea.MouseButtonWheelDown {
		return m, nil
	}

	if m.focusedPanel == panelLogs {
		if msg.Button == tea.MouseButtonWheelUp {
			m.scrollLogs(-3)
		} else {
			m.scrollLogs(3)
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
	m.syncSelectedID()
	if m.detailOpen && m.cursor < len(m.sites) {
		return m, m.loadDetailCmd(m.sites[m.cursor].ID)
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
	case stateSettings:
		return m.handleSettingsKey(msg)
	case stateMaintDetail:
		return m.handleMaintDetailKey(msg)
	case stateDashboard:
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
		m.recalcLayout()
		m.refreshLive()
	case "enter":
		m.filterMode = false
		m.recalcLayout()
	case "backspace":
		if len(m.filterText) > 0 {
			m.filterText = m.filterText[:len(m.filterText)-1]
			m.cursor = 0
			m.tableOffset = 0
			m.refreshLive()
		}
	case "ctrl+c":
		return m, tea.Quit
	default:
		if len(msg.Runes) == 1 {
			m.filterText += string(msg.Runes)
			m.cursor = 0
			m.tableOffset = 0
			m.refreshLive()
		}
	}
	return m, nil
}

func (m *Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.detailViewport.ScrollUp(1)
		return m, nil
	case "down", "j":
		m.detailViewport.ScrollDown(1)
		return m, nil
	case "pgup":
		m.detailViewport.ScrollUp(m.detailViewport.Height / 2)
		return m, nil
	case "pgdown":
		m.detailViewport.ScrollDown(m.detailViewport.Height / 2)
		return m, nil
	case "esc":
		if m.sparkTooltipIdx >= 0 {
			m.sparkTooltipIdx = -1
			return m, nil
		}
		m.sparkTooltipIdx = -1
		m.state = stateDashboard
	case "i":
		m.sparkTooltipIdx = -1
		m.state = stateDashboard
	case "e":
		return m.handleEditItem()
	case "h":
		if m.cursor < len(m.sites) {
			site := m.sites[m.cursor]
			m.historySiteName = site.Name
			m.historySiteID = site.ID
			m.historyChanges = nil
			m.historyViewport = viewport.New(
				m.termWidth-chromePadH,
				m.termHeight-10,
			)
			m.historyViewport.SetContent("\n  Loading state history...")
			m.state = stateHistory
			return m, m.loadHistoryCmd(site.ID)
		}
	case "s":
		if m.cursor < len(m.sites) {
			return m, m.openSLAView(m.sites[m.cursor])
		}
	case "q":
		m.state = stateDashboard
	}
	return m, nil
}

func (m *Model) handleSparklineClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.cursor >= len(m.sites) {
		return m, nil
	}
	site := m.sites[m.cursor]
	hist, _ := m.engine.GetHistory(site.ID)

	if zi := m.zones.Get("spark-latency"); zi != nil && !zi.IsZero() && zi.InBounds(msg) {
		x, _ := zi.Pos(msg)
		m.sparkTooltipIdx = resolveSparklineIndex(x, detailSparkWidth, len(hist.Latencies))
		return m, nil
	}
	if zi := m.zones.Get("spark-heartbeat"); zi != nil && !zi.IsZero() && zi.InBounds(msg) {
		x, _ := zi.Pos(msg)
		m.sparkTooltipIdx = resolveSparklineIndex(x, detailSparkWidth, len(hist.Statuses))
		return m, nil
	}

	m.sparkTooltipIdx = -1
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
			return m, m.loadSLACmd(m.slaSiteID, idx)
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

func (m *Model) openSLAView(site models.Site) tea.Cmd {
	m.slaSiteName = site.Name
	m.slaSiteID = site.ID
	m.slaPeriodIdx = 2 // default 30d
	m.slaViewport = viewport.New(
		m.termWidth-chromePadH,
		m.termHeight-16,
	)
	m.slaViewport.SetContent("\n  Loading SLA report...")
	m.state = stateSLA
	return m.loadSLACmd(site.ID, m.slaPeriodIdx)
}

// handleSLAData folds an async SLA load into the model. The SLA math itself is
// pure CPU and cheap, so it runs here; only the state-change read happens in
// the Cmd. Replies for a different site or period than currently selected are
// stale and dropped.
func (m *Model) handleSLAData(msg slaDataMsg) (tea.Model, tea.Cmd) {
	if msg.siteID != m.slaSiteID || msg.periodIdx != m.slaPeriodIdx {
		return m, nil
	}
	period := slaPeriods[msg.periodIdx]

	var currentStatus models.Status
	for _, s := range m.sites {
		if s.ID == msg.siteID {
			currentStatus = s.Status
			break
		}
	}

	m.slaReport = monitor.ComputeSLA(msg.changes, currentStatus, period.duration)
	m.slaDailyBreakdown = monitor.ComputeDailyBreakdown(msg.changes, currentStatus, period.days, time.Now())

	m.slaViewport = viewport.New(
		m.termWidth-chromePadH,
		m.termHeight-16,
	)
	m.slaViewport.SetContent(m.buildSLADailyContent())
	m.slaViewport.GotoTop()
	return m, nil
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

func (m *Model) handleMaintDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	mw := m.findMaintWindow(m.maintDetailID)
	switch msg.String() {
	case "q", "esc":
		m.state = stateDashboard
		m.focusedPanel = panelMaint
	case "x":
		if mw != nil {
			now := time.Now()
			isActive := !mw.StartTime.After(now) && (mw.EndTime.IsZero() || mw.EndTime.After(now))
			if isActive {
				st := m.store
				ctx := m.ctx
				id := mw.ID
				m.state = stateDashboard
				m.focusedPanel = panelMaint
				m.refreshLive()
				return m, writeCmd("End maintenance", func() error {
					return st.EndMaintenanceWindow(ctx, id)
				})
			}
		}
	case "d":
		if mw != nil {
			m.deleteID = mw.ID
			m.deleteName = mw.Title
			m.deleteKind = "maint"
			m.state = stateConfirmDelete
		}
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) findMaintWindow(id int) *models.MaintenanceWindow {
	for i := range m.maintenanceWindows {
		if m.maintenanceWindows[i].ID == id {
			return &m.maintenanceWindows[i]
		}
	}
	return nil
}

func (m *Model) handleAlertDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "i", "esc":
		if m.returnState == stateSettings {
			m.state = stateSettings
		} else {
			m.state = stateDashboard
		}
	}
	return m, nil
}

func (m *Model) handleSettingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "S":
		m.state = stateDashboard
	case "ctrl+c":
		return m, tea.Quit
	case "left":
		m.switchSettingsSection(m.settingsSection - 1)
		m.settingsCursor = 0
		m.settingsOffset = 0
	case "right":
		m.switchSettingsSection(m.settingsSection + 1)
		m.settingsCursor = 0
		m.settingsOffset = 0
	case "up", "k":
		if m.settingsCursor > 0 {
			m.settingsCursor--
			if m.settingsCursor < m.settingsOffset {
				m.settingsOffset = m.settingsCursor
			}
		}
	case "down", "j":
		max := m.settingsListLen() - 1
		if m.settingsCursor < max {
			m.settingsCursor++
			if m.settingsCursor >= m.settingsOffset+m.maxTableRows {
				m.settingsOffset++
			}
		}
	case "n":
		m.editID = 0
		m.editToken = ""
		m.returnState = stateSettings
		switch m.settingsSection {
		case sectionAlerts:
			m.state = stateFormAlert
			return m, m.initAlertHuhForm()
		case sectionUsers:
			if m.isAdmin {
				m.state = stateFormUser
				return m, m.initUserHuhForm()
			}
		}
	case "e":
		m.returnState = stateSettings
		switch m.settingsSection {
		case sectionAlerts:
			if len(m.alerts) > 0 && m.settingsCursor < len(m.alerts) {
				m.editID = m.alerts[m.settingsCursor].ID
				m.state = stateFormAlert
				return m, m.initAlertHuhForm()
			}
		case sectionUsers:
			if m.isAdmin && len(m.users) > 0 && m.settingsCursor < len(m.users) {
				m.editID = m.users[m.settingsCursor].ID
				m.state = stateFormUser
				return m, m.initUserHuhForm()
			}
		}
	case "d":
		m.returnState = stateSettings
		switch m.settingsSection {
		case sectionAlerts:
			if len(m.alerts) > 0 && m.settingsCursor < len(m.alerts) {
				m.deleteID = m.alerts[m.settingsCursor].ID
				m.deleteName = m.alerts[m.settingsCursor].Name
				m.deleteKind = "alert"
				m.state = stateConfirmDelete
			}
		case sectionUsers:
			if m.isAdmin && len(m.users) > 0 && m.settingsCursor < len(m.users) {
				m.deleteID = m.users[m.settingsCursor].ID
				m.deleteName = m.users[m.settingsCursor].Username
				m.deleteKind = "user"
				m.state = stateConfirmDelete
			}
		}
	case "t":
		if m.settingsSection == sectionAlerts && len(m.alerts) > 0 && m.settingsCursor < len(m.alerts) {
			a := m.alerts[m.settingsCursor]
			return m, m.testAlertCmd(a.ID, a.Name)
		}
	case "i":
		if m.settingsSection == sectionAlerts && len(m.alerts) > 0 && m.settingsCursor < len(m.alerts) {
			m.returnState = stateSettings
			m.state = stateAlertDetail
		}
	}
	return m, nil
}

func (m *Model) handleDashboardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "/":
		m.filterMode = true
		m.recalcLayout()
		return m, nil
	case ">", ".":
		m.sortColumn = (m.sortColumn + 1) % sortMax
		m.sortAsc = false
		m.refreshLive()
	case "<", ",":
		m.sortColumn = (m.sortColumn - 1 + sortMax) % sortMax
		m.sortAsc = false
		m.refreshLive()
	case "r":
		m.sortAsc = !m.sortAsc
		m.refreshLive()
	case "m":
		if m.termWidth >= wideBreakpoint {
			if m.focusedPanel == panelMaint {
				m.maintOpen = false
				m.focusedPanel = panelMonitors
			} else {
				m.maintOpen = true
				m.focusedPanel = panelMaint
			}
			m.recalcLayout()
		}
	case "l":
		if m.logsOpen {
			m.logsOpen = false
			m.focusedPanel = panelMonitors
		} else {
			m.logsOpen = true
			m.focusedPanel = panelLogs
		}
		m.recalcLayout()
	case "up", "k":
		if m.focusedPanel == panelMaint {
			m.scrollMaintCursor(-1)
			return m, nil
		}
		if m.focusedPanel == panelLogs {
			m.scrollLogs(-1)
			return m, nil
		}
		if m.cursor > 0 {
			m.cursor--
			if m.cursor < m.tableOffset {
				m.tableOffset = m.cursor
			}
			m.syncSelectedID()
			if m.detailOpen && m.cursor < len(m.sites) {
				m.detailMode = detailDefault
				return m, m.loadDetailCmd(m.sites[m.cursor].ID)
			}
		}
	case "down", "j":
		if m.focusedPanel == panelMaint {
			m.scrollMaintCursor(1)
			return m, nil
		}
		if m.focusedPanel == panelLogs {
			m.scrollLogs(1)
			return m, nil
		}
		max := m.currentListLen() - 1
		if m.cursor < max {
			m.cursor++
			if m.cursor >= m.tableOffset+m.maxTableRows {
				m.tableOffset++
			}
			m.syncSelectedID()
			if m.detailOpen && m.cursor < len(m.sites) {
				m.detailMode = detailDefault
				return m, m.loadDetailCmd(m.sites[m.cursor].ID)
			}
		}
	case "n":
		if m.focusedPanel == panelMaint {
			m.state = stateFormMaint
			return m, m.initMaintHuhForm()
		}
		return m.handleNewItem()
	case "enter":
		if m.focusedPanel == panelMaint {
			windows := m.activeMaintWindows()
			if m.maintCursor < len(windows) {
				m.maintDetailID = windows[m.maintCursor].ID
				m.state = stateMaintDetail
			}
			return m, nil
		}
		if m.focusedPanel == panelLogs {
			m.refreshLogContent()
			m.logViewport.GotoTop()
			m.state = stateLogs
			return m, nil
		}
		if len(m.sites) > 0 {
			site := m.sites[m.cursor]
			if site.Type == "group" {
				m.collapsed[site.ID] = !m.collapsed[site.ID]
				payload := collapsedJSON(m.collapsed)
				st := m.store
				ctx := m.ctx
				m.refreshLive()
				return m, writeCmd("Save collapsed groups", func() error {
					return st.SetPreference(ctx, "collapsed_groups", payload)
				})
			}
			if !m.detailOpen {
				m.detailOpen = true
				m.detailMode = detailDefault
				m.recalcLayout()
			}
			return m, m.loadDetailCmd(site.ID)
		}
	case "e":
		return m.handleEditItem()
	case " ":
		if len(m.sites) > 0 && m.sites[m.cursor].Type == "group" {
			gid := m.sites[m.cursor].ID
			m.collapsed[gid] = !m.collapsed[gid]
			payload := collapsedJSON(m.collapsed)
			st := m.store
			ctx := m.ctx
			m.refreshLive()
			return m, writeCmd("Save collapsed groups", func() error {
				return st.SetPreference(ctx, "collapsed_groups", payload)
			})
		}
	case "p":
		if len(m.sites) > 0 {
			id := m.sites[m.cursor].ID
			paused := m.engine.ToggleSitePause(id)
			st := m.store
			ctx := m.ctx
			m.refreshLive()
			return m, writeCmd("Update pause state", func() error {
				return st.UpdateSitePaused(ctx, id, paused)
			})
		}
	case "i":
		if len(m.sites) > 0 {
			m.detailOpen = !m.detailOpen
			m.recalcLayout()
			st := m.store
			ctx := m.ctx
			open := m.detailOpen
			var cmd tea.Cmd
			if m.detailOpen {
				cmd = m.loadDetailCmd(m.sites[m.cursor].ID)
			}
			saveCmd := writeCmd("Save detail preference", func() error {
				v := "false"
				if open {
					v = "true"
				}
				return st.SetPreference(ctx, "detail_open", v)
			})
			if cmd != nil {
				return m, tea.Batch(cmd, saveCmd)
			}
			return m, saveCmd
		}
	case "esc":
		if m.focusedPanel != panelMonitors {
			m.focusedPanel = panelMonitors
		} else if m.detailOpen && m.detailMode != detailDefault {
			m.detailMode = detailDefault
		} else if m.detailOpen {
			m.detailOpen = false
			m.detailMode = detailDefault
			m.recalcLayout()
			st := m.store
			ctx := m.ctx
			return m, writeCmd("Save detail preference", func() error {
				return st.SetPreference(ctx, "detail_open", "false")
			})
		}
	case "h":
		if m.detailOpen && m.cursor < len(m.sites) {
			site := m.sites[m.cursor]
			m.historySiteName = site.Name
			m.historySiteID = site.ID
			m.historyChanges = nil
			m.detailMode = detailHistory
			return m, m.loadHistoryCmd(site.ID)
		}
	case "s":
		if m.detailOpen && m.cursor < len(m.sites) {
			site := m.sites[m.cursor]
			m.slaSiteName = site.Name
			m.slaSiteID = site.ID
			m.slaPeriodIdx = 2
			m.detailMode = detailSLA
			return m, m.loadSLACmd(site.ID, m.slaPeriodIdx)
		}
	case "1", "2", "3", "4":
		if m.detailOpen && m.detailMode == detailSLA {
			idx := int(msg.String()[0]-'0') - 1
			if idx >= 0 && idx < len(slaPeriods) {
				m.slaPeriodIdx = idx
				return m, m.loadSLACmd(m.slaSiteID, idx)
			}
		}
	case "x":
		if m.focusedPanel == panelMaint {
			windows := m.activeMaintWindows()
			if m.maintCursor < len(windows) {
				mw := windows[m.maintCursor]
				now := time.Now()
				isActive := !mw.StartTime.After(now) && (mw.EndTime.IsZero() || mw.EndTime.After(now))
				if isActive {
					st := m.store
					ctx := m.ctx
					id := mw.ID
					m.refreshLive()
					return m, writeCmd("End maintenance", func() error {
						return st.EndMaintenanceWindow(ctx, id)
					})
				}
			}
		}
	case "S":
		m.settingsCursor = 0
		m.settingsOffset = 0
		m.state = stateSettings
		return m, nil
	case "T":
		m.themeIndex = (m.themeIndex + 1) % len(themes)
		m.theme = themes[m.themeIndex]
		m.st = newStyles(m.theme)
		st := m.store
		ctx := m.ctx
		name := m.theme.Name
		return m, writeCmd("Save theme", func() error {
			return st.SetPreference(ctx, "theme", name)
		})
	case "d":
		if m.focusedPanel == panelMaint {
			windows := m.activeMaintWindows()
			if m.maintCursor < len(windows) {
				mw := windows[m.maintCursor]
				m.deleteID = mw.ID
				m.deleteName = mw.Title
				m.deleteKind = "maint"
				m.state = stateConfirmDelete
			}
			return m, nil
		}
		return m.handleDeleteItem()
	}
	return m, nil
}

func (m *Model) handleNewItem() (tea.Model, tea.Cmd) {
	m.editID = 0
	m.editToken = ""
	m.state = stateFormSite
	return m, m.initSiteHuhForm()
}

func (m *Model) handleEditItem() (tea.Model, tea.Cmd) {
	if len(m.sites) > 0 {
		m.editID = m.sites[m.cursor].ID
		m.editToken = m.sites[m.cursor].Token
		m.state = stateFormSite
		return m, m.initSiteHuhForm()
	}
	return m, nil
}

func (m *Model) handleDeleteItem() (tea.Model, tea.Cmd) {
	if len(m.sites) > 0 {
		m.deleteID = m.sites[m.cursor].ID
		m.deleteName = m.sites[m.cursor].Name
		m.deleteKind = "site"
		m.state = stateConfirmDelete
	}
	return m, nil
}

func (m *Model) handleClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	sortZones := []struct {
		zone string
		col  int
	}{
		{"sort-status", sortStatus},
		{"sort-name", sortName},
		{"sort-latency", sortLatency},
	}
	for _, sz := range sortZones {
		if m.zones.Get(sz.zone).InBounds(msg) {
			if m.sortColumn == sz.col {
				m.sortAsc = !m.sortAsc
			} else {
				m.sortColumn = sz.col
				m.sortAsc = false
			}
			m.refreshLive()
			return m, nil
		}
	}

	if m.maintOpen && m.zones.Get("panel-maint").InBounds(msg) {
		m.focusedPanel = panelMaint
		return m, nil
	} else if m.zones.Get("panel-monitors").InBounds(msg) {
		m.focusedPanel = panelMonitors
	} else if m.zones.Get("panel-logs").InBounds(msg) {
		m.focusedPanel = panelLogs
		return m, nil
	} else if m.detailOpen && m.zones.Get("panel-detail").InBounds(msg) {
		m.focusedPanel = panelDetail
		return m, nil
	}

	end := m.tableOffset + m.maxTableRows
	if end > len(m.sites) {
		end = len(m.sites)
	}
	for i := m.tableOffset; i < end; i++ {
		if m.zones.Get(fmt.Sprintf("site-%d", i)).InBounds(msg) {
			m.cursor = i
			m.syncSelectedID()
			m.focusedPanel = panelMonitors
			if m.detailOpen {
				return m, m.loadDetailCmd(m.sites[m.cursor].ID)
			}
			return m, nil
		}
	}

	return m, nil
}

func (m *Model) adjustCursor(_ int) {
	m.clampCursor()
}

func (m *Model) submitForm() tea.Cmd {
	switch m.state {
	case stateFormSite:
		if m.siteFormData != nil {
			return m.submitSiteForm()
		}
	case stateFormAlert:
		if m.alertFormData != nil {
			return m.submitAlertForm()
		}
	case stateFormUser:
		if m.userFormData != nil {
			return m.submitUserForm()
		}
	case stateFormMaint:
		if m.maintFormData != nil {
			return m.submitMaintForm()
		}
	}
	return nil
}

func (m Model) currentListLen() int {
	return len(m.sites)
}
