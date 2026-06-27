package storetest

import (
	"context"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

// BaseMock implements store.Store with no-op defaults. Embed it in test-specific
// mocks and override only the methods you need via the exported Func fields or
// by shadowing the method on the embedding struct.
type BaseMock struct {
	GetSitesFunc                    func(ctx context.Context) ([]models.SiteConfig, error)
	AddSiteFunc                     func(ctx context.Context, site models.SiteConfig) error
	UpdateSiteFunc                  func(ctx context.Context, site models.SiteConfig) error
	GetAllAlertsFunc                func(ctx context.Context) ([]models.AlertConfig, error)
	GetAlertFunc                    func(ctx context.Context, id int) (models.AlertConfig, error)
	GetAllUsersFunc                 func(ctx context.Context) ([]models.User, error)
	GetAllNodesFunc                 func(ctx context.Context) ([]models.ProbeNode, error)
	GetActiveMaintenanceWindowsFunc func(ctx context.Context) ([]models.MaintenanceWindow, error)
	GetAllMaintenanceWindowsFunc    func(ctx context.Context, limit int) ([]models.MaintenanceWindow, error)
	IsMonitorInMaintenanceFunc      func(ctx context.Context, id int) (bool, error)
	LoadAlertHealthFunc             func(ctx context.Context) (map[int]models.AlertHealthRecord, error)
	LoadAllHistoryFunc              func(ctx context.Context, limit int) (map[int][]models.CheckRecord, error)
	SaveCheckFunc                   func(ctx context.Context, siteID int, latencyNs int64, isUp bool) error
	SaveCheckFromNodeFunc           func(ctx context.Context, siteID int, nodeID string, latencyNs int64, isUp bool) error
	SaveLogFunc                     func(ctx context.Context, message string) error
	SaveStateChangeFunc             func(ctx context.Context, siteID int, from, to, reason string) error
	SaveAlertHealthFunc             func(ctx context.Context, h models.AlertHealthRecord) error
	GetStateChangesFunc             func(ctx context.Context, siteID, limit int) ([]models.StateChange, error)
	GetStateChangesSinceFunc        func(ctx context.Context, siteID int, since time.Time) ([]models.StateChange, error)
	ExportDataFunc                  func(ctx context.Context) (models.Backup, error)
	ImportDataFunc                  func(ctx context.Context, data models.Backup) error
	RegisterNodeFunc                func(ctx context.Context, node models.ProbeNode) error
	GetNodeFunc                     func(ctx context.Context, id string) (models.ProbeNode, error)
	GetPreferenceFunc               func(ctx context.Context, key string) (string, error)
	SetPreferenceFunc               func(ctx context.Context, key, value string) error
}

func (m *BaseMock) Init(_ context.Context) error { return nil }
func (m *BaseMock) Close() error                 { return nil }

func (m *BaseMock) GetSites(ctx context.Context) ([]models.SiteConfig, error) {
	if m.GetSitesFunc != nil {
		return m.GetSitesFunc(ctx)
	}
	return nil, nil
}

func (m *BaseMock) AddSite(ctx context.Context, site models.SiteConfig) error {
	if m.AddSiteFunc != nil {
		return m.AddSiteFunc(ctx, site)
	}
	return nil
}

func (m *BaseMock) UpdateSite(ctx context.Context, site models.SiteConfig) error {
	if m.UpdateSiteFunc != nil {
		return m.UpdateSiteFunc(ctx, site)
	}
	return nil
}

func (m *BaseMock) UpdateSitePaused(_ context.Context, _ int, _ bool) error { return nil }

func (m *BaseMock) DeleteSite(_ context.Context, _ int) error { return nil }

func (m *BaseMock) GetAllAlerts(ctx context.Context) ([]models.AlertConfig, error) {
	if m.GetAllAlertsFunc != nil {
		return m.GetAllAlertsFunc(ctx)
	}
	return nil, nil
}

func (m *BaseMock) GetAlert(ctx context.Context, id int) (models.AlertConfig, error) {
	if m.GetAlertFunc != nil {
		return m.GetAlertFunc(ctx, id)
	}
	return models.AlertConfig{}, nil
}

func (m *BaseMock) AddAlert(_ context.Context, _ string, _ string, _ map[string]string) error {
	return nil
}

func (m *BaseMock) UpdateAlert(_ context.Context, _ int, _ string, _ string, _ map[string]string) error {
	return nil
}

func (m *BaseMock) DeleteAlert(_ context.Context, _ int) error { return nil }

func (m *BaseMock) GetSiteByName(_ context.Context, _ string) (models.SiteConfig, error) {
	return models.SiteConfig{}, nil
}

func (m *BaseMock) GetAlertByName(_ context.Context, _ string) (models.AlertConfig, error) {
	return models.AlertConfig{}, nil
}

func (m *BaseMock) AddSiteReturningID(_ context.Context, _ models.SiteConfig) (int, error) {
	return 0, nil
}

func (m *BaseMock) AddAlertReturningID(_ context.Context, _ string, _ string, _ map[string]string) (int, error) {
	return 0, nil
}

func (m *BaseMock) GetAllUsers(ctx context.Context) ([]models.User, error) {
	if m.GetAllUsersFunc != nil {
		return m.GetAllUsersFunc(ctx)
	}
	return nil, nil
}

func (m *BaseMock) AddUser(_ context.Context, _ string, _ string, _ string) error { return nil }

func (m *BaseMock) UpdateUser(_ context.Context, _ int, _ string, _ string, _ string) error {
	return nil
}

func (m *BaseMock) DeleteUser(_ context.Context, _ int) error { return nil }

func (m *BaseMock) SaveCheck(ctx context.Context, siteID int, latencyNs int64, isUp bool) error {
	if m.SaveCheckFunc != nil {
		return m.SaveCheckFunc(ctx, siteID, latencyNs, isUp)
	}
	return nil
}

func (m *BaseMock) SaveCheckFromNode(ctx context.Context, siteID int, nodeID string, latencyNs int64, isUp bool) error {
	if m.SaveCheckFromNodeFunc != nil {
		return m.SaveCheckFromNodeFunc(ctx, siteID, nodeID, latencyNs, isUp)
	}
	return nil
}

func (m *BaseMock) LoadAllHistory(ctx context.Context, limit int) (map[int][]models.CheckRecord, error) {
	if m.LoadAllHistoryFunc != nil {
		return m.LoadAllHistoryFunc(ctx, limit)
	}
	return nil, nil
}

func (m *BaseMock) PruneCheckHistory(_ context.Context) error { return nil }

func (m *BaseMock) SaveStateChange(ctx context.Context, siteID int, from, to, reason string) error {
	if m.SaveStateChangeFunc != nil {
		return m.SaveStateChangeFunc(ctx, siteID, from, to, reason)
	}
	return nil
}

func (m *BaseMock) GetStateChanges(ctx context.Context, siteID, limit int) ([]models.StateChange, error) {
	if m.GetStateChangesFunc != nil {
		return m.GetStateChangesFunc(ctx, siteID, limit)
	}
	return nil, nil
}

func (m *BaseMock) GetStateChangesSince(ctx context.Context, siteID int, since time.Time) ([]models.StateChange, error) {
	if m.GetStateChangesSinceFunc != nil {
		return m.GetStateChangesSinceFunc(ctx, siteID, since)
	}
	return nil, nil
}

func (m *BaseMock) PruneStateChanges(_ context.Context) error { return nil }

func (m *BaseMock) RegisterNode(ctx context.Context, node models.ProbeNode) error {
	if m.RegisterNodeFunc != nil {
		return m.RegisterNodeFunc(ctx, node)
	}
	return nil
}

func (m *BaseMock) GetNode(ctx context.Context, id string) (models.ProbeNode, error) {
	if m.GetNodeFunc != nil {
		return m.GetNodeFunc(ctx, id)
	}
	return models.ProbeNode{}, nil
}

func (m *BaseMock) GetAllNodes(ctx context.Context) ([]models.ProbeNode, error) {
	if m.GetAllNodesFunc != nil {
		return m.GetAllNodesFunc(ctx)
	}
	return nil, nil
}

func (m *BaseMock) UpdateNodeLastSeen(_ context.Context, _ string) error { return nil }
func (m *BaseMock) DeleteNode(_ context.Context, _ string) error         { return nil }

func (m *BaseMock) LoadAlertHealth(ctx context.Context) (map[int]models.AlertHealthRecord, error) {
	if m.LoadAlertHealthFunc != nil {
		return m.LoadAlertHealthFunc(ctx)
	}
	return nil, nil
}

func (m *BaseMock) SaveAlertHealth(ctx context.Context, h models.AlertHealthRecord) error {
	if m.SaveAlertHealthFunc != nil {
		return m.SaveAlertHealthFunc(ctx, h)
	}
	return nil
}

func (m *BaseMock) SaveLog(ctx context.Context, message string) error {
	if m.SaveLogFunc != nil {
		return m.SaveLogFunc(ctx, message)
	}
	return nil
}

func (m *BaseMock) LoadLogs(_ context.Context, _ int) ([]models.LogEntry, error) { return nil, nil }
func (m *BaseMock) PruneLogs(_ context.Context) error                            { return nil }

func (m *BaseMock) GetActiveMaintenanceWindows(ctx context.Context) ([]models.MaintenanceWindow, error) {
	if m.GetActiveMaintenanceWindowsFunc != nil {
		return m.GetActiveMaintenanceWindowsFunc(ctx)
	}
	return nil, nil
}

func (m *BaseMock) GetAllMaintenanceWindows(ctx context.Context, limit int) ([]models.MaintenanceWindow, error) {
	if m.GetAllMaintenanceWindowsFunc != nil {
		return m.GetAllMaintenanceWindowsFunc(ctx, limit)
	}
	return nil, nil
}

func (m *BaseMock) GetOverlappingMaintenanceWindows(_ context.Context, _ int, _, _ time.Time) ([]models.MaintenanceWindow, error) {
	return nil, nil
}

func (m *BaseMock) AddMaintenanceWindow(_ context.Context, _ models.MaintenanceWindow) error {
	return nil
}

func (m *BaseMock) EndMaintenanceWindow(_ context.Context, _ int) error    { return nil }
func (m *BaseMock) DeleteMaintenanceWindow(_ context.Context, _ int) error { return nil }

func (m *BaseMock) PruneExpiredMaintenanceWindows(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}

func (m *BaseMock) IsMonitorInMaintenance(ctx context.Context, id int) (bool, error) {
	if m.IsMonitorInMaintenanceFunc != nil {
		return m.IsMonitorInMaintenanceFunc(ctx, id)
	}
	return false, nil
}

func (m *BaseMock) GetPreference(ctx context.Context, key string) (string, error) {
	if m.GetPreferenceFunc != nil {
		return m.GetPreferenceFunc(ctx, key)
	}
	return "", nil
}

func (m *BaseMock) SetPreference(ctx context.Context, key, value string) error {
	if m.SetPreferenceFunc != nil {
		return m.SetPreferenceFunc(ctx, key, value)
	}
	return nil
}

func (m *BaseMock) ExportData(ctx context.Context) (models.Backup, error) {
	if m.ExportDataFunc != nil {
		return m.ExportDataFunc(ctx)
	}
	return models.Backup{}, nil
}

func (m *BaseMock) ImportData(ctx context.Context, data models.Backup) error {
	if m.ImportDataFunc != nil {
		return m.ImportDataFunc(ctx, data)
	}
	return nil
}
