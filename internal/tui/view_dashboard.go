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
		switch m.deleteKind {
		case "maint":
			kind = "maintenance window"
		case "alert":
			kind = "alert"
		case "user":
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
	case stateLogs:
		return m.viewLogsFullscreen()
	case stateAlertDetail:
		return m.viewAlertDetailPanel()
	case stateSettings:
		return m.viewSettingsOverlay()
	case stateMaintDetail:
		return m.viewMaintDetailPanel()
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
		if !n.LastSeen.IsZero() && time.Since(n.LastSeen) > nodeStaleThreshold {
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

func (m Model) viewMonitorsLayout() string {
	availW := m.termWidth - chromePadH
	wide := m.termWidth >= wideBreakpoint

	showDetail := m.detailOpen && wide

	var detailW, monW int
	if showDetail {
		monW = availW * 60 / 100
		detailW = availW - monW
	} else {
		monW = availW
	}

	m.contentWidth = monW - 2

	monitors := m.viewSitesTab()
	monPanel := m.zones.Mark("panel-monitors", m.titledPanel("Monitors", monitors, monW, m.focusedPanel == panelMonitors))

	var topParts []string
	topParts = append(topParts, monPanel)
	if showDetail {
		title := ""
		if m.cursor < len(m.sites) {
			title = m.sites[m.cursor].Name
		}
		switch m.detailMode {
		case detailSLA:
			title = "SLA · " + title
		case detailHistory:
			title = "History · " + title
		}
		monHeight := lipgloss.Height(monPanel)
		detail := m.viewDetailInline(detailW-2, monHeight)
		footer := m.detailFooter(detailW - 2)
		detailPanel := m.zones.Mark("panel-detail", m.titledPanelH(title, detail, footer, detailW, monHeight, m.detailScrollOffset, m.focusedPanel == panelDetail))
		topParts = append(topParts, detailPanel)
	}

	top := lipgloss.JoinHorizontal(lipgloss.Top, topParts...)

	switch m.bottomPanel {
	case bottomLogs:
		logContent := m.viewLogsStrip(availW-2, logsStripHeight-2)
		logPanel := m.zones.Mark("panel-logs", m.titledPanel("Logs", logContent, availW, m.focusedPanel == panelLogs))
		return top + "\n" + logPanel
	case bottomMaint:
		maintContent := m.viewMaintStrip(availW-2, logsStripHeight-2)
		maintPanel := m.zones.Mark("panel-maint", m.titledPanel("Maint", maintContent, availW, m.focusedPanel == panelMaint))
		return top + "\n" + maintPanel
	}
	return top
}

func (m Model) viewDashboard() string {
	stats := m.computeStats()

	header := m.renderStatusLine(stats)

	content := m.viewMonitorsLayout()

	content = strings.TrimSpace(content)
	footer := m.renderFooter(stats)

	outerPad := lipgloss.NewStyle().Padding(1, 2)
	_, frameV := outerPad.GetFrameSize()
	availHeight := m.termHeight - frameV
	if availHeight < 5 {
		availHeight = 5
	}

	contentHeight := availHeight - lipgloss.Height(header) - lipgloss.Height(footer)
	if contentHeight < 1 {
		contentHeight = 1
	}
	paddedContent := lipgloss.NewStyle().Height(contentHeight).MaxHeight(contentHeight).Render(content)

	return outerPad.Render(lipgloss.JoinVertical(lipgloss.Top, header, paddedContent, footer))
}

func (m Model) renderStatusLine(stats dashboardStats) string {
	dot := m.st.subtleStyle.Render(" · ")

	upCount := stats.totalMonitors - stats.downCount - stats.lateCount
	var upStr string
	if stats.downCount > 0 {
		upStr = m.st.dangerStyle.Render(fmt.Sprintf("%d/%d UP", upCount, stats.totalMonitors))
	} else if stats.lateCount > 0 {
		upStr = m.st.warnStyle.Render(fmt.Sprintf("%d/%d UP", upCount, stats.totalMonitors))
	} else {
		upStr = m.st.specialStyle.Render(fmt.Sprintf("%d/%d UP", upCount, stats.totalMonitors))
	}

	parts := []string{m.pulseIndicator() + " " + upStr}

	if stats.lateCount > 0 {
		parts = append(parts, m.st.warnStyle.Render(fmt.Sprintf("%d late", stats.lateCount)))
	}
	if stats.activeMaint > 0 {
		parts = append(parts, m.st.maintStyle.Render(fmt.Sprintf("%d maint", stats.activeMaint)))
	}
	if len(m.nodes) > 0 {
		online := 0
		for _, n := range m.nodes {
			if !n.LastSeen.IsZero() && time.Since(n.LastSeen) < nodeOnlineThreshold {
				online++
			}
		}
		label := "probes"
		if online == 1 {
			label = "probe"
		}
		parts = append(parts, fmt.Sprintf("%d %s", online, label))
	}

	left := strings.Join(parts, dot)
	ver := m.st.subtleStyle.Render("v" + m.version)

	padW := m.termWidth - chromePadH - lipgloss.Width(left) - lipgloss.Width(ver)
	if padW < 2 {
		padW = 2
	}

	return left + strings.Repeat(" ", padW) + ver
}

func (m Model) hotkey(key, desc string) string {
	k := lipgloss.NewStyle().Foreground(m.theme.Accent).Render(key)
	d := m.st.subtleStyle.Render(desc)
	return k + " " + d
}

func (m Model) renderFooter(_ dashboardStats) string {
	dot := m.st.subtleStyle.Render(" · ")

	if m.filterMode {
		cursor := lipgloss.NewStyle().Foreground(m.theme.Accent).Render("│")
		keys := m.hotkey("Enter", "Apply") + dot + m.hotkey("Esc", "Clear")
		return "\n" + m.st.titleStyle.Render("/") + " " + m.filterText + cursor + "  " + keys
	}

	var parts []string
	if m.focusedPanel == panelMaint {
		parts = []string{m.hotkey("n", "New"), m.hotkey("Enter", "Detail"), m.hotkey("x", "End"), m.hotkey("d", "Del"), m.hotkey("m/Esc", "Back")}
	} else if m.focusedPanel == panelLogs {
		parts = []string{m.hotkey("↑/↓", "Scroll"), m.hotkey("Enter", "Expand"), m.hotkey("l/Esc", "Back")}
	} else if m.detailOpen && m.detailMode == detailSLA {
		parts = []string{m.hotkey("1-4", "Period"), m.hotkey("Esc", "Back")}
	} else if m.detailOpen && m.detailMode == detailHistory {
		parts = []string{m.hotkey("Esc", "Back")}
	} else if m.detailOpen {
		parts = []string{m.hotkey("Enter", "Close"), m.hotkey("h", "History"), m.hotkey("s", "SLA"), m.hotkey("e", "Edit")}
	} else {
		parts = []string{m.hotkey("/", "Filter"), m.hotkey("Enter", "Detail")}
		if m.cursor < len(m.sites) && m.sites[m.cursor].Type == "group" {
			if m.collapsed[m.sites[m.cursor].ID] {
				parts = append(parts, m.hotkey("Space", "Expand"))
			} else {
				parts = append(parts, m.hotkey("Space", "Collapse"))
			}
		}
		parts = append(parts, m.hotkey("n", "New"), m.hotkey("e", "Edit"), m.hotkey("d", "Del"))
	}
	parts = append(parts, m.hotkey("m", "Maint"), m.hotkey("l", "Logs"), m.hotkey("S", "Settings"), m.hotkey("T", "Theme"), m.hotkey("q", "Quit"))

	line := strings.Join(parts, dot)
	if m.filterText != "" {
		line = m.st.subtleStyle.Render(fmt.Sprintf("filter: %s  ", m.filterText)) + line
	}

	return "\n" + line
}
