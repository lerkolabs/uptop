package monitor

import (
	"context"
	"testing"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

// --- Group 1: State Machine ---

func TestHandleStatusChange_PendingToUp(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "test", MaxRetries: 3, AlertID: 1},
		SiteState:  models.SiteState{Status: "PENDING"},
	}
	injectSite(e, site)

	e.handleStatusChange(site, "UP", 200, 10*time.Millisecond, "")

	s, _ := getSite(e, 1)
	if s.Status != "UP" {
		t.Errorf("expected UP, got %s", s.Status)
	}
	if s.FailureCount != 0 {
		t.Errorf("expected FailureCount 0, got %d", s.FailureCount)
	}
	waitAsync()
	if len(ms.getAlertCallsSnapshot()) != 0 {
		t.Error("expected no alert for PENDING→UP")
	}
}

func TestHandleStatusChange_UpIncrementFailure(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "test", MaxRetries: 3},
		SiteState:  models.SiteState{Status: "UP", FailureCount: 0},
	}
	injectSite(e, site)

	e.handleStatusChange(site, "DOWN", 500, 0, "test error")

	s, _ := getSite(e, 1)
	if s.Status != "UP" {
		t.Errorf("expected UP (under retry threshold), got %s", s.Status)
	}
	if s.FailureCount != 1 {
		t.Errorf("expected FailureCount 1, got %d", s.FailureCount)
	}
}

func TestHandleStatusChange_UpToDown_ExceedsRetries(t *testing.T) {
	ms := newMockStore()
	ms.alerts[1] = models.AlertConfig{ID: 1, Name: "discord", Type: "webhook", Settings: map[string]string{"url": "http://example.com"}}
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "test", MaxRetries: 2, AlertID: 1},
		SiteState:  models.SiteState{Status: "UP", FailureCount: 2},
	}
	injectSite(e, site)

	e.handleStatusChange(site, "DOWN", 500, 0, "test error")

	s, _ := getSite(e, 1)
	if s.Status != "DOWN" {
		t.Errorf("expected DOWN, got %s", s.Status)
	}
	if s.FailureCount != 3 {
		t.Errorf("expected FailureCount 3, got %d", s.FailureCount)
	}
	waitAsync()
	calls := ms.getAlertCallsSnapshot()
	if len(calls) == 0 || calls[0] != 1 {
		t.Errorf("expected alert call for alertID 1, got %v", calls)
	}
}

func TestHandleStatusChange_UpToDown_ZeroRetries(t *testing.T) {
	ms := newMockStore()
	ms.alerts[1] = models.AlertConfig{ID: 1, Name: "test", Type: "webhook", Settings: map[string]string{"url": "http://example.com"}}
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "test", MaxRetries: 0, AlertID: 1},
		SiteState:  models.SiteState{Status: "UP", FailureCount: 0},
	}
	injectSite(e, site)

	e.handleStatusChange(site, "DOWN", 0, 0, "test error")

	s, _ := getSite(e, 1)
	if s.Status != "DOWN" {
		t.Errorf("expected DOWN, got %s", s.Status)
	}
	waitAsync()
	if len(ms.getAlertCallsSnapshot()) == 0 {
		t.Error("expected alert on immediate DOWN")
	}
}

func TestHandleStatusChange_DownToUp_Recovery(t *testing.T) {
	ms := newMockStore()
	ms.alerts[1] = models.AlertConfig{ID: 1, Name: "test", Type: "webhook", Settings: map[string]string{"url": "http://example.com"}}
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "test", AlertID: 1},
		SiteState:  models.SiteState{Status: "DOWN", FailureCount: 4},
	}
	injectSite(e, site)

	e.handleStatusChange(site, "UP", 200, 5*time.Millisecond, "")

	s, _ := getSite(e, 1)
	if s.Status != "UP" {
		t.Errorf("expected UP, got %s", s.Status)
	}
	if s.FailureCount != 0 {
		t.Errorf("expected FailureCount 0, got %d", s.FailureCount)
	}
	waitAsync()
	if len(ms.getAlertCallsSnapshot()) == 0 {
		t.Error("expected recovery alert")
	}
}

