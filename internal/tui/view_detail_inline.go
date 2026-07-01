package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/monitor"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) viewDetailInline(width, height int) string {
	if m.cursor >= len(m.sites) {
		return ""
	}
	switch m.detailMode {
	case detailSLA:
		return m.viewSLASidebar(width, height)
	case detailHistory:
		return m.viewHistorySidebar(width, height)
	default:
		site := m.sites[m.cursor]
		hist, _ := m.engine.GetHistory(site.ID)
		return m.buildDetailContent(site, hist, width, false)
	}
}

func (m Model) viewDetailFullscreen() string {
	if m.cursor >= len(m.sites) {
		return ""
	}

	totalW := m.termWidth - chromePadH
	site := m.sites[m.cursor]

	var title string
	switch m.detailMode {
	case detailSLA:
		title = "SLA · " + site.Name
	case detailHistory:
		title = "History · " + site.Name
	default:
		title = site.Name
	}

	var breadcrumb string
	if site.ParentID > 0 {
		for _, s := range m.sites {
			if s.ID == site.ParentID {
				breadcrumb = m.st.subtleStyle.Render("Monitors > "+s.Name+" > ") + m.st.titleStyle.Render(site.Name)
				break
			}
		}
	}
	if breadcrumb == "" {
		breadcrumb = m.st.subtleStyle.Render("Monitors > ") + m.st.titleStyle.Render(title)
	}

	header := "  " + breadcrumb + "\n" + m.divider()

	var content string
	switch m.detailMode {
	case detailSLA:
		content = m.viewSLASidebar(totalW, 0)
	case detailHistory:
		content = m.viewHistorySidebar(totalW, 0)
	default:
		hist, _ := m.engine.GetHistory(site.ID)
		content = m.buildDetailContent(site, hist, totalW, true)
	}

	footer := m.divider() + "\n" + m.detailFooter(totalW)

	contentLines := strings.Split(content, "\n")
	contentH := m.termHeight - 8
	if contentH < 5 {
		contentH = 5
	}

	if m.detailScrollOffset > len(contentLines)-contentH {
		m.detailScrollOffset = len(contentLines) - contentH
	}
	if m.detailScrollOffset < 0 {
		m.detailScrollOffset = 0
	}

	end := m.detailScrollOffset + contentH
	if end > len(contentLines) {
		end = len(contentLines)
	}
	visible := strings.Join(contentLines[m.detailScrollOffset:end], "\n")

	return lipgloss.NewStyle().Padding(1, 2).Render(
		header + "\n" + visible + "\n" + footer)
}

