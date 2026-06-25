package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/monitor"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) viewDetailPanel() string {
	if m.cursor >= len(m.sites) {
		return ""
	}
	site := m.sites[m.cursor]
	hist, _ := m.engine.GetHistory(site.ID)

	var b strings.Builder
	totalW := m.termWidth - chromePadH

	var breadcrumb string
	if site.ParentID > 0 {
		for _, s := range m.sites {
			if s.ID == site.ParentID {
				breadcrumb = m.st.subtleStyle.Render("  Monitors > "+s.Name+" > ") + m.st.titleStyle.Render(site.Name)
				break
			}
		}
	}
	if breadcrumb == "" {
		breadcrumb = m.st.subtleStyle.Render("  Monitors > ") + m.st.titleStyle.Render(site.Name)
	}
	b.WriteString(breadcrumb + "\n")
	b.WriteString(m.divider() + "\n")

	// Two-column layout for key info
	colW := (totalW - 4) / 2
	if colW < 30 {
		colW = 30
	}

	row := func(label, value string) string {
		return fmt.Sprintf("  %-16s %s", m.st.subtleStyle.Render(label), value)
	}

	divW := totalW - 4
	if divW < 20 {
		divW = 20
	}
	sectionDiv := m.st.subtleStyle.Render(strings.Repeat("─", divW))
	sectionHead := func(title string) string {
		return m.st.titleStyle.Render("  "+title) + " " + m.st.subtleStyle.Render(strings.Repeat("─", divW-len(title)-3))
	}

	// Left column: status + endpoint
	var left []string
	left = append(left, row("Status", m.fmtStatus(site.Status, site.Paused, m.isMonitorInMaintenance(site.ID))))

	if (site.Status == models.StatusDown || site.Status == models.StatusSSLExp || site.Status == models.StatusLate || site.Status == models.StatusStale) && site.LastError != "" {
		errW := colW - 19
		if errW < 20 {
			errW = 20
		}
		errMsg := limitStr(site.LastError, errW)
		left = append(left, row("Error", m.st.dangerStyle.Render(errMsg)))
	}

	if site.Type == "http" && site.StatusCode > 0 {
		left = append(left, row("HTTP Code", strconv.Itoa(site.StatusCode)))
	}
	if !site.StatusChangedAt.IsZero() {
		dur := time.Since(site.StatusChangedAt)
		left = append(left, row("State Since", fmtDuration(dur)+" ago"))
	}

	left = append(left, "")
	left = append(left, m.st.titleStyle.Render("  ENDPOINT"))
	left = append(left, row("Type", site.Type))
	if site.URL != "" {
		left = append(left, row("URL", limitStr(site.URL, colW-19)))
	}
	if site.Hostname != "" {
		left = append(left, row("Host", site.Hostname))
	}
	if site.Port > 0 {
		left = append(left, row("Port", strconv.Itoa(site.Port)))
	}

	// Right column: timing + config
	var right []string
	right = append(right, row("Latency", m.fmtLatency(site.Latency)))
	right = append(right, row("Uptime", m.fmtUptime(hist.Statuses)))
	right = append(right, row("Interval", fmt.Sprintf("%ds", site.Interval)))
	if !site.LastCheck.IsZero() {
		right = append(right, row("Last Check", m.fmtTimeAgo(site.LastCheck)))
	}
	if !site.LastSuccessAt.IsZero() {
		right = append(right, row("Last Success", m.fmtTimeAgo(site.LastSuccessAt)))
	}

	if site.Type == "http" {
		right = append(right, "")
		right = append(right, m.st.titleStyle.Render("  HTTP"))
		codes := site.AcceptedCodes
		if codes == "" {
			codes = "200-299"
		}
		right = append(right, row("Codes", codes))
		right = append(right, row("SSL", m.fmtSSL(site)))
		if site.Method != "" && site.Method != "GET" {
			right = append(right, row("Method", site.Method))
		}
	}

	if site.MaxRetries > 0 {
		right = append(right, row("Retries", m.fmtRetries(site)))
	}

	// Pad shorter column
	for len(left) < len(right) {
		left = append(left, "")
	}
	for len(right) < len(left) {
		right = append(right, "")
	}

	leftCol := lipgloss.NewStyle().Width(colW).Render(strings.Join(left, "\n"))
	rightCol := lipgloss.NewStyle().Width(colW).Render(strings.Join(right, "\n"))
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, leftCol, rightCol) + "\n")
	b.WriteString(sectionDiv + "\n")

	// Connection chain (full width, only on errors)
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

	// Maintenance
	if m.isMonitorInMaintenance(site.ID) {
		for _, mw := range m.maintenanceWindows {
			if mw.Type == "maintenance" && (mw.MonitorID == 0 || mw.MonitorID == site.ID || mw.MonitorID == site.ParentID) {
				fmt.Fprintf(&b, "  %-16s %s\n", m.st.subtleStyle.Render("Maintenance"), m.st.maintStyle.Render(mw.Title))
				break
			}
		}
	}

	// Push token
	if site.Type == "push" && site.Token != "" {
		fmt.Fprintf(&b, "  %-16s %s\n", m.st.subtleStyle.Render("Token"), site.Token)
	}

	// Probe results
	probeResults := m.engine.GetProbeResults(site.ID)
	if len(probeResults) > 0 {
		nodeIDs := make([]string, 0, len(probeResults))
		for id := range probeResults {
			nodeIDs = append(nodeIDs, id)
		}
		sort.Strings(nodeIDs)
		b.WriteString("\n" + sectionHead("PROBE RESULTS") + "\n")
		for _, nodeID := range nodeIDs {
			result := probeResults[nodeID]
			status := m.st.specialStyle.Render("UP")
			if !result.IsUp {
				status = m.st.dangerStyle.Render("DN")
			}
			latency := time.Duration(result.LatencyNs).Milliseconds()
			ago := time.Since(result.CheckedAt).Truncate(time.Second)
			line := fmt.Sprintf("  %-14s %s  %dms  %s ago", nodeID, status, latency, ago)
			if !result.IsUp && result.ErrorReason != "" {
				line += "  " + m.st.dangerStyle.Render(result.ErrorReason)
			}
			b.WriteString(line + "\n")
		}
	}

	// State changes
	var stateChanges []models.StateChange
	if m.detailChangesSiteID == site.ID {
		stateChanges = m.detailChanges
	}
	if len(stateChanges) > 0 {
		b.WriteString("\n" + sectionHead("STATE CHANGES") + "\n")
		for i, sc := range stateChanges {
			from := m.fmtStatusWord(string(sc.FromStatus))
			to := m.fmtStatusWord(string(sc.ToStatus))
			ago := fmtDuration(time.Since(sc.ChangedAt))
			line := fmt.Sprintf("  %s → %s  %s ago", from, to, ago)
			if sc.ToStatus == "UP" {
				dur := computeOutageDuration(stateChanges, i)
				if dur > 0 {
					line += "  " + m.st.warnStyle.Render("outage "+fmtDuration(dur))
				}
			}
			if sc.ErrorReason != "" {
				line += "  " + m.st.dangerStyle.Render(sc.ErrorReason)
			}
			b.WriteString(line + "\n")
		}
	}

	// Sparkline + stats
	b.WriteString("\n" + sectionHead("LATENCY") + "\n")
	if site.Type == "push" {
		b.WriteString("  " + m.zones.Mark("spark-heartbeat", m.heartbeatSparkline(hist.Statuses, detailSparkWidth, nil)))
		if len(hist.Statuses) > 0 {
			up := 0
			for _, s := range hist.Statuses {
				if s {
					up++
				}
			}
			fmt.Fprintf(&b, "\n  %s %d/%d checks up",
				m.st.subtleStyle.Render("Heartbeats"),
				up, len(hist.Statuses))
		}
	} else {
		b.WriteString("  " + m.zones.Mark("spark-latency", m.latencySparkline(hist.Latencies, hist.Statuses, detailSparkWidth, nil)))
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
			fmt.Fprintf(&b, "\n  %s %dms  %s %dms  %s %dms",
				m.st.subtleStyle.Render("Min"), minL.Milliseconds(),
				m.st.subtleStyle.Render("Avg"), avg.Milliseconds(),
				m.st.subtleStyle.Render("Max"), maxL.Milliseconds())
		}
	}

	if m.sparkTooltipIdx >= 0 {
		b.WriteString("\n" + m.renderSparkTooltip(site, hist, detailSparkWidth))
	}

	// Histogram
	if site.Type != "push" && len(hist.Latencies) > 5 {
		histW := totalW - 4
		if histW < 30 {
			histW = 30
		}
		b.WriteString("\n" + sectionHead("RESPONSE TIME DISTRIBUTION") + "\n")
		b.WriteString(m.latencyHistogram(hist.Latencies, hist.Statuses, histW))
	}

	b.WriteString("\n")
	b.WriteString(m.divider() + "\n")
	b.WriteString(m.st.subtleStyle.Render("  [q/Esc] Back  [e] Edit  [h] History  [s] SLA  [click] Inspect"))

	// Wrap in a viewport for scrolling
	content := b.String()
	contentH := m.termHeight - 4
	if contentH < 10 {
		contentH = 10
	}
	lines := strings.Split(content, "\n")
	if len(lines) > contentH {
		m.detailViewport.SetContent(content)
		m.detailViewport.Width = totalW
		m.detailViewport.Height = contentH
		return lipgloss.NewStyle().Padding(1, 2).Render(m.detailViewport.View())
	}

	return lipgloss.NewStyle().Padding(1, 2).Render(content)
}

