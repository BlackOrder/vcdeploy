// Package audit provides audit logging functionality.
package audit

import (
	"context"
	"fmt"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// Ensure Service implements the interface.
var _ services.AuditServicer = (*Service)(nil)

// Service handles audit logging.
type Service struct {
	db *storage.DB
}

// New creates a new audit Service.
func New(db *storage.DB) *Service {
	return &Service{db: db}
}

// Log creates an audit log entry.
func (s *Service) Log(ctx context.Context, entry *storage.AuditEntry) error {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	if err := s.db.LogAudit(ctx, entry); err != nil {
		return fmt.Errorf("logging audit entry: %w", err)
	}
	return nil
}

// LogWithSnapshot creates an audit log entry with a JSON snapshot of the resource.
// This is useful for capturing resource state before deletion.
func (s *Service) LogWithSnapshot(ctx context.Context, entry *storage.AuditEntry, resourceSnapshot any) error {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	if err := s.db.LogAuditWithSnapshot(ctx, entry, resourceSnapshot); err != nil {
		return fmt.Errorf("logging audit entry with snapshot: %w", err)
	}
	return nil
}

// List returns audit log entries with pagination.
func (s *Service) List(ctx context.Context, limit, offset int) ([]*storage.AuditEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	entries, err := s.db.ListAuditLogs(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing audit logs: %w", err)
	}
	return entries, nil
}

// Cleanup removes audit log entries older than the cutoff.
func (s *Service) Cleanup(ctx context.Context, cutoff time.Time) (int64, error) {
	count, err := s.db.CleanupOldAuditLogs(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("cleaning up audit logs: %w", err)
	}
	return count, nil
}

// LogAction is a convenience method for logging common actions.
func (s *Service) LogAction(ctx context.Context, source, user, action, resource, details, ipAddress, result string) error {
	return s.Log(ctx, &storage.AuditEntry{
		Source:    source,
		User:      user,
		Action:    action,
		Resource:  resource,
		Details:   details,
		IPAddress: ipAddress,
		Result:    result,
	})
}
