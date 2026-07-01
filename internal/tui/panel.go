package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) titledPanelH(title, content, footer string, width, height, scrollOffset int, focused bool) string {
	if height <= 0 {
		return m.titledPanel(title, content, width, focused)
	}

	borderColor := m.theme.Border
	titleColor := m.theme.Muted
	if focused {
		borderColor = m.theme.Accent
		titleColor = m.theme.Accent
	}

	bc := lipgloss.NewStyle().Foreground(borderColor)
	tc := lipgloss.NewStyle().Foreground(titleColor).Bold(true)

	innerW := width - 2
	if innerW < 10 {
		innerW = 10
	}

	titleRendered := tc.Render(" " + title + " ")
	titleLen := len([]rune(title)) + 2
	fillLen := innerW - titleLen - 1
	if fillLen < 0 {
		fillLen = 0
	}

	top := bc.Render("╭─") + titleRendered + bc.Render(strings.Repeat("─", fillLen)+"╮")
	bottom := bc.Render("╰" + strings.Repeat("─", innerW) + "╯")

	contentStyle := lipgloss.NewStyle().Width(innerW).MaxWidth(innerW)
	inner := contentStyle.Render(content)
	contentLines := strings.Split(inner, "\n")

	var footerLines []string
	if footer != "" {
		footerRendered := contentStyle.Render(footer)
		footerLines = strings.Split(footerRendered, "\n")
	}

	bodyH := height - 2 - len(footerLines)
	if bodyH < 1 {
		bodyH = 1
	}

	if scrollOffset > len(contentLines)-bodyH {
		scrollOffset = len(contentLines) - bodyH
	}
	if scrollOffset < 0 {
		scrollOffset = 0
	}

	end := scrollOffset + bodyH
	if end > len(contentLines) {
		end = len(contentLines)
	}
	visible := contentLines[scrollOffset:end]

	borderLine := func(line string) string {
		return bc.Render("│") + line + strings.Repeat(" ", max(0, innerW-lipgloss.Width(line))) + bc.Render("│")
	}
	emptyLine := borderLine(strings.Repeat(" ", innerW))

	var lines []string
	lines = append(lines, top)
	for _, line := range visible {
		lines = append(lines, borderLine(line))
	}
	for len(lines) < height-1-len(footerLines) {
		lines = append(lines, emptyLine)
	}
	for _, line := range footerLines {
		lines = append(lines, borderLine(line))
	}
	lines = append(lines, bottom)

	return strings.Join(lines, "\n")
}

func (m Model) titledPanel(title, content string, width int, focused bool) string {
	borderColor := m.theme.Border
	titleColor := m.theme.Muted
	if focused {
		borderColor = m.theme.Accent
		titleColor = m.theme.Accent
	}

	bc := lipgloss.NewStyle().Foreground(borderColor)
	tc := lipgloss.NewStyle().Foreground(titleColor).Bold(true)

	innerW := width - 2
	if innerW < 10 {
		innerW = 10
	}

	titleRendered := tc.Render(" " + title + " ")
	titleLen := len([]rune(title)) + 2
	fillLen := innerW - titleLen - 1
	if fillLen < 0 {
		fillLen = 0
	}

	top := bc.Render("╭─") + titleRendered + bc.Render(strings.Repeat("─", fillLen)+"╮")

	contentStyle := lipgloss.NewStyle().Width(innerW).MaxWidth(innerW)
	inner := contentStyle.Render(content)

	var lines []string
	lines = append(lines, top)
	for _, line := range strings.Split(inner, "\n") {
		lines = append(lines, bc.Render("│")+line+strings.Repeat(" ", max(0, innerW-lipgloss.Width(line)))+bc.Render("│"))
	}
	lines = append(lines, bc.Render("╰"+strings.Repeat("─", innerW)+"╯"))

	return strings.Join(lines, "\n")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func placeOverlay(fg string, termW, termH int) string {
	return lipgloss.Place(termW, termH, lipgloss.Center, lipgloss.Center, fg)
}
