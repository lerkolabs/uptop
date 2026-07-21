package tui

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/monitor"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/store"
	tea "github.com/charmbracelet/bubbletea"
)

func loadCollapsed(ctx context.Context, s store.Store) map[int]bool {
	m := make(map[int]bool)
	raw, err := s.GetPreference(ctx, "collapsed_groups")
	if err != nil || raw == "" {
		return m
	}
	var ids []int
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return m
	}
	for _, id := range ids {
		m[id] = true
	}
	return m
}

// collapsedJSON snapshots the collapsed-group set for persistence. Marshaling
// happens on the UI goroutine so the write Cmd never reads the live map.
func collapsedJSON(collapsed map[int]bool) string {
	var ids []int
	for id, v := range collapsed {
		if v {
			ids = append(ids, id)
		}
	}
	data, _ := json.Marshal(ids)
	return string(data)
}

// writeCmd runs a store mutation off the UI goroutine. The closure must only
// capture values snapshotted in Update — never the model itself.
func writeCmd(op string, fn func() error) tea.Cmd {
	return func() tea.Msg {
		return writeDoneMsg{op: op, err: fn()}
	}
}

func (m *Model) saveBottomPanelPref() tea.Cmd {
	v := "logs"
	switch m.bottomPanel {
	case bottomNone:
		v = "none"
	case bottomMaint:
		v = "maint"
	}
	st := m.store
	ctx := m.ctx
	return writeCmd("Save bottom panel preference", func() error {
		return st.SetPreference(ctx, "bottom_panel", v)
	})
}

func sortSitesForDisplay(allSites []models.Site, collapsed map[int]bool, sortCol int, sortAsc bool) []models.Site {
	var groups, ungrouped []models.Site
	children := make(map[int][]models.Site)
	for _, s := range allSites {
		if s.Type == "group" {
			groups = append(groups, s)
		} else if s.ParentID > 0 {
			children[s.ParentID] = append(children[s.ParentID], s)
		} else {
			ungrouped = append(ungrouped, s)
		}
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].ID < groups[j].ID })

	sortSlice := func(s []models.Site) {
		sort.Slice(s, func(i, j int) bool { return s[i].ID < s[j].ID })
		sort.SliceStable(s, func(i, j int) bool {
			var less bool
			switch sortCol {
			case sortName:
				less = strings.ToLower(s[i].Name) < strings.ToLower(s[j].Name)
			case sortLatency:
				less = s[i].Latency < s[j].Latency
			default:
				less = siteOrder(s[i]) < siteOrder(s[j])
			}
			if !sortAsc {
				return less
			}
			return !less
		})
	}

	for pid := range children {
		c := children[pid]
		sortSlice(c)
		children[pid] = c
	}
	sortSlice(ungrouped)

	var ordered []models.Site
	for _, g := range groups {
		ordered = append(ordered, g)
		if !collapsed[g.ID] {
			ordered = append(ordered, children[g.ID]...)
		}
	}
	ordered = append(ordered, ungrouped...)
	return ordered
}

