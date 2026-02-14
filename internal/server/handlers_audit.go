// Package server provides audit log handlers for the master server.
package server

import (
	"context"
	"net/http"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// handleAuditLogs handles GET for /api/v1/audit.
func (s *MasterServer) handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := r.Context()

	// Admin-only: viewing audit logs
	if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	// Parse pagination params
	p := parsePagination(r)

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	entries, err := s.auditService.List(ctx, p.Limit, p.Offset)
	if err != nil {
		s.logger.Error("Failed to list audit logs", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Note: totalCount is approximate - we return number of items fetched
	// For a proper total, the storage layer would need a Count method
	var totalCount int
	if len(entries) == p.Limit {
		// If we got exactly limit items, there are likely more
		// This is an approximation; proper totalCount would require a COUNT query
		totalCount = p.Offset + p.Limit + 1
	} else {
		totalCount = p.Offset + len(entries)
	}

	s.jsonResponse(w, PaginatedResponse{
		Items:      entries,
		TotalCount: int64(totalCount),
		Limit:      p.Limit,
		Offset:     p.Offset,
	})
}

// logAudit creates an audit log entry with request context.
// C4 FIX: Audit failures are logged at ERROR level with full context.
// Audit logging is critical for security compliance - failures must be visible.
func (s *MasterServer) logAudit(r *http.Request, action, resource, details, result string) {
	// Get username from context (set by auth middleware)
	user := "anonymous"
	if username, ok := r.Context().Value("username").(string); ok {
		user = username
	}

	// Get IP address
	ip := extractClientIP(r)

	entry := &storage.AuditEntry{
		Source:    "http",
		User:      user,
		Action:    action,
		Resource:  resource,
		Details:   details,
		IPAddress: ip,
		Result:    result,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.auditService.Log(ctx, entry); err != nil {
		// C4: Log detailed context on audit failure for investigation
		s.logger.Error("AUDIT FAILURE: Failed to write audit log",
			zap.Error(err),
			zap.String("action", action),
			zap.String("resource", resource),
			zap.String("user", user),
			zap.String("ip", ip),
			zap.String("result", result),
			zap.String("details", details),
		)
	} else {
		// Publish SSE event for real-time audit monitoring
		s.publishAuditEvent(entry.ID, user, action, resource, ip)
	}
}

// logAuditWithSnapshot creates an audit log entry with request context and a resource snapshot.
// This is used for delete operations to capture the resource state before deletion.
// C4 FIX: Audit failures are logged at ERROR level with full context.
func (s *MasterServer) logAuditWithSnapshot(r *http.Request, action, resource, resourceID string, snapshot any, details, result string) {
	// Get username from context (set by auth middleware)
	user := "anonymous"
	if username, ok := r.Context().Value("username").(string); ok {
		user = username
	}

	// Get IP address
	ip := extractClientIP(r)

	entry := &storage.AuditEntry{
		Source:     "http",
		User:       user,
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Details:    details,
		IPAddress:  ip,
		Result:     result,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.auditService.LogWithSnapshot(ctx, entry, snapshot); err != nil {
		// C4: Log detailed context on audit failure for investigation
		s.logger.Error("AUDIT FAILURE: Failed to write audit log with snapshot",
			zap.Error(err),
			zap.String("action", action),
			zap.String("resource", resource),
			zap.String("resource_id", resourceID),
			zap.String("user", user),
			zap.String("ip", ip),
			zap.String("result", result),
			zap.String("details", details),
		)
	} else {
		// Publish SSE event for real-time audit monitoring
		s.publishAuditEvent(entry.ID, user, action, resource, ip)
	}
}
