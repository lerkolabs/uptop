package store

import (
	"context"
	"fmt"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

func (s *SQLStore) SaveStateChange(ctx context.Context, siteID int, fromStatus, toStatus, errorReason string) error {
	_, err := s.db.ExecContext(ctx, s.q("INSERT INTO state_changes (site_id, from_status, to_status, error_reason) VALUES (?, ?, ?, ?)"),
		siteID, fromStatus, toStatus, errorReason)
	return err
}

func (s *SQLStore) GetStateChanges(ctx context.Context, siteID int, limit int) ([]models.StateChange, error) {
	rows, err := s.db.QueryContext(ctx, s.q("SELECT id, site_id, from_status, to_status, error_reason, changed_at FROM state_changes WHERE site_id = ? ORDER BY changed_at DESC LIMIT ?"), siteID, limit)
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

func (s *SQLStore) GetStateChangesSince(ctx context.Context, siteID int, since time.Time) ([]models.StateChange, error) {
	rows, err := s.db.QueryContext(ctx, s.q("SELECT id, site_id, from_status, to_status, error_reason, changed_at FROM state_changes WHERE site_id = ? AND changed_at >= ? ORDER BY changed_at DESC"), siteID, since)
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

func (s *SQLStore) SaveCheck(ctx context.Context, siteID int, latencyNs int64, isUp bool) error {
	return s.SaveCheckFromNode(ctx, siteID, "", latencyNs, isUp)
}

// SaveCheckFromNode inserts a single check row. Retention is handled out of
// band by PruneCheckHistory on a timer, not per-insert, to keep the write hot
// path a plain INSERT.
func (s *SQLStore) SaveCheckFromNode(ctx context.Context, siteID int, nodeID string, latencyNs int64, isUp bool) error {
	_, err := s.db.ExecContext(ctx, s.q("INSERT INTO check_history (site_id, node_id, latency_ns, is_up) VALUES (?, ?, ?, ?)"), siteID, nodeID, latencyNs, isUp)
	return err
}

// PruneCheckHistory trims check_history to the newest maxCheckHistory rows per
// site, across all sites, in one pass. Intended to run periodically.
func (s *SQLStore) PruneCheckHistory(ctx context.Context) error {
	q := fmt.Sprintf(`DELETE FROM check_history WHERE id IN (
		SELECT id FROM (
			SELECT id, ROW_NUMBER() OVER (PARTITION BY site_id ORDER BY checked_at DESC, id DESC) AS rn
			FROM check_history
		) ranked WHERE rn > %d
	)`, maxCheckHistory)
	_, err := s.db.ExecContext(ctx, s.q(q))
	return err
}

// PruneStateChanges trims state_changes to the newest maxStateChangesPerSite
// rows per site. Generous so realistic SLA windows are unaffected; bounds the
// otherwise unbounded growth of a flapping monitor's history.
func (s *SQLStore) PruneStateChanges(ctx context.Context) error {
	q := fmt.Sprintf(`DELETE FROM state_changes WHERE id IN (
		SELECT id FROM (
			SELECT id, ROW_NUMBER() OVER (PARTITION BY site_id ORDER BY changed_at DESC, id DESC) AS rn
			FROM state_changes
		) ranked WHERE rn > %d
	)`, maxStateChangesPerSite)
	_, err := s.db.ExecContext(ctx, s.q(q))
	return err
}

func (s *SQLStore) LoadAllHistory(ctx context.Context, limit int) (map[int][]models.CheckRecord, error) {
	result := make(map[int][]models.CheckRecord)
	rows, err := s.db.QueryContext(ctx, s.q(`
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
