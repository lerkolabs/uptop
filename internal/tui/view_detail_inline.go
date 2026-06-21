package tui

import (
	"fmt"
	"strings"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) viewDetailInline(width int) string {
	if m.cursor >= len(m.sites) {
		return ""
	}
	site := m.sites[m.cursor]
	hist, _ := m.engine.GetHistory(site.ID)

	var b strings.Builder

	status := m.fmtStatus(site.Status, site.Paused, m.isMonitorInMaintenance(site.ID))
	latency := m.fmtLatency(site.Latency)
	uptime := m.fmtUptime(hist.Statuses)

	line1Parts := []string{status}
	if site.Latency > 0 {
		line1Parts = append(line1Parts, latency)
	}
	line1Parts = append(line1Parts, fmt.Sprintf("Uptime %s", uptime))
	if !site.LastCheck.IsZero() {
		line1Parts = append(line1Parts, fmt.Sprintf("Checked %s", m.fmtTimeAgo(site.LastCheck)))
	}
	b.WriteString("  " + strings.Join(line1Parts, m.st.subtleStyle.Render("  ·  ")) + "\n")

	if (site.Status == models.StatusDown || site.Status == models.StatusSSLExp ||
		site.Status == models.StatusLate || site.Status == models.StatusStale) && site.LastError != "" {
		errW := width - 12
		if errW < 20 {
			errW = 20
		}
		errMsg := limitStr(site.LastError, errW)
		b.WriteString("  " + m.st.subtleStyle.Render("Error") + "  " + m.st.dangerStyle.Render(errMsg) + "\n")
	}

	var stateChanges []models.StateChange
	if m.detailChangesSiteID == site.ID {
		stateChanges = m.detailChanges
	}
	if len(stateChanges) > 0 {
		var parts []string
		limit := 3
		if len(stateChanges) < limit {
			limit = len(stateChanges)
		}
		for _, sc := range stateChanges[:limit] {
			ago := fmtDuration(time.Since(sc.ChangedAt))
			arrow := m.st.subtleStyle.Render("→")
			from := m.fmtStatusWord(sc.FromStatus)
			to := m.fmtStatusWord(sc.ToStatus)
			entry := from + " " + arrow + " " + to + " " + m.st.subtleStyle.Render(ago+" ago")
			if sc.ErrorReason != "" {
				entry += "  " + m.st.dangerStyle.Render(limitStr(sc.ErrorReason, 30))
			}
			parts = append(parts, entry)
		}
		b.WriteString("  " + strings.Join(parts, m.st.subtleStyle.Render("  ·  ")) + "\n")
	}

	if len(hist.Latencies) > 0 {
		chartW := width - 4
		if chartW < 20 {
			chartW = 20
		}
		chart := m.latencyChart(hist.Latencies, hist.Statuses, chartW, 3)
		if chart != "" {
			b.WriteString(chart + "\n")
		}
	}

	keys := m.st.subtleStyle.Render("[h] History  [s] SLA  [e] Edit  [esc] Close")
	b.WriteString("  " + keys + "\n")

	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(b.String())
}

func (m Model) fmtStatusWord(status string) string {
	switch status {
	case "DOWN":
		return m.st.dangerStyle.Render("DOWN")
	case "UP":
		return m.st.specialStyle.Render("UP")
	default:
		return m.st.subtleStyle.Render(status)
	}
}
