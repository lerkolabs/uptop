package tui

import (
	"strings"
)

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
