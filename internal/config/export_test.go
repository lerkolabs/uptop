package config

import (
	"context"
	"testing"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

func TestExportEmpty(t *testing.T) {
	s := newTestStore(t)
	f, err := Export(context.Background(), s)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(f.Alerts) != 0 || len(f.Monitors) != 0 {
		t.Fatalf("expected empty file, got %d alerts %d monitors", len(f.Alerts), len(f.Monitors))
	}
}

func TestExportAlertNames(t *testing.T) {
	s := newTestStore(t)
	s.AddAlert(context.Background(), "Discord", "discord", map[string]string{"url": "https://example.com"})
	alerts, _ := s.GetAllAlerts(context.Background())
	s.AddSite(context.Background(), models.Site{Name: "Web", URL: "https://example.com", Type: "http", Interval: 30, AlertID: alerts[0].ID, ExpiryThreshold: 7, Method: "GET", AcceptedCodes: "200-299"})

	f, err := Export(context.Background(), s)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	if len(f.Monitors) != 1 {
		t.Fatalf("expected 1 monitor, got %d", len(f.Monitors))
	}
	if f.Monitors[0].Alert != "Discord" {
		t.Fatalf("expected alert name 'Discord', got %q", f.Monitors[0].Alert)
	}
}

func TestExportGroupHierarchy(t *testing.T) {
	s := newTestStore(t)
	groupID, _ := s.AddSiteReturningID(context.Background(), models.Site{Name: "Prod", Type: "group", ExpiryThreshold: 7, Method: "GET", AcceptedCodes: "200-299"})
	s.AddSite(context.Background(), models.Site{Name: "Prod Web", URL: "https://prod.example.com", Type: "http", Interval: 15, ParentID: groupID, ExpiryThreshold: 7, Method: "GET", AcceptedCodes: "200-299"})
	s.AddSite(context.Background(), models.Site{Name: "Top Level", URL: "https://example.com", Type: "http", Interval: 30, ExpiryThreshold: 7, Method: "GET", AcceptedCodes: "200-299"})

	f, err := Export(context.Background(), s)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	if len(f.Monitors) != 2 {
		t.Fatalf("expected 2 top-level monitors, got %d", len(f.Monitors))
	}

	var group *Monitor
	for i := range f.Monitors {
		if f.Monitors[i].Type == "group" {
			group = &f.Monitors[i]
			break
		}
	}
	if group == nil {
		t.Fatal("group not found in export")
	}
	if len(group.Monitors) != 1 {
		t.Fatalf("expected 1 child in group, got %d", len(group.Monitors))
	}
	if group.Monitors[0].Name != "Prod Web" {
		t.Fatalf("expected child 'Prod Web', got %q", group.Monitors[0].Name)
	}
}

func TestExportOmitsDefaults(t *testing.T) {
	s := newTestStore(t)
	s.AddSite(context.Background(), models.Site{
		Name: "Web", URL: "https://example.com", Type: "http", Interval: 30,
		Method: "GET", AcceptedCodes: "200-299", ExpiryThreshold: 7,
	})

	f, err := Export(context.Background(), s)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	m := f.Monitors[0]
	if m.Method != "" {
		t.Errorf("expected empty method (default omitted), got %q", m.Method)
	}
	if m.AcceptedCodes != "" {
		t.Errorf("expected empty accepted_codes (default omitted), got %q", m.AcceptedCodes)
	}
	if m.ExpiryThreshold != 0 {
		t.Errorf("expected 0 expiry_threshold (default omitted), got %d", m.ExpiryThreshold)
	}
}

func TestExportRoundTrip(t *testing.T) {
	s1 := newTestStore(t)
	s1.AddAlert(context.Background(), "Discord", "discord", map[string]string{"url": "https://example.com"})
	alerts, _ := s1.GetAllAlerts(context.Background())
	s1.AddSite(context.Background(), models.Site{Name: "Web", URL: "https://example.com", Type: "http", Interval: 30, AlertID: alerts[0].ID, ExpiryThreshold: 7, Method: "GET", AcceptedCodes: "200-299"})
	s1.AddSite(context.Background(), models.Site{Name: "Ping", Type: "ping", Hostname: "10.0.0.1", Interval: 60, ExpiryThreshold: 7, Method: "GET", AcceptedCodes: "200-299"})

	exported, err := Export(context.Background(), s1)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	s2 := newTestStore(t)
	changes, err := Apply(context.Background(), s2, exported, ApplyOpts{})
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

	reexported, err := Export(context.Background(), s2)
	if err != nil {
		t.Fatalf("re-Export: %v", err)
	}

	if len(reexported.Alerts) != len(exported.Alerts) {
		t.Fatalf("alert count mismatch: %d vs %d", len(reexported.Alerts), len(exported.Alerts))
	}
	if len(reexported.Monitors) != len(exported.Monitors) {
		t.Fatalf("monitor count mismatch: %d vs %d", len(reexported.Monitors), len(exported.Monitors))
	}

	for i, m := range reexported.Monitors {
		if m.Name != exported.Monitors[i].Name {
			t.Errorf("monitor %d name mismatch: %q vs %q", i, m.Name, exported.Monitors[i].Name)
		}
	}
}
