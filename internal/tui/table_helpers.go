package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

type StyleOverride func(row, col int) *lipgloss.Style

const (
	wideBreakpoint   = 120
	mediumBreakpoint = 90
)

func (m Model) isWide() bool {
	w := m.contentWidth
	if w == 0 {
		w = m.termWidth
	}
	return w >= wideBreakpoint
}

func (m Model) renderTable(headers []string, items int, buildRows func(start, end int) [][]string, colWidths []int, styleOverride StyleOverride) string {
	if items == 0 {
		return ""
	}

	end := m.tableOffset + m.maxTableRows
	if end > items {
		end = items
	}

	selectedVisual := m.cursor - m.tableOffset
	rows := buildRows(m.tableOffset, end)

	colTotal := 0
	for _, w := range colWidths {
		colTotal += w
	}
	borderOverhead := 2 + len(colWidths) - 1
	tableWidth := colTotal + borderOverhead
	cw := m.contentWidth
	if cw == 0 {
		cw = m.termWidth
	}
	maxWidth := cw - chromePadH - 2
	if tableWidth > maxWidth {
		tableWidth = maxWidth
	}
	if tableWidth < 40 {
		tableWidth = 40
	}

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(m.st.tableBorderStyle).
		Width(tableWidth).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				h := m.st.tableHeaderStyle
				if col < len(colWidths) && colWidths[col] > 0 {
					h = h.Width(colWidths[col]).MaxWidth(colWidths[col])
				}
				return h
			}
			isSelected := row == selectedVisual
			if styleOverride != nil {
				if s := styleOverride(row, col); s != nil {
					style := *s
					if row%2 == 1 {
						style = style.Background(m.st.tableZebraStyle.GetBackground())
					}
					if isSelected {
						style = m.st.tableSelectedStyle.Foreground(s.GetForeground())
					}
					if col < len(colWidths) && colWidths[col] > 0 {
						style = style.Width(colWidths[col]).MaxWidth(colWidths[col])
					}
					return style
				}
			}
			base := m.st.tableCellStyle
			if row%2 == 1 {
				base = m.st.tableZebraStyle
			}
			if isSelected {
				base = m.st.tableSelectedStyle
			}
			if col < len(colWidths) && colWidths[col] > 0 {
				base = base.Width(colWidths[col]).MaxWidth(colWidths[col])
			}
			return base
		})

	return "\n" + t.Render()
}
