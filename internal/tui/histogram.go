package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

type latencyBucket struct {
	label string
	min   int64
	max   int64
	count int
}

func (m Model) latencyHistogram(latencies []time.Duration, statuses []bool, width int) string {
	if len(latencies) == 0 || width < 30 {
		return ""
	}

	buckets := []latencyBucket{
		{"0-50ms", 0, 50, 0},
		{"50-100ms", 50, 100, 0},
		{"100-200ms", 100, 200, 0},
		{"200-500ms", 200, 500, 0},
		{"500ms+", 500, 999999, 0},
	}

	for i, l := range latencies {
		if i < len(statuses) && !statuses[i] {
			continue
		}
		ms := l.Milliseconds()
		for j := range buckets {
			if ms >= buckets[j].min && ms < buckets[j].max {
				buckets[j].count++
				break
			}
		}
	}

	maxCount := 0
	for _, b := range buckets {
		if b.count > maxCount {
			maxCount = b.count
		}
	}
	if maxCount == 0 {
		return ""
	}

	labelW := 10
	countW := len(fmt.Sprintf("%d", maxCount))
	barW := width - labelW - countW - 4
	if barW < 5 {
		barW = 5
	}

	var sb strings.Builder
	for _, b := range buckets {
		fill := 0
		if maxCount > 0 {
			fill = b.count * barW / maxCount
		}

		var barColor lipgloss.Style
		switch {
		case b.min < 100:
			barColor = m.st.specialStyle
		case b.min < 500:
			barColor = m.st.warnStyle
		default:
			barColor = m.st.dangerStyle
		}

		bar := barColor.Render(strings.Repeat("█", fill))
		if fill < barW {
			bar += m.st.subtleStyle.Render(strings.Repeat("░", barW-fill))
		}

		label := fmt.Sprintf("%*s", labelW, b.label)
		count := fmt.Sprintf("%*d", countW, b.count)
		sb.WriteString("  " + m.st.subtleStyle.Render(label) + " " + bar + " " + count + "\n")
	}

	return sb.String()
}
