package tui

import (
	"encoding/json"
	"sort"
	"strings"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/store"
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

func (m *Model) refreshData() {
	allSites := m.engine.GetAllSites()
	ordered := sortSitesForDisplay(allSites, m.collapsed)
	if m.filterText != "" {
		ordered = filterSites(ordered, m.filterText)
	}
	m.sites = ordered

	if alerts, err := m.store.GetAllAlerts(); err == nil {
		m.alerts = alerts
	}
	if m.isAdmin {
		if users, err := m.store.GetAllUsers(); err == nil {
			m.users = users
		}
	}
	if nodes, err := m.store.GetAllNodes(); err == nil {
		m.nodes = nodes
	}
	if windows, err := m.store.GetAllMaintenanceWindows(100); err == nil {
		m.maintenanceWindows = windows
	}
	m.logViewport.SetContent(strings.Join(m.engine.GetLogs(), "\n"))

	listLen := len(m.sites)
	switch m.currentTab {
	case 1:
		listLen = len(m.alerts)
	case 3:
		listLen = len(m.nodes)
	case 4:
		listLen = len(m.maintenanceWindows)
	case 5:
		listLen = len(m.users)
	}
	if listLen > 0 && m.cursor >= listLen {
		m.cursor = listLen - 1
	}
	if m.cursor < m.tableOffset {
		m.tableOffset = m.cursor
	}
}
