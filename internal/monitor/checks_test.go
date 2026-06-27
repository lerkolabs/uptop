package monitor

import (
	"context"
	"testing"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

// --- Group 2: Heartbeat ---

func TestRecordHeartbeat_ValidToken(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "push-test", Type: "push", Token: "abc123"},
		SiteState:  models.SiteState{Status: "UP"},
	}
	injectSite(e, site)

	if !e.RecordHeartbeat("abc123") {
		t.Error("expected true for valid token")
	}

	s, _ := getSite(e, 1)
	if s.Status != "UP" {
		t.Errorf("expected UP, got %s", s.Status)
	}
	if time.Since(s.LastCheck) > time.Second {
		t.Error("expected LastCheck to be recent")
	}
}

func TestRecordHeartbeat_RecoveryFromDown(t *testing.T) {
	ms := newMockStore()
	ms.alerts[1] = models.AlertConfig{ID: 1, Name: "test", Type: "webhook", Settings: map[string]string{"url": "http://example.com"}}
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "push-test", Type: "push", Token: "abc123", AlertID: 1},
		SiteState:  models.SiteState{Status: "DOWN", FailureCount: 3},
	}
	injectSite(e, site)

	if !e.RecordHeartbeat("abc123") {
		t.Error("expected true")
	}

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

func TestRecordHeartbeat_UnknownToken(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)

	if e.RecordHeartbeat("unknown") {
		t.Error("expected false for unknown token")
	}
}

func TestRecordHeartbeat_InactiveEngine(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Type: "push", Token: "abc123"},
		SiteState:  models.SiteState{Status: "UP"},
	}
	injectSite(e, site)
	e.SetActive(false)

	if e.RecordHeartbeat("abc123") {
		t.Error("expected false when inactive")
	}
}

// --- Group 3: Push Deadline ---

func TestCheckPush_DeadlineMissed(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "push", Type: "push", Interval: 10, MaxRetries: 0},
		SiteState:  models.SiteState{Status: "UP", LastCheck: time.Now().Add(-120 * time.Second)},
	}
	injectSite(e, site)

	e.checkPush(context.Background(), site)

	s, _ := getSite(e, 1)
	if s.Status != "DOWN" {
		t.Errorf("expected DOWN after missed deadline, got %s", s.Status)
	}
}

func TestCheckPush_OverdueBecomesLate(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "push", Type: "push", Interval: 300},
		SiteState:  models.SiteState{Status: "UP", LastCheck: time.Now().Add(-310 * time.Second)},
	}
	injectSite(e, site)

	e.checkPush(context.Background(), site)

	s, _ := getSite(e, 1)
	if s.Status != "LATE" {
		t.Errorf("expected LATE when overdue but within grace, got %s", s.Status)
	}
}

func TestCheckPush_OverdueBecomesStale(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	// interval=300, grace=150 (300/2), staleMark=overdue+75
	// at 380s: past staleMark(375) but before graceEnd(450)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "push", Type: "push", Interval: 300},
		SiteState:  models.SiteState{Status: "UP", LastCheck: time.Now().Add(-380 * time.Second)},
	}
	injectSite(e, site)

	e.checkPush(context.Background(), site)

	s, _ := getSite(e, 1)
	if s.Status != "STALE" {
		t.Errorf("expected STALE when past midpoint of grace, got %s", s.Status)
	}
}

func TestCheckPush_WithinDeadline(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "push", Type: "push", Interval: 60},
		SiteState:  models.SiteState{Status: "UP", LastCheck: time.Now()},
	}
	injectSite(e, site)

	e.checkPush(context.Background(), site)

	s, _ := getSite(e, 1)
	if s.Status != "UP" {
		t.Errorf("expected UP, got %s", s.Status)
	}
}

func TestCheckPush_PendingStaysPending(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "push", Type: "push", Interval: 60},
		SiteState:  models.SiteState{Status: "PENDING"},
	}
	injectSite(e, site)

	e.checkPush(context.Background(), site)

	s, _ := getSite(e, 1)
	if s.Status != "PENDING" {
		t.Errorf("expected PENDING to stay until first heartbeat, got %s", s.Status)
	}
}

// --- Group 4: Group Checks ---

func TestCheckGroup_AllChildrenUp(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	group := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "group", Type: "group"},
		SiteState:  models.SiteState{Status: "PENDING"},
	}
	child1 := models.Site{
		SiteConfig: models.SiteConfig{ID: 2, Name: "child1", Type: "http", ParentID: 1},
		SiteState:  models.SiteState{Status: "UP"},
	}
	child2 := models.Site{
		SiteConfig: models.SiteConfig{ID: 3, Name: "child2", Type: "http", ParentID: 1},
		SiteState:  models.SiteState{Status: "UP"},
	}
	injectSite(e, group)
	injectSite(e, child1)
	injectSite(e, child2)

	e.checkGroup(context.Background(), group)

	s, _ := getSite(e, 1)
	if s.Status != "UP" {
		t.Errorf("expected group UP, got %s", s.Status)
	}
}

