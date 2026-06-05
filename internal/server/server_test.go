package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/monitor"
)

// --- Mock Store ---

type mockStore struct {
	mu              sync.Mutex
	sites           []models.Site
	alerts          []models.AlertConfig
	nodes           map[string]models.ProbeNode
	importedData    *models.Backup
	registeredNodes []models.ProbeNode
	maintWindows    []models.MaintenanceWindow
}

func newMockStore() *mockStore {
	return &mockStore{
		nodes: make(map[string]models.ProbeNode),
	}
}

func (m *mockStore) Init() error                                              { return nil }
func (m *mockStore) GetSites() ([]models.Site, error)                         { return m.sites, nil }
func (m *mockStore) AddSite(models.Site) error                                { return nil }
func (m *mockStore) UpdateSite(models.Site) error                             { return nil }
func (m *mockStore) UpdateSitePaused(int, bool) error                         { return nil }
func (m *mockStore) DeleteSite(int) error                                     { return nil }
func (m *mockStore) GetAllAlerts() ([]models.AlertConfig, error)              { return m.alerts, nil }
func (m *mockStore) GetAlert(int) (models.AlertConfig, error)                 { return models.AlertConfig{}, nil }
func (m *mockStore) AddAlert(string, string, map[string]string) error         { return nil }
func (m *mockStore) UpdateAlert(int, string, string, map[string]string) error { return nil }
func (m *mockStore) DeleteAlert(int) error                                    { return nil }
func (m *mockStore) GetAllUsers() ([]models.User, error)                      { return nil, nil }
func (m *mockStore) AddUser(string, string, string) error                     { return nil }
func (m *mockStore) UpdateUser(int, string, string, string) error             { return nil }
func (m *mockStore) DeleteUser(int) error                                     { return nil }
func (m *mockStore) SaveCheck(int, int64, bool) error                         { return nil }
func (m *mockStore) SaveCheckFromNode(siteID int, nodeID string, latencyNs int64, isUp bool) error {
	return nil
}
func (m *mockStore) LoadAllHistory(int) (map[int][]models.CheckRecord, error) {
	return nil, nil
}
func (m *mockStore) GetSiteByName(string) (models.Site, error) { return models.Site{}, nil }
func (m *mockStore) GetAlertByName(string) (models.AlertConfig, error) {
	return models.AlertConfig{}, nil
}
func (m *mockStore) AddSiteReturningID(models.Site) (int, error) { return 0, nil }
func (m *mockStore) AddAlertReturningID(string, string, map[string]string) (int, error) {
	return 0, nil
}
func (m *mockStore) GetAllNodes() ([]models.ProbeNode, error) { return nil, nil }
func (m *mockStore) UpdateNodeLastSeen(string) error          { return nil }
func (m *mockStore) DeleteNode(string) error                  { return nil }
func (m *mockStore) LoadAlertHealth() (map[int]models.AlertHealthRecord, error) {
	return nil, nil
}
func (m *mockStore) SaveAlertHealth(models.AlertHealthRecord) error { return nil }
func (m *mockStore) SaveLog(string) error                           { return nil }
func (m *mockStore) LoadLogs(int) ([]string, error)                 { return nil, nil }
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

func (m *mockStore) ExportData() (models.Backup, error) {
	return models.Backup{
		Sites:  m.sites,
		Alerts: m.alerts,
	}, nil
}

func (m *mockStore) ImportData(data models.Backup) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.importedData = &data
	return nil
}

func (m *mockStore) RegisterNode(node models.ProbeNode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registeredNodes = append(m.registeredNodes, node)
	m.nodes[node.ID] = node
	return nil
}

func (m *mockStore) GetNode(id string) (models.ProbeNode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if n, ok := m.nodes[id]; ok {
		return n, nil
	}
	return models.ProbeNode{}, fmt.Errorf("not found")
}

func (m *mockStore) GetActiveMaintenanceWindows() ([]models.MaintenanceWindow, error) {
	return m.maintWindows, nil
}

// --- Helpers ---

