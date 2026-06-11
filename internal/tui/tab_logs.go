package tui

import (
	"fmt"
	"strings"
)

type logSeverity int

const (
	severityInfo logSeverity = iota
	severityWarn
	severityDown
	severityUp
	severitySystem
)

func classifyLog(line string) logSeverity {
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "confirmed down"),
		strings.Contains(lower, "is down"),
		strings.Contains(lower, "missed heartbeat"),
		strings.Contains(lower, "alert send failed"):
		return severityDown
	case strings.Contains(lower, "recovered"),
		strings.Contains(lower, "is up"),
		strings.Contains(lower, "recovery"),
		strings.Contains(lower, "first heartbeat"):
		return severityUp
	case strings.Contains(lower, "failed check"),
		strings.Contains(lower, "ssl warning"),
		strings.Contains(lower, "overdue"),
		strings.Contains(lower, "was late"):
		return severityWarn
	case strings.Contains(lower, "engine"),
		strings.Contains(lower, "cluster"),
		strings.Contains(lower, "loaded"),
		strings.Contains(lower, "paused"),
		strings.Contains(lower, "resumed"):
		return severitySystem
	default:
		return severityInfo
	}
}

func isImportantLog(sev logSeverity) bool {
	return sev == severityDown || sev == severityUp || sev == severitySystem
}

func (m Model) renderLogTag(sev logSeverity) string {
	switch sev {
	case severityDown:
		return m.st.dangerStyle.Render(" DOWN ")
	case severityUp:
		return m.st.specialStyle.Render("  UP  ")
	case severityWarn:
		return m.st.warnStyle.Render(" WARN ")
	case severitySystem:
		return m.st.titleStyle.Render(" SYS  ")
	default:
		return m.st.subtleStyle.Render(" info ")
	}
}

func (m Model) renderLogLine(line string) string {
	sev := classifyLog(line)
	tag := m.renderLogTag(sev)

	ts := ""
	msg := line
	if len(line) > 10 && line[0] == '[' {
		if idx := strings.Index(line, "]"); idx > 0 && idx < 12 {
			ts = m.st.subtleStyle.Render(line[1:idx])
			msg = strings.TrimSpace(line[idx+1:])
		}
	}

	if ts != "" {
		return fmt.Sprintf("  %s  %s  %s", ts, tag, msg)
	}
	return fmt.Sprintf("  %s  %s", tag, msg)
}

func (m Model) viewLogsTab() string {
	content := m.logViewport.View()
	if strings.TrimSpace(content) == "" || content == "Waiting for logs..." {
		return m.emptyState("No log entries yet.", "Logs appear as monitors run checks")
	}

	lines := strings.Split(content, "\n")
	var rendered []string
	total := 0
	shown := 0

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		total++
		sev := classifyLog(line)
		if m.logFilterImportant && !isImportantLog(sev) {
			continue
		}
		shown++
		rendered = append(rendered, m.renderLogLine(line))
	}

	filterLabel := "All"
	if m.logFilterImportant {
		filterLabel = "Important"
	}

	header := m.st.subtleStyle.Render(fmt.Sprintf(
		"  %d entries  Filter: %s", shown, filterLabel))

	if m.logFilterImportant && shown < total {
		header += m.st.subtleStyle.Render(fmt.Sprintf("  (%d hidden)", total-shown))
	}

	m.logViewport.SetContent(strings.Join(rendered, "\n"))
	return "\n" + header + "\n\n" + m.logViewport.View()
}
