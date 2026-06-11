package tui

import (
	"encoding/json"
	"sort"
	"strings"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/store"
	tea "github.com/charmbracelet/bubbletea"
)

func loadCollapsed(s store.Store) map[int]bool {
	m := make(map[int]bool)
	raw, err := s.GetPreference("collapsed_groups")
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

func saveCollapsed(s store.Store, collapsed map[int]bool) {
	var ids []int
	for id, v := range collapsed {
		if v {
			ids = append(ids, id)
		}
	}
	data, _ := json.Marshal(ids)
	_ = s.SetPreference("collapsed_groups", string(data))
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
	m.logViewport.SetContent(strings.Join(m.engine.GetLogs(), "\n"))
	m.clampCursor()
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
// UI goroutine. The closure reads only stable fields (store, isAdmin) and never
// mutates the model; results come back as a tabDataMsg. On the first store
// error it returns an error-only msg so the model keeps its previous data.
func (m *Model) loadTabDataCmd() tea.Cmd {
	st := m.store
	isAdmin := m.isAdmin
	return func() tea.Msg {
		alerts, err := st.GetAllAlerts()
		if err != nil {
			return tabDataMsg{err: err}
		}
		var users []models.User
		if isAdmin {
			if users, err = st.GetAllUsers(); err != nil {
				return tabDataMsg{err: err}
			}
		}
		nodes, err := st.GetAllNodes()
		if err != nil {
			return tabDataMsg{err: err}
		}
		maint, err := st.GetAllMaintenanceWindows(100)
		if err != nil {
			return tabDataMsg{err: err}
		}
		return tabDataMsg{alerts: alerts, users: users, nodes: nodes, maint: maint}
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
