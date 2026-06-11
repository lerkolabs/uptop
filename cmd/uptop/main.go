package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/cluster"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/config"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/importer"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/monitor"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/server"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/store"
	"gitea.lerkolabs.com/lerkolabs/uptop/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	bm "github.com/charmbracelet/wish/bubbletea"
	"github.com/mattn/go-isatty"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	log.SetOutput(os.Stderr)

	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "apply":
			runApply(os.Args[2:])
			return
		case "export":
			runExport(os.Args[2:])
			return
		case "version", "--version", "-v":
			printVersion()
			return
		case "migrate-secrets":
			runMigrateSecrets(os.Args[2:])
			return
		}
	}
	runServe(os.Args[1:])
}

func printVersion() {
	if version == "dev" {
		fmt.Println("uptop dev")
	} else {
		fmt.Printf("uptop %s (%s, %s)\n", version, commit, date)
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func redactDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "***"
	}
	u.User = nil
	return u.String()
}

// parseTrustedProxies turns UPTOP_TRUSTED_PROXIES (comma-separated CIDRs or
// bare IPs) into networks the rate limiter trusts to set X-Forwarded-For. Bare
// IPs are treated as single-host ranges. Invalid entries are warned about and
// skipped, so a typo degrades to "ignore XFF" (safe) rather than aborting boot.
func parseTrustedProxies(raw string) []*net.IPNet {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var cidrs []*net.IPNet
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !strings.Contains(part, "/") {
			if ip := net.ParseIP(part); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				part = fmt.Sprintf("%s/%d", part, bits)
			}
		}
		_, ipnet, err := net.ParseCIDR(part)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: ignoring invalid UPTOP_TRUSTED_PROXIES entry %q: %v\n", part, err)
			continue
		}
		cidrs = append(cidrs, ipnet)
	}
	return cidrs
}

func openStore(dbType, dsn string) store.Store {
	var ss *store.SQLStore
	var err error
	if dbType == "postgres" {
		ss, err = store.NewPostgresStore(dsn)
	} else {
		ss, err = store.NewSQLiteStore(dsn)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "database error: %v\n", err)
		os.Exit(1)
	}
	if encKey := os.Getenv("UPTOP_ENCRYPTION_KEY"); encKey != "" {
		enc, err := store.NewEncryptor(encKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "encryption key error: %v\n", err)
			os.Exit(1)
		}
		ss.SetEncryptor(enc)
	} else {
		fmt.Println("WARNING: No UPTOP_ENCRYPTION_KEY set. Alert credentials stored unencrypted.")
	}
	if err := ss.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "database init error: %v\n", err)
		os.Exit(1)
	}
	return ss
}

