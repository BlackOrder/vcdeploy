// Package storage provides database operations for vcdeploy.
package storage

import (
	"context"
	"fmt"
	"time"
)

// --- Cleanup operations ---

// CleanupExpiredSessions removes sessions that expired before the cutoff time.
func (db *DB) CleanupExpiredSessions(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := db.conn.ExecContext(ctx, `
		DELETE FROM sessions WHERE expires_at < ?
	`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("cleaning up expired sessions: %w", err)
	}
	return result.RowsAffected()
}

// CleanupOldDeployments removes completed deployment records older than the cutoff.
func (db *DB) CleanupOldDeployments(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := db.conn.ExecContext(ctx, `
		DELETE FROM deployments 
		WHERE completed_at IS NOT NULL 
		  AND completed_at < ?
		  AND status IN ('success', 'failed', 'cancelled')
	`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// CleanupOldDeploymentLogs removes deployment logs older than the cutoff.
func (db *DB) CleanupOldDeploymentLogs(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := db.conn.ExecContext(ctx, `
		DELETE FROM deployment_logs 
		WHERE created_at < ?
	`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// CleanupOldAuditLogs removes audit log entries older than the cutoff.
func (db *DB) CleanupOldAuditLogs(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := db.conn.ExecContext(ctx, `
		DELETE FROM audit_logs WHERE timestamp < ?
	`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// MarkStaleAgents marks agents that haven't been seen since the cutoff as disconnected.
func (db *DB) MarkStaleAgents(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := db.conn.ExecContext(ctx, `
		UPDATE agents SET status = 'disconnected'
		WHERE status = 'connected' AND last_seen_at < ?
	`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// CleanupExpiredAPIKeys removes API keys that have expired before now.
func (db *DB) CleanupExpiredAPIKeys(ctx context.Context, now time.Time) (int64, error) {
	result, err := db.conn.ExecContext(ctx, `
		DELETE FROM api_keys 
		WHERE expires_at IS NOT NULL AND expires_at < ?
	`, now)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// CleanupOrphanedWebhooks removes webhook configs for projects that no longer exist.
func (db *DB) CleanupOrphanedWebhooks(ctx context.Context) (int64, error) {
	result, err := db.conn.ExecContext(ctx, `
		DELETE FROM project_webhooks 
		WHERE project_id NOT IN (SELECT id FROM projects)
	`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}


