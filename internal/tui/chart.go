package tui

import (
	"fmt"
	"strings"
	"time"
)

func (m Model) latencyChart(latencies []time.Duration, statuses []bool, width, height int) string {
	if len(latencies) == 0 || width < 20 || height < 1 {
		return ""
	}

	var minMs, maxMs, sumMs int64
	minMs = latencies[0].Milliseconds()
	maxMs = minMs
	upCount := 0
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
		upCount++
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

	samples := latencies
	sampledStatuses := statuses
	if len(samples) > chartW {
		samples = samples[len(samples)-chartW:]
		if len(sampledStatuses) > chartW {
			sampledStatuses = sampledStatuses[len(sampledStatuses)-chartW:]
		}
	}

	spread := maxMs - minMs
	if spread == 0 {
		spread = 1
	}
	totalLevels := height * 8

	levels := make([]int, len(samples))
	for i, l := range samples {
		ms := l.Milliseconds()
		if i < len(sampledStatuses) && !sampledStatuses[i] {
			levels[i] = 0
			continue
		}
		lvl := int(float64(ms-minMs) / float64(spread) * float64(totalLevels-1))
		if lvl < 1 && ms > 0 {
			lvl = 1
		}
		if lvl >= totalLevels {
			lvl = totalLevels - 1
		}
		levels[i] = lvl
	}

	labelStyle := m.st.subtleStyle

	var rows []string
	for row := height - 1; row >= 0; row-- {
		var label string
		switch row {
		case height - 1:
			label = fmt.Sprintf("%*s", labelW, maxLabel)
		case 0:
			label = fmt.Sprintf("%*s", labelW, minLabel)
		default:
			label = strings.Repeat(" ", labelW)
		}

		var sb strings.Builder
		sb.WriteString(labelStyle.Render(label))

		rowBase := row * 8
		for i := range samples {
			fill := levels[i] - rowBase
			if fill <= 0 {
				sb.WriteString(" ")
				continue
			}
			if fill > 8 {
				fill = 8
			}
			ch := string(sparkChars[fill-1])

			isDown := i < len(sampledStatuses) && !sampledStatuses[i]
			if isDown {
				sb.WriteString(m.st.dangerStyle.Render(ch))
			} else {
				sb.WriteString(m.latencyStyle(samples[i].Milliseconds(), nil).Render(ch))
			}
		}
		rows = append(rows, sb.String())
	}

	stats := fmt.Sprintf("%*s Min %s  Avg %s  Max %s",
		labelW, "",
		m.fmtLatency(time.Duration(minMs)*time.Millisecond),
		m.fmtLatency(time.Duration(avgMs)*time.Millisecond),
		m.fmtLatency(time.Duration(maxMs)*time.Millisecond))
	rows = append(rows, stats)

	return strings.Join(rows, "\n")
}
