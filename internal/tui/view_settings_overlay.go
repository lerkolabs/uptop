package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) viewSettingsOverlay() string {
	overlayW := m.termWidth - 8
	if overlayW > 90 {
		overlayW = 90
	}
	if overlayW < 40 {
		overlayW = 40
	}

	overlayH := m.termHeight - 6
	if overlayH < 10 {
		overlayH = 10
	}

	savedCursor, savedOffset := m.cursor, m.tableOffset
	savedContentW := m.contentWidth
	m.cursor = m.settingsCursor
	m.tableOffset = m.settingsOffset
	m.contentWidth = overlayW - 4

	content := m.viewSettingsTab()

	m.cursor = savedCursor
	m.tableOffset = savedOffset
	m.contentWidth = savedContentW

	settingsListLen := m.settingsListLen()
	footer := m.settingsFooterKeys()
	if settingsListLen > 0 {
		footer = fmt.Sprintf("%d items  %s", settingsListLen, footer)
	}
	footerLine := m.st.subtleStyle.Render(footer)

	inner := content + "\n" + footerLine
	box := m.titledPanel("Settings", inner, overlayW, true)

	boxLines := lipgloss.Height(box)
	if boxLines > overlayH {
		lines := splitLines(box, overlayH)
		box = lines
	}

	return placeOverlay(box, m.termWidth, m.termHeight)
}

func (m Model) settingsFooterKeys() string {
	switch m.settingsSection {
	case sectionAlerts:
		return "[n]New [e]Edit [i]Info [d]Del [t]Test [←/→]Section [Esc]Close"
	case sectionUsers:
		if m.isAdmin {
			return "[n]Add [d]Revoke [←/→]Section [Esc]Close"
		}
		return "[←/→]Section [Esc]Close"
	default:
		return "[←/→]Section [Esc]Close"
	}
}

func (m Model) settingsListLen() int {
	switch m.settingsSection {
	case sectionAlerts:
		return len(m.alerts)
	case sectionNodes:
		return len(m.nodes)
	case sectionUsers:
		return len(m.users)
	}
	return 0
}

func splitLines(s string, maxLines int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return strings.Join(lines, "\n")
}
