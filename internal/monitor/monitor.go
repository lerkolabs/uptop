package monitor

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/rand/v2"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/alert"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/store"
)

const (
	maxLogEntries         = 100
	pollInterval          = 5 * time.Second
	minCheckInterval      = 5
	minPushGrace          = 60 * time.Second
	maintPruneInterval    = 15 * time.Minute
	defaultMaintRetention = 7 * 24 * time.Hour
	dbWriteBuffer         = 4096
	dbPruneInterval       = 10 * time.Minute
)

type AlertHealth struct {
	LastSendAt time.Time
	LastSendOK bool
	LastError  string
	SendCount  int
	FailCount  int
}

type Engine struct {
	mu        sync.RWMutex
	liveState map[int]models.Site

	logMu    sync.RWMutex
	logStore []string

	activeMu sync.RWMutex
	isActive bool

	histMu    sync.RWMutex
	histories map[int]*SiteHistory

	tokenIndex map[string]int // protected by mu

	probeResultsMu sync.RWMutex
	probeResults   map[int]map[string]NodeResult
	aggStrategy    AggregationStrategy

	alertHealthMu sync.RWMutex
	alertHealth   map[int]AlertHealth

	recheckMu sync.RWMutex
	recheck   map[int]chan struct{}

	maintCacheMu sync.RWMutex
	maintCache   map[int]bool

	db                  store.Store
	insecureSkipVerify  bool
	allowPrivateTargets bool
	maintRetention      time.Duration
	strictClient        *http.Client
	insecureClient      *http.Client

	dbWrites  chan dbWrite
	writerWG  sync.WaitGroup
	checkerWG sync.WaitGroup
	cancel    context.CancelFunc
	stopOnce  sync.Once
}

func NewEngine(s store.Store) *Engine {
	return newEngine(s, false)
}

func NewEngineWithOpts(s store.Store, allowPrivateTargets bool) *Engine {
	return newEngine(s, allowPrivateTargets)
}

func newEngine(s store.Store, allowPrivateTargets bool) *Engine {
	dial := SafeDialContext(allowPrivateTargets)
	return &Engine{
		liveState:           make(map[int]models.Site),
		histories:           make(map[int]*SiteHistory),
		tokenIndex:          make(map[string]int),
		recheck:             make(map[int]chan struct{}),
		probeResults:        make(map[int]map[string]NodeResult),
		alertHealth:         make(map[int]AlertHealth),
		aggStrategy:         AggAnyDown,
		isActive:            true,
		allowPrivateTargets: allowPrivateTargets,
		maintRetention:      defaultMaintRetention,
		dbWrites:            make(chan dbWrite, dbWriteBuffer),
		db:                  s,
		strictClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
				DialContext:     dial,
			},
		},
		insecureClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // intentional for IgnoreTLS sites
				DialContext:     dial,
			},
		},
	}
}

// SetInsecureSkipVerify must be called before Start: the field is read by
// checker goroutines without synchronization.
func (e *Engine) SetInsecureSkipVerify(skip bool) {
	e.insecureSkipVerify = skip
}

