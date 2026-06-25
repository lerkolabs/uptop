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

	row := func(label, value string) {
		fmt.Fprintf(&b, "  %-16s %s\n", m.st.subtleStyle.Render(label), value)
	}

	section := func(label string) {
		b.WriteString("\n" + m.st.subtleStyle.Render("  "+label) + "\n")
	}

	row("Status", m.fmtStatus(site.Status, site.Paused, m.isMonitorInMaintenance(site.ID)))

	if (site.Status == models.StatusDown || site.Status == models.StatusSSLExp || site.Status == models.StatusLate || site.Status == models.StatusStale) && site.LastError != "" {
		errWidth := m.termWidth - chromePadH - 19
		if errWidth < 30 {
			errWidth = 30
		}
		wrapped := lipgloss.NewStyle().Width(errWidth).Render(site.LastError)
		row("Error", m.st.dangerStyle.Render(wrapped))
	}

	if site.Type == "http" && site.StatusCode > 0 {
		row("HTTP Code", strconv.Itoa(site.StatusCode))
	}

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

	if !site.StatusChangedAt.IsZero() {
		dur := time.Since(site.StatusChangedAt)
		row("State Since", site.StatusChangedAt.Format("2006-01-02 15:04:05")+" ("+fmtDuration(dur)+")")
	}

	if !site.LastSuccessAt.IsZero() {
		ago := time.Since(site.LastSuccessAt)
		row("Last Success", site.LastSuccessAt.Format("15:04:05")+" ("+fmtDuration(ago)+" ago)")
	}

	if m.isMonitorInMaintenance(site.ID) {
		for _, mw := range m.maintenanceWindows {
			if mw.Type == "maintenance" && (mw.MonitorID == 0 || mw.MonitorID == site.ID || mw.MonitorID == site.ParentID) {
				row("Maintenance", m.st.maintStyle.Render(mw.Title))
				break
			}
		}
	}

	section("ENDPOINT")
	row("Type", site.Type)
	if site.Type == "push" && site.Token != "" {
		row("Token", site.Token)
		row("Push", "curl -X POST -H 'Authorization: Bearer "+site.Token+"' <host>/api/push")
	}
	if site.URL != "" {
		row("URL", site.URL)
	}
	if site.Hostname != "" {
		row("Host", site.Hostname)
	}
	if site.Port > 0 {
		row("Port", strconv.Itoa(site.Port))
	}

	section("TIMING")
	row("Interval", fmt.Sprintf("%ds", site.Interval))
	if site.Timeout > 0 {
		row("Timeout", fmt.Sprintf("%ds", site.Timeout))
	}
	row("Latency", m.fmtLatency(site.Latency))
	row("Uptime", m.fmtUptime(hist.Statuses))
	if !site.LastCheck.IsZero() {
		row("Last Check", m.fmtTimeAgo(site.LastCheck))
	}

	if site.Type == "http" {
		section("HTTP")
		if site.Method != "" && site.Method != "GET" {
			row("Method", site.Method)
		}
		codes := site.AcceptedCodes
		if codes == "" {
			codes = "200-299"
		}
		row("Codes", codes)
		row("SSL", m.fmtSSL(site))
		if site.IgnoreTLS {
			row("TLS Verify", m.st.dangerStyle.Render("disabled"))
		}
	}

	if site.MaxRetries > 0 || site.Regions != "" || site.Description != "" {
		section("CONFIG")
		if site.MaxRetries > 0 {
			row("Retries", m.fmtRetries(site))
		}
		if site.Regions != "" {
			row("Regions", site.Regions)
		}
		if site.Description != "" {
			row("Description", site.Description)
		}
	}

	probeResults := m.engine.GetProbeResults(site.ID)
	if len(probeResults) > 0 {
		nodeIDs := make([]string, 0, len(probeResults))
		for id := range probeResults {
			nodeIDs = append(nodeIDs, id)
		}
		sort.Strings(nodeIDs)
		b.WriteString("\n" + m.st.subtleStyle.Render("  PROBE RESULTS") + "\n")
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

	// Loaded on panel-enter (loadDetailCmd) and cached, so View does no DB IO.
	var stateChanges []models.StateChange
	if m.detailChangesSiteID == site.ID {
		stateChanges = m.detailChanges
	}
	if len(stateChanges) > 0 {
		b.WriteString("\n" + m.st.subtleStyle.Render("  STATE CHANGES") + "\n")
		for i, sc := range stateChanges {
			ago := fmtDuration(time.Since(sc.ChangedAt))
			arrow := m.st.subtleStyle.Render(sc.FromStatus) + " → "
			if sc.ToStatus == string(models.StatusUp) {
				arrow += m.st.specialStyle.Render(sc.ToStatus)
			} else {
				arrow += m.st.dangerStyle.Render(sc.ToStatus)
			}
			line := fmt.Sprintf("  %s  %s", arrow, m.st.subtleStyle.Render(ago+" ago"))
			if dur := computeOutageDuration(stateChanges, i); dur > 0 {
				line += "  " + m.st.warnStyle.Render("outage "+fmtDuration(dur))
			}
			if sc.ErrorReason != "" && sc.ToStatus != string(models.StatusUp) {
				line += "  " + m.st.dangerStyle.Render(sc.ErrorReason)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("  " + m.st.subtleStyle.Render("[h] History") + "\n")
	}

	b.WriteString(m.divider() + "\n")
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

	if site.Type != "push" && len(hist.Latencies) > 5 {
		histW := m.termWidth - chromePadH - 4
		if histW < 30 {
			histW = 30
		}
		b.WriteString("\n\n" + m.st.subtleStyle.Render("  RESPONSE TIME DISTRIBUTION") + "\n")
		b.WriteString(m.latencyHistogram(hist.Latencies, hist.Statuses, histW))
	}

	b.WriteString("\n")
	b.WriteString(m.divider() + "\n")
	b.WriteString(m.st.subtleStyle.Render("  [q/Esc] Back  [e] Edit  [h] History  [s] SLA  [click] Inspect"))

	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
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
