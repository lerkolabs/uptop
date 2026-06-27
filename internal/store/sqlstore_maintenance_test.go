package store

import (
	"context"
	"testing"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

func TestPruneExpiredMaintenanceWindows(t *testing.T) {
	s := newTestStore(t)

	now := time.Now()

	// Expired 10 days ago — should be pruned with 7d retention.
	old := models.MaintenanceWindow{
		MonitorID: 0,
		Title:     "Old Window",
		Type:      "maintenance",
		StartTime: now.Add(-11 * 24 * time.Hour),
		EndTime:   now.Add(-10 * 24 * time.Hour),
	}
	if err := s.AddMaintenanceWindow(context.Background(), old); err != nil {
		t.Fatalf("AddMaintenanceWindow (old): %v", err)
	}

	// Expired 1 day ago — within 7d retention, should survive.
	recent := models.MaintenanceWindow{
		MonitorID: 0,
		Title:     "Recent Window",
		Type:      "maintenance",
		StartTime: now.Add(-2 * 24 * time.Hour),
		EndTime:   now.Add(-1 * 24 * time.Hour),
	}
	if err := s.AddMaintenanceWindow(context.Background(), recent); err != nil {
		t.Fatalf("AddMaintenanceWindow (recent): %v", err)
	}

	// Ongoing — no end time, should survive.
	ongoing := models.MaintenanceWindow{
		MonitorID: 0,
		Title:     "Ongoing Window",
		Type:      "maintenance",
		StartTime: now.Add(-1 * time.Hour),
	}
	if err := s.AddMaintenanceWindow(context.Background(), ongoing); err != nil {
		t.Fatalf("AddMaintenanceWindow (ongoing): %v", err)
	}

	pruned, err := s.PruneExpiredMaintenanceWindows(context.Background(), 7*24*time.Hour)
	if err != nil {
		t.Fatalf("PruneExpiredMaintenanceWindows: %v", err)
	}
	if pruned != 1 {
		t.Errorf("expected 1 pruned, got %d", pruned)
	}

	all, err := s.GetAllMaintenanceWindows(context.Background(), 100)
	if err != nil {
		t.Fatalf("GetAllMaintenanceWindows: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 remaining windows, got %d", len(all))
	}
	for _, w := range all {
		if w.Title == "Old Window" {
			t.Error("old window should have been pruned")
		}
	}
}

func TestGetOverlappingMaintenanceWindows(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	site := models.SiteConfig{Name: "web", URL: "https://example.com", Interval: 30}
	if err := s.AddSite(ctx, site); err != nil {
		t.Fatalf("AddSite: %v", err)
	}

	now := time.Now()

	active := models.MaintenanceWindow{
		MonitorID: 1,
		Title:     "Deploy v2",
		Type:      "maintenance",
		StartTime: now.Add(-30 * time.Minute),
		EndTime:   now.Add(30 * time.Minute),
	}
	if err := s.AddMaintenanceWindow(ctx, active); err != nil {
		t.Fatalf("AddMaintenanceWindow: %v", err)
	}

	ended := models.MaintenanceWindow{
		MonitorID: 1,
		Title:     "Old deploy",
		Type:      "maintenance",
		StartTime: now.Add(-3 * time.Hour),
		EndTime:   now.Add(-2 * time.Hour),
	}
	if err := s.AddMaintenanceWindow(ctx, ended); err != nil {
		t.Fatalf("AddMaintenanceWindow: %v", err)
	}

	t.Run("same monitor overlaps", func(t *testing.T) {
		overlaps, err := s.GetOverlappingMaintenanceWindows(ctx, 1, now, now.Add(1*time.Hour))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(overlaps) != 1 {
			t.Fatalf("expected 1 overlap, got %d", len(overlaps))
		}
		if overlaps[0].Title != "Deploy v2" {
			t.Errorf("expected 'Deploy v2', got %q", overlaps[0].Title)
		}
	})

	t.Run("different monitor no overlap", func(t *testing.T) {
		overlaps, err := s.GetOverlappingMaintenanceWindows(ctx, 99, now, now.Add(1*time.Hour))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(overlaps) != 0 {
			t.Errorf("expected 0 overlaps, got %d", len(overlaps))
		}
	})

	t.Run("global window overlaps all", func(t *testing.T) {
		global := models.MaintenanceWindow{
			MonitorID: 0,
			Title:     "Global freeze",
			Type:      "maintenance",
			StartTime: now.Add(-10 * time.Minute),
			EndTime:   now.Add(2 * time.Hour),
		}
		if err := s.AddMaintenanceWindow(ctx, global); err != nil {
			t.Fatalf("AddMaintenanceWindow: %v", err)
		}
		overlaps, err := s.GetOverlappingMaintenanceWindows(ctx, 1, now, now.Add(1*time.Hour))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(overlaps) != 2 {
			t.Errorf("expected 2 overlaps (specific + global), got %d", len(overlaps))
		}
	})

	t.Run("indefinite window overlaps", func(t *testing.T) {
		overlaps, err := s.GetOverlappingMaintenanceWindows(ctx, 1, now, time.Time{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(overlaps) < 1 {
			t.Error("expected at least 1 overlap for indefinite window")
		}
	})

	t.Run("ended window excluded", func(t *testing.T) {
		overlaps, err := s.GetOverlappingMaintenanceWindows(ctx, 1, now.Add(-4*time.Hour), now.Add(-3*time.Hour))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(overlaps) != 0 {
			t.Errorf("expected 0 overlaps for past range, got %d", len(overlaps))
		}
	})
}