func TestHandleStatusChange_DownStaysDown(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "test", MaxRetries: 2},
		SiteState:  models.SiteState{Status: "DOWN", FailureCount: 3},
	}
	injectSite(e, site)

	e.handleStatusChange(site, "DOWN", 0, 0, "test error")

	s, _ := getSite(e, 1)
	if s.Status != "DOWN" {
		t.Errorf("expected DOWN, got %s", s.Status)
	}
	waitAsync()
	if len(ms.getAlertCallsSnapshot()) != 0 {
		t.Error("expected no re-alert for already DOWN")
	}
}

func TestHandleStatusChange_SSLExpired(t *testing.T) {
	ms := newMockStore()
	ms.alerts[1] = models.AlertConfig{ID: 1, Name: "test", Type: "webhook", Settings: map[string]string{"url": "http://example.com"}}
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "test", MaxRetries: 0, AlertID: 1},
		SiteState:  models.SiteState{Status: "UP"},
	}
	injectSite(e, site)

	e.handleStatusChange(site, "SSL EXP", 0, 0, "SSL certificate expired")

	s, _ := getSite(e, 1)
	if s.Status != "SSL EXP" {
		t.Errorf("expected SSL EXP, got %s", s.Status)
	}
	waitAsync()
	if len(ms.getAlertCallsSnapshot()) == 0 {
		t.Error("expected alert on SSL EXP")
	}
}

func TestHandleStatusChange_AlertSuppressedMaintenance(t *testing.T) {
	ms := newMockStore()
	ms.maintenance[1] = true
	ms.alerts[1] = models.AlertConfig{ID: 1, Name: "test", Type: "webhook", Settings: map[string]string{"url": "http://example.com"}}
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "test", MaxRetries: 0, AlertID: 1},
		SiteState:  models.SiteState{Status: "UP"},
	}
	injectSite(e, site)
	e.refreshMaintenanceCache(context.Background())

	e.handleStatusChange(site, "DOWN", 0, 0, "test error")

	s, _ := getSite(e, 1)
	if s.Status != "DOWN" {
		t.Errorf("expected DOWN, got %s", s.Status)
	}
	waitAsync()
	if len(ms.getAlertCallsSnapshot()) != 0 {
		t.Error("expected no alert during maintenance")
	}
	logs := e.GetLogs()
	found := false
	for _, l := range logs {
		if containsStr(l.Message, "suppressed") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected log mentioning suppressed")
	}
}

func TestHandleStatusChange_RecoverySuppressedMaintenance(t *testing.T) {
	ms := newMockStore()
	ms.maintenance[1] = true
	ms.alerts[1] = models.AlertConfig{ID: 1, Name: "test", Type: "webhook", Settings: map[string]string{"url": "http://example.com"}}
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "test", AlertID: 1},
		SiteState:  models.SiteState{Status: "DOWN"},
	}
	injectSite(e, site)
	e.refreshMaintenanceCache(context.Background())

	e.handleStatusChange(site, "UP", 200, 0, "")

	s, _ := getSite(e, 1)
	if s.Status != "UP" {
		t.Errorf("expected UP, got %s", s.Status)
	}
	waitAsync()
	if len(ms.getAlertCallsSnapshot()) != 0 {
		t.Error("expected no alert during maintenance recovery")
	}
}

func TestHandleStatusChange_SSLWarning(t *testing.T) {
	ms := newMockStore()
	ms.alerts[1] = models.AlertConfig{ID: 1, Name: "test", Type: "webhook", Settings: map[string]string{"url": "http://example.com"}}
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "test", Type: "http", CheckSSL: true, ExpiryThreshold: 30, AlertID: 1},
		SiteState:  models.SiteState{Status: "UP", HasSSL: true, SentSSLWarning: false, CertExpiry: time.Now().Add(15 * 24 * time.Hour)},
	}
	injectSite(e, site)

	e.handleStatusChange(site, "UP", 200, 0, "")

	s, _ := getSite(e, 1)
	if !s.SentSSLWarning {
		t.Error("expected SentSSLWarning=true")
	}
	waitAsync()
	if len(ms.getAlertCallsSnapshot()) == 0 {
		t.Error("expected SSL warning alert")
	}
}

