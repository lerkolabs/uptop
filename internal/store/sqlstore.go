package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

const (
	maxCheckHistory        = 1000
	maxLogRows             = 200
	maxStateChangesPerSite = 5000
	maxMaintenanceExport   = 1000
	maxRequestBody         = 1 << 20
)

type SQLStore struct {
	db        *sql.DB
	dialect   Dialect
	dollar    bool
	encryptor *Encryptor
}

func NewSQLStore(driverName, dsn string, dialect Dialect) (*SQLStore, error) {
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	_, isDollar := dialect.(*PostgresDialect)
	return &SQLStore{db: db, dialect: dialect, dollar: isDollar}, nil
}

func (s *SQLStore) SetEncryptor(enc *Encryptor) {
	s.encryptor = enc
}

func (s *SQLStore) encryptSettings(jsonStr string) (string, error) {
	if s.encryptor == nil {
		return jsonStr, nil
	}
	return s.encryptor.Encrypt(jsonStr)
}

func (s *SQLStore) decryptSettings(data string) (string, error) {
	if s.encryptor == nil {
		return data, nil
	}
	return s.encryptor.Decrypt(data)
}

func (s *SQLStore) q(query string) string {
	return rewritePlaceholders(query, s.dollar)
}

func generateToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto/rand failed: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func (s *SQLStore) Close() error {
	return s.db.Close()
}

func (s *SQLStore) Init() error {
	for _, stmt := range s.dialect.CreateTablesSQL() {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	for _, m := range s.dialect.MigrationsSQL() {
		if _, err := s.db.Exec(m); err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "already exists") || strings.Contains(errMsg, "duplicate column") {
				continue
			}
			return fmt.Errorf("migration failed: %w", err)
		}
	}
	return nil
}

func (s *SQLStore) GetSites() ([]models.Site, error) {
	bf := s.dialect.BoolFalse()
	query := fmt.Sprintf( //nolint:gosec // bf is a dialect boolean literal, not user input
		"SELECT id, COALESCE(name, url), url, COALESCE(type, 'http'), COALESCE(token, ''), interval, alert_id, check_ssl, threshold, max_retries, COALESCE(hostname, ''), COALESCE(port, 0), COALESCE(timeout, 0), COALESCE(method, 'GET'), COALESCE(description, ''), COALESCE(parent_id, 0), COALESCE(accepted_codes, '200-299'), COALESCE(dns_resolve_type, ''), COALESCE(dns_server, ''), COALESCE(ignore_tls, %s), COALESCE(paused, %s), COALESCE(regions, '') FROM sites",
		bf, bf,
	)
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sites []models.Site
	for rows.Next() {
		var st models.Site
		if err := rows.Scan(&st.ID, &st.Name, &st.URL, &st.Type, &st.Token, &st.Interval, &st.AlertID,
			&st.CheckSSL, &st.ExpiryThreshold, &st.MaxRetries, &st.Hostname, &st.Port, &st.Timeout,
			&st.Method, &st.Description, &st.ParentID, &st.AcceptedCodes, &st.DNSResolveType,
			&st.DNSServer, &st.IgnoreTLS, &st.Paused, &st.Regions); err != nil {
			return sites, err
		}
		sites = append(sites, st)
	}
	return sites, rows.Err()
}

func (s *SQLStore) AddSite(site models.Site) error {
	token := ""
	if site.Type == "push" {
		var err error
		token, err = generateToken()
		if err != nil {
			return fmt.Errorf("generate push token: %w", err)
		}
	}
	_, err := s.db.Exec(s.q("INSERT INTO sites (name, url, type, token, interval, alert_id, check_ssl, threshold, max_retries, hostname, port, timeout, method, description, parent_id, accepted_codes, dns_resolve_type, dns_server, ignore_tls, paused, regions) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"),
		site.Name, site.URL, site.Type, token, site.Interval, site.AlertID, site.CheckSSL, site.ExpiryThreshold, site.MaxRetries,
		site.Hostname, site.Port, site.Timeout, site.Method, site.Description, site.ParentID, site.AcceptedCodes, site.DNSResolveType, site.DNSServer, site.IgnoreTLS, site.Paused, site.Regions)
	return err
}

