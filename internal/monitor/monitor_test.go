package monitor

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/store/storetest"
)

// --- Mock Store ---

type savedCheck struct {
	SiteID    int
	LatencyNs int64
	IsUp      bool
}

type mockStore struct {
	storetest.BaseMock
	mu            sync.Mutex
	sites         []models.SiteConfig
	alerts        map[int]models.AlertConfig
	maintenance   map[int]bool
	logs          []models.LogEntry
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

func (m *mockStore) GetSites(context.Context) ([]models.SiteConfig, error) { return m.sites, nil }

func (m *mockStore) GetActiveMaintenanceWindows(context.Context) ([]models.MaintenanceWindow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var windows []models.MaintenanceWindow
	for id := range m.maintenance {
		windows = append(windows, models.MaintenanceWindow{MonitorID: id})
	}
	return windows, nil
}

func (m *mockStore) GetAllAlerts(context.Context) ([]models.AlertConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []models.AlertConfig
	for _, a := range m.alerts {
		result = append(result, a)
	}
	return result, nil
}

func (m *mockStore) GetAlert(_ context.Context, id int) (models.AlertConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getAlertCalls = append(m.getAlertCalls, id)
	if a, ok := m.alerts[id]; ok {
		return a, nil
	}
	return models.AlertConfig{}, fmt.Errorf("alert %d not found", id)
}

func (m *mockStore) GetAlertByName(_ context.Context, name string) (models.AlertConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.alerts {
		if a.Name == name {
			return a, nil
		}
	}
	return models.AlertConfig{}, fmt.Errorf("alert %q not found", name)
}

func (m *mockStore) IsMonitorInMaintenance(_ context.Context, id int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.maintenance[id], nil
}

func (m *mockStore) SaveCheck(_ context.Context, siteID int, latencyNs int64, isUp bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.savedChecks = append(m.savedChecks, savedCheck{siteID, latencyNs, isUp})
	return nil
}

func (m *mockStore) SaveLog(_ context.Context, msg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.savedLogs = append(m.savedLogs, msg)
	return nil
}

func (m *mockStore) LoadLogs(_ context.Context, _ int) ([]models.LogEntry, error) {
	return m.logs, nil
}

func (m *mockStore) LoadAllHistory(_ context.Context, _ int) (map[int][]models.CheckRecord, error) {
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
	e.InitHistory(context.Background())

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
	if !containsStr(logs[0].Message, "log-104") {
		t.Errorf("expected newest log first, got %s", logs[0].Message)
	}
}

func TestInitLogs_LoadsFromDB(t *testing.T) {
	ms := newMockStore()
	ms.logs = []models.LogEntry{
		{Message: "old-log-1"},
		{Message: "old-log-2"},
	}
	e := newTestEngine(ms)
	e.InitLogs(context.Background())

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
			SiteConfig: models.SiteConfig{ID: i + 1, Type: "push", Token: fmt.Sprintf("tok-%d", i+1)},
			SiteState:  models.SiteState{Status: "UP"},
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
	site := models.Site{
		SiteConfig: models.SiteConfig{ID: 1, Name: "test", MaxRetries: 100},
		SiteState:  models.SiteState{Status: "UP"},
	}
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

// --- Group 11: single DB writer ---

// Writes enqueued through the engine are persisted by the writer goroutine and
// fully drained when the engine stops — no fire-and-forget, no lost writes.
func TestDBWriter_DrainsOnStop(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	e.Start(context.Background())

	e.enqueueWrite(writeCheck{siteID: 7, latencyNs: 100, isUp: true})
	e.enqueueWrite(writeLog{message: "drain-me"})

	e.Stop() // blocks until the writer has drained the queue

	ms.mu.Lock()
	defer ms.mu.Unlock()
	gotCheck := false
	for _, c := range ms.savedChecks {
		if c.SiteID == 7 {
			gotCheck = true
		}
	}
	if !gotCheck {
		t.Error("check was not persisted before Stop returned")
	}
	gotLog := false
	for _, l := range ms.savedLogs {
		if l == "drain-me" {
			gotLog = true
		}
	}
	if !gotLog {
		t.Error("log was not persisted before Stop returned")
	}
}

// Stop must be idempotent — safe to call more than once.
func TestEngineStop_Idempotent(t *testing.T) {
	ms := newMockStore()
	e := newTestEngine(ms)
	e.Start(context.Background())
	e.Stop()
	e.Stop() // must not panic or block
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
