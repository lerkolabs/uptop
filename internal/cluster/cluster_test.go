package cluster

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/monitor"
)

// --- Mock Store (minimal, for monitor.NewEngine) ---

type mockStore struct {
	sites []models.Site
}

func (m *mockStore) Init() error                                              { return nil }
func (m *mockStore) GetSites() ([]models.Site, error)                         { return m.sites, nil }
func (m *mockStore) AddSite(models.Site) error                                { return nil }
func (m *mockStore) UpdateSite(models.Site) error                             { return nil }
func (m *mockStore) UpdateSitePaused(int, bool) error                         { return nil }
func (m *mockStore) DeleteSite(int) error                                     { return nil }
func (m *mockStore) GetAllAlerts() ([]models.AlertConfig, error)              { return nil, nil }
func (m *mockStore) GetAlert(int) (models.AlertConfig, error)                 { return models.AlertConfig{}, nil }
func (m *mockStore) AddAlert(string, string, map[string]string) error         { return nil }
func (m *mockStore) UpdateAlert(int, string, string, map[string]string) error { return nil }
func (m *mockStore) DeleteAlert(int) error                                    { return nil }
func (m *mockStore) GetAllUsers() ([]models.User, error)                      { return nil, nil }
func (m *mockStore) AddUser(string, string, string) error                     { return nil }
func (m *mockStore) UpdateUser(int, string, string, string) error             { return nil }
func (m *mockStore) DeleteUser(int) error                                     { return nil }
func (m *mockStore) SaveCheck(int, int64, bool) error                         { return nil }
func (m *mockStore) SaveCheckFromNode(int, string, int64, bool) error         { return nil }
func (m *mockStore) LoadAllHistory(int) (map[int][]models.CheckRecord, error) { return nil, nil }
func (m *mockStore) ExportData() (models.Backup, error)                       { return models.Backup{}, nil }
func (m *mockStore) ImportData(models.Backup) error                           { return nil }
func (m *mockStore) GetSiteByName(string) (models.Site, error)                { return models.Site{}, nil }
func (m *mockStore) GetAlertByName(string) (models.AlertConfig, error) {
	return models.AlertConfig{}, nil
}
func (m *mockStore) AddSiteReturningID(models.Site) (int, error) { return 0, nil }
func (m *mockStore) AddAlertReturningID(string, string, map[string]string) (int, error) {
	return 0, nil
}
func (m *mockStore) RegisterNode(models.ProbeNode) error      { return nil }
func (m *mockStore) GetNode(string) (models.ProbeNode, error) { return models.ProbeNode{}, nil }
func (m *mockStore) GetAllNodes() ([]models.ProbeNode, error) { return nil, nil }
func (m *mockStore) UpdateNodeLastSeen(string) error          { return nil }
func (m *mockStore) DeleteNode(string) error                  { return nil }
func (m *mockStore) LoadAlertHealth() (map[int]models.AlertHealthRecord, error) {
	return nil, nil
}
func (m *mockStore) SaveAlertHealth(models.AlertHealthRecord) error { return nil }
func (m *mockStore) SaveLog(string) error                           { return nil }
func (m *mockStore) LoadLogs(int) ([]string, error)                 { return nil, nil }
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
func (m *mockStore) IsMonitorInMaintenance(int) (bool, error)                    { return false, nil }
func (m *mockStore) GetPreference(string) (string, error)                        { return "", nil }
func (m *mockStore) SetPreference(string, string) error                          { return nil }
func (m *mockStore) SaveStateChange(int, string, string, string) error           { return nil }
func (m *mockStore) GetStateChanges(int, int) ([]models.StateChange, error)      { return nil, nil }
func (m *mockStore) GetStateChangesSince(int, time.Time) ([]models.StateChange, error) {
	return nil, nil
}
func (m *mockStore) Close() error { return nil }

// --- Cluster Start Tests ---

func TestStart_LeaderMode(t *testing.T) {
	eng := monitor.NewEngine(&mockStore{})
	eng.SetActive(false)

	ctx := context.Background()
	Start(ctx, Config{Mode: "leader"}, eng)

	if !eng.IsActive() {
		t.Error("leader mode should set engine active")
	}
}

