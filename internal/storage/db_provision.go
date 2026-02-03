// Package storage provides database operations for vcdeploy.
package storage

import (
	"context"
	"database/sql"
	"time"
	"fmt"
)

// --- Provision Job Methods ---

// CreateProvisionJob creates a new provisioning job.
func (db *DB) CreateProvisionJob(ctx context.Context, job *ProvisionJob) error {
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO agent_provision_jobs (id, target_host, target_port, target_user, ssh_key_id, agent_binary_id, status, stage, progress, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, job.ID, job.TargetHost, job.TargetPort, job.TargetUser, job.SSHKeyID, job.AgentBinaryID, job.Status, job.Stage, job.Progress, job.StartedAt)
	if err != nil {
		return fmt.Errorf("creating provision job: %w", err)
	}
	return nil
}

// GetProvisionJob retrieves a provisioning job by ID.
func (db *DB) GetProvisionJob(ctx context.Context, id string) (*ProvisionJob, error) {
	var job ProvisionJob
	var completedAt sql.NullTime
	var stage sql.NullString
	var errorMsg sql.NullString
	var rollbackData sql.NullString

	err := db.conn.QueryRowContext(ctx, `
		SELECT id, target_host, target_port, target_user, ssh_key_id, agent_binary_id, status, stage, progress, error_message, rollback_data, started_at, completed_at
		FROM agent_provision_jobs WHERE id = ?
	`, id).Scan(&job.ID, &job.TargetHost, &job.TargetPort, &job.TargetUser, &job.SSHKeyID, &job.AgentBinaryID, &job.Status, &stage, &job.Progress, &errorMsg, &rollbackData, &job.StartedAt, &completedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting provision job: %w", err)
	}

	if stage.Valid {
		job.Stage = stage.String
	}
	if errorMsg.Valid {
		job.ErrorMessage = errorMsg.String
	}
	if rollbackData.Valid {
		job.RollbackData = rollbackData.String
	}
	if completedAt.Valid {
		job.CompletedAt = &completedAt.Time
	}

	return &job, nil
}

// UpdateProvisionJobStatus updates the status of a provisioning job.
func (db *DB) UpdateProvisionJobStatus(ctx context.Context, id, status, stage, errorMessage string, progress int) error {
	var completedAt interface{}
	if status == "completed" || status == "failed" || status == "cancelled" {
		now := time.Now()
		completedAt = &now
	}

	result, err := db.conn.ExecContext(ctx, `
		UPDATE agent_provision_jobs 
		SET status = ?, stage = ?, progress = ?, error_message = ?, completed_at = ?
		WHERE id = ?
	`, status, stage, progress, errorMessage, completedAt, id)
	if err != nil {
		return fmt.Errorf("updating provision job status: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// ListPendingProvisionJobs returns all pending provisioning jobs.
func (db *DB) ListPendingProvisionJobs(ctx context.Context) ([]*ProvisionJob, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, target_host, target_port, target_user, ssh_key_id, agent_binary_id, status, stage, progress, error_message, rollback_data, started_at, completed_at
		FROM agent_provision_jobs 
		WHERE status IN ('pending', 'running')
		ORDER BY started_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("listing pending provision jobs: %w", err)
	}
	defer rows.Close()

	return scanProvisionJobs(rows)
}

// ListProvisionJobsByHost returns provisioning jobs for a specific host.
func (db *DB) ListProvisionJobsByHost(ctx context.Context, host string, limit, offset int) ([]*ProvisionJob, int64, error) {
	var total int64
	err := db.conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM agent_provision_jobs WHERE target_host = ?
	`, host).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting provision jobs: %w", err)
	}

	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, target_host, target_port, target_user, ssh_key_id, agent_binary_id, status, stage, progress, error_message, rollback_data, started_at, completed_at
		FROM agent_provision_jobs 
		WHERE target_host = ?
		ORDER BY started_at DESC
		LIMIT ? OFFSET ?
	`, host, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing provision jobs: %w", err)
	}
	defer rows.Close()

	jobs, err := scanProvisionJobs(rows)
	if err != nil {
		return nil, 0, err
	}
	return jobs, total, nil
}

// CleanupOldProvisionJobs removes old completed/failed jobs.
func (db *DB) CleanupOldProvisionJobs(ctx context.Context, before time.Time) (int64, error) {
	result, err := db.conn.ExecContext(ctx, `
		DELETE FROM agent_provision_jobs 
		WHERE status IN ('completed', 'failed', 'cancelled') AND completed_at < ?
	`, before)
	if err != nil {
		return 0, fmt.Errorf("cleaning up old provision jobs: %w", err)
	}
	return result.RowsAffected()
}

// SaveProvisionLog saves a log entry for a provisioning job.
func (db *DB) SaveProvisionLog(ctx context.Context, jobID, level, message string) error {
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO provision_logs (job_id, timestamp, level, message)
		VALUES (?, ?, ?, ?)
	`, jobID, time.Now(), level, message)
	if err != nil {
		return fmt.Errorf("saving provision log: %w", err)
	}
	return nil
}

// GetProvisionLogs retrieves all logs for a provisioning job.
func (db *DB) GetProvisionLogs(ctx context.Context, jobID string) ([]*ProvisionLog, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, job_id, timestamp, level, message
		FROM provision_logs
		WHERE job_id = ?
		ORDER BY timestamp ASC
	`, jobID)
	if err != nil {
		return nil, fmt.Errorf("getting provision logs: %w", err)
	}
	defer rows.Close()

	var logs []*ProvisionLog
	for rows.Next() {
		var log ProvisionLog
		if err := rows.Scan(&log.ID, &log.JobID, &log.Timestamp, &log.Level, &log.Message); err != nil {
			return nil, fmt.Errorf("scanning provision log: %w", err)
		}
		logs = append(logs, &log)
	}
	return logs, rows.Err()
}

// scanProvisionJobs is a helper to scan provision job rows.
func scanProvisionJobs(rows *sql.Rows) ([]*ProvisionJob, error) {
	var jobs []*ProvisionJob
	for rows.Next() {
		var job ProvisionJob
		var completedAt sql.NullTime
		var stage sql.NullString
		var errorMsg sql.NullString
		var rollbackData sql.NullString

		if err := rows.Scan(&job.ID, &job.TargetHost, &job.TargetPort, &job.TargetUser, &job.SSHKeyID, &job.AgentBinaryID, &job.Status, &stage, &job.Progress, &errorMsg, &rollbackData, &job.StartedAt, &completedAt); err != nil {
			return nil, fmt.Errorf("scanning provision job: %w", err)
		}

		if stage.Valid {
			job.Stage = stage.String
		}
		if errorMsg.Valid {
			job.ErrorMessage = errorMsg.String
		}
		if rollbackData.Valid {
			job.RollbackData = rollbackData.String
		}
		if completedAt.Valid {
			job.CompletedAt = &completedAt.Time
		}

		jobs = append(jobs, &job)
	}
	return jobs, rows.Err()
}


