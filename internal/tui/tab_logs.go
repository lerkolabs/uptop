package tui

import (
	"fmt"
	"strings"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"github.com/charmbracelet/lipgloss"
)

type logSeverity int

const (
	severityInfo logSeverity = iota
	severityWarn
	severityDown
	severityUp
	severitySystem
)

func classifyLog(msg string) logSeverity {
	lower := strings.ToLower(msg)
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

func (m Model) renderLogLine(entry models.LogEntry) string {
	sev := classifyLog(entry.Message)
	tag := m.renderLogTag(sev)
	ts := m.st.subtleStyle.Render(entry.CreatedAt.Local().Format("01/02 15:04"))
	return fmt.Sprintf("  %s  %s  %s", ts, tag, entry.Message)
}

func (m Model) viewLogsFullscreen() string {
	header := m.st.subtleStyle.Render("  Logs") + "\n" + m.divider()

	filterLabel := m.st.subtleStyle.Render("[f] All")
	if m.logFilterImportant {
		filterLabel = m.st.subtleStyle.Render("[f] Important only")
	}
	countLabel := m.st.subtleStyle.Render(fmt.Sprintf("%d/%d", m.logShown, m.logTotal))
	footer := m.divider() + "\n  " + m.st.subtleStyle.Render("[q/Esc] Back") + "  " + filterLabel + "  " + countLabel

	m.logViewport.Width = m.termWidth - chromePadH
	m.logViewport.Height = m.termHeight - 8

	return lipgloss.NewStyle().Padding(1, 2).Render(
		header + "\n" + m.logViewport.View() + "\n" + footer)
}

func (m *Model) refreshLogContent() {
	var rendered []string
	total := 0
	shown := 0

	for _, entry := range m.engine.GetLogs() {
		if strings.TrimSpace(entry.Message) == "" {
			continue
		}
		total++
		sev := classifyLog(entry.Message)
		if m.logFilterImportant && !isImportantLog(sev) {
			continue
		}
		shown++
		rendered = append(rendered, m.renderLogLine(entry))
	}

	m.logTotal = total
	m.logShown = shown
	m.logViewport.SetContent(strings.Join(rendered, "\n"))
}