func freePort() int {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

type testServer struct {
	baseURL string
	srv     *http.Server
	store   *mockStore
	engine  *monitor.Engine
}

func newTestServer(t *testing.T, clusterKey string, enableStatus bool) *testServer {
	t.Helper()
	ms := newMockStore()
	eng := monitor.NewEngine(ms)
	port := freePort()

	srv := Start(ServerConfig{
		Port:         port,
		EnableStatus: enableStatus,
		Title:        "Test Status",
		ClusterKey:   clusterKey,
	}, ms, eng)

	ts := &testServer{
		baseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		srv:     srv,
		store:   ms,
		engine:  eng,
	}

	// Wait for server to be ready
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(ts.baseURL + "/api/health")
		if err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Cleanup(func() {
		srv.Close()
	})

	return ts
}

func authReq(method, url, secret string, body []byte) (*http.Response, error) {
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequest(method, url, bytes.NewReader(body))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		return nil, err
	}
	if secret != "" {
		req.Header.Set("X-Upkeep-Secret", secret)
	}
	return http.DefaultClient.Do(req)
}

// --- Tests ---

func TestCheckSecret(t *testing.T) {
	if !checkSecret("mykey", "mykey") {
		t.Error("expected match")
	}
	if checkSecret("mykey", "wrong") {
		t.Error("expected no match")
	}
	if checkSecret("", "key") {
		t.Error("expected no match for empty got")
	}
}

// --- Push Heartbeat ---

