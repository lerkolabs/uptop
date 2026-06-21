package tui

import (
	"time"

	"github.com/NimbleMarkets/ntcharts/canvas/runes"
	"github.com/NimbleMarkets/ntcharts/linechart/streamlinechart"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) latencyChart(latencies []time.Duration, statuses []bool, width, height int) string {
	if len(latencies) == 0 || width < 10 || height < 3 {
		return ""
	}

	chartW := len(latencies)
	if chartW > width {
		chartW = width
	}

	lineStyle := lipgloss.NewStyle().Foreground(m.theme.Accent)
	slc := streamlinechart.New(chartW, height,
		streamlinechart.WithStyles(runes.ThinLineStyle, lineStyle),
	)

	for i, l := range latencies {
		ms := float64(l.Milliseconds())
		if i < len(statuses) && !statuses[i] {
			ms = 0
		}
		slc.Push(ms)
	}
	slc.Draw()

	return slc.View()
}
