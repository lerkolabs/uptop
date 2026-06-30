package tui

import (
	"strings"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

type historyStats struct {
	totalEvents   int
	outageCount   int
	totalDowntime time.Duration
}

func computeOutageDuration(changes []models.StateChange, idx int) time.Duration {
	sc := changes[idx]
	if sc.ToStatus != string(models.StatusUp) {
		return 0
	}
	if idx+1 >= len(changes) {
		return 0
	}
	prev := changes[idx+1]
	if prev.ToStatus == string(models.StatusUp) {
		return 0
	}
	dur := sc.ChangedAt.Sub(prev.ChangedAt)
	if dur < 0 {
		return 0
	}
	return dur
}

func computeHistoryStats(changes []models.StateChange) historyStats {
	var s historyStats
	s.totalEvents = len(changes)
	for i := range changes {
		dur := computeOutageDuration(changes, i)
		if dur > 0 {
			s.outageCount++
			s.totalDowntime += dur
		}
	}
	return s
}

var stateChangeChars = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

func (m Model) stateChangeSparkline(changes []models.StateChange, width int) string {
	if len(changes) < 2 || width < 4 {
		return ""
	}

	oldest := changes[len(changes)-1].ChangedAt
	newest := changes[0].ChangedAt
	span := newest.Sub(oldest)
	if span <= 0 {
		return ""
	}

	buckets := make([]int, width)
	for _, sc := range changes {
		pos := int(float64(sc.ChangedAt.Sub(oldest)) / float64(span) * float64(width-1))
		if pos >= width {
			pos = width - 1
		}
		if pos < 0 {
			pos = 0
		}
		buckets[pos]++
	}

	maxVal := 0
	for _, v := range buckets {
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal == 0 {
		return ""
	}

	var sb strings.Builder
	for _, v := range buckets {
		if v == 0 {
			sb.WriteRune('·')
			continue
		}
		idx := int(float64(v) / float64(maxVal) * 7)
		if idx > 7 {
			idx = 7
		}
		ch := string(stateChangeChars[idx])
		switch {
		case v >= 3:
			sb.WriteString(m.st.dangerStyle.Render(ch))
		case v >= 2:
			sb.WriteString(m.st.warnStyle.Render(ch))
		default:
			sb.WriteString(m.st.subtleStyle.Render(ch))
		}
	}
	return sb.String()
}
