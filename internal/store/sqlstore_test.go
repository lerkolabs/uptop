package store

import (
	"context"
	"fmt"
	"testing"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

func newTestStore(t *testing.T) *SQLStore {
	t.Helper()
	s, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	if err := s.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return s
}

func TestSiteCRUD(t *testing.T) {
	s := newTestStore(t)

	sites, err := s.GetSites(context.Background())
	if err != nil {
		t.Fatalf("GetSites: %v", err)
	}
	if len(sites) != 0 {
		t.Fatalf("expected 0 sites, got %d", len(sites))
	}

	if err := s.AddSite(context.Background(), models.SiteConfig{Name: "Test", URL: "https://example.com", Type: "http", Interval: 30}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}

	sites, err = s.GetSites(context.Background())
	if err != nil {
		t.Fatalf("GetSites: %v", err)
	}
	if len(sites) != 1 {
		t.Fatalf("expected 1 site, got %d", len(sites))
	}
	if sites[0].Name != "Test" {
		t.Errorf("expected name 'Test', got '%s'", sites[0].Name)
	}

	sites[0].Name = "Updated"
	if err := s.UpdateSite(context.Background(), sites[0]); err != nil {
		t.Fatalf("UpdateSite: %v", err)
	}

	sites, err = s.GetSites(context.Background())
	if err != nil {
		t.Fatalf("GetSites: %v", err)
	}
	if sites[0].Name != "Updated" {
		t.Errorf("expected name 'Updated', got '%s'", sites[0].Name)
	}

	if err := s.DeleteSite(context.Background(), sites[0].ID); err != nil {
		t.Fatalf("DeleteSite: %v", err)
	}

	sites, err = s.GetSites(context.Background())
	if err != nil {
		t.Fatalf("GetSites: %v", err)
	}
	if len(sites) != 0 {
		t.Fatalf("expected 0 sites after delete, got %d", len(sites))
	}
}

func TestUserCRUD(t *testing.T) {
	s := newTestStore(t)

	if err := s.AddUser(context.Background(), "admin", "ssh-ed25519 AAAA...", "admin"); err != nil {
		t.Fatalf("AddUser: %v", err)
	}

	users, err := s.GetAllUsers(context.Background())
	if err != nil {
		t.Fatalf("GetAllUsers: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if users[0].Username != "admin" {
		t.Errorf("expected username 'admin', got '%s'", users[0].Username)
	}

	if err := s.UpdateUser(context.Background(), users[0].ID, "root", "ssh-ed25519 BBBB...", "admin"); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	users, err = s.GetAllUsers(context.Background())
	if err != nil {
		t.Fatalf("GetAllUsers: %v", err)
	}
	if users[0].Username != "root" {
		t.Errorf("expected username 'root', got '%s'", users[0].Username)
	}

	if err := s.DeleteUser(context.Background(), users[0].ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}

	users, err = s.GetAllUsers(context.Background())
	if err != nil {
		t.Fatalf("GetAllUsers: %v", err)
	}
	if len(users) != 0 {
		t.Fatalf("expected 0 users after delete, got %d", len(users))
	}
}

func TestPushTokenGeneration(t *testing.T) {
	s := newTestStore(t)

	if err := s.AddSite(context.Background(), models.SiteConfig{Name: "Push Monitor", Type: "push", Interval: 60}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}

	sites, err := s.GetSites(context.Background())
	if err != nil {
		t.Fatalf("GetSites: %v", err)
	}
	if len(sites) != 1 {
		t.Fatalf("expected 1 site, got %d", len(sites))
	}
	if sites[0].Token == "" {
		t.Error("expected non-empty token for push monitor")
	}
	if len(sites[0].Token) != 32 {
		t.Errorf("expected 32-char hex token, got %d chars", len(sites[0].Token))
	}
}

func TestImportExport(t *testing.T) {
	s := newTestStore(t)

	if err := s.AddAlert(context.Background(), "Test Alert", "webhook", map[string]string{"url": "https://example.com"}); err != nil {
		t.Fatalf("AddAlert: %v", err)
	}
	if err := s.AddSite(context.Background(), models.SiteConfig{Name: "Site1", URL: "https://example.com", Type: "http", Interval: 30}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}
	if err := s.AddUser(context.Background(), "user1", "ssh-ed25519 KEY", "user"); err != nil {
		t.Fatalf("AddUser: %v", err)
	}

	backup, err := s.ExportData(context.Background())
	if err != nil {
		t.Fatalf("ExportData: %v", err)
	}
	if len(backup.Sites) != 1 || len(backup.Alerts) != 1 || len(backup.Users) != 1 {
		t.Fatalf("export mismatch: %d sites, %d alerts, %d users", len(backup.Sites), len(backup.Alerts), len(backup.Users))
	}

	s2 := newTestStore(t)
	if err := s2.ImportData(context.Background(), backup); err != nil {
		t.Fatalf("ImportData: %v", err)
	}

	sites, err := s2.GetSites(context.Background())
	if err != nil {
		t.Fatalf("GetSites: %v", err)
	}
	alerts, err := s2.GetAllAlerts(context.Background())
	if err != nil {
		t.Fatalf("GetAllAlerts: %v", err)
	}
	users, err := s2.GetAllUsers(context.Background())
	if err != nil {
		t.Fatalf("GetAllUsers: %v", err)
	}
	if len(sites) != 1 || len(alerts) != 1 || len(users) != 1 {
		t.Fatalf("import mismatch: %d sites, %d alerts, %d users", len(sites), len(alerts), len(users))
	}
}

func TestImportData_WipesHistory(t *testing.T) {
	s := newTestStore(t)

	if err := s.AddSite(context.Background(), models.SiteConfig{Name: "OldSite", URL: "https://old.com", Type: "http", Interval: 30}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}
	if err := s.SaveCheck(context.Background(), 1, 5000, true); err != nil {
		t.Fatalf("SaveCheck: %v", err)
	}
	if err := s.SaveStateChange(context.Background(), 1, "UP", "DOWN", "timeout"); err != nil {
		t.Fatalf("SaveStateChange: %v", err)
	}
	if err := s.SaveAlertHealth(context.Background(), models.AlertHealthRecord{AlertID: 1, LastSendOK: true, SendCount: 1}); err != nil {
		t.Fatalf("SaveAlertHealth: %v", err)
	}

	backup := models.Backup{
		Sites: []models.SiteConfig{{ID: 1, Name: "NewSite", URL: "https://new.com", Type: "http", Interval: 60}},
	}
	if err := s.ImportData(context.Background(), backup); err != nil {
		t.Fatalf("ImportData: %v", err)
	}

	history, err := s.LoadAllHistory(context.Background(), 100)
	if err != nil {
		t.Fatalf("LoadAllHistory: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("expected empty check_history after import, got %d sites with history", len(history))
	}

	changes, err := s.GetStateChanges(context.Background(), 1, 100)
	if err != nil {
		t.Fatalf("GetStateChanges: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected empty state_changes after import, got %d", len(changes))
	}
}

func TestImportData_NilUsersPreservesExisting(t *testing.T) {
	s := newTestStore(t)

	if err := s.AddUser(context.Background(), "admin", "ssh-ed25519 ADMINKEY", "admin"); err != nil {
		t.Fatalf("AddUser: %v", err)
	}

	backup := models.Backup{
		Sites:  []models.SiteConfig{{ID: 1, Name: "New", URL: "https://new.com", Type: "http", Interval: 30}},
		Alerts: []models.AlertConfig{{ID: 1, Name: "a", Type: "webhook", Settings: map[string]string{"url": "https://h.com"}}},
		Users:  nil,
	}
	if err := s.ImportData(context.Background(), backup); err != nil {
		t.Fatalf("ImportData: %v", err)
	}

	users, err := s.GetAllUsers(context.Background())
	if err != nil {
		t.Fatalf("GetAllUsers: %v", err)
	}
	if len(users) != 1 || users[0].Username != "admin" {
		t.Errorf("expected existing admin user preserved, got %d users", len(users))
	}
}

func TestPruneLogs(t *testing.T) {
	s := newTestStore(t)

	for i := 0; i < maxLogRows+50; i++ {
		if err := s.SaveLog(context.Background(), fmt.Sprintf("log %d", i)); err != nil {
			t.Fatalf("SaveLog: %v", err)
		}
	}
	if err := s.PruneLogs(context.Background()); err != nil {
		t.Fatalf("PruneLogs: %v", err)
	}

	logs, err := s.LoadLogs(context.Background(), maxLogRows*2)
	if err != nil {
		t.Fatalf("LoadLogs: %v", err)
	}
	if len(logs) != maxLogRows {
		t.Errorf("expected %d logs after prune, got %d", maxLogRows, len(logs))
	}
	// Newest must survive; oldest must be gone (membership, not position —
	// LoadLogs ordering ties when rows share a created_at second).
	present := make(map[string]bool, len(logs))
	for _, l := range logs {
		present[l.Message] = true
	}
	if !present[fmt.Sprintf("log %d", maxLogRows+50-1)] {
		t.Error("newest log was pruned")
	}
	if present["log 0"] {
		t.Error("oldest log survived prune")
	}
}
