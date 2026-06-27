package monitor

import (
	"context"
	"fmt"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/alert"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

type AlertHealth struct {
	LastSendAt time.Time
	LastSendOK bool
	LastError  string
	SendCount  int
	FailCount  int
}

// InitAlertHealth restores persisted alert send health so the dashboard shows real
// "last sent" / health state on startup instead of resetting every channel to "never".
func (e *Engine) InitAlertHealth() {
	records, err := e.db.LoadAlertHealth(context.Background())
	if err != nil {
		return
	}
	e.alertHealthMu.Lock()
	defer e.alertHealthMu.Unlock()
	for id, r := range records {
		e.alertHealth[id] = AlertHealth{
			LastSendAt: r.LastSendAt,
			LastSendOK: r.LastSendOK,
			LastError:  r.LastError,
			SendCount:  r.SendCount,
			FailCount:  r.FailCount,
		}
	}
}

// handleStatusChange folds a check result into the live state. snap is the
// stale snapshot the check ran against; the actual mutation is applied onto the
// CURRENT live entry via applyState, so a concurrent pause / config edit /
// heartbeat is never reverted by this write. Logs and alerts are emitted after
// the lock is released, off the critical section.
func (e *Engine) handleStatusChange(snap models.Site, rawStatus string, code int, latency time.Duration, errorReason string) {
	if !e.IsActive() {
		return
	}

	inMaint := e.isInMaintenance(snap.ID)
	status := models.Status(rawStatus)

	var (
		prev, next            models.Status
		name, typ             string
		alertID               int
		failCount, maxRetries int
		confirmedDown         bool
		failedCheck           bool
		downSince             time.Time
		sslWarnFire           bool
		sslDays               int
		skipped               bool
		changed               bool
	)

	_, exists := e.applyState(snap.ID, func(s *models.Site) {
		// A non-UP result computed from a stale snapshot must not override a
		// heartbeat (or newer check) that landed while we were evaluating.
		if status != models.StatusUp && s.LastCheck.After(snap.LastCheck) {
			skipped = true
			return
		}

		prev = s.Status
		name = s.Name
		typ = s.Type
		alertID = s.AlertID
		maxRetries = s.MaxRetries
		downSince = s.StatusChangedAt

		// Fresh check results (measured by the run against snap).
		s.StatusCode = code
		s.Latency = snap.Latency
		s.LastCheck = snap.LastCheck
		s.HasSSL = snap.HasSSL
		s.CertExpiry = snap.CertExpiry
		s.LastError = errorReason
		if status == models.StatusUp {
			s.LastSuccessAt = time.Now()
			s.LastError = ""
		}

		// Status + failure-count transition, based on the CURRENT live status.
		if status == models.StatusUp {
			s.FailureCount = 0
			s.Status = models.StatusUp
		} else {
			if s.FailureCount <= s.MaxRetries {
				s.FailureCount++
			}
			if s.FailureCount > s.MaxRetries {
				if s.Status != status {
					confirmedDown = true
				}
				s.Status = status
				s.FailureCount = s.MaxRetries + 1
			} else {
				failedCheck = true
			}
		}
		failCount = s.FailureCount

		if s.Status != prev && prev != models.StatusPending {
			s.StatusChangedAt = time.Now()
		} else if s.StatusChangedAt.IsZero() && s.Status != models.StatusPending {
			s.StatusChangedAt = time.Now()
		}

		// SSL expiry warning (fresh HasSSL/CertExpiry + config threshold).
		if typ == "http" && s.CheckSSL && s.HasSSL {
			days := int(time.Until(s.CertExpiry).Hours() / 24)
			if days <= s.ExpiryThreshold && !s.SentSSLWarning && status != models.StatusSSLExp {
				sslWarnFire = true
				sslDays = days
				s.SentSSLWarning = true
			} else if days > s.ExpiryThreshold {
				s.SentSSLWarning = false
			}
		}

		next = s.Status
		changed = next != prev
	})

	if !exists || skipped {
		return
	}

	e.recordCheck(snap.ID, latency, status == models.StatusUp)

	if confirmedDown {
		if errorReason != "" {
			e.AddLog(fmt.Sprintf("Monitor '%s' confirmed DOWN: %s", name, errorReason))
		} else {
			e.AddLog(fmt.Sprintf("Monitor '%s' confirmed DOWN", name))
		}
	} else if failedCheck {
		e.AddLog(fmt.Sprintf("Monitor '%s' failed check %d/%d", name, failCount, maxRetries))
	}

	if changed && prev != models.StatusPending {
		e.enqueueWrite(writeStateChange{siteID: snap.ID, fromStatus: string(prev), toStatus: string(next), reason: errorReason})
	}

	if sslWarnFire {
		if !inMaint {
			e.triggerAlert(alertID, "SSL WARNING", fmt.Sprintf("SSL for '%s' expires in %d days", name, sslDays))
		} else {
			e.AddLog(fmt.Sprintf("SSL warning for '%s' suppressed (maintenance)", name))
		}
	}

	if prev == models.StatusUp && next == models.StatusLate {
		e.AddLog(fmt.Sprintf("Monitor '%s' heartbeat overdue", name))
	}

	if !prev.IsBroken() && next.IsBroken() && next != models.StatusPending {
		if inMaint {
			e.AddLog(fmt.Sprintf("Monitor '%s' is DOWN (alerts suppressed — maintenance)", name))
		} else {
			msg := fmt.Sprintf("Monitor '%s' is DOWN (%s)", name, rawStatus)
			if errorReason != "" {
				msg = fmt.Sprintf("Monitor '%s' is DOWN: %s", name, errorReason)
			}
			if typ == "push" {
				msg = fmt.Sprintf("Push Monitor '%s' missed heartbeat.", name)
			}
			e.triggerAlert(alertID, "🚨 ALERT", msg)
		}
	}
	if prev.IsBroken() && next == models.StatusUp {
		downDur := ""
		if !downSince.IsZero() {
			downDur = fmt.Sprintf(" (was down %s)", fmtDurationShort(time.Since(downSince)))
		}
		e.AddLog(fmt.Sprintf("Monitor '%s' recovered%s", name, downDur))
		if !inMaint {
			e.triggerAlert(alertID, "✅ RECOVERY", fmt.Sprintf("Monitor '%s' is UP%s", name, downDur))
		}
	}
	if prev == models.StatusLate && next == models.StatusUp && !prev.IsBroken() {
		e.AddLog(fmt.Sprintf("Monitor '%s' heartbeat arrived (was late)", name))
	}
}

func (e *Engine) triggerAlert(alertID int, title, message string) {
	if alertID <= 0 {
		return
	}
	cfg, err := e.db.GetAlert(context.Background(), alertID)
	if err != nil {
		e.AddLog(fmt.Sprintf("Failed to load alert config %d: %v", alertID, err))
		return
	}
	provider := alert.GetProvider(cfg)
	if provider != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), alertSendTimeout)
			defer cancel()
			if err := provider.Send(ctx, title, message); err != nil {
				e.AddLog(fmt.Sprintf("Alert send failed (%s): %v", cfg.Name, err))
				e.recordAlertResult(alertID, false, err.Error())
			} else {
				e.recordAlertResult(alertID, true, "")
			}
		}()
	}
}