func TestHandleStatusChange_SSLWarningNotRepeated(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "test", Type: "http", CheckSSL: true, ExpiryThreshold: 30, AlertID: 1},
		SiteState:  models.SiteState{Status: "UP", HasSSL: true, SentSSLWarning: true, CertExpiry: time.Now().Add(15 * 24 * time.Hour)},
	}
	injectSite(e, site)

	e.handleStatusChange(site, "UP", 200, 0, "")

	waitAsync()
	if len(ms.getAlertCallsSnapshot()) != 0 {
		t.Error("expected no repeat SSL warning")
	}
}

func TestHandleStatusChange_SSLWarningReset(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "test", Type: "http", CheckSSL: true, ExpiryThreshold: 30},
		SiteState:  models.SiteState{Status: "UP", HasSSL: true, SentSSLWarning: true, CertExpiry: time.Now().Add(60 * 24 * time.Hour)},
	}
	injectSite(e, site)

	e.handleStatusChange(site, "UP", 200, 0, "")

	s, _ := getSite(e, 1)
	if s.SentSSLWarning {
		t.Error("expected SentSSLWarning reset to false")
	}
}

func TestHandleStatusChange_SSLWarningSuppressedMaint(t *testing.T) {
	ms := newMockStore()
	ms.maintenance[1] = true
	ms.alerts[1] = models.AlertConfig{ID: 1, Name: "test", Type: "webhook", Settings: map[string]string{"url": "http://example.com"}}
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "test", Type: "http", CheckSSL: true, ExpiryThreshold: 30, AlertID: 1},
		SiteState:  models.SiteState{Status: "UP", HasSSL: true, SentSSLWarning: false, CertExpiry: time.Now().Add(15 * 24 * time.Hour)},
	}
	injectSite(e, site)
	e.refreshMaintenanceCache(context.Background())

	e.handleStatusChange(site, "UP", 200, 0, "")

	s, _ := getSite(e, 1)
	if !s.SentSSLWarning {
		t.Error("expected SentSSLWarning=true even in maintenance")
	}
	waitAsync()
	if len(ms.getAlertCallsSnapshot()) != 0 {
		t.Error("expected no alert during maintenance")
	}
}

func TestHandleStatusChange_InactiveEngine(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "test", MaxRetries: 0},
		SiteState:  models.SiteState{Status: "UP"},
	}
	injectSite(e, site)
	e.SetActive(false)

	e.handleStatusChange(site, "DOWN", 0, 0, "test error")

	s, _ := getSite(e, 1)
	if s.Status != "UP" {
		t.Error("expected no state change when inactive")
	}
}

// --- Group 10: liveState merge (lost-update race) ---

// A pause that lands while a check is in flight must survive the check's
// write-back. The old code snapshotted the site, ran the check, then wrote the
// whole stale struct back — reverting the pause.
func TestHandleStatusChange_PauseDuringCheckSurvives(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "test", MaxRetries: 0},
		SiteState:  models.SiteState{Status: "UP"},
	}
	injectSite(e, site)

	// `site` is the stale snapshot the check ran against (Paused=false).
	// Meanwhile the user pauses the monitor.
	e.ToggleSitePause(1)

	// Check completes and folds its result in using the stale snapshot.
	e.handleStatusChange(site, "DOWN", 500, 0, "boom")

	s, _ := getSite(e, 1)
	if !s.Paused {
		t.Error("pause was reverted by a stale check write-back")
	}
	if s.Status != "DOWN" {
		t.Errorf("expected check result still applied (DOWN), got %s", s.Status)
	}
}