func (m Model) renderSparkTooltip(site models.Site, hist monitor.SiteHistory, sparkWidth int) string {
	idx := m.sparkTooltipIdx

	var dataLen int
	if site.Type == "push" {
		dataLen = len(hist.Statuses)
	} else {
		dataLen = len(hist.Latencies)
	}
	if idx < 0 || idx >= dataLen {
		return ""
	}

	var parts []string

	checksAgo := dataLen - 1 - idx
	approxSecs := checksAgo * site.Interval
	if approxSecs == 0 {
		parts = append(parts, "latest")
	} else {
		parts = append(parts, "~"+fmtDuration(time.Duration(approxSecs)*time.Second)+" ago")
	}

	if site.Type != "push" && idx < len(hist.Latencies) {
		parts = append(parts, m.fmtLatency(hist.Latencies[idx]))
	}

	if idx < len(hist.Statuses) {
		if hist.Statuses[idx] {
			parts = append(parts, m.st.specialStyle.Render("UP"))
		} else {
			parts = append(parts, m.st.dangerStyle.Render("DOWN"))
		}
	}

	sep := m.st.subtleStyle.Render(" | ")
	pos := m.st.subtleStyle.Render(fmt.Sprintf("[%d/%d]", idx+1, dataLen))
	return "  " + strings.Join(parts, sep) + "  " + pos
}
