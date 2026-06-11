package store

import (
	"database/sql"
	"log/slog"

	_ "github.com/lib/pq"
)

type PostgresDialect struct{}

func NewPostgresStore(connStr string) (*SQLStore, error) {
	return NewSQLStore("postgres", connStr, &PostgresDialect{})
}

func (d *PostgresDialect) DriverName() string   { return "postgres" }
func (d *PostgresDialect) BoolFalse() string    { return "FALSE" }
func (d *PostgresDialect) BaselineVersion() int { return 21 }

func (d *PostgresDialect) CreateTablesSQL() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS alerts (
			id SERIAL PRIMARY KEY,
			name TEXT, type TEXT, settings TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS sites (
			id SERIAL PRIMARY KEY,
			name TEXT DEFAULT 'New Monitor', url TEXT, type TEXT DEFAULT 'http',
			token TEXT, interval INTEGER, alert_id INTEGER,
			check_ssl BOOLEAN DEFAULT FALSE, threshold INTEGER DEFAULT 7,
			max_retries INTEGER DEFAULT 0, hostname TEXT DEFAULT '',
			port INTEGER DEFAULT 0, timeout INTEGER DEFAULT 0,
			method TEXT DEFAULT 'GET', description TEXT DEFAULT '',
			parent_id INTEGER DEFAULT 0, accepted_codes TEXT DEFAULT '200-299',
			dns_resolve_type TEXT DEFAULT '', dns_server TEXT DEFAULT '',
			ignore_tls BOOLEAN DEFAULT FALSE, paused BOOLEAN DEFAULT FALSE,
			regions TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			username TEXT NOT NULL, public_key TEXT NOT NULL,
			role TEXT DEFAULT 'user'
		)`,
		`CREATE TABLE IF NOT EXISTS check_history (
			id SERIAL PRIMARY KEY,
			site_id INTEGER NOT NULL, latency_ns BIGINT,
			is_up BOOLEAN, checked_at TIMESTAMPTZ DEFAULT NOW(),
			node_id TEXT DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_check_history_site ON check_history(site_id, checked_at DESC)`,
		`CREATE TABLE IF NOT EXISTS nodes (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			region TEXT DEFAULT '',
			last_seen TIMESTAMPTZ DEFAULT NOW(),
			version TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS logs (
			id SERIAL PRIMARY KEY,
			message TEXT NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS maintenance_windows (
			id SERIAL PRIMARY KEY,
			monitor_id INTEGER DEFAULT 0,
			title TEXT NOT NULL,
			description TEXT DEFAULT '',
			type TEXT DEFAULT 'maintenance',
			start_time TIMESTAMPTZ NOT NULL,
			end_time TIMESTAMPTZ,
			created_by TEXT DEFAULT '',
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS preferences (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS state_changes (
			id SERIAL PRIMARY KEY,
			site_id INTEGER NOT NULL,
			from_status TEXT NOT NULL,
			to_status TEXT NOT NULL,
			error_reason TEXT DEFAULT '',
			changed_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_state_changes_site ON state_changes(site_id, changed_at DESC)`,
		`CREATE TABLE IF NOT EXISTS alert_health (
			alert_id INTEGER PRIMARY KEY,
			last_send_at TIMESTAMPTZ,
			last_send_ok BOOLEAN DEFAULT FALSE,
			last_error TEXT DEFAULT '',
			send_count INTEGER DEFAULT 0,
			fail_count INTEGER DEFAULT 0
		)`,
	}
}

func (d *PostgresDialect) Migrations() []Migration {
	return []Migration{
		{1, "ALTER TABLE sites ADD COLUMN IF NOT EXISTS hostname TEXT DEFAULT ''"},
		{2, "ALTER TABLE sites ADD COLUMN IF NOT EXISTS port INTEGER DEFAULT 0"},
		{3, "ALTER TABLE sites ADD COLUMN IF NOT EXISTS timeout INTEGER DEFAULT 0"},
		{4, "ALTER TABLE sites ADD COLUMN IF NOT EXISTS method TEXT DEFAULT 'GET'"},
		{5, "ALTER TABLE sites ADD COLUMN IF NOT EXISTS description TEXT DEFAULT ''"},
		{6, "ALTER TABLE sites ADD COLUMN IF NOT EXISTS parent_id INTEGER DEFAULT 0"},
		{7, "ALTER TABLE sites ADD COLUMN IF NOT EXISTS accepted_codes TEXT DEFAULT '200-299'"},
		{8, "ALTER TABLE sites ADD COLUMN IF NOT EXISTS dns_resolve_type TEXT DEFAULT ''"},
		{9, "ALTER TABLE sites ADD COLUMN IF NOT EXISTS dns_server TEXT DEFAULT ''"},
		{10, "ALTER TABLE sites ADD COLUMN IF NOT EXISTS ignore_tls BOOLEAN DEFAULT FALSE"},
		{11, "ALTER TABLE sites ADD COLUMN IF NOT EXISTS paused BOOLEAN DEFAULT FALSE"},
		{12, "ALTER TABLE check_history ADD COLUMN IF NOT EXISTS node_id TEXT DEFAULT ''"},
		{13, "ALTER TABLE sites ADD COLUMN IF NOT EXISTS regions TEXT DEFAULT ''"},
		{14, "ALTER TABLE check_history ALTER COLUMN checked_at TYPE TIMESTAMPTZ USING checked_at AT TIME ZONE 'UTC'"},
		{15, "ALTER TABLE nodes ALTER COLUMN last_seen TYPE TIMESTAMPTZ USING last_seen AT TIME ZONE 'UTC'"},
		{16, "ALTER TABLE logs ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC'"},
		{17, "ALTER TABLE maintenance_windows ALTER COLUMN start_time TYPE TIMESTAMPTZ USING start_time AT TIME ZONE 'UTC'"},
		{18, "ALTER TABLE maintenance_windows ALTER COLUMN end_time TYPE TIMESTAMPTZ USING end_time AT TIME ZONE 'UTC'"},
		{19, "ALTER TABLE maintenance_windows ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC'"},
		{20, "ALTER TABLE state_changes ALTER COLUMN changed_at TYPE TIMESTAMPTZ USING changed_at AT TIME ZONE 'UTC'"},
		{21, "ALTER TABLE alert_health ALTER COLUMN last_send_at TYPE TIMESTAMPTZ USING last_send_at AT TIME ZONE 'UTC'"},
	}
}

func (d *PostgresDialect) UpsertNodeSQL() string {
	return "INSERT INTO nodes (id, name, region, last_seen, version) VALUES ($1, $2, $3, NOW(), $4) ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, region = EXCLUDED.region, last_seen = NOW(), version = EXCLUDED.version"
}

func (d *PostgresDialect) UpsertAlertHealthSQL() string {
	return "INSERT INTO alert_health (alert_id, last_send_at, last_send_ok, last_error, send_count, fail_count) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (alert_id) DO UPDATE SET last_send_at = EXCLUDED.last_send_at, last_send_ok = EXCLUDED.last_send_ok, last_error = EXCLUDED.last_error, send_count = EXCLUDED.send_count, fail_count = EXCLUDED.fail_count"
}

func (d *PostgresDialect) ResetSequenceOnEmpty(db *sql.DB, table string) {}

func (d *PostgresDialect) ImportWipe(tx *sql.Tx) {
	if _, err := tx.Exec("TRUNCATE TABLE sites RESTART IDENTITY CASCADE"); err != nil {
		slog.Debug("import wipe failed", "table", "sites", "err", err)
	}
	if _, err := tx.Exec("TRUNCATE TABLE alerts RESTART IDENTITY CASCADE"); err != nil {
		slog.Debug("import wipe failed", "table", "alerts", "err", err)
	}
	if _, err := tx.Exec("TRUNCATE TABLE users RESTART IDENTITY CASCADE"); err != nil {
		slog.Debug("import wipe failed", "table", "users", "err", err)
	}
	if _, err := tx.Exec("TRUNCATE TABLE maintenance_windows RESTART IDENTITY CASCADE"); err != nil {
		slog.Debug("import wipe failed", "table", "maintenance_windows", "err", err)
	}
	if _, err := tx.Exec("TRUNCATE TABLE check_history RESTART IDENTITY CASCADE"); err != nil {
		slog.Debug("import wipe failed", "table", "check_history", "err", err)
	}
	if _, err := tx.Exec("TRUNCATE TABLE state_changes RESTART IDENTITY CASCADE"); err != nil {
		slog.Debug("import wipe failed", "table", "state_changes", "err", err)
	}
	if _, err := tx.Exec("TRUNCATE TABLE alert_health RESTART IDENTITY CASCADE"); err != nil {
		slog.Debug("import wipe failed", "table", "alert_health", "err", err)
	}
}

func (d *PostgresDialect) ImportResetSequences(tx *sql.Tx) {
	if _, err := tx.Exec("SELECT setval('sites_id_seq', (SELECT COALESCE(MAX(id), 1) FROM sites))"); err != nil {
		slog.Debug("sequence reset failed", "table", "sites", "err", err)
	}
	if _, err := tx.Exec("SELECT setval('alerts_id_seq', (SELECT COALESCE(MAX(id), 1) FROM alerts))"); err != nil {
		slog.Debug("sequence reset failed", "table", "alerts", "err", err)
	}
	if _, err := tx.Exec("SELECT setval('users_id_seq', (SELECT COALESCE(MAX(id), 1) FROM users))"); err != nil {
		slog.Debug("sequence reset failed", "table", "users", "err", err)
	}
	if _, err := tx.Exec("SELECT setval('maintenance_windows_id_seq', (SELECT COALESCE(MAX(id), 1) FROM maintenance_windows))"); err != nil {
		slog.Debug("sequence reset failed", "table", "maintenance_windows", "err", err)
	}
}
