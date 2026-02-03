// Package storage provides database operations for vcdeploy.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// --- Deployment operations ---

// CreateDeployment creates a new deployment record.
func (db *DB) CreateDeployment(ctx context.Context, d *DeploymentRecord) error {
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO deployments (id, project, target, branch, commit_hash, status, triggered_by, trigger_source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, d.ID, d.Project, d.Target, d.Branch, d.CommitHash, d.Status, d.TriggeredBy, d.TriggerSource)
	if err != nil {
		return fmt.Errorf("creating deployment: %w", err)
	}
	return nil
}

// UpdateDeployment updates a deployment record.
func (db *DB) UpdateDeployment(ctx context.Context, d *DeploymentRecord) error {
	_, err := db.conn.ExecContext(ctx, `
		UPDATE deployments SET
			status = ?, release_number = ?, completed_at = ?, error_message = ?
		WHERE id = ?
	`, d.Status, d.ReleaseNumber, d.CompletedAt, d.ErrorMessage, d.ID)
	if err != nil {
		return fmt.Errorf("updating deployment: %w", err)
	}
	return nil
}

// GetDeployment retrieves a deployment by ID.
func (db *DB) GetDeployment(ctx context.Context, id string) (*DeploymentRecord, error) {
	var d DeploymentRecord
	var completedAt sql.NullTime
	var releaseNumber sql.NullInt64
	var commitHash, triggeredBy, triggerSource, errorMessage sql.NullString

	err := db.conn.QueryRowContext(ctx, `
		SELECT id, project, target, branch, commit_hash, status, release_number,
		       started_at, completed_at, triggered_by, trigger_source, error_message
		FROM deployments WHERE id = ?
	`, id).Scan(
		&d.ID, &d.Project, &d.Target, &d.Branch, &commitHash, &d.Status, &releaseNumber,
		&d.StartedAt, &completedAt, &triggeredBy, &triggerSource, &errorMessage,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying deployment: %w", err)
	}

	if completedAt.Valid {
		d.CompletedAt = &completedAt.Time
	}
	if releaseNumber.Valid {
		d.ReleaseNumber = int(releaseNumber.Int64)
	}
	d.CommitHash = commitHash.String
	d.TriggeredBy = triggeredBy.String
	d.TriggerSource = triggerSource.String
	d.ErrorMessage = errorMessage.String
	return &d, nil
}

// --- Deployment log operations ---

// CreateDeploymentLog creates a deployment log entry.
func (db *DB) CreateDeploymentLog(ctx context.Context, log *DeploymentLog) error {
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO deployment_logs (deployment_id, level, message, source, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, log.DeploymentID, log.Level, log.Message, log.Source, log.CreatedAt)
	if err != nil {
		return fmt.Errorf("creating deployment log: %w", err)
	}
	return nil
}

// ListDeploymentLogs returns logs for a deployment.
func (db *DB) ListDeploymentLogs(ctx context.Context, deploymentID string) ([]*DeploymentLog, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, deployment_id, level, message, source, created_at
		FROM deployment_logs WHERE deployment_id = ? ORDER BY created_at
	`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("querying deployment logs: %w", err)
	}
	defer rows.Close()

	var logs []*DeploymentLog
	for rows.Next() {
		var log DeploymentLog
		if err := rows.Scan(&log.ID, &log.DeploymentID, &log.Level, &log.Message, &log.Source, &log.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning deployment log: %w", err)
		}
		logs = append(logs, &log)
	}
	return logs, rows.Err()
}

// ListDeploymentLogsAfter returns logs for a deployment after a specific log ID.
// This is used for streaming/polling new logs.
func (db *DB) ListDeploymentLogsAfter(ctx context.Context, deploymentID string, afterID int64) ([]*DeploymentLog, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, deployment_id, level, message, source, created_at
		FROM deployment_logs WHERE deployment_id = ? AND id > ? ORDER BY created_at
	`, deploymentID, afterID)
	if err != nil {
		return nil, fmt.Errorf("querying deployment logs: %w", err)
	}
	defer rows.Close()

	var logs []*DeploymentLog
	for rows.Next() {
		var log DeploymentLog
		if err := rows.Scan(&log.ID, &log.DeploymentID, &log.Level, &log.Message, &log.Source, &log.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning deployment log: %w", err)
		}
		logs = append(logs, &log)
	}
	return logs, rows.Err()
}

