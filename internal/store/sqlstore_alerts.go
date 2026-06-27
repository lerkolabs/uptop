package store

import (
	"context"
	"encoding/json"
	"fmt"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

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

func (s *SQLStore) GetAlertByName(ctx context.Context, name string) (models.AlertConfig, error) {
	var a models.AlertConfig
	var settingsRaw string
	err := s.db.QueryRowContext(ctx, s.q("SELECT id, name, type, settings FROM alerts WHERE name = ?"), name).Scan(&a.ID, &a.Name, &a.Type, &settingsRaw)
	if err != nil {
		return a, err
	}
	a.Settings, err = s.unmarshalSettings(settingsRaw)
	if err != nil {
		return a, fmt.Errorf("alert %q: %w", name, err)
	}
	return a, nil
}

func (s *SQLStore) AddAlertReturningID(ctx context.Context, name, aType string, settings map[string]string) (int, error) {
	stored, err := s.marshalSettings(settings)
	if err != nil {
		return 0, err
	}
	if s.dollar {
		var id int
		err := s.db.QueryRowContext(ctx, s.q("INSERT INTO alerts (name, type, settings) VALUES (?, ?, ?) RETURNING id"), name, aType, stored).Scan(&id)
		return id, err
	}
	result, err := s.db.ExecContext(ctx, s.q("INSERT INTO alerts (name, type, settings) VALUES (?, ?, ?)"), name, aType, stored)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return int(id), err
}

func (s *SQLStore) GetAllAlerts(ctx context.Context) ([]models.AlertConfig, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, name, type, settings FROM alerts")
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

func (s *SQLStore) GetAlert(ctx context.Context, id int) (models.AlertConfig, error) {
	var a models.AlertConfig
	var settingsRaw string
	err := s.db.QueryRowContext(ctx, s.q("SELECT id, name, type, settings FROM alerts WHERE id = ?"), id).Scan(&a.ID, &a.Name, &a.Type, &settingsRaw)
	if err != nil {
		return a, err
	}
	a.Settings, err = s.unmarshalSettings(settingsRaw)
	if err != nil {
		return a, fmt.Errorf("alert %d: %w", id, err)
	}
	return a, nil
}

func (s *SQLStore) AddAlert(ctx context.Context, name, aType string, settings map[string]string) error {
	stored, err := s.marshalSettings(settings)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, s.q("INSERT INTO alerts (name, type, settings) VALUES (?, ?, ?)"), name, aType, stored)
	return err
}

func (s *SQLStore) UpdateAlert(ctx context.Context, id int, name, aType string, settings map[string]string) error {
	stored, err := s.marshalSettings(settings)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, s.q("UPDATE alerts SET name=?, type=?, settings=? WHERE id=?"), name, aType, stored, id)
	return err
}

func (s *SQLStore) DeleteAlert(ctx context.Context, id int) error {
	if _, err := s.db.ExecContext(ctx, s.q("UPDATE sites SET alert_id = 0 WHERE alert_id = ?"), id); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, s.q("DELETE FROM alerts WHERE id=?"), id); err != nil {
		return err
	}
	s.dialect.ResetSequenceOnEmpty(s.db, "alerts")
	return nil
}
