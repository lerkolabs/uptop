package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

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
				breadcrumb = subtleStyle.Render("  Sites > "+s.Name+" > ") + titleStyle.Render(site.Name)
				break
			}
		}
	}
	if breadcrumb == "" {
		breadcrumb = subtleStyle.Render("  Sites > ") + titleStyle.Render(site.Name)
	}
	b.WriteString(breadcrumb + "\n\n")

	row := func(label, value string) {
		fmt.Fprintf(&b, "  %-16s %s\n", subtleStyle.Render(label), value)
	}

	section := func(label string) {
		b.WriteString("\n" + subtleStyle.Render("  "+label) + "\n")
	}

	errCat := classifyError(site.LastError, site.Type, site.StatusCode)
	row("Status", fmtStatus(site.Status, site.Paused, m.isMonitorInMaintenance(site.ID), errCat))

	if (site.Status == "DOWN" || site.Status == "SSL EXP" || site.Status == "LATE" || site.Status == "STALE") && site.LastError != "" {
		errWidth := m.termWidth - chromePadH - 19
		if errWidth < 30 {
			errWidth = 30
		}
		wrapped := lipgloss.NewStyle().Width(errWidth).Render(site.LastError)
		row("Error", dangerStyle.Render(wrapped))
	}

	if site.Type == "http" && site.StatusCode > 0 {
		row("HTTP Code", strconv.Itoa(site.StatusCode))
	}

	if (site.Status == "DOWN" || site.Status == "SSL EXP") && site.LastError != "" {
		chain := connectionChain(site.LastError, site.Type, site.StatusCode, strings.HasPrefix(site.URL, "https"))
		if len(chain) > 0 {
			b.WriteString("\n")
			for _, step := range chain {
				var icon string
				switch step.Status {
				case stepPassed:
					icon = specialStyle.Render("✓")
				case stepFailed:
					icon = dangerStyle.Render("✗")
				case stepSkipped:
					icon = subtleStyle.Render("·")
				}
				line := fmt.Sprintf("  %s %-16s", icon, step.Name)
				if step.Detail != "" {
					switch step.Status {
					case stepFailed:
						line += " " + dangerStyle.Render(step.Detail)
					case stepSkipped:
						line += " " + subtleStyle.Render(step.Detail)
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
				row("Maintenance", maintStyle.Render(mw.Title))
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
	row("Latency", fmtLatency(site.Latency))
	row("Uptime", fmtUptime(hist.Statuses))
	if !site.LastCheck.IsZero() {
		row("Last Check", site.LastCheck.Format("15:04:05"))
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
		row("SSL", fmtSSL(site))
		if site.IgnoreTLS {
			row("TLS Verify", dangerStyle.Render("disabled"))
		}
	}

	if site.MaxRetries > 0 || site.Regions != "" || site.Description != "" {
		section("CONFIG")
		if site.MaxRetries > 0 {
			row("Retries", fmtRetries(site))
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
		b.WriteString("\n" + subtleStyle.Render("  PROBE RESULTS") + "\n")
		for nodeID, result := range probeResults {
			status := specialStyle.Render("UP")
			if !result.IsUp {
				status = dangerStyle.Render("DN")
			}
			latency := time.Duration(result.LatencyNs).Milliseconds()
			ago := time.Since(result.CheckedAt).Truncate(time.Second)
			line := fmt.Sprintf("  %-14s %s  %dms  %s ago", nodeID, status, latency, ago)
			if !result.IsUp && result.ErrorReason != "" {
				line += "  " + dangerStyle.Render(result.ErrorReason)
			}
			b.WriteString(line + "\n")
		}
	}

	stateChanges := m.engine.GetStateChanges(site.ID, 5)
	if len(stateChanges) > 0 {
		b.WriteString("\n" + subtleStyle.Render("  STATE CHANGES") + "\n")
		for i, sc := range stateChanges {
			ago := fmtDuration(time.Since(sc.ChangedAt))
			arrow := subtleStyle.Render(sc.FromStatus) + " → "
			if sc.ToStatus == "UP" {
				arrow += specialStyle.Render(sc.ToStatus)
			} else {
				arrow += dangerStyle.Render(sc.ToStatus)
			}
			line := fmt.Sprintf("  %s  %s", arrow, subtleStyle.Render(ago+" ago"))
			if dur := computeOutageDuration(stateChanges, i); dur > 0 {
				line += "  " + warnStyle.Render("outage "+fmtDuration(dur))
			}
			if sc.ErrorReason != "" && sc.ToStatus != "UP" {
				line += "  " + dangerStyle.Render(sc.ErrorReason)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("  " + subtleStyle.Render("[h] History") + "\n")
	}

	b.WriteString("\n")
	const sparkWidth = 40
	if site.Type == "push" {
		b.WriteString("  " + heartbeatSparkline(hist.Statuses, sparkWidth))
		if len(hist.Statuses) > 0 {
			up := 0
			for _, s := range hist.Statuses {
				if s {
					up++
				}
			}
			fmt.Fprintf(&b, "\n  %s %d/%d checks up",
				subtleStyle.Render("Heartbeats"),
				up, len(hist.Statuses))
		}
	} else {
		b.WriteString("  " + latencySparkline(hist.Latencies, hist.Statuses, sparkWidth))
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
				subtleStyle.Render("Min"), minL.Milliseconds(),
				subtleStyle.Render("Avg"), avg.Milliseconds(),
				subtleStyle.Render("Max"), maxL.Milliseconds())
		}
	}

	b.WriteString("\n\n")
	b.WriteString(subtleStyle.Render("  [i/Esc] Back  [e] Edit  [h] History  [q] Quit"))

	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}
