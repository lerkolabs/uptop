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

func (m *mockStore) Init(_ context.Context) error                                 { return nil }
func (m *mockStore) GetSites(_ context.Context) ([]models.Site, error)            { return m.sites, nil }
func (m *mockStore) AddSite(_ context.Context, _ models.Site) error               { return nil }
func (m *mockStore) UpdateSite(_ context.Context, _ models.Site) error            { return nil }
func (m *mockStore) UpdateSitePaused(_ context.Context, _ int, _ bool) error      { return nil }
func (m *mockStore) DeleteSite(_ context.Context, _ int) error                    { return nil }
func (m *mockStore) GetAllAlerts(_ context.Context) ([]models.AlertConfig, error) { return nil, nil }
func (m *mockStore) GetAlert(_ context.Context, _ int) (models.AlertConfig, error) {
	return models.AlertConfig{}, nil
}
func (m *mockStore) AddAlert(_ context.Context, _ string, _ string, _ map[string]string) error {
	return nil
}
func (m *mockStore) UpdateAlert(_ context.Context, _ int, _ string, _ string, _ map[string]string) error {
	return nil
}
func (m *mockStore) DeleteAlert(_ context.Context, _ int) error                    { return nil }
func (m *mockStore) GetAllUsers(_ context.Context) ([]models.User, error)          { return nil, nil }
func (m *mockStore) AddUser(_ context.Context, _ string, _ string, _ string) error { return nil }
func (m *mockStore) UpdateUser(_ context.Context, _ int, _ string, _ string, _ string) error {
	return nil
}
func (m *mockStore) DeleteUser(_ context.Context, _ int) error                 { return nil }
func (m *mockStore) SaveCheck(_ context.Context, _ int, _ int64, _ bool) error { return nil }
func (m *mockStore) LoadAllHistory(_ context.Context, _ int) (map[int][]models.CheckRecord, error) {
	return nil, nil
}
func (m *mockStore) ExportData(_ context.Context) (models.Backup, error) { return models.Backup{}, nil }
func (m *mockStore) ImportData(_ context.Context, _ models.Backup) error { return nil }
func (m *mockStore) GetSiteByName(_ context.Context, _ string) (models.Site, error) {
	return models.Site{}, nil
}
func (m *mockStore) GetAlertByName(_ context.Context, _ string) (models.AlertConfig, error) {
	return models.AlertConfig{}, nil
}
func (m *mockStore) AddSiteReturningID(_ context.Context, _ models.Site) (int, error) { return 0, nil }
func (m *mockStore) AddAlertReturningID(_ context.Context, _ string, _ string, _ map[string]string) (int, error) {
	return 0, nil
}
func (m *mockStore) SaveCheckFromNode(_ context.Context, _ int, _ string, _ int64, _ bool) error {
	return nil
}
func (m *mockStore) RegisterNode(_ context.Context, _ models.ProbeNode) error { return nil }
func (m *mockStore) GetNode(_ context.Context, _ string) (models.ProbeNode, error) {
	return models.ProbeNode{}, nil
}
func (m *mockStore) GetAllNodes(_ context.Context) ([]models.ProbeNode, error) { return nil, nil }
func (m *mockStore) UpdateNodeLastSeen(_ context.Context, _ string) error      { return nil }
func (m *mockStore) DeleteNode(_ context.Context, _ string) error              { return nil }
func (m *mockStore) LoadAlertHealth(_ context.Context) (map[int]models.AlertHealthRecord, error) {
	return nil, nil
}
func (m *mockStore) SaveAlertHealth(_ context.Context, _ models.AlertHealthRecord) error { return nil }
func (m *mockStore) SaveLog(_ context.Context, _ string) error                           { return nil }
func (m *mockStore) PruneLogs(_ context.Context) error                                   { return nil }
func (m *mockStore) PruneCheckHistory(_ context.Context) error                           { return nil }
func (m *mockStore) PruneStateChanges(_ context.Context) error                           { return nil }
func (m *mockStore) LoadLogs(_ context.Context, _ int) ([]string, error)                 { return nil, nil }
func (m *mockStore) GetActiveMaintenanceWindows(_ context.Context) ([]models.MaintenanceWindow, error) {
	return nil, nil
}
func (m *mockStore) GetAllMaintenanceWindows(_ context.Context, _ int) ([]models.MaintenanceWindow, error) {
	return nil, nil
}
func (m *mockStore) AddMaintenanceWindow(_ context.Context, _ models.MaintenanceWindow) error {
	return nil
}
func (m *mockStore) EndMaintenanceWindow(_ context.Context, _ int) error    { return nil }
func (m *mockStore) DeleteMaintenanceWindow(_ context.Context, _ int) error { return nil }
func (m *mockStore) PruneExpiredMaintenanceWindows(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}
func (m *mockStore) IsMonitorInMaintenance(_ context.Context, _ int) (bool, error) { return false, nil }
func (m *mockStore) GetPreference(_ context.Context, _ string) (string, error)     { return "", nil }
func (m *mockStore) SetPreference(_ context.Context, _ string, _ string) error     { return nil }
func (m *mockStore) SaveStateChange(_ context.Context, _ int, _ string, _ string, _ string) error {
	return nil
}
func (m *mockStore) GetStateChanges(_ context.Context, _ int, _ int) ([]models.StateChange, error) {
	return nil, nil
}
func (m *mockStore) GetStateChangesSince(_ context.Context, _ int, _ time.Time) ([]models.StateChange, error) {
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
