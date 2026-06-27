package store

import (
	"context"
	"strings"
	"testing"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

func TestAlertCRUD(t *testing.T) {
	s := newTestStore(t)

	if err := s.AddAlert(context.Background(), "Discord", "discord", map[string]string{"url": "https://example.com/hook"}); err != nil {
		t.Fatalf("AddAlert: %v", err)
	}

	alerts, err := s.GetAllAlerts(context.Background())
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

	a, err := s.GetAlert(context.Background(), alerts[0].ID)
	if err != nil {
		t.Fatalf("GetAlert: %v", err)
	}
	if a.Name != "Discord" {
		t.Errorf("expected name 'Discord', got '%s'", a.Name)
	}

	if err := s.UpdateAlert(context.Background(), a.ID, "Slack", "slack", map[string]string{"url": "https://slack.com/hook"}); err != nil {
		t.Fatalf("UpdateAlert: %v", err)
	}

	a, err = s.GetAlert(context.Background(), a.ID)
	if err != nil {
		t.Fatalf("GetAlert: %v", err)
	}
	if a.Type != "slack" {
		t.Errorf("expected type 'slack', got '%s'", a.Type)
	}

	if err := s.DeleteAlert(context.Background(), a.ID); err != nil {
		t.Fatalf("DeleteAlert: %v", err)
	}

	alerts, err = s.GetAllAlerts(context.Background())
	if err != nil {
		t.Fatalf("GetAllAlerts: %v", err)
	}
	if len(alerts) != 0 {
		t.Fatalf("expected 0 alerts after delete, got %d", len(alerts))
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
	if err := s.ImportData(context.Background(), backup); err != nil {
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

	alerts, err := s.GetAllAlerts(context.Background())
	if err != nil {
		t.Fatalf("GetAllAlerts: %v", err)
	}
	if len(alerts) != 1 || alerts[0].Settings["token"] != "123:SECRET" {
		t.Errorf("decrypt round-trip failed: %+v", alerts)
	}
}
