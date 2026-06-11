package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
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
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

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
			slog.Warn("ignoring invalid UPTOP_TRUSTED_PROXIES entry", "entry", part, "err", err) //nolint:gosec // structured slog, not format string
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
		slog.Error("database connection failed", "err", err)
		os.Exit(1)
	}
	if encKey := os.Getenv("UPTOP_ENCRYPTION_KEY"); encKey != "" {
		enc, err := store.NewEncryptor(encKey)
		if err != nil {
			slog.Error("encryption key invalid", "err", err)
			os.Exit(1)
		}
		ss.SetEncryptor(enc)
	} else {
		slog.Warn("no UPTOP_ENCRYPTION_KEY set, alert credentials stored unencrypted")
	}
	if err := ss.Init(context.Background()); err != nil {
		slog.Error("database init failed", "err", err)
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
		slog.Error("config load failed", "err", err)
		os.Exit(1)
	}

	changes, err := config.Apply(context.Background(), s, f, config.ApplyOpts{
		DryRun: *dryRun,
		Prune:  *prune,
	})
	if err != nil {
		slog.Error("config apply failed", "err", err)
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

	f, err := config.Export(context.Background(), s)
	if err != nil {
		slog.Error("export failed", "err", err)
		os.Exit(1)
	}

	if err := config.WriteFile(f, *outPath); err != nil {
		slog.Error("export write failed", "err", err)
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
		slog.Error("encryption key invalid", "err", err)
		os.Exit(1)
	}

	var ss *store.SQLStore
	if *dbType == "postgres" {
		ss, err = store.NewPostgresStore(*dsn)
	} else {
		ss, err = store.NewSQLiteStore(*dsn)
	}
	if err != nil {
		slog.Error("database connection failed", "err", err)
		os.Exit(1)
	}
	if err := ss.Init(context.Background()); err != nil {
		slog.Error("database init failed", "err", err)
		os.Exit(1)
	}

	alerts, err := ss.GetAllAlerts(context.Background())
	if err != nil {
		slog.Error("failed to load alerts", "err", err)
		os.Exit(1)
	}

	ss.SetEncryptor(enc)
	migrated := 0
	for _, a := range alerts {
		if err := ss.UpdateAlert(context.Background(), a.ID, a.Name, a.Type, a.Settings); err != nil {
			slog.Error("alert migration failed", "alert", a.Name, "err", err)
			os.Exit(1)
		}
		migrated++
	}
	fmt.Printf("Migrated %d alert(s) to encrypted storage.\n", migrated)
}

func runServe(args []string) {
	cfg := parseConfig()

	if cfg.ClusterMode == "probe" {
		if cfg.NodeID == "" {
			fmt.Fprintln(os.Stderr, "UPTOP_NODE_ID is required for probe mode")
			os.Exit(1)
		}
		if cfg.PeerURL == "" {
			fmt.Fprintln(os.Stderr, "UPTOP_PEER_URL is required for probe mode")
			os.Exit(1)
		}

		fmt.Printf("Cluster: Running as PROBE (node=%s, region=%s)\n", cfg.NodeID, cfg.NodeRegion)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan os.Signal, 1)
		signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-done
			cancel()
		}()

		if cfg.AllowPrivateTargets {
			slog.Warn("private target blocking disabled, monitor URLs can reach internal networks")
		}

		if err := cluster.RunProbe(ctx, cluster.ProbeConfig{
			NodeID:              cfg.NodeID,
			NodeName:            cfg.NodeName,
			Region:              cfg.NodeRegion,
			LeaderURL:           cfg.PeerURL,
			SharedKey:           cfg.ClusterSecret,
			Interval:            30,
			AllowPrivateTargets: cfg.AllowPrivateTargets,
		}); err != nil {
			slog.Error("probe failed", "err", err)
		}
		return
	}

	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs.Int("port", cfg.Port, "SSH Port")
	flagDBType := fs.String("db-type", cfg.DBType, "Database type")
	flagDSN := fs.String("dsn", cfg.DBDSN, "Database DSN")
	demo := fs.Bool("demo", false, "Seed demo data")
	importKuma := fs.String("import-kuma", "", "Import Uptime Kuma backup JSON file")
	_ = fs.Parse(args) // ExitOnError: parse errors exit before returning

	var ss *store.SQLStore
	var dbErr error
	if *flagDBType == "postgres" {
		ss, dbErr = store.NewPostgresStore(*flagDSN)
		slog.Info("database connected", "type", "postgres", "dsn", redactDSN(*flagDSN))
	} else {
		ss, dbErr = store.NewSQLiteStore(*flagDSN)
		slog.Info("database connected", "type", "sqlite", "dsn", *flagDSN)
	}
	if dbErr != nil {
		slog.Error("database connection failed", "err", dbErr)
		os.Exit(1)
	}
	defer ss.Close()

	if cfg.EncryptionKey != "" {
		enc, err := store.NewEncryptor(cfg.EncryptionKey)
		if err != nil {
			slog.Error("encryption key invalid", "err", err)
			os.Exit(1)
		}
		ss.SetEncryptor(enc)
	} else {
		slog.Warn("no UPTOP_ENCRYPTION_KEY set, alert credentials stored unencrypted")
	}

	kc := newKeyCache(ss)
	var s store.Store = &userInvalidatingStore{Store: ss, kc: kc}
	if err := s.Init(context.Background()); err != nil {
		slog.Error("database init failed", "err", err)
		os.Exit(1)
	}
	if *demo {
		seedDemoData(s)
	}

	seedKeysFromEnv(s)

	if *importKuma != "" {
		kb, err := importer.LoadKumaFile(*importKuma)
		if err != nil {
			slog.Error("kuma import failed", "err", err)
			os.Exit(1)
		}
		backup := importer.ConvertKuma(kb)
		if err := s.ImportData(context.Background(), backup); err != nil {
			slog.Error("import failed", "err", err)
			os.Exit(1)
		}
		fmt.Printf("Imported %d monitors and %d alerts from Uptime Kuma v%s\n", len(backup.Sites), len(backup.Alerts), kb.Version)
	}

	if cfg.AllowPrivateTargets {
		slog.Warn("private target blocking disabled, monitor URLs can reach internal networks")
	}

	eng := monitor.NewEngineWithOpts(s, cfg.AllowPrivateTargets)
	if cfg.InsecureSkipVerify {
		eng.SetInsecureSkipVerify(true)
	}
	if cfg.AggStrategy != "" {
		eng.SetAggStrategy(monitor.AggregationStrategy(cfg.AggStrategy))
	}
	eng.SetMaintRetention(cfg.MaintRetention)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eng.InitHistory()
	eng.InitLogs()
	eng.InitAlertHealth()
	eng.Start(ctx)

	localTUI := isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())

	httpSrv := server.Start(cfg.serverConfig(localTUI), s, eng)

	cluster.Start(ctx, cluster.Config{
		Mode:      cfg.ClusterMode,
		PeerURL:   cfg.PeerURL,
		SharedKey: cfg.ClusterSecret,
	}, eng)

	sshSrv := startSSHServer(*port, s, eng, kc)

	if localTUI {
		p := tea.NewProgram(tui.InitialModel(true, s, eng, version), tea.WithAltScreen(), tea.WithMouseCellMotion())
		if _, err := p.Run(); err != nil {
			slog.Error("TUI failed", "err", err)
		}
	} else {
		fmt.Println("uptop running in HEADLESS mode")
		done := make(chan os.Signal, 1)
		signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		<-done
		fmt.Println("Shutting down...")
	}
	cancel()

	eng.Stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if httpSrv != nil {
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			slog.Error("HTTP shutdown failed", "err", err)
		}
	}
	if sshSrv != nil {
		if err := sshSrv.Shutdown(shutdownCtx); err != nil {
			slog.Error("SSH shutdown failed", "err", err)
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
		slog.Error("SSH server failed", "err", err)
		return nil
	}
	go func() {
		if err := s.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			slog.Error("SSH server failed", "err", err)
		}
	}()
	return s
}

