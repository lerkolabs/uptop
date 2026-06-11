package tui

import (
	"testing"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/monitor"
	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
)

// --- minimal Store mock for TUI data-flow tests ---

type tuiMockStore struct {
	alerts           []models.AlertConfig
	users            []models.User
	nodes            []models.ProbeNode
	maint            []models.MaintenanceWindow
	stateChanges     []models.StateChange
	stateChangeCalls int // counts GetStateChanges hits (to prove View does no IO)
}

func (m *tuiMockStore) GetAllAlerts() ([]models.AlertConfig, error) { return m.alerts, nil }
func (m *tuiMockStore) GetAllUsers() ([]models.User, error)         { return m.users, nil }
func (m *tuiMockStore) GetAllNodes() ([]models.ProbeNode, error)    { return m.nodes, nil }
func (m *tuiMockStore) GetStateChanges(int, int) ([]models.StateChange, error) {
	m.stateChangeCalls++
	return m.stateChanges, nil
}
func (m *tuiMockStore) GetAllMaintenanceWindows(int) ([]models.MaintenanceWindow, error) {
	return m.maint, nil
}

func (m *tuiMockStore) Init() error                                              { return nil }
func (m *tuiMockStore) GetSites() ([]models.Site, error)                         { return nil, nil }
func (m *tuiMockStore) AddSite(models.Site) error                                { return nil }
func (m *tuiMockStore) UpdateSite(models.Site) error                             { return nil }
func (m *tuiMockStore) UpdateSitePaused(int, bool) error                         { return nil }
func (m *tuiMockStore) DeleteSite(int) error                                     { return nil }
func (m *tuiMockStore) GetAlert(int) (models.AlertConfig, error)                 { return models.AlertConfig{}, nil }
func (m *tuiMockStore) AddAlert(string, string, map[string]string) error         { return nil }
func (m *tuiMockStore) UpdateAlert(int, string, string, map[string]string) error { return nil }
func (m *tuiMockStore) DeleteAlert(int) error                                    { return nil }
func (m *tuiMockStore) GetSiteByName(string) (models.Site, error)                { return models.Site{}, nil }
func (m *tuiMockStore) GetAlertByName(string) (models.AlertConfig, error) {
	return models.AlertConfig{}, nil
}
func (m *tuiMockStore) AddSiteReturningID(models.Site) (int, error) { return 0, nil }
func (m *tuiMockStore) AddAlertReturningID(string, string, map[string]string) (int, error) {
	return 0, nil
}
func (m *tuiMockStore) AddUser(string, string, string) error         { return nil }
func (m *tuiMockStore) UpdateUser(int, string, string, string) error { return nil }
func (m *tuiMockStore) DeleteUser(int) error                         { return nil }
func (m *tuiMockStore) SaveCheck(int, int64, bool) error             { return nil }
func (m *tuiMockStore) SaveCheckFromNode(int, string, int64, bool) error {
	return nil
}
func (m *tuiMockStore) LoadAllHistory(int) (map[int][]models.CheckRecord, error) {
	return nil, nil
}
func (m *tuiMockStore) PruneCheckHistory() error                          { return nil }
func (m *tuiMockStore) SaveStateChange(int, string, string, string) error { return nil }
func (m *tuiMockStore) GetStateChangesSince(int, time.Time) ([]models.StateChange, error) {
	return nil, nil
}
func (m *tuiMockStore) PruneStateChanges() error                 { return nil }
func (m *tuiMockStore) RegisterNode(models.ProbeNode) error      { return nil }
func (m *tuiMockStore) GetNode(string) (models.ProbeNode, error) { return models.ProbeNode{}, nil }
func (m *tuiMockStore) UpdateNodeLastSeen(string) error          { return nil }
func (m *tuiMockStore) DeleteNode(string) error                  { return nil }
func (m *tuiMockStore) LoadAlertHealth() (map[int]models.AlertHealthRecord, error) {
	return nil, nil
}
func (m *tuiMockStore) SaveAlertHealth(models.AlertHealthRecord) error { return nil }
func (m *tuiMockStore) SaveLog(string) error                           { return nil }
func (m *tuiMockStore) LoadLogs(int) ([]string, error)                 { return nil, nil }
func (m *tuiMockStore) PruneLogs() error                               { return nil }
func (m *tuiMockStore) GetActiveMaintenanceWindows() ([]models.MaintenanceWindow, error) {
	return nil, nil
}
func (m *tuiMockStore) AddMaintenanceWindow(models.MaintenanceWindow) error { return nil }
func (m *tuiMockStore) EndMaintenanceWindow(int) error                      { return nil }
func (m *tuiMockStore) DeleteMaintenanceWindow(int) error                   { return nil }
func (m *tuiMockStore) PruneExpiredMaintenanceWindows(time.Duration) (int64, error) {
	return 0, nil
}
func (m *tuiMockStore) IsMonitorInMaintenance(int) (bool, error) { return false, nil }
func (m *tuiMockStore) GetPreference(string) (string, error)     { return "", nil }
func (m *tuiMockStore) SetPreference(string, string) error       { return nil }
func (m *tuiMockStore) ExportData() (models.Backup, error)       { return models.Backup{}, nil }
func (m *tuiMockStore) ImportData(models.Backup) error           { return nil }
func (m *tuiMockStore) Close() error                             { return nil }

