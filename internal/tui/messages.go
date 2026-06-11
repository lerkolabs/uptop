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
// a transient store error. seq orders in-flight loads: replies whose seq is
// older than the model's current tabSeq are dropped, so a slow load can never
// overwrite the result of a newer one.
type tabDataMsg struct {
	seq    int
	alerts []models.AlertConfig
	users  []models.User
	nodes  []models.ProbeNode
	maint  []models.MaintenanceWindow
	err    error
}

// detailDataMsg carries the state-change history for the detail panel, loaded
// on entry and refreshed on the tab-data cadence so View never touches the
// database.
type detailDataMsg struct {
	siteID  int
	changes []models.StateChange
}

// historyDataMsg carries the full state-change history for the history view.
// siteID guards against a slow reply landing after the user opened a
// different site's history.
type historyDataMsg struct {
	siteID  int
	changes []models.StateChange
}

// slaDataMsg carries the state changes backing the SLA view for one
// site+period request. siteID and periodIdx guard stale replies the same way
// historyDataMsg does.
type slaDataMsg struct {
	siteID    int
	periodIdx int
	changes   []models.StateChange
}

// writeDoneMsg reports a store mutation that ran off the UI goroutine. op
// names the action for the error log; the handler reloads tab data so the UI
// converges on what was actually written.
type writeDoneMsg struct {
	op  string
	err error
}
