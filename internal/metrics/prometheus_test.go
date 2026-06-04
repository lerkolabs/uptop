package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/monitor"
)

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
func (m *mockStore) LoadAllHistory(int) (map[int][]models.CheckRecord, error) {
	return nil, nil
}
func (m *mockStore) ExportData() (models.Backup, error)        { return models.Backup{}, nil }
func (m *mockStore) ImportData(models.Backup) error            { return nil }
func (m *mockStore) GetSiteByName(string) (models.Site, error) { return models.Site{}, nil }
func (m *mockStore) GetAlertByName(string) (models.AlertConfig, error) {
	return models.AlertConfig{}, nil
}
func (m *mockStore) AddSiteReturningID(models.Site) (int, error) { return 0, nil }
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
func (m *mockStore) SaveLog(string) error                           { return nil }
func (m *mockStore) LoadLogs(int) ([]string, error)                 { return nil, nil }
func (m *mockStore) GetActiveMaintenanceWindows() ([]models.MaintenanceWindow, error) {
	return nil, nil
}
func (m *mockStore) GetAllMaintenanceWindows(int) ([]models.MaintenanceWindow, error) {
	return nil, nil
}
func (m *mockStore) AddMaintenanceWindow(models.MaintenanceWindow) error    { return nil }
func (m *mockStore) EndMaintenanceWindow(int) error                         { return nil }
func (m *mockStore) DeleteMaintenanceWindow(int) error                      { return nil }
func (m *mockStore) IsMonitorInMaintenance(int) (bool, error)               { return false, nil }
func (m *mockStore) GetPreference(string) (string, error)                   { return "", nil }
func (m *mockStore) SetPreference(string, string) error                     { return nil }
func (m *mockStore) SaveStateChange(int, string, string, string) error      { return nil }
func (m *mockStore) GetStateChanges(int, int) ([]models.StateChange, error) { return nil, nil }
func (m *mockStore) GetStateChangesSince(int, time.Time) ([]models.StateChange, error) {
	return nil, nil
}
func (m *mockStore) Close() error { return nil }

func TestMetricsHandler(t *testing.T) {
	ms := &mockStore{
		sites: []models.Site{
			{ID: 1, Name: "Example", URL: "https://example.com", Type: "http", Interval: 30},
			{ID: 2, Name: "DNS Check", Type: "dns", Interval: 60},
		},
	}
	eng := monitor.NewEngine(ms)
	ctx, cancel := context.WithCancel(context.Background())
	eng.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	rec := httptest.NewRecorder()
	Handler(eng)(rec, httptest.NewRequest("GET", "/metrics", nil))
	cancel()

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("expected text/plain content type, got %q", ct)
	}

	expected := []string{
		"# HELP uptop_monitor_up",
		"# TYPE uptop_monitor_up gauge",
		`uptop_monitor_up{id="1",name="Example",type="http"}`,
		`uptop_monitor_up{id="2",name="DNS Check",type="dns"}`,
		"# HELP uptop_monitor_latency_seconds",
		"# HELP uptop_monitor_paused",
		"# HELP uptop_monitor_checks_total",
	}
	for _, s := range expected {
		if !strings.Contains(body, s) {
			t.Errorf("missing expected line: %s", s)
		}
	}
}

func TestEscapeLabelValue(t *testing.T) {
	cases := []struct{ in, want string }{
		{`simple`, `simple`},
		{`has "quotes"`, `has \"quotes\"`},
		{"has\nnewline", `has\nnewline`},
		{`back\slash`, `back\\slash`},
	}
	for _, tc := range cases {
		got := escapeLabelValue(tc.in)
		if got != tc.want {
			t.Errorf("escapeLabelValue(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