func newTestModel(ms *tuiMockStore) Model {
	return Model{
		store:               ms,
		engine:              monitor.NewEngine(ms),
		isAdmin:             true,
		zones:               zone.New(),
		detailChangesSiteID: -1,
		theme:               themeFlexokiDark,
		st:                  newStyles(themeFlexokiDark),
	}
}

// --- Tests ---

func TestLoadTabDataCmd_ReturnsRows(t *testing.T) {
	ms := &tuiMockStore{
		alerts: []models.AlertConfig{{ID: 1, Name: "a"}},
		nodes:  []models.ProbeNode{{ID: "n1"}},
		users:  []models.User{{Username: "u"}},
		maint:  []models.MaintenanceWindow{{ID: 7}},
	}
	m := newTestModel(ms)

	msg := m.loadTabDataCmd()()
	td, ok := msg.(tabDataMsg)
	if !ok {
		t.Fatalf("expected tabDataMsg, got %T", msg)
	}
	if len(td.alerts) != 1 || len(td.nodes) != 1 || len(td.users) != 1 || len(td.maint) != 1 {
		t.Errorf("unexpected counts: %+v", td)
	}
	if td.err != nil {
		t.Errorf("unexpected err: %v", td.err)
	}
}

func TestHandleTabData_PopulatesModel(t *testing.T) {
	m := newTestModel(&tuiMockStore{})
	msg := tabDataMsg{
		alerts: []models.AlertConfig{{ID: 1}},
		nodes:  []models.ProbeNode{{ID: "n"}},
		users:  []models.User{{Username: "u"}},
		maint:  []models.MaintenanceWindow{{ID: 2}},
	}
	updated, _ := m.handleTabData(msg)
	got := updated.(*Model)
	if len(got.alerts) != 1 || len(got.nodes) != 1 || len(got.users) != 1 || len(got.maintenanceWindows) != 1 {
		t.Errorf("model not populated: alerts=%d nodes=%d users=%d maint=%d",
			len(got.alerts), len(got.nodes), len(got.users), len(got.maintenanceWindows))
	}
}

func TestHandleTabData_ErrorKeepsPreviousData(t *testing.T) {
	m := newTestModel(&tuiMockStore{})
	m.alerts = []models.AlertConfig{{ID: 99}} // pre-existing data
	updated, _ := m.handleTabData(tabDataMsg{err: errSentinel})
	got := updated.(*Model)
	if len(got.alerts) != 1 || got.alerts[0].ID != 99 {
		t.Error("transient error wiped previous tab data")
	}
}

var errSentinel = &stubErr{}

type stubErr struct{}

func (*stubErr) Error() string { return "boom" }

func TestDetailLoad_CachesAndViewDoesNoIO(t *testing.T) {
	ms := &tuiMockStore{stateChanges: []models.StateChange{{FromStatus: "UP", ToStatus: "DOWN"}}}
	m := newTestModel(ms)
	m.sites = []models.Site{{ID: 1, Name: "site", Status: "DOWN"}}
	m.cursor = 0
	m.state = stateDetail
	m.termWidth = 120
	m.termHeight = 40

	// Entering detail dispatches the load Cmd.
	cmd := m.loadDetailCmd(1)
	if cmd == nil {
		t.Fatal("loadDetailCmd returned nil")
	}
	msg := cmd()
	dd, ok := msg.(detailDataMsg)
	if !ok || dd.siteID != 1 || len(dd.changes) != 1 {
		t.Fatalf("unexpected detailDataMsg: %+v", msg)
	}
	if ms.stateChangeCalls != 1 {
		t.Fatalf("expected exactly 1 store hit from the load Cmd, got %d", ms.stateChangeCalls)
	}

	// Apply the msg through Update (caches into the model).
	updated, _ := m.Update(dd)
	m = updated.(Model)
	if m.detailChangesSiteID != 1 || len(m.detailChanges) != 1 {
		t.Fatalf("detail changes not cached: id=%d n=%d", m.detailChangesSiteID, len(m.detailChanges))
	}

	// Render the detail panel several times — it must read the cache, not the DB.
	for i := 0; i < 3; i++ {
		_ = m.viewDetailPanel()
	}
	if ms.stateChangeCalls != 1 {
		t.Errorf("View performed DB IO: store hit %d times (want 1, from the Cmd only)", ms.stateChangeCalls)
	}
}

func TestHandleTick_ThrottlesTabLoad(t *testing.T) {
	m := newTestModel(&tuiMockStore{})
	mp := &m

	t0 := time.Unix(1_000_000, 0)
	mp.handleTick(t0)
	if !mp.lastTabLoad.Equal(t0) {
		t.Fatalf("first tick should dispatch + stamp lastTabLoad=%v, got %v", t0, mp.lastTabLoad)
	}

	// Within the TTL → no new dispatch, stamp unchanged.
	mp.handleTick(t0.Add(time.Second))
	if !mp.lastTabLoad.Equal(t0) {
		t.Errorf("tick within TTL should not re-dispatch; lastTabLoad=%v", mp.lastTabLoad)
	}

	// Past the TTL → dispatch again.
	t2 := t0.Add(tabRefreshTTL + time.Second)
	mp.handleTick(t2)
	if !mp.lastTabLoad.Equal(t2) {
		t.Errorf("tick past TTL should re-dispatch; lastTabLoad=%v want %v", mp.lastTabLoad, t2)
	}
}

