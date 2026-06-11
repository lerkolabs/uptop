package config

import (
	"context"
	"strings"
	"testing"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/store"
)

func newTestStore(t *testing.T) store.Store {
	t.Helper()
	s, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	if err := s.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return s
}

func TestApplyCreateFromScratch(t *testing.T) {
	s := newTestStore(t)
	f := &File{
		Alerts: []Alert{
			{Name: "Discord", Type: "discord", Settings: map[string]string{"url": "https://example.com"}},
		},
		Monitors: []Monitor{
			{Name: "Web", Type: "http", URL: "https://example.com", Interval: 30, Alert: "Discord"},
			{Name: "Ping", Type: "ping", Hostname: "10.0.0.1", Interval: 30},
		},
	}

	changes, err := Apply(context.Background(), s, f, ApplyOpts{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	creates := 0
	for _, c := range changes {
		if c.Action == "create" {
			creates++
		}
	}
	if creates != 3 {
		t.Fatalf("expected 3 creates, got %d", creates)
	}

	sites, _ := s.GetSites(context.Background())
	if len(sites) != 2 {
		t.Fatalf("expected 2 sites, got %d", len(sites))
	}

	alerts, _ := s.GetAllAlerts(context.Background())
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
}

func TestApplyIdempotent(t *testing.T) {
	s := newTestStore(t)
	f := &File{
		Alerts: []Alert{
			{Name: "Discord", Type: "discord", Settings: map[string]string{"url": "https://example.com"}},
		},
		Monitors: []Monitor{
			{Name: "Web", Type: "http", URL: "https://example.com", Interval: 30, Alert: "Discord"},
		},
	}

	if _, err := Apply(context.Background(), s, f, ApplyOpts{}); err != nil {
		t.Fatalf("first Apply: %v", err)
	}

	changes, err := Apply(context.Background(), s, f, ApplyOpts{})
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}

	if len(changes) != 0 {
		t.Fatalf("expected 0 changes on second apply, got %d: %+v", len(changes), changes)
	}
}

func TestApplyUpdate(t *testing.T) {
	s := newTestStore(t)
	f := &File{
		Monitors: []Monitor{
			{Name: "Web", Type: "http", URL: "https://example.com", Interval: 30},
		},
	}

	if _, err := Apply(context.Background(), s, f, ApplyOpts{}); err != nil {
		t.Fatalf("first Apply: %v", err)
	}

	f.Monitors[0].Interval = 60
	changes, err := Apply(context.Background(), s, f, ApplyOpts{})
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}

	if len(changes) != 1 || changes[0].Action != "update" {
		t.Fatalf("expected 1 update, got %+v", changes)
	}

	sites, _ := s.GetSites(context.Background())
	if sites[0].Interval != 60 {
		t.Fatalf("expected interval 60, got %d", sites[0].Interval)
	}
}

func TestApplyPrune(t *testing.T) {
	s := newTestStore(t)
	s.AddSite(context.Background(), models.Site{Name: "Keep", URL: "https://keep.com", Type: "http", Interval: 30, ExpiryThreshold: 7, Method: "GET", AcceptedCodes: "200-299"})
	s.AddSite(context.Background(), models.Site{Name: "Remove", URL: "https://remove.com", Type: "http", Interval: 30, ExpiryThreshold: 7, Method: "GET", AcceptedCodes: "200-299"})

	f := &File{
		Monitors: []Monitor{
			{Name: "Keep", Type: "http", URL: "https://keep.com", Interval: 30},
		},
	}

	changes, err := Apply(context.Background(), s, f, ApplyOpts{Prune: true})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	deleteCount := 0
	for _, c := range changes {
		if c.Action == "delete" {
			deleteCount++
		}
	}
	if deleteCount != 1 {
		t.Fatalf("expected 1 delete, got %d", deleteCount)
	}

	sites, _ := s.GetSites(context.Background())
	if len(sites) != 1 || sites[0].Name != "Keep" {
		t.Fatalf("expected only 'Keep', got %+v", sites)
	}
}

