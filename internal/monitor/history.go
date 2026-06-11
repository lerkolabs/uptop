package monitor

import (
	"context"
	"time"
)

const maxHistoryLen = 60

type SiteHistory struct {
	Latencies   []time.Duration
	Statuses    []bool
	TotalChecks int
	UpChecks    int
}

func (e *Engine) InitHistory() {
	all, err := e.db.LoadAllHistory(context.Background(), maxHistoryLen)
	if err != nil {
		e.AddLog("Failed to load check history: " + err.Error())
		return
	}
	e.histMu.Lock()
	defer e.histMu.Unlock()
	for siteID, records := range all {
		h := &SiteHistory{}
		for _, r := range records {
			h.TotalChecks++
			if r.IsUp {
				h.UpChecks++
			}
			h.Latencies = append(h.Latencies, time.Duration(r.LatencyNs))
			h.Statuses = append(h.Statuses, r.IsUp)
		}
		e.histories[siteID] = h
	}
	if len(all) > 0 {
		e.AddLog("Loaded check history from database")
	}
}

func (e *Engine) recordCheck(siteID int, latency time.Duration, isUp bool) {
	e.histMu.Lock()
	defer e.histMu.Unlock()

	h, ok := e.histories[siteID]
	if !ok {
		h = &SiteHistory{}
		e.histories[siteID] = h
	}

	h.TotalChecks++
	if isUp {
		h.UpChecks++
	}

	h.Latencies = append(h.Latencies, latency)
	if len(h.Latencies) > maxHistoryLen {
		h.Latencies = h.Latencies[len(h.Latencies)-maxHistoryLen:]
	}

	h.Statuses = append(h.Statuses, isUp)
	if len(h.Statuses) > maxHistoryLen {
		h.Statuses = h.Statuses[len(h.Statuses)-maxHistoryLen:]
	}

	e.enqueueWrite(writeCheck{siteID: siteID, latencyNs: latency.Nanoseconds(), isUp: isUp})
}

func (e *Engine) GetHistory(siteID int) (SiteHistory, bool) {
	e.histMu.RLock()
	defer e.histMu.RUnlock()
	h, ok := e.histories[siteID]
	if !ok {
		return SiteHistory{}, false
	}
	cp := SiteHistory{
		TotalChecks: h.TotalChecks,
		UpChecks:    h.UpChecks,
		Latencies:   make([]time.Duration, len(h.Latencies)),
		Statuses:    make([]bool, len(h.Statuses)),
	}
	copy(cp.Latencies, h.Latencies)
	copy(cp.Statuses, h.Statuses)
	return cp, true
}

func (e *Engine) removeHistory(siteID int) {
	e.histMu.Lock()
	defer e.histMu.Unlock()
	delete(e.histories, siteID)
}