// keyMsg builds a plain-rune key message ("h", "s", ...).
func keyMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestHandleTabData_DropsStaleSeq(t *testing.T) {
	m := newTestModel(&tuiMockStore{})
	mp := &m
	_ = mp.loadTabDataCmd() // seq 1 (superseded)
	_ = mp.loadTabDataCmd() // seq 2 (newest)

	updated, _ := mp.handleTabData(tabDataMsg{seq: 1, alerts: []models.AlertConfig{{ID: 1}}})
	if got := updated.(*Model); len(got.alerts) != 0 {
		t.Error("stale tab-data reply was applied over a newer in-flight load")
	}

	updated, _ = mp.handleTabData(tabDataMsg{seq: 2, alerts: []models.AlertConfig{{ID: 2}}})
	if got := updated.(*Model); len(got.alerts) != 1 || got.alerts[0].ID != 2 {
		t.Error("fresh tab-data reply was not applied")
	}
}

func TestHistoryKey_LoadsOffUIGoroutine(t *testing.T) {
	ms := &tuiMockStore{stateChanges: []models.StateChange{{FromStatus: "UP", ToStatus: "DOWN"}}}
	m := newTestModel(ms)
	m.sites = []models.Site{{ID: 7, Name: "site"}}
	m.state = stateDetail
	m.termWidth, m.termHeight = 120, 40

	updated, cmd := (&m).handleDetailKey(keyMsg("h"))
	if ms.stateChangeCalls != 0 {
		t.Fatal("history keypress hit the store synchronously in Update")
	}
	got := updated.(*Model)
	if got.state != stateHistory || got.historySiteID != 7 {
		t.Fatalf("history view not opened: state=%v siteID=%d", got.state, got.historySiteID)
	}
	if cmd == nil {
		t.Fatal("expected a history load Cmd")
	}

	msg := cmd()
	hd, ok := msg.(historyDataMsg)
	if !ok || hd.siteID != 7 || len(hd.changes) != 1 {
		t.Fatalf("unexpected historyDataMsg: %+v", msg)
	}

	folded, _ := got.Update(hd)
	m2 := folded.(Model)
	if len(m2.historyChanges) != 1 {
		t.Fatal("history reply not folded into the model")
	}

	// A reply for a previously opened site must not clobber the current one.
	m2.historySiteID = 9
	stale, _ := m2.Update(historyDataMsg{siteID: 7, changes: nil})
	if m3 := stale.(Model); len(m3.historyChanges) != 1 {
		t.Error("stale history reply overwrote the current view")
	}
}

func TestSLAData_DropsStaleReply(t *testing.T) {
	m := newTestModel(&tuiMockStore{})
	m.termWidth, m.termHeight = 120, 40
	m.sites = []models.Site{{ID: 3, Status: "UP"}}

	if cmd := (&m).openSLAView(m.sites[0]); cmd == nil {
		t.Fatal("openSLAView should return a load Cmd")
	}

	// Reply for a different period than currently selected → dropped.
	// (slaDataMsg routes through a pointer-receiver handler, so Update
	// returns *Model on this path.)
	updated, _ := m.Update(slaDataMsg{siteID: 3, periodIdx: 0})
	if mm := updated.(*Model); mm.slaDailyBreakdown != nil {
		t.Error("stale SLA reply (old period) was applied")
	}

	// Matching reply → report computed.
	updated, _ = updated.(*Model).Update(slaDataMsg{siteID: 3, periodIdx: m.slaPeriodIdx})
	if mm := updated.(*Model); mm.slaDailyBreakdown == nil {
		t.Error("matching SLA reply was not applied")
	}
}

func TestDetailRefreshCmd_OnlyWhileDetailOpen(t *testing.T) {
	ms := &tuiMockStore{stateChanges: []models.StateChange{{FromStatus: "UP", ToStatus: "DOWN"}}}
	m := newTestModel(ms)
	m.sites = []models.Site{{ID: 5, Name: "site"}}

	m.state = stateDashboard
	if (&m).detailRefreshCmd() != nil {
		t.Error("refresh Cmd issued outside the detail view")
	}

	m.state = stateDetail
	cmd := (&m).detailRefreshCmd()
	if cmd == nil {
		t.Fatal("open detail panel should refresh on the tab-data cadence")
	}
	dd, ok := cmd().(detailDataMsg)
	if !ok || dd.siteID != 5 || len(dd.changes) != 1 {
		t.Fatalf("unexpected detail refresh reply: %+v", dd)
	}

	m.cursor = 7 // cursor out of range → no refresh, no panic
	if (&m).detailRefreshCmd() != nil {
		t.Error("refresh Cmd issued for an out-of-range cursor")
	}
}
