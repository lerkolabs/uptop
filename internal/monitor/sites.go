package monitor

import (
	"context"
	"fmt"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

func (e *Engine) SetActive(active bool) {
	e.activeMu.Lock()
	defer e.activeMu.Unlock()
	if e.isActive != active {
		e.isActive = active
		status := "RESUMED (Active)"
		if !active {
			status = "PAUSED (Passive)"
		}
		e.AddLog(fmt.Sprintf("Engine %s", status))
	}
}

func (e *Engine) IsActive() bool {
	e.activeMu.RLock()
	defer e.activeMu.RUnlock()
	return e.isActive
}

func (e *Engine) GetAllSites() []models.Site {
	e.mu.RLock()
	defer e.mu.RUnlock()
	sites := make([]models.Site, 0, len(e.liveState))
	for _, s := range e.liveState {
		sites = append(sites, s)
	}
	return sites
}

func (e *Engine) GetLiveState() map[int]models.Site {
	e.mu.RLock()
	defer e.mu.RUnlock()
	cp := make(map[int]models.Site, len(e.liveState))
	for k, v := range e.liveState {
		cp[k] = v
	}
	return cp
}

func (e *Engine) addToTokenIndex(site models.Site) {
	if site.Type == "push" && site.Token != "" {
		e.tokenIndex[site.Token] = site.ID
	}
}

func (e *Engine) removeFromTokenIndex(id int) {
	for token, sid := range e.tokenIndex {
		if sid == id {
			delete(e.tokenIndex, token)
			return
		}
	}
}

func (e *Engine) UpdateSiteConfig(cfg models.SiteConfig) {
	e.mu.Lock()
	if existing, ok := e.liveState[cfg.ID]; ok {
		e.removeFromTokenIndex(cfg.ID)
		existing.SiteConfig = cfg
		e.liveState[cfg.ID] = existing
		e.addToTokenIndex(existing)
	}
	e.mu.Unlock()

	e.signalRecheck(cfg.ID)
}

func (e *Engine) RemoveSite(id int) {
	e.mu.Lock()
	e.removeFromTokenIndex(id)
	delete(e.liveState, id)
	e.mu.Unlock()
	e.removeHistory(id)

	e.probeResultsMu.Lock()
	delete(e.probeResults, id)
	e.probeResultsMu.Unlock()

	e.recheckMu.Lock()
	delete(e.recheck, id)
	e.recheckMu.Unlock()
}

func (e *Engine) ToggleSitePause(id int) bool {
	var (
		paused bool
		name   string
	)
	_, ok := e.applyState(id, func(s *models.Site) {
		s.Paused = !s.Paused
		paused = s.Paused
		name = s.Name
	})
	if !ok {
		return false
	}
	if paused {
		e.AddLog(fmt.Sprintf("Monitor '%s' paused", name))
	} else {
		e.AddLog(fmt.Sprintf("Monitor '%s' resumed", name))
	}
	return paused
}

// applyState atomically reads, mutates, and writes back the live entry for id.
// The mutator runs under the engine write lock and receives a pointer to the
// CURRENT live state, so concurrent config edits, pauses, and heartbeats are
// never clobbered by a stale snapshot. The mutator must only touch runtime /
// check-result fields — config fields (Name/URL/Type/Token/Interval/AlertID/…)
// are owned by UpdateSiteConfig and must not be written here. Returns the
// post-mutation copy and whether the site still exists.
func (e *Engine) applyState(id int, mutate func(s *models.Site)) (models.Site, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	cur, ok := e.liveState[id]
	if !ok {
		return models.Site{}, false
	}
	mutate(&cur)
	e.liveState[id] = cur
	return cur, true
}

func (e *Engine) GetDisplayStatus(site models.Site) string {
	if site.Paused {
		return "PAUSED"
	}
	if e.isInMaintenance(site.ID) {
		return "MAINT"
	}
	return string(site.Status)
}

func (e *Engine) GetStateChanges(ctx context.Context, siteID int, limit int) []models.StateChange {
	changes, err := e.db.GetStateChanges(ctx, siteID, limit)
	if err != nil {
		return nil
	}
	return changes
}

func (e *Engine) GetStateChangesSince(ctx context.Context, siteID int, since time.Time) []models.StateChange {
	changes, err := e.db.GetStateChangesSince(ctx, siteID, since)
	if err != nil {
		return nil
	}
	return changes
}
