package monitor

import (
	"context"
	"fmt"
	"time"
)

func (e *Engine) maintenancePruner(ctx context.Context) {
	ticker := time.NewTicker(maintPruneInterval)
	defer ticker.Stop()

	e.pruneMaintenanceWindows(ctx)

	for {
		select {
		case <-ticker.C:
			e.pruneMaintenanceWindows(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (e *Engine) pruneMaintenanceWindows(ctx context.Context) {
	pruned, err := e.db.PruneExpiredMaintenanceWindows(ctx, e.maintRetention)
	if err != nil {
		e.AddLog(fmt.Sprintf("Maintenance prune error: %v", err))
		return
	}
	if pruned > 0 {
		e.AddLog(fmt.Sprintf("Pruned %d expired maintenance window(s)", pruned))
	}
}

func (e *Engine) isInMaintenance(monitorID int) bool {
	e.maintCacheMu.RLock()
	defer e.maintCacheMu.RUnlock()
	return e.maintCache[monitorID]
}

func (e *Engine) refreshMaintenanceCache(ctx context.Context) {
	windows, err := e.db.GetActiveMaintenanceWindows(ctx)
	if err != nil {
		return
	}

	directMaint := make(map[int]bool)
	var globalMaint bool
	for _, w := range windows {
		if w.MonitorID == 0 {
			globalMaint = true
		} else {
			directMaint[w.MonitorID] = true
		}
	}

	resolved := make(map[int]bool)
	e.mu.RLock()
	for id, site := range e.liveState {
		if globalMaint || directMaint[id] || (site.ParentID > 0 && directMaint[site.ParentID]) {
			resolved[id] = true
		}
	}
	e.mu.RUnlock()

	e.maintCacheMu.Lock()
	e.maintCache = resolved
	e.maintCacheMu.Unlock()
}
