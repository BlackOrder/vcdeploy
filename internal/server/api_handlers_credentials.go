// Package server provides API endpoint handlers.
package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/services/security"
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
			s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
		return
	}

	// Extract credential ID from path
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")

	// Parse credential ID
	credID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, "Invalid credential ID")
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
			s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
		return
	}

	if len(parts) == 2 && parts[1] == "test" {
		// /api/v1/credentials/{id}/test
		s.handleTestCredential(w, r, credID)
		return
	}

	s.jsonError(w, http.StatusNotFound, "Not found")
}

// handleListCredentials lists all credentials.
func (s *MasterServer) handleListCredentials(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	credService := security.NewCredentialService(s.store, s.kms, s.logger)
	creds, err := credService.ListCredentials(ctx)
	if err != nil {
		s.logger.Error("Failed to list credentials", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.jsonResponse(w, map[string]interface{}{
		"items": creds,
		"count": len(creds),
	})
}

// handleGetCredential gets a specific credential.
func (s *MasterServer) handleGetCredential(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()

	credService := security.NewCredentialService(s.store, s.kms, s.logger)
	cred, err := credService.GetCredential(ctx, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || services.IsNotFound(err) {
			s.jsonError(w, http.StatusNotFound, "Credential not found")
			return
		}
		s.logger.Error("Failed to get credential", zap.Error(err), zap.Int64("id", id))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.jsonResponse(w, cred)
}

// handleCreateCredential creates a new credential.
func (s *MasterServer) handleCreateCredential(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req security.CreateCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Set creator from auth context
	if userID, ok := GetUserIDFromContext(ctx); ok && userID > 0 {
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
		if inputErr, ok := err.(*services.InputError); ok {
			s.jsonError(w, http.StatusBadRequest, inputErr.Message)
			return
		}
		s.logger.Error("Failed to create credential", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.logAudit(r, "create", "credential", "Created credential: "+cred.Name, "success")
	s.jsonResponse(w, cred)
}

// handleUpdateCredential updates an existing credential.
func (s *MasterServer) handleUpdateCredential(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()

	var req security.UpdateCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	credService := security.NewCredentialService(s.store, s.kms, s.logger)
	cred, err := credService.UpdateCredential(ctx, id, req)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || services.IsNotFound(err) {
			s.jsonError(w, http.StatusNotFound, "Credential not found")
			return
		}
		if inputErr, ok := err.(*services.InputError); ok {
			s.jsonError(w, http.StatusBadRequest, inputErr.Message)
			return
		}
		s.logger.Error("Failed to update credential", zap.Error(err), zap.Int64("id", id))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.logAudit(r, "update", "credential", "Updated credential: "+cred.Name, "success")
	s.jsonResponse(w, cred)
}

// handleDeleteCredential deletes a credential.
func (s *MasterServer) handleDeleteCredential(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()

	credService := security.NewCredentialService(s.store, s.kms, s.logger)
	if err := credService.DeleteCredential(ctx, id); err != nil {
		if strings.Contains(err.Error(), "not found") || services.IsNotFound(err) {
			s.jsonError(w, http.StatusNotFound, "Credential not found")
			return
		}
		s.logger.Error("Failed to delete credential", zap.Error(err), zap.Int64("id", id))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.logAudit(r, "delete", "credential", "Deleted credential ID: "+strconv.FormatInt(id, 10), "success")
	w.WriteHeader(http.StatusNoContent)
}

// TestCredentialRequestBody represents the request body for testing a credential.
type TestCredentialRequestBody struct {
	RepoURL string `json:"repo_url"`
}

// handleTestCredential tests a credential against a repo URL.
func (s *MasterServer) handleTestCredential(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx := r.Context()

	var req TestCredentialRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "Invalid request body")
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
			s.jsonError(w, http.StatusNotFound, "Credential not found")
			return
		}
		s.logger.Error("Failed to test credential", zap.Error(err), zap.Int64("id", id))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.jsonResponse(w, result)
}
