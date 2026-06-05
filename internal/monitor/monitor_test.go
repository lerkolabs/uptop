package monitor

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

// --- Mock Store ---

type savedCheck struct {
	SiteID    int
	LatencyNs int64
	IsUp      bool
}

type mockStore struct {
	mu            sync.Mutex
	sites         []models.Site
	alerts        map[int]models.AlertConfig
	maintenance   map[int]bool
	logs          []string
	history       map[int][]models.CheckRecord
	savedChecks   []savedCheck
	savedLogs     []string
	getAlertCalls []int
}

func newMockStore() *mockStore {
	return &mockStore{
		alerts:      make(map[int]models.AlertConfig),
		maintenance: make(map[int]bool),
		history:     make(map[int][]models.CheckRecord),
	}
}

func (m *mockStore) Init() error                                              { return nil }
func (m *mockStore) GetSites() ([]models.Site, error)                         { return m.sites, nil }
func (m *mockStore) AddSite(models.Site) error                                { return nil }
func (m *mockStore) UpdateSite(models.Site) error                             { return nil }
func (m *mockStore) UpdateSitePaused(int, bool) error                         { return nil }
func (m *mockStore) DeleteSite(int) error                                     { return nil }
func (m *mockStore) AddAlert(string, string, map[string]string) error         { return nil }
func (m *mockStore) UpdateAlert(int, string, string, map[string]string) error { return nil }
func (m *mockStore) DeleteAlert(int) error                                    { return nil }
func (m *mockStore) GetAllUsers() ([]models.User, error)                      { return nil, nil }
func (m *mockStore) AddUser(string, string, string) error                     { return nil }
func (m *mockStore) UpdateUser(int, string, string, string) error             { return nil }
func (m *mockStore) DeleteUser(int) error                                     { return nil }
func (m *mockStore) ExportData() (models.Backup, error)                       { return models.Backup{}, nil }
func (m *mockStore) ImportData(models.Backup) error                           { return nil }
func (m *mockStore) GetSiteByName(string) (models.Site, error)                { return models.Site{}, nil }
func (m *mockStore) AddSiteReturningID(models.Site) (int, error)              { return 0, nil }
func (m *mockStore) AddAlertReturningID(string, string, map[string]string) (int, error) {
	return 0, nil
}
func (m *mockStore) SaveCheckFromNode(int, string, int64, bool) error { return nil }
func (m *mockStore) RegisterNode(models.ProbeNode) error              { return nil }
func (m *mockStore) GetNode(string) (models.ProbeNode, error)         { return models.ProbeNode{}, nil }
func (m *mockStore) GetAllNodes() ([]models.ProbeNode, error)         { return nil, nil }
func (m *mockStore) UpdateNodeLastSeen(string) error                  { return nil }
func (m *mockStore) DeleteNode(string) error                          { return nil }
func (m *mockStore) LoadAlertHealth() (map[int]models.AlertHealthRecord, error) {
	return nil, nil
}
func (m *mockStore) SaveAlertHealth(models.AlertHealthRecord) error { return nil }
func (m *mockStore) GetActiveMaintenanceWindows() ([]models.MaintenanceWindow, error) {
	return nil, nil
}
func (m *mockStore) GetAllMaintenanceWindows(int) ([]models.MaintenanceWindow, error) {
	return nil, nil
}
func (m *mockStore) AddMaintenanceWindow(models.MaintenanceWindow) error         { return nil }
func (m *mockStore) EndMaintenanceWindow(int) error                              { return nil }
func (m *mockStore) DeleteMaintenanceWindow(int) error                           { return nil }
func (m *mockStore) PruneExpiredMaintenanceWindows(time.Duration) (int64, error) { return 0, nil }
func (m *mockStore) GetPreference(string) (string, error)                        { return "", nil }
func (m *mockStore) SetPreference(string, string) error                          { return nil }
func (m *mockStore) SaveStateChange(int, string, string, string) error           { return nil }
func (m *mockStore) GetStateChanges(int, int) ([]models.StateChange, error)      { return nil, nil }
func (m *mockStore) GetStateChangesSince(int, time.Time) ([]models.StateChange, error) {
	return nil, nil
}
func (m *mockStore) Close() error { return nil }

