package monitor

import (
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/store"
)

// dbWrite is a single unit of deferred persistence. The engine enqueues these
// onto a buffered channel; a single writer goroutine drains and executes them,
// serializing all writes through one connection and surfacing errors instead of
// discarding them. desc names the write for diagnostics on drop/failure.
type dbWrite interface {
	exec(s store.Store) error
	desc() string
}

type writeLog struct{ message string }

func (w writeLog) exec(s store.Store) error { return s.SaveLog(w.message) }
func (w writeLog) desc() string             { return "log" }

type writeCheck struct {
	siteID    int
	latencyNs int64
	isUp      bool
}

func (w writeCheck) exec(s store.Store) error { return s.SaveCheck(w.siteID, w.latencyNs, w.isUp) }
func (w writeCheck) desc() string             { return "check" }

type writeStateChange struct {
	siteID     int
	fromStatus string
	toStatus   string
	reason     string
}

func (w writeStateChange) exec(s store.Store) error {
	return s.SaveStateChange(w.siteID, w.fromStatus, w.toStatus, w.reason)
}
func (w writeStateChange) desc() string { return "state-change" }

type writeAlertHealth struct{ rec models.AlertHealthRecord }

func (w writeAlertHealth) exec(s store.Store) error { return s.SaveAlertHealth(w.rec) }
func (w writeAlertHealth) desc() string             { return "alert-health" }
