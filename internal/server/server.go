package server

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/importer"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/metrics"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/monitor"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/store"
)

const maxRequestBody = 1 << 20

func checkSecret(got, want string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

// Alert-settings redaction policy lives in models.RedactAlertSettings so the
// TUI detail panel and this export path share one allowlist.

var statusTpl = template.Must(template.New("status").Parse(`
<!DOCTYPE html>
<html>
<head>
	<title>{{.Title}}</title>
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<style>
		body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; background: #1a1b26; color: #a9b1d6; padding: 20px; margin: 0; }
		h1 { text-align: center; color: #7aa2f7; margin-bottom: 30px; }
		.container { max-width: 800px; margin: 0 auto; }
		.card { background: #24283b; padding: 20px; margin-bottom: 15px; border-radius: 8px; display: flex; align-items: center; justify-content: space-between; box-shadow: 0 4px 6px rgba(0,0,0,0.1); }
		.info { display: flex; flex-direction: column; }
		.name { font-size: 1.2em; font-weight: bold; color: #c0caf5; margin-bottom: 5px; }
		.meta { font-size: 0.85em; color: #565f89; }
		.status { font-weight: bold; padding: 6px 12px; border-radius: 6px; min-width: 60px; text-align: center; }
		.UP { background: #9ece6a; color: #1a1b26; }
		.DOWN { background: #f7768e; color: #1a1b26; }
		.PENDING { background: #e0af68; color: #1a1b26; }
		.LATE { background: #e0af68; color: #1a1b26; }
		.SSL-EXP { background: #e0af68; color: #1a1b26; }
		.PAUSED { background: #565f89; color: #c0caf5; }
		.MAINT { background: #bb9af7; color: #1a1b26; }
		.summary { display: flex; justify-content: center; gap: 16px; margin-bottom: 24px; font-size: 0.95em; font-weight: 600; }
		.summary span { padding: 4px 12px; border-radius: 6px; }
		.summary .s-up { color: #9ece6a; }
		.summary .s-down { color: #f7768e; }
		.summary .s-paused { color: #565f89; }
		.summary .s-total { color: #7aa2f7; }
		.stale-bar { text-align: center; font-size: 0.8em; color: #565f89; margin-bottom: 16px; transition: color 0.3s; }
		.stale-bar.warn { color: #e0af68; }
		.stale-bar.error { color: #f7768e; }
	</style>
</head>
<body>
	<div class="container">
		<h1>{{.Title}}</h1>
		<div id="summary" class="summary"></div>
		<div id="stale" class="stale-bar"></div>
		<div id="cards"></div>
		<div style="text-align: center; margin-top: 40px; color: #565f89; font-size: 0.8em;">Powered by uptop</div>
	</div>
	<script>
		var lastUpdate = null;

		function esc(s) {
			var d = document.createElement('div');
			d.appendChild(document.createTextNode(s));
			return d.innerHTML;
		}

		function cssClass(status) {
			return status.replace(/\s+/g, '-');
		}

		function renderSummary(sites) {
			var up = 0, down = 0, paused = 0, maint = 0, total = sites.length;
			for (var i = 0; i < sites.length; i++) {
				if (sites[i].Paused) { paused++; continue; }
				if (sites[i].Status === 'MAINT') { maint++; continue; }
				if (sites[i].Status === 'UP') up++;
				else if (sites[i].Status === 'DOWN') down++;
			}
			var el = document.getElementById('summary');
			var parts = ['<span class="s-total">' + up + '/' + total + ' UP</span>'];
			if (down > 0) parts.push('<span class="s-down">' + down + ' DOWN</span>');
			if (maint > 0) parts.push('<span style="color:#bb9af7">' + maint + ' MAINT</span>');
			if (paused > 0) parts.push('<span class="s-paused">' + paused + ' PAUSED</span>');
			el.innerHTML = parts.join('<span style="color:#383838">·</span>');
		}

		function renderStale() {
			var el = document.getElementById('stale');
			if (!lastUpdate) { el.textContent = ''; return; }
			var ago = Math.round((Date.now() - lastUpdate) / 1000);
			el.className = 'stale-bar';
			if (ago < 10) {
				el.textContent = 'Updated just now';
			} else if (ago < 30) {
				el.textContent = 'Updated ' + ago + 's ago';
				el.className = 'stale-bar warn';
			} else {
				el.textContent = 'Stale — last update ' + ago + 's ago';
				el.className = 'stale-bar error';
			}
		}

		function render(sites) {
			var c = document.getElementById('cards');
			var html = '';
			sites.sort(function(a, b) {
				if (a.Status !== b.Status) {
					if (a.Status === 'DOWN') return -1;
					if (b.Status === 'DOWN') return 1;
				}
				return a.Name < b.Name ? -1 : a.Name > b.Name ? 1 : 0;
			});
			renderSummary(sites);
			for (var i = 0; i < sites.length; i++) {
				var s = sites[i];
				var st = s.Status === 'MAINT' ? 'MAINT' : s.Paused ? 'PAUSED' : s.Status;
				var cls = cssClass(st);
				var meta = esc(s.Type) + ' | ' + (s.Type === 'http' ? esc(s.URL) : 'Heartbeat Monitor');
				var lc = s.LastCheck ? new Date(s.LastCheck).toLocaleTimeString('en-GB', {hour12: false}) : '—';
				html += '<div class="card"><div class="info">' +
					'<div class="name">' + esc(s.Name) + '</div>' +
					'<div class="meta">' + meta + '</div>' +
					'<div class="meta" style="margin-top:4px;">Last Check: ' + lc + '</div>' +
					'</div><div class="status ' + cls + '">' + esc(st) + '</div></div>';
			}
			c.innerHTML = html;
		}

		function refresh() {
			fetch('/status/json')
				.then(function(r) { return r.json(); })
				.then(function(data) {
					var sites = [];
					for (var k in data) sites.push(data[k]);
					lastUpdate = Date.now();
					render(sites);
				})
				.catch(function() {});
			renderStale();
			setTimeout(refresh, 5000);
		}

		setInterval(renderStale, 1000);
		refresh();
	</script>
</body>
</html>`))

type ServerConfig struct {
	Port           int
	EnableStatus   bool
	Title          string
	ClusterKey     string
	TLSCert        string
	TLSKey         string
	ClusterMode    string
	MetricsPublic  bool
	CORSOrigin     string
	TrustedProxies []*net.IPNet
	// QuietHTTPLog disables per-request stderr logging. Set when the local
	// TUI owns the terminal — request logs would scribble over the alt screen.
	QuietHTTPLog bool
}

// statusSite is the public DTO for /status/json. models.Site must never be
// serialized raw here: it carries internal fields (LastError, Hostname, Port,
// DNSServer, AlertID, Token, ...) and every field added to it would become
// public by default. Field names match what the status page JS reads.
type statusSite struct {
	Name      string
	Type      string
	URL       string
	Status    string
	Paused    bool
	LastCheck time.Time
	Latency   time.Duration
}

func Start(cfg ServerConfig, s store.Store, eng *monitor.Engine) *http.Server {
	if cfg.ClusterKey == "" {
		fmt.Println("WARNING: No UPTOP_CLUSTER_SECRET set. Cluster API endpoints are unauthenticated.")
	}

	pushRL := NewRateLimiter(60, cfg.TrustedProxies)
	probeRL := NewRateLimiter(30, cfg.TrustedProxies)
	backupRL := NewRateLimiter(10, cfg.TrustedProxies)
	statusRL := NewRateLimiter(120, cfg.TrustedProxies)

	mux := http.NewServeMux()

	// 1. Push Heartbeat
	mux.HandleFunc("/api/push", RateLimit(pushRL, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		token := extractBearerToken(r)
		if token == "" {
			if qt := r.URL.Query().Get("token"); qt != "" {
				token = qt
				log.Printf("DEPRECATED: push token in query string — use Authorization: Bearer header instead")
			}
		}
		if token == "" {
			http.Error(w, "Missing token", http.StatusBadRequest)
			return
		}
		if eng.RecordHeartbeat(token) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		} else {
			http.Error(w, "Invalid Token", http.StatusNotFound)
		}
	}))

	// 2. Health Check (For Cluster Follower)
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if cfg.ClusterKey != "" && !checkSecret(r.Header.Get("X-Upkeep-Secret"), cfg.ClusterKey) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// 3. Config Export
	mux.HandleFunc("/api/backup/export", RateLimit(backupRL, func(w http.ResponseWriter, r *http.Request) {
		if cfg.ClusterKey == "" || !checkSecret(r.Header.Get("X-Upkeep-Secret"), cfg.ClusterKey) {
			http.Error(w, "Unauthorized: UPTOP_CLUSTER_SECRET required", http.StatusUnauthorized)
			return
		}
		data, err := s.ExportData()
		if err != nil {
			log.Printf("Export failed: %v", err)
			http.Error(w, "Export failed", http.StatusInternalServerError)
			return
		}
		if r.URL.Query().Get("redact_secrets") != "false" {
			for i := range data.Alerts {
				data.Alerts[i].Settings = models.RedactAlertSettings(data.Alerts[i].Type, data.Alerts[i].Settings)
			}
		}
		_ = json.NewEncoder(w).Encode(data) //nolint:errcheck
	}))

	// 4. Config Import
	mux.HandleFunc("/api/backup/import", RateLimit(backupRL, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		if cfg.ClusterKey == "" || !checkSecret(r.Header.Get("X-Upkeep-Secret"), cfg.ClusterKey) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
		var data models.Backup
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		if err := s.ImportData(data); err != nil {
			log.Printf("Import failed: %v", err)
			http.Error(w, "Import failed", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("Import Successful"))
	}))

	// 5. Kuma Import
	mux.HandleFunc("/api/import/kuma", RateLimit(backupRL, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		if cfg.ClusterKey == "" || !checkSecret(r.Header.Get("X-Upkeep-Secret"), cfg.ClusterKey) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
		var kb importer.KumaBackup
		if err := json.NewDecoder(r.Body).Decode(&kb); err != nil {
			log.Printf("Invalid Kuma JSON: %v", err)
			http.Error(w, "Invalid Kuma JSON", http.StatusBadRequest)
			return
		}
		backup := importer.ConvertKuma(&kb)
		if err := s.ImportData(backup); err != nil {
			log.Printf("Kuma import failed: %v", err)
			http.Error(w, "Import failed", http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "Imported %d monitors, %d alerts from Kuma v%s", len(backup.Sites), len(backup.Alerts), kb.Version)
	}))

	// 6. Probe Registration
	mux.HandleFunc("/api/probe/register", RateLimit(probeRL, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		if cfg.ClusterKey == "" || !checkSecret(r.Header.Get("X-Upkeep-Secret"), cfg.ClusterKey) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
		var req struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Region  string `json:"region"`
			Version string `json:"version"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		if req.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		if err := s.RegisterNode(models.ProbeNode{
			ID: req.ID, Name: req.Name, Region: req.Region, Version: req.Version,
		}); err != nil {
			log.Printf("Probe register failed: %v", err)
			http.Error(w, "Registration failed", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true}) //nolint:errcheck
	}))

	// 7. Probe Assignment Fetch
	mux.HandleFunc("/api/probe/assignments", RateLimit(probeRL, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if cfg.ClusterKey == "" || !checkSecret(r.Header.Get("X-Upkeep-Secret"), cfg.ClusterKey) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		nodeID := r.URL.Query().Get("node_id")
		var nodeRegion string
		if nodeID != "" {
			if node, err := s.GetNode(nodeID); err == nil {
				nodeRegion = node.Region
			}
		}
		sites := eng.GetAllSites()
		var assigned []models.Site
		for _, site := range sites {
			if site.Paused || site.Type == "push" || site.Type == "group" {
				continue
			}
			if site.Regions != "" && nodeRegion != "" {
				matched := false
				for _, r := range strings.Split(site.Regions, ",") {
					if strings.TrimSpace(r) == nodeRegion {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
			assigned = append(assigned, site)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string][]models.Site{"sites": assigned}) //nolint:errcheck
	}))

	// 8. Probe Result Submission
	mux.HandleFunc("/api/probe/results", RateLimit(probeRL, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		if cfg.ClusterKey == "" || !checkSecret(r.Header.Get("X-Upkeep-Secret"), cfg.ClusterKey) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
		var req struct {
			NodeID  string `json:"node_id"`
			Results []struct {
				SiteID      int    `json:"site_id"`
				LatencyNs   int64  `json:"latency_ns"`
				IsUp        bool   `json:"is_up"`
				ErrorReason string `json:"error_reason"`
			} `json:"results"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		if req.NodeID == "" {
			http.Error(w, "node_id is required", http.StatusBadRequest)
			return
		}
		for _, result := range req.Results {
			if err := s.SaveCheckFromNode(result.SiteID, req.NodeID, result.LatencyNs, result.IsUp); err != nil {
				log.Printf("Failed to save probe result: %v", err)
			}
			eng.IngestProbeResult(req.NodeID, result.SiteID, result.LatencyNs, result.IsUp, result.ErrorReason)
		}
		if err := s.UpdateNodeLastSeen(req.NodeID); err != nil {
			log.Printf("Failed to update node last seen: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true}) //nolint:errcheck
	}))

	// 9. Prometheus Metrics
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !cfg.MetricsPublic && cfg.ClusterKey != "" {
			if !checkSecret(r.Header.Get("X-Upkeep-Secret"), cfg.ClusterKey) {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}
		metrics.Handler(eng)(w, r)
	})

	// 10. Status Page
	if cfg.EnableStatus {
		mux.HandleFunc("/status", RateLimit(statusRL, func(w http.ResponseWriter, r *http.Request) { renderStatusPage(w, cfg.Title, eng) }))
		mux.HandleFunc("/status/json", RateLimit(statusRL, func(w http.ResponseWriter, r *http.Request) {
			state := eng.GetLiveState()
			activeWindows, _ := s.GetActiveMaintenanceWindows()
			maintSet := make(map[int]bool)
			allInMaint := false
			for _, mw := range activeWindows {
				if mw.Type != "maintenance" {
					continue
				}
				if mw.MonitorID == 0 {
					allInMaint = true
				} else {
					maintSet[mw.MonitorID] = true
				}
			}
			public := make(map[int]statusSite, len(state))
			for id, site := range state {
				status := site.Status
				if allInMaint || maintSet[site.ID] || (site.ParentID > 0 && maintSet[site.ParentID]) {
					status = "MAINT"
				}
				public[id] = statusSite{
					Name:      site.Name,
					Type:      site.Type,
					URL:       site.URL,
					Status:    status,
					Paused:    site.Paused,
					LastCheck: site.LastCheck,
					Latency:   site.Latency,
				}
			}
			if cfg.CORSOrigin != "" {
				w.Header().Set("Access-Control-Allow-Origin", cfg.CORSOrigin)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(public) //nolint:errcheck
		}))
	}

	if cfg.ClusterMode != "" && cfg.ClusterMode != "leader" && cfg.TLSCert == "" {
		fmt.Println("WARNING: Cluster mode active without TLS. Secrets transmitted in cleartext.")
	}

	handler := securityHeadersMiddleware(mux)
	if !cfg.QuietHTTPLog {
		handler = loggingMiddleware(cfg.TrustedProxies, handler)
	}
	if cfg.TLSCert != "" {
		handler = hstsMiddleware(handler)
	}

	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		if cfg.TLSCert != "" && cfg.TLSKey != "" {
			fmt.Printf("HTTPS Server listening on %s\n", addr)
			if err := srv.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey); err != nil && err != http.ErrServerClosed {
				log.Printf("HTTPS server error: %v", err)
			}
		} else {
			fmt.Printf("HTTP Server listening on %s\n", addr)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("HTTP server error: %v", err)
			}
		}
	}()
	return srv
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

func loggingMiddleware(trusted []*net.IPNet, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: 200}
		next.ServeHTTP(sw, r)
		path := strings.ReplaceAll(strings.ReplaceAll(r.URL.Path, "\n", ""), "\r", "")
		log.Printf("%s %s %d %s %s", r.Method, path, sw.code, time.Since(start).Round(time.Millisecond), clientIP(r, trusted)) //nolint:gosec // path sanitized above
	})
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline'")
		next.ServeHTTP(w, r)
	})
}

func hstsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}

func renderStatusPage(w http.ResponseWriter, title string, eng *monitor.Engine) {
	sites := eng.GetAllSites()

	sort.Slice(sites, func(i, j int) bool {
		if sites[i].Status != sites[j].Status {
			if sites[i].Status == "DOWN" {
				return true
			}
			if sites[j].Status == "DOWN" {
				return false
			}
		}
		return sites[i].Name < sites[j].Name
	})

	data := struct {
		Title string
		Sites []models.Site
	}{Title: title, Sites: sites}
	if err := statusTpl.Execute(w, data); err != nil {
		log.Printf("Failed to render status page: %v", err)
	}
}