func (m *mockStore) GetAllAlerts() ([]models.AlertConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []models.AlertConfig
	for _, a := range m.alerts {
		result = append(result, a)
	}
	return result, nil
}

func (m *mockStore) GetAlert(id int) (models.AlertConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getAlertCalls = append(m.getAlertCalls, id)
	if a, ok := m.alerts[id]; ok {
		return a, nil
	}
	return models.AlertConfig{}, fmt.Errorf("alert %d not found", id)
}

func (m *mockStore) GetAlertByName(name string) (models.AlertConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.alerts {
		if a.Name == name {
			return a, nil
		}
	}
	return models.AlertConfig{}, fmt.Errorf("alert %q not found", name)
}

func (m *mockStore) IsMonitorInMaintenance(id int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.maintenance[id], nil
}

func (m *mockStore) SaveCheck(siteID int, latencyNs int64, isUp bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.savedChecks = append(m.savedChecks, savedCheck{siteID, latencyNs, isUp})
	return nil
}

func (m *mockStore) SaveLog(msg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.savedLogs = append(m.savedLogs, msg)
	return nil
}

func (m *mockStore) LoadLogs(limit int) ([]string, error) {
	return m.logs, nil
}

func (m *mockStore) LoadAllHistory(limit int) (map[int][]models.CheckRecord, error) {
	return m.history, nil
}

// --- Helpers ---

func newTestEngine(ms *mockStore) *Engine {
	return NewEngine(ms)
}

func injectSite(e *Engine, site models.Site) {
	e.mu.Lock()
	e.liveState[site.ID] = site
	e.addToTokenIndex(site)
	e.mu.Unlock()
}

func getSite(e *Engine, id int) (models.Site, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s, ok := e.liveState[id]
	return s, ok
}

func waitAsync() {
	time.Sleep(50 * time.Millisecond)
}

func (m *mockStore) getAlertCallsSnapshot() []int {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]int, len(m.getAlertCalls))
	copy(cp, m.getAlertCalls)
	return cp
}

// --- Group 1: State Machine ---

func TestHandleStatusChange_PendingToUp(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{ID: 1, Name: "test", Status: "PENDING", MaxRetries: 3, AlertID: 1}
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
	site := models.Site{ID: 1, Name: "test", Status: "UP", MaxRetries: 3, FailureCount: 0}
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
	site := models.Site{ID: 1, Name: "test", Status: "UP", MaxRetries: 2, FailureCount: 2, AlertID: 1}
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
	site := models.Site{ID: 1, Name: "test", Status: "UP", MaxRetries: 0, FailureCount: 0, AlertID: 1}
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
	site := models.Site{ID: 1, Name: "test", Status: "DOWN", FailureCount: 4, AlertID: 1}
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
	site := models.Site{ID: 1, Name: "test", Status: "DOWN", MaxRetries: 2, FailureCount: 3}
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
	site := models.Site{ID: 1, Name: "test", Status: "UP", MaxRetries: 0, AlertID: 1}
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
	site := models.Site{ID: 1, Name: "test", Status: "UP", MaxRetries: 0, AlertID: 1}
	injectSite(e, site)

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
		if containsStr(l, "suppressed") {
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
	site := models.Site{ID: 1, Name: "test", Status: "DOWN", AlertID: 1}
	injectSite(e, site)

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
		ID: 1, Name: "test", Status: "UP", Type: "http",
		CheckSSL: true, HasSSL: true, ExpiryThreshold: 30,
		SentSSLWarning: false, AlertID: 1,
		CertExpiry: time.Now().Add(15 * 24 * time.Hour),
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
		ID: 1, Name: "test", Status: "UP", Type: "http",
		CheckSSL: true, HasSSL: true, ExpiryThreshold: 30,
		SentSSLWarning: true, AlertID: 1,
		CertExpiry: time.Now().Add(15 * 24 * time.Hour),
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
		ID: 1, Name: "test", Status: "UP", Type: "http",
		CheckSSL: true, HasSSL: true, ExpiryThreshold: 30,
		SentSSLWarning: true,
		CertExpiry:     time.Now().Add(60 * 24 * time.Hour),
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
		ID: 1, Name: "test", Status: "UP", Type: "http",
		CheckSSL: true, HasSSL: true, ExpiryThreshold: 30,
		SentSSLWarning: false, AlertID: 1,
		CertExpiry: time.Now().Add(15 * 24 * time.Hour),
	}
	injectSite(e, site)

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
	site := models.Site{ID: 1, Name: "test", Status: "UP", MaxRetries: 0}
	injectSite(e, site)
	e.SetActive(false)

	e.handleStatusChange(site, "DOWN", 0, 0, "test error")

	s, _ := getSite(e, 1)
	if s.Status != "UP" {
		t.Error("expected no state change when inactive")
	}
}

