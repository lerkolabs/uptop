package tui

import (
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

// tabRefreshTTL bounds how often the DB-backed tab data (alerts, users, nodes,
// maintenance windows) is reloaded. Live sites + logs come from in-memory
// engine copies and refresh every tick; the DB tables change rarely, so a 5s
// floor keeps tab-bar counts fresh without a per-second query storm.
const tabRefreshTTL = 5 * time.Second

// tickMsg is the once-per-second heartbeat. A named type (vs a bare time.Time)
// keeps it from colliding with any other time-valued message.
type tickMsg time.Time

// tabDataMsg carries the result of an async load of the DB-backed tab tables.
// On err, the model keeps its previous data and logs — never wiping the view on
// a transient store error.
type tabDataMsg struct {
	alerts []models.AlertConfig
	users  []models.User
	nodes  []models.ProbeNode
	maint  []models.MaintenanceWindow
	err    error
}

// detailDataMsg carries the state-change history for the detail panel, loaded
// when the panel is opened so View never touches the database.
type detailDataMsg struct {
	siteID  int
	changes []models.StateChange
}
