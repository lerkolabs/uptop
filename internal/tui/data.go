package tui

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/store"
	tea "github.com/charmbracelet/bubbletea"
)

func loadCollapsed(s store.Store) map[int]bool {
	m := make(map[int]bool)
	raw, err := s.GetPreference(context.Background(), "collapsed_groups")
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

func sortSitesForDisplay(allSites []models.Site, collapsed map[int]bool) []models.Site {
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
	for pid := range children {
		c := children[pid]
		sort.Slice(c, func(i, j int) bool { return c[i].ID < c[j].ID })
		sort.SliceStable(c, func(i, j int) bool { return siteOrder(c[i]) < siteOrder(c[j]) })
		children[pid] = c
	}
	sort.Slice(ungrouped, func(i, j int) bool { return ungrouped[i].ID < ungrouped[j].ID })
	sort.SliceStable(ungrouped, func(i, j int) bool { return siteOrder(ungrouped[i]) < siteOrder(ungrouped[j]) })

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
	ordered := sortSitesForDisplay(allSites, m.collapsed)
	if m.filterText != "" {
		ordered = filterSites(ordered, m.filterText)
	}
	m.sites = ordered
	m.refreshLogContent()

	if m.currentTab == tabMonitors && m.selectedID != 0 {
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
	if m.currentTab == tabMonitors && m.cursor < len(m.sites) {
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
	isAdmin := m.isAdmin
	return func() tea.Msg {
		ctx := context.Background()
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
	return func() tea.Msg {
		return detailDataMsg{siteID: siteID, changes: eng.GetStateChanges(siteID, 5)}
	}
}

// loadHistoryCmd loads the full state-change history for the history view off
// the UI goroutine.
func (m *Model) loadHistoryCmd(siteID int) tea.Cmd {
	eng := m.engine
	return func() tea.Msg {
		return historyDataMsg{siteID: siteID, changes: eng.GetStateChanges(siteID, 100)}
	}
}

// loadSLACmd loads the state changes backing the SLA view off the UI
// goroutine. The reply carries the request's site and period so a stale reply
// can be recognized and dropped.
func (m *Model) loadSLACmd(siteID, periodIdx int) tea.Cmd {
	eng := m.engine
	since := time.Now().Add(-slaPeriods[periodIdx].duration)
	return func() tea.Msg {
		return slaDataMsg{
			siteID:    siteID,
			periodIdx: periodIdx,
			changes:   eng.GetStateChangesSince(siteID, since),
		}
	}
}
