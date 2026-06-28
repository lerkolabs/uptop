package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) viewSettingsTab() string {
	maxSections := 2
	if m.isAdmin {
		maxSections = 3
	}

	sections := []string{"Alerts", "Nodes"}
	if m.isAdmin {
		sections = append(sections, "Users")
	}
	_ = maxSections

	var tabs []string
	for i, name := range sections {
		var rendered string
		if i == m.settingsSection {
			rendered = m.st.activeTab.Render(name)
		} else {
			rendered = m.st.inactiveTab.Render(name)
		}
		tabs = append(tabs, m.zones.Mark(fmt.Sprintf("section-%d", i), rendered))
	}
	header := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)

	var content string
	switch m.settingsSection {
	case sectionAlerts:
		content = m.viewAlertsTab()
	case sectionNodes:
		content = m.viewNodesTab()
	case sectionUsers:
		if m.isAdmin {
			content = m.viewUsersTab()
		}
	}

	return header + "\n" + content
}

func (m *Model) switchSettingsSection(idx int) {
	max := 1
	if m.isAdmin {
		max = 2
	}
	if idx > max {
		idx = 0
	}
	if idx < 0 {
		idx = max
	}
	m.settingsSection = idx
	m.cursor = 0
	m.tableOffset = 0
}