// A config edit that lands while a check is in flight must survive; the check
// must not resurrect the old config from its snapshot.
func TestHandleStatusChange_ConfigEditDuringCheckSurvives(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "test", URL: "http://old.com", Type: "http", MaxRetries: 0, Interval: 30},
		SiteState:  models.SiteState{Status: "UP"},
	}
	injectSite(e, site)

	// Config changes mid-check.
	e.UpdateSiteConfig(models.SiteConfig{ID: 1, Name: "test", URL: "http://new.com", Type: "http", Interval: 60})

	// Stale check (ran against http://old.com) folds its result in.
	e.handleStatusChange(site, "UP", 200, 5*time.Millisecond, "")

	s, _ := getSite(e, 1)
	if s.URL != "http://new.com" {
		t.Errorf("config edit reverted: URL=%s", s.URL)
	}
	if s.Interval != 60 {
		t.Errorf("config edit reverted: Interval=%d", s.Interval)
	}
}

// The classic push false-DOWN: a heartbeat marks the monitor UP while a
// staleness evaluation (computed from the older LastCheck) is mid-flight.
// The stale DOWN must not overwrite the fresh heartbeat.
func TestHandleStatusChange_HeartbeatNotOverwrittenByStaleDown(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	// Snapshot the engine would have taken before evaluating staleness:
	// LastCheck is old, so checkPush decided "DOWN".
	snap := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "push", Type: "push", Token: "tok", Interval: 10},
		SiteState:  models.SiteState{Status: "UP", LastCheck: time.Now().Add(-120 * time.Second)},
	}
	injectSite(e, snap)

	// A heartbeat lands first, advancing LastCheck and confirming UP.
	if !e.RecordHeartbeat("tok") {
		t.Fatal("heartbeat rejected")
	}

	// Now the in-flight stale evaluation tries to write DOWN.
	e.handleStatusChange(snap, "DOWN", 0, 0, "heartbeat missed")

	s, _ := getSite(e, 1)
	if s.Status != "UP" {
		t.Errorf("stale DOWN overwrote a fresh heartbeat: status=%s", s.Status)
	}
}

// A check result for a site removed mid-check must be dropped, not recreate it.
func TestHandleStatusChange_RemovedSiteDropped(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "test", MaxRetries: 0},
		SiteState:  models.SiteState{Status: "UP"},
	}
	injectSite(e, site)

	e.RemoveSite(1)
	e.handleStatusChange(site, "DOWN", 500, 0, "boom")

	if _, ok := getSite(e, 1); ok {
		t.Error("removed site was recreated by a late check write-back")
	}
}

// --- Group 12: Phase 3 engine correctness ---

// PENDING→DOWN must honor MaxRetries instead of alerting on first failure.
func TestHandleStatusChange_PendingRetriesBeforeDown(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "new-monitor", MaxRetries: 2},
		SiteState:  models.SiteState{Status: "PENDING"},
	}
	injectSite(e, site)

	e.handleStatusChange(site, "DOWN", 0, 0, "timeout")
	s, _ := getSite(e, 1)
	if s.Status != "PENDING" {
		t.Errorf("expected PENDING during retry, got %s", s.Status)
	}
	if s.FailureCount != 1 {
		t.Errorf("expected FailureCount 1, got %d", s.FailureCount)
	}

	e.handleStatusChange(s, "DOWN", 0, 0, "timeout")
	s, _ = getSite(e, 1)
	if s.Status != "PENDING" {
		t.Errorf("expected PENDING during retry 2, got %s", s.Status)
	}

	e.handleStatusChange(s, "DOWN", 0, 0, "timeout")
	s, _ = getSite(e, 1)
	if s.Status != "DOWN" {
		t.Errorf("expected DOWN after retries exhausted, got %s", s.Status)
	}
}

// LATE→DOWN must also honor MaxRetries.
func TestHandleStatusChange_LateRetriesBeforeDown(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "push-mon", MaxRetries: 1},
		SiteState:  models.SiteState{Status: "LATE"},
	}
	injectSite(e, site)

	e.handleStatusChange(site, "DOWN", 0, 0, "missed heartbeat")
	s, _ := getSite(e, 1)
	if s.Status != "LATE" {
		t.Errorf("expected LATE during retry, got %s", s.Status)
	}

	e.handleStatusChange(s, "DOWN", 0, 0, "missed heartbeat")
	s, _ = getSite(e, 1)
	if s.Status != "DOWN" {
		t.Errorf("expected DOWN after retries exhausted, got %s", s.Status)
	}
}
