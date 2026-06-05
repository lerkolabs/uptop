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

	db                  store.Store
	insecureSkipVerify  bool
	allowPrivateTargets bool
	maintRetention      time.Duration
	strictClient        *http.Client
	insecureClient      *http.Client
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

func (e *Engine) SetInsecureSkipVerify(skip bool) {
	e.insecureSkipVerify = skip
}

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

func (e *Engine) AddLog(msg string) {
	e.logMu.Lock()
	defer e.logMu.Unlock()
	ts := time.Now().Format("15:04:05")
	entry := fmt.Sprintf("[%s] %s", ts, sanitizeLog(msg))
	e.logStore = append([]string{entry}, e.logStore...)
	if len(e.logStore) > maxLogEntries {
		e.logStore = e.logStore[:maxLogEntries]
	}
	go func() { _ = e.db.SaveLog(entry) }()
}

func (e *Engine) InitLogs() {
	logs, err := e.db.LoadLogs(maxLogEntries)
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
	records, err := e.db.LoadAlertHealth()
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

	e.mu.Lock()
	defer e.mu.Unlock()

	targetID, ok := e.tokenIndex[token]
	if !ok {
		return false
	}

	site, exists := e.liveState[targetID]
	if !exists {
		return false
	}

	prevStatus := site.Status
	site.LastCheck = time.Now()
	site.Status = "UP"
	site.FailureCount = 0
	site.Latency = 0
	site.LastError = ""
	site.LastSuccessAt = time.Now()

	if prevStatus != "UP" {
		site.StatusChangedAt = time.Now()
	}

	e.liveState[targetID] = site

	switch prevStatus {
	case "PENDING":
		e.AddLog(fmt.Sprintf("Push Monitor '%s' received first heartbeat", site.Name))
	case "LATE":
		e.AddLog(fmt.Sprintf("Push Monitor '%s' heartbeat arrived (was late)", site.Name))
	case "STALE":
		e.AddLog(fmt.Sprintf("Push Monitor '%s' heartbeat arrived (was stale)", site.Name))
	case "DOWN":
		downDur := ""
		if !site.StatusChangedAt.IsZero() {
			downDur = fmt.Sprintf(" (was down %s)", fmtDurationShort(time.Since(site.StatusChangedAt)))
		}
		e.AddLog(fmt.Sprintf("Push Monitor '%s' recovered%s", site.Name, downDur))
		go e.triggerAlert(site.AlertID, "✅ RECOVERY", fmt.Sprintf("Push Monitor '%s' is receiving heartbeats.%s", site.Name, downDur))
	}

	if prevStatus != "UP" && prevStatus != "PENDING" {
		go func() { _ = e.db.SaveStateChange(targetID, prevStatus, "UP", "") }()
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
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			sites, err := e.db.GetSites()
			if err != nil {
				e.AddLog(fmt.Sprintf("Failed to load sites: %v", err))
				select {
				case <-time.After(pollInterval):
				case <-ctx.Done():
					return
				}
				continue
			}
			for _, s := range sites {
				e.mu.RLock()
				_, exists := e.liveState[s.ID]
				e.mu.RUnlock()
				if !exists {
					e.mu.Lock()
					s.Status = "PENDING"
					if h, ok := e.GetHistory(s.ID); ok && len(h.Statuses) > 0 {
						if h.Statuses[len(h.Statuses)-1] {
							s.Status = "UP"
						} else {
							s.Status = "DOWN"
						}
						if len(h.Latencies) > 0 {
							s.Latency = h.Latencies[len(h.Latencies)-1]
						}
					}
					e.liveState[s.ID] = s
					e.addToTokenIndex(s)
					e.mu.Unlock()
					go e.monitorRoutine(ctx, s.ID)
				}
			}

			select {
			case <-time.After(pollInterval):
			case <-ctx.Done():
				return
			}
		}
	}()

	go e.maintenancePruner(ctx)
}

