package tui

import (
	"strings"
	"time"
)

var sparkChars = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

func latencySparkline(latencies []time.Duration, statuses []bool, width int) string {
	if len(latencies) == 0 {
		return subtleStyle.Render(strings.Repeat("·", width))
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

	var sb strings.Builder
	if remaining := width - len(samples); remaining > 0 {
		sb.WriteString(subtleStyle.Render(strings.Repeat("·", remaining)))
	}
	spread := maxL - minL
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
			sb.WriteString(dangerStyle.Render(ch))
		} else {
			ms := l.Milliseconds()
			if ms < 200 {
				sb.WriteString(specialStyle.Render(ch))
			} else if ms < 500 {
				sb.WriteString(warnStyle.Render(ch))
			} else {
				sb.WriteString(dangerStyle.Render(ch))
			}
		}
	}
	return sb.String()
}

func heartbeatSparkline(statuses []bool, width int) string {
	if len(statuses) == 0 {
		return subtleStyle.Render(strings.Repeat("·", width))
	}

	samples := statuses
	if len(samples) > width {
		samples = samples[len(samples)-width:]
	}

	var sb strings.Builder
	if remaining := width - len(samples); remaining > 0 {
		sb.WriteString(subtleStyle.Render(strings.Repeat("·", remaining)))
	}
	for _, up := range samples {
		if up {
			sb.WriteString(specialStyle.Render("▁"))
		} else {
			sb.WriteString(dangerStyle.Render("█"))
		}
	}
	return sb.String()
}

func (m Model) groupSparkline(groupID int, width int) string {
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
		return subtleStyle.Render(strings.Repeat("·", width))
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
		sb.WriteString(subtleStyle.Render(strings.Repeat("·", remaining)))
	}
	for _, up := range aggregated {
		if up {
			sb.WriteString(specialStyle.Render("●"))
		} else {
			sb.WriteString(dangerStyle.Render("●"))
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
