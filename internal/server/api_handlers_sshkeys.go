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

// --- SSH Key API Handlers ---

// handleSSHKeys handles /api/v1/ssh-keys endpoints.
func (s *MasterServer) handleSSHKeys(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/ssh-keys")

	if path == "" || path == "/" {
		switch r.Method {
		case http.MethodGet:
			s.handleListSSHKeys(w, r)
		case http.MethodPost:
			s.handleGenerateSSHKey(w, r)
		default:
			s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
		return
	}

	// Handle /import endpoint
	if path == "/import" {
		s.handleImportSSHKey(w, r)
		return
	}

	// Extract key ID from path
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")

	// Parse key ID
	keyID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, "Invalid SSH key ID")
		return
	}

	if len(parts) == 1 {
		// /api/v1/ssh-keys/{id}
		switch r.Method {
		case http.MethodGet:
			s.handleGetSSHKey(w, r, keyID)
		case http.MethodDelete:
			s.handleDeleteSSHKey(w, r, keyID)
		default:
			s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
		return
	}

	if len(parts) == 2 && parts[1] == "public" {
		// /api/v1/ssh-keys/{id}/public
		s.handleGetSSHKeyPublic(w, r, keyID)
		return
	}

	s.jsonError(w, http.StatusNotFound, "Not found")
}

// handleListSSHKeys lists all SSH keys.
func (s *MasterServer) handleListSSHKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sshKeyService := security.NewSSHKeyService(s.store, s.kms, s.logger)
	keys, err := sshKeyService.ListSSHKeys(ctx)
	if err != nil {
		s.logger.Error("Failed to list SSH keys", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.jsonResponse(w, map[string]interface{}{
		"items": keys,
		"count": len(keys),
	})
}

// handleGetSSHKey gets a specific SSH key.
func (s *MasterServer) handleGetSSHKey(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()

	sshKeyService := security.NewSSHKeyService(s.store, s.kms, s.logger)
	key, err := sshKeyService.GetSSHKey(ctx, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || services.IsNotFound(err) {
			s.jsonError(w, http.StatusNotFound, "SSH key not found")
			return
		}
		s.logger.Error("Failed to get SSH key", zap.Error(err), zap.Int64("id", id))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.jsonResponse(w, key)
}

// handleGenerateSSHKey generates a new SSH key.
func (s *MasterServer) handleGenerateSSHKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req security.GenerateSSHKeyRequest
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

	sshKeyService := security.NewSSHKeyService(s.store, s.kms, s.logger)
	key, err := sshKeyService.GenerateSSHKey(ctx, req)
	if err != nil {
		if inputErr, ok := err.(*services.InputError); ok {
			s.jsonError(w, http.StatusBadRequest, inputErr.Message)
			return
		}
		s.logger.Error("Failed to generate SSH key", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.logAudit(r, "create", "ssh_key", "Generated SSH key: "+key.Name, "success")
	s.jsonResponse(w, key)
}

// handleImportSSHKey imports an existing SSH key.
func (s *MasterServer) handleImportSSHKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx := r.Context()

	var req security.ImportSSHKeyRequest
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

	sshKeyService := security.NewSSHKeyService(s.store, s.kms, s.logger)
	key, err := sshKeyService.ImportSSHKey(ctx, req)
	if err != nil {
		if inputErr, ok := err.(*services.InputError); ok {
			s.jsonError(w, http.StatusBadRequest, inputErr.Message)
			return
		}
		s.logger.Error("Failed to import SSH key", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.logAudit(r, "import", "ssh_key", "Imported SSH key: "+key.Name, "success")
	s.jsonResponse(w, key)
}

// handleDeleteSSHKey deletes an SSH key.
func (s *MasterServer) handleDeleteSSHKey(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()

	sshKeyService := security.NewSSHKeyService(s.store, s.kms, s.logger)
	if err := sshKeyService.DeleteSSHKey(ctx, id); err != nil {
		if strings.Contains(err.Error(), "not found") || services.IsNotFound(err) {
			s.jsonError(w, http.StatusNotFound, "SSH key not found")
			return
		}
		s.logger.Error("Failed to delete SSH key", zap.Error(err), zap.Int64("id", id))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.logAudit(r, "delete", "ssh_key", "Deleted SSH key ID: "+strconv.FormatInt(id, 10), "success")
	s.jsonResponse(w, StatusResponse{Status: "deleted"})
}

// handleGetSSHKeyPublic returns just the public key (for authorized_keys).
func (s *MasterServer) handleGetSSHKeyPublic(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx := r.Context()

	sshKeyService := security.NewSSHKeyService(s.store, s.kms, s.logger)
	publicKey, err := sshKeyService.GetPublicKey(ctx, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || services.IsNotFound(err) {
			s.jsonError(w, http.StatusNotFound, "SSH key not found")
			return
		}
		s.logger.Error("Failed to get SSH key public key", zap.Error(err), zap.Int64("id", id))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// Return as plain text (for easy copy-paste to authorized_keys)
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(publicKey))
}
