package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var sparkChars = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

func parseHex(hex string) (r, g, b uint8) {
	if len(hex) == 7 && hex[0] == '#' {
		fmt.Sscanf(hex[1:], "%02x%02x%02x", &r, &g, &b)
	}
	return
}

func dimColor(hex string, brightness float64) lipgloss.Color {
	r, g, b := parseHex(hex)
	f := 0.3 + brightness*0.7
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x",
		uint8(float64(r)*f),
		uint8(float64(g)*f),
		uint8(float64(b)*f),
	))
}

func withBg(s lipgloss.Style, bg lipgloss.Color) lipgloss.Style {
	if bg != "" {
		return s.Background(bg)
	}
	return s
}

func latencyStyle(ms int64, bg lipgloss.Color) lipgloss.Style {
	var hex string
	var t float64
	switch {
	case ms < 200:
		hex = sparkSuccess
		t = float64(ms) / 200
	case ms < 500:
		hex = sparkWarning
		t = float64(ms-200) / 300
	default:
		hex = sparkDanger
		t = float64(ms-500) / 1500
		if t > 1 {
			t = 1
		}
	}
	s := lipgloss.NewStyle().Foreground(dimColor(hex, t))
	return withBg(s, bg)
}

func latencySparkline(latencies []time.Duration, statuses []bool, width int, bg lipgloss.Color) string {
	if len(latencies) == 0 {
		return withBg(subtleStyle, bg).Render(strings.Repeat("·", width))
	}

	samples := latencies
	sampledStatuses := statuses
	if len(samples) > width {
		samples = samples[len(samples)-width:]
		if len(sampledStatuses) > width {
			sampledStatuses = sampledStatuses[len(sampledStatuses)-width:]
		}
	}

	minL, maxL := samples[0], samples[0]
	for _, l := range samples {
		if l < minL {
			minL = l
		}
		if l > maxL {
			maxL = l
		}
	}
	spread := maxL - minL

	var sb strings.Builder
	if remaining := width - len(samples); remaining > 0 {
		sb.WriteString(withBg(subtleStyle, bg).Render(strings.Repeat("·", remaining)))
	}
	for i, l := range samples {
		idx := 0
		if spread > 0 {
			idx = int(float64(l-minL) / float64(spread) * 7)
			if idx > 7 {
				idx = 7
			}
		}
		ch := string(sparkChars[idx])
		isDown := i < len(sampledStatuses) && !sampledStatuses[i]
		if isDown {
			sb.WriteString(withBg(dangerStyle, bg).Render(ch))
		} else {
			sb.WriteString(latencyStyle(l.Milliseconds(), bg).Render(ch))
		}
	}
	return sb.String()
}

func heartbeatSparkline(statuses []bool, width int, bg lipgloss.Color) string {
	if len(statuses) == 0 {
		return withBg(subtleStyle, bg).Render(strings.Repeat("·", width))
	}

	samples := statuses
	if len(samples) > width {
		samples = samples[len(samples)-width:]
	}

	var sb strings.Builder
	if remaining := width - len(samples); remaining > 0 {
		sb.WriteString(withBg(subtleStyle, bg).Render(strings.Repeat("·", remaining)))
	}
	for _, up := range samples {
		if up {
			sb.WriteString(withBg(specialStyle, bg).Render("▁"))
		} else {
			sb.WriteString(withBg(dangerStyle, bg).Render("█"))
		}
	}
	return sb.String()
}

func (m Model) groupSparkline(groupID int, width int, bg lipgloss.Color) string {
	allSites := m.engine.GetAllSites()
	var childStatuses [][]bool
	for _, s := range allSites {
		if s.ParentID == groupID && !s.Paused && !m.isMonitorInMaintenance(s.ID) {
			hist, _ := m.engine.GetHistory(s.ID)
			if len(hist.Statuses) > 0 {
				childStatuses = append(childStatuses, hist.Statuses)
			}
		}
	}

	if len(childStatuses) == 0 {
		return withBg(subtleStyle, bg).Render(strings.Repeat("·", width))
	}

	maxLen := 0
	for _, s := range childStatuses {
		if len(s) > maxLen {
			maxLen = len(s)
		}
	}
	if maxLen > width {
		maxLen = width
	}

	aggregated := make([]bool, maxLen)
	for i := 0; i < maxLen; i++ {
		allUp := true
		for _, statuses := range childStatuses {
			idx := len(statuses) - maxLen + i
			if idx >= 0 && !statuses[idx] {
				allUp = false
				break
			}
		}
		aggregated[i] = allUp
	}

	var sb strings.Builder
	if remaining := width - len(aggregated); remaining > 0 {
		sb.WriteString(withBg(subtleStyle, bg).Render(strings.Repeat("·", remaining)))
	}
	for _, up := range aggregated {
		if up {
			sb.WriteString(withBg(subtleStyle, bg).Render("·"))
		} else {
			sb.WriteString(withBg(dangerStyle, bg).Render("•"))
		}
	}
	return sb.String()
}

func (m Model) groupUptime(groupID int) string {
	allSites := m.engine.GetAllSites()
	var allStatuses [][]bool
	for _, s := range allSites {
		if s.ParentID == groupID && !s.Paused && !m.isMonitorInMaintenance(s.ID) {
			hist, _ := m.engine.GetHistory(s.ID)
			if len(hist.Statuses) > 0 {
				allStatuses = append(allStatuses, hist.Statuses)
			}
		}
	}
	if len(allStatuses) == 0 {
		return subtleStyle.Render("—")
	}
	total, up := 0, 0
	for _, statuses := range allStatuses {
		for _, s := range statuses {
			total++
			if s {
				up++
			}
		}
	}
	return fmtUptime(func() []bool {
		out := make([]bool, total)
		idx := 0
		for _, statuses := range allStatuses {
			copy(out[idx:], statuses)
			idx += len(statuses)
		}
		return out
	}())
}