func runApply(args []string) {
	fs := flag.NewFlagSet("apply", flag.ExitOnError)
	filePath := fs.String("f", "", "Path to YAML config file (required)")
	dryRun := fs.Bool("dry-run", false, "Show planned changes without applying")
	prune := fs.Bool("prune", false, "Delete monitors/alerts not in YAML")
	dbType := fs.String("db-type", envOrDefault("UPTOP_DB_TYPE", "sqlite"), "Database type")
	dsn := fs.String("dsn", envOrDefault("UPTOP_DB_DSN", "uptop.db"), "Database DSN")
	_ = fs.Parse(args) // ExitOnError: parse errors exit before returning

	if *filePath == "" {
		fmt.Fprintln(os.Stderr, "error: -f flag is required")
		fs.Usage()
		os.Exit(1)
	}

	s := openStore(*dbType, *dsn)

	f, err := config.LoadFile(*filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	changes, err := config.Apply(s, f, config.ApplyOpts{
		DryRun: *dryRun,
		Prune:  *prune,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(config.FormatChanges(changes, *dryRun))
}

func runExport(args []string) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	outPath := fs.String("o", "-", "Output file path (- for stdout)")
	dbType := fs.String("db-type", envOrDefault("UPTOP_DB_TYPE", "sqlite"), "Database type")
	dsn := fs.String("dsn", envOrDefault("UPTOP_DB_DSN", "uptop.db"), "Database DSN")
	_ = fs.Parse(args) // ExitOnError: parse errors exit before returning

	s := openStore(*dbType, *dsn)

	f, err := config.Export(s)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := config.WriteFile(f, *outPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runMigrateSecrets(args []string) {
	fs := flag.NewFlagSet("migrate-secrets", flag.ExitOnError)
	dbType := fs.String("db-type", envOrDefault("UPTOP_DB_TYPE", "sqlite"), "Database type")
	dsn := fs.String("dsn", envOrDefault("UPTOP_DB_DSN", "uptop.db"), "Database DSN")
	_ = fs.Parse(args)

	encKey := os.Getenv("UPTOP_ENCRYPTION_KEY")
	if encKey == "" {
		fmt.Fprintln(os.Stderr, "error: UPTOP_ENCRYPTION_KEY must be set")
		os.Exit(1)
	}
	enc, err := store.NewEncryptor(encKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	var ss *store.SQLStore
	if *dbType == "postgres" {
		ss, err = store.NewPostgresStore(*dsn)
	} else {
		ss, err = store.NewSQLiteStore(*dsn)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "database error: %v\n", err)
		os.Exit(1)
	}
	if err := ss.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "database init error: %v\n", err)
		os.Exit(1)
	}

	alerts, err := ss.GetAllAlerts()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading alerts: %v\n", err)
		os.Exit(1)
	}

	ss.SetEncryptor(enc)
	migrated := 0
	for _, a := range alerts {
		if err := ss.UpdateAlert(a.ID, a.Name, a.Type, a.Settings); err != nil {
			fmt.Fprintf(os.Stderr, "error migrating alert %q: %v\n", a.Name, err)
			os.Exit(1)
		}
		migrated++
	}
	fmt.Printf("Migrated %d alert(s) to encrypted storage.\n", migrated)
}

func runServe(args []string) {
	portVal := 23234
	dbType := "sqlite"
	dbDSN := "uptop.db"
	httpPort := 8080
	enableStatus := false
	statusTitle := "System Status"
	clusterMode := "leader"
	clusterPeer := ""
	clusterKey := ""

	if v := os.Getenv("UPTOP_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			portVal = p
		}
	}
	if v := os.Getenv("UPTOP_DB_TYPE"); v != "" {
		dbType = v
	}
	if v := os.Getenv("UPTOP_DB_DSN"); v != "" {
		dbDSN = v
	}
	if v := os.Getenv("UPTOP_HTTP_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			httpPort = p
		}
	}
	if v := os.Getenv("UPTOP_STATUS_ENABLED"); v == "true" {
		enableStatus = true
	}
	if v := os.Getenv("UPTOP_STATUS_TITLE"); v != "" {
		statusTitle = v
	}
	if v := os.Getenv("UPTOP_CLUSTER_MODE"); v != "" {
		clusterMode = v
	}
	if v := os.Getenv("UPTOP_PEER_URL"); v != "" {
		clusterPeer = v
	}
	if v := os.Getenv("UPTOP_CLUSTER_SECRET"); v != "" {
		clusterKey = v
	}

	nodeID := os.Getenv("UPTOP_NODE_ID")
	nodeName := os.Getenv("UPTOP_NODE_NAME")
	nodeRegion := os.Getenv("UPTOP_NODE_REGION")
	aggStrategy := os.Getenv("UPTOP_AGG_STRATEGY")

	if clusterMode == "probe" {
		if nodeID == "" {
			fmt.Fprintln(os.Stderr, "UPTOP_NODE_ID is required for probe mode")
			os.Exit(1)
		}
		if clusterPeer == "" {
			fmt.Fprintln(os.Stderr, "UPTOP_PEER_URL is required for probe mode")
			os.Exit(1)
		}

		fmt.Printf("Cluster: Running as PROBE (node=%s, region=%s)\n", nodeID, nodeRegion)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan os.Signal, 1)
		signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-done
			cancel()
		}()

		probeAllowPrivate := os.Getenv("UPTOP_ALLOW_PRIVATE_TARGETS") == "true"
		if probeAllowPrivate {
			fmt.Println("WARNING: Private target blocking disabled. Monitor URLs can reach internal networks.")
		}

		if err := cluster.RunProbe(ctx, cluster.ProbeConfig{
			NodeID:              nodeID,
			NodeName:            nodeName,
			Region:              nodeRegion,
			LeaderURL:           clusterPeer,
			SharedKey:           clusterKey,
			Interval:            30,
			AllowPrivateTargets: probeAllowPrivate,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "Probe error: %v\n", err)
		}
		return
	}

	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs.Int("port", portVal, "SSH Port")
	flagDBType := fs.String("db-type", dbType, "Database type")
	flagDSN := fs.String("dsn", dbDSN, "Database DSN")
	demo := fs.Bool("demo", false, "Seed demo data")
	importKuma := fs.String("import-kuma", "", "Import Uptime Kuma backup JSON file")
	_ = fs.Parse(args) // ExitOnError: parse errors exit before returning

	var ss *store.SQLStore
	var dbErr error
	if *flagDBType == "postgres" {
		ss, dbErr = store.NewPostgresStore(*flagDSN)
		fmt.Printf("Using PostgreSQL: %s\n", redactDSN(*flagDSN))
	} else {
		ss, dbErr = store.NewSQLiteStore(*flagDSN)
		fmt.Printf("Using SQLite: %s\n", *flagDSN)
	}
	if dbErr != nil {
		fmt.Fprintf(os.Stderr, "database connection error: %v\n", dbErr)
		os.Exit(1)
	}
	defer ss.Close()

	if encKey := os.Getenv("UPTOP_ENCRYPTION_KEY"); encKey != "" {
		enc, err := store.NewEncryptor(encKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "encryption key error: %v\n", err)
			os.Exit(1)
		}
		ss.SetEncryptor(enc)
	} else {
		fmt.Println("WARNING: No UPTOP_ENCRYPTION_KEY set. Alert credentials stored unencrypted.")
	}

	kc := newKeyCache(ss)
	var s store.Store = &userInvalidatingStore{Store: ss, kc: kc}
	if err := s.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "database init error: %v\n", err)
		os.Exit(1)
	}
	if *demo {
		seedDemoData(s)
	}

	seedKeysFromEnv(s)

	if *importKuma != "" {
		kb, err := importer.LoadKumaFile(*importKuma)
		if err != nil {
			fmt.Fprintf(os.Stderr, "kuma import error: %v\n", err)
			os.Exit(1)
		}
		backup := importer.ConvertKuma(kb)
		if err := s.ImportData(backup); err != nil {
			fmt.Fprintf(os.Stderr, "import failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Imported %d monitors and %d alerts from Uptime Kuma v%s\n", len(backup.Sites), len(backup.Alerts), kb.Version)
	}

	allowPrivate := os.Getenv("UPTOP_ALLOW_PRIVATE_TARGETS") == "true"
	if allowPrivate {
		fmt.Println("WARNING: Private target blocking disabled. Monitor URLs can reach internal networks.")
	}

	eng := monitor.NewEngineWithOpts(s, allowPrivate)
	if os.Getenv("UPTOP_INSECURE_SKIP_VERIFY") == "true" {
		eng.SetInsecureSkipVerify(true)
	}
	if aggStrategy != "" {
		eng.SetAggStrategy(monitor.AggregationStrategy(aggStrategy))
	}
	if v := os.Getenv("UPTOP_MAINT_RETENTION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			eng.SetMaintRetention(d)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eng.InitHistory()
	eng.InitLogs()
	eng.InitAlertHealth()
	eng.Start(ctx)

	tlsCert := os.Getenv("UPTOP_TLS_CERT")
	tlsKey := os.Getenv("UPTOP_TLS_KEY")

	// When the local TUI owns the terminal, per-request HTTP logs to stderr
	// would scribble over the alt screen.
	localTUI := isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())

	httpSrv := server.Start(server.ServerConfig{
		Port:           httpPort,
		EnableStatus:   enableStatus,
		Title:          statusTitle,
		ClusterKey:     clusterKey,
		TLSCert:        tlsCert,
		TLSKey:         tlsKey,
		ClusterMode:    clusterMode,
		MetricsPublic:  os.Getenv("UPTOP_METRICS_PUBLIC") == "true",
		CORSOrigin:     os.Getenv("UPTOP_CORS_ORIGIN"),
		TrustedProxies: parseTrustedProxies(os.Getenv("UPTOP_TRUSTED_PROXIES")),
		QuietHTTPLog:   localTUI,
	}, s, eng)

	cluster.Start(ctx, cluster.Config{
		Mode:      clusterMode,
		PeerURL:   clusterPeer,
		SharedKey: clusterKey,
	}, eng)

	sshSrv := startSSHServer(*port, s, eng, kc)

	if localTUI {
		p := tea.NewProgram(tui.InitialModel(true, s, eng, version), tea.WithAltScreen(), tea.WithMouseCellMotion())
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
	} else {
		fmt.Println("uptop running in HEADLESS mode")
		done := make(chan os.Signal, 1)
		signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		<-done
		fmt.Println("Shutting down...")
	}
	cancel()

	// Drain pending DB writes before the deferred ss.Close() runs, so no
	// write races a closed database.
	eng.Stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if httpSrv != nil {
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP shutdown error: %v", err)
		}
	}
	if sshSrv != nil {
		if err := sshSrv.Shutdown(shutdownCtx); err != nil {
			log.Printf("SSH shutdown error: %v", err)
		}
	}
}