// --- Group 2: Heartbeat ---

func TestRecordHeartbeat_ValidToken(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{ID: 1, Name: "push-test", Type: "push", Token: "abc123", Status: "UP"}
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
	site := models.Site{ID: 1, Name: "push-test", Type: "push", Token: "abc123", Status: "DOWN", AlertID: 1, FailureCount: 3}
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
	site := models.Site{ID: 1, Type: "push", Token: "abc123", Status: "UP"}
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
		ID: 1, Name: "push", Type: "push", Status: "UP",
		Interval: 10, MaxRetries: 0,
		LastCheck: time.Now().Add(-120 * time.Second),
	}
	injectSite(e, site)

	e.checkPush(site)

	s, _ := getSite(e, 1)
	if s.Status != "DOWN" {
		t.Errorf("expected DOWN after missed deadline, got %s", s.Status)
	}
}

func TestCheckPush_OverdueBecomesLate(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{
		ID: 1, Name: "push", Type: "push", Status: "UP",
		Interval:  300,
		LastCheck: time.Now().Add(-310 * time.Second),
	}
	injectSite(e, site)

	e.checkPush(site)

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
		ID: 1, Name: "push", Type: "push", Status: "UP",
		Interval:  300,
		LastCheck: time.Now().Add(-380 * time.Second),
	}
	injectSite(e, site)

	e.checkPush(site)

	s, _ := getSite(e, 1)
	if s.Status != "STALE" {
		t.Errorf("expected STALE when past midpoint of grace, got %s", s.Status)
	}
}

func TestCheckPush_WithinDeadline(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{
		ID: 1, Name: "push", Type: "push", Status: "UP",
		Interval: 60, LastCheck: time.Now(),
	}
	injectSite(e, site)

	e.checkPush(site)

	s, _ := getSite(e, 1)
	if s.Status != "UP" {
		t.Errorf("expected UP, got %s", s.Status)
	}
}

func TestCheckPush_PendingStaysPending(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{
		ID: 1, Name: "push", Type: "push", Status: "PENDING",
		Interval: 60,
	}
	injectSite(e, site)

	e.checkPush(site)

	s, _ := getSite(e, 1)
	if s.Status != "PENDING" {
		t.Errorf("expected PENDING to stay until first heartbeat, got %s", s.Status)
	}
}

// --- Group 4: Group Checks ---

func TestCheckGroup_AllChildrenUp(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	group := models.Site{ID: 1, Name: "group", Type: "group", Status: "PENDING"}
	child1 := models.Site{ID: 2, Name: "child1", Type: "http", ParentID: 1, Status: "UP"}
	child2 := models.Site{ID: 3, Name: "child2", Type: "http", ParentID: 1, Status: "UP"}
	injectSite(e, group)
	injectSite(e, child1)
	injectSite(e, child2)

	e.checkGroup(group)

	s, _ := getSite(e, 1)
	if s.Status != "UP" {
		t.Errorf("expected group UP, got %s", s.Status)
	}
}