func (m Model) buildDetailContent(site models.Site, hist monitor.SiteHistory, width int, fullscreen bool) string {
	dot := m.st.subtleStyle.Render(" · ")
	label := m.st.subtleStyle
	innerW := width - 4
	if innerW < 20 {
		innerW = 20
	}

	var b strings.Builder

	// Status + latency + last check + state since
	status := m.fmtStatus(site.Status, site.Paused, m.isMonitorInMaintenance(site.ID))
	statusParts := []string{status}
	if site.Latency > 0 {
		statusParts = append(statusParts, m.fmtLatency(site.Latency))
	}
	if !site.LastCheck.IsZero() {
		statusParts = append(statusParts, m.fmtTimeAgo(site.LastCheck))
	}
	if !site.StatusChangedAt.IsZero() {
		dur := time.Since(site.StatusChangedAt)
		statusParts = append(statusParts, label.Render("for")+" "+fmtDuration(dur))
	}
	b.WriteString("  " + strings.Join(statusParts, dot) + "\n")

	// Type-specific details
	typeParts := m.detailTypeLine(site)
	if len(typeParts) > 0 {
		b.WriteString("  " + strings.Join(typeParts, dot) + "\n")
	}

	// Extended endpoint fields
	m.writeEndpointFields(&b, site, label, innerW, fullscreen)

	// Uptime + retries + last success
	uptimeStr := m.fmtUptime(hist.Statuses)
	if m.isMonitorInMaintenance(site.ID) {
		uptimeStr = m.st.subtleStyle.Render("—")
	}
	uptimeParts := []string{label.Render("Uptime") + " " + uptimeStr}
	if site.Type != "group" && site.MaxRetries > 0 {
		uptimeParts = append(uptimeParts, label.Render("Retries")+" "+m.fmtRetries(site))
	}
	if site.Type != "push" && !site.LastSuccessAt.IsZero() {
		uptimeParts = append(uptimeParts, label.Render("Last OK")+" "+m.fmtTimeAgo(site.LastSuccessAt))
	}
	b.WriteString("  " + strings.Join(uptimeParts, dot) + "\n")

	// Maintenance window name
	if m.isMonitorInMaintenance(site.ID) {
		for _, mw := range m.maintenanceWindows {
			if mw.Type == "maintenance" && (mw.MonitorID == 0 || mw.MonitorID == site.ID || mw.MonitorID == site.ParentID) {
				b.WriteString("  " + label.Render("Maint") + " " + m.st.maintStyle.Render(mw.Title) + "\n")
				break
			}
		}
	}

	// Error line
	if (site.Status == models.StatusDown || site.Status == models.StatusSSLExp ||
		site.Status == models.StatusLate || site.Status == models.StatusStale) && site.LastError != "" {
		errW := innerW
		if errW < 20 {
			errW = 20
		}
		b.WriteString("  " + label.Render("Error") + " " + m.st.dangerStyle.Render(limitStr(site.LastError, errW)) + "\n")
	}

	// Connection chain
	if (site.Status == models.StatusDown || site.Status == models.StatusSSLExp) && site.LastError != "" {
		chain := connectionChain(site.LastError, site.Type, site.StatusCode, strings.HasPrefix(site.URL, "https"))
		if len(chain) > 0 {
			b.WriteString("\n")
			for _, step := range chain {
				var icon string
				switch step.Status {
				case stepPassed:
					icon = m.st.specialStyle.Render("✓")
				case stepFailed:
					icon = m.st.dangerStyle.Render("✗")
				case stepSkipped:
					icon = m.st.subtleStyle.Render("·")
				}
				line := fmt.Sprintf("  %s %-16s", icon, step.Name)
				if step.Detail != "" {
					switch step.Status {
					case stepFailed:
						line += " " + m.st.dangerStyle.Render(step.Detail)
					case stepSkipped:
						line += " " + m.st.subtleStyle.Render(step.Detail)
					}
				}
				b.WriteString(line + "\n")
			}
		}
	}

	b.WriteString("\n")

	// Probe results
	probeResults := m.engine.GetProbeResults(site.ID)
	if len(probeResults) > 0 {
		nodeIDs := make([]string, 0, len(probeResults))
		for id := range probeResults {
			nodeIDs = append(nodeIDs, id)
		}
		sort.Strings(nodeIDs)
		for _, nodeID := range nodeIDs {
			result := probeResults[nodeID]
			probeStatus := m.st.specialStyle.Render("UP")
			if !result.IsUp {
				probeStatus = m.st.dangerStyle.Render("DN")
			}
			latency := time.Duration(result.LatencyNs).Milliseconds()
			ago := time.Since(result.CheckedAt).Truncate(time.Second)
			line := fmt.Sprintf("  %-14s %s  %dms  %s ago", nodeID, probeStatus, latency, ago)
			if !result.IsUp && result.ErrorReason != "" {
				line += "  " + m.st.dangerStyle.Render(result.ErrorReason)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}

	// Latency chart
	if len(hist.Latencies) > 0 {
		chart := m.latencyChart(hist.Latencies, hist.Statuses, innerW, 3)
		if chart != "" {
			b.WriteString(chart + "\n")
		}
	}

	// 30d uptime timeline
	if len(m.detailDailyDays) > 0 && m.detailChangesSiteID == site.ID {
		b.WriteString("  " + label.Render("30d") + " " + m.uptimeTimeline(m.detailDailyDays, innerW) + "\n")
	}

	// Sparkline + min/avg/max
	if site.Type != "push" && len(hist.Latencies) > 0 {
		b.WriteString("  " + m.latencySparkline(hist.Latencies, hist.Statuses, innerW, nil) + "\n")
		var minL, maxL, total time.Duration
		count := 0
		for i, l := range hist.Latencies {
			if i < len(hist.Statuses) && !hist.Statuses[i] {
				continue
			}
			if count == 0 {
				minL, maxL = l, l
			} else if l < minL {
				minL = l
			} else if l > maxL {
				maxL = l
			}
			total += l
			count++
		}
		if count > 0 {
			avg := total / time.Duration(count)
			fmt.Fprintf(&b, "  %s %dms  %s %dms  %s %dms\n",
				label.Render("Min"), minL.Milliseconds(),
				label.Render("Avg"), avg.Milliseconds(),
				label.Render("Max"), maxL.Milliseconds())
		}
	}

	// Latency histogram
	if site.Type != "push" && len(hist.Latencies) > 5 {
		histContent := m.latencyHistogram(hist.Latencies, hist.Statuses, innerW)
		if histContent != "" {
			b.WriteString("\n")
			b.WriteString(histContent)
		}
	}

	b.WriteString("\n")

	// State changes
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
			entry := from + " " + arrow + " " + to + " " + label.Render(ago+" ago")
			if sc.ErrorReason != "" {
				reasonW := innerW - 25
				if reasonW < 15 {
					reasonW = 15
				}
				entry += "  " + m.st.dangerStyle.Render(limitStr(sc.ErrorReason, reasonW))
			}
			b.WriteString("  " + entry + "\n")
		}
	} else {
		b.WriteString("  " + label.Render("No state changes") + "\n")
	}

	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(b.String())
}