func startSSHServer(port int, db store.Store, eng *monitor.Engine, kc *keyCache) *ssh.Server {
	s, err := wish.NewServer(
		wish.WithAddress(fmt.Sprintf(":%d", port)),
		wish.WithHostKeyPath(envOrDefault("UPTOP_SSH_HOST_KEY", ".ssh/id_ed25519")),
		wish.WithPublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool {
			return kc.IsAllowed(key)
		}),
		wish.WithMiddleware(
			bm.Middleware(func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
				return tui.InitialModel(false, db, eng, version), []tea.ProgramOption{tea.WithAltScreen(), tea.WithMouseCellMotion()}
			}),
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SSH server error: %v\n", err)
		return nil
	}
	go func() {
		if err := s.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			log.Printf("SSH server error: %v", err)
		}
	}()
	return s
}

func seedDemoData(s store.Store) {
	existing, _ := s.GetSites()
	if len(existing) > 0 {
		return
	}
	fmt.Println("Seeding demo data...")

	if err := s.AddAlert("Discord Ops", "discord", map[string]string{"url": "https://discord.com/api/webhooks/demo/token"}); err != nil {
		log.Printf("demo seed: add alert: %v", err)
		return
	}
	if err := s.AddAlert("Slack Infra", "slack", map[string]string{"url": "https://hooks.slack.com/services/DEMO/WEBHOOK"}); err != nil {
		log.Printf("demo seed: add alert: %v", err)
		return
	}
	if err := s.AddAlert("Email Oncall", "email", map[string]string{
		"host": "smtp.example.com", "port": "587",
		"user": "oncall@example.com", "pass": "replace-me",
		"from": "oncall@example.com", "to": "team@example.com",
	}); err != nil {
		log.Printf("demo seed: add alert: %v", err)
		return
	}

	alerts, _ := s.GetAllAlerts()
	alertID := 0
	if len(alerts) > 0 {
		alertID = alerts[0].ID
	}

	demoSites := []models.Site{
		{Name: "Google", URL: "https://www.google.com", Type: "http", Interval: 30, AlertID: alertID, CheckSSL: true, ExpiryThreshold: 14, MaxRetries: 2},
		{Name: "GitHub", URL: "https://github.com", Type: "http", Interval: 30, AlertID: alertID, CheckSSL: true, ExpiryThreshold: 7, MaxRetries: 3},
		{Name: "Cloudflare DNS", URL: "https://1.1.1.1", Type: "http", Interval: 60, AlertID: alertID, ExpiryThreshold: 7, MaxRetries: 1},
		{Name: "JSON Placeholder", URL: "https://jsonplaceholder.typicode.com/posts/1", Type: "http", Interval: 45, AlertID: alertID, ExpiryThreshold: 7, MaxRetries: 2},
		{Name: "Nonexistent Site", URL: "https://this-domain-does-not-exist-12345.com", Type: "http", Interval: 30, AlertID: alertID, ExpiryThreshold: 7, MaxRetries: 3},
		{Name: "Bad Port", URL: "https://localhost:19999", Type: "http", Interval: 30, ExpiryThreshold: 7, MaxRetries: 1},
		{Name: "Backup Cron", Type: "push", Interval: 300, AlertID: alertID, ExpiryThreshold: 7},
		{Name: "DB Healthcheck", Type: "push", Interval: 120, AlertID: alertID, ExpiryThreshold: 7},
		{Name: "Gateway", Type: "ping", Interval: 30, AlertID: alertID, Hostname: "10.0.0.1", Timeout: 5, ExpiryThreshold: 7},
		{Name: "SSH Server", Type: "port", Interval: 60, AlertID: alertID, Hostname: "10.0.0.1", Port: 22, Timeout: 5, ExpiryThreshold: 7},
	}
	for _, site := range demoSites {
		if err := s.AddSite(site); err != nil {
			log.Printf("demo seed: add site %q: %v", site.Name, err)
		}
	}
}

