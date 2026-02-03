// Package storage provides database operations for vcdeploy.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// --- API Key Management ---

// CreateAPIKey creates a new API key.
func (db *DB) CreateAPIKey(ctx context.Context, key *APIKey) error {
	result, err := db.conn.ExecContext(ctx, `
		INSERT INTO api_keys (user_id, name, key_hash, key_prefix, scopes, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, key.UserID, key.Name, key.KeyHash, key.KeyPrefix, key.Scopes, key.ExpiresAt, key.CreatedAt)
	if err != nil {
		return fmt.Errorf("create API key: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		db.logger.Warn("failed to get LastInsertId for API key", zap.Error(err))
	}
	key.ID = id
	return nil
}

// GetAPIKeyByID retrieves an API key by its ID.
func (db *DB) GetAPIKeyByID(ctx context.Context, keyID int64) (*APIKey, error) {
	var key APIKey
	var expiresAt, lastUsedAt sql.NullTime
	var scopes, keyPrefix sql.NullString

	err := db.conn.QueryRowContext(ctx, `
		SELECT id, user_id, name, key_hash, key_prefix, scopes, expires_at, last_used_at, created_at
		FROM api_keys WHERE id = ?
	`, keyID).Scan(
		&key.ID, &key.UserID, &key.Name, &key.KeyHash,
		&keyPrefix, &scopes, &expiresAt, &lastUsedAt, &key.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get API key: %w", err)
	}

	key.KeyPrefix = keyPrefix.String
	key.Scopes = scopes.String
	if expiresAt.Valid {
		key.ExpiresAt = &expiresAt.Time
	}
	if lastUsedAt.Valid {
		key.LastUsedAt = &lastUsedAt.Time
	}

	return &key, nil
}

// GetAPIKeyByHash retrieves an API key by its hash.
func (db *DB) GetAPIKeyByHash(ctx context.Context, keyHash string) (*APIKey, error) {
	var key APIKey
	var expiresAt, lastUsedAt sql.NullTime
	var scopes, keyPrefix sql.NullString

	err := db.conn.QueryRowContext(ctx, `
		SELECT id, user_id, name, key_hash, key_prefix, scopes, expires_at, last_used_at, created_at
		FROM api_keys WHERE key_hash = ?
	`, keyHash).Scan(
		&key.ID, &key.UserID, &key.Name, &key.KeyHash,
		&keyPrefix, &scopes, &expiresAt, &lastUsedAt, &key.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get API key: %w", err)
	}

	key.KeyPrefix = keyPrefix.String
	key.Scopes = scopes.String
	if expiresAt.Valid {
		key.ExpiresAt = &expiresAt.Time
	}
	if lastUsedAt.Valid {
		key.LastUsedAt = &lastUsedAt.Time
	}

	return &key, nil
}

// UpdateAPIKeyUsage updates the last used timestamp for an API key.
func (db *DB) UpdateAPIKeyUsage(ctx context.Context, keyID int64) error {
	_, err := db.conn.ExecContext(ctx, `
		UPDATE api_keys SET last_used_at = ? WHERE id = ?
	`, time.Now(), keyID)
	if err != nil {
		return fmt.Errorf("updating API key usage: %w", err)
	}
	return nil
}

// DeleteAPIKey permanently deletes an API key.
func (db *DB) DeleteAPIKey(ctx context.Context, keyID int64) error {
	_, err := db.conn.ExecContext(ctx, `DELETE FROM api_keys WHERE id = ?`, keyID)
	if err != nil {
		return fmt.Errorf("deleting API key: %w", err)
	}
	return nil
}

// ListAPIKeys returns all API keys for a user.
func (db *DB) ListAPIKeys(ctx context.Context, userID int64) ([]*APIKey, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, user_id, name, key_hash, scopes, expires_at, last_used_at, created_at
		FROM api_keys WHERE user_id = ?
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying API keys: %w", err)
	}
	defer rows.Close()

	var keys []*APIKey
	for rows.Next() {
		var key APIKey
		var expiresAt, lastUsedAt sql.NullTime
		var scopes sql.NullString

		err := rows.Scan(&key.ID, &key.UserID, &key.Name, &key.KeyHash,
			&scopes, &expiresAt, &lastUsedAt, &key.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scanning API key: %w", err)
		}

		key.Scopes = scopes.String
		if expiresAt.Valid {
			key.ExpiresAt = &expiresAt.Time
		}
		if lastUsedAt.Valid {
			key.LastUsedAt = &lastUsedAt.Time
		}

		keys = append(keys, &key)
	}
	return keys, rows.Err()
}
