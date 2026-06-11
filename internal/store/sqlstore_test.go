package store

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

func newTestStore(t *testing.T) *SQLStore {
	t.Helper()
	s, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return s
}

func TestSiteCRUD(t *testing.T) {
	s := newTestStore(t)

	sites, err := s.GetSites()
	if err != nil {
		t.Fatalf("GetSites: %v", err)
	}
	if len(sites) != 0 {
		t.Fatalf("expected 0 sites, got %d", len(sites))
	}

	if err := s.AddSite(models.Site{Name: "Test", URL: "https://example.com", Type: "http", Interval: 30}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}

	sites, err = s.GetSites()
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
	if err := s.UpdateSite(sites[0]); err != nil {
		t.Fatalf("UpdateSite: %v", err)
	}

	sites, err = s.GetSites()
	if err != nil {
		t.Fatalf("GetSites: %v", err)
	}
	if sites[0].Name != "Updated" {
		t.Errorf("expected name 'Updated', got '%s'", sites[0].Name)
	}

	if err := s.DeleteSite(sites[0].ID); err != nil {
		t.Fatalf("DeleteSite: %v", err)
	}

	sites, err = s.GetSites()
	if err != nil {
		t.Fatalf("GetSites: %v", err)
	}
	if len(sites) != 0 {
		t.Fatalf("expected 0 sites after delete, got %d", len(sites))
	}
}