// ListDeploymentLogsPaginated returns deployment logs with pagination support.
func (db *DB) ListDeploymentLogsPaginated(ctx context.Context, deploymentID string, limit, offset int) ([]*DeploymentLog, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, deployment_id, level, message, source, created_at
		FROM deployment_logs WHERE deployment_id = ? ORDER BY created_at
		LIMIT ? OFFSET ?
	`, deploymentID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("querying deployment logs: %w", err)
	}
	defer rows.Close()

	var logs []*DeploymentLog
	for rows.Next() {
		var log DeploymentLog
		if err := rows.Scan(&log.ID, &log.DeploymentID, &log.Level, &log.Message, &log.Source, &log.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning deployment log: %w", err)
		}
		logs = append(logs, &log)
	}
	return logs, rows.Err()
}

// CountDeploymentLogs returns the total number of logs for a deployment.
func (db *DB) CountDeploymentLogs(ctx context.Context, deploymentID string) (int64, error) {
	var count int64
	err := db.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM deployment_logs WHERE deployment_id = ?`, deploymentID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting deployment logs: %w", err)
	}
	return count, nil
}

// --- Scheduled Deployment operations ---

// CreateScheduledDeployment creates a scheduled deployment.
func (db *DB) CreateScheduledDeployment(ctx context.Context, id, project, target, branch string, scheduledAt time.Time, scheduledBy string) error {
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO deployments (id, project, target, branch, status, scheduled_at, scheduled_by, triggered_by)
		VALUES (?, ?, ?, ?, 'scheduled', ?, ?, ?)
	`, id, project, target, branch, scheduledAt, scheduledBy, scheduledBy)
	if err != nil {
		return fmt.Errorf("creating scheduled deployment: %w", err)
	}
	return nil
}

// ListPendingScheduledDeployments returns deployments that are due to run.
func (db *DB) ListPendingScheduledDeployments(ctx context.Context) ([]*ScheduledDeployment, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, project, target, branch, scheduled_at, scheduled_by, status
		FROM deployments 
		WHERE scheduled_at IS NOT NULL 
		  AND scheduled_at <= datetime('now') 
		  AND status = 'scheduled'
		ORDER BY scheduled_at
	`)
	if err != nil {
		return nil, fmt.Errorf("querying scheduled deployments: %w", err)
	}
	defer rows.Close()

	var deployments []*ScheduledDeployment
	for rows.Next() {
		var d ScheduledDeployment
		if err := rows.Scan(&d.ID, &d.Project, &d.Target, &d.Branch, &d.ScheduledAt, &d.ScheduledBy, &d.Status); err != nil {
			return nil, fmt.Errorf("scanning deployment: %w", err)
		}
		deployments = append(deployments, &d)
	}
	return deployments, rows.Err()
}

// CancelScheduledDeployment cancels a scheduled deployment.
func (db *DB) CancelScheduledDeployment(ctx context.Context, id string) error {
	result, err := db.conn.ExecContext(ctx, `
		UPDATE deployments SET status = 'cancelled', completed_at = datetime('now')
		WHERE id = ? AND status = 'scheduled'
	`, id)
	if err != nil {
		return fmt.Errorf("cancelling scheduled deployment: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		db.logger.Warn("failed to get RowsAffected for cancel deployment", zap.Error(err))
	}
	if rows == 0 {
		return fmt.Errorf("deployment not found or not in scheduled status")
	}
	return nil
}

// --- Additional Deployment operations ---

// ListDeploymentsRecent returns recent deployments.
func (db *DB) ListDeploymentsRecent(ctx context.Context, limit int) ([]*DeploymentRecord, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, project, target, branch, commit_hash, status, release_number,
		       started_at, completed_at, triggered_by, trigger_source, error_message
		FROM deployments
		ORDER BY started_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("querying deployments: %w", err)
	}
	defer rows.Close()

	var deployments []*DeploymentRecord
	for rows.Next() {
		var d DeploymentRecord
		var completedAt sql.NullTime
		var releaseNumber sql.NullInt64
		var commitHash, triggeredBy, triggerSource, errorMessage sql.NullString

		if err := rows.Scan(
			&d.ID, &d.Project, &d.Target, &d.Branch, &commitHash, &d.Status, &releaseNumber,
			&d.StartedAt, &completedAt, &triggeredBy, &triggerSource, &errorMessage,
		); err != nil {
			return nil, fmt.Errorf("scanning deployment: %w", err)
		}

		if completedAt.Valid {
			d.CompletedAt = &completedAt.Time
		}
		if releaseNumber.Valid {
			d.ReleaseNumber = int(releaseNumber.Int64)
		}
		d.CommitHash = commitHash.String
		d.TriggeredBy = triggeredBy.String
		d.TriggerSource = triggerSource.String
		d.ErrorMessage = errorMessage.String
		deployments = append(deployments, &d)
	}
	return deployments, rows.Err()
}