func TestApplyDryRun(t *testing.T) {
	s := newTestStore(t)
	f := &File{
		Monitors: []Monitor{
			{Name: "Web", Type: "http", URL: "https://example.com", Interval: 30},
		},
	}

	changes, err := Apply(context.Background(), s, f, ApplyOpts{DryRun: true})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if len(changes) != 1 || changes[0].Action != "create" {
		t.Fatalf("expected 1 create in dry-run, got %+v", changes)
	}

	sites, _ := s.GetSites(context.Background())
	if len(sites) != 0 {
		t.Fatalf("expected 0 sites after dry-run, got %d", len(sites))
	}
}

func TestApplyGroupHierarchy(t *testing.T) {
	s := newTestStore(t)
	f := &File{
		Monitors: []Monitor{
			{
				Name: "Prod", Type: "group",
				Monitors: []Monitor{
					{Name: "Prod Web", Type: "http", URL: "https://prod.example.com", Interval: 15},
					{Name: "Prod DB", Type: "port", Hostname: "db.internal", Port: 5432, Interval: 30},
				},
			},
		},
	}

	changes, err := Apply(context.Background(), s, f, ApplyOpts{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if len(changes) != 3 {
		t.Fatalf("expected 3 creates, got %d", len(changes))
	}

	sites, _ := s.GetSites(context.Background())
	var group models.Site
	for _, s := range sites {
		if s.Type == "group" {
			group = s
			break
		}
	}

	if group.ID == 0 {
		t.Fatal("group not found")
	}

	childCount := 0
	for _, s := range sites {
		if s.ParentID == group.ID {
			childCount++
		}
	}
	if childCount != 2 {
		t.Fatalf("expected 2 children, got %d", childCount)
	}
}

func TestApplyAlertReference(t *testing.T) {
	s := newTestStore(t)
	f := &File{
		Alerts: []Alert{
			{Name: "Discord", Type: "discord", Settings: map[string]string{"url": "https://example.com"}},
		},
		Monitors: []Monitor{
			{Name: "Web", Type: "http", URL: "https://example.com", Interval: 30, Alert: "Discord"},
		},
	}

	if _, err := Apply(context.Background(), s, f, ApplyOpts{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	sites, _ := s.GetSites(context.Background())
	alerts, _ := s.GetAllAlerts(context.Background())

	if sites[0].AlertID != alerts[0].ID {
		t.Fatalf("expected alert_id %d, got %d", alerts[0].ID, sites[0].AlertID)
	}
}

func TestApplyInvalidAlertRef(t *testing.T) {
	s := newTestStore(t)
	f := &File{
		Monitors: []Monitor{
			{Name: "Web", Type: "http", URL: "https://example.com", Interval: 30, Alert: "Nonexistent"},
		},
	}

	_, err := Apply(context.Background(), s, f, ApplyOpts{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected alert not found error, got %v", err)
	}
}

func TestApplyDuplicateNames(t *testing.T) {
	s := newTestStore(t)
	f := &File{
		Monitors: []Monitor{
			{Name: "Web", Type: "http", URL: "https://a.com", Interval: 30},
			{Name: "Web", Type: "http", URL: "https://b.com", Interval: 30},
		},
	}

	_, err := Apply(context.Background(), s, f, ApplyOpts{})
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestApplyExistingAlertReference(t *testing.T) {
	s := newTestStore(t)
	s.AddAlert(context.Background(), "Existing", "webhook", map[string]string{"url": "https://example.com"})

	f := &File{
		Monitors: []Monitor{
			{Name: "Web", Type: "http", URL: "https://example.com", Interval: 30, Alert: "Existing"},
		},
	}

	changes, err := Apply(context.Background(), s, f, ApplyOpts{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if len(changes) != 1 || changes[0].Action != "create" {
		t.Fatalf("expected 1 create, got %+v", changes)
	}

	sites, _ := s.GetSites(context.Background())
	if sites[0].AlertID == 0 {
		t.Fatal("expected non-zero alert_id for existing alert reference")
	}
}
