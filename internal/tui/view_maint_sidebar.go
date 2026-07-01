package tui

import (
	"fmt"
	"strings"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) viewMaintDetailPanel() string {
	mw := m.findMaintWindow(m.maintDetailID)
	if mw == nil {
		return lipgloss.NewStyle().Padding(1, 2).Render(m.st.subtleStyle.Render("Maintenance window not found"))
	}

	var b strings.Builder

	header := "  " + m.st.subtleStyle.Render("Maintenance >") + " " + m.st.titleStyle.Render(mw.Title)
	b.WriteString(header + "\n")
	b.WriteString(m.divider() + "\n")

	row := func(label, value string) {
		fmt.Fprintf(&b, "  %-16s %s\n", m.st.subtleStyle.Render(label), value)
	}

	maintType := m.st.maintStyle.Render("maintenance")
	if mw.Type == "incident" {
		maintType = m.st.dangerStyle.Render("incident")
	}
	row("Type", maintType)

	now := time.Now()
	if mw.StartTime.After(now) {
		row("Status", m.st.warnStyle.Render("SCHEDULED"))
	} else if !mw.EndTime.IsZero() && mw.EndTime.Before(now) {
		row("Status", m.st.subtleStyle.Render("ENDED"))
	} else {
		row("Status", m.st.specialStyle.Render("ACTIVE"))
	}

	if mw.MonitorID == 0 {
		row("Monitors", "All")
	} else {
		name := fmt.Sprintf("#%d", mw.MonitorID)
		allSites := m.engine.GetAllSites()
		for _, s := range allSites {
			if s.ID == mw.MonitorID {
				name = s.Name
				break
			}
		}
		row("Monitors", name)
	}

	row("Started", mw.StartTime.Format("2006-01-02 15:04"))
	if mw.EndTime.IsZero() {
		row("Ends", m.st.subtleStyle.Render("indefinite"))
	} else {
		row("Ends", mw.EndTime.Format("2006-01-02 15:04"))
		if mw.EndTime.After(now) && !mw.StartTime.After(now) {
			remaining := time.Until(mw.EndTime)
			row("Remaining", fmtDuration(remaining))
		}
		dur := mw.EndTime.Sub(mw.StartTime)
		row("Duration", fmtDuration(dur))
	}

	if mw.Description != "" {
		b.WriteString("\n")
		row("Description", mw.Description)
	}

	b.WriteString("\n" + m.divider() + "\n")

	var keys []string
	isActive := !mw.StartTime.After(now) && (mw.EndTime.IsZero() || mw.EndTime.After(now))
	if isActive {
		keys = append(keys, "[x] End")
	}
	keys = append(keys, "[d] Delete", "[q/Esc] Back")
	b.WriteString("  " + m.st.subtleStyle.Render(strings.Join(keys, "  ")))

	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}

func (m Model) monitorNameByID(id int) string {
	for _, s := range m.engine.GetAllSites() {
		if s.ID == id {
			return s.Name
		}
	}
	return fmt.Sprintf("#%d", id)
}

func (m Model) activeMaintWindows() []models.MaintenanceWindow {
	now := time.Now()
	var out []models.MaintenanceWindow
	for _, mw := range m.maintenanceWindows {
		if !mw.EndTime.IsZero() && mw.EndTime.Before(now) {
			continue
		}
		out = append(out, mw)
	}
	return out
}

func (m Model) viewMaintStrip(width, maxLines int) string {
	windows := m.activeMaintWindows()
	if len(windows) == 0 {
		return m.st.subtleStyle.Render("  No active maintenance")
	}

	now := time.Now()
	titleW := 30
	monW := 20
	if width < 80 {
		titleW = width / 3
		monW = width / 5
	}

	end := maxLines
	if end > len(windows) {
		end = len(windows)
	}

	style := lipgloss.NewStyle().Width(width).MaxWidth(width)
	var lines []string
	for i := 0; i < end; i++ {
		mw := windows[i]
		selected := m.focusedPanel == panelMaint && i == m.maintCursor

		isActive := !mw.StartTime.After(now) && (mw.EndTime.IsZero() || mw.EndTime.After(now))

		var icon string
		if isActive {
			icon = m.st.specialStyle.Render("●")
		} else {
			icon = m.st.warnStyle.Render("○")
		}

		title := limitStr(mw.Title, titleW)

		monName := "All Monitors"
		if mw.MonitorID > 0 {
			monName = m.monitorNameByID(mw.MonitorID)
		}
		monName = limitStr(monName, monW)

		var status string
		if isActive {
			if mw.EndTime.IsZero() {
				status = m.st.subtleStyle.Render("active · indefinite")
			} else {
				status = m.st.subtleStyle.Render("active · " + fmtDuration(time.Until(mw.EndTime)) + " left")
			}
		} else {
			status = m.st.subtleStyle.Render("scheduled · " + mw.StartTime.Format("Jan 02 15:04"))
		}

		line := fmt.Sprintf(" %s %-*s  %-*s  %s", icon, titleW, title, monW, m.st.subtleStyle.Render(monName), status)

		if selected {
			line = m.st.tableSelectedStyle.Render(lipgloss.NewStyle().Width(width).Render(line))
		}

		lines = append(lines, line)
	}

	if len(windows) > end {
		more := fmt.Sprintf("  %d more", len(windows)-end)
		lines = append(lines, m.st.subtleStyle.Render(more))
	}

	return style.Render(strings.Join(lines, "\n"))
}

func (m *Model) scrollMaintCursor(delta int) {
	windows := m.activeMaintWindows()
	total := len(windows)
	if total == 0 {
		return
	}
	m.maintCursor += delta
	if m.maintCursor < 0 {
		m.maintCursor = 0
	}
	if m.maintCursor >= total {
		m.maintCursor = total - 1
	}
}
