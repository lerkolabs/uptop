package tui

import (
	"fmt"
	"strings"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) dividerWidth() int {
	w := m.termWidth - chromePadH - 4
	if w < 40 {
		w = 40
	}
	return w
}

func (m Model) divider() string {
	return "  " + subtleStyle.Render(strings.Repeat("─", m.dividerWidth()))
}

func (m Model) emptyState(message, hint string) string {
	content := message
	if hint != "" {
		content += "\n\n" + subtleStyle.Render(hint)
	}
	return "\n" + lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Accent).
		Padding(1, 3).
		Render(content)
}

func limitStr(text string, max int) string {
	runes := []rune(text)
	if len(runes) > max {
		return string(runes[:max-3]) + "..."
	}
	return text
}

func siteOrder(s models.Site) int {
	if s.Paused {
		return 3
	}
	switch s.Status {
	case "DOWN", "SSL EXP":
		return 0
	case "STALE":
		return 1
	case "LATE":
		return 1
	case "PENDING":
		return 3
	default:
		return 2
	}
}

func typeIcon(siteType string, collapsed bool) string {
	switch siteType {
	case "http":
		return "→"
	case "push":
		return "↓"
	case "ping":
		return "↔"
	case "port":
		return "⊡"
	case "dns":
		return "◆"
	case "group":
		if collapsed {
			return "▶"
		}
		return "▼"
	default:
		return "·"
	}
}

func fmtLatency(d time.Duration) string {
	ms := d.Milliseconds()
	if ms == 0 {
		return subtleStyle.Render("—")
	}
	var s string
	if ms < 1000 {
		s = fmt.Sprintf("%dms", ms)
	} else {
		s = fmt.Sprintf("%.1fs", float64(ms)/1000)
	}
	if ms < 200 {
		return specialStyle.Render(s)
	}
	if ms < 500 {
		return warnStyle.Render(s)
	}
	return dangerStyle.Render(s)
}

func fmtUptime(statuses []bool) string {
	if len(statuses) == 0 {
		return subtleStyle.Render("—")
	}
	up := 0
	for _, s := range statuses {
		if s {
			up++
		}
	}
	pct := float64(up) / float64(len(statuses)) * 100
	s := fmt.Sprintf("%.1f%%", pct)
	if pct >= 99 {
		return specialStyle.Render(s)
	}
	if pct >= 95 {
		return warnStyle.Render(s)
	}
	return dangerStyle.Render(s)
}

func fmtSSL(site models.Site) string {
	if site.Type != "http" || !site.CheckSSL || !site.HasSSL {
		return subtleStyle.Render("-")
	}
	days := int(time.Until(site.CertExpiry).Hours() / 24)
	s := fmt.Sprintf("%dd", days)
	if days <= 0 {
		return dangerStyle.Render("EXPIRED")
	}
	if days <= site.ExpiryThreshold {
		return warnStyle.Render(s)
	}
	return specialStyle.Render(s)
}

func fmtRetries(site models.Site) string {
	dispCount := site.FailureCount
	if dispCount > site.MaxRetries {
		dispCount = site.MaxRetries
	}
	s := fmt.Sprintf("%d/%d", dispCount, site.MaxRetries)
	if site.Status == "DOWN" {
		return dangerStyle.Render(s)
	}
	if site.Status == "UP" && site.FailureCount > 0 {
		return warnStyle.Render(s)
	}
	return s
}

func fmtStatus(status string, paused bool, inMaint bool) string {
	if paused {
		return warnStyle.Render("◇ PAUSED")
	}
	if inMaint {
		return maintStyle.Render("◼ MAINT")
	}
	switch status {
	case "DOWN":
		return dangerStyle.Render("▼ DOWN")
	case "SSL EXP":
		return dangerStyle.Render("▼ SSL EXP")
	case "LATE":
		return warnStyle.Render("◆ LATE")
	case "STALE":
		return staleStyle.Render("◆ STALE")
	case "PENDING":
		return subtleStyle.Render("○ PENDING")
	default:
		return specialStyle.Render("▲ " + status)
	}
}

func fmtTimeAgo(t time.Time) string {
	if t.IsZero() {
		return subtleStyle.Render("never")
	}
	d := time.Since(t)
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours())/24)
}

func fmtDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m > 0 {
			return fmt.Sprintf("%dh %dm", h, m)
		}
		return fmt.Sprintf("%dh", h)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	if hours > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	return fmt.Sprintf("%dd", days)
}