func TestCheckGroup_OneChildDown(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	group := models.Site{ID: 1, Name: "group", Type: "group", Status: "UP"}
	child1 := models.Site{ID: 2, Name: "child1", Type: "http", ParentID: 1, Status: "UP"}
	child2 := models.Site{ID: 3, Name: "child2", Type: "http", ParentID: 1, Status: "DOWN"}
	injectSite(e, group)
	injectSite(e, child1)
	injectSite(e, child2)

	e.checkGroup(group)

	s, _ := getSite(e, 1)
	if s.Status != "DOWN" {
		t.Errorf("expected group DOWN, got %s", s.Status)
	}
}

func TestCheckGroup_PausedChildIgnored(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	group := models.Site{ID: 1, Name: "group", Type: "group"}
	child1 := models.Site{ID: 2, Name: "child1", Type: "http", ParentID: 1, Status: "UP"}
	child2 := models.Site{ID: 3, Name: "child2", Type: "http", ParentID: 1, Status: "DOWN", Paused: true}
	injectSite(e, group)
	injectSite(e, child1)
	injectSite(e, child2)

	e.checkGroup(group)

	s, _ := getSite(e, 1)
	if s.Status != "UP" {
		t.Errorf("expected UP (paused child ignored), got %s", s.Status)
	}
}

func TestCheckGroup_MaintenanceChildIgnored(t *testing.T) {
	ms := newMockStore()
	ms.maintenance[3] = true
	e := newTestEngine(ms)
	group := models.Site{ID: 1, Name: "group", Type: "group"}
	child1 := models.Site{ID: 2, Name: "child1", Type: "http", ParentID: 1, Status: "UP"}
	child2 := models.Site{ID: 3, Name: "child2", Type: "http", ParentID: 1, Status: "DOWN"}
	injectSite(e, group)
	injectSite(e, child1)
	injectSite(e, child2)

	e.checkGroup(group)

	s, _ := getSite(e, 1)
	if s.Status != "UP" {
		t.Errorf("expected UP (maint child ignored), got %s", s.Status)
	}
}

func TestCheckGroup_NoChildren(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	group := models.Site{ID: 1, Name: "group", Type: "group", Status: "UP"}
	injectSite(e, group)

	e.checkGroup(group)

	s, _ := getSite(e, 1)
	if s.Status != "PENDING" {
		t.Errorf("expected PENDING for no children, got %s", s.Status)
	}
}

// --- Group 5: History ---

func TestRecordCheck_Appends(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)

	e.recordCheck(1, 5*time.Millisecond, true)

	h, ok := e.GetHistory(1)
	if !ok {
		t.Fatal("expected history for site 1")
	}
	if h.TotalChecks != 1 || h.UpChecks != 1 {
		t.Errorf("expected 1/1, got %d/%d", h.TotalChecks, h.UpChecks)
	}
	if len(h.Latencies) != 1 || h.Latencies[0] != 5*time.Millisecond {
		t.Errorf("unexpected latencies: %v", h.Latencies)
	}
}

func TestRecordCheck_RollingWindow(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)

	for i := 0; i < 65; i++ {
		e.recordCheck(1, time.Duration(i)*time.Millisecond, i%2 == 0)
	}

	h, _ := e.GetHistory(1)
	if len(h.Latencies) != 60 {
		t.Errorf("expected 60 latencies, got %d", len(h.Latencies))
	}
	if len(h.Statuses) != 60 {
		t.Errorf("expected 60 statuses, got %d", len(h.Statuses))
	}
	if h.TotalChecks != 65 {
		t.Errorf("expected TotalChecks 65, got %d", h.TotalChecks)
	}
}

func TestGetHistory_DeepCopy(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	e.recordCheck(1, 5*time.Millisecond, true)

	h1, _ := e.GetHistory(1)
	h1.Latencies[0] = 999 * time.Second
	h1.TotalChecks = 999

	h2, _ := e.GetHistory(1)
	if h2.Latencies[0] == 999*time.Second {
		t.Error("GetHistory returned reference, not copy")
	}
	if h2.TotalChecks == 999 {
		t.Error("GetHistory returned reference, not copy")
	}
}

