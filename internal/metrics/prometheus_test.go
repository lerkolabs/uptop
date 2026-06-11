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
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/store/storetest"
)

type mockStore struct {
	storetest.BaseMock
	sites []models.Site
}

func (m *mockStore) GetSites(_ context.Context) ([]models.Site, error) {
	return m.sites, nil
}

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
