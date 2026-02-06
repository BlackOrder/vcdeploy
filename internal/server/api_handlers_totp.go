// Package server provides the TOTP API handlers.
package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/BlackOrder/vcdeploy/internal/validation"
	"go.uber.org/zap"
)

// --- User TOTP Self-Service Handlers (consolidated under /users/me/totp) ---

// handleUserMe routes /api/v1/users/me/* requests.
// GET /users/me - returns current user info
// GET /users/me/totp - returns TOTP status
// POST /users/me/totp/setup - initiates TOTP setup
// PUT /users/me/totp - enables TOTP
// DELETE /users/me/totp - disables TOTP
// POST /users/me/totp/recovery - regenerates recovery codes
func (s *MasterServer) handleUserMe(w http.ResponseWriter, r *http.Request) {
	user, ok := GetUserFromContext(r.Context())
	if !ok {
		s.jsonError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/users/me")

	// Handle /users/me (user info)
	if path == "" || path == "/" {
		switch r.Method {
		case http.MethodGet:
			s.jsonResponse(w, UserResponse{
				ID:        user.ID,
				Username:  user.Username,
				Email:     user.Email,
				Role:      user.Role,
				CreatedAt: user.CreatedAt,
			})
		default:
			s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	// Handle /users/me/totp/*
	if strings.HasPrefix(path, "/totp") {
		totpPath := strings.TrimPrefix(path, "/totp")
		s.handleUserTOTP(w, r, user, totpPath)
		return
	}

	s.jsonError(w, http.StatusNotFound, "not found")
}

// handleUserTOTP handles TOTP operations for the current user.
func (s *MasterServer) handleUserTOTP(w http.ResponseWriter, r *http.Request, user *storage.User, path string) {
	ctx := r.Context()

	switch {
	case path == "" || path == "/":
		// GET /users/me/totp - status
		// PUT /users/me/totp - enable
		// DELETE /users/me/totp - disable
		switch r.Method {
		case http.MethodGet:
			s.handleUserTOTPStatus(w, r, user)
		case http.MethodPut:
			s.handleUserTOTPEnable(w, r, user)
		case http.MethodDelete:
			s.handleUserTOTPDisableForUser(w, r, user)
		default:
			s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		}

	case path == "/setup":
		// POST /users/me/totp/setup
		if r.Method != http.MethodPost {
			s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleUserTOTPSetup(w, r, user)

	case path == "/recovery":
		// POST /users/me/totp/recovery
		if r.Method != http.MethodPost {
			s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleUserTOTPRecovery(w, r, user)

	default:
		s.jsonError(w, http.StatusNotFound, "not found")
	}

	_ = ctx // Reserved for future use
}

// handleUserTOTPStatus returns TOTP status for the current user.
func (s *MasterServer) handleUserTOTPStatus(w http.ResponseWriter, r *http.Request, user *storage.User) {
	ctx := r.Context()

	// Get recovery codes count
	var recoveryCodesRemaining int
	if user.TOTPEnabled {
		codes, err := s.store.ListRecoveryCodes(ctx, user.ID)
		if err == nil {
			for _, code := range codes {
				if code.UsedAt == nil {
					recoveryCodesRemaining++
				}
			}
		}
	}

	s.jsonResponse(w, map[string]interface{}{
		"enabled":                  user.TOTPEnabled,
		"recovery_codes_remaining": recoveryCodesRemaining,
	})
}

// handleUserTOTPSetup initiates TOTP setup.
func (s *MasterServer) handleUserTOTPSetup(w http.ResponseWriter, r *http.Request, user *storage.User) {
	if user.TOTPEnabled {
		s.jsonError(w, http.StatusBadRequest, "TOTP is already enabled")
		return
	}

	secret, err := security.GenerateTOTPSecret()
	if err != nil {
		s.logger.Error("Failed to generate TOTP secret", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "failed to generate TOTP secret")
		return
	}

	// Generate URI for QR code
	config := security.DefaultTOTPConfig()
	uri := security.GenerateTOTPURI(secret, user.Username, config)

	// Store secret temporarily (will be confirmed on enable)
	ctx := r.Context()
	user.TOTPSecret = secret
	if err := s.userService.Update(ctx, user); err != nil {
		s.logger.Error("Failed to store TOTP secret", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "failed to store TOTP secret")
		return
	}

	s.jsonResponse(w, map[string]interface{}{
		"secret": secret,
		"uri":    uri,
	})
}

// handleUserTOTPEnable enables TOTP after verifying the code.
func (s *MasterServer) handleUserTOTPEnable(w http.ResponseWriter, r *http.Request, user *storage.User) {
	if user.TOTPEnabled {
		s.jsonError(w, http.StatusBadRequest, "TOTP is already enabled")
		return
	}

	if user.TOTPSecret == "" {
		s.jsonError(w, http.StatusBadRequest, "TOTP setup not initiated. Call POST /users/me/totp/setup first")
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.Code == "" {
		s.jsonError(w, http.StatusBadRequest, "code is required")
		return
	}

	// Verify the code
	if !security.ValidateTOTP(user.TOTPSecret, req.Code, security.DefaultTOTPConfig()) {
		s.jsonError(w, http.StatusBadRequest, "invalid TOTP code")
		return
	}

	// Enable TOTP
	ctx := r.Context()
	user.TOTPEnabled = true
	if err := s.userService.Update(ctx, user); err != nil {
		s.logger.Error("Failed to enable TOTP", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "failed to enable TOTP")
		return
	}

	// Generate recovery codes
	codes, hashes, err := security.GenerateRecoveryCodes()
	if err != nil {
		s.logger.Error("Failed to generate recovery codes", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "failed to generate recovery codes")
		return
	}

	// Store recovery codes (store hashes, return plaintext)
	recoveryCodes := make([]*storage.RecoveryCode, len(hashes))
	for i, hash := range hashes {
		recoveryCodes[i] = &storage.RecoveryCode{
			UserID:   user.ID,
			CodeHash: hash,
		}
	}
	if err := s.store.SaveRecoveryCodes(ctx, user.ID, recoveryCodes); err != nil {
		s.logger.Error("Failed to save recovery codes", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "failed to save recovery codes")
		return
	}

	s.logAudit(r, "enable", "totp", "TOTP enabled for user: "+user.Username, "success")

	s.jsonResponse(w, map[string]interface{}{
		"status":         "enabled",
		"recovery_codes": security.FormatRecoveryCodes(codes),
	})
}

// handleUserTOTPDisableForUser disables TOTP for the current user.
func (s *MasterServer) handleUserTOTPDisableForUser(w http.ResponseWriter, r *http.Request, user *storage.User) {
	if !user.TOTPEnabled {
		s.jsonError(w, http.StatusBadRequest, "TOTP is not enabled")
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.Code == "" {
		s.jsonError(w, http.StatusBadRequest, "code is required for verification")
		return
	}

	ctx := r.Context()

	// Verify the code (allow TOTP or recovery code)
	if !security.ValidateTOTP(user.TOTPSecret, req.Code, security.DefaultTOTPConfig()) {
		// Try as recovery code
		codes, err := s.store.ListRecoveryCodes(ctx, user.ID)
		if err != nil {
			s.logger.Error("Failed to get recovery codes", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "internal error")
			return
		}
		// Collect hashes for verification
		var hashes []string
		for _, code := range codes {
			if code.UsedAt == nil {
				hashes = append(hashes, code.CodeHash)
			}
		}
		if security.VerifyRecoveryCode(req.Code, hashes) < 0 {
			s.jsonError(w, http.StatusBadRequest, "invalid code")
			return
		}
	}

	// Disable TOTP
	user.TOTPEnabled = false
	user.TOTPSecret = ""
	if err := s.userService.Update(ctx, user); err != nil {
		s.logger.Error("Failed to disable TOTP", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "failed to disable TOTP")
		return
	}

	// Delete recovery codes
	if err := s.store.DeleteRecoveryCodes(ctx, user.ID); err != nil {
		s.logger.Warn("Failed to delete recovery codes", zap.Error(err))
	}

	s.logAudit(r, "disable", "totp", "TOTP disabled for user: "+user.Username, "success")

	s.jsonResponse(w, StatusResponse{Status: "disabled"})
}

// handleUserTOTPRecovery regenerates recovery codes for the current user.
func (s *MasterServer) handleUserTOTPRecovery(w http.ResponseWriter, r *http.Request, user *storage.User) {
	if !user.TOTPEnabled {
		s.jsonError(w, http.StatusBadRequest, "TOTP is not enabled")
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.Code == "" {
		s.jsonError(w, http.StatusBadRequest, "code is required for verification")
		return
	}

	// Verify the code
	if !security.ValidateTOTP(user.TOTPSecret, req.Code, security.DefaultTOTPConfig()) {
		s.jsonError(w, http.StatusBadRequest, "invalid TOTP code")
		return
	}

	// Generate new recovery codes
	codes, hashes, err := security.GenerateRecoveryCodes()
	if err != nil {
		s.logger.Error("Failed to generate recovery codes", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "failed to generate recovery codes")
		return
	}

	// Delete old codes and save new ones
	ctx := r.Context()
	if err := s.store.DeleteRecoveryCodes(ctx, user.ID); err != nil {
		s.logger.Warn("Failed to delete old recovery codes", zap.Error(err))
	}
	recoveryCodes := make([]*storage.RecoveryCode, len(hashes))
	for i, hash := range hashes {
		recoveryCodes[i] = &storage.RecoveryCode{
			UserID:   user.ID,
			CodeHash: hash,
		}
	}
	if err := s.store.SaveRecoveryCodes(ctx, user.ID, recoveryCodes); err != nil {
		s.logger.Error("Failed to save recovery codes", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "failed to save recovery codes")
		return
	}

	s.logAudit(r, "regenerate", "totp_recovery", "Recovery codes regenerated for user: "+user.Username, "success")

	s.jsonResponse(w, map[string]interface{}{
		"recovery_codes": security.FormatRecoveryCodes(codes),
	})
}

// --- Admin TOTP Handlers (consolidated under /users/{id}/totp) ---

// handleAdminUserTOTP handles admin TOTP operations for a specific user.
// GET /users/{id}/totp - get TOTP status for user
// DELETE /users/{id}/totp - disable TOTP for user (admin override)
func (s *MasterServer) handleAdminUserTOTP(w http.ResponseWriter, r *http.Request, userID int64) {
	// Require admin role
	admin, ok := GetUserFromContext(r.Context())
	if !ok || admin.Role != "admin" {
		s.jsonError(w, http.StatusForbidden, "admin access required")
		return
	}

	ctx := r.Context()

	// Get target user
	user, err := s.userService.GetByID(ctx, userID)
	if err != nil {
		s.jsonError(w, http.StatusNotFound, "user not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Return TOTP status
		var recoveryCodesRemaining int
		if user.TOTPEnabled {
			codes, err := s.store.ListRecoveryCodes(ctx, user.ID)
			if err == nil {
				for _, code := range codes {
					if code.UsedAt == nil {
						recoveryCodesRemaining++
					}
				}
			}
		}

		s.jsonResponse(w, map[string]interface{}{
			"user_id":                  user.ID,
			"username":                 user.Username,
			"enabled":                  user.TOTPEnabled,
			"recovery_codes_remaining": recoveryCodesRemaining,
		})

	case http.MethodDelete:
		// Admin force-disable TOTP (no code required)
		if !user.TOTPEnabled {
			s.jsonError(w, http.StatusBadRequest, "TOTP is not enabled for this user")
			return
		}

		user.TOTPEnabled = false
		user.TOTPSecret = ""
		if err := s.userService.Update(ctx, user); err != nil {
			s.logger.Error("Failed to disable TOTP", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "failed to disable TOTP")
			return
		}

		// Delete recovery codes
		if err := s.store.DeleteRecoveryCodes(ctx, user.ID); err != nil {
			s.logger.Warn("Failed to delete recovery codes", zap.Error(err))
		}

		s.logAudit(r, "admin_disable", "totp",
			"Admin "+admin.Username+" disabled TOTP for user: "+user.Username, "success")

		s.jsonResponse(w, StatusResponse{Status: "disabled"})

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
