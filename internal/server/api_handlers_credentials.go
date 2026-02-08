// Package server provides API endpoint handlers.
package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/services/security"
	"github.com/BlackOrder/vcdeploy/internal/validation"
	"go.uber.org/zap"
)

// --- Credential API Handlers ---

// handleCredentials handles /api/v1/credentials endpoints.
func (s *MasterServer) handleCredentials(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/credentials")

	if path == "" || path == "/" {
		switch r.Method {
		case http.MethodGet:
			s.handleListCredentials(w, r)
		case http.MethodPost:
			s.handleCreateCredential(w, r)
		default:
			s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	// Extract credential ID from path
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")

	// Parse credential ID
	credID := parts[0]
	if credID == "" {
		s.jsonError(w, http.StatusBadRequest, "invalid credential ID")
		return
	}

	if len(parts) == 1 {
		// /api/v1/credentials/{id}
		switch r.Method {
		case http.MethodGet:
			s.handleGetCredential(w, r, credID)
		case http.MethodPut:
			s.handleUpdateCredential(w, r, credID)
		case http.MethodDelete:
			s.handleDeleteCredential(w, r, credID)
		default:
			s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if len(parts) == 2 && parts[1] == "test" {
		// /api/v1/credentials/{id}/test
		s.handleTestCredential(w, r, credID)
		return
	}

	s.jsonError(w, http.StatusNotFound, "not found")
}

// handleListCredentials lists all credentials.
func (s *MasterServer) handleListCredentials(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	credService := security.NewCredentialService(s.store, s.kms, s.logger)
	creds, err := credService.ListCredentials(ctx)
	if err != nil {
		s.logger.Error("Failed to list credentials", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Apply pagination
	p := parsePagination(r)
	totalCount := len(creds)

	// Apply offset and limit
	if p.Offset >= totalCount {
		creds = nil
	} else {
		creds = creds[p.Offset:]
		if p.Limit > 0 && p.Limit < len(creds) {
			creds = creds[:p.Limit]
		}
	}

	s.jsonResponse(w, PaginatedResponse{
		Items:      creds,
		TotalCount: int64(totalCount),
		Limit:      p.Limit,
		Offset:     p.Offset,
	})
}

// handleGetCredential gets a specific credential.
func (s *MasterServer) handleGetCredential(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()

	credService := security.NewCredentialService(s.store, s.kms, s.logger)
	cred, err := credService.GetCredential(ctx, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || services.IsNotFound(err) {
			s.jsonError(w, http.StatusNotFound, "credential not found")
			return
		}
		s.logger.Error("Failed to get credential", zap.Error(err), zap.String("id", id))
		s.jsonError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	s.jsonResponse(w, cred)
}

// handleCreateCredential creates a new credential.
func (s *MasterServer) handleCreateCredential(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req security.CreateCredentialRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Set creator from auth context
	if userID, ok := GetUserIDFromContext(ctx); ok && userID != "" {
		user, err := s.userService.GetByID(ctx, userID)
		if err == nil {
			req.CreatedBy = user.Username
		}
	}
	if req.CreatedBy == "" {
		req.CreatedBy = "system"
	}

	credService := security.NewCredentialService(s.store, s.kms, s.logger)
	cred, err := credService.CreateCredential(ctx, req)
	if err != nil {
		var inputErr *services.InputError
		if errors.As(err, &inputErr) {
			s.jsonError(w, http.StatusBadRequest, inputErr.Message)
			return
		}
		s.logger.Error("Failed to create credential", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	s.logAudit(r, "create", "credential", "Created credential: "+cred.Name, "success")
	s.writeJSON(w, http.StatusCreated, cred)
}

// handleUpdateCredential updates an existing credential.
func (s *MasterServer) handleUpdateCredential(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()

	var req security.UpdateCredentialRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	credService := security.NewCredentialService(s.store, s.kms, s.logger)
	cred, err := credService.UpdateCredential(ctx, id, req)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || services.IsNotFound(err) {
			s.jsonError(w, http.StatusNotFound, "credential not found")
			return
		}
		var inputErr *services.InputError
		if errors.As(err, &inputErr) {
			s.jsonError(w, http.StatusBadRequest, inputErr.Message)
			return
		}
		s.logger.Error("Failed to update credential", zap.Error(err), zap.String("id", id))
		s.jsonError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	s.logAudit(r, "update", "credential", "Updated credential: "+cred.Name, "success")
	s.jsonResponse(w, cred)
}

// handleDeleteCredential deletes a credential.
func (s *MasterServer) handleDeleteCredential(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()

	credService := security.NewCredentialService(s.store, s.kms, s.logger)
	if err := credService.DeleteCredential(ctx, id); err != nil {
		if strings.Contains(err.Error(), "not found") || services.IsNotFound(err) {
			s.jsonError(w, http.StatusNotFound, "credential not found")
			return
		}
		s.logger.Error("Failed to delete credential", zap.Error(err), zap.String("id", id))
		s.jsonError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	s.logAudit(r, "delete", "credential", "Deleted credential ID: "+id, "success")
	w.WriteHeader(http.StatusNoContent)
}

// TestCredentialRequestBody represents the request body for testing a credential.
type TestCredentialRequestBody struct {
	RepoURL string `json:"repo_url"`
}

// handleTestCredential tests a credential against a repo URL.
func (s *MasterServer) handleTestCredential(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := r.Context()

	var req TestCredentialRequestBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.RepoURL == "" {
		s.jsonError(w, http.StatusBadRequest, "repo_url is required")
		return
	}

	credService := security.NewCredentialService(s.store, s.kms, s.logger)
	result, err := credService.TestCredential(ctx, id, req.RepoURL)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || services.IsNotFound(err) {
			s.jsonError(w, http.StatusNotFound, "credential not found")
			return
		}
		s.logger.Error("Failed to test credential", zap.Error(err), zap.String("id", id))
		s.jsonError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	s.jsonResponse(w, result)
}
