package tui

import (
	"strings"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderCompactLogLine(entry models.LogEntry, maxW int) string {
	sev := classifyLog(entry.Message)

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

	ts := entry.CreatedAt.Local().Format("01/02 15:04")

	msg := entry.Message
	msg = strings.TrimPrefix(msg, "Monitor ")
	msg = strings.TrimPrefix(msg, "Push ")

	prefixW := len(ts) + 4
	msgW := maxW - prefixW
	if msgW < 5 {
		msgW = 5
	}
	msg = limitStr(msg, msgW)

	return " " + m.st.subtleStyle.Render(ts) + " " + tag + " " + msg
}

func (m Model) viewLogsSidebar(width, maxLines int) string {
	logs := m.engine.GetLogs()
	if len(logs) == 0 {
		return m.st.subtleStyle.Render("  No logs yet")
	}

	sidebarStyle := lipgloss.NewStyle().Width(width).MaxWidth(width)

	var all []string
	for _, entry := range logs {
		if strings.TrimSpace(entry.Message) == "" {
			continue
		}
		if m.logFilterImportant && !isImportantLog(classifyLog(entry.Message)) {
			continue
		}
		all = append(all, m.renderCompactLogLine(entry, width))
	}

	start := m.logScrollOffset
	if start > len(all) {
		start = len(all)
	}
	end := start + maxLines
	if end > len(all) {
		end = len(all)
	}
	visible := all[start:end]

	return sidebarStyle.Render(strings.Join(visible, "\n"))
}

func (m *Model) scrollLogs(delta int) {
	logs := m.engine.GetLogs()
	total := 0
	for _, entry := range logs {
		if strings.TrimSpace(entry.Message) == "" {
			continue
		}
		if m.logFilterImportant && !isImportantLog(classifyLog(entry.Message)) {
			continue
		}
		total++
	}

	m.logScrollOffset += delta
	if m.logScrollOffset < 0 {
		m.logScrollOffset = 0
	}
	if m.logScrollOffset > total-1 {
		m.logScrollOffset = total - 1
	}
	if m.logScrollOffset < 0 {
		m.logScrollOffset = 0
	}
}
