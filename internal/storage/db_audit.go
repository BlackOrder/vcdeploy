// Package storage provides database operations for vcdeploy.
package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// --- Audit log operations ---

// LogAudit creates an audit log entry.
func (db *DB) LogAudit(ctx context.Context, entry *AuditEntry) error {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO audit_logs (timestamp, source, user, action, resource, resource_id, resource_data, details, ip_address, result)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, entry.Timestamp, entry.Source, entry.User, entry.Action, entry.Resource, entry.ResourceID, entry.ResourceData, entry.Details, entry.IPAddress, entry.Result)
	if err != nil {
		return fmt.Errorf("logging audit entry: %w", err)
	}
	return nil
}

// LogAuditWithSnapshot creates an audit log entry with a JSON snapshot of the resource.
// This is useful for capturing resource state before deletion.
func (db *DB) LogAuditWithSnapshot(ctx context.Context, entry *AuditEntry, resourceSnapshot any) error {
	if resourceSnapshot != nil {
		data, err := json.Marshal(resourceSnapshot)
		if err != nil {
			return fmt.Errorf("marshal resource snapshot: %w", err)
		}
		entry.ResourceData = string(data)
	}
	return db.LogAudit(ctx, entry)
}

// ListAuditLogs returns audit log entries with optional filtering.
func (db *DB) ListAuditLogs(ctx context.Context, limit int, offset int) ([]*AuditEntry, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, timestamp, source, user, action, resource, COALESCE(resource_id, ''), COALESCE(resource_data, ''), details, ip_address, result
		FROM audit_logs ORDER BY timestamp DESC LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("querying audit logs: %w", err)
	}
	defer rows.Close()

	var entries []*AuditEntry
	for rows.Next() {
		var entry AuditEntry
		if err := rows.Scan(&entry.ID, &entry.Timestamp, &entry.Source, &entry.User,
			&entry.Action, &entry.Resource, &entry.ResourceID, &entry.ResourceData, &entry.Details, &entry.IPAddress, &entry.Result); err != nil {
			return nil, fmt.Errorf("scanning audit entry: %w", err)
		}
		entries = append(entries, &entry)
	}

	return entries, rows.Err()
}

// ListAuditLogsSince returns audit log entries since the given time.
func (db *DB) ListAuditLogsSince(ctx context.Context, since time.Time) ([]*AuditEntry, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, timestamp, source, user, action, resource, COALESCE(resource_id, ''), COALESCE(resource_data, ''), details, ip_address, result
		FROM audit_logs WHERE timestamp >= ? ORDER BY timestamp DESC
	`, since)
	if err != nil {
		return nil, fmt.Errorf("querying audit logs since %v: %w", since, err)
	}
	defer rows.Close()

	var entries []*AuditEntry
	for rows.Next() {
		var entry AuditEntry
		if err := rows.Scan(&entry.ID, &entry.Timestamp, &entry.Source, &entry.User,
			&entry.Action, &entry.Resource, &entry.ResourceID, &entry.ResourceData, &entry.Details, &entry.IPAddress, &entry.Result); err != nil {
			return nil, fmt.Errorf("scanning audit entry: %w", err)
		}
		entries = append(entries, &entry)
	}

	return entries, rows.Err()
}
