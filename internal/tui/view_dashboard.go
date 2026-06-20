package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"github.com/charmbracelet/lipgloss"
)

func sinApprox(x float64) float64 {
	return math.Sin(x)
}

func (m Model) pulseIndicator() string {
	hasDown := false
	for _, s := range m.sites {
		if !s.Paused && !m.isMonitorInMaintenance(s.ID) && (s.Status == models.StatusDown || s.Status == models.StatusSSLExp) {
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
		msg := m.st.dangerStyle.Render(fmt.Sprintf("Delete %s \"%s\"?", kind, m.deleteName))
		hint := m.st.subtleStyle.Render("[y] Confirm  [n] Cancel")
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
			header := m.st.titleStyle.Render(title)
			footer := m.st.subtleStyle.Render("\n[Esc] Cancel")
			return lipgloss.NewStyle().Padding(1, 2).Render(header + "\n\n" + m.huhForm.View() + "\n" + footer)
		}
		return ""
	case stateDetail:
		return m.zones.Scan(m.viewDetailPanel())
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
		case models.StatusDown, models.StatusSSLExp:
			s.downCount++
		case models.StatusLate:
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
	case tabMonitors:
		showSidebar := m.termWidth >= wideBreakpoint
		if showSidebar {
			availW := m.termWidth - chromePadH
			leftW := availW * 70 / 100
			rightW := availW - leftW
			m.contentWidth = leftW - 2
			monitors := m.viewSitesTab()
			monPanel := m.titledPanel("Monitors", monitors, leftW, !m.detailOpen)
			sidebarContent := m.viewLogsSidebar(rightW-2, m.maxTableRows)
			logPanel := m.titledPanel("Logs", sidebarContent, rightW, false)
			top := lipgloss.JoinHorizontal(lipgloss.Top, monPanel, logPanel)
			if m.detailOpen {
				site := ""
				if m.cursor < len(m.sites) {
					site = m.sites[m.cursor].Name
				}
				detail := m.viewDetailInline(availW - 2)
				detailPanel := m.titledPanel(site, detail, availW, true)
				content = top + "\n" + detailPanel
			} else {
				content = top
			}
		} else {
			m.contentWidth = m.termWidth - 2
			monitors := m.viewSitesTab()
			availW := m.termWidth - chromePadH
			monPanel := m.titledPanel("Monitors", monitors, availW, !m.detailOpen)
			if m.detailOpen {
				site := ""
				if m.cursor < len(m.sites) {
					site = m.sites[m.cursor].Name
				}
				detail := m.viewDetailInline(availW - 2)
				detailPanel := m.titledPanel(site, detail, availW, true)
				content = monPanel + "\n" + detailPanel
			} else {
				content = monPanel
			}
		}
	case tabMaint:
		m.contentWidth = m.termWidth
		content = m.viewMaintTab()
	case tabSettings:
		m.contentWidth = m.termWidth
		content = m.viewSettingsTab()
	}

	content = strings.TrimSpace(content)
	footer := m.renderFooter(stats)

	outerPad := lipgloss.NewStyle().Padding(1, 2)
	_, frameV := outerPad.GetFrameSize()
	availHeight := m.termHeight - frameV
	if availHeight < 5 {
		availHeight = 5
	}

	divW := m.termWidth - chromePadH
	if divW < 40 {
		divW = 40
	}
	tabDivider := m.st.subtleStyle.Render(strings.Repeat("─", divW))

	contentHeight := availHeight - lipgloss.Height(header) - 1 - lipgloss.Height(footer)
	if contentHeight < 1 {
		contentHeight = 1
	}
	paddedContent := lipgloss.NewStyle().Height(contentHeight).MaxHeight(contentHeight).Render(content)

	return outerPad.Render(lipgloss.JoinVertical(lipgloss.Top, header, tabDivider, paddedContent, footer))
}

type tabEntry struct {
	name  string
	count int
	warn  int
}

