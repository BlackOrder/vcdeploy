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
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Admin-only: viewing audit logs
	if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
		http.Error(w, msg, status)
		return
	}

	// Parse pagination params
	p := parsePagination(r)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	entries, err := s.auditService.List(ctx, p.Limit, p.Offset)
	if err != nil {
		s.logger.Error("Failed to list audit logs", zap.Error(err))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, entries)
}

// logAudit creates an audit log entry with request context.
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
		s.logger.Error("Failed to write audit log", zap.Error(err))
	}
}

// logAuditWithSnapshot creates an audit log entry with request context and a resource snapshot.
// This is used for delete operations to capture the resource state before deletion.
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
		s.logger.Error("Failed to write audit log with snapshot", zap.Error(err))
	}
}
