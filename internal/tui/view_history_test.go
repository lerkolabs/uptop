package tui

import (
	"testing"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

func TestComputeOutageDuration(t *testing.T) {
	now := time.Date(2026, 6, 3, 14, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		changes []models.StateChange
		idx     int
		want    time.Duration
	}{
		{
			"recovery with preceding DOWN",
			[]models.StateChange{
				{ToStatus: "UP", ChangedAt: now},
				{ToStatus: "DOWN", ChangedAt: now.Add(-10 * time.Minute)},
			},
			0,
			10 * time.Minute,
		},
		{
			"not a recovery transition",
			[]models.StateChange{
				{ToStatus: "DOWN", ChangedAt: now},
				{ToStatus: "UP", ChangedAt: now.Add(-1 * time.Hour)},
			},
			0,
			0,
		},
		{
			"no preceding entry",
			[]models.StateChange{
				{ToStatus: "UP", ChangedAt: now},
			},
			0,
			0,
		},
		{
			"preceding is also UP",
			[]models.StateChange{
				{ToStatus: "UP", ChangedAt: now},
				{ToStatus: "UP", ChangedAt: now.Add(-5 * time.Minute)},
			},
			0,
			0,
		},
		{
			"empty slice",
			[]models.StateChange{},
			0,
			0,
		},
		{
			"middle of list",
			[]models.StateChange{
				{ToStatus: "DOWN", ChangedAt: now},
				{ToStatus: "UP", ChangedAt: now.Add(-30 * time.Minute)},
				{ToStatus: "DOWN", ChangedAt: now.Add(-2 * time.Hour)},
			},
			1,
			90 * time.Minute,
		},
		{
			"recovery from LATE",
			[]models.StateChange{
				{ToStatus: "UP", ChangedAt: now},
				{ToStatus: "LATE", ChangedAt: now.Add(-5 * time.Minute)},
			},
			0,
			5 * time.Minute,
		},
		{
			"recovery from SSL EXP",
			[]models.StateChange{
				{ToStatus: "UP", ChangedAt: now},
				{ToStatus: "SSL EXP", ChangedAt: now.Add(-1 * time.Hour)},
			},
			0,
			1 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.idx >= len(tt.changes) {
				if tt.want != 0 {
					t.Fatalf("invalid test: idx %d out of range", tt.idx)
				}
				return
			}
			got := computeOutageDuration(tt.changes, tt.idx)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestComputeHistoryStats(t *testing.T) {
	now := time.Date(2026, 6, 3, 14, 0, 0, 0, time.UTC)
	changes := []models.StateChange{
		{ToStatus: "UP", ChangedAt: now},
		{ToStatus: "DOWN", ChangedAt: now.Add(-10 * time.Minute)},
		{ToStatus: "UP", ChangedAt: now.Add(-1 * time.Hour)},
		{ToStatus: "DOWN", ChangedAt: now.Add(-3 * time.Hour)},
	}

	stats := computeHistoryStats(changes)

	if stats.totalEvents != 4 {
		t.Errorf("totalEvents: got %d, want 4", stats.totalEvents)
	}
	if stats.outageCount != 2 {
		t.Errorf("outageCount: got %d, want 2", stats.outageCount)
	}
	expectedDowntime := 10*time.Minute + 2*time.Hour
	if stats.totalDowntime != expectedDowntime {
		t.Errorf("totalDowntime: got %v, want %v", stats.totalDowntime, expectedDowntime)
	}
}

func TestComputeHistoryStats_Empty(t *testing.T) {
	stats := computeHistoryStats(nil)
	if stats.totalEvents != 0 || stats.outageCount != 0 || stats.totalDowntime != 0 {
		t.Errorf("expected zero stats for nil, got %+v", stats)
	}
}

func TestStateChangeSparkline(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := styledModel.stateChangeSparkline(nil, 20); got != "" {
			t.Errorf("expected empty for nil, got %q", got)
		}
	})

	t.Run("single event", func(t *testing.T) {
		changes := []models.StateChange{{ChangedAt: time.Now()}}
		if got := styledModel.stateChangeSparkline(changes, 20); got != "" {
			t.Errorf("expected empty for single event, got %q", got)
		}
	})

	t.Run("two events produces output", func(t *testing.T) {
		now := time.Now()
		changes := []models.StateChange{
			{ChangedAt: now},
			{ChangedAt: now.Add(-1 * time.Hour)},
		}
		got := styledModel.stateChangeSparkline(changes, 20)
		if got == "" {
			t.Error("expected non-empty sparkline for two events")
		}
	})

	t.Run("width too small", func(t *testing.T) {
		now := time.Now()
		changes := []models.StateChange{
			{ChangedAt: now},
			{ChangedAt: now.Add(-1 * time.Hour)},
		}
		if got := styledModel.stateChangeSparkline(changes, 3); got != "" {
			t.Errorf("expected empty for width 3, got %q", got)
		}
	})
}
