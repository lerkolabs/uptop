package monitor

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

func (e *Engine) RecordHeartbeat(token string) bool {
	if !e.IsActive() {
		return false
	}

	e.mu.RLock()
	targetID, ok := e.tokenIndex[token]
	e.mu.RUnlock()
	if !ok {
		return false
	}

	var (
		prevStatus models.Status
		name       string
		alertID    int
		downSince  time.Time
	)
	_, exists := e.applyState(targetID, func(s *models.Site) {
		prevStatus = s.Status
		name = s.Name
		alertID = s.AlertID
		downSince = s.StatusChangedAt // captured before mutation = when it went down

		s.LastCheck = time.Now()
		s.Status = models.StatusUp
		s.FailureCount = 0
		s.Latency = 0
		s.LastError = ""
		s.LastSuccessAt = time.Now()
		if prevStatus != models.StatusUp {
			s.StatusChangedAt = time.Now()
		}
	})
	if !exists {
		return false
	}

	switch prevStatus {
	case models.StatusPending:
		e.AddLog(fmt.Sprintf("Push Monitor '%s' received first heartbeat", name))
	case models.StatusLate:
		e.AddLog(fmt.Sprintf("Push Monitor '%s' heartbeat arrived (was late)", name))
	case models.StatusStale:
		e.AddLog(fmt.Sprintf("Push Monitor '%s' heartbeat arrived (was stale)", name))
	case models.StatusDown:
		downDur := ""
		if !downSince.IsZero() {
			downDur = fmt.Sprintf(" (was down %s)", fmtDurationShort(time.Since(downSince)))
		}
		e.AddLog(fmt.Sprintf("Push Monitor '%s' recovered%s", name, downDur))
		go e.triggerAlert(alertID, "✅ RECOVERY", fmt.Sprintf("Push Monitor '%s' is receiving heartbeats.%s", name, downDur))
	}

	e.recordCheck(targetID, 0, true)

	if prevStatus != models.StatusUp && prevStatus != models.StatusPending {
		e.enqueueWrite(writeStateChange{siteID: targetID, fromStatus: string(prevStatus), toStatus: string(models.StatusUp)})
	}

	return true
}

func (e *Engine) getRecheckChan(id int) chan struct{} {
	e.recheckMu.Lock()
	defer e.recheckMu.Unlock()
	ch, ok := e.recheck[id]
	if !ok {
		ch = make(chan struct{}, 1)
		e.recheck[id] = ch
	}
	return ch
}

func (e *Engine) signalRecheck(id int) {
	ch := e.getRecheckChan(id)
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (e *Engine) monitorRoutine(ctx context.Context, id int) {
	recheckCh := e.getRecheckChan(id)

	// Stagger initial check to avoid thundering herd on startup
	stagger := time.Duration(rand.IntN(3000)) * time.Millisecond //nolint:gosec // non-security jitter
	select {
	case <-time.After(stagger):
	case <-ctx.Done():
		return
	}

	e.checkByID(ctx, id)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if !e.IsActive() {
			select {
			case <-time.After(pollInterval):
			case <-ctx.Done():
				return
			case <-recheckCh:
			}
			continue
		}

		e.mu.RLock()
		site, exists := e.liveState[id]
		e.mu.RUnlock()
		if !exists {
			return
		}

		if site.Paused {
			select {
			case <-time.After(pollInterval):
			case <-ctx.Done():
				return
			case <-recheckCh:
			}
			continue
		}

		interval := site.Interval
		if interval < minCheckInterval {
			interval = minCheckInterval
		}
		jitter := time.Duration(rand.IntN(interval*100)) * time.Millisecond //nolint:gosec // non-security jitter
		select {
		case <-time.After(time.Duration(interval)*time.Second + jitter):
		case <-ctx.Done():
			return
		case <-recheckCh:
		}
		e.checkByID(ctx, id)
	}
}

func (e *Engine) checkByID(ctx context.Context, id int) {
	if !e.IsActive() {
		return
	}

	e.mu.RLock()
	site, exists := e.liveState[id]
	e.mu.RUnlock()
	if !exists || site.Paused {
		return
	}

	switch site.Type {
	case "push":
		e.checkPush(ctx, site)
	case "group":
		e.checkGroup(ctx, site)
	default:
		result := RunCheck(ctx, site.SiteConfig, e.strictClient, e.insecureClient, e.insecureSkipVerify, e.allowPrivateTargets)
		updatedSite := site
		updatedSite.HasSSL = result.HasSSL
		updatedSite.CertExpiry = result.CertExpiry
		updatedSite.Latency = time.Duration(result.LatencyNs)
		updatedSite.LastCheck = time.Now()
		e.handleStatusChange(updatedSite, result.Status, result.StatusCode, time.Duration(result.LatencyNs), result.ErrorReason)
	}
}

