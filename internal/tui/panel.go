package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

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