// CountDeploymentsByStatus returns deployment counts grouped by status.
func (db *DB) CountDeploymentsByStatus(ctx context.Context) (map[string]int64, error) {
	rows, err := db.conn.QueryContext(ctx, `SELECT COALESCE(status, 'unknown'), COUNT(*) FROM deployments GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("counting deployments by status: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("scanning deployment count: %w", err)
		}
		counts[status] = count
	}
	return counts, rows.Err()
}

// ListDeploymentsPaginated returns deployments with pagination support.
func (db *DB) ListDeploymentsPaginated(ctx context.Context, limit, offset int) ([]*DeploymentRecord, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, project, target, branch, commit_hash, status, release_number,
		       started_at, completed_at, triggered_by, trigger_source, error_message
		FROM deployments
		ORDER BY started_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("querying deployments: %w", err)
	}
	defer rows.Close()

	var deployments []*DeploymentRecord
	for rows.Next() {
		var d DeploymentRecord
		var completedAt sql.NullTime
		var releaseNumber sql.NullInt64
		var commitHash, triggeredBy, triggerSource, errorMessage sql.NullString

		if err := rows.Scan(
			&d.ID, &d.Project, &d.Target, &d.Branch, &commitHash, &d.Status, &releaseNumber,
			&d.StartedAt, &completedAt, &triggeredBy, &triggerSource, &errorMessage,
		); err != nil {
			return nil, fmt.Errorf("scanning deployment: %w", err)
		}

		if completedAt.Valid {
			d.CompletedAt = &completedAt.Time
		}
		if releaseNumber.Valid {
			d.ReleaseNumber = int(releaseNumber.Int64)
		}
		d.CommitHash = commitHash.String
		d.TriggeredBy = triggeredBy.String
		d.TriggerSource = triggerSource.String
		d.ErrorMessage = errorMessage.String
		deployments = append(deployments, &d)
	}
	return deployments, rows.Err()
}

// CountDeployments returns the total number of deployments.
func (db *DB) CountDeployments(ctx context.Context) (int64, error) {
	var count int64
	err := db.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM deployments`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting deployments: %w", err)
	}
	return count, nil
}

// --- Deployment Rollback Operations ---