// SetMaintRetention must be called before Start: the field is read by the
// maintenance prune goroutine without synchronization.
func (e *Engine) SetMaintRetention(d time.Duration) {
	e.maintRetention = d
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func sanitizeLog(s string) string {
	s = ansiRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

func fmtDurationShort(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
}

// appendLog adds a timestamped entry to the in-memory ring buffer and returns
// it. It never touches the database, so it is safe to call from the db-write
// drop/error path without recursing back through the write queue.
func (e *Engine) appendLog(msg string) string {
	ts := time.Now().Format("15:04:05")
	entry := fmt.Sprintf("[%s] %s", ts, sanitizeLog(msg))
	e.logMu.Lock()
	e.logStore = append([]string{entry}, e.logStore...)
	if len(e.logStore) > maxLogEntries {
		e.logStore = e.logStore[:maxLogEntries]
	}
	e.logMu.Unlock()
	return entry
}

func (e *Engine) AddLog(msg string) {
	entry := e.appendLog(msg)
	e.enqueueWrite(writeLog{message: entry})
}

// enqueueWrite hands a persistence task to the writer goroutine without
// blocking the caller. If the queue is saturated the write is dropped and noted
// in the in-memory log only (never re-enqueued, to avoid recursion via AddLog).
func (e *Engine) enqueueWrite(w dbWrite) {
	select {
	case e.dbWrites <- w:
	default:
		e.appendLog(fmt.Sprintf("db write queue full, dropped %s", w.desc()))
	}
}

// dbWriter is the single goroutine that owns all writes. Serializing writes
// through one path removes the fire-and-forget goroutine pile-up, surfaces
// errors, and lets retention run on a timer instead of per-insert. It drains
// any buffered writes on shutdown before returning.
func (e *Engine) dbWriter(ctx context.Context) {
	defer e.writerWG.Done()

	pruneTicker := time.NewTicker(dbPruneInterval)
	defer pruneTicker.Stop()
	e.prune(ctx)

	for {
		select {
		case w := <-e.dbWrites:
			if err := w.exec(ctx, e.db); err != nil {
				e.appendLog(fmt.Sprintf("db %s write failed: %v", w.desc(), err))
			}
		case <-pruneTicker.C:
			e.prune(ctx)
		case <-ctx.Done():
			e.drainWrites()
			return
		}
	}
}

// drainWrites flushes everything still buffered, best-effort, at shutdown.
func (e *Engine) drainWrites() {
	for {
		select {
		case w := <-e.dbWrites:
			if err := w.exec(context.Background(), e.db); err != nil {
				e.appendLog(fmt.Sprintf("db %s write failed (drain): %v", w.desc(), err))
			}
		default:
			return
		}
	}
}

func (e *Engine) prune(ctx context.Context) {
	if err := e.db.PruneLogs(ctx); err != nil {
		e.appendLog(fmt.Sprintf("log prune failed: %v", err))
	}
	if err := e.db.PruneCheckHistory(ctx); err != nil {
		e.appendLog(fmt.Sprintf("check-history prune failed: %v", err))
	}
	if err := e.db.PruneStateChanges(ctx); err != nil {
		e.appendLog(fmt.Sprintf("state-change prune failed: %v", err))
	}
}

// Stop signals the writer goroutine to drain and exit, then blocks until it
// has. Call it before closing the store so no write races a closed DB.
func (e *Engine) Stop() {
	e.stopOnce.Do(func() {
		if e.cancel != nil {
			e.cancel()
		}
		e.checkerWG.Wait()
		e.writerWG.Wait()
		e.drainWrites()
	})
}

func (e *Engine) InitLogs() {
	logs, err := e.db.LoadLogs(context.Background(), maxLogEntries)
	if err != nil {
		return
	}
	if len(logs) == 0 {
		return
	}
	e.logMu.Lock()
	defer e.logMu.Unlock()
	e.logStore = logs
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

func (e *Engine) GetLogs() []string {
	e.logMu.RLock()
	defer e.logMu.RUnlock()
	logs := make([]string, len(e.logStore))
	copy(logs, e.logStore)
	return logs
}

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

func (e *Engine) Start(ctx context.Context) {
	// e.cancel is invoked by Stop() to drain and halt the writer; gosec can't
	// trace the cross-method call, and cancelling the parent reaps this child
	// regardless, so the leak it warns about can't occur.
	ctx, e.cancel = context.WithCancel(ctx) //nolint:gosec // cancel is called in Stop()

	e.writerWG.Add(1)
	go e.dbWriter(ctx)

	e.checkerWG.Add(1)
	go func() {
		defer e.checkerWG.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			e.refreshMaintenanceCache(ctx)

			configs, err := e.db.GetSites(ctx)
			if err != nil {
				e.AddLog(fmt.Sprintf("Failed to load sites: %v", err))
				select {
				case <-time.After(pollInterval):
				case <-ctx.Done():
					return
				}
				continue
			}
			dbIDs := make(map[int]bool, len(configs))
			for _, cfg := range configs {
				dbIDs[cfg.ID] = true
				e.mu.RLock()
				existing, exists := e.liveState[cfg.ID]
				e.mu.RUnlock()
				if !exists {
					e.mu.Lock()
					site := models.Site{SiteConfig: cfg, SiteState: models.SiteState{Status: models.StatusPending}}
					if h, ok := e.GetHistory(cfg.ID); ok && len(h.Statuses) > 0 {
						if h.Statuses[len(h.Statuses)-1] {
							site.Status = models.StatusUp
						} else {
							site.Status = models.StatusDown
						}
						if len(h.Latencies) > 0 {
							site.Latency = h.Latencies[len(h.Latencies)-1]
						}
					}
					e.liveState[cfg.ID] = site
					e.addToTokenIndex(site)
					e.mu.Unlock()
					e.checkerWG.Add(1)
					go func(id int) {
						defer e.checkerWG.Done()
						e.monitorRoutine(ctx, id)
					}(cfg.ID)
				} else if existing.SiteConfig != cfg {
					e.UpdateSiteConfig(cfg)
				}
			}

			e.mu.RLock()
			var vanished []int
			for id := range e.liveState {
				if !dbIDs[id] {
					vanished = append(vanished, id)
				}
			}
			e.mu.RUnlock()
			for _, id := range vanished {
				e.RemoveSite(id)
				e.AddLog(fmt.Sprintf("Monitor removed (no longer in DB): ID %d", id))
			}

			select {
			case <-time.After(pollInterval):
			case <-ctx.Done():
				return
			}
		}
	}()

	e.checkerWG.Add(1)
	go func() {
		defer e.checkerWG.Done()
		e.maintenancePruner(ctx)
	}()
}

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
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

func (e *Engine) GetDisplayStatus(site models.Site) string {
	if site.Paused {
		return "PAUSED"
	}
	if e.isInMaintenance(site.ID) {
		return "MAINT"
	}
	return string(site.Status)
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

func (e *Engine) GetStateChanges(siteID int, limit int) []models.StateChange {
	changes, err := e.db.GetStateChanges(context.Background(), siteID, limit)
	if err != nil {
		return nil
	}
	return changes
}

func (e *Engine) GetStateChangesSince(siteID int, since time.Time) []models.StateChange {
	changes, err := e.db.GetStateChangesSince(context.Background(), siteID, since)
	if err != nil {
		return nil
	}
	return changes
}
