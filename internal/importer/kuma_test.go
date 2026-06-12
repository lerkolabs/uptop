package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "backup.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadKumaFileMissingFile(t *testing.T) {
	_, err := LoadKumaFile(filepath.Join(t.TempDir(), "nope.json"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadKumaFileMalformedInput(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"empty file", ""},
		{"truncated JSON", `{"version": "1.23", "monitorList": [`},
		{"not JSON", "definitely not json"},
		{"wrong root type", `[1, 2, 3]`},
		{"monitorList wrong type", `{"monitorList": {"a": 1}}`},
		{"monitor field wrong type", `{"monitorList": [{"id": "not-an-int"}]}`},
		{"notificationList wrong type", `{"notificationList": "oops"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadKumaFile(writeTemp(t, tc.body))
			if err == nil {
				t.Fatalf("expected parse error for %s", tc.name)
			}
			if !strings.Contains(err.Error(), "parse JSON") {
				t.Fatalf("expected wrapped parse error, got: %v", err)
			}
		})
	}
}

func TestLoadKumaFileNullLists(t *testing.T) {
	kb, err := LoadKumaFile(writeTemp(t, `{"version": "1.23", "monitorList": null, "notificationList": null}`))
	if err != nil {
		t.Fatal(err)
	}
	backup := ConvertKuma(kb)
	if len(backup.Sites) != 0 || len(backup.Alerts) != 0 {
		t.Fatalf("expected empty backup, got %d sites %d alerts", len(backup.Sites), len(backup.Alerts))
	}
}

func TestConvertKumaSkipsMalformedNotificationConfig(t *testing.T) {
	kb := &KumaBackup{
		NotificationList: []KumaNotifEntry{
			{ID: 1, Name: "broken", Config: "{not json"},
			{ID: 2, Name: "good", Config: `{"type": "discord", "ntfyserverurl": "https://example.com/hook"}`},
		},
		MonitorList: []KumaMonitor{
			{ID: 10, Name: "site", Type: "http", URL: "https://example.com", NotificationIDs: map[string]bool{"1": true}},
		},
	}
	backup := ConvertKuma(kb)
	if len(backup.Alerts) != 1 {
		t.Fatalf("expected broken notification skipped, got %d alerts", len(backup.Alerts))
	}
	if backup.Alerts[0].Type != "discord" {
		t.Fatalf("expected discord alert, got %q", backup.Alerts[0].Type)
	}
	if backup.Sites[0].AlertID != 0 {
		t.Fatalf("site referencing skipped notification should keep AlertID 0, got %d", backup.Sites[0].AlertID)
	}
}

func TestConvertKumaNtfyNotification(t *testing.T) {
	kb := &KumaBackup{
		NotificationList: []KumaNotifEntry{
			{ID: 3, Name: "ntfy", Config: `{
				"type": "ntfy",
				"ntfyserverurl": "https://ntfy.example.com/",
				"ntfytopic": "uptime",
				"ntfyPriority": 4,
				"ntfyAuthenticationMethod": "usernamePassword",
				"ntfyusername": "u",
				"ntfypassword": "p"
			}`},
		},
	}
	backup := ConvertKuma(kb)
	if len(backup.Alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(backup.Alerts))
	}
	a := backup.Alerts[0]
	if a.Type != "ntfy" {
		t.Fatalf("expected ntfy, got %q", a.Type)
	}
	if a.Settings["url"] != "https://ntfy.example.com" {
		t.Fatalf("expected trailing slash trimmed, got %q", a.Settings["url"])
	}
	if a.Settings["topic"] != "uptime" || a.Settings["priority"] != "4" {
		t.Fatalf("unexpected settings: %v", a.Settings)
	}
	if a.Settings["username"] != "u" || a.Settings["password"] != "p" {
		t.Fatalf("expected credentials mapped, got %v", a.Settings)
	}
}

func TestConvertKumaUnknownNotificationFallsBackToWebhook(t *testing.T) {
	kb := &KumaBackup{
		NotificationList: []KumaNotifEntry{
			{ID: 4, Name: "matrix", Config: `{"type": "matrix", "ntfyserverurl": "https://example.com/hook"}`},
		},
	}
	backup := ConvertKuma(kb)
	if len(backup.Alerts) != 1 || backup.Alerts[0].Type != "webhook" {
		t.Fatalf("expected webhook fallback, got %+v", backup.Alerts)
	}
}

func TestConvertKumaHTTPMonitor(t *testing.T) {
	kb := &KumaBackup{
		NotificationList: []KumaNotifEntry{
			{ID: 1, Name: "hook", Config: `{"type": "slack", "ntfyserverurl": "https://example.com/hook"}`},
		},
		MonitorList: []KumaMonitor{{
			ID:              7,
			Name:            "web",
			Type:            "http",
			URL:             "https://example.com",
			Interval:        60,
			Timeout:         30,
			MaxRetries:      2,
			Method:          "GET",
			AcceptedCodes:   []string{"200", "301"},
			IgnoreTLS:       true,
			ExpiryNotif:     true,
			Active:          false,
			NotificationIDs: map[string]bool{"1": true},
		}},
	}
	backup := ConvertKuma(kb)
	if len(backup.Sites) != 1 {
		t.Fatalf("expected 1 site, got %d", len(backup.Sites))
	}
	s := backup.Sites[0]
	if s.URL != "https://example.com" || !s.CheckSSL || !s.IgnoreTLS {
		t.Fatalf("http fields not mapped: %+v", s)
	}
	if !s.Paused {
		t.Fatal("inactive monitor should import paused")
	}
	if s.AcceptedCodes != "200,301" {
		t.Fatalf("expected joined accepted codes, got %q", s.AcceptedCodes)
	}
	if s.AlertID != 1 {
		t.Fatalf("expected alert mapped, got %d", s.AlertID)
	}
}

func TestConvertKumaPushMonitorGetsToken(t *testing.T) {
	kb := &KumaBackup{
		MonitorList: []KumaMonitor{{ID: 1, Name: "push", Type: "push", Active: true}},
	}
	backup := ConvertKuma(kb)
	token := backup.Sites[0].Token
	if len(token) != 32 {
		t.Fatalf("expected 32-char hex token, got %q", token)
	}
}

func TestConvertKumaNonNumericNotificationID(t *testing.T) {
	kb := &KumaBackup{
		MonitorList: []KumaMonitor{{
			ID:              1,
			Name:            "site",
			Type:            "http",
			NotificationIDs: map[string]bool{"abc": true},
		}},
	}
	backup := ConvertKuma(kb)
	if backup.Sites[0].AlertID != 0 {
		t.Fatalf("non-numeric notification ID should not map, got %d", backup.Sites[0].AlertID)
	}
}

func TestConvertKumaGroupAndChildren(t *testing.T) {
	kb := &KumaBackup{
		MonitorList: []KumaMonitor{
			{ID: 1, Name: "grp", Type: "group", Active: true},
			{ID: 2, Name: "ping", Type: "ping", Hostname: "10.0.0.1", Parent: 1, Active: true},
		},
	}
	backup := ConvertKuma(kb)
	if backup.Sites[0].Type != "group" {
		t.Fatalf("expected group type, got %q", backup.Sites[0].Type)
	}
	if backup.Sites[1].ParentID != 1 || backup.Sites[1].Hostname != "10.0.0.1" {
		t.Fatalf("child not mapped: %+v", backup.Sites[1])
	}
}