func (m Model) renderTabBar(stats dashboardStats) string {
	settingsCount := len(m.alerts) + len(m.nodes)
	settingsWarn := stats.offlineNodes
	if m.isAdmin {
		settingsCount += len(m.users)
	}
	tabs := []tabEntry{
		{"Monitors", stats.totalMonitors, stats.downCount + stats.lateCount},
		{"Maint", len(m.maintenanceWindows), stats.activeMaint},
		{"Settings", settingsCount, settingsWarn},
	}

	countStyle := lipgloss.NewStyle().Foreground(m.theme.Muted)

	var renderedTabs []string
	for i, t := range tabs {
		label := t.name
		if t.count > 0 {
			badge := countStyle.Render(fmt.Sprintf(" %d", t.count))
			if t.warn > 0 {
				badge = m.st.dangerStyle.Render(fmt.Sprintf(" %d", t.warn))
			}
			label += badge
		}

		var rendered string
		if i == m.currentTab {
			rendered = m.st.activeTab.Render(label)
		} else {
			rendered = m.st.inactiveTab.Render(label)
		}
		renderedTabs = append(renderedTabs, m.zones.Mark(fmt.Sprintf("tab-%d", i), rendered))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, renderedTabs...)
}

func (m Model) renderFooter(stats dashboardStats) string {
	if m.filterMode {
		cursor := lipgloss.NewStyle().Foreground(m.theme.Accent).Render("│")
		return "\n" + m.st.titleStyle.Render("/") + " " + m.filterText + cursor + "  " + m.st.subtleStyle.Render("[Enter]Apply [Esc]Clear")
	}

	upCount := stats.totalMonitors - stats.downCount - stats.lateCount
	var upStr string
	if stats.downCount > 0 {
		upStr = m.st.dangerStyle.Render(fmt.Sprintf("%d/%d UP", upCount, stats.totalMonitors))
	} else if stats.lateCount > 0 {
		upStr = m.st.warnStyle.Render(fmt.Sprintf("%d/%d UP", upCount, stats.totalMonitors))
	} else {
		upStr = m.st.specialStyle.Render(fmt.Sprintf("%d/%d UP", upCount, stats.totalMonitors))
	}
	statusParts := []string{upStr}
	if stats.lateCount > 0 {
		statusParts = append(statusParts, m.st.warnStyle.Render(fmt.Sprintf("%d LATE", stats.lateCount)))
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
	statusLine := strings.Join(statusParts, m.st.subtleStyle.Render(" · "))

	var keys string
	switch m.currentTab {
	case tabMonitors:
		if m.detailOpen {
			keys = "[i]Close [Enter]Expand [h]History [s]SLA [e]Edit [↑/↓]Select [T]Theme [q]Quit"
		} else {
			keys = "[/]Filter [i]Info [Enter]Detail [n]New [e]Edit [d]Del [p]Pause [T]Theme [Tab]Switch [q]Quit"
		}
	case tabMaint:
		keys = "[n]New [x]End [d]Del [T]Theme [Tab]Switch [q]Quit"
	case tabSettings:
		switch m.settingsSection {
		case sectionAlerts:
			keys = "[n]New [e]Edit [i]Info [d]Del [t]Test [←/→]Section [T]Theme [Tab]Switch [q]Quit"
		case sectionUsers:
			keys = "[n]Add [d]Revoke [←/→]Section [T]Theme [Tab]Switch [q]Quit"
		default:
			keys = "[←/→]Section [T]Theme [Tab]Switch [q]Quit"
		}
	default:
		keys = "[T]Theme [Tab]Switch [q]Quit"
	}

	ver := m.st.subtleStyle.Render("v" + m.version)
	line := statusLine + "  " + m.st.subtleStyle.Render(keys) + "  " + ver
	if m.filterText != "" && m.currentTab == tabMonitors {
		line = m.st.subtleStyle.Render(fmt.Sprintf("filter: %s", m.filterText)) + "  " + statusLine + "  " + m.st.subtleStyle.Render(keys) + "  " + ver
	}

	divW := m.termWidth - chromePadH
	if divW < 40 {
		divW = 40
	}
	return m.st.subtleStyle.Render(strings.Repeat("─", divW)) + "\n" + line
}
