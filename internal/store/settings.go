package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// SettingValue returns the raw stored value for key, or "" if unset. Values are
// generic text — see migrations/0001_init.sql: numeric settings (FX rate, metal
// prices, margins) are stored as fixed-point integer text (micros), parsed by
// internal/money callers.
func (s *Store) SettingValue(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("store: setting %q: %w", key, err)
	}
	return v, nil
}

// SettingValues returns the raw stored values for the given keys, keyed by key. A key
// with no row is simply absent from the returned map.
func (s *Store) SettingValues(ctx context.Context, keys []string) (map[string]string, error) {
	if len(keys) == 0 {
		return map[string]string{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(keys)), ",")
	args := make([]any, len(keys))
	for i, k := range keys {
		args[i] = k
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT key, value FROM settings WHERE key IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: setting values: %w", err)
	}
	defer rows.Close()

	values := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("store: setting values: %w", err)
		}
		values[k] = v
	}
	return values, rows.Err()
}

// SetSetting upserts a setting's value, recording which admin changed it.
func (s *Store) SetSetting(ctx context.Context, key, value string, updatedByUserID int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_by) VALUES (?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET
			value      = excluded.value,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
			updated_by = excluded.updated_by`,
		key, value, updatedByUserID,
	)
	if err != nil {
		return fmt.Errorf("store: set setting %q: %w", key, err)
	}
	return nil
}