// CreateDeploymentRollback creates a new rollback record.
func (db *DB) CreateDeploymentRollback(ctx context.Context, rollback *DeploymentRollback) error {
	result, err := db.conn.ExecContext(ctx, `
		INSERT INTO deployment_rollbacks (deployment_id, project_name, from_release, to_release, reason, 
			triggered_by, health_check_failed, health_check_error, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, rollback.DeploymentID, rollback.ProjectName, rollback.FromRelease, rollback.ToRelease,
		rollback.Reason, rollback.TriggeredBy, rollback.HealthCheckFailed, rollback.HealthCheckError, rollback.Status)
	if err != nil {
		return fmt.Errorf("creating deployment rollback: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting rollback id: %w", err)
	}
	rollback.ID = id
	return nil
}

// GetDeploymentRollback retrieves a rollback record by ID.
func (db *DB) GetDeploymentRollback(ctx context.Context, id int64) (*DeploymentRollback, error) {
	var rollback DeploymentRollback
	var completedAt sql.NullTime
	var errorMsg, healthError sql.NullString

	err := db.conn.QueryRowContext(ctx, `
		SELECT id, deployment_id, project_name, from_release, to_release, reason, triggered_by,
			health_check_failed, health_check_error, status, error_message, started_at, completed_at
		FROM deployment_rollbacks WHERE id = ?
	`, id).Scan(&rollback.ID, &rollback.DeploymentID, &rollback.ProjectName, &rollback.FromRelease,
		&rollback.ToRelease, &rollback.Reason, &rollback.TriggeredBy, &rollback.HealthCheckFailed,
		&healthError, &rollback.Status, &errorMsg, &rollback.StartedAt, &completedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting deployment rollback: %w", err)
	}
	rollback.ErrorMessage = errorMsg.String
	rollback.HealthCheckError = healthError.String
	if completedAt.Valid {
		rollback.CompletedAt = &completedAt.Time
	}
	return &rollback, nil
}

// UpdateDeploymentRollback updates a rollback record status.
func (db *DB) UpdateDeploymentRollback(ctx context.Context, rollback *DeploymentRollback) error {
	_, err := db.conn.ExecContext(ctx, `
		UPDATE deployment_rollbacks SET status = ?, error_message = ?, completed_at = ? WHERE id = ?
	`, rollback.Status, rollback.ErrorMessage, rollback.CompletedAt, rollback.ID)
	if err != nil {
		return fmt.Errorf("updating deployment rollback: %w", err)
	}
	return nil
}

// ListDeploymentRollbacks retrieves rollback records with optional filtering.
func (db *DB) ListDeploymentRollbacks(ctx context.Context, projectName string, limit, offset int) ([]*DeploymentRollback, int64, error) {
	// Build query based on filter
	countQuery := `SELECT COUNT(*) FROM deployment_rollbacks`
	selectQuery := `
		SELECT id, deployment_id, project_name, from_release, to_release, reason, triggered_by,
			health_check_failed, health_check_error, status, error_message, started_at, completed_at
		FROM deployment_rollbacks`
	orderQuery := ` ORDER BY started_at DESC LIMIT ? OFFSET ?`

	var args []interface{}
	if projectName != "" {
		countQuery += ` WHERE project_name = ?`
		selectQuery += ` WHERE project_name = ?`
		args = append(args, projectName)
	}

	// Get total count
	var total int64
	err := db.conn.QueryRowContext(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting rollbacks: %w", err)
	}

	args = append(args, limit, offset)
	rows, err := db.conn.QueryContext(ctx, selectQuery+orderQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing deployment rollbacks: %w", err)
	}
	defer rows.Close()

	var rollbacks []*DeploymentRollback
	for rows.Next() {
		var rollback DeploymentRollback
		var completedAt sql.NullTime
		var errorMsg, healthError sql.NullString

		if err := rows.Scan(&rollback.ID, &rollback.DeploymentID, &rollback.ProjectName, &rollback.FromRelease,
			&rollback.ToRelease, &rollback.Reason, &rollback.TriggeredBy, &rollback.HealthCheckFailed,
			&healthError, &rollback.Status, &errorMsg, &rollback.StartedAt, &completedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning rollback: %w", err)
		}
		rollback.ErrorMessage = errorMsg.String
		rollback.HealthCheckError = healthError.String
		if completedAt.Valid {
			rollback.CompletedAt = &completedAt.Time
		}
		rollbacks = append(rollbacks, &rollback)
	}
	return rollbacks, total, rows.Err()
}

// GetLatestRollbackForDeployment returns the most recent rollback for a deployment.
func (db *DB) GetLatestRollbackForDeployment(ctx context.Context, deploymentID string) (*DeploymentRollback, error) {
	var rollback DeploymentRollback
	var completedAt sql.NullTime
	var errorMsg, healthError sql.NullString

	err := db.conn.QueryRowContext(ctx, `
		SELECT id, deployment_id, project_name, from_release, to_release, reason, triggered_by,
			health_check_failed, health_check_error, status, error_message, started_at, completed_at
		FROM deployment_rollbacks WHERE deployment_id = ? ORDER BY started_at DESC LIMIT 1
	`, deploymentID).Scan(&rollback.ID, &rollback.DeploymentID, &rollback.ProjectName, &rollback.FromRelease,
		&rollback.ToRelease, &rollback.Reason, &rollback.TriggeredBy, &rollback.HealthCheckFailed,
		&healthError, &rollback.Status, &errorMsg, &rollback.StartedAt, &completedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting latest rollback for deployment: %w", err)
	}
	rollback.ErrorMessage = errorMsg.String
	rollback.HealthCheckError = healthError.String
	if completedAt.Valid {
		rollback.CompletedAt = &completedAt.Time
	}
	return &rollback, nil
}

// UpdateProjectHealthCheck updates a project's health check configuration reference.
func (db *DB) UpdateProjectHealthCheck(ctx context.Context, projectID int64, healthCheckID *int64, autoRollback, rollbackOnHealthFail bool) error {
	_, err := db.conn.ExecContext(ctx, `
		UPDATE projects SET health_check_id = ?, auto_rollback_enabled = ?, rollback_on_health_fail = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, healthCheckID, autoRollback, rollbackOnHealthFail, projectID)
	if err != nil {
		return fmt.Errorf("updating project health check: %w", err)
	}
	return nil
}
