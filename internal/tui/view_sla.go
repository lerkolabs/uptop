package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var slaPeriods = []struct {
	label    string
	key      string
	duration time.Duration
	days     int
}{
	{"24h", "1", 24 * time.Hour, 1},
	{"7d", "2", 7 * 24 * time.Hour, 7},
	{"30d", "3", 30 * 24 * time.Hour, 30},
	{"90d", "4", 90 * 24 * time.Hour, 90},
}

func (m Model) viewSLAPanel() string {
	var b strings.Builder

	header := "  " + m.st.titleStyle.Render("SLA REPORT: "+m.slaSiteName)
	header += "  " + m.st.subtleStyle.Render("[q] Back")
	b.WriteString(header + "\n")
	b.WriteString(m.divider() + "\n")

	period := slaPeriods[m.slaPeriodIdx]
	b.WriteString("  " + m.st.subtleStyle.Render("Period: Last "+period.label) + "\n\n")

	r := m.slaReport

	barWidth := m.dividerWidth() - 30
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
	fmt.Fprintf(&b, "  %-16s %s  %s\n", m.st.subtleStyle.Render("Uptime"), uptimeColor.Render(fmt.Sprintf("%s%%", fmtPct(r.UptimePct))), bar)
	fmt.Fprintf(&b, "  %-16s %s\n", m.st.subtleStyle.Render("Downtime"), fmtDuration(r.Downtime))
	fmt.Fprintf(&b, "  %-16s %d\n", m.st.subtleStyle.Render("Outages"), r.OutageCount)

	if r.OutageCount > 0 {
		fmt.Fprintf(&b, "  %-16s %s\n", m.st.subtleStyle.Render("Longest"), fmtDuration(r.LongestOut))
		fmt.Fprintf(&b, "  %-16s %s\n", m.st.subtleStyle.Render("MTTR"), fmtDuration(r.MTTR))
		fmt.Fprintf(&b, "  %-16s %s\n", m.st.subtleStyle.Render("MTBF"), fmtDuration(r.MTBF))
	}

	b.WriteString("\n" + m.divider() + "\n")

	if len(m.slaDailyBreakdown) > 0 {
		b.WriteString(m.slaViewport.View())
	}

	b.WriteString("\n" + m.divider() + "\n")

	var keys []string
	for i, p := range slaPeriods {
		label := fmt.Sprintf("[%s] %s", p.key, p.label)
		if i == m.slaPeriodIdx {
			keys = append(keys, m.st.titleStyle.Render(label))
		} else {
			keys = append(keys, m.st.subtleStyle.Render(label))
		}
	}
	b.WriteString("  " + strings.Join(keys, "  "))
	b.WriteString("  " + m.st.subtleStyle.Render("[j/k/↑/↓] Scroll  [q/Esc] Back"))

	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}

func (m Model) buildSLADailyContent() string {
	var b strings.Builder

	barWidth := m.dividerWidth() - 30
	if barWidth < 10 {
		barWidth = 10
	}

	b.WriteString("  " + m.st.subtleStyle.Render("DAILY BREAKDOWN") + "\n")
	for _, day := range m.slaDailyBreakdown {
		dateStr := day.Date.Format("Jan 02")
		bar := m.uptimeBar(day.UptimePct, barWidth)
		pctStr := fmtPct(day.UptimePct) + "%"

		color := m.st.specialStyle
		if day.UptimePct < uptimeExcellentPct {
			color = m.st.warnStyle
		}
		if day.UptimePct < uptimeGoodPct {
			color = m.st.dangerStyle
		}

		fmt.Fprintf(&b, "  %-8s %s  %s\n", m.st.subtleStyle.Render(dateStr), bar, color.Render(pctStr))
	}

	return b.String()
}

func (m Model) uptimeBar(pct float64, width int) string {
	filled := int(math.Round(pct / 100 * float64(width)))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	empty := width - filled

	bar := m.st.specialStyle.Render(strings.Repeat("█", filled))
	if empty > 0 {
		bar += m.st.subtleStyle.Render(strings.Repeat("░", empty))
	}
	return bar
}

func fmtPct(pct float64) string {
	if pct == 100 {
		return "100.00"
	}
	if pct >= uptimePrecisionPct {
		return fmt.Sprintf("%.3f", pct)
	}
	return fmt.Sprintf("%.2f", pct)
}
