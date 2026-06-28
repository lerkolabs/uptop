package tui

import (
	"fmt"
	"strings"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/monitor"
	"github.com/charmbracelet/lipgloss"
)

const detailTwoColMinWidth = 80

func (m Model) viewDetailInline(width int) string {
	if m.cursor >= len(m.sites) {
		return ""
	}
	site := m.sites[m.cursor]
	hist, _ := m.engine.GetHistory(site.ID)

	if width < detailTwoColMinWidth {
		return m.viewDetailSingleCol(site, hist, width)
	}
	return m.viewDetailTwoCol(site, hist, width)
}

func (m Model) viewDetailTwoCol(site models.Site, hist monitor.SiteHistory, width int) string {
	leftW := width * 55 / 100
	rightW := width - leftW - 3 // 3 for " │ " divider

	left := m.detailLeftCol(site, hist, leftW)
	right := m.detailRightCol(site, hist, rightW)

	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")

	lineCount := len(leftLines)
	if len(rightLines) > lineCount {
		lineCount = len(rightLines)
	}
	for len(leftLines) < lineCount {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < lineCount {
		rightLines = append(rightLines, "")
	}

	divChar := m.st.subtleStyle.Render("│")
	leftStyle := lipgloss.NewStyle().Width(leftW).MaxWidth(leftW)
	rightStyle := lipgloss.NewStyle().Width(rightW).MaxWidth(rightW)

	var b strings.Builder
	for i := range lineCount {
		l := leftStyle.Render(leftLines[i])
		r := rightStyle.Render(rightLines[i])
		b.WriteString(l + " " + divChar + " " + r + "\n")
	}

	keys := m.st.subtleStyle.Render("[h] History  [s] SLA  [e] Edit  [esc] Close")
	b.WriteString("  " + keys + "\n")

	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(b.String())
}

func (m Model) detailLeftCol(site models.Site, hist monitor.SiteHistory, width int) string {
	var b strings.Builder

	if len(hist.Latencies) > 0 {
		chartW := width - 2
		if chartW < 20 {
			chartW = 20
		}
		chart := m.latencyChart(hist.Latencies, hist.Statuses, chartW, 3)
		if chart != "" {
			b.WriteString(chart + "\n")
		}
	}

	if len(m.detailDailyDays) > 0 && m.detailChangesSiteID == site.ID {
		timelineW := width - 2
		if timelineW < 20 {
			timelineW = 20
		}
		b.WriteString("  " + m.st.subtleStyle.Render("30d") + " " + m.uptimeTimeline(m.detailDailyDays, timelineW) + "\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

func (m Model) detailRightCol(site models.Site, hist monitor.SiteHistory, width int) string {
	dot := m.st.subtleStyle.Render("  ·  ")
	label := m.st.subtleStyle

	var b strings.Builder

	// Line 1: status + latency + last check
	status := m.fmtStatus(site.Status, site.Paused, m.isMonitorInMaintenance(site.ID))
	parts := []string{status}
	if site.Latency > 0 {
		parts = append(parts, m.fmtLatency(site.Latency))
	}
	if !site.LastCheck.IsZero() {
		parts = append(parts, m.fmtTimeAgo(site.LastCheck))
	}
	b.WriteString(strings.Join(parts, dot) + "\n")

	// Line 2: type-specific details
	typeParts := m.detailTypeLine(site)
	if len(typeParts) > 0 {
		b.WriteString(strings.Join(typeParts, dot) + "\n")
	}

	// Line 3: uptime + retries
	uptimeParts := []string{label.Render("Uptime") + " " + m.fmtUptime(hist.Statuses)}
	if site.Type != "group" && site.MaxRetries > 0 {
		uptimeParts = append(uptimeParts, label.Render("Retries")+" "+m.fmtRetries(site))
	}
	b.WriteString(strings.Join(uptimeParts, dot) + "\n")

	// Error line (if down/broken)
	if (site.Status == models.StatusDown || site.Status == models.StatusSSLExp ||
		site.Status == models.StatusLate || site.Status == models.StatusStale) && site.LastError != "" {
		errW := width - 8
		if errW < 20 {
			errW = 20
		}
		b.WriteString(label.Render("Error") + " " + m.st.dangerStyle.Render(limitStr(site.LastError, errW)) + "\n")
	}

	// Blank line before state changes
	b.WriteString("\n")

	// State changes (one per line, compact)
	var stateChanges []models.StateChange
	if m.detailChangesSiteID == site.ID {
		stateChanges = m.detailChanges
	}
	if len(stateChanges) > 0 {
		limit := 5
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
				reasonW := width - 30
				if reasonW < 15 {
					reasonW = 15
				}
				entry += "  " + m.st.dangerStyle.Render(limitStr(sc.ErrorReason, reasonW))
			}
			b.WriteString(entry + "\n")
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

func (m Model) detailTypeLine(site models.Site) []string {
	label := m.st.subtleStyle
	var parts []string

	switch site.Type {
	case "http":
		if site.StatusCode > 0 {
			codeStr := fmt.Sprintf("HTTP %d", site.StatusCode)
			if site.StatusCode >= httpErrorThreshold {
				parts = append(parts, m.st.dangerStyle.Render(codeStr))
			} else {
				parts = append(parts, m.st.specialStyle.Render(codeStr))
			}
		}
		if site.CheckSSL && site.HasSSL {
			days := int(time.Until(site.CertExpiry).Hours() / 24)
			sslStr := fmt.Sprintf("SSL %dd", days)
			switch {
			case days <= 0:
				parts = append(parts, m.st.dangerStyle.Render("SSL EXPIRED"))
			case days <= site.ExpiryThreshold:
				parts = append(parts, m.st.warnStyle.Render(sslStr))
			default:
				parts = append(parts, m.st.specialStyle.Render(sslStr))
			}
		}
		if site.URL != "" {
			parts = append(parts, label.Render(limitStr(site.URL, 40)))
		}
	case "push":
		parts = append(parts, label.Render("Push"))
		if site.Interval > 0 {
			parts = append(parts, label.Render(fmt.Sprintf("every %s", fmtDuration(time.Duration(site.Interval)*time.Second))))
		}
		if !site.LastSuccessAt.IsZero() {
			parts = append(parts, label.Render("last")+" "+m.fmtTimeAgo(site.LastSuccessAt))
		}
	case "ping":
		parts = append(parts, label.Render("Ping"))
		if site.Hostname != "" {
			parts = append(parts, label.Render(site.Hostname))
		}
	case "port":
		parts = append(parts, label.Render("Port"))
		if site.Hostname != "" {
			target := site.Hostname
			if site.Port > 0 {
				target = fmt.Sprintf("%s:%d", site.Hostname, site.Port)
			}
			parts = append(parts, label.Render(target))
		}
	case "dns":
		parts = append(parts, label.Render("DNS"))
		if site.DNSResolveType != "" {
			parts = append(parts, label.Render(site.DNSResolveType))
		}
		if site.DNSServer != "" {
			parts = append(parts, label.Render(site.DNSServer))
		}
	}

	return parts
}

// viewDetailSingleCol is the narrow-terminal fallback (original stacked layout).
func (m Model) viewDetailSingleCol(site models.Site, hist monitor.SiteHistory, width int) string {
	var b strings.Builder
	dot := m.st.subtleStyle.Render("  ·  ")

	status := m.fmtStatus(site.Status, site.Paused, m.isMonitorInMaintenance(site.ID))
	parts := []string{status}
	if site.Latency > 0 {
		parts = append(parts, m.fmtLatency(site.Latency))
	}
	parts = append(parts, fmt.Sprintf("Uptime %s", m.fmtUptime(hist.Statuses)))
	if !site.LastCheck.IsZero() {
		parts = append(parts, m.fmtTimeAgo(site.LastCheck))
	}
	b.WriteString("  " + strings.Join(parts, dot) + "\n")

	if (site.Status == models.StatusDown || site.Status == models.StatusSSLExp ||
		site.Status == models.StatusLate || site.Status == models.StatusStale) && site.LastError != "" {
		errW := width - 12
		if errW < 20 {
			errW = 20
		}
		b.WriteString("  " + m.st.subtleStyle.Render("Error") + "  " + m.st.dangerStyle.Render(limitStr(site.LastError, errW)) + "\n")
	}

	var stateChanges []models.StateChange
	if m.detailChangesSiteID == site.ID {
		stateChanges = m.detailChanges
	}
	if len(stateChanges) > 0 {
		limit := 3
		if len(stateChanges) < limit {
			limit = len(stateChanges)
		}
		var scParts []string
		for _, sc := range stateChanges[:limit] {
			ago := fmtDuration(time.Since(sc.ChangedAt))
			arrow := m.st.subtleStyle.Render("→")
			from := m.fmtStatusWord(sc.FromStatus)
			to := m.fmtStatusWord(sc.ToStatus)
			entry := from + " " + arrow + " " + to + " " + m.st.subtleStyle.Render(ago+" ago")
			if sc.ErrorReason != "" {
				entry += "  " + m.st.dangerStyle.Render(limitStr(sc.ErrorReason, 30))
			}
			scParts = append(scParts, entry)
		}
		b.WriteString("  " + strings.Join(scParts, dot) + "\n")
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

	if len(m.detailDailyDays) > 0 && m.detailChangesSiteID == site.ID {
		timelineW := width - 4
		if timelineW < 20 {
			timelineW = 20
		}
		b.WriteString("  " + m.st.subtleStyle.Render("30d") + " " + m.uptimeTimeline(m.detailDailyDays, timelineW) + "\n")
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