type keyCache struct {
	mu      sync.RWMutex
	keys    []ssh.PublicKey
	updated time.Time
	ttl     time.Duration
	db      store.Store
}

func newKeyCache(db store.Store) *keyCache {
	return &keyCache{db: db, ttl: 30 * time.Second}
}

func (c *keyCache) refresh() {
	users, err := c.db.GetAllUsers()
	if err != nil {
		// Keep the previous key set: a transient DB error must not lock every
		// admin out. Revocation still fails closed because Invalidate clears
		// the set immediately.
		log.Printf("SSH key cache refresh failed: %v", err)
		return
	}
	keys := make([]ssh.PublicKey, 0, len(users))
	for _, u := range users {
		k, _, _, _, err := ssh.ParseAuthorizedKey([]byte(u.PublicKey))
		if err != nil {
			continue
		}
		keys = append(keys, k)
	}
	c.mu.Lock()
	c.keys = keys
	c.updated = time.Now()
	c.mu.Unlock()
}

// Invalidate clears the cached key set, not just the timestamp. If the
// refresh that follows a user revocation fails, auth fails closed (everyone
// re-authenticates after the next successful refresh) instead of the revoked
// key silently continuing to work off the stale cache.
func (c *keyCache) Invalidate() {
	c.mu.Lock()
	c.keys = nil
	c.updated = time.Time{}
	c.mu.Unlock()
}

