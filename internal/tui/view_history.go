package tui

import (
	"fmt"
	"strings"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"github.com/charmbracelet/lipgloss"
)

type historyStats struct {
	totalEvents   int
	outageCount   int
	totalDowntime time.Duration
}

func computeOutageDuration(changes []models.StateChange, idx int) time.Duration {
	sc := changes[idx]
	if sc.ToStatus != "UP" {
		return 0
	}
	if idx+1 >= len(changes) {
		return 0
	}
	prev := changes[idx+1]
	if prev.ToStatus == "UP" {
		return 0
	}
	dur := sc.ChangedAt.Sub(prev.ChangedAt)
	if dur < 0 {
		return 0
	}
	return dur
}

func computeHistoryStats(changes []models.StateChange) historyStats {
	var s historyStats
	s.totalEvents = len(changes)
	for i := range changes {
		dur := computeOutageDuration(changes, i)
		if dur > 0 {
			s.outageCount++
			s.totalDowntime += dur
		}
	}
	return s
}

func stateChangeSparkline(changes []models.StateChange, width int) string {
	if len(changes) < 2 || width < 4 {
		return ""
	}

	oldest := changes[len(changes)-1].ChangedAt
	newest := changes[0].ChangedAt
	span := newest.Sub(oldest)
	if span <= 0 {
		return ""
	}

	buckets := make([]int, width)
	for _, sc := range changes {
		pos := int(float64(sc.ChangedAt.Sub(oldest)) / float64(span) * float64(width-1))
		if pos >= width {
			pos = width - 1
		}
		if pos < 0 {
			pos = 0
		}
		buckets[pos]++
	}

	maxVal := 0
	for _, v := range buckets {
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal == 0 {
		return ""
	}

	var sb strings.Builder
	for _, v := range buckets {
		if v == 0 {
			sb.WriteRune('·')
			continue
		}
		idx := int(float64(v) / float64(maxVal) * 7)
		if idx > 7 {
			idx = 7
		}
		ch := string(sparkChars[idx])
		switch {
		case v >= 3:
			sb.WriteString(dangerStyle.Render(ch))
		case v >= 2:
			sb.WriteString(warnStyle.Render(ch))
		default:
			sb.WriteString(subtleStyle.Render(ch))
		}
	}
	return sb.String()
}

func (m Model) buildHistoryContent() string {
	var b strings.Builder

	reasonWidth := m.termWidth - chromePadH - 55
	if reasonWidth < 10 {
		reasonWidth = 10
	}
	if reasonWidth > 60 {
		reasonWidth = 60
	}

	for i, sc := range m.historyChanges {
		ts := sc.ChangedAt.Format("2006-01-02 15:04")

		arrow := subtleStyle.Render(sc.FromStatus) + " → "
		switch sc.ToStatus {
		case "UP":
			arrow += specialStyle.Render(sc.ToStatus)
		case "LATE":
			arrow += warnStyle.Render(sc.ToStatus)
		case "STALE":
			arrow += staleStyle.Render(sc.ToStatus)
		default:
			arrow += dangerStyle.Render(sc.ToStatus)
		}

		durStr := ""
		if dur := computeOutageDuration(m.historyChanges, i); dur > 0 {
			durStr = warnStyle.Render("outage " + fmtDuration(dur))
		}

		reason := ""
		if sc.ErrorReason != "" && sc.ToStatus != "UP" {
			reason = dangerStyle.Render(limitStr(sc.ErrorReason, reasonWidth))
		}

		fmt.Fprintf(&b, "  %-18s %s  %-12s %s\n", ts, arrow, durStr, reason)
	}

	return b.String()
}

func (m Model) viewHistoryPanel() string {
	var b strings.Builder

	header := "  " + titleStyle.Render("STATE HISTORY: "+m.historySiteName)
	header += "  " + subtleStyle.Render("[q] Back")
	b.WriteString(header + "\n")

	divWidth := m.dividerWidth()
	b.WriteString(m.divider() + "\n")

	sparkline := stateChangeSparkline(m.historyChanges, divWidth)
	if sparkline != "" {
		b.WriteString("  " + sparkline + "\n")
		b.WriteString(m.divider() + "\n")
	}

	fmt.Fprintf(&b, "  %-18s %-17s %-12s %s\n",
		subtleStyle.Render("TIME"),
		subtleStyle.Render("TRANSITION"),
		subtleStyle.Render("DURATION"),
		subtleStyle.Render("REASON"))

	if len(m.historyChanges) == 0 {
		b.WriteString("\n  " + subtleStyle.Render("No state changes recorded") + "\n")
	} else {
		b.WriteString(m.historyViewport.View())
	}

	b.WriteString("\n" + m.divider() + "\n")

	stats := computeHistoryStats(m.historyChanges)
	parts := []string{fmt.Sprintf("%d events", stats.totalEvents)}
	if stats.outageCount > 0 {
		parts = append(parts, fmt.Sprintf("%d outages", stats.outageCount))
		avg := stats.totalDowntime / time.Duration(stats.outageCount)
		parts = append(parts, "avg outage "+fmtDuration(avg))
	}
	b.WriteString("  " + subtleStyle.Render(strings.Join(parts, " │ ")) + "\n")
	b.WriteString("  " + subtleStyle.Render("[j/k/↑/↓] Scroll  [q/Esc] Back"))

	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}