func seedDemoData(s store.Store) {
	ctx := context.Background()
	existing, _ := s.GetSites(ctx)
	if len(existing) > 0 {
		return
	}
	fmt.Println("Seeding demo data...")

	if err := s.AddAlert(ctx, "Discord Ops", "discord", map[string]string{"url": "https://discord.com/api/webhooks/demo/token"}); err != nil {
		slog.Error("demo seed failed", "step", "add alert", "err", err)
		return
	}
	if err := s.AddAlert(ctx, "Slack Infra", "slack", map[string]string{"url": "https://hooks.slack.com/services/DEMO/WEBHOOK"}); err != nil {
		slog.Error("demo seed failed", "step", "add alert", "err", err)
		return
	}
	if err := s.AddAlert(ctx, "Email Oncall", "email", map[string]string{
		"host": "smtp.example.com", "port": "587",
		"user": "oncall@example.com", "pass": "replace-me",
		"from": "oncall@example.com", "to": "team@example.com",
	}); err != nil {
		slog.Error("demo seed failed", "step", "add alert", "err", err)
		return
	}

	alerts, _ := s.GetAllAlerts(ctx)
	alertID := 0
	if len(alerts) > 0 {
		alertID = alerts[0].ID
	}

	demoSites := []models.SiteConfig{
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
		if err := s.AddSite(ctx, site); err != nil {
			slog.Error("demo seed failed", "step", "add site", "site", site.Name, "err", err)
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
	users, err := c.db.GetAllUsers(context.Background())
	if err != nil {
		// Keep the previous key set: a transient DB error must not lock every
		// admin out. Revocation still fails closed because Invalidate clears
		// the set immediately.
		slog.Error("SSH key cache refresh failed", "err", err)
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

func (s *userInvalidatingStore) AddUser(ctx context.Context, username, publicKey, role string) error {
	err := s.Store.AddUser(ctx, username, publicKey, role)
	s.kc.Invalidate()
	return err
}

func (s *userInvalidatingStore) UpdateUser(ctx context.Context, id int, username, publicKey, role string) error {
	err := s.Store.UpdateUser(ctx, id, username, publicKey, role)
	s.kc.Invalidate()
	return err
}

func (s *userInvalidatingStore) DeleteUser(ctx context.Context, id int) error {
	err := s.Store.DeleteUser(ctx, id)
	s.kc.Invalidate()
	return err
}

func (s *userInvalidatingStore) ImportData(ctx context.Context, data models.Backup) error {
	err := s.Store.ImportData(ctx, data)
	s.kc.Invalidate()
	return err
}

func seedKeysFromEnv(s store.Store) {
	ctx := context.Background()
	var keys []string

	if v := os.Getenv("UPTOP_ADMIN_KEY"); v != "" {
		keys = append(keys, strings.TrimSpace(v))
	}

	if path := os.Getenv("UPTOP_KEYS"); path != "" {
		f, err := os.Open(filepath.Clean(path))
		if err != nil {
			slog.Warn("failed to open UPTOP_KEYS file", "path", path, "err", err) //nolint:gosec // structured slog, not format string
		} else {
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

	existing, err := s.GetAllUsers(ctx)
	if err != nil {
		slog.Warn("could not check existing users", "err", err)
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
		if err := s.AddUser(ctx, username, key, "admin"); err != nil {
			slog.Warn("failed to seed user", "user", username, "err", err) //nolint:gosec // structured slog, not format string
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
