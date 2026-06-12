package tui

import (
	"strings"
	"testing"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

func TestAlertDetailPanel_MasksSecretsStableOrder(t *testing.T) {
	m := newTestModel(&tuiMockStore{})
	m.termWidth, m.termHeight = 120, 40
	m.alerts = []models.AlertConfig{{
		ID: 1, Name: "ops", Type: "email",
		Settings: map[string]string{
			"host": "smtp.example.com",
			"port": "587",
			"user": "oncall@example.com",
			"pass": "hunter2-secret",
			"to":   "team@example.com",
		},
	}}
	m.cursor = 0

	out := m.viewAlertDetailPanel()
	if strings.Contains(out, "hunter2-secret") {
		t.Error("SMTP password rendered in alert detail panel")
	}
	if strings.Contains(out, "oncall@example.com") {
		t.Error("SMTP user (not on the allowlist) rendered in alert detail panel")
	}
	if !strings.Contains(out, "smtp.example.com") {
		t.Error("allowlisted setting (host) missing from panel")
	}

	// Map iteration must not reshuffle rows between renders.
	for i := 0; i < 5; i++ {
		if m.viewAlertDetailPanel() != out {
			t.Fatal("panel output unstable across renders — settings keys not sorted")
		}
	}
}

func TestFmtAlertConfig_MasksSecrets(t *testing.T) {
	m := newTestModel(&tuiMockStore{})

	webhook := m.fmtAlertConfig(models.AlertConfig{Type: "discord", Settings: map[string]string{"url": "https://discord.com/api/webhooks/123456/SeCrEtToKeN"}})
	if strings.Contains(webhook, "SeCrEtToKeN") || strings.Contains(webhook, "123456") {
		t.Errorf("webhook URL path (the credential) rendered in table: %q", webhook)
	}
	if !strings.Contains(webhook, "discord.com") {
		t.Errorf("webhook host missing from table config: %q", webhook)
	}

	pd := m.fmtAlertConfig(models.AlertConfig{Type: "pagerduty", Settings: map[string]string{"routing_key": "R0123456789ABCDEFGHIJ"}})
	if strings.Contains(pd, "R0123456789ABCDEFGHIJ") {
		t.Errorf("pagerduty routing key rendered raw in table: %q", pd)
	}
	if !strings.Contains(pd, "R012") || !strings.Contains(pd, "GHIJ") {
		t.Errorf("masked routing key should keep identifying ends: %q", pd)
	}
}