func (s *SQLStore) UpdateSite(site models.Site) error {
	var existingToken string
	_ = s.db.QueryRow(s.q("SELECT token FROM sites WHERE id=?"), site.ID).Scan(&existingToken) //nolint:errcheck
	if site.Type == "push" && existingToken == "" {
		var err error
		existingToken, err = generateToken()
		if err != nil {
			return fmt.Errorf("generate push token: %w", err)
		}
	}
	_, err := s.db.Exec(s.q("UPDATE sites SET name=?, url=?, type=?, token=?, interval=?, alert_id=?, check_ssl=?, threshold=?, max_retries=?, hostname=?, port=?, timeout=?, method=?, description=?, parent_id=?, accepted_codes=?, dns_resolve_type=?, dns_server=?, ignore_tls=?, paused=?, regions=? WHERE id=?"),
		site.Name, site.URL, site.Type, existingToken, site.Interval, site.AlertID, site.CheckSSL, site.ExpiryThreshold, site.MaxRetries,
		site.Hostname, site.Port, site.Timeout, site.Method, site.Description, site.ParentID, site.AcceptedCodes, site.DNSResolveType, site.DNSServer, site.IgnoreTLS, site.Paused, site.Regions, site.ID)
	return err
}

func (s *SQLStore) UpdateSitePaused(id int, paused bool) error {
	_, err := s.db.Exec(s.q("UPDATE sites SET paused=? WHERE id=?"), paused, id)
	return err
}