func TestInitHistory_LoadsFromDB(t *testing.T) {
	ms := newMockStore()
	ms.history[1] = []models.CheckRecord{
		{SiteID: 1, LatencyNs: 5000000, IsUp: true},
		{SiteID: 1, LatencyNs: 3000000, IsUp: false},
	}
	e := newTestEngine(ms)
	e.InitHistory()

	h, ok := e.GetHistory(1)
	if !ok {
		t.Fatal("expected history for site 1")
	}
	if h.TotalChecks != 2 {
		t.Errorf("expected TotalChecks 2, got %d", h.TotalChecks)
	}
	if h.UpChecks != 1 {
		t.Errorf("expected UpChecks 1, got %d", h.UpChecks)
	}
}

// --- Group 6: State Management ---

func TestUpdateSiteConfig_PreservesRuntime(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{ID: 1, Name: "test", URL: "http://old.com", Status: "DOWN", FailureCount: 3, Latency: 100 * time.Millisecond}
	injectSite(e, site)

	updated := models.Site{ID: 1, Name: "test", URL: "http://new.com", Interval: 60}
	e.UpdateSiteConfig(updated)

	s, _ := getSite(e, 1)
	if s.URL != "http://new.com" {
		t.Errorf("expected URL updated, got %s", s.URL)
	}
	if s.Status != "DOWN" {
		t.Errorf("expected Status preserved, got %s", s.Status)
	}
	if s.FailureCount != 3 {
		t.Errorf("expected FailureCount preserved, got %d", s.FailureCount)
	}
	if s.Latency != 100*time.Millisecond {
		t.Errorf("expected Latency preserved, got %v", s.Latency)
	}
}

func TestRemoveSite_CleansUp(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{ID: 1, Name: "test", Type: "push", Token: "tok1", Status: "UP"}
	injectSite(e, site)
	e.recordCheck(1, 5*time.Millisecond, true)

	e.RemoveSite(1)

	if _, ok := getSite(e, 1); ok {
		t.Error("expected site removed from liveState")
	}
	if e.RecordHeartbeat("tok1") {
		t.Error("expected token removed from index")
	}
	if _, ok := e.GetHistory(1); ok {
		t.Error("expected history removed")
	}
}

func TestToggleSitePause(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{ID: 1, Name: "test", Status: "UP"}
	injectSite(e, site)

	paused := e.ToggleSitePause(1)
	if !paused {
		t.Error("expected paused=true after first toggle")
	}
	s, _ := getSite(e, 1)
	if !s.Paused {
		t.Error("expected Paused=true in state")
	}

	paused = e.ToggleSitePause(1)
	if paused {
		t.Error("expected paused=false after second toggle")
	}
}

func TestToggleSitePause_NonexistentSite(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	if e.ToggleSitePause(999) {
		t.Error("expected false for nonexistent site")
	}
}

func TestGetAllSites_ReturnsCopy(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	injectSite(e, models.Site{ID: 1, Name: "s1", Status: "UP"})
	injectSite(e, models.Site{ID: 2, Name: "s2", Status: "DOWN"})

	sites := e.GetAllSites()
	if len(sites) != 2 {
		t.Fatalf("expected 2 sites, got %d", len(sites))
	}
	sites[0].Name = "mutated"

	fresh := e.GetAllSites()
	for _, s := range fresh {
		if s.Name == "mutated" {
			t.Error("GetAllSites returned reference, not copy")
		}
	}
}

func TestGetLiveState_ReturnsCopy(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	injectSite(e, models.Site{ID: 1, Name: "s1", Status: "UP"})

	state := e.GetLiveState()
	state[1] = models.Site{Name: "mutated"}

	fresh := e.GetLiveState()
	if fresh[1].Name == "mutated" {
		t.Error("GetLiveState returned reference, not copy")
	}
}

// --- Group 7: Logs ---

func TestAddLog_PrependAndCap(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)

	for i := 0; i < 105; i++ {
		e.AddLog(fmt.Sprintf("log-%d", i))
	}

	logs := e.GetLogs()
	if len(logs) != 100 {
		t.Errorf("expected 100 logs, got %d", len(logs))
	}
	if !containsStr(logs[0], "log-104") {
		t.Errorf("expected newest log first, got %s", logs[0])
	}
}

