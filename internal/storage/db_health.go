// Package storage provides database operations for vcdeploy.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/rs/xid"
)

// --- Health Check Configuration Operations ---

// CreateHealthCheckConfig creates a new health check configuration.
func (db *DB) CreateHealthCheckConfig(ctx context.Context, config *HealthCheckConfig) error {
	if config.UID == "" {
		config.UID = xid.New().String()
	}
	result, err := db.conn.ExecContext(ctx, `
		INSERT INTO health_check_configs (uid, project_id, name, url, method, expected_status, timeout_seconds, 
			retries, retry_delay_seconds, headers, body, body_contains, enabled, is_global)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, config.UID, config.ProjectID, config.Name, config.URL, config.Method, config.ExpectedStatus, config.TimeoutSeconds,
		config.Retries, config.RetryDelaySeconds, config.Headers, config.Body, config.BodyContains,
		config.Enabled, config.IsGlobal)
	if err != nil {
		return fmt.Errorf("creating health check config: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting health check config id: %w", err)
	}
	config.ID = id
	return nil
}

// GetHealthCheckConfig retrieves a health check configuration by ID.
func (db *DB) GetHealthCheckConfig(ctx context.Context, id int64) (*HealthCheckConfig, error) {
	var config HealthCheckConfig
	var projectID sql.NullInt64
	var headers, body, bodyContains sql.NullString

	err := db.conn.QueryRowContext(ctx, `
		SELECT id, uid, project_id, name, url, method, expected_status, timeout_seconds, retries, 
			retry_delay_seconds, headers, body, body_contains, enabled, is_global, created_at, updated_at
		FROM health_check_configs WHERE id = ?
	`, id).Scan(&config.ID, &config.UID, &projectID, &config.Name, &config.URL, &config.Method, &config.ExpectedStatus,
		&config.TimeoutSeconds, &config.Retries, &config.RetryDelaySeconds, &headers, &body, &bodyContains,
		&config.Enabled, &config.IsGlobal, &config.CreatedAt, &config.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting health check config: %w", err)
	}
	if projectID.Valid {
		config.ProjectID = &projectID.Int64
	}
	config.Headers = headers.String
	config.Body = body.String
	config.BodyContains = bodyContains.String
	return &config, nil
}

// GetGlobalHealthCheckConfig retrieves the global health check configuration.
func (db *DB) GetGlobalHealthCheckConfig(ctx context.Context) (*HealthCheckConfig, error) {
	var config HealthCheckConfig
	var projectID sql.NullInt64
	var headers, body, bodyContains sql.NullString

	err := db.conn.QueryRowContext(ctx, `
		SELECT id, uid, project_id, name, url, method, expected_status, timeout_seconds, retries, 
			retry_delay_seconds, headers, body, body_contains, enabled, is_global, created_at, updated_at
		FROM health_check_configs WHERE is_global = 1 LIMIT 1
	`).Scan(&config.ID, &config.UID, &projectID, &config.Name, &config.URL, &config.Method, &config.ExpectedStatus,
		&config.TimeoutSeconds, &config.Retries, &config.RetryDelaySeconds, &headers, &body, &bodyContains,
		&config.Enabled, &config.IsGlobal, &config.CreatedAt, &config.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting global health check config: %w", err)
	}
	if projectID.Valid {
		config.ProjectID = &projectID.Int64
	}
	config.Headers = headers.String
	config.Body = body.String
	config.BodyContains = bodyContains.String
	return &config, nil
}

// GetHealthCheckConfigForProject retrieves the health check config for a project.
// Returns the project-specific config if set, otherwise the global config.
func (db *DB) GetHealthCheckConfigForProject(ctx context.Context, projectID int64) (*HealthCheckConfig, error) {
	// First try to get project-specific config
	var config HealthCheckConfig
	var pid sql.NullInt64
	var headers, body, bodyContains sql.NullString

	err := db.conn.QueryRowContext(ctx, `
		SELECT id, uid, project_id, name, url, method, expected_status, timeout_seconds, retries, 
			retry_delay_seconds, headers, body, body_contains, enabled, is_global, created_at, updated_at
		FROM health_check_configs WHERE project_id = ? AND enabled = 1
	`, projectID).Scan(&config.ID, &config.UID, &pid, &config.Name, &config.URL, &config.Method, &config.ExpectedStatus,
		&config.TimeoutSeconds, &config.Retries, &config.RetryDelaySeconds, &headers, &body, &bodyContains,
		&config.Enabled, &config.IsGlobal, &config.CreatedAt, &config.UpdatedAt)
	if err == nil {
		if pid.Valid {
			config.ProjectID = &pid.Int64
		}
		config.Headers = headers.String
		config.Body = body.String
		config.BodyContains = bodyContains.String
		return &config, nil
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("getting project health check config: %w", err)
	}

	// Fall back to global config
	return db.GetGlobalHealthCheckConfig(ctx)
}

// UpdateHealthCheckConfig updates a health check configuration.
func (db *DB) UpdateHealthCheckConfig(ctx context.Context, config *HealthCheckConfig) error {
	_, err := db.conn.ExecContext(ctx, `
		UPDATE health_check_configs SET
			name = ?, url = ?, method = ?, expected_status = ?, timeout_seconds = ?,
			retries = ?, retry_delay_seconds = ?, headers = ?, body = ?, body_contains = ?,
			enabled = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, config.Name, config.URL, config.Method, config.ExpectedStatus, config.TimeoutSeconds,
		config.Retries, config.RetryDelaySeconds, config.Headers, config.Body, config.BodyContains,
		config.Enabled, config.ID)
	if err != nil {
		return fmt.Errorf("updating health check config: %w", err)
	}
	return nil
}

// ListHealthCheckConfigs retrieves all health check configurations.
func (db *DB) ListHealthCheckConfigs(ctx context.Context) ([]*HealthCheckConfig, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, uid, project_id, name, url, method, expected_status, timeout_seconds, retries, 
			retry_delay_seconds, headers, body, body_contains, enabled, is_global, created_at, updated_at
		FROM health_check_configs ORDER BY is_global DESC, name
	`)
	if err != nil {
		return nil, fmt.Errorf("listing health check configs: %w", err)
	}
	defer rows.Close()

	var configs []*HealthCheckConfig
	for rows.Next() {
		var config HealthCheckConfig
		var projectID sql.NullInt64
		var headers, body, bodyContains sql.NullString

		if err := rows.Scan(&config.ID, &config.UID, &projectID, &config.Name, &config.URL, &config.Method, &config.ExpectedStatus,
			&config.TimeoutSeconds, &config.Retries, &config.RetryDelaySeconds, &headers, &body, &bodyContains,
			&config.Enabled, &config.IsGlobal, &config.CreatedAt, &config.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning health check config: %w", err)
		}
		if projectID.Valid {
			config.ProjectID = &projectID.Int64
		}
		config.Headers = headers.String
		config.Body = body.String
		config.BodyContains = bodyContains.String
		configs = append(configs, &config)
	}
	return configs, rows.Err()
}

// DeleteHealthCheckConfig deletes a health check configuration.
func (db *DB) DeleteHealthCheckConfig(ctx context.Context, id int64) error {
	// Don't allow deleting the global config
	var isGlobal bool
	err := db.conn.QueryRowContext(ctx, `SELECT is_global FROM health_check_configs WHERE id = ?`, id).Scan(&isGlobal)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("checking global config: %w", err)
	}
	if isGlobal {
		return fmt.Errorf("cannot delete global health check config")
	}

	_, err = db.conn.ExecContext(ctx, `DELETE FROM health_check_configs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting health check config: %w", err)
	}
	return nil
}
