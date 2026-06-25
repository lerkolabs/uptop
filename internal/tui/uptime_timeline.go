package tui

import (
	"fmt"
	"strings"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/monitor"
)

func (m Model) uptimeTimeline(days []monitor.DayReport, width int) string {
	if len(days) == 0 {
		return m.st.subtleStyle.Render("No uptime data")
	}

	maxDays := width - 10
	if maxDays < 10 {
		maxDays = 10
	}

	display := days
	if len(display) > maxDays {
		display = display[len(display)-maxDays:]
	}

	var sb strings.Builder
	for _, d := range display {
		ch := "█"
		switch {
		case d.UptimePct >= 99.0:
			sb.WriteString(m.st.specialStyle.Render(ch))
		case d.UptimePct >= 95.0:
			sb.WriteString(m.st.warnStyle.Render(ch))
		case d.UptimePct > 0:
			sb.WriteString(m.st.dangerStyle.Render(ch))
		default:
			sb.WriteString(m.st.subtleStyle.Render("░"))
		}
	}

	pct := days[len(days)-1].UptimePct
	pctStyle := m.st.specialStyle
	if pct < 99.0 {
		pctStyle = m.st.dangerStyle
	} else if pct < 99.9 {
		pctStyle = m.st.warnStyle
	}
	sb.WriteString("  " + pctStyle.Render(fmt.Sprintf("%.2f%%", pct)))

	return sb.String()
}