func (e *Engine) recordAlertResult(alertID int, ok bool, errMsg string) {
	e.alertHealthMu.Lock()
	defer e.alertHealthMu.Unlock()
	h := e.alertHealth[alertID]
	h.LastSendAt = time.Now()
	h.LastSendOK = ok
	h.SendCount++
	if ok {
		h.LastError = ""
	} else {
		h.LastError = errMsg
		h.FailCount++
	}
	e.alertHealth[alertID] = h

	// Persist so health survives restarts; DB IO off the alert path.
	e.enqueueWrite(writeAlertHealth{rec: models.AlertHealthRecord{
		AlertID:    alertID,
		LastSendAt: h.LastSendAt,
		LastSendOK: h.LastSendOK,
		LastError:  h.LastError,
		SendCount:  h.SendCount,
		FailCount:  h.FailCount,
	}})
}

func (e *Engine) GetAlertHealth(alertID int) AlertHealth {
	e.alertHealthMu.RLock()
	defer e.alertHealthMu.RUnlock()
	return e.alertHealth[alertID]
}

func (e *Engine) TestAlert(alertID int) error {
	cfg, err := e.db.GetAlert(context.Background(), alertID)
	if err != nil {
		return fmt.Errorf("failed to load alert: %w", err)
	}
	provider := alert.GetProvider(cfg)
	if provider == nil {
		return fmt.Errorf("no provider for type %q", cfg.Type)
	}
	ctx, cancel := context.WithTimeout(context.Background(), alertSendTimeout)
	defer cancel()
	err = provider.Send(ctx, "🧪 Test Alert", fmt.Sprintf("Test notification from uptop for channel '%s'.", cfg.Name))
	if err != nil {
		e.recordAlertResult(alertID, false, err.Error())
		return err
	}
	e.recordAlertResult(alertID, true, "")
	e.AddLog(fmt.Sprintf("Test alert sent to '%s'", cfg.Name))
	return nil
}
