package tui

import (
	"context"
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
		id := m.deleteID
		var cmd tea.Cmd
		switch m.deleteTab {
		case 0:
			cmd = writeCmd("Delete site", func() error { return st.DeleteSite(context.Background(), id) })
			m.engine.RemoveSite(id)
			m.adjustCursor(len(m.sites) - 1)
		case 1:
			cmd = writeCmd("Delete alert", func() error { return st.DeleteAlert(context.Background(), id) })
			m.adjustCursor(len(m.alerts) - 1)
		case 4:
			cmd = writeCmd("Delete maintenance window", func() error { return st.DeleteMaintenanceWindow(context.Background(), id) })
			m.adjustCursor(len(m.maintenanceWindows) - 1)
		case 5:
			cmd = writeCmd("Delete user", func() error { return st.DeleteUser(context.Background(), id) })
			m.adjustCursor(len(m.users) - 1)
		}
		m.refreshLive()
		m.state = stateDashboard
		if m.deleteTab == 5 {
			m.state = stateUsers
		}
		return m, cmd
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
	return func() tea.Msg {
		if err := eng.TestAlert(id); err != nil {
			eng.AddLog(fmt.Sprintf("Test alert failed (%s): %v", name, err))
		}
		return nil
	}
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
		m.refreshLive()
	case "enter":
		m.filterMode = false
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
		if len(msg.String()) == 1 {
			m.filterText += msg.String()
			m.cursor = 0
			m.tableOffset = 0
			m.refreshLive()
		}
	}
	return m, nil
}

func (m *Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
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
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) handleSparklineClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.cursor >= len(m.sites) {
		return m, nil
	}
	site := m.sites[m.cursor]
	hist, _ := m.engine.GetHistory(site.ID)

	const sparkWidth = 40

	if zi := m.zones.Get("spark-latency"); zi != nil && !zi.IsZero() && zi.InBounds(msg) {
		x, _ := zi.Pos(msg)
		m.sparkTooltipIdx = resolveSparklineIndex(x, sparkWidth, len(hist.Latencies))
		return m, nil
	}
	if zi := m.zones.Get("spark-heartbeat"); zi != nil && !zi.IsZero() && zi.InBounds(msg) {
		x, _ := zi.Pos(msg)
		m.sparkTooltipIdx = resolveSparklineIndex(x, sparkWidth, len(hist.Statuses))
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

	var currentStatus string
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
			return m, m.testAlertCmd(a.ID, a.Name)
		}
	case " ":
		if m.currentTab == 0 && len(m.sites) > 0 && m.sites[m.cursor].Type == "group" {
			gid := m.sites[m.cursor].ID
			m.collapsed[gid] = !m.collapsed[gid]
			payload := collapsedJSON(m.collapsed)
			st := m.store
			m.refreshLive()
			return m, writeCmd("Save collapsed groups", func() error {
				return st.SetPreference(context.Background(), "collapsed_groups", payload)
			})
		}
	case "p":
		if m.currentTab == 0 && len(m.sites) > 0 {
			id := m.sites[m.cursor].ID
			paused := m.engine.ToggleSitePause(id)
			st := m.store
			m.refreshLive()
			return m, writeCmd("Update pause state", func() error {
				return st.UpdateSitePaused(context.Background(), id, paused)
			})
		}
	case "i":
		if m.currentTab == 0 && len(m.sites) > 0 {
			m.state = stateDetail
			return m, m.loadDetailCmd(m.sites[m.cursor].ID)
		} else if m.currentTab == 1 && len(m.alerts) > 0 {
			m.state = stateAlertDetail
		}
	case "x":
		if m.currentTab == 4 && len(m.maintenanceWindows) > 0 {
			mw := m.maintenanceWindows[m.cursor]
			now := time.Now()
			isActive := !mw.StartTime.After(now) && (mw.EndTime.IsZero() || mw.EndTime.After(now))
			if isActive {
				st := m.store
				id := mw.ID
				m.refreshLive()
				return m, writeCmd("End maintenance", func() error {
					return st.EndMaintenanceWindow(context.Background(), id)
				})
			}
		}
	case "T":
		m.themeIndex = (m.themeIndex + 1) % len(themes)
		m.theme = themes[m.themeIndex]
		m.st = newStyles(m.theme)
		st := m.store
		name := m.theme.Name
		return m, writeCmd("Save theme", func() error {
			return st.SetPreference(context.Background(), "theme", name)
		})
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
