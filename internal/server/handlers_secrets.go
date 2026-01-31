// Package server provides secret management handlers for the master server.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services"
	"go.uber.org/zap"
)

// handleSecrets handles GET/POST for /api/v1/secrets and per-project secrets.
func (s *MasterServer) handleSecrets(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Parse query params for filtering
	projectFilter := r.URL.Query().Get("project")
	scopeFilter := r.URL.Query().Get("scope")

	switch r.Method {
	case http.MethodGet:
		// Read access: viewer role + read scope
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		if s.secretService == nil {
			s.jsonError(w, http.StatusInternalServerError, "Secret service not configured")
			return
		}

		var secretsList []services.SecretMetadata
		var err error

		if projectFilter != "" && scopeFilter != "" {
			// Get secrets for specific project/scope
			secretsList, err = s.secretService.List(ctx, projectFilter, scopeFilter)
		} else if projectFilter != "" {
			// Get all secrets for a project
			secretsList, err = s.secretService.ListByProject(ctx, projectFilter)
		} else {
			// Get all secrets (admin only)
			secretsList, err = s.secretService.ListAll(ctx)
		}

		if err != nil {
			s.logger.Error("Failed to list secrets", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Failed to list secrets")
			return
		}

		// Return metadata only (no values for security)
		type secretResponse struct {
			ID        int64     `json:"id"`
			Project   string    `json:"project"`
			Scope     string    `json:"scope"`
			Key       string    `json:"key"`
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
		}

		result := make([]secretResponse, 0, len(secretsList))
		for _, sec := range secretsList {
			result = append(result, secretResponse{
				ID:        sec.ID,
				Project:   sec.Project,
				Scope:     sec.Scope,
				Key:       sec.Key,
				CreatedAt: sec.CreatedAt,
				UpdatedAt: sec.UpdatedAt,
			})
		}
		s.jsonResponse(w, result)

	case http.MethodPost:
		// Write access: user role + write scope
		if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		// Create or update a secret - limit body size to 1MB
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var req struct {
			Project string `json:"project"`
			Scope   string `json:"scope"`
			Key     string `json:"key"`
			Value   string `json:"value"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.jsonError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		if req.Project == "" || req.Key == "" {
			s.jsonError(w, http.StatusBadRequest, "project and key are required")
			return
		}

		// Default scope to "default" if not provided
		if req.Scope == "" {
			req.Scope = "default"
		}

		// Use SecretService for encryption and storage
		if s.secretService == nil {
			s.jsonError(w, http.StatusInternalServerError, "Secret service not configured")
			return
		}

		if err := s.secretService.Set(ctx, req.Project, req.Scope, req.Key, req.Value); err != nil {
			s.logger.Error("Failed to store secret", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Failed to store secret")
			return
		}

		s.logAudit(r, "create", "secret", fmt.Sprintf("project=%s scope=%s key=%s", req.Project, req.Scope, req.Key), "success")

		w.WriteHeader(http.StatusCreated)
		s.jsonResponse(w, map[string]string{
			"status":  "created",
			"project": req.Project,
			"scope":   req.Scope,
			"key":     req.Key,
		})

	case http.MethodDelete:
		// Delete a secret
		project := r.URL.Query().Get("project")
		scope := r.URL.Query().Get("scope")
		key := r.URL.Query().Get("key")

		if project == "" || key == "" {
			s.jsonError(w, http.StatusBadRequest, "project and key are required")
			return
		}
		if scope == "" {
			scope = "default"
		}

		if s.secretService == nil {
			s.jsonError(w, http.StatusInternalServerError, "Secret service not configured")
			return
		}

		if err := s.secretService.Delete(ctx, project, scope, key); err != nil {
			s.logger.Error("Failed to delete secret", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Failed to delete secret")
			return
		}

		s.logAudit(r, "delete", "secret", fmt.Sprintf("project=%s scope=%s key=%s", project, scope, key), "success")

		s.jsonResponse(w, map[string]string{"status": "deleted"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
