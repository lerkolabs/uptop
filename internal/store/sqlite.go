package store

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"

	_ "modernc.org/sqlite"
)

type SQLiteDialect struct{}

func NewSQLiteStore(path string) (*SQLStore, error) {
	// Apply pragmas via the DSN so every pooled connection gets them — a
	// post-open PRAGMA Exec only affects a single connection. WAL allows
	// concurrent readers alongside the single writer goroutine; busy_timeout
	// rides out brief lock contention; synchronous=NORMAL is durable under WAL
	// and far faster than the FULL default. (:memory: is left untouched —
	// these pragmas are no-ops or harmful for the in-memory test DB.)
	dsn := path
	if path != ":memory:" {
		dsn = fmt.Sprintf("file:%s?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)&_pragma=synchronous(normal)", path)
	}
	s, err := NewSQLStore("sqlite", dsn, &SQLiteDialect{})
	if err != nil {
		return nil, err
	}
	if path != ":memory:" {
		for _, suffix := range []string{"", "-wal", "-shm"} {
			if err := os.Chmod(path+suffix, 0600); err != nil && !os.IsNotExist(err) {
				slog.Warn("failed to chmod database file", "path", path+suffix, "err", err)
			}
		}
	}
	return s, nil
}

func (d *SQLiteDialect) DriverName() string   { return "sqlite" }
func (d *SQLiteDialect) BoolFalse() string    { return "0" }
func (d *SQLiteDialect) BaselineVersion() int { return 13 }