func (m Model) writeEndpointFields(b *strings.Builder, site models.Site, label lipgloss.Style, innerW int, fullscreen bool) {
	dot := m.st.subtleStyle.Render(" · ")
	var fields []string

	if site.Interval > 0 {
		fields = append(fields, label.Render("Every")+" "+fmt.Sprintf("%ds", site.Interval))
	}
	if site.Timeout > 0 {
		fields = append(fields, label.Render("Timeout")+" "+fmt.Sprintf("%ds", site.Timeout))
	}
	if site.Type == "http" && site.Method != "" && site.Method != "GET" {
		fields = append(fields, label.Render("Method")+" "+site.Method)
	}
	if site.Type == "http" {
		codes := site.AcceptedCodes
		if codes == "" {
			codes = "200-299"
		}
		fields = append(fields, label.Render("Codes")+" "+codes)
	}
	if site.Regions != "" {
		fields = append(fields, label.Render("Regions")+" "+site.Regions)
	}

	if len(fields) > 0 {
		b.WriteString("  " + strings.Join(fields, dot) + "\n")
	}

	if site.Description != "" {
		maxDescW := innerW
		if !fullscreen && maxDescW > 60 {
			maxDescW = 60
		}
		b.WriteString("  " + label.Render(limitStr(site.Description, maxDescW)) + "\n")
	}

	if site.Type == "push" && site.Token != "" {
		b.WriteString("  " + label.Render("Token") + " " + site.Token + "\n")
	}
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

func (m Model) detailFooter(width int) string {
	dot := m.st.subtleStyle.Render(" · ")
	var parts []string

	switch m.detailMode {
	case detailSLA:
		for i, p := range slaPeriods {
			if i == m.slaPeriodIdx {
				parts = append(parts, m.st.titleStyle.Render(p.key)+" "+m.st.titleStyle.Render(p.label))
			} else {
				parts = append(parts, m.hotkey(p.key, p.label))
			}
		}
		parts = append(parts, m.hotkey("Esc", "Back"))
	case detailHistory:
		parts = append(parts, m.hotkey("Esc", "Back"))
	default:
		parts = append(parts, m.hotkey("e", "Edit"), m.hotkey("h", "History"), m.hotkey("s", "SLA"), m.hotkey("Esc", "Back"))
	}

	content := "  " + strings.Join(parts, dot)
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(content)
}

func (m Model) fmtStatusWord(status string) string {
	switch status {
	case "DOWN", "SSL EXP":
		return m.st.dangerStyle.Render(status)
	case "UP":
		return m.st.specialStyle.Render("UP")
	case "LATE":
		return m.st.warnStyle.Render("LATE")
	case "STALE":
		return m.st.staleStyle.Render("STALE")
	case "PENDING":
		return m.st.subtleStyle.Render("PENDING")
	case "PAUSED":
		return m.st.warnStyle.Render("PAUSED")
	default:
		return m.st.subtleStyle.Render(status)
	}
}

func (m Model) viewSLASidebar(width, _ int) string {
	var b strings.Builder
	label := m.st.subtleStyle
	innerW := width - 4
	if innerW < 20 {
		innerW = 20
	}

	period := slaPeriods[m.slaPeriodIdx]
	b.WriteString("  " + label.Render("Period: Last "+period.label) + "\n\n")

	r := m.slaReport
	barWidth := innerW - 25
	if barWidth < 10 {
		barWidth = 10
	}
	bar := m.uptimeBar(r.UptimePct, barWidth)
	uptimeColor := m.st.specialStyle
	if r.UptimePct < uptimeExcellentPct {
		uptimeColor = m.st.warnStyle
	}
	if r.UptimePct < uptimeGoodPct {
		uptimeColor = m.st.dangerStyle
	}
	fmt.Fprintf(&b, "  %-14s %s  %s\n", label.Render("Uptime"), uptimeColor.Render(fmtPct(r.UptimePct)+"%"), bar)
	fmt.Fprintf(&b, "  %-14s %s\n", label.Render("Downtime"), fmtDuration(r.Downtime))
	fmt.Fprintf(&b, "  %-14s %d\n", label.Render("Outages"), r.OutageCount)

	if r.OutageCount > 0 {
		fmt.Fprintf(&b, "  %-14s %s\n", label.Render("Longest"), fmtDuration(r.LongestOut))
		fmt.Fprintf(&b, "  %-14s %s\n", label.Render("MTTR"), fmtDuration(r.MTTR))
		fmt.Fprintf(&b, "  %-14s %s\n", label.Render("MTBF"), fmtDuration(r.MTBF))
	}

	b.WriteString("\n")

	if len(m.slaDailyBreakdown) > 0 {
		b.WriteString("  " + m.st.titleStyle.Render("DAILY BREAKDOWN") + "\n")
		dayBarW := innerW - 20
		if dayBarW < 10 {
			dayBarW = 10
		}
		for _, day := range m.slaDailyBreakdown {
			dateStr := day.Date.Format("Jan 02")
			dayBar := m.uptimeBar(day.UptimePct, dayBarW)
			pctStr := fmtPct(day.UptimePct) + "%"
			color := m.st.specialStyle
			if day.UptimePct < uptimeExcellentPct {
				color = m.st.warnStyle
			}
			if day.UptimePct < uptimeGoodPct {
				color = m.st.dangerStyle
			}
			fmt.Fprintf(&b, "  %-8s %s %s\n", label.Render(dateStr), dayBar, color.Render(pctStr))
		}
	}

	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(b.String())
}

func (m Model) viewHistorySidebar(width, _ int) string {
	var b strings.Builder
	label := m.st.subtleStyle
	innerW := width - 4
	if innerW < 20 {
		innerW = 20
	}

	sparkline := m.stateChangeSparkline(m.historyChanges, innerW)
	if sparkline != "" {
		b.WriteString("  " + sparkline + "\n\n")
	}

	if len(m.historyChanges) == 0 {
		b.WriteString("  " + label.Render("No state changes recorded") + "\n")
	} else {
		reasonW := innerW - 45
		if reasonW < 10 {
			reasonW = 10
		}
		for i, sc := range m.historyChanges {
			ts := sc.ChangedAt.Format("01/02 15:04")

			arrow := label.Render(sc.FromStatus) + " → "
			switch sc.ToStatus {
			case string(models.StatusUp):
				arrow += m.st.specialStyle.Render(sc.ToStatus)
			case string(models.StatusLate):
				arrow += m.st.warnStyle.Render(sc.ToStatus)
			case string(models.StatusStale):
				arrow += m.st.staleStyle.Render(sc.ToStatus)
			default:
				arrow += m.st.dangerStyle.Render(sc.ToStatus)
			}

			durStr := ""
			if dur := computeOutageDuration(m.historyChanges, i); dur > 0 {
				durStr = m.st.warnStyle.Render(fmtDuration(dur))
			}

			reason := ""
			if sc.ErrorReason != "" && sc.ToStatus != string(models.StatusUp) {
				reason = m.st.dangerStyle.Render(limitStr(sc.ErrorReason, reasonW))
			}

			fmt.Fprintf(&b, "  %-12s %s  %s %s\n", ts, arrow, durStr, reason)
		}
	}

	b.WriteString("\n")

	stats := computeHistoryStats(m.historyChanges)
	statParts := []string{fmt.Sprintf("%d events", stats.totalEvents)}
	if stats.outageCount > 0 {
		statParts = append(statParts, fmt.Sprintf("%d outages", stats.outageCount))
		avg := stats.totalDowntime / time.Duration(stats.outageCount)
		statParts = append(statParts, "avg "+fmtDuration(avg))
	}
	b.WriteString("  " + label.Render(strings.Join(statParts, " │ ")) + "\n")

	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(b.String())
}
