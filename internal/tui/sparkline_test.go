package tui

import (
	"strings"
	"testing"
	"time"
)

func TestLatencySparkline_Empty(t *testing.T) {
	got := latencySparkline(nil, nil, 10)
	if !strings.Contains(got, "··········") {
		t.Errorf("empty sparkline should be dots, got %q", got)
	}
}

func TestLatencySparkline_SingleValue(t *testing.T) {
	latencies := []time.Duration{100 * time.Millisecond}
	statuses := []bool{true}
	got := latencySparkline(latencies, statuses, 5)
	if len(got) == 0 {
		t.Error("sparkline should not be empty")
	}
}

func TestLatencySparkline_WidthTruncation(t *testing.T) {
	latencies := make([]time.Duration, 20)
	statuses := make([]bool, 20)
	for i := range latencies {
		latencies[i] = time.Duration(i*50) * time.Millisecond
		statuses[i] = true
	}
	got := latencySparkline(latencies, statuses, 5)
	if len(got) == 0 {
		t.Error("sparkline should not be empty")
	}
}

func TestHeartbeatSparkline_Empty(t *testing.T) {
	got := heartbeatSparkline(nil, 10)
	if !strings.Contains(got, "··········") {
		t.Errorf("empty heartbeat should be dots, got %q", got)
	}
}

func TestHeartbeatSparkline_Mixed(t *testing.T) {
	statuses := []bool{true, false, true, true, false}
	got := heartbeatSparkline(statuses, 5)
	if len(got) == 0 {
		t.Error("heartbeat sparkline should not be empty")
	}
}

func TestHeartbeatSparkline_PaddedWidth(t *testing.T) {
	statuses := []bool{true, true}
	got := heartbeatSparkline(statuses, 5)
	if !strings.Contains(got, "···") {
		t.Errorf("should have dot padding for width > data, got %q", got)
	}
}