func (e *Engine) checkPush(_ context.Context, site models.Site) {
	if site.Status == models.StatusPending {
		return
	}

	interval := time.Duration(site.Interval) * time.Second
	grace := interval / 2
	if grace < minPushGrace {
		grace = minPushGrace
	}

	overdue := site.LastCheck.Add(interval)
	staleMark := overdue.Add(grace / 2)
	graceEnd := overdue.Add(grace)
	now := time.Now()

	if now.After(graceEnd) {
		if site.Status != models.StatusDown {
			e.handleStatusChange(site, string(models.StatusDown), 0, 0, "heartbeat missed")
		}
	} else if now.After(staleMark) {
		if site.Status != models.StatusStale {
			e.handleStatusChange(site, string(models.StatusStale), 0, 0, "heartbeat stale")
		}
	} else if now.After(overdue) {
		if site.Status != models.StatusLate {
			e.handleStatusChange(site, string(models.StatusLate), 0, 0, "heartbeat overdue")
		}
	}
}

func (e *Engine) checkGroup(_ context.Context, site models.Site) {
	e.mu.RLock()
	status := models.StatusUp
	hasChildren := false
	for _, child := range e.liveState {
		if child.ParentID != site.ID || child.Type == "group" {
			continue
		}
		hasChildren = true
		if child.Paused || e.isInMaintenance(child.ID) {
			continue
		}
		if child.Status == models.StatusDown || child.Status == models.StatusSSLExp {
			status = models.StatusDown
		} else if child.Status == models.StatusStale && status != models.StatusDown {
			status = models.StatusStale
		} else if child.Status == models.StatusLate && status != models.StatusDown && status != models.StatusStale {
			status = models.StatusLate
		} else if child.Status == models.StatusPending && status != models.StatusDown && status != models.StatusStale && status != models.StatusLate {
			status = models.StatusPending
		}
	}
	e.mu.RUnlock()

	if !hasChildren {
		status = models.StatusPending
	}

	e.applyState(site.ID, func(s *models.Site) {
		s.Status = status
	})
	e.recordCheck(site.ID, 0, !status.IsBroken())
}

func (e *Engine) EnqueueProbeCheck(siteID int, nodeID string, latencyNs int64, isUp bool) {
	e.enqueueWrite(writeProbeCheck{siteID: siteID, nodeID: nodeID, latencyNs: latencyNs, isUp: isUp})
}

// SetAggStrategy must be called before Start: the field is read by the probe
// aggregation path without synchronization.
func (e *Engine) SetAggStrategy(strategy AggregationStrategy) {
	e.aggStrategy = strategy
}

func (e *Engine) IngestProbeResult(nodeID string, siteID int, latencyNs int64, isUp bool, errorReason string) {
	e.mu.RLock()
	site, exists := e.liveState[siteID]
	e.mu.RUnlock()
	if !exists {
		return
	}

	staleAfter := time.Duration(site.Interval) * time.Second * 3
	if staleAfter < time.Minute {
		staleAfter = time.Minute
	}

	now := time.Now()
	e.probeResultsMu.Lock()
	if e.probeResults[siteID] == nil {
		e.probeResults[siteID] = make(map[string]NodeResult)
	}
	e.probeResults[siteID][nodeID] = NodeResult{
		NodeID:      nodeID,
		IsUp:        isUp,
		LatencyNs:   latencyNs,
		CheckedAt:   now,
		ErrorReason: errorReason,
	}
	results := make([]NodeResult, 0, len(e.probeResults[siteID]))
	for id, r := range e.probeResults[siteID] {
		if now.Sub(r.CheckedAt) > staleAfter {
			delete(e.probeResults[siteID], id)
			continue
		}
		results = append(results, r)
	}
	e.probeResultsMu.Unlock()

	aggUp, avgLatency := AggregateStatus(results, e.aggStrategy)

	probeStatus := models.StatusUp
	if !aggUp {
		probeStatus = models.StatusDown
	}

	updatedSite := site
	updatedSite.Latency = time.Duration(avgLatency)
	updatedSite.LastCheck = time.Now()
	e.handleStatusChange(updatedSite, string(probeStatus), 0, time.Duration(avgLatency), errorReason)
}

func (e *Engine) GetProbeResults(siteID int) map[string]NodeResult {
	e.probeResultsMu.RLock()
	defer e.probeResultsMu.RUnlock()
	src := e.probeResults[siteID]
	cp := make(map[string]NodeResult, len(src))
	for k, v := range src {
		cp[k] = v
	}
	return cp
}
