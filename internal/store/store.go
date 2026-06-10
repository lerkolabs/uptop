package store

import (
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

type Store interface {
	Init() error

	// Sites
	GetSites() ([]models.Site, error)
	AddSite(site models.Site) error
	UpdateSite(site models.Site) error
	UpdateSitePaused(id int, paused bool) error
	DeleteSite(id int) error

	// Alerts
	GetAllAlerts() ([]models.AlertConfig, error)
	GetAlert(id int) (models.AlertConfig, error)
	AddAlert(name, aType string, settings map[string]string) error
	UpdateAlert(id int, name, aType string, settings map[string]string) error
	DeleteAlert(id int) error

	// Declarative config support
	GetSiteByName(name string) (models.Site, error)
	GetAlertByName(name string) (models.AlertConfig, error)
	AddSiteReturningID(site models.Site) (int, error)
	AddAlertReturningID(name, aType string, settings map[string]string) (int, error)

	// Users
	GetAllUsers() ([]models.User, error)
	AddUser(username, publicKey, role string) error
	UpdateUser(id int, username, publicKey, role string) error
	DeleteUser(id int) error

	// History
	SaveCheck(siteID int, latencyNs int64, isUp bool) error
	SaveCheckFromNode(siteID int, nodeID string, latencyNs int64, isUp bool) error
	LoadAllHistory(limit int) (map[int][]models.CheckRecord, error)
	PruneCheckHistory() error

	// State Changes
	SaveStateChange(siteID int, fromStatus, toStatus, errorReason string) error
	GetStateChanges(siteID int, limit int) ([]models.StateChange, error)
	GetStateChangesSince(siteID int, since time.Time) ([]models.StateChange, error)
	PruneStateChanges() error

	// Nodes
	RegisterNode(node models.ProbeNode) error
	GetNode(id string) (models.ProbeNode, error)
	GetAllNodes() ([]models.ProbeNode, error)
	UpdateNodeLastSeen(id string) error
	DeleteNode(id string) error

	// Alert Health
	LoadAlertHealth() (map[int]models.AlertHealthRecord, error)
	SaveAlertHealth(h models.AlertHealthRecord) error

	// Logs
	SaveLog(message string) error
	LoadLogs(limit int) ([]string, error)
	PruneLogs() error

	// Maintenance Windows
	GetActiveMaintenanceWindows() ([]models.MaintenanceWindow, error)
	GetAllMaintenanceWindows(limit int) ([]models.MaintenanceWindow, error)
	AddMaintenanceWindow(mw models.MaintenanceWindow) error
	EndMaintenanceWindow(id int) error
	DeleteMaintenanceWindow(id int) error
	PruneExpiredMaintenanceWindows(retention time.Duration) (int64, error)
	IsMonitorInMaintenance(monitorID int) (bool, error)

	// Preferences
	GetPreference(key string) (string, error)
	SetPreference(key, value string) error

	// Backup & Restore
	ExportData() (models.Backup, error)
	ImportData(data models.Backup) error

	// Lifecycle
	Close() error
}
