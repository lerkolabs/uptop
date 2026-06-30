package tui

import (
	"fmt"
	"math"
	"strings"
	"time"
)

var slaPeriods = []struct {
	label    string
	key      string
	duration time.Duration
	days     int
}{
	{"24h", "1", 24 * time.Hour, 1},
	{"7d", "2", 7 * 24 * time.Hour, 7},
	{"30d", "3", 30 * 24 * time.Hour, 30},
	{"90d", "4", 90 * 24 * time.Hour, 90},
}

func (m Model) uptimeBar(pct float64, width int) string {
	filled := int(math.Round(pct / 100 * float64(width)))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	empty := width - filled

	bar := m.st.specialStyle.Render(strings.Repeat("█", filled))
	if empty > 0 {
		bar += m.st.subtleStyle.Render(strings.Repeat("░", empty))
	}
	return bar
}

func fmtPct(pct float64) string {
	if pct == 100 {
		return "100.00"
	}
	if pct >= uptimePrecisionPct {
		return fmt.Sprintf("%.3f", pct)
	}
	return fmt.Sprintf("%.2f", pct)
}