func filterSites(sites []models.Site, needle string) []models.Site {
	lower := strings.ToLower(needle)
	var filtered []models.Site
	for _, s := range sites {
		if strings.Contains(strings.ToLower(s.Name), lower) {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// refreshLive updates everything sourced from in-memory engine copies — the
// live site list (sorted + filtered) and the log viewport. It does no database
// IO, so it is safe to call on every tick. DB-backed tab data is loaded
// separately via loadTabDataCmd.
func (m *Model) refreshLive() {
	allSites := m.engine.GetAllSites()
	ordered := sortSitesForDisplay(allSites, m.collapsed, m.sortColumn, m.sortAsc)
	if m.filterText != "" {
		ordered = filterSites(ordered, m.filterText)
	}
	m.sites = ordered
	m.buildMaintSet()
	m.refreshLogContent()

	if m.selectedID != 0 {
		for i, s := range m.sites {
			if s.ID == m.selectedID {
				m.cursor = i
				break
			}
		}
	}
	m.clampCursor()
}

func (m *Model) syncSelectedID() {
	if m.cursor < len(m.sites) {
		m.selectedID = m.sites[m.cursor].ID
	}
}

// clampCursor keeps the cursor and scroll offset within the current tab's list.
func (m *Model) clampCursor() {
	listLen := m.currentListLen()
	if listLen > 0 && m.cursor >= listLen {
		m.cursor = listLen - 1
	}
	if m.cursor < m.tableOffset {
		m.tableOffset = m.cursor
	}
}

func (m *Model) detailCmdIfNeeded() tea.Cmd {
	if m.focusedPanel == panelMonitors && m.detailOpen && m.cursor < len(m.sites) {
		m.detailMode = detailDefault
		m.detailScrollOffset = 0
		return m.loadDetailCmd(m.sites[m.cursor].ID)
	}
	return nil
}

func (m *Model) jumpToTop() {
	switch m.focusedPanel {
	case panelDetail:
		m.detailScrollOffset = 0
	case panelMaint:
		m.maintCursor = 0
	case panelLogs:
		m.logScrollOffset = 0
	default:
		m.cursor = 0
		m.tableOffset = 0
		m.syncSelectedID()
	}
}

func (m *Model) jumpToBottom() {
	switch m.focusedPanel {
	case panelDetail:
		m.detailScrollOffset = 9999
	case panelMaint:
		windows := m.activeMaintWindows()
		if len(windows) > 0 {
			m.maintCursor = len(windows) - 1
		}
	case panelLogs:
		total := m.filteredLogCount()
		if total > 0 {
			m.logScrollOffset = total - 1
		}
	default:
		max := m.currentListLen() - 1
		if max >= 0 {
			m.cursor = max
			if m.cursor >= m.tableOffset+m.maxTableRows {
				m.tableOffset = m.cursor - m.maxTableRows + 1
			}
			m.syncSelectedID()
		}
	}
}

func (m *Model) halfPageUp() {
	half := m.maxTableRows / 2
	if half < 1 {
		half = 1
	}
	switch m.focusedPanel {
	case panelDetail:
		m.detailScrollOffset -= half
		if m.detailScrollOffset < 0 {
			m.detailScrollOffset = 0
		}
	case panelMaint:
		m.maintCursor -= half
		if m.maintCursor < 0 {
			m.maintCursor = 0
		}
	case panelLogs:
		m.scrollLogs(-half)
	default:
		m.cursor -= half
		if m.cursor < 0 {
			m.cursor = 0
		}
		if m.cursor < m.tableOffset {
			m.tableOffset = m.cursor
		}
		m.syncSelectedID()
	}
}

func (m *Model) halfPageDown() {
	half := m.maxTableRows / 2
	if half < 1 {
		half = 1
	}
	switch m.focusedPanel {
	case panelDetail:
		m.detailScrollOffset += half
	case panelMaint:
		windows := m.activeMaintWindows()
		m.maintCursor += half
		if len(windows) > 0 && m.maintCursor >= len(windows) {
			m.maintCursor = len(windows) - 1
		}
	case panelLogs:
		m.scrollLogs(half)
	default:
		max := m.currentListLen() - 1
		m.cursor += half
		if m.cursor > max {
			m.cursor = max
		}
		if m.cursor < 0 {
			m.cursor = 0
		}
		if m.cursor >= m.tableOffset+m.maxTableRows {
			m.tableOffset = m.cursor - m.maxTableRows + 1
		}
		m.syncSelectedID()
	}
}

// loadTabDataCmd returns a tea.Cmd that loads the DB-backed tab tables off the
// UI goroutine. Each call bumps tabSeq and stamps the reply with it, so
// handleTabData can drop out-of-order results from slower earlier loads. The
// closure reads only stable fields (store, isAdmin) and never mutates the
// model; results come back as a tabDataMsg. On the first store error it
// returns an error-only msg so the model keeps its previous data.
func (m *Model) loadTabDataCmd() tea.Cmd {
	m.tabSeq++
	seq := m.tabSeq
	st := m.store
	ctx := m.ctx
	isAdmin := m.isAdmin
	return func() tea.Msg {
		alerts, err := st.GetAllAlerts(ctx)
		if err != nil {
			return tabDataMsg{seq: seq, err: err}
		}
		var users []models.User
		if isAdmin {
			if users, err = st.GetAllUsers(ctx); err != nil {
				return tabDataMsg{seq: seq, err: err}
			}
		}
		nodes, err := st.GetAllNodes(ctx)
		if err != nil {
			return tabDataMsg{seq: seq, err: err}
		}
		maint, err := st.GetAllMaintenanceWindows(ctx, 100)
		if err != nil {
			return tabDataMsg{seq: seq, err: err}
		}
		return tabDataMsg{seq: seq, alerts: alerts, users: users, nodes: nodes, maint: maint}
	}
}

// loadDetailCmd loads the state-change history for the detail panel off the UI
// goroutine. View renders the cached result rather than querying the DB.
func (m *Model) loadDetailCmd(siteID int) tea.Cmd {
	eng := m.engine
	ctx := m.ctx
	var currentStatus models.Status
	for _, s := range m.sites {
		if s.ID == siteID {
			currentStatus = s.Status
			break
		}
	}
	return func() tea.Msg {
		changes := eng.GetStateChanges(ctx, siteID, 5)
		now := time.Now()
		allChanges := eng.GetStateChangesSince(ctx, siteID, now.Add(-stateHistoryLookback))
		daily := monitor.ComputeDailyBreakdown(allChanges, currentStatus, stateHistoryDays, now)
		return detailDataMsg{siteID: siteID, changes: changes, dailyDays: daily}
	}
}

// loadHistoryCmd loads the full state-change history for the history view off
// the UI goroutine.
func (m *Model) loadHistoryCmd(siteID int) tea.Cmd {
	eng := m.engine
	ctx := m.ctx
	return func() tea.Msg {
		return historyDataMsg{siteID: siteID, changes: eng.GetStateChanges(ctx, siteID, stateHistoryLimit)}
	}
}

// loadSLACmd loads the state changes backing the SLA view off the UI
// goroutine. The reply carries the request's site and period so a stale reply
// can be recognized and dropped.
func (m *Model) loadSLACmd(siteID, periodIdx int) tea.Cmd {
	eng := m.engine
	ctx := m.ctx
	since := time.Now().Add(-slaPeriods[periodIdx].duration)
	return func() tea.Msg {
		return slaDataMsg{
			siteID:    siteID,
			periodIdx: periodIdx,
			changes:   eng.GetStateChangesSince(ctx, siteID, since),
		}
	}
}
