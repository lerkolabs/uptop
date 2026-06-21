package store

import (
	"context"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

type Store interface {
	Init(ctx context.Context) error

	// Sites
	GetSites(ctx context.Context) ([]models.SiteConfig, error)
	AddSite(ctx context.Context, site models.SiteConfig) error
	UpdateSite(ctx context.Context, site models.SiteConfig) error
	UpdateSitePaused(ctx context.Context, id int, paused bool) error
	DeleteSite(ctx context.Context, id int) error

	// Alerts
	GetAllAlerts(ctx context.Context) ([]models.AlertConfig, error)
	GetAlert(ctx context.Context, id int) (models.AlertConfig, error)
	AddAlert(ctx context.Context, name, aType string, settings map[string]string) error
	UpdateAlert(ctx context.Context, id int, name, aType string, settings map[string]string) error
	DeleteAlert(ctx context.Context, id int) error

	// Declarative config support
	GetSiteByName(ctx context.Context, name string) (models.SiteConfig, error)
	GetAlertByName(ctx context.Context, name string) (models.AlertConfig, error)
	AddSiteReturningID(ctx context.Context, site models.SiteConfig) (int, error)
	AddAlertReturningID(ctx context.Context, name, aType string, settings map[string]string) (int, error)

	// Users
	GetAllUsers(ctx context.Context) ([]models.User, error)
	AddUser(ctx context.Context, username, publicKey, role string) error
	UpdateUser(ctx context.Context, id int, username, publicKey, role string) error
	DeleteUser(ctx context.Context, id int) error

	// History
	SaveCheck(ctx context.Context, siteID int, latencyNs int64, isUp bool) error
	SaveCheckFromNode(ctx context.Context, siteID int, nodeID string, latencyNs int64, isUp bool) error
	LoadAllHistory(ctx context.Context, limit int) (map[int][]models.CheckRecord, error)
	PruneCheckHistory(ctx context.Context) error

	// State Changes
	SaveStateChange(ctx context.Context, siteID int, fromStatus, toStatus, errorReason string) error
	GetStateChanges(ctx context.Context, siteID int, limit int) ([]models.StateChange, error)
	GetStateChangesSince(ctx context.Context, siteID int, since time.Time) ([]models.StateChange, error)
	PruneStateChanges(ctx context.Context) error

	// Nodes
	RegisterNode(ctx context.Context, node models.ProbeNode) error
	GetNode(ctx context.Context, id string) (models.ProbeNode, error)
	GetAllNodes(ctx context.Context) ([]models.ProbeNode, error)
	UpdateNodeLastSeen(ctx context.Context, id string) error
	DeleteNode(ctx context.Context, id string) error

	// Alert Health
	LoadAlertHealth(ctx context.Context) (map[int]models.AlertHealthRecord, error)
	SaveAlertHealth(ctx context.Context, h models.AlertHealthRecord) error

	// Logs
	SaveLog(ctx context.Context, message string) error
	LoadLogs(ctx context.Context, limit int) ([]models.LogEntry, error)
	PruneLogs(ctx context.Context) error

	// Maintenance Windows
	GetActiveMaintenanceWindows(ctx context.Context) ([]models.MaintenanceWindow, error)
	GetAllMaintenanceWindows(ctx context.Context, limit int) ([]models.MaintenanceWindow, error)
	AddMaintenanceWindow(ctx context.Context, mw models.MaintenanceWindow) error
	EndMaintenanceWindow(ctx context.Context, id int) error
	DeleteMaintenanceWindow(ctx context.Context, id int) error
	PruneExpiredMaintenanceWindows(ctx context.Context, retention time.Duration) (int64, error)
	IsMonitorInMaintenance(ctx context.Context, monitorID int) (bool, error)

	// Preferences
	GetPreference(ctx context.Context, key string) (string, error)
	SetPreference(ctx context.Context, key, value string) error

	// Backup & Restore
	ExportData(ctx context.Context) (models.Backup, error)
	ImportData(ctx context.Context, data models.Backup) error

	// Lifecycle
	Close() error
}
