package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/NimbleMarkets/ntcharts/sparkline"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) latencyChart(latencies []time.Duration, statuses []bool, width, height int) string {
	if len(latencies) == 0 || width < 20 || height < 2 {
		return ""
	}

	var minMs, maxMs, sumMs int64
	minMs = latencies[0].Milliseconds()
	maxMs = minMs
	for i, l := range latencies {
		ms := l.Milliseconds()
		if i < len(statuses) && !statuses[i] {
			continue
		}
		if ms < minMs {
			minMs = ms
		}
		if ms > maxMs {
			maxMs = ms
		}
		sumMs += ms
	}
	upCount := 0
	for _, s := range statuses {
		if s {
			upCount++
		}
	}
	var avgMs int64
	if upCount > 0 {
		avgMs = sumMs / int64(upCount)
	}

	maxLabel := fmt.Sprintf("%dms", maxMs)
	minLabel := fmt.Sprintf("%dms", minMs)
	labelW := len(maxLabel)
	if len(minLabel) > labelW {
		labelW = len(minLabel)
	}
	labelW += 1

	chartW := width - labelW
	if chartW > len(latencies) {
		chartW = len(latencies)
	}
	if chartW < 10 {
		chartW = 10
	}

	style := lipgloss.NewStyle().Foreground(m.theme.Accent)
	sl := sparkline.New(chartW, height, sparkline.WithStyle(style))

	vals := make([]float64, len(latencies))
	for i, l := range latencies {
		ms := float64(l.Milliseconds())
		if i < len(statuses) && !statuses[i] {
			ms = 0
		}
		vals[i] = ms
	}
	sl.PushAll(vals)
	sl.Draw()

	chartLines := strings.Split(sl.View(), "\n")

	var result []string
	labelStyle := m.st.subtleStyle
	for i, line := range chartLines {
		var label string
		if i == 0 {
			label = fmt.Sprintf("%*s", labelW, maxLabel)
		} else if i == len(chartLines)-1 {
			label = fmt.Sprintf("%*s", labelW, minLabel)
		} else {
			label = strings.Repeat(" ", labelW)
		}
		result = append(result, labelStyle.Render(label)+line)
	}

	stats := fmt.Sprintf("%*s Min %s  Avg %s  Max %s",
		labelW, "",
		m.fmtLatency(time.Duration(minMs)*time.Millisecond),
		m.fmtLatency(time.Duration(avgMs)*time.Millisecond),
		m.fmtLatency(time.Duration(maxMs)*time.Millisecond))
	result = append(result, stats)

	return strings.Join(result, "\n")
}
