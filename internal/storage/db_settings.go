// Package storage provides database operations for vcdeploy.
package storage

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/rs/xid"
)

// --- Settings operations ---

// GetSetting retrieves a single setting.
func (db *DB) GetSetting(ctx context.Context, category, key string) (*Setting, error) {
	var s Setting
	var encrypted int
	var description sql.NullString
	err := db.conn.QueryRowContext(ctx, `
		SELECT id, category, key, value, value_type, encrypted, description, created_at, updated_at
		FROM settings WHERE category = ? AND key = ?
	`, category, key).Scan(
		&s.ID, &s.Category, &s.Key, &s.Value, &s.ValueType, &encrypted, &description, &s.CreatedAt, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying setting: %w", err)
	}
	s.Encrypted = encrypted == 1
	s.Description = description.String
	return &s, nil
}

// SetSetting creates or updates a setting.
func (db *DB) SetSetting(ctx context.Context, category, key, value, valueType string, encrypted bool) error {
	settingID := xid.New().String()
	encVal := 0
	if encrypted {
		encVal = 1
	}
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO settings (id, category, key, value, value_type, encrypted)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(category, key) DO UPDATE SET
			value = excluded.value,
			value_type = excluded.value_type,
			encrypted = excluded.encrypted,
			updated_at = CURRENT_TIMESTAMP
	`, settingID, category, key, value, valueType, encVal)
	if err != nil {
		return fmt.Errorf("setting setting: %w", err)
	}
	return nil
}

// InitSetting seeds a setting only if it does not already exist (INSERT OR IGNORE).
// Used for runtime settings where user edits should survive server restarts.
func (db *DB) InitSetting(ctx context.Context, category, key, value, valueType string, encrypted bool) error {
	settingID := xid.New().String()
	encVal := 0
	if encrypted {
		encVal = 1
	}
	_, err := db.conn.ExecContext(ctx, `
		INSERT OR IGNORE INTO settings (id, category, key, value, value_type, encrypted)
		VALUES (?, ?, ?, ?, ?, ?)
	`, settingID, category, key, value, valueType, encVal)
	if err != nil {
		return fmt.Errorf("init setting: %w", err)
	}
	return nil
}

// ListSettingsByCategory retrieves all settings in a category.
func (db *DB) ListSettingsByCategory(ctx context.Context, category string) ([]*Setting, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, category, key, value, value_type, encrypted, description, created_at, updated_at
		FROM settings WHERE category = ? ORDER BY key
	`, category)
	if err != nil {
		return nil, fmt.Errorf("querying settings: %w", err)
	}
	defer rows.Close()

	var settings []*Setting
	for rows.Next() {
		var s Setting
		var encrypted int
		var description sql.NullString
		if err := rows.Scan(&s.ID, &s.Category, &s.Key, &s.Value, &s.ValueType, &encrypted, &description, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning setting: %w", err)
		}
		s.Encrypted = encrypted == 1
		s.Description = description.String
		settings = append(settings, &s)
	}
	return settings, rows.Err()
}

// ListAllSettings retrieves all settings.
func (db *DB) ListAllSettings(ctx context.Context) ([]*Setting, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, category, key, value, value_type, encrypted, description, created_at, updated_at
		FROM settings ORDER BY category, key
	`)
	if err != nil {
		return nil, fmt.Errorf("querying settings: %w", err)
	}
	defer rows.Close()

	var settings []*Setting
	for rows.Next() {
		var s Setting
		var encrypted int
		var description sql.NullString
		if err := rows.Scan(&s.ID, &s.Category, &s.Key, &s.Value, &s.ValueType, &encrypted, &description, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning setting: %w", err)
		}
		s.Encrypted = encrypted == 1
		s.Description = description.String
		settings = append(settings, &s)
	}
	return settings, rows.Err()
}

// DeleteSetting deletes a setting.
func (db *DB) DeleteSetting(ctx context.Context, category, key string) error {
	_, err := db.conn.ExecContext(ctx, `DELETE FROM settings WHERE category = ? AND key = ?`, category, key)
	if err != nil {
		return fmt.Errorf("deleting setting: %w", err)
	}
	return nil
}

// HasSettings checks if any settings exist (for init detection).
func (db *DB) HasSettings(ctx context.Context) (bool, error) {
	var count int
	err := db.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM settings`).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking settings: %w", err)
	}
	return count > 0, nil
}
