// Package storage provides database operations for vcdeploy.
package storage

import (
	"context"
	"database/sql"
	"fmt"
)

// --- Secret operations ---

// SetSecretEncrypted creates or updates a secret with pre-encrypted value.
func (db *DB) SetSecretEncrypted(ctx context.Context, project, scope, key string, valueEncrypted []byte) error {
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO secrets (project, scope, key, value_encrypted)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(project, scope, key) DO UPDATE SET
			value_encrypted = excluded.value_encrypted,
			updated_at = CURRENT_TIMESTAMP
	`, project, scope, key, valueEncrypted)
	if err != nil {
		return fmt.Errorf("setting encrypted secret: %w", err)
	}
	return nil
}

// GetSecret retrieves a secret.
func (db *DB) GetSecret(ctx context.Context, project, scope, key string) (*Secret, error) {
	var s Secret
	err := db.conn.QueryRowContext(ctx, `
		SELECT id, project, scope, key, value_encrypted, created_at, updated_at
		FROM secrets WHERE project = ? AND scope = ? AND key = ?
	`, project, scope, key).Scan(
		&s.ID, &s.Project, &s.Scope, &s.Key, &s.ValueEncrypted, &s.CreatedAt, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying secret: %w", err)
	}
	return &s, nil
}

// ListSecrets returns secret metadata for a scope (CLI version)
func (db *DB) ListSecrets(scope string) ([]*SecretInfo, error) {
	rows, err := db.conn.Query(`
		SELECT key, scope, updated_at FROM secrets WHERE project = ? ORDER BY key
	`, scope)
	if err != nil {
		return nil, fmt.Errorf("querying secrets: %w", err)
	}
	defer rows.Close()

	var secrets []*SecretInfo
	for rows.Next() {
		var s SecretInfo
		if err := rows.Scan(&s.Key, &s.Scope, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning secret: %w", err)
		}
		secrets = append(secrets, &s)
	}
	return secrets, rows.Err()
}

// ListSecretsCtx returns all secrets for a project (without values) with context.
func (db *DB) ListSecretsCtx(ctx context.Context, project string) ([]*Secret, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, project, scope, key, created_at, updated_at
		FROM secrets WHERE project = ? ORDER BY scope, key
	`, project)
	if err != nil {
		return nil, fmt.Errorf("querying secrets: %w", err)
	}
	defer rows.Close()

	var secrets []*Secret
	for rows.Next() {
		var s Secret
		if err := rows.Scan(&s.ID, &s.Project, &s.Scope, &s.Key, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning secret: %w", err)
		}
		secrets = append(secrets, &s)
	}

	return secrets, rows.Err()
}

// DeleteSecret deletes a secret (CLI version).
func (db *DB) DeleteSecret(scope, key string) error {
	_, err := db.conn.Exec(`DELETE FROM secrets WHERE project = ? AND key = ?`, scope, key)
	if err != nil {
		return fmt.Errorf("deleting secret: %w", err)
	}
	return nil
}

// DeleteSecretCtx deletes a secret with context.
func (db *DB) DeleteSecretCtx(ctx context.Context, project, scope, key string) error {
	_, err := db.conn.ExecContext(ctx, `
		DELETE FROM secrets WHERE project = ? AND scope = ? AND key = ?
	`, project, scope, key)
	if err != nil {
		return fmt.Errorf("deleting secret: %w", err)
	}
	return nil
}

// ListSecretsWithScope returns all secrets for a project and scope with encrypted values.
func (db *DB) ListSecretsWithScope(ctx context.Context, project, scope string) ([]*Secret, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, project, scope, key, value_encrypted, created_at, updated_at
		FROM secrets WHERE project = ? AND scope = ? ORDER BY key
	`, project, scope)
	if err != nil {
		return nil, fmt.Errorf("querying secrets: %w", err)
	}
	defer rows.Close()

	var secrets []*Secret
	for rows.Next() {
		var s Secret
		if err := rows.Scan(&s.ID, &s.Project, &s.Scope, &s.Key, &s.ValueEncrypted, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning secret: %w", err)
		}
		secrets = append(secrets, &s)
	}
	return secrets, rows.Err()
}

// ListAllSecretsCtx returns all secrets from all projects (for re-encryption).
func (db *DB) ListAllSecretsCtx(ctx context.Context) ([]*Secret, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, project, scope, key, value_encrypted, created_at, updated_at
		FROM secrets ORDER BY project, scope, key
	`)
	if err != nil {
		return nil, fmt.Errorf("querying all secrets: %w", err)
	}
	defer rows.Close()

	var secrets []*Secret
	for rows.Next() {
		var s Secret
		if err := rows.Scan(&s.ID, &s.Project, &s.Scope, &s.Key, &s.ValueEncrypted, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning secret: %w", err)
		}
		secrets = append(secrets, &s)
	}
	return secrets, rows.Err()
}