func (e *Engine) maintenancePruner(ctx context.Context) {
	ticker := time.NewTicker(maintPruneInterval)
	defer ticker.Stop()

	e.pruneMaintenanceWindows()

	for {
		select {
		case <-ticker.C:
			e.pruneMaintenanceWindows()
		case <-ctx.Done():
			return
		}
	}
}

func (e *Engine) pruneMaintenanceWindows() {
	pruned, err := e.db.PruneExpiredMaintenanceWindows(e.maintRetention)
	if err != nil {
		e.AddLog(fmt.Sprintf("Maintenance prune error: %v", err))
		return
	}
	if pruned > 0 {
		e.AddLog(fmt.Sprintf("Pruned %d expired maintenance window(s)", pruned))
	}
}

func (e *Engine) UpdateSiteConfig(site models.Site) {
	e.mu.Lock()
	if existing, ok := e.liveState[site.ID]; ok {
		e.removeFromTokenIndex(site.ID)
		site.Status = existing.Status
		site.StatusCode = existing.StatusCode
		site.Latency = existing.Latency
		site.CertExpiry = existing.CertExpiry
		site.HasSSL = existing.HasSSL
		site.LastCheck = existing.LastCheck
		site.SentSSLWarning = existing.SentSSLWarning
		site.FailureCount = existing.FailureCount
		site.LastError = existing.LastError
		site.StatusChangedAt = existing.StatusChangedAt
		site.LastSuccessAt = existing.LastSuccessAt
		e.liveState[site.ID] = site
		e.addToTokenIndex(site)
	}
	e.mu.Unlock()

	e.signalRecheck(site.ID)
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

	e.recheckMu.Lock()
	delete(e.recheck, id)
	e.recheckMu.Unlock()
}

func (e *Engine) ToggleSitePause(id int) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	site, ok := e.liveState[id]
	if !ok {
		return false
	}
	site.Paused = !site.Paused
	e.liveState[id] = site
	if site.Paused {
		e.AddLog(fmt.Sprintf("Monitor '%s' paused", site.Name))
	} else {
		e.AddLog(fmt.Sprintf("Monitor '%s' resumed", site.Name))
	}
	return site.Paused
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

	e.checkByID(id)
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
		e.checkByID(id)
	}
}

func (e *Engine) checkByID(id int) {
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
		e.checkPush(site)
	case "group":
		e.checkGroup(site)
	default:
		result := RunCheck(site, e.strictClient, e.insecureClient, e.insecureSkipVerify, e.allowPrivateTargets)
		updatedSite := site
		updatedSite.HasSSL = result.HasSSL
		updatedSite.CertExpiry = result.CertExpiry
		updatedSite.Latency = time.Duration(result.LatencyNs)
		updatedSite.LastCheck = time.Now()
		e.handleStatusChange(updatedSite, result.Status, result.StatusCode, time.Duration(result.LatencyNs), result.ErrorReason)
	}
}

func (e *Engine) checkPush(site models.Site) {
	if site.Status == "PENDING" {
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
		if site.Status != "DOWN" {
			e.handleStatusChange(site, "DOWN", 0, 0, "heartbeat missed")
		}
	} else if now.After(staleMark) {
		if site.Status != "STALE" {
			e.handleStatusChange(site, "STALE", 0, 0, "heartbeat stale")
		}
	} else if now.After(overdue) {
		if site.Status != "LATE" {
			e.handleStatusChange(site, "LATE", 0, 0, "heartbeat overdue")
		}
	}
}