func (s *SQLStore) DeleteSite(id int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, q := range []string{
		"DELETE FROM maintenance_windows WHERE monitor_id = ?",
		"DELETE FROM check_history WHERE site_id = ?",
		"DELETE FROM state_changes WHERE site_id = ?",
		"DELETE FROM sites WHERE id = ?",
	} {
		if _, err := tx.Exec(s.q(q), id); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	s.dialect.ResetSequenceOnEmpty(s.db, "sites")
	return nil
}

func (s *SQLStore) GetSiteByName(name string) (models.Site, error) {
	bf := s.dialect.BoolFalse()
	query := fmt.Sprintf( //nolint:gosec // bf is a dialect boolean literal, not user input
		"SELECT id, COALESCE(name, url), url, COALESCE(type, 'http'), COALESCE(token, ''), interval, alert_id, check_ssl, threshold, max_retries, COALESCE(hostname, ''), COALESCE(port, 0), COALESCE(timeout, 0), COALESCE(method, 'GET'), COALESCE(description, ''), COALESCE(parent_id, 0), COALESCE(accepted_codes, '200-299'), COALESCE(dns_resolve_type, ''), COALESCE(dns_server, ''), COALESCE(ignore_tls, %s), COALESCE(paused, %s), COALESCE(regions, '') FROM sites WHERE name = %s",
		bf, bf, s.q("?"),
	)
	var st models.Site
	err := s.db.QueryRow(query, name).Scan(&st.ID, &st.Name, &st.URL, &st.Type, &st.Token, &st.Interval, &st.AlertID,
		&st.CheckSSL, &st.ExpiryThreshold, &st.MaxRetries, &st.Hostname, &st.Port, &st.Timeout,
		&st.Method, &st.Description, &st.ParentID, &st.AcceptedCodes, &st.DNSResolveType,
		&st.DNSServer, &st.IgnoreTLS, &st.Paused, &st.Regions)
	return st, err
}

func (s *SQLStore) unmarshalSettings(raw string) (map[string]string, error) {
	decrypted, err := s.decryptSettings(raw)
	if err != nil {
		return nil, fmt.Errorf("decrypt settings: %w", err)
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(decrypted), &m); err != nil {
		return nil, fmt.Errorf("unmarshal settings: %w", err)
	}
	return m, nil
}

func (s *SQLStore) marshalSettings(settings map[string]string) (string, error) {
	jsonBytes, err := json.Marshal(settings)
	if err != nil {
		return "", err
	}
	return s.encryptSettings(string(jsonBytes))
}

func (s *SQLStore) GetAlertByName(name string) (models.AlertConfig, error) {
	var a models.AlertConfig
	var settingsRaw string
	err := s.db.QueryRow(s.q("SELECT id, name, type, settings FROM alerts WHERE name = ?"), name).Scan(&a.ID, &a.Name, &a.Type, &settingsRaw)
	if err != nil {
		return a, err
	}
	a.Settings, err = s.unmarshalSettings(settingsRaw)
	if err != nil {
		return a, fmt.Errorf("alert %q: %w", name, err)
	}
	return a, nil
}

func (s *SQLStore) AddSiteReturningID(site models.Site) (int, error) {
	token := ""
	if site.Type == "push" {
		var err error
		token, err = generateToken()
		if err != nil {
			return 0, fmt.Errorf("generate push token: %w", err)
		}
	}
	if s.dollar {
		var id int
		err := s.db.QueryRow(s.q("INSERT INTO sites (name, url, type, token, interval, alert_id, check_ssl, threshold, max_retries, hostname, port, timeout, method, description, parent_id, accepted_codes, dns_resolve_type, dns_server, ignore_tls, paused, regions) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id"),
			site.Name, site.URL, site.Type, token, site.Interval, site.AlertID, site.CheckSSL, site.ExpiryThreshold, site.MaxRetries,
			site.Hostname, site.Port, site.Timeout, site.Method, site.Description, site.ParentID, site.AcceptedCodes, site.DNSResolveType, site.DNSServer, site.IgnoreTLS, site.Paused, site.Regions).Scan(&id)
		return id, err
	}
	result, err := s.db.Exec(s.q("INSERT INTO sites (name, url, type, token, interval, alert_id, check_ssl, threshold, max_retries, hostname, port, timeout, method, description, parent_id, accepted_codes, dns_resolve_type, dns_server, ignore_tls, paused, regions) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"),
		site.Name, site.URL, site.Type, token, site.Interval, site.AlertID, site.CheckSSL, site.ExpiryThreshold, site.MaxRetries,
		site.Hostname, site.Port, site.Timeout, site.Method, site.Description, site.ParentID, site.AcceptedCodes, site.DNSResolveType, site.DNSServer, site.IgnoreTLS, site.Paused, site.Regions)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return int(id), err
}

func (s *SQLStore) AddAlertReturningID(name, aType string, settings map[string]string) (int, error) {
	stored, err := s.marshalSettings(settings)
	if err != nil {
		return 0, err
	}
	if s.dollar {
		var id int
		err := s.db.QueryRow(s.q("INSERT INTO alerts (name, type, settings) VALUES (?, ?, ?) RETURNING id"), name, aType, stored).Scan(&id)
		return id, err
	}
	result, err := s.db.Exec(s.q("INSERT INTO alerts (name, type, settings) VALUES (?, ?, ?)"), name, aType, stored)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return int(id), err
}

func (s *SQLStore) GetAllAlerts() ([]models.AlertConfig, error) {
	rows, err := s.db.Query("SELECT id, name, type, settings FROM alerts")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var alerts []models.AlertConfig
	for rows.Next() {
		var a models.AlertConfig
		var settingsRaw string
		if err := rows.Scan(&a.ID, &a.Name, &a.Type, &settingsRaw); err != nil {
			return alerts, err
		}
		a.Settings, err = s.unmarshalSettings(settingsRaw)
		if err != nil {
			return alerts, fmt.Errorf("alert %q: %w", a.Name, err)
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

func (s *SQLStore) GetAlert(id int) (models.AlertConfig, error) {
	var a models.AlertConfig
	var settingsRaw string
	err := s.db.QueryRow(s.q("SELECT id, name, type, settings FROM alerts WHERE id = ?"), id).Scan(&a.ID, &a.Name, &a.Type, &settingsRaw)
	if err != nil {
		return a, err
	}
	a.Settings, err = s.unmarshalSettings(settingsRaw)
	if err != nil {
		return a, fmt.Errorf("alert %d: %w", id, err)
	}
	return a, nil
}

func (s *SQLStore) AddAlert(name, aType string, settings map[string]string) error {
	stored, err := s.marshalSettings(settings)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(s.q("INSERT INTO alerts (name, type, settings) VALUES (?, ?, ?)"), name, aType, stored)
	return err
}

func (s *SQLStore) UpdateAlert(id int, name, aType string, settings map[string]string) error {
	stored, err := s.marshalSettings(settings)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(s.q("UPDATE alerts SET name=?, type=?, settings=? WHERE id=?"), name, aType, stored, id)
	return err
}

func (s *SQLStore) DeleteAlert(id int) error {
	_, err := s.db.Exec(s.q("DELETE FROM alerts WHERE id=?"), id)
	if err != nil {
		return err
	}
	s.dialect.ResetSequenceOnEmpty(s.db, "alerts")
	return nil
}

func (s *SQLStore) GetAllUsers() ([]models.User, error) {
	rows, err := s.db.Query("SELECT id, username, public_key, role FROM users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Username, &u.PublicKey, &u.Role); err != nil {
			return users, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *SQLStore) AddUser(username, publicKey, role string) error {
	_, err := s.db.Exec(s.q("INSERT INTO users (username, public_key, role) VALUES (?, ?, ?)"), username, publicKey, role)
	return err
}

func (s *SQLStore) UpdateUser(id int, username, publicKey, role string) error {
	_, err := s.db.Exec(s.q("UPDATE users SET username=?, public_key=?, role=? WHERE id=?"), username, publicKey, role, id)
	return err
}

func (s *SQLStore) DeleteUser(id int) error {
	_, err := s.db.Exec(s.q("DELETE FROM users WHERE id=?"), id)
	return err
}

func (s *SQLStore) SaveStateChange(siteID int, fromStatus, toStatus, errorReason string) error {
	_, err := s.db.Exec(s.q("INSERT INTO state_changes (site_id, from_status, to_status, error_reason) VALUES (?, ?, ?, ?)"),
		siteID, fromStatus, toStatus, errorReason)
	return err
}

func (s *SQLStore) GetStateChanges(siteID int, limit int) ([]models.StateChange, error) {
	rows, err := s.db.Query(s.q("SELECT id, site_id, from_status, to_status, error_reason, changed_at FROM state_changes WHERE site_id = ? ORDER BY changed_at DESC LIMIT ?"), siteID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var changes []models.StateChange
	for rows.Next() {
		var sc models.StateChange
		if err := rows.Scan(&sc.ID, &sc.SiteID, &sc.FromStatus, &sc.ToStatus, &sc.ErrorReason, &sc.ChangedAt); err != nil {
			return changes, err
		}
		changes = append(changes, sc)
	}
	return changes, rows.Err()
}

func (s *SQLStore) GetStateChangesSince(siteID int, since time.Time) ([]models.StateChange, error) {
	rows, err := s.db.Query(s.q("SELECT id, site_id, from_status, to_status, error_reason, changed_at FROM state_changes WHERE site_id = ? AND changed_at >= ? ORDER BY changed_at DESC"), siteID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var changes []models.StateChange
	for rows.Next() {
		var sc models.StateChange
		if err := rows.Scan(&sc.ID, &sc.SiteID, &sc.FromStatus, &sc.ToStatus, &sc.ErrorReason, &sc.ChangedAt); err != nil {
			return changes, err
		}
		changes = append(changes, sc)
	}
	return changes, rows.Err()
}

func (s *SQLStore) SaveCheck(siteID int, latencyNs int64, isUp bool) error {
	return s.SaveCheckFromNode(siteID, "", latencyNs, isUp)
}

// SaveCheckFromNode inserts a single check row. Retention is handled out of
// band by PruneCheckHistory on a timer, not per-insert, to keep the write hot
// path a plain INSERT.
func (s *SQLStore) SaveCheckFromNode(siteID int, nodeID string, latencyNs int64, isUp bool) error {
	_, err := s.db.Exec(s.q("INSERT INTO check_history (site_id, node_id, latency_ns, is_up) VALUES (?, ?, ?, ?)"), siteID, nodeID, latencyNs, isUp)
	return err
}

// PruneCheckHistory trims check_history to the newest maxCheckHistory rows per
// site, across all sites, in one pass. Intended to run periodically.
func (s *SQLStore) PruneCheckHistory() error {
	q := fmt.Sprintf(`DELETE FROM check_history WHERE id IN (
		SELECT id FROM (
			SELECT id, ROW_NUMBER() OVER (PARTITION BY site_id ORDER BY checked_at DESC, id DESC) AS rn
			FROM check_history
		) ranked WHERE rn > %d
	)`, maxCheckHistory)
	_, err := s.db.Exec(s.q(q))
	return err
}

// PruneStateChanges trims state_changes to the newest maxStateChangesPerSite
// rows per site. Generous so realistic SLA windows are unaffected; bounds the
// otherwise unbounded growth of a flapping monitor's history.
func (s *SQLStore) PruneStateChanges() error {
	q := fmt.Sprintf(`DELETE FROM state_changes WHERE id IN (
		SELECT id FROM (
			SELECT id, ROW_NUMBER() OVER (PARTITION BY site_id ORDER BY changed_at DESC, id DESC) AS rn
			FROM state_changes
		) ranked WHERE rn > %d
	)`, maxStateChangesPerSite)
	_, err := s.db.Exec(s.q(q))
	return err
}

func (s *SQLStore) RegisterNode(node models.ProbeNode) error {
	_, err := s.db.Exec(s.dialect.UpsertNodeSQL(), node.ID, node.Name, node.Region, node.Version)
	return err
}

func (s *SQLStore) GetNode(id string) (models.ProbeNode, error) {
	var n models.ProbeNode
	err := s.db.QueryRow(s.q("SELECT id, name, region, last_seen, version FROM nodes WHERE id = ?"), id).
		Scan(&n.ID, &n.Name, &n.Region, &n.LastSeen, &n.Version)
	return n, err
}

func (s *SQLStore) GetAllNodes() ([]models.ProbeNode, error) {
	rows, err := s.db.Query("SELECT id, name, region, last_seen, version FROM nodes ORDER BY region, name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nodes []models.ProbeNode
	for rows.Next() {
		var n models.ProbeNode
		if err := rows.Scan(&n.ID, &n.Name, &n.Region, &n.LastSeen, &n.Version); err != nil {
			return nodes, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

func (s *SQLStore) UpdateNodeLastSeen(id string) error {
	_, err := s.db.Exec(s.q("UPDATE nodes SET last_seen = CURRENT_TIMESTAMP WHERE id = ?"), id)
	return err
}

func (s *SQLStore) DeleteNode(id string) error {
	_, err := s.db.Exec(s.q("DELETE FROM nodes WHERE id = ?"), id)
	return err
}

func (s *SQLStore) LoadAlertHealth() (map[int]models.AlertHealthRecord, error) {
	rows, err := s.db.Query("SELECT alert_id, last_send_at, last_send_ok, last_error, send_count, fail_count FROM alert_health")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int]models.AlertHealthRecord)
	for rows.Next() {
		var r models.AlertHealthRecord
		var lastSend sql.NullTime
		if err := rows.Scan(&r.AlertID, &lastSend, &r.LastSendOK, &r.LastError, &r.SendCount, &r.FailCount); err != nil {
			return out, err
		}
		if lastSend.Valid {
			r.LastSendAt = lastSend.Time
		}
		out[r.AlertID] = r
	}
	return out, rows.Err()
}

func (s *SQLStore) SaveAlertHealth(h models.AlertHealthRecord) error {
	var lastSend interface{}
	if !h.LastSendAt.IsZero() {
		lastSend = h.LastSendAt
	}
	_, err := s.db.Exec(s.dialect.UpsertAlertHealthSQL(),
		h.AlertID, lastSend, h.LastSendOK, h.LastError, h.SendCount, h.FailCount)
	return err
}

// SaveLog inserts a single log row. Retention is handled by PruneLogs on a
// timer, not per-insert.
func (s *SQLStore) SaveLog(message string) error {
	_, err := s.db.Exec(s.q("INSERT INTO logs (message) VALUES (?)"), message)
	return err
}

// PruneLogs trims the logs table to the newest maxLogRows rows. The id DESC
// tiebreak keeps ordering deterministic when rows share a created_at second.
func (s *SQLStore) PruneLogs() error {
	q := fmt.Sprintf(`DELETE FROM logs WHERE id NOT IN (
		SELECT id FROM logs ORDER BY created_at DESC, id DESC LIMIT %d
	)`, maxLogRows)
	_, err := s.db.Exec(s.q(q))
	return err
}

func (s *SQLStore) LoadLogs(limit int) ([]string, error) {
	rows, err := s.db.Query(s.q("SELECT message FROM logs ORDER BY created_at DESC LIMIT ?"), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []string
	for rows.Next() {
		var msg string
		if err := rows.Scan(&msg); err != nil {
			return logs, err
		}
		logs = append(logs, msg)
	}
	return logs, rows.Err()
}

func (s *SQLStore) LoadAllHistory(limit int) (map[int][]models.CheckRecord, error) {
	result := make(map[int][]models.CheckRecord)
	rows, err := s.db.Query(s.q(`
		SELECT site_id, latency_ns, is_up FROM (
			SELECT site_id, latency_ns, is_up,
				ROW_NUMBER() OVER (PARTITION BY site_id ORDER BY checked_at DESC) AS rn
			FROM check_history
		) sub WHERE rn <= ?`), limit)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var r models.CheckRecord
		if err := rows.Scan(&r.SiteID, &r.LatencyNs, &r.IsUp); err != nil {
			return result, err
		}
		result[r.SiteID] = append(result[r.SiteID], r)
	}
	for id, records := range result {
		for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
			records[i], records[j] = records[j], records[i]
		}
		result[id] = records
	}
	return result, rows.Err()
}

func (s *SQLStore) scanMaintenanceWindow(rows *sql.Rows) (models.MaintenanceWindow, error) {
	var mw models.MaintenanceWindow
	var endTime sql.NullTime
	if err := rows.Scan(&mw.ID, &mw.MonitorID, &mw.Title, &mw.Description, &mw.Type, &mw.StartTime, &endTime, &mw.CreatedBy, &mw.CreatedAt); err != nil {
		return mw, err
	}
	if endTime.Valid {
		mw.EndTime = endTime.Time
	}
	return mw, nil
}

func (s *SQLStore) GetActiveMaintenanceWindows() ([]models.MaintenanceWindow, error) {
	rows, err := s.db.Query(s.q("SELECT id, monitor_id, title, description, type, start_time, end_time, created_by, created_at FROM maintenance_windows WHERE start_time <= CURRENT_TIMESTAMP AND (end_time IS NULL OR end_time > CURRENT_TIMESTAMP) ORDER BY start_time DESC"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var windows []models.MaintenanceWindow
	for rows.Next() {
		mw, err := s.scanMaintenanceWindow(rows)
		if err != nil {
			return windows, err
		}
		windows = append(windows, mw)
	}
	return windows, rows.Err()
}

func (s *SQLStore) GetAllMaintenanceWindows(limit int) ([]models.MaintenanceWindow, error) {
	rows, err := s.db.Query(s.q("SELECT id, monitor_id, title, description, type, start_time, end_time, created_by, created_at FROM maintenance_windows ORDER BY created_at DESC LIMIT ?"), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var windows []models.MaintenanceWindow
	for rows.Next() {
		mw, err := s.scanMaintenanceWindow(rows)
		if err != nil {
			return windows, err
		}
		windows = append(windows, mw)
	}
	return windows, rows.Err()
}

func (s *SQLStore) AddMaintenanceWindow(mw models.MaintenanceWindow) error {
	if mw.StartTime.IsZero() {
		mw.StartTime = time.Now()
	}
	_, err := s.db.Exec(s.q("INSERT INTO maintenance_windows (monitor_id, title, description, type, start_time, end_time, created_by) VALUES (?, ?, ?, ?, ?, ?, ?)"),
		mw.MonitorID, mw.Title, mw.Description, mw.Type, mw.StartTime, sql.NullTime{Time: mw.EndTime, Valid: !mw.EndTime.IsZero()}, mw.CreatedBy)
	return err
}

func (s *SQLStore) EndMaintenanceWindow(id int) error {
	_, err := s.db.Exec(s.q("UPDATE maintenance_windows SET end_time = CURRENT_TIMESTAMP WHERE id = ?"), id)
	return err
}

func (s *SQLStore) DeleteMaintenanceWindow(id int) error {
	_, err := s.db.Exec(s.q("DELETE FROM maintenance_windows WHERE id = ?"), id)
	if err != nil {
		return err
	}
	s.dialect.ResetSequenceOnEmpty(s.db, "maintenance_windows")
	return nil
}

func (s *SQLStore) PruneExpiredMaintenanceWindows(retention time.Duration) (int64, error) {
	cutoff := time.Now().Add(-retention)
	result, err := s.db.Exec(
		s.q("DELETE FROM maintenance_windows WHERE end_time IS NOT NULL AND end_time < ?"),
		cutoff,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *SQLStore) IsMonitorInMaintenance(monitorID int) (bool, error) {
	var count int
	err := s.db.QueryRow(s.q(`SELECT COUNT(*) FROM maintenance_windows
		WHERE type = 'maintenance'
		AND start_time <= CURRENT_TIMESTAMP
		AND (end_time IS NULL OR end_time > CURRENT_TIMESTAMP)
		AND (monitor_id = 0 OR monitor_id = ?
			OR monitor_id IN (SELECT parent_id FROM sites WHERE id = ? AND parent_id > 0))`),
		monitorID, monitorID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *SQLStore) GetPreference(key string) (string, error) {
	var value string
	err := s.db.QueryRow(s.q("SELECT value FROM preferences WHERE key = ?"), key).Scan(&value)
	if err != nil {
		return "", err
	}
	return value, nil
}

func (s *SQLStore) SetPreference(key, value string) error {
	if s.dollar {
		_, err := s.db.Exec(s.q("INSERT INTO preferences (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = ?"), key, value, value)
		return err
	}
	_, err := s.db.Exec("INSERT OR REPLACE INTO preferences (key, value) VALUES (?, ?)", key, value)
	return err
}

func (s *SQLStore) ExportData() (models.Backup, error) {
	sites, err := s.GetSites()
	if err != nil {
		return models.Backup{}, err
	}
	alerts, err := s.GetAllAlerts()
	if err != nil {
		return models.Backup{}, err
	}
	users, err := s.GetAllUsers()
	if err != nil {
		return models.Backup{}, err
	}
	windows, err := s.GetAllMaintenanceWindows(maxMaintenanceExport)
	if err != nil {
		return models.Backup{}, err
	}
	return models.Backup{Sites: sites, Alerts: alerts, Users: users, MaintenanceWindows: windows}, nil
}

func (s *SQLStore) ImportData(data models.Backup) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	s.dialect.ImportWipe(tx)

	for _, u := range data.Users {
		if _, err := tx.Exec(s.q("INSERT INTO users (username, public_key, role) VALUES (?, ?, ?)"), u.Username, u.PublicKey, u.Role); err != nil {
			return err
		}
	}
	for _, a := range data.Alerts {
		// Encrypt on import exactly as AddAlert/UpdateAlert do, so a restore
		// honors UPTOP_ENCRYPTION_KEY instead of writing secrets in plaintext.
		settingsStr, err := s.marshalSettings(a.Settings)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(s.q("INSERT INTO alerts (id, name, type, settings) VALUES (?, ?, ?, ?)"), a.ID, a.Name, a.Type, settingsStr); err != nil {
			return err
		}
	}
	for _, st := range data.Sites {
		if _, err := tx.Exec(s.q("INSERT INTO sites (id, name, url, type, token, interval, alert_id, check_ssl, threshold, max_retries, hostname, port, timeout, method, description, parent_id, accepted_codes, dns_resolve_type, dns_server, ignore_tls, paused, regions) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"),
			st.ID, st.Name, st.URL, st.Type, st.Token, st.Interval, st.AlertID, st.CheckSSL, st.ExpiryThreshold, st.MaxRetries,
			st.Hostname, st.Port, st.Timeout, st.Method, st.Description, st.ParentID, st.AcceptedCodes, st.DNSResolveType, st.DNSServer, st.IgnoreTLS, st.Paused, st.Regions); err != nil {
			return err
		}
	}

	for _, mw := range data.MaintenanceWindows {
		if _, err := tx.Exec(s.q("INSERT INTO maintenance_windows (id, monitor_id, title, description, type, start_time, end_time, created_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?)"),
			mw.ID, mw.MonitorID, mw.Title, mw.Description, mw.Type, mw.StartTime, sql.NullTime{Time: mw.EndTime, Valid: !mw.EndTime.IsZero()}, mw.CreatedBy); err != nil {
			return err
		}
	}

	s.dialect.ImportResetSequences(tx)

	return tx.Commit()
}
