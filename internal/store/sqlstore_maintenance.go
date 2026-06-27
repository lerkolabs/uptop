package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

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

func (s *SQLStore) GetActiveMaintenanceWindows(ctx context.Context) ([]models.MaintenanceWindow, error) {
	rows, err := s.db.QueryContext(ctx, s.q("SELECT id, monitor_id, title, description, type, start_time, end_time, created_by, created_at FROM maintenance_windows WHERE start_time <= CURRENT_TIMESTAMP AND (end_time IS NULL OR end_time > CURRENT_TIMESTAMP) ORDER BY start_time DESC"))
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

func (s *SQLStore) GetAllMaintenanceWindows(ctx context.Context, limit int) ([]models.MaintenanceWindow, error) {
	rows, err := s.db.QueryContext(ctx, s.q("SELECT id, monitor_id, title, description, type, start_time, end_time, created_by, created_at FROM maintenance_windows ORDER BY created_at DESC LIMIT ?"), limit)
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

func (s *SQLStore) GetOverlappingMaintenanceWindows(ctx context.Context, monitorID int, startTime, endTime time.Time) ([]models.MaintenanceWindow, error) {
	var timeClause string
	var args []interface{}

	if endTime.IsZero() {
		timeClause = "(end_time IS NULL OR end_time > ?)"
		args = append(args, startTime)
	} else {
		timeClause = "(end_time IS NULL OR end_time > ?) AND start_time < ?"
		args = append(args, startTime, endTime)
	}

	var scopeClause string
	if monitorID == 0 {
		scopeClause = "1=1"
	} else {
		scopeClause = "(monitor_id = ? OR monitor_id = 0 OR monitor_id IN (SELECT parent_id FROM sites WHERE id = ? AND parent_id > 0))"
		args = append(args, monitorID, monitorID)
	}

	query := fmt.Sprintf(
		"SELECT id, monitor_id, title, description, type, start_time, end_time, created_by, created_at FROM maintenance_windows WHERE %s AND %s ORDER BY start_time",
		timeClause, scopeClause,
	)

	rows, err := s.db.QueryContext(ctx, s.q(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var windows []models.MaintenanceWindow
	for rows.Next() {
		mw, err := s.scanMaintenanceWindow(rows)
		if err != nil {
			return nil, err
		}
		windows = append(windows, mw)
	}
	return windows, rows.Err()
}

func (s *SQLStore) AddMaintenanceWindow(ctx context.Context, mw models.MaintenanceWindow) error {
	if mw.StartTime.IsZero() {
		mw.StartTime = time.Now()
	}
	_, err := s.db.ExecContext(ctx, s.q("INSERT INTO maintenance_windows (monitor_id, title, description, type, start_time, end_time, created_by) VALUES (?, ?, ?, ?, ?, ?, ?)"),
		mw.MonitorID, mw.Title, mw.Description, mw.Type, mw.StartTime, sql.NullTime{Time: mw.EndTime, Valid: !mw.EndTime.IsZero()}, mw.CreatedBy)
	return err
}

func (s *SQLStore) EndMaintenanceWindow(ctx context.Context, id int) error {
	_, err := s.db.ExecContext(ctx, s.q("UPDATE maintenance_windows SET end_time = CURRENT_TIMESTAMP WHERE id = ?"), id)
	return err
}

func (s *SQLStore) DeleteMaintenanceWindow(ctx context.Context, id int) error {
	_, err := s.db.ExecContext(ctx, s.q("DELETE FROM maintenance_windows WHERE id = ?"), id)
	if err != nil {
		return err
	}
	s.dialect.ResetSequenceOnEmpty(s.db, "maintenance_windows")
	return nil
}

func (s *SQLStore) PruneExpiredMaintenanceWindows(ctx context.Context, retention time.Duration) (int64, error) {
	cutoff := time.Now().Add(-retention)
	result, err := s.db.ExecContext(ctx,
		s.q("DELETE FROM maintenance_windows WHERE end_time IS NOT NULL AND end_time < ?"),
		cutoff,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *SQLStore) IsMonitorInMaintenance(ctx context.Context, monitorID int) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM maintenance_windows
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
