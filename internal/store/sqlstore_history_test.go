package store

import (
	"context"
	"testing"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

func TestCheckHistory(t *testing.T) {
	s := newTestStore(t)

	if err := s.SaveCheck(context.Background(), 1, 5000000, true); err != nil {
		t.Fatalf("SaveCheck: %v", err)
	}
	if err := s.SaveCheck(context.Background(), 1, 10000000, false); err != nil {
		t.Fatalf("SaveCheck: %v", err)
	}
	if err := s.SaveCheck(context.Background(), 2, 3000000, true); err != nil {
		t.Fatalf("SaveCheck site 2: %v", err)
	}

	history, err := s.LoadAllHistory(context.Background(), 10)
	if err != nil {
		t.Fatalf("LoadAllHistory: %v", err)
	}
	if len(history[1]) != 2 {
		t.Fatalf("expected 2 records for site 1, got %d", len(history[1]))
	}
	if len(history[2]) != 1 {
		t.Fatalf("expected 1 record for site 2, got %d", len(history[2]))
	}

	upCount := 0
	for _, r := range history[1] {
		if r.IsUp {
			upCount++
		}
	}
	if upCount != 1 {
		t.Errorf("expected 1 up record for site 1, got %d", upCount)
	}
}

func TestPruneCheckHistory(t *testing.T) {
	s := newTestStore(t)

	for i := 0; i < maxCheckHistory+5; i++ {
		if err := s.SaveCheck(context.Background(), 1, int64(i), true); err != nil {
			t.Fatalf("SaveCheck site 1: %v", err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := s.SaveCheck(context.Background(), 2, int64(i), true); err != nil {
			t.Fatalf("SaveCheck site 2: %v", err)
		}
	}

	if err := s.PruneCheckHistory(context.Background()); err != nil {
		t.Fatalf("PruneCheckHistory: %v", err)
	}

	history, err := s.LoadAllHistory(context.Background(), maxCheckHistory*2)
	if err != nil {
		t.Fatalf("LoadAllHistory: %v", err)
	}
	if len(history[1]) != maxCheckHistory {
		t.Errorf("site 1: expected %d rows after prune, got %d", maxCheckHistory, len(history[1]))
	}
	if len(history[2]) != 3 {
		t.Errorf("site 2: expected 3 rows untouched, got %d", len(history[2]))
	}
}

func TestDeleteSiteCascade(t *testing.T) {
	s := newTestStore(t)

	site := models.SiteConfig{Name: "Cascade Test", URL: "https://example.com", Interval: 30}
	if err := s.AddSite(context.Background(), site); err != nil {
		t.Fatalf("AddSite: %v", err)
	}
	sites, err := s.GetSites(context.Background())
	if err != nil {
		t.Fatalf("GetSites: %v", err)
	}
	siteID := sites[0].ID

	if err := s.SaveCheck(context.Background(), siteID, 1000, true); err != nil {
		t.Fatalf("SaveCheck: %v", err)
	}
	if err := s.SaveStateChange(context.Background(), siteID, "UP", "DOWN", "timeout"); err != nil {
		t.Fatalf("SaveStateChange: %v", err)
	}
	mw := models.MaintenanceWindow{
		MonitorID: siteID,
		Title:     "Test MW",
		Type:      "maintenance",
		StartTime: time.Now(),
	}
	if err := s.AddMaintenanceWindow(context.Background(), mw); err != nil {
		t.Fatalf("AddMaintenanceWindow: %v", err)
	}

	if err := s.DeleteSite(context.Background(), siteID); err != nil {
		t.Fatalf("DeleteSite: %v", err)
	}

	history, err := s.LoadAllHistory(context.Background(), 100)
	if err != nil {
		t.Fatalf("LoadAllHistory: %v", err)
	}
	if len(history[siteID]) != 0 {
		t.Errorf("expected 0 check_history rows, got %d", len(history[siteID]))
	}

	changes, err := s.GetStateChanges(context.Background(), siteID, 100)
	if err != nil {
		t.Fatalf("GetStateChanges: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected 0 state_changes rows, got %d", len(changes))
	}

	windows, err := s.GetActiveMaintenanceWindows(context.Background())
	if err != nil {
		t.Fatalf("GetActiveMaintenanceWindows: %v", err)
	}
	for _, w := range windows {
		if w.MonitorID == siteID {
			t.Errorf("orphaned maintenance window found: id=%d", w.ID)
		}
	}
}
