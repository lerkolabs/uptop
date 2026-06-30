package tui

import (
	"fmt"
	"strings"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/monitor"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) viewDetailInline(width int) string {
	if m.cursor >= len(m.sites) {
		return ""
	}
	site := m.sites[m.cursor]
	hist, _ := m.engine.GetHistory(site.ID)
	return m.viewDetailSidebar(site, hist, width)
}

func (m Model) viewDetailSidebar(site models.Site, hist monitor.SiteHistory, width int) string {
	dot := m.st.subtleStyle.Render(" · ")
	label := m.st.subtleStyle
	var b strings.Builder
	innerW := width - 4
	if innerW < 20 {
		innerW = 20
	}

	// Status + latency + last check
	status := m.fmtStatus(site.Status, site.Paused, m.isMonitorInMaintenance(site.ID))
	statusParts := []string{status}
	if site.Latency > 0 {
		statusParts = append(statusParts, m.fmtLatency(site.Latency))
	}
	if !site.LastCheck.IsZero() {
		statusParts = append(statusParts, m.fmtTimeAgo(site.LastCheck))
	}
	b.WriteString("  " + strings.Join(statusParts, dot) + "\n")

	// Type-specific details
	typeParts := m.detailTypeLine(site)
	if len(typeParts) > 0 {
		b.WriteString("  " + strings.Join(typeParts, dot) + "\n")
	}

	// Uptime + retries
	uptimeParts := []string{label.Render("Uptime") + " " + m.fmtUptime(hist.Statuses)}
	if site.Type != "group" && site.MaxRetries > 0 {
		uptimeParts = append(uptimeParts, label.Render("Retries")+" "+m.fmtRetries(site))
	}
	b.WriteString("  " + strings.Join(uptimeParts, dot) + "\n")

	// Error line
	if (site.Status == models.StatusDown || site.Status == models.StatusSSLExp ||
		site.Status == models.StatusLate || site.Status == models.StatusStale) && site.LastError != "" {
		errW := innerW
		if errW < 20 {
			errW = 20
		}
		b.WriteString("  " + label.Render("Error") + " " + m.st.dangerStyle.Render(limitStr(site.LastError, errW)) + "\n")
	}

	b.WriteString("\n")

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

	b.WriteString("\n  " + m.detailKeys() + "\n")

	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(b.String())
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

func (m Model) detailKeys() string {
	return m.st.subtleStyle.Render("[e] Edit  [h] History  [s] SLA  [q/Esc] Back")
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
