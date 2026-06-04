package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func sinApprox(x float64) float64 {
	return math.Sin(x)
}

func (m Model) pulseIndicator() string {
	hasDown := false
	for _, s := range m.sites {
		if !s.Paused && !m.isMonitorInMaintenance(s.ID) && (s.Status == "DOWN" || s.Status == "SSL EXP") {
			hasDown = true
			break
		}
	}
	if m.demoMode {
		c := m.theme.Success
		if hasDown {
			c = m.theme.Danger
		}
		return lipgloss.NewStyle().Foreground(c).Render("●")
	}
	frame := m.tickCount % len(pulseFrames)
	brightness := int(m.pulsePos*155) + 100
	if brightness > 255 {
		brightness = 255
	}
	var color string
	if hasDown {
		color = fmt.Sprintf("#%02x%02x%02x", brightness, brightness/4, brightness/4)
	} else {
		color = fmt.Sprintf("#%02x%02x%02x", brightness/3, brightness, brightness/2)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(pulseFrames[frame])
}

func (m Model) View() string {
	switch m.state {
	case stateConfirmDelete:
		kind := "monitor"
		switch m.deleteTab {
		case 1:
			kind = "alert"
		case 4:
			kind = "maintenance window"
		case 5:
			kind = "user"
		}
		msg := dangerStyle.Render(fmt.Sprintf("Delete %s \"%s\"?", kind, m.deleteName))
		hint := subtleStyle.Render("[y] Confirm  [n] Cancel")
		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(m.theme.Danger).
			Padding(1, 3).
			Render(msg + "\n\n" + hint)
		return lipgloss.NewStyle().Padding(2, 4).Render(box)
	case stateFormSite, stateFormAlert, stateFormUser, stateFormMaint:
		if m.huhForm != nil {
			title := ""
			switch m.state {
			case stateFormSite:
				title = "Add Monitor"
				if m.editID > 0 {
					title = fmt.Sprintf("Edit Monitor #%d", m.editID)
				}
			case stateFormAlert:
				title = "Add Alert"
				if m.editID > 0 {
					title = fmt.Sprintf("Edit Alert #%d", m.editID)
				}
			case stateFormUser:
				title = "Add User"
				if m.editID > 0 {
					title = fmt.Sprintf("Edit User #%d", m.editID)
				}
			case stateFormMaint:
				title = "New Maintenance Window"
			}
			formHeight := m.termHeight - 7
			if formHeight < 5 {
				formHeight = 5
			}
			m.huhForm.WithHeight(formHeight)
			header := titleStyle.Render(title)
			footer := subtleStyle.Render("\n[Esc] Cancel")
			return lipgloss.NewStyle().Padding(1, 2).Render(header + "\n\n" + m.huhForm.View() + "\n" + footer)
		}
		return ""
	case stateDetail:
		return m.viewDetailPanel()
	case stateHistory:
		return m.viewHistoryPanel()
	case stateSLA:
		return m.viewSLAPanel()
	case stateAlertDetail:
		return m.viewAlertDetailPanel()
	default:
		return m.zones.Scan(m.viewDashboard())
	}
}

type dashboardStats struct {
	totalMonitors int
	downCount     int
	lateCount     int
	offlineNodes  int
	activeMaint   int
}

func (m Model) computeStats() dashboardStats {
	allSites := m.engine.GetAllSites()
	var s dashboardStats
	for _, site := range allSites {
		if site.Type == "group" {
			continue
		}
		s.totalMonitors++
		if site.Paused || m.isMonitorInMaintenance(site.ID) {
			continue
		}
		switch site.Status {
		case "DOWN", "SSL EXP":
			s.downCount++
		case "LATE":
			s.lateCount++
		}
	}
	for _, n := range m.nodes {
		if !n.LastSeen.IsZero() && time.Since(n.LastSeen) > 5*time.Minute {
			s.offlineNodes++
		}
	}
	for _, mw := range m.maintenanceWindows {
		now := time.Now()
		if !mw.StartTime.After(now) && (mw.EndTime.IsZero() || mw.EndTime.After(now)) {
			s.activeMaint++
		}
	}
	return s
}

func (m Model) viewDashboard() string {
	stats := m.computeStats()

	header := m.renderTabBar(stats)
	header = m.pulseIndicator() + " " + header

	var content string
	switch m.currentTab {
	case 0:
		content = m.viewSitesTab()
	case 1:
		content = m.viewAlertsTab()
	case 2:
		content = m.viewLogsTab()
	case 3:
		content = m.viewNodesTab()
	case 4:
		content = m.viewMaintTab()
	case 5:
		if m.isAdmin {
			content = m.viewUsersTab()
		}
	}

	content = strings.TrimSpace(content)
	footer := m.renderFooter(stats)

	outerPad := lipgloss.NewStyle().Padding(1, 2)
	_, frameV := outerPad.GetFrameSize()
	availHeight := m.termHeight - frameV
	if availHeight < 5 {
		availHeight = 5
	}

	contentHeight := availHeight - lipgloss.Height(header) - lipgloss.Height(footer) - 2
	if contentHeight < 1 {
		contentHeight = 1
	}
	paddedContent := lipgloss.NewStyle().Height(contentHeight).MaxHeight(contentHeight).Render(content)

	return outerPad.Render(lipgloss.JoinVertical(lipgloss.Top, header, paddedContent, footer))
}

func (m Model) renderTabBar(stats dashboardStats) string {
	var sitesLabel string
	if stats.downCount > 0 {
		sitesLabel = fmt.Sprintf("Sites (%d↓)", stats.downCount)
	} else if stats.lateCount > 0 {
		sitesLabel = fmt.Sprintf("Sites (%d⚠)", stats.lateCount)
	} else if stats.totalMonitors > 0 {
		sitesLabel = fmt.Sprintf("Sites (%d)", stats.totalMonitors)
	} else {
		sitesLabel = "Sites"
	}

	var nodesLabel string
	if stats.offlineNodes > 0 {
		nodesLabel = fmt.Sprintf("Nodes (%d!)", stats.offlineNodes)
	} else if len(m.nodes) > 0 {
		nodesLabel = fmt.Sprintf("Nodes (%d)", len(m.nodes))
	} else {
		nodesLabel = "Nodes"
	}

	var maintLabel string
	if stats.activeMaint > 0 {
		maintLabel = fmt.Sprintf("Maint (%d)", stats.activeMaint)
	} else {
		maintLabel = "Maint"
	}

	tabs := []string{sitesLabel, "Alerts", "Logs", nodesLabel, maintLabel}
	if m.isAdmin {
		tabs = append(tabs, "Users")
	}
	var renderedTabs []string
	for i, t := range tabs {
		var rendered string
		if i == m.currentTab {
			rendered = activeTab.Render(t)
		} else {
			rendered = inactiveTab.Render(t)
		}
		renderedTabs = append(renderedTabs, m.zones.Mark(fmt.Sprintf("tab-%d", i), rendered))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, renderedTabs...)
}

func (m Model) renderFooter(stats dashboardStats) string {
	if m.filterMode {
		cursor := lipgloss.NewStyle().Foreground(m.theme.Accent).Render("│")
		return "\n" + titleStyle.Render("/") + " " + m.filterText + cursor + "  " + subtleStyle.Render("[Enter]Apply [Esc]Clear")
	}

	upCount := stats.totalMonitors - stats.downCount - stats.lateCount
	var upStr string
	if stats.downCount > 0 {
		upStr = dangerStyle.Render(fmt.Sprintf("%d/%d UP", upCount, stats.totalMonitors))
	} else if stats.lateCount > 0 {
		upStr = warnStyle.Render(fmt.Sprintf("%d/%d UP", upCount, stats.totalMonitors))
	} else {
		upStr = specialStyle.Render(fmt.Sprintf("%d/%d UP", upCount, stats.totalMonitors))
	}
	statusParts := []string{upStr}
	if stats.lateCount > 0 {
		statusParts = append(statusParts, warnStyle.Render(fmt.Sprintf("%d LATE", stats.lateCount)))
	}
	if len(m.nodes) > 0 {
		online := 0
		for _, n := range m.nodes {
			if !n.LastSeen.IsZero() && time.Since(n.LastSeen) < 60*time.Second {
				online++
			}
		}
		probeLabel := "probes"
		if online == 1 {
			probeLabel = "probe"
		}
		statusParts = append(statusParts, fmt.Sprintf("%d %s", online, probeLabel))
	}
	statusLine := strings.Join(statusParts, subtleStyle.Render(" · "))

	var keys string
	switch m.currentTab {
	case 0:
		keys = "[/]Filter [n]New [e]Edit [i]Info [d]Del [p]Pause [T]Theme [Tab]Switch [q]Quit"
	case 1:
		keys = "[n]New [e]Edit [i]Info [d]Del [t]Test [T]Theme [Tab]Switch [q]Quit"
	case 2:
		keys = "[↑/↓]Scroll  [PgUp/PgDn]Page  [f]Filter  [T]Theme  [Tab]Switch  [q]Quit"
	case 4:
		keys = "[n]New [x]End [d]Del [T]Theme [Tab]Switch [q]Quit"
	case 5:
		keys = "[n]Add [d]Revoke [T]Theme [Tab]Switch [q]Quit"
	default:
		keys = "[T]Theme [Tab]Switch [q]Quit"
	}

	ver := subtleStyle.Render("v" + m.version)
	footer := statusLine + "  " + subtleStyle.Render(keys) + "  " + ver
	if m.filterText != "" && m.currentTab == 0 {
		footer = subtleStyle.Render(fmt.Sprintf("filter: %s", m.filterText)) + "  " + statusLine + "  " + subtleStyle.Render(keys) + "  " + ver
	}
	return footer
}
