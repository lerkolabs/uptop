package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderCompactLogLine(line string, maxW int) string {
	sev := classifyLog(line)

	var tag string
	switch sev {
	case severityDown:
		tag = m.st.dangerStyle.Render("▼")
	case severityUp:
		tag = m.st.specialStyle.Render("▲")
	case severityWarn:
		tag = m.st.warnStyle.Render("◆")
	case severitySystem:
		tag = m.st.titleStyle.Render("●")
	default:
		tag = m.st.subtleStyle.Render("·")
	}

	ts := ""
	msg := line
	if len(line) > 10 && line[0] == '[' {
		if idx := strings.Index(line, "]"); idx > 0 && idx < 12 {
			ts = line[1:idx]
			msg = strings.TrimSpace(line[idx+1:])
		}
	}

	msg = strings.TrimPrefix(msg, "Monitor ")
	msg = strings.TrimPrefix(msg, "Push ")

	// prefix: " HH:MM ● " = 9 visible chars, or " ● " = 3 without timestamp
	prefixW := 3
	if ts != "" {
		prefixW = len(ts) + 4
	}
	msgW := maxW - prefixW
	if msgW < 5 {
		msgW = 5
	}
	msg = limitStr(msg, msgW)

	if ts != "" {
		return " " + m.st.subtleStyle.Render(ts) + " " + tag + " " + msg
	}
	return " " + tag + " " + msg
}

func (m Model) viewLogsSidebar(width int) string {
	logs := m.engine.GetLogs()
	if len(logs) == 0 {
		return m.st.subtleStyle.Render("  No logs yet")
	}

	sidebarStyle := lipgloss.NewStyle().Width(width).MaxWidth(width)

	var lines []string
	for _, line := range logs {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if m.logFilterImportant && !isImportantLog(classifyLog(line)) {
			continue
		}
		lines = append(lines, m.renderCompactLogLine(line, width))
	}

	return sidebarStyle.Render(strings.Join(lines, "\n"))
}