func TestInitLogs_LoadsFromDB(t *testing.T) {
	ms := newMockStore()
	ms.logs = []string{"old-log-1", "old-log-2"}
	e := newTestEngine(ms)
	e.InitLogs()

	logs := e.GetLogs()
	if len(logs) != 2 {
		t.Errorf("expected 2 logs, got %d", len(logs))
	}
}

// --- Group 8: Probe Aggregation ---

func TestAggregateStatus_AnyDown(t *testing.T) {
	results := []NodeResult{
		{IsUp: true, LatencyNs: 100},
		{IsUp: false, LatencyNs: 200},
	}
	isUp, _ := AggregateStatus(results, AggAnyDown)
	if isUp {
		t.Error("AggAnyDown: expected DOWN when any node is down")
	}
}

func TestAggregateStatus_AnyDown_AllUp(t *testing.T) {
	results := []NodeResult{
		{IsUp: true, LatencyNs: 100},
		{IsUp: true, LatencyNs: 200},
	}
	isUp, _ := AggregateStatus(results, AggAnyDown)
	if !isUp {
		t.Error("AggAnyDown: expected UP when all nodes up")
	}
}

func TestAggregateStatus_Majority(t *testing.T) {
	results := []NodeResult{
		{IsUp: true, LatencyNs: 100},
		{IsUp: true, LatencyNs: 200},
		{IsUp: false, LatencyNs: 300},
	}
	isUp, _ := AggregateStatus(results, AggMajorityDown)
	if !isUp {
		t.Error("AggMajority: expected UP when 2/3 are up")
	}
}

func TestAggregateStatus_AllDown(t *testing.T) {
	results := []NodeResult{
		{IsUp: false, LatencyNs: 100},
		{IsUp: false, LatencyNs: 200},
		{IsUp: true, LatencyNs: 300},
	}
	isUp, _ := AggregateStatus(results, AggAllDown)
	if !isUp {
		t.Error("AggAllDown: expected UP when at least one node up")
	}
}

func TestAggregateStatus_Empty(t *testing.T) {
	isUp, avg := AggregateStatus(nil, AggAnyDown)
	if !isUp {
		t.Error("expected UP for empty results")
	}
	if avg != 0 {
		t.Errorf("expected 0 avg latency, got %d", avg)
	}
}

func TestAggregateStatus_LatencyAverage(t *testing.T) {
	results := []NodeResult{
		{IsUp: true, LatencyNs: 100},
		{IsUp: true, LatencyNs: 200},
		{IsUp: true, LatencyNs: 300},
	}
	_, avg := AggregateStatus(results, AggAnyDown)
	if avg != 200 {
		t.Errorf("expected avg 200, got %d", avg)
	}
}

// --- Group 9: Concurrency ---

func TestConcurrent_RecordHeartbeat(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	for i := 0; i < 10; i++ {
		injectSite(e, models.Site{
			ID: i + 1, Type: "push", Token: fmt.Sprintf("tok-%d", i+1), Status: "UP",
		})
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			e.RecordHeartbeat(fmt.Sprintf("tok-%d", (n%10)+1))
		}(i)
	}
	wg.Wait()
}

func TestConcurrent_HandleStatusChangeAndGetState(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	site := models.Site{ID: 1, Name: "test", Status: "UP", MaxRetries: 100}
	injectSite(e, site)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			e.handleStatusChange(site, "DOWN", 500, 0, "test error")
		}()
		go func() {
			defer wg.Done()
			e.GetLiveState()
		}()
	}
	wg.Wait()
}

func TestConcurrent_RecordCheckAndGetHistory(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			e.recordCheck(1, time.Duration(n)*time.Millisecond, true)
		}(i)
		go func() {
			defer wg.Done()
			e.GetHistory(1)
		}()
	}
	wg.Wait()

	h, ok := e.GetHistory(1)
	if !ok {
		t.Fatal("expected history")
	}
	if len(h.Latencies) > maxHistoryLen {
		t.Errorf("history exceeded max: %d", len(h.Latencies))
	}
}

// --- Utilities ---

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