func (c *keyCache) IsAllowed(incomingKey ssh.PublicKey) bool {
	c.mu.RLock()
	stale := time.Since(c.updated) > c.ttl
	c.mu.RUnlock()

	if stale {
		c.refresh()
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, k := range c.keys {
		if ssh.KeysEqual(k, incomingKey) {
			return true
		}
	}
	return false
}

// userInvalidatingStore drops the SSH key cache whenever the user table
// changes, so a revocation takes effect on the next connection attempt
// instead of after the cache TTL — and fails closed if the DB is unreachable
// when that next attempt re-reads the table.
type userInvalidatingStore struct {
	store.Store
	kc *keyCache
}

func (s *userInvalidatingStore) AddUser(username, publicKey, role string) error {
	err := s.Store.AddUser(username, publicKey, role)
	s.kc.Invalidate()
	return err
}

func (s *userInvalidatingStore) UpdateUser(id int, username, publicKey, role string) error {
	err := s.Store.UpdateUser(id, username, publicKey, role)
	s.kc.Invalidate()
	return err
}

func (s *userInvalidatingStore) DeleteUser(id int) error {
	err := s.Store.DeleteUser(id)
	s.kc.Invalidate()
	return err
}

func (s *userInvalidatingStore) ImportData(data models.Backup) error {
	err := s.Store.ImportData(data)
	s.kc.Invalidate()
	return err
}

func seedKeysFromEnv(s store.Store) {
	var keys []string

	if v := os.Getenv("UPTOP_ADMIN_KEY"); v != "" {
		keys = append(keys, strings.TrimSpace(v))
	}

	if path := os.Getenv("UPTOP_KEYS"); path != "" {
		f, err := os.Open(filepath.Clean(path))
		if err == nil {
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				keys = append(keys, line)
			}
			_ = f.Close()
		}
	}

	if len(keys) == 0 {
		return
	}

	existing, err := s.GetAllUsers()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not check existing users: %v\n", err)
		return
	}

	existingKeys := make(map[string]bool)
	for _, u := range existing {
		existingKeys[u.PublicKey] = true
	}

	added := 0
	for i, key := range keys {
		if existingKeys[key] {
			continue
		}

		username := usernameFromKey(key, i, len(existing)+added)
		if err := s.AddUser(username, key, "admin"); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to seed user %q: %v\n", username, err)
			continue
		}
		fmt.Printf("Seeded admin user %q from %s\n", username, seedSource(i, len(keys), os.Getenv("UPTOP_ADMIN_KEY") != ""))
		added++
	}
}

func usernameFromKey(key string, index, totalExisting int) string {
	parts := strings.Fields(key)
	if len(parts) >= 3 {
		comment := parts[2]
		if at := strings.Index(comment, "@"); at > 0 {
			return comment[:at]
		}
		return comment
	}
	if index == 0 && totalExisting == 0 {
		return "admin"
	}
	return fmt.Sprintf("user-%d", totalExisting+1)
}

func seedSource(index, total int, hasEnvKey bool) string {
	if hasEnvKey && index == 0 {
		return "UPTOP_ADMIN_KEY"
	}
	return "UPTOP_KEYS"
}
