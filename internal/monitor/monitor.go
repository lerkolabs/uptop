package monitor

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

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
	alertSendTimeout      = 30 * time.Second
	dbPruneInterval       = 10 * time.Minute
)

type Engine struct {
	mu        sync.RWMutex
	liveState map[int]models.Site

	logMu    sync.RWMutex
	logStore []models.LogEntry

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
	ctx       context.Context
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
		ctx:                 context.Background(),
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
func (e *Engine) appendLog(msg string) models.LogEntry {
	entry := models.LogEntry{
		Message:   sanitizeLog(msg),
		CreatedAt: time.Now(),
	}
	e.logMu.Lock()
	e.logStore = append([]models.LogEntry{entry}, e.logStore...)
	if len(e.logStore) > maxLogEntries {
		e.logStore = e.logStore[:maxLogEntries]
	}
	e.logMu.Unlock()
	return entry
}

func (e *Engine) AddLog(msg string) {
	entry := e.appendLog(msg)
	e.enqueueWrite(writeLog{message: entry.Message})
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
// Uses context.Background because the engine ctx is already cancelled when
// this runs — writes still need to reach the DB.
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

func (e *Engine) InitLogs(ctx context.Context) {
	entries, err := e.db.LoadLogs(ctx, maxLogEntries)
	if err != nil {
		return
	}
	if len(entries) == 0 {
		return
	}
	e.logMu.Lock()
	defer e.logMu.Unlock()
	e.logStore = entries
}

func (e *Engine) GetLogs() []models.LogEntry {
	e.logMu.RLock()
	defer e.logMu.RUnlock()
	logs := make([]models.LogEntry, len(e.logStore))
	copy(logs, e.logStore)
	return logs
}

func (e *Engine) Start(ctx context.Context) {
	// e.cancel is invoked by Stop() to drain and halt the writer; gosec can't
	// trace the cross-method call, and cancelling the parent reaps this child
	// regardless, so the leak it warns about can't occur.
	ctx, e.cancel = context.WithCancel(ctx) //nolint:gosec // cancel is called in Stop()
	e.ctx = ctx

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

			// Refresh after sites load so the cache covers newly added sites.
			// On first iteration liveState was empty before the loop above.
			e.refreshMaintenanceCache(ctx)

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