func (e *Engine) handleStatusChange(site models.Site, rawStatus string, code int, latency time.Duration, errorReason string) {
	if !e.IsActive() {
		return
	}

	newState := site
	newState.StatusCode = code
	newState.LastError = errorReason

	if rawStatus == "UP" {
		newState.LastSuccessAt = time.Now()
		newState.LastError = ""
	} else {
		newState.LastSuccessAt = site.LastSuccessAt
	}

	if site.Status == "UP" && rawStatus != "UP" {
		newState.FailureCount++
		if newState.FailureCount > site.MaxRetries {
			newState.Status = rawStatus
			newState.FailureCount = site.MaxRetries + 1
			if errorReason != "" {
				e.AddLog(fmt.Sprintf("Monitor '%s' confirmed DOWN: %s", site.Name, errorReason))
			} else {
				e.AddLog(fmt.Sprintf("Monitor '%s' confirmed DOWN", site.Name))
			}
		} else {
			e.AddLog(fmt.Sprintf("Monitor '%s' failed check %d/%d", site.Name, newState.FailureCount, site.MaxRetries))
		}
	} else if rawStatus == "UP" {
		newState.FailureCount = 0
		newState.Status = "UP"
	} else {
		newState.Status = rawStatus
		newState.FailureCount = site.MaxRetries + 1
	}

	if newState.Status != site.Status && site.Status != "PENDING" {
		newState.StatusChangedAt = time.Now()
	} else if site.StatusChangedAt.IsZero() && newState.Status != "PENDING" {
		newState.StatusChangedAt = time.Now()
	} else {
		newState.StatusChangedAt = site.StatusChangedAt
	}

	inMaint := e.isInMaintenance(site.ID)

	if site.Type == "http" && site.CheckSSL && site.HasSSL {
		daysLeft := int(time.Until(site.CertExpiry).Hours() / 24)
		if daysLeft <= site.ExpiryThreshold && !site.SentSSLWarning && rawStatus != "SSL EXP" {
			if !inMaint {
				e.triggerAlert(site.AlertID, "SSL WARNING", fmt.Sprintf("SSL for '%s' expires in %d days", site.Name, daysLeft))
			} else {
				e.AddLog(fmt.Sprintf("SSL warning for '%s' suppressed (maintenance)", site.Name))
			}
			newState.SentSSLWarning = true
		} else if daysLeft > site.ExpiryThreshold {
			newState.SentSSLWarning = false
		}
	}

	e.mu.Lock()
	if _, ok := e.liveState[site.ID]; ok {
		e.liveState[site.ID] = newState
	}
	e.mu.Unlock()

	e.recordCheck(site.ID, latency, rawStatus == "UP")

	if newState.Status != site.Status && site.Status != "PENDING" {
		go func() { _ = e.db.SaveStateChange(site.ID, site.Status, newState.Status, errorReason) }()
	}

	isBroken := func(s string) bool { return s == "DOWN" || s == "SSL EXP" }

	if site.Status == "UP" && newState.Status == "LATE" {
		e.AddLog(fmt.Sprintf("Monitor '%s' heartbeat overdue", site.Name))
	}

	if !isBroken(site.Status) && isBroken(newState.Status) && newState.Status != "PENDING" {
		if inMaint {
			e.AddLog(fmt.Sprintf("Monitor '%s' is DOWN (alerts suppressed — maintenance)", site.Name))
		} else {
			msg := fmt.Sprintf("Monitor '%s' is DOWN (%s)", site.Name, rawStatus)
			if errorReason != "" {
				msg = fmt.Sprintf("Monitor '%s' is DOWN: %s", site.Name, errorReason)
			}
			if site.Type == "push" {
				msg = fmt.Sprintf("Push Monitor '%s' missed heartbeat.", site.Name)
			}
			e.triggerAlert(site.AlertID, "🚨 ALERT", msg)
		}
	}
	if isBroken(site.Status) && newState.Status == "UP" {
		downDur := ""
		if !site.StatusChangedAt.IsZero() {
			downDur = fmt.Sprintf(" (was down %s)", fmtDurationShort(time.Since(site.StatusChangedAt)))
		}
		e.AddLog(fmt.Sprintf("Monitor '%s' recovered%s", site.Name, downDur))
		if !inMaint {
			e.triggerAlert(site.AlertID, "✅ RECOVERY", fmt.Sprintf("Monitor '%s' is UP%s", site.Name, downDur))
		}
	}
	if site.Status == "LATE" && newState.Status == "UP" && !isBroken(site.Status) {
		e.AddLog(fmt.Sprintf("Monitor '%s' heartbeat arrived (was late)", site.Name))
	}
}

