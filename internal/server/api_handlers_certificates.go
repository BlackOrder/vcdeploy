// Package server provides API endpoint handlers.
package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	coreSecurity "github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/services/security"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// --- Certificate API Handlers ---

// handleCertificates handles /api/v1/certificates/agents endpoints.
func (s *MasterServer) handleCertificates(w http.ResponseWriter, r *http.Request) {
	// Route: /api/v1/certificates/agents
	// or:    /api/v1/certificates/agents/{id}
	// or:    /api/v1/certificates/agents/{id}/revoke

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/certificates/agents")

	if path == "" || path == "/" {
		// List all agent certificates
		s.handleListAgentCertificates(w, r)
		return
	}

	// Extract agent ID from path
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	agentID := parts[0]

	if len(parts) == 1 {
		// /api/v1/certificates/agents/{id}
		s.handleGetAgentCertificate(w, r, agentID)
		return
	}

	if len(parts) == 2 && parts[1] == "revoke" {
		// /api/v1/certificates/agents/{id}/revoke
		s.handleRevokeAgentCertificate(w, r, agentID)
		return
	}

	s.jsonError(w, http.StatusNotFound, "Not found")
}

// handleListAgentCertificates lists all agent certificates.
func (s *MasterServer) handleListAgentCertificates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx := r.Context()

	certService := security.NewCertificateService(s.store, s.logger)
	certs, err := certService.ListAgentCertificates(ctx)
	if err != nil {
		s.logger.Error("Failed to list agent certificates", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.jsonResponse(w, ListCountResponse{
		Items: certs,
		Count: len(certs),
	})
}

// handleGetAgentCertificate gets a specific agent's certificate.
func (s *MasterServer) handleGetAgentCertificate(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx := r.Context()

	certService := security.NewCertificateService(s.store, s.logger)
	cert, err := certService.GetAgentCertificate(ctx, agentID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.jsonError(w, http.StatusNotFound, "Certificate not found")
			return
		}
		s.logger.Error("Failed to get agent certificate", zap.Error(err), zap.String("agent_id", agentID))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.jsonResponse(w, cert)
}

// RevokeRequest represents a certificate revocation request.
type RevokeRequest struct {
	Reason string `json:"reason"`
}

// handleRevokeAgentCertificate revokes an agent's certificate.
func (s *MasterServer) handleRevokeAgentCertificate(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx := r.Context()

	// Get revocation reason from request body
	var req RevokeRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
			s.jsonError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
	}

	// Get user from context for audit
	revokedBy := "system"
	if userID, ok := GetUserIDFromContext(ctx); ok && userID > 0 {
		user, err := s.userService.GetByID(ctx, userID)
		if err == nil {
			revokedBy = user.Username
		}
	}

	certService := security.NewCertificateService(s.store, s.logger)
	if err := certService.RevokeAgentCertificate(ctx, agentID, req.Reason, revokedBy); err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.jsonError(w, http.StatusNotFound, "Certificate not found")
			return
		}
		s.logger.Error("Failed to revoke certificate", zap.Error(err), zap.String("agent_id", agentID))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.logAudit(r, "revoke", "certificate", "Revoked certificate for agent: "+agentID, "success")
	s.jsonResponse(w, StatusResponse{Status: "revoked"})
}

// handleCAs handles /api/v1/certificates/cas endpoints.
func (s *MasterServer) handleCAs(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/certificates/cas")

	if path == "" || path == "/" {
		s.handleListCAs(w, r)
		return
	}

	if path == "/rotate" {
		s.handleRotateCA(w, r)
		return
	}

	s.jsonError(w, http.StatusNotFound, "Not found")
}

// handleListCAs lists all certificate authorities.
func (s *MasterServer) handleListCAs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx := r.Context()

	certService := security.NewCertificateService(s.store, s.logger)
	cas, err := certService.ListCAs(ctx)
	if err != nil {
		s.logger.Error("Failed to list CAs", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.jsonResponse(w, ListCountResponse{
		Items: cas,
		Count: len(cas),
	})
}

// handleRotateCA initiates CA rotation.
func (s *MasterServer) handleRotateCA(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx := r.Context()

	// Admin only
	if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	// CA rotation via CAManager
	if s.caManager == nil {
		s.jsonError(w, http.StatusServiceUnavailable, "CA manager not initialized")
		return
	}

	_, err := s.caManager.RotateCA(ctx, coreSecurity.DefaultCAConfig())
	if err != nil {
		s.logger.Error("Failed to rotate CA", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Failed to rotate CA: "+err.Error())
		return
	}

	s.logAudit(r, "rotate", "ca", "Rotated certificate authority", "success")
	s.jsonResponse(w, StatusResponse{Status: "rotated"})
}

// handleServerCertificate handles /api/v1/certificates/server endpoints.
func (s *MasterServer) handleServerCertificate(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/certificates/server")

	if path == "" || path == "/" {
		s.handleGetServerCertificate(w, r)
		return
	}

	if path == "/renew" {
		s.handleRenewServerCertificate(w, r)
		return
	}

	s.jsonError(w, http.StatusNotFound, "Not found")
}

// handleGetServerCertificate gets the server certificate.
func (s *MasterServer) handleGetServerCertificate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx := r.Context()

	certService := security.NewCertificateService(s.store, s.logger)
	certs, err := certService.ListServerCertificates(ctx)
	if err != nil {
		s.logger.Error("Failed to get server certificates", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.jsonResponse(w, ListCountResponse{
		Items: certs,
		Count: len(certs),
	})
}

// handleRenewServerCertificate renews the server certificate.
func (s *MasterServer) handleRenewServerCertificate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx := r.Context()

	// Admin only
	if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	// Server certificate renewal via CAManager
	if s.caManager == nil {
		s.jsonError(w, http.StatusServiceUnavailable, "CA manager not initialized")
		return
	}

	// Server certificate renewal is handled via ACME/Let's Encrypt typically
	// Manual renewal through the API is not yet implemented
	s.jsonError(w, http.StatusNotImplemented, "Server certificate renewal through API not yet implemented. Use ACME/Let's Encrypt or manual certificate management.")
}

// handleCertAudit handles /api/v1/certificates/audit endpoints.
func (s *MasterServer) handleCertAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx := r.Context()

	// Parse query parameters for filtering
	filter := storage.CertAuditFilter{}

	if agentID := r.URL.Query().Get("agent_id"); agentID != "" {
		filter.AgentID = agentID
	}
	if eventType := r.URL.Query().Get("event_type"); eventType != "" {
		filter.EventType = eventType
	}
	if since := r.URL.Query().Get("since"); since != "" {
		if t, err := time.Parse(time.RFC3339, since); err == nil {
			filter.Since = &t
		}
	}
	if until := r.URL.Query().Get("until"); until != "" {
		if t, err := time.Parse(time.RFC3339, until); err == nil {
			filter.Until = &t
		}
	}
	if limit := r.URL.Query().Get("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil && l > 0 {
			filter.Limit = l
		}
	}
	if offset := r.URL.Query().Get("offset"); offset != "" {
		if o, err := strconv.Atoi(offset); err == nil && o >= 0 {
			filter.Offset = o
		}
	}

	// Default limit
	if filter.Limit == 0 {
		filter.Limit = 100
	}

	certService := security.NewCertificateService(s.store, s.logger)
	events, err := certService.ListCertAuditEvents(ctx, filter)
	if err != nil {
		s.logger.Error("Failed to list cert audit events", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.jsonResponse(w, ListCountResponse{
		Items: events,
		Count: len(events),
	})
}
