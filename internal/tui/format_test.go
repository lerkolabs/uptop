package tui

import (
	"testing"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

func init() {
	applyTheme(themeFlexokiDark)
}

func TestLimitStr(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 8, "hello..."},
		{"", 5, ""},
		{"abc", 3, "abc"},
		{"abcd", 3, "..."},
		{"日本語テスト", 4, "日..."},
	}
	for _, tt := range tests {
		got := limitStr(tt.input, tt.max)
		if got != tt.want {
			t.Errorf("limitStr(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
		}
	}
}

func TestSiteOrder(t *testing.T) {
	tests := []struct {
		name string
		site models.Site
		want int
	}{
		{"down", models.Site{Status: "DOWN"}, 0},
		{"ssl exp", models.Site{Status: "SSL EXP"}, 0},
		{"late", models.Site{Status: "LATE"}, 1},
		{"up", models.Site{Status: "UP"}, 2},
		{"pending", models.Site{Status: "PENDING"}, 3},
		{"paused up", models.Site{Status: "UP", Paused: true}, 3},
		{"paused down", models.Site{Status: "DOWN", Paused: true}, 3},
	}
	for _, tt := range tests {
		got := siteOrder(tt.site)
		if got != tt.want {
			t.Errorf("siteOrder(%s) = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestFmtStatus_ErrorCategory(t *testing.T) {
	tests := []struct {
		status  string
		paused  bool
		inMaint bool
		cat     ErrorCategory
		wantSub string
	}{
		{"DOWN", false, false, ErrCatDNS, "DOWN:DNS"},
		{"DOWN", false, false, ErrCatTLS, "DOWN:TLS"},
		{"DOWN", false, false, ErrCatHTTP, "DOWN:HTTP"},
		{"DOWN", false, false, ErrCatTCP, "DOWN:TCP"},
		{"DOWN", false, false, ErrCatTimeout, "DOWN:TMO"},
		{"DOWN", false, false, ErrCatICMP, "DOWN:ICMP"},
		{"DOWN", false, false, ErrCatPrivate, "DOWN:PRIV"},
		{"DOWN", false, false, ErrCatUnknown, "DOWN"},
		{"UP", false, false, ErrCatUnknown, "UP"},
		{"SSL EXP", false, false, ErrCatUnknown, "SSL EXP"},
		{"DOWN", true, false, ErrCatDNS, "PAUSED"},
		{"DOWN", false, true, ErrCatDNS, "MAINT"},
	}
	for _, tt := range tests {
		got := fmtStatus(tt.status, tt.paused, tt.inMaint, tt.cat)
		if !containsPlain(got, tt.wantSub) {
			t.Errorf("fmtStatus(%q, paused=%v, maint=%v, %q): %q missing %q",
				tt.status, tt.paused, tt.inMaint, tt.cat, got, tt.wantSub)
		}
	}
}

func TestFmtDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{2*time.Hour + 30*time.Minute, "2h 30m"},
		{2 * time.Hour, "2h"},
		{25 * time.Hour, "1d 1h"},
		{48 * time.Hour, "2d"},
		{49 * time.Hour, "2d 1h"},
	}
	for _, tt := range tests {
		got := fmtDuration(tt.d)
		if got != tt.want {
			t.Errorf("fmtDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestTypeIcon(t *testing.T) {
	tests := []struct {
		siteType  string
		collapsed bool
		want      string
	}{
		{"http", false, "→"},
		{"push", false, "↓"},
		{"ping", false, "↔"},
		{"port", false, "⊡"},
		{"dns", false, "◆"},
		{"group", false, "▼"},
		{"group", true, "▶"},
		{"unknown", false, "·"},
	}
	for _, tt := range tests {
		got := typeIcon(tt.siteType, tt.collapsed)
		if got != tt.want {
			t.Errorf("typeIcon(%q, %v) = %q, want %q", tt.siteType, tt.collapsed, got, tt.want)
		}
	}
}

func TestFmtUptime(t *testing.T) {
	tests := []struct {
		name     string
		statuses []bool
		wantSub  string
	}{
		{"empty", nil, "—"},
		{"all up", []bool{true, true, true, true}, "100.0%"},
		{"half", []bool{true, false, true, false}, "50.0%"},
		{"all down", []bool{false, false}, "0.0%"},
	}
	for _, tt := range tests {
		got := fmtUptime(tt.statuses)
		if !containsPlain(got, tt.wantSub) {
			t.Errorf("fmtUptime(%s): %q missing %q", tt.name, got, tt.wantSub)
		}
	}
}

func TestFmtLatency(t *testing.T) {
	tests := []struct {
		d       time.Duration
		wantSub string
	}{
		{0, "—"},
		{50 * time.Millisecond, "50ms"},
		{300 * time.Millisecond, "300ms"},
		{1500 * time.Millisecond, "1.5s"},
	}
	for _, tt := range tests {
		got := fmtLatency(tt.d)
		if !containsPlain(got, tt.wantSub) {
			t.Errorf("fmtLatency(%v): %q missing %q", tt.d, got, tt.wantSub)
		}
	}
}

func containsPlain(styled, sub string) bool {
	// ANSI-styled strings contain the substring somewhere
	return len(styled) > 0 && contains(styled, sub)
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return len(sub) == 0
}