func TestAlertCRUD(t *testing.T) {
	s := newTestStore(t)

	if err := s.AddAlert("Discord", "discord", map[string]string{"url": "https://example.com/hook"}); err != nil {
		t.Fatalf("AddAlert: %v", err)
	}

	alerts, err := s.GetAllAlerts()
	if err != nil {
		t.Fatalf("GetAllAlerts: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Type != "discord" {
		t.Errorf("expected type 'discord', got '%s'", alerts[0].Type)
	}
	if alerts[0].Settings["url"] != "https://example.com/hook" {
		t.Errorf("settings url mismatch")
	}

	a, err := s.GetAlert(alerts[0].ID)
	if err != nil {
		t.Fatalf("GetAlert: %v", err)
	}
	if a.Name != "Discord" {
		t.Errorf("expected name 'Discord', got '%s'", a.Name)
	}

	if err := s.UpdateAlert(a.ID, "Slack", "slack", map[string]string{"url": "https://slack.com/hook"}); err != nil {
		t.Fatalf("UpdateAlert: %v", err)
	}

	a, err = s.GetAlert(a.ID)
	if err != nil {
		t.Fatalf("GetAlert: %v", err)
	}
	if a.Type != "slack" {
		t.Errorf("expected type 'slack', got '%s'", a.Type)
	}

	if err := s.DeleteAlert(a.ID); err != nil {
		t.Fatalf("DeleteAlert: %v", err)
	}

	alerts, err = s.GetAllAlerts()
	if err != nil {
		t.Fatalf("GetAllAlerts: %v", err)
	}
	if len(alerts) != 0 {
		t.Fatalf("expected 0 alerts after delete, got %d", len(alerts))
	}
}

func TestUserCRUD(t *testing.T) {
	s := newTestStore(t)

	if err := s.AddUser("admin", "ssh-ed25519 AAAA...", "admin"); err != nil {
		t.Fatalf("AddUser: %v", err)
	}

	users, err := s.GetAllUsers()
	if err != nil {
		t.Fatalf("GetAllUsers: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if users[0].Username != "admin" {
		t.Errorf("expected username 'admin', got '%s'", users[0].Username)
	}

	if err := s.UpdateUser(users[0].ID, "root", "ssh-ed25519 BBBB...", "admin"); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	users, err = s.GetAllUsers()
	if err != nil {
		t.Fatalf("GetAllUsers: %v", err)
	}
	if users[0].Username != "root" {
		t.Errorf("expected username 'root', got '%s'", users[0].Username)
	}

	if err := s.DeleteUser(users[0].ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}

	users, err = s.GetAllUsers()
	if err != nil {
		t.Fatalf("GetAllUsers: %v", err)
	}
	if len(users) != 0 {
		t.Fatalf("expected 0 users after delete, got %d", len(users))
	}
}

func TestPushTokenGeneration(t *testing.T) {
	s := newTestStore(t)

	if err := s.AddSite(models.Site{Name: "Push Monitor", Type: "push", Interval: 60}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}

	sites, err := s.GetSites()
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

	if err := s.AddAlert("Test Alert", "webhook", map[string]string{"url": "https://example.com"}); err != nil {
		t.Fatalf("AddAlert: %v", err)
	}
	if err := s.AddSite(models.Site{Name: "Site1", URL: "https://example.com", Type: "http", Interval: 30}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}
	if err := s.AddUser("user1", "ssh-ed25519 KEY", "user"); err != nil {
		t.Fatalf("AddUser: %v", err)
	}

	backup, err := s.ExportData()
	if err != nil {
		t.Fatalf("ExportData: %v", err)
	}
	if len(backup.Sites) != 1 || len(backup.Alerts) != 1 || len(backup.Users) != 1 {
		t.Fatalf("export mismatch: %d sites, %d alerts, %d users", len(backup.Sites), len(backup.Alerts), len(backup.Users))
	}

	s2 := newTestStore(t)
	if err := s2.ImportData(backup); err != nil {
		t.Fatalf("ImportData: %v", err)
	}

	sites, err := s2.GetSites()
	if err != nil {
		t.Fatalf("GetSites: %v", err)
	}
	alerts, err := s2.GetAllAlerts()
	if err != nil {
		t.Fatalf("GetAllAlerts: %v", err)
	}
	users, err := s2.GetAllUsers()
	if err != nil {
		t.Fatalf("GetAllUsers: %v", err)
	}
	if len(sites) != 1 || len(alerts) != 1 || len(users) != 1 {
		t.Fatalf("import mismatch: %d sites, %d alerts, %d users", len(sites), len(alerts), len(users))
	}
}

func TestImportData_WipesHistory(t *testing.T) {
	s := newTestStore(t)

	if err := s.AddSite(models.Site{Name: "OldSite", URL: "https://old.com", Type: "http", Interval: 30}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}
	if err := s.SaveCheck(1, 5000, true); err != nil {
		t.Fatalf("SaveCheck: %v", err)
	}
	if err := s.SaveStateChange(1, "UP", "DOWN", "timeout"); err != nil {
		t.Fatalf("SaveStateChange: %v", err)
	}
	if err := s.SaveAlertHealth(models.AlertHealthRecord{AlertID: 1, LastSendOK: true, SendCount: 1}); err != nil {
		t.Fatalf("SaveAlertHealth: %v", err)
	}

	backup := models.Backup{
		Sites: []models.Site{{ID: 1, Name: "NewSite", URL: "https://new.com", Type: "http", Interval: 60}},
	}
	if err := s.ImportData(backup); err != nil {
		t.Fatalf("ImportData: %v", err)
	}

	history, err := s.LoadAllHistory(100)
	if err != nil {
		t.Fatalf("LoadAllHistory: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("expected empty check_history after import, got %d sites with history", len(history))
	}

	changes, err := s.GetStateChanges(1, 100)
	if err != nil {
		t.Fatalf("GetStateChanges: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected empty state_changes after import, got %d", len(changes))
	}
}

func TestCheckHistory(t *testing.T) {
	s := newTestStore(t)

	if err := s.SaveCheck(1, 5000000, true); err != nil {
		t.Fatalf("SaveCheck: %v", err)
	}
	if err := s.SaveCheck(1, 10000000, false); err != nil {
		t.Fatalf("SaveCheck: %v", err)
	}
	if err := s.SaveCheck(2, 3000000, true); err != nil {
		t.Fatalf("SaveCheck site 2: %v", err)
	}

	history, err := s.LoadAllHistory(10)
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

func TestDeleteSiteCascade(t *testing.T) {
	s := newTestStore(t)

	site := models.Site{Name: "Cascade Test", URL: "https://example.com", Interval: 30}
	if err := s.AddSite(site); err != nil {
		t.Fatalf("AddSite: %v", err)
	}
	sites, _ := s.GetSites()
	siteID := sites[0].ID

	if err := s.SaveCheck(siteID, 1000, true); err != nil {
		t.Fatalf("SaveCheck: %v", err)
	}
	if err := s.SaveStateChange(siteID, "UP", "DOWN", "timeout"); err != nil {
		t.Fatalf("SaveStateChange: %v", err)
	}
	mw := models.MaintenanceWindow{
		MonitorID: siteID,
		Title:     "Test MW",
		Type:      "maintenance",
		StartTime: time.Now(),
	}
	if err := s.AddMaintenanceWindow(mw); err != nil {
		t.Fatalf("AddMaintenanceWindow: %v", err)
	}

	if err := s.DeleteSite(siteID); err != nil {
		t.Fatalf("DeleteSite: %v", err)
	}

	history, _ := s.LoadAllHistory(100)
	if len(history[siteID]) != 0 {
		t.Errorf("expected 0 check_history rows, got %d", len(history[siteID]))
	}

	changes, _ := s.GetStateChanges(siteID, 100)
	if len(changes) != 0 {
		t.Errorf("expected 0 state_changes rows, got %d", len(changes))
	}

	windows, _ := s.GetActiveMaintenanceWindows()
	for _, w := range windows {
		if w.MonitorID == siteID {
			t.Errorf("orphaned maintenance window found: id=%d", w.ID)
		}
	}
}

func TestPruneLogs(t *testing.T) {
	s := newTestStore(t)

	for i := 0; i < maxLogRows+50; i++ {
		if err := s.SaveLog(fmt.Sprintf("log %d", i)); err != nil {
			t.Fatalf("SaveLog: %v", err)
		}
	}
	if err := s.PruneLogs(); err != nil {
		t.Fatalf("PruneLogs: %v", err)
	}

	logs, err := s.LoadLogs(maxLogRows * 2)
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
		present[l] = true
	}
	if !present[fmt.Sprintf("log %d", maxLogRows+50-1)] {
		t.Error("newest log was pruned")
	}
	if present["log 0"] {
		t.Error("oldest log survived prune")
	}
}

func TestPruneCheckHistory(t *testing.T) {
	s := newTestStore(t)

	for i := 0; i < maxCheckHistory+5; i++ {
		if err := s.SaveCheck(1, int64(i), true); err != nil {
			t.Fatalf("SaveCheck site 1: %v", err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := s.SaveCheck(2, int64(i), true); err != nil {
			t.Fatalf("SaveCheck site 2: %v", err)
		}
	}

	if err := s.PruneCheckHistory(); err != nil {
		t.Fatalf("PruneCheckHistory: %v", err)
	}

	history, err := s.LoadAllHistory(maxCheckHistory * 2)
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
	if err := s.AddMaintenanceWindow(old); err != nil {
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
	if err := s.AddMaintenanceWindow(recent); err != nil {
		t.Fatalf("AddMaintenanceWindow (recent): %v", err)
	}

	// Ongoing — no end time, should survive.
	ongoing := models.MaintenanceWindow{
		MonitorID: 0,
		Title:     "Ongoing Window",
		Type:      "maintenance",
		StartTime: now.Add(-1 * time.Hour),
	}
	if err := s.AddMaintenanceWindow(ongoing); err != nil {
		t.Fatalf("AddMaintenanceWindow (ongoing): %v", err)
	}

	pruned, err := s.PruneExpiredMaintenanceWindows(7 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("PruneExpiredMaintenanceWindows: %v", err)
	}
	if pruned != 1 {
		t.Errorf("expected 1 pruned, got %d", pruned)
	}

	all, err := s.GetAllMaintenanceWindows(100)
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

// ImportData must encrypt alert settings (like AddAlert/UpdateAlert) so a
// restore with UPTOP_ENCRYPTION_KEY set never lands secrets in plaintext.
func TestImportData_EncryptsAlertSettings(t *testing.T) {
	s := newTestStore(t)
	enc, err := NewEncryptor(strings.Repeat("ab", 32)) // 64 hex chars = 32 bytes
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	s.SetEncryptor(enc)

	backup := models.Backup{
		Alerts: []models.AlertConfig{
			{ID: 1, Name: "tg", Type: "telegram", Settings: map[string]string{"token": "123:SECRET", "chat_id": "42"}},
		},
	}
	if err := s.ImportData(backup); err != nil {
		t.Fatalf("ImportData: %v", err)
	}

	var raw string
	if err := s.db.QueryRow("SELECT settings FROM alerts WHERE id = 1").Scan(&raw); err != nil {
		t.Fatalf("query settings: %v", err)
	}
	if !strings.HasPrefix(raw, encryptedPrefix) {
		t.Errorf("imported settings not encrypted: %q", raw)
	}
	if strings.Contains(raw, "SECRET") {
		t.Errorf("plaintext secret found in stored column: %q", raw)
	}

	alerts, err := s.GetAllAlerts()
	if err != nil {
		t.Fatalf("GetAllAlerts: %v", err)
	}
	if len(alerts) != 1 || alerts[0].Settings["token"] != "123:SECRET" {
		t.Errorf("decrypt round-trip failed: %+v", alerts)
	}
}