func TestCheckGroup_OneChildDown(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	group := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "group", Type: "group"},
		SiteState:  models.SiteState{Status: "UP"},
	}
	child1 := models.Site{
		SiteConfig: models.SiteConfig{ID: 2, Name: "child1", Type: "http", ParentID: 1},
		SiteState:  models.SiteState{Status: "UP"},
	}
	child2 := models.Site{
		SiteConfig: models.SiteConfig{ID: 3, Name: "child2", Type: "http", ParentID: 1},
		SiteState:  models.SiteState{Status: "DOWN"},
	}
	injectSite(e, group)
	injectSite(e, child1)
	injectSite(e, child2)

	e.checkGroup(context.Background(), group)

	s, _ := getSite(e, 1)
	if s.Status != "DOWN" {
		t.Errorf("expected group DOWN, got %s", s.Status)
	}
}

func TestCheckGroup_PausedChildIgnored(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	group := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "group", Type: "group"},
	}
	child1 := models.Site{
		SiteConfig: models.SiteConfig{ID: 2, Name: "child1", Type: "http", ParentID: 1},
		SiteState:  models.SiteState{Status: "UP"},
	}
	child2 := models.Site{
		SiteConfig: models.SiteConfig{ID: 3, Name: "child2", Type: "http", ParentID: 1, Paused: true},
		SiteState:  models.SiteState{Status: "DOWN"},
	}
	injectSite(e, group)
	injectSite(e, child1)
	injectSite(e, child2)

	e.checkGroup(context.Background(), group)

	s, _ := getSite(e, 1)
	if s.Status != "UP" {
		t.Errorf("expected UP (paused child ignored), got %s", s.Status)
	}
}

func TestCheckGroup_MaintenanceChildIgnored(t *testing.T) {
	ms := newMockStore()
	ms.maintenance[3] = true
	e := newTestEngine(ms)
	group := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "group", Type: "group"},
	}
	child1 := models.Site{
		SiteConfig: models.SiteConfig{ID: 2, Name: "child1", Type: "http", ParentID: 1},
		SiteState:  models.SiteState{Status: "UP"},
	}
	child2 := models.Site{
		SiteConfig: models.SiteConfig{ID: 3, Name: "child2", Type: "http", ParentID: 1},
		SiteState:  models.SiteState{Status: "DOWN"},
	}
	injectSite(e, group)
	injectSite(e, child1)
	injectSite(e, child2)
	e.refreshMaintenanceCache(context.Background())

	e.checkGroup(context.Background(), group)

	s, _ := getSite(e, 1)
	if s.Status != "UP" {
		t.Errorf("expected UP (maint child ignored), got %s", s.Status)
	}
}

func TestCheckGroup_NoChildren(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	group := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "group", Type: "group"},
		SiteState:  models.SiteState{Status: "UP"},
	}
	injectSite(e, group)

	e.checkGroup(context.Background(), group)

	s, _ := getSite(e, 1)
	if s.Status != "PENDING" {
		t.Errorf("expected PENDING for no children, got %s", s.Status)
	}
}

// Groups must not auto-pause when all children are paused — that creates a
// one-way trap because monitorRoutine skips paused sites.
func TestCheckGroup_AllPausedNoAutoFreeze(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	group := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "group", Type: "group"},
		SiteState:  models.SiteState{Status: "UP"},
	}
	child1 := models.Site{
		SiteConfig: models.SiteConfig{ID: 2, Name: "child1", Type: "http", ParentID: 1, Paused: true},
		SiteState:  models.SiteState{Status: "UP"},
	}
	child2 := models.Site{
		SiteConfig: models.SiteConfig{ID: 3, Name: "child2", Type: "http", ParentID: 1, Paused: true},
		SiteState:  models.SiteState{Status: "UP"},
	}
	injectSite(e, group)
	injectSite(e, child1)
	injectSite(e, child2)

	e.checkGroup(context.Background(), group)

	s, _ := getSite(e, 1)
	if s.Paused {
		t.Error("group must not auto-pause when all children are paused")
	}
}

// Dead probe results must be expired so they don't poison aggregation.
func TestIngestProbeResult_ExpiresStaleProbes(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "test", Type: "http", Interval: 30},
		SiteState:  models.SiteState{Status: "UP"},
	}
	injectSite(e, site)

	e.probeResultsMu.Lock()
	e.probeResults[1] = map[string]NodeResult{
		"dead-probe": {
			NodeID:    "dead-probe",
			IsUp:      false,
			CheckedAt: time.Now().Add(-10 * time.Minute),
		},
	}
	e.probeResultsMu.Unlock()

	e.IngestProbeResult("live-probe", 1, 5000, true, "")

	e.probeResultsMu.RLock()
	_, deadExists := e.probeResults[1]["dead-probe"]
	_, liveExists := e.probeResults[1]["live-probe"]
	e.probeResultsMu.RUnlock()

	if deadExists {
		t.Error("stale probe result should have been expired")
	}
	if !liveExists {
		t.Error("live probe result should still exist")
	}
}

// RemoveSite must clean up probeResults.
func TestRemoveSite_CleansProbeResults(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "test", Type: "http"},
		SiteState:  models.SiteState{Status: "UP"},
	}
	injectSite(e, site)

	e.probeResultsMu.Lock()
	e.probeResults[1] = map[string]NodeResult{
		"node-a": {NodeID: "node-a", IsUp: true, CheckedAt: time.Now()},
	}
	e.probeResultsMu.Unlock()

	e.RemoveSite(1)

	e.probeResultsMu.RLock()
	defer e.probeResultsMu.RUnlock()
	if _, exists := e.probeResults[1]; exists {
		t.Error("probe results should be cleaned up after RemoveSite")
	}
}