func (e *Engine) triggerAlert(alertID int, title, message string) {
	cfg, err := e.db.GetAlert(alertID)
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

	// Persist best-effort so health survives restarts; DB IO off the alert path.
	go func(rec models.AlertHealthRecord) {
		_ = e.db.SaveAlertHealth(rec)
	}(models.AlertHealthRecord{
		AlertID:    alertID,
		LastSendAt: h.LastSendAt,
		LastSendOK: h.LastSendOK,
		LastError:  h.LastError,
		SendCount:  h.SendCount,
		FailCount:  h.FailCount,
	})
}

func (e *Engine) GetAlertHealth(alertID int) AlertHealth {
	e.alertHealthMu.RLock()
	defer e.alertHealthMu.RUnlock()
	return e.alertHealth[alertID]
}

func (e *Engine) TestAlert(alertID int) error {
	cfg, err := e.db.GetAlert(alertID)
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
	inMaint, err := e.db.IsMonitorInMaintenance(monitorID)
	if err != nil {
		return false
	}
	return inMaint
}

func (e *Engine) GetDisplayStatus(site models.Site) string {
	if site.Paused {
		return "PAUSED"
	}
	if e.isInMaintenance(site.ID) {
		return "MAINT"
	}
	return site.Status
}

func (e *Engine) checkGroup(site models.Site) {
	e.mu.RLock()
	status := "UP"
	hasChildren := false
	allPaused := true
	for _, child := range e.liveState {
		if child.ParentID != site.ID || child.Type == "group" {
			continue
		}
		hasChildren = true
		if !child.Paused {
			allPaused = false
		}
		if child.Paused || e.isInMaintenance(child.ID) {
			continue
		}
		if child.Status == "DOWN" || child.Status == "SSL EXP" {
			status = "DOWN"
		} else if child.Status == "STALE" && status != "DOWN" {
			status = "STALE"
		} else if child.Status == "LATE" && status != "DOWN" && status != "STALE" {
			status = "LATE"
		} else if child.Status == "PENDING" && status != "DOWN" && status != "STALE" && status != "LATE" {
			status = "PENDING"
		}
	}
	e.mu.RUnlock()

	if !hasChildren {
		status = "PENDING"
	}

	e.mu.Lock()
	s := e.liveState[site.ID]
	s.Status = status
	if hasChildren && allPaused {
		s.Paused = true
	}
	e.liveState[site.ID] = s
	e.mu.Unlock()
}

func (e *Engine) SetAggStrategy(strategy AggregationStrategy) {
	e.aggStrategy = strategy
}

func (e *Engine) IngestProbeResult(nodeID string, siteID int, latencyNs int64, isUp bool, errorReason string) {
	e.probeResultsMu.Lock()
	if e.probeResults[siteID] == nil {
		e.probeResults[siteID] = make(map[string]NodeResult)
	}
	e.probeResults[siteID][nodeID] = NodeResult{
		NodeID:      nodeID,
		IsUp:        isUp,
		LatencyNs:   latencyNs,
		CheckedAt:   time.Now(),
		ErrorReason: errorReason,
	}
	results := make([]NodeResult, 0, len(e.probeResults[siteID]))
	for _, r := range e.probeResults[siteID] {
		results = append(results, r)
	}
	e.probeResultsMu.Unlock()

	aggUp, avgLatency := AggregateStatus(results, e.aggStrategy)

	e.mu.RLock()
	site, exists := e.liveState[siteID]
	e.mu.RUnlock()
	if !exists {
		return
	}

	rawStatus := "UP"
	if !aggUp {
		rawStatus = "DOWN"
	}

	updatedSite := site
	updatedSite.Latency = time.Duration(avgLatency)
	updatedSite.LastCheck = time.Now()
	e.handleStatusChange(updatedSite, rawStatus, 0, time.Duration(avgLatency), errorReason)
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
	changes, err := e.db.GetStateChanges(siteID, limit)
	if err != nil {
		return nil
	}
	return changes
}

func (e *Engine) GetStateChangesSince(siteID int, since time.Time) []models.StateChange {
	changes, err := e.db.GetStateChangesSince(siteID, since)
	if err != nil {
		return nil
	}
	return changes
}
