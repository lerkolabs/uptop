package tui

import (
	"fmt"
	"strings"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
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
	end := maxLines
	if end > len(windows) {
		end = len(windows)
	}

	selectedVisual := -1
	if m.focusedPanel == panelMaint {
		selectedVisual = m.maintCursor
	}

	var rows [][]string
	for i := 0; i < end; i++ {
		mw := windows[i]
		isActive := !mw.StartTime.After(now) && (mw.EndTime.IsZero() || mw.EndTime.After(now))

		var icon string
		if isActive {
			icon = m.st.specialStyle.Render("●")
		} else {
			icon = m.st.warnStyle.Render("○")
		}

		monName := "All Monitors"
		if mw.MonitorID > 0 {
			monName = m.monitorNameByID(mw.MonitorID)
		}

		var status string
		if isActive {
			if mw.EndTime.IsZero() {
				status = "indefinite"
			} else {
				status = fmtDuration(time.Until(mw.EndTime)) + " left"
			}
		} else {
			status = "starts " + mw.StartTime.Format("Jan 02 15:04")
		}

		rows = append(rows, []string{icon, mw.Title, monName, status})
	}

	colWidths := []int{3, 0, 0, 0}
	remaining := width - colWidths[0] - 6
	colWidths[1] = remaining * 40 / 100
	colWidths[2] = remaining * 30 / 100
	colWidths[3] = remaining - colWidths[1] - colWidths[2]

	t := table.New().
		Border(lipgloss.HiddenBorder()).
		Width(width).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			isSelected := row == selectedVisual
			base := m.st.tableCellStyle
			if row%2 == 1 {
				base = m.st.tableZebraStyle
			}
			if isSelected {
				base = m.st.tableSelectedStyle
			}
			if col < len(colWidths) && colWidths[col] > 0 {
				base = base.Width(colWidths[col]).MaxWidth(colWidths[col])
			}
			return base
		})

	return t.Render()
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