func (d *SQLiteDialect) CreateTablesSQL() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS alerts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT, type TEXT, settings TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS sites (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT DEFAULT 'New Monitor', url TEXT, type TEXT DEFAULT 'http',
			token TEXT, interval INTEGER, alert_id INTEGER,
			check_ssl BOOLEAN DEFAULT 0, threshold INTEGER DEFAULT 7,
			max_retries INTEGER DEFAULT 0, hostname TEXT DEFAULT '',
			port INTEGER DEFAULT 0, timeout INTEGER DEFAULT 0,
			method TEXT DEFAULT 'GET', description TEXT DEFAULT '',
			parent_id INTEGER DEFAULT 0, accepted_codes TEXT DEFAULT '200-299',
			dns_resolve_type TEXT DEFAULT '', dns_server TEXT DEFAULT '',
			ignore_tls BOOLEAN DEFAULT 0, paused BOOLEAN DEFAULT 0,
			regions TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL, public_key TEXT NOT NULL,
			role TEXT DEFAULT 'user'
		)`,
		`CREATE TABLE IF NOT EXISTS check_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			site_id INTEGER NOT NULL, latency_ns INTEGER,
			is_up BOOLEAN, checked_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			node_id TEXT DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_check_history_site ON check_history(site_id, checked_at DESC)`,
		`CREATE TABLE IF NOT EXISTS nodes (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			region TEXT DEFAULT '',
			last_seen DATETIME DEFAULT CURRENT_TIMESTAMP,
			version TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			message TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS maintenance_windows (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			monitor_id INTEGER DEFAULT 0,
			title TEXT NOT NULL,
			description TEXT DEFAULT '',
			type TEXT DEFAULT 'maintenance',
			start_time DATETIME NOT NULL,
			end_time DATETIME,
			created_by TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS preferences (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS state_changes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			site_id INTEGER NOT NULL,
			from_status TEXT NOT NULL,
			to_status TEXT NOT NULL,
			error_reason TEXT DEFAULT '',
			changed_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_state_changes_site ON state_changes(site_id, changed_at DESC)`,
		`CREATE TABLE IF NOT EXISTS alert_health (
			alert_id INTEGER PRIMARY KEY,
			last_send_at DATETIME,
			last_send_ok BOOLEAN DEFAULT 0,
			last_error TEXT DEFAULT '',
			send_count INTEGER DEFAULT 0,
			fail_count INTEGER DEFAULT 0
		)`,
	}
}

func (d *SQLiteDialect) Migrations() []Migration {
	return []Migration{
		{1, "ALTER TABLE sites ADD COLUMN hostname TEXT DEFAULT ''"},
		{2, "ALTER TABLE sites ADD COLUMN port INTEGER DEFAULT 0"},
		{3, "ALTER TABLE sites ADD COLUMN timeout INTEGER DEFAULT 0"},
		{4, "ALTER TABLE sites ADD COLUMN method TEXT DEFAULT 'GET'"},
		{5, "ALTER TABLE sites ADD COLUMN description TEXT DEFAULT ''"},
		{6, "ALTER TABLE sites ADD COLUMN parent_id INTEGER DEFAULT 0"},
		{7, "ALTER TABLE sites ADD COLUMN accepted_codes TEXT DEFAULT '200-299'"},
		{8, "ALTER TABLE sites ADD COLUMN dns_resolve_type TEXT DEFAULT ''"},
		{9, "ALTER TABLE sites ADD COLUMN dns_server TEXT DEFAULT ''"},
		{10, "ALTER TABLE sites ADD COLUMN ignore_tls BOOLEAN DEFAULT 0"},
		{11, "ALTER TABLE sites ADD COLUMN paused BOOLEAN DEFAULT 0"},
		{12, "ALTER TABLE check_history ADD COLUMN node_id TEXT DEFAULT ''"},
		{13, "ALTER TABLE sites ADD COLUMN regions TEXT DEFAULT ''"},
	}
}

func (d *SQLiteDialect) UpsertNodeSQL() string {
	return "INSERT OR REPLACE INTO nodes (id, name, region, last_seen, version) VALUES (?, ?, ?, CURRENT_TIMESTAMP, ?)"
}

func (d *SQLiteDialect) UpsertAlertHealthSQL() string {
	return "INSERT OR REPLACE INTO alert_health (alert_id, last_send_at, last_send_ok, last_error, send_count, fail_count) VALUES (?, ?, ?, ?, ?, ?)"
}

func (d *SQLiteDialect) ResetSequenceOnEmpty(db *sql.DB, table string) {
	var count int
	_ = db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count) //nolint:errcheck
	if count == 0 {
		if _, err := db.Exec("DELETE FROM sqlite_sequence WHERE name=?", table); err != nil {
			slog.Debug("sequence cleanup failed", "table", table, "err", err)
		}
	}
}

func (d *SQLiteDialect) ImportWipe(tx *sql.Tx) {
	if _, err := tx.Exec("DELETE FROM sites"); err != nil {
		slog.Debug("import wipe failed", "table", "sites", "err", err)
	}
	if _, err := tx.Exec("DELETE FROM sqlite_sequence WHERE name='sites'"); err != nil {
		slog.Debug("import wipe failed", "table", "sqlite_sequence(sites)", "err", err)
	}
	if _, err := tx.Exec("DELETE FROM alerts"); err != nil {
		slog.Debug("import wipe failed", "table", "alerts", "err", err)
	}
	if _, err := tx.Exec("DELETE FROM sqlite_sequence WHERE name='alerts'"); err != nil {
		slog.Debug("import wipe failed", "table", "sqlite_sequence(alerts)", "err", err)
	}
	if _, err := tx.Exec("DELETE FROM users"); err != nil {
		slog.Debug("import wipe failed", "table", "users", "err", err)
	}
	if _, err := tx.Exec("DELETE FROM sqlite_sequence WHERE name='users'"); err != nil {
		slog.Debug("import wipe failed", "table", "sqlite_sequence(users)", "err", err)
	}
	if _, err := tx.Exec("DELETE FROM maintenance_windows"); err != nil {
		slog.Debug("import wipe failed", "table", "maintenance_windows", "err", err)
	}
	if _, err := tx.Exec("DELETE FROM sqlite_sequence WHERE name='maintenance_windows'"); err != nil {
		slog.Debug("import wipe failed", "table", "sqlite_sequence(maintenance_windows)", "err", err)
	}
	if _, err := tx.Exec("DELETE FROM check_history"); err != nil {
		slog.Debug("import wipe failed", "table", "check_history", "err", err)
	}
	if _, err := tx.Exec("DELETE FROM state_changes"); err != nil {
		slog.Debug("import wipe failed", "table", "state_changes", "err", err)
	}
	if _, err := tx.Exec("DELETE FROM alert_health"); err != nil {
		slog.Debug("import wipe failed", "table", "alert_health", "err", err)
	}
}

func (d *SQLiteDialect) ImportResetSequences(tx *sql.Tx) {}