func TestStart_FollowerMode(t *testing.T) {
	eng := monitor.NewEngine(&mockStore{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Start(ctx, Config{Mode: "follower", PeerURL: "http://localhost:9999"}, eng)
	time.Sleep(50 * time.Millisecond)

	if eng.IsActive() {
		t.Error("follower mode should set engine inactive")
	}
}

// --- Follower Loop Tests ---

func TestFollowerLoop_FailoverOnLeaderDown(t *testing.T) {
	eng := monitor.NewEngine(&mockStore{})
	eng.SetActive(false)

	// Server always returns 503
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runFollowerLoop(ctx, Config{PeerURL: srv.URL, SharedKey: "key"}, eng)

	// Follower checks every 5s, needs 3 failures → ~15s minimum
	// But we can't wait that long in a test. The loop sleeps 5s between checks.
	// We'll wait up to 20s for failover.
	deadline := time.After(20 * time.Second)
	for {
		if eng.IsActive() {
			return // success
		}
		select {
		case <-deadline:
			t.Fatal("expected failover to ACTIVE after 3 failures")
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func TestFollowerLoop_RecoveryOnLeaderReturn(t *testing.T) {
	eng := monitor.NewEngine(&mockStore{})
	eng.SetActive(true) // simulate already failed over

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runFollowerLoop(ctx, Config{PeerURL: srv.URL}, eng)

	deadline := time.After(10 * time.Second)
	for {
		if !eng.IsActive() {
			return // success — switched back to passive
		}
		select {
		case <-deadline:
			t.Fatal("expected switch back to PASSIVE when leader returns")
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func TestFollowerLoop_SendsSecret(t *testing.T) {
	var mu sync.Mutex
	var receivedSecret string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedSecret = r.Header.Get("X-Upkeep-Secret")
		mu.Unlock()
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	}))
	defer srv.Close()

	eng := monitor.NewEngine(&mockStore{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runFollowerLoop(ctx, Config{PeerURL: srv.URL, SharedKey: "test-secret"}, eng)

	deadline := time.After(10 * time.Second)
	for {
		mu.Lock()
		got := receivedSecret
		mu.Unlock()
		if got == "test-secret" {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("expected secret 'test-secret', got %q", got)
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func TestFollowerLoop_CancelContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	eng := monitor.NewEngine(&mockStore{})
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		runFollowerLoop(ctx, Config{PeerURL: srv.URL}, eng)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("expected follower loop to exit on context cancel")
	}
}

// --- Probe Tests ---

func TestProbeRegister_Success(t *testing.T) {
	var received map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	err := probeRegister(context.Background(), srv.Client(), ProbeConfig{
		NodeID: "n1", NodeName: "US East", Region: "us-east", LeaderURL: srv.URL, SharedKey: "key",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if received["id"] != "n1" {
		t.Errorf("expected id n1, got %s", received["id"])
	}
}

func TestProbeRegister_Failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	}))
	defer srv.Close()

	err := probeRegister(context.Background(), srv.Client(), ProbeConfig{
		LeaderURL: srv.URL,
	})
	if err == nil {
		t.Error("expected error on 401")
	}
}

func TestProbeFetchAssignments_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string][]models.Site{
			"sites": {{ID: 1, Name: "s1", Type: "http", URL: "http://example.com"}},
		})
	}))
	defer srv.Close()

	sites, err := probeFetchAssignments(context.Background(), srv.Client(), ProbeConfig{
		NodeID: "n1", LeaderURL: srv.URL, SharedKey: "key",
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(sites) != 1 {
		t.Errorf("expected 1 site, got %d", len(sites))
	}
}

func TestProbeFetchAssignments_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	}))
	defer srv.Close()

	_, err := probeFetchAssignments(context.Background(), srv.Client(), ProbeConfig{
		LeaderURL: srv.URL,
	})
	if err == nil {
		t.Error("expected error on 401")
	}
}

func TestProbeExecuteChecks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	sites := []models.Site{
		{ID: 1, Type: "http", URL: srv.URL},
		{ID: 2, Type: "http", URL: srv.URL},
	}

	strict := &http.Client{}
	insecure := &http.Client{}
	results := probeExecuteChecks(context.Background(), sites, strict, insecure, true)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.IsUp {
			t.Errorf("site %d expected UP", r.SiteID)
		}
	}
}

func TestProbeExecuteChecks_Concurrency(t *testing.T) {
	var concurrent int64
	var maxConcurrent int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := atomic.AddInt64(&concurrent, 1)
		for {
			old := atomic.LoadInt64(&maxConcurrent)
			if cur <= old || atomic.CompareAndSwapInt64(&maxConcurrent, old, cur) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt64(&concurrent, -1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	var sites []models.Site
	for i := 0; i < 20; i++ {
		sites = append(sites, models.Site{ID: i + 1, Type: "http", URL: srv.URL})
	}

	results := probeExecuteChecks(context.Background(), sites, &http.Client{}, &http.Client{}, true)
	if len(results) != 20 {
		t.Errorf("expected 20 results, got %d", len(results))
	}
	mc := atomic.LoadInt64(&maxConcurrent)
	if mc > 10 {
		t.Errorf("expected max 10 concurrent, got %d", mc)
	}
}

func TestProbeReportResults_Success(t *testing.T) {
	var received struct {
		NodeID  string            `json:"node_id"`
		Results []probeResultItem `json:"results"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	err := probeReportResults(context.Background(), srv.Client(), ProbeConfig{
		NodeID: "n1", LeaderURL: srv.URL, SharedKey: "key",
	}, []probeResultItem{{SiteID: 1, LatencyNs: 5000000, IsUp: true}})

	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if received.NodeID != "n1" {
		t.Errorf("expected n1, got %s", received.NodeID)
	}
	if len(received.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(received.Results))
	}
}

func TestProbeReportResults_Failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	err := probeReportResults(context.Background(), srv.Client(), ProbeConfig{
		LeaderURL: srv.URL,
	}, []probeResultItem{{SiteID: 1}})

	if err == nil {
		t.Error("expected error on 500")
	}
}

// --- sleepCtx ---

func TestSleepCtx_Cancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	sleepCtx(ctx, 10*time.Second)
	if time.Since(start) > time.Second {
		t.Error("expected immediate return on canceled context")
	}
}
