// Package server provides the master daemon HTTP and gRPC servers.
package server

import (
	"context"
	"net/http"
)

// Auth helper methods for MasterServer.
// These methods reduce boilerplate when checking access permissions in handlers.
// All methods return JSON error responses via jsonError().

// requireReadAccess checks read permission and writes JSON error response if denied.
// Returns true if access is granted, false if denied (error already written to w).
func (s *MasterServer) requireReadAccess(ctx context.Context, w http.ResponseWriter) bool {
	if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return false
	}
	return true
}

// requireWriteAccess checks write permission and writes JSON error response if denied.
// Returns true if access is granted, false if denied (error already written to w).
func (s *MasterServer) requireWriteAccess(ctx context.Context, w http.ResponseWriter) bool {
	if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return false
	}
	return true
}

// requireAdminAccess checks admin permission and writes JSON error response if denied.
// Returns true if access is granted, false if denied (error already written to w).
func (s *MasterServer) requireAdminAccess(ctx context.Context, w http.ResponseWriter) bool {
	if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return false
	}
	return true
}
