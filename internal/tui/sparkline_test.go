package tui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestLatencySparkline_Empty(t *testing.T) {
	got := latencySparkline(nil, nil, 10, "")
	if !strings.Contains(got, "··········") {
		t.Errorf("empty sparkline should be dots, got %q", got)
	}
}

func TestLatencySparkline_SingleValue(t *testing.T) {
	latencies := []time.Duration{100 * time.Millisecond}
	statuses := []bool{true}
	got := latencySparkline(latencies, statuses, 5, "")
	if len(got) == 0 {
		t.Error("sparkline should not be empty")
	}
	if !strings.Contains(got, "····") {
		t.Errorf("single value with width=5 should have 4 dot padding, got %q", got)
	}
}

func TestLatencySparkline_WidthTruncation(t *testing.T) {
	latencies := make([]time.Duration, 20)
	statuses := make([]bool, 20)
	for i := range latencies {
		latencies[i] = time.Duration(i*50) * time.Millisecond
		statuses[i] = true
	}
	got := latencySparkline(latencies, statuses, 5, "")
	if len(got) == 0 {
		t.Error("sparkline should not be empty")
	}
	if strings.Contains(got, "·") {
		t.Errorf("20 samples in width=5 should have no padding, got %q", got)
	}
}

func TestLatencySparkline_RelativeHeight(t *testing.T) {
	latencies := []time.Duration{10 * time.Millisecond, 50 * time.Millisecond, 10 * time.Millisecond}
	statuses := []bool{true, true, true}
	out := stripANSI(latencySparkline(latencies, statuses, 3, ""))
	runes := []rune(out)
	if len(runes) < 3 {
		t.Fatalf("expected 3 runes, got %d", len(runes))
	}
	if runes[0] == runes[1] {
		t.Errorf("min and max should have different bar heights, got %c %c %c", runes[0], runes[1], runes[2])
	}
}

func TestLatencyStyle_BandsProduceDifferentColors(t *testing.T) {
	sparkSuccess = "#00ff00"
	sparkWarning = "#ffff00"
	sparkDanger = "#ff0000"
	defer func() {
		sparkSuccess = ""
		sparkWarning = ""
		sparkDanger = ""
	}()

	green := latencyStyle(50, "")
	yellow := latencyStyle(300, "")
	red := latencyStyle(800, "")

	gfg := green.GetForeground()
	yfg := yellow.GetForeground()
	rfg := red.GetForeground()

	if gfg == yfg || yfg == rfg || gfg == rfg {
		t.Errorf("bands should produce distinct foreground colors: green=%v yellow=%v red=%v", gfg, yfg, rfg)
	}
}

func TestLatencyStyle_BrightnessVariesWithinBand(t *testing.T) {
	sparkSuccess = "#00ff00"
	defer func() { sparkSuccess = "" }()

	dim := latencyStyle(10, "")
	bright := latencyStyle(190, "")

	if dim.GetForeground() == bright.GetForeground() {
		t.Error("10ms and 190ms should have different brightness within green band")
	}
}

func TestLatencySparkline_OutputWidth(t *testing.T) {
	latencies := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 300 * time.Millisecond}
	statuses := []bool{true, true, true}
	got := latencySparkline(latencies, statuses, 5, "")
	count := utf8.RuneCountInString(stripANSI(got))
	if count != 5 {
		t.Errorf("expected 5 rune-width output, got %d from %q", count, got)
	}
}

func stripANSI(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

func TestHeartbeatSparkline_Empty(t *testing.T) {
	got := heartbeatSparkline(nil, 10, "")
	if !strings.Contains(got, "··········") {
		t.Errorf("empty heartbeat should be dots, got %q", got)
	}
}

func TestHeartbeatSparkline_Mixed(t *testing.T) {
	statuses := []bool{true, false, true, true, false}
	got := heartbeatSparkline(statuses, 5, "")
	if len(got) == 0 {
		t.Error("heartbeat sparkline should not be empty")
	}
}

func TestHeartbeatSparkline_PaddedWidth(t *testing.T) {
	statuses := []bool{true, true}
	got := heartbeatSparkline(statuses, 5, "")
	if !strings.Contains(got, "···") {
		t.Errorf("should have dot padding for width > data, got %q", got)
	}
}

func TestResolveSparklineIndex(t *testing.T) {
	tests := []struct {
		name       string
		x          int
		sparkWidth int
		dataLen    int
		want       int
	}{
		{"exact fit first", 0, 5, 5, 0},
		{"exact fit last", 4, 5, 5, 4},
		{"padding returns -1", 0, 10, 5, -1},
		{"padding boundary", 4, 10, 5, -1},
		{"first data after padding", 5, 10, 5, 0},
		{"last data after padding", 9, 10, 5, 4},
		{"truncated first visible", 0, 5, 20, 15},
		{"truncated last visible", 4, 5, 20, 19},
		{"single data point", 9, 10, 1, 0},
		{"single data point on padding", 0, 10, 1, -1},
		{"zero data", 0, 10, 0, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveSparklineIndex(tt.x, tt.sparkWidth, tt.dataLen)
			if got != tt.want {
				t.Errorf("resolveSparklineIndex(%d, %d, %d) = %d, want %d",
					tt.x, tt.sparkWidth, tt.dataLen, got, tt.want)
			}
		})
	}
}