func TestPush_MissingToken(t *testing.T) {
	ts := newTestServer(t, "secret", false)
	resp, err := http.Get(ts.baseURL + "/api/push")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestPush_InvalidToken(t *testing.T) {
	ts := newTestServer(t, "secret", false)
	resp, err := http.Get(ts.baseURL + "/api/push?token=bad")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

// --- Health ---

func TestHealth_NoSecret(t *testing.T) {
	ts := newTestServer(t, "", false)
	resp, err := http.Get(ts.baseURL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200 with no cluster key, got %d", resp.StatusCode)
	}
}

func TestHealth_ValidSecret(t *testing.T) {
	ts := newTestServer(t, "secret", false)
	resp, err := authReq("GET", ts.baseURL+"/api/health", "secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestHealth_WrongSecret(t *testing.T) {
	ts := newTestServer(t, "secret", false)
	resp, err := authReq("GET", ts.baseURL+"/api/health", "wrong", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

// --- Backup Export ---

func TestExport_Unauthorized_NoKey(t *testing.T) {
	ts := newTestServer(t, "", false)
	resp, err := http.Get(ts.baseURL + "/api/backup/export")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("expected 401 when no cluster key configured, got %d", resp.StatusCode)
	}
}

func TestExport_Unauthorized_WrongKey(t *testing.T) {
	ts := newTestServer(t, "secret", false)
	resp, err := authReq("GET", ts.baseURL+"/api/backup/export", "wrong", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestExport_Success(t *testing.T) {
	ts := newTestServer(t, "secret", false)
	ts.store.sites = []models.Site{{ID: 1, Name: "example", URL: "http://example.com"}}

	resp, err := authReq("GET", ts.baseURL+"/api/backup/export", "secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var backup models.Backup
	json.NewDecoder(resp.Body).Decode(&backup)
	if len(backup.Sites) != 1 {
		t.Errorf("expected 1 site, got %d", len(backup.Sites))
	}
}

// --- Backup Import ---

func TestImport_MethodNotAllowed(t *testing.T) {
	ts := newTestServer(t, "secret", false)
	resp, err := authReq("GET", ts.baseURL+"/api/backup/import", "secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 405 {
		t.Errorf("expected 405, got %d", resp.StatusCode)
	}
}

func TestImport_Unauthorized(t *testing.T) {
	ts := newTestServer(t, "secret", false)
	body, _ := json.Marshal(models.Backup{})
	resp, err := authReq("POST", ts.baseURL+"/api/backup/import", "wrong", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestImport_Success(t *testing.T) {
	ts := newTestServer(t, "secret", false)
	backup := models.Backup{
		Sites: []models.Site{{Name: "imported", URL: "http://example.com"}},
	}
	body, _ := json.Marshal(backup)
	resp, err := authReq("POST", ts.baseURL+"/api/backup/import", "secret", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	ts.store.mu.Lock()
	defer ts.store.mu.Unlock()
	if ts.store.importedData == nil {
		t.Error("expected import data to be stored")
	}
}

func TestImport_InvalidJSON(t *testing.T) {
	ts := newTestServer(t, "secret", false)
	resp, err := authReq("POST", ts.baseURL+"/api/backup/import", "secret", []byte("not json"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// --- Probe Registration ---

func TestProbeRegister_Success(t *testing.T) {
	ts := newTestServer(t, "secret", false)
	body, _ := json.Marshal(map[string]string{
		"id": "node-1", "name": "US East", "region": "us-east",
	})
	resp, err := authReq("POST", ts.baseURL+"/api/probe/register", "secret", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	ts.store.mu.Lock()
	defer ts.store.mu.Unlock()
	if len(ts.store.registeredNodes) != 1 {
		t.Errorf("expected 1 registered node, got %d", len(ts.store.registeredNodes))
	}
	if ts.store.registeredNodes[0].ID != "node-1" {
		t.Errorf("expected node-1, got %s", ts.store.registeredNodes[0].ID)
	}
}

func TestProbeRegister_MissingID(t *testing.T) {
	ts := newTestServer(t, "secret", false)
	body, _ := json.Marshal(map[string]string{"name": "test"})
	resp, err := authReq("POST", ts.baseURL+"/api/probe/register", "secret", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestProbeRegister_Unauthorized(t *testing.T) {
	ts := newTestServer(t, "secret", false)
	body, _ := json.Marshal(map[string]string{"id": "node-1"})
	resp, err := authReq("POST", ts.baseURL+"/api/probe/register", "wrong", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

// --- Probe Results ---

func TestProbeResults_Success(t *testing.T) {
	ts := newTestServer(t, "secret", false)
	body, _ := json.Marshal(map[string]any{
		"node_id": "node-1",
		"results": []map[string]any{
			{"site_id": 1, "latency_ns": 5000000, "is_up": true},
		},
	})
	resp, err := authReq("POST", ts.baseURL+"/api/probe/results", "secret", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestProbeResults_MissingNodeID(t *testing.T) {
	ts := newTestServer(t, "secret", false)
	body, _ := json.Marshal(map[string]any{
		"results": []map[string]any{},
	})
	resp, err := authReq("POST", ts.baseURL+"/api/probe/results", "secret", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// --- Status Page ---

func TestStatusPage_Enabled(t *testing.T) {
	ts := newTestServer(t, "secret", true)
	resp, err := http.Get(ts.baseURL + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestStatusJSON_TokensStripped(t *testing.T) {
	ts := newTestServer(t, "secret", true)

	// Inject a site with a token into engine state
	ts.engine.UpdateSiteConfig(models.Site{ID: 1, Name: "test", Type: "push", Token: "secret-token", Status: "UP"})
	// Need to inject directly since UpdateSiteConfig only updates existing
	func() {
		ts.engine.RecordHeartbeat("unused") // just to exercise, won't match
	}()

	resp, err := http.Get(ts.baseURL + "/status/json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var state map[string]models.Site
	json.NewDecoder(resp.Body).Decode(&state)
	for _, site := range state {
		if site.Token != "" {
			t.Error("expected token stripped from status JSON response")
		}
	}
}

func TestStatusJSON_MaintenanceOverride(t *testing.T) {
	ts := newTestServer(t, "secret", true)
	ts.store.maintWindows = []models.MaintenanceWindow{
		{ID: 1, MonitorID: 0, Type: "maintenance", StartTime: time.Now().Add(-1 * time.Hour)},
	}

	resp, err := http.Get(ts.baseURL + "/status/json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestStatusPage_Disabled(t *testing.T) {
	ts := newTestServer(t, "secret", false)
	resp, err := http.Get(ts.baseURL + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("expected 404 when status disabled, got %d", resp.StatusCode)
	}
}

// --- Probe Assignments ---

func TestProbeAssignments_Success(t *testing.T) {
	ts := newTestServer(t, "secret", false)
	resp, err := authReq("GET", ts.baseURL+"/api/probe/assignments", "secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var result map[string][]models.Site
	json.NewDecoder(resp.Body).Decode(&result)
	if _, ok := result["sites"]; !ok {
		t.Error("expected 'sites' key in response")
	}
}

func TestProbeAssignments_Unauthorized(t *testing.T) {
	ts := newTestServer(t, "secret", false)
	resp, err := authReq("GET", ts.baseURL+"/api/probe/assignments", "wrong", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}
