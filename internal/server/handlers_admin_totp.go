package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"go.uber.org/zap"
)

// handleAdminTOTPUsers returns a list of users with TOTP enabled.
// GET /api/v1/admin/totp/users
func (s *MasterServer) handleAdminTOTPUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Require admin role
	user, ok := GetUserFromContext(r.Context())
	if !ok || user.Role != "admin" {
		s.jsonError(w, http.StatusForbidden, "Admin access required")
		return
	}

	ctx := r.Context()
	allUsers, err := s.userService.List(ctx)
	if err != nil {
		s.logger.Error("Failed to list users", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Failed to list users")
		return
	}

	// Filter to TOTP-enabled users
	type totpUser struct {
		ID          int64  `json:"id"`
		Username    string `json:"username"`
		Email       string `json:"email"`
		Role        string `json:"role"`
		TOTPEnabled bool   `json:"totpEnabled"`
	}

	var result []totpUser
	for _, u := range allUsers {
		if u.TOTPEnabled {
			result = append(result, totpUser{
				ID:          u.ID,
				Username:    u.Username,
				Email:       u.Email,
				Role:        u.Role,
				TOTPEnabled: true,
			})
		}
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"users": result,
	})
}

// handleAdminTOTPStatus returns the TOTP status for a user.
// GET /api/v1/admin/totp/status/{username}
func (s *MasterServer) handleAdminTOTPStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Require admin role
	admin, ok := GetUserFromContext(r.Context())
	if !ok || admin.Role != "admin" {
		s.jsonError(w, http.StatusForbidden, "Admin access required")
		return
	}

	// Extract username from path: /api/v1/admin/totp/status/{username}
	path := r.URL.Path
	prefix := "/api/v1/admin/totp/status/"
	if !strings.HasPrefix(path, prefix) {
		s.jsonError(w, http.StatusBadRequest, "Invalid path")
		return
	}
	username := strings.TrimPrefix(path, prefix)
	if username == "" {
		s.jsonError(w, http.StatusBadRequest, "Username required")
		return
	}

	ctx := r.Context()

	user, err := s.userService.GetByUsername(ctx, username)
	if err != nil {
		s.jsonError(w, http.StatusNotFound, "User not found")
		return
	}

	var recoveryCodesRemaining int
	if user.TOTPEnabled {
		remaining, err := s.store.CountUnusedRecoveryCodes(ctx, user.ID)
		if err != nil {
			s.logger.Warn("Failed to count recovery codes", zap.Error(err))
		} else {
			recoveryCodesRemaining = remaining
		}
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"user_id":                  user.ID,
		"username":                 user.Username,
		"email":                    user.Email,
		"role":                     user.Role,
		"totp_enabled":             user.TOTPEnabled,
		"recovery_codes_remaining": recoveryCodesRemaining,
	})
}

// handleAdminTOTPDisable disables TOTP for a user.
// POST /api/v1/admin/totp/disable
func (s *MasterServer) handleAdminTOTPDisable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Require admin role
	admin, ok := GetUserFromContext(r.Context())
	if !ok || admin.Role != "admin" {
		s.jsonError(w, http.StatusForbidden, "Admin access required")
		return
	}

	// Parse request
	var req struct {
		Username string `json:"username"`
		Reason   string `json:"reason"`
		TOTPCode string `json:"totp_code"` // Admin's TOTP for verification (optional)
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate inputs
	if req.Username == "" {
		s.jsonError(w, http.StatusBadRequest, "Username is required")
		return
	}
	if len(req.Reason) < 10 {
		s.jsonError(w, http.StatusBadRequest, "Reason must be at least 10 characters")
		return
	}

	ctx := r.Context()

	// Find target user
	targetUser, err := s.userService.GetByUsername(ctx, req.Username)
	if err != nil {
		s.jsonError(w, http.StatusNotFound, "User not found")
		return
	}

	// Prevent self-disable via API
	if targetUser.ID == admin.ID {
		s.jsonError(w, http.StatusForbidden, "Cannot disable your own TOTP via admin interface")
		return
	}

	// Check if target has TOTP enabled
	if !targetUser.TOTPEnabled {
		s.jsonError(w, http.StatusBadRequest, "User does not have TOTP enabled")
		return
	}

	// Verify admin's TOTP if admin has it enabled
	if admin.TOTPEnabled {
		if req.TOTPCode == "" {
			s.jsonError(w, http.StatusUnauthorized, "Admin TOTP verification required")
			return
		}
		if !security.ValidateTOTP(admin.TOTPSecret, req.TOTPCode, security.DefaultTOTPConfig()) {
			s.logAudit(r, "admin_totp_disable_failed", "security",
				"invalid admin TOTP for: "+req.Username, "failure")
			s.jsonError(w, http.StatusUnauthorized, "Invalid admin TOTP code")
			return
		}
	}

	// Disable TOTP
	if err := s.userService.SetTOTP(ctx, targetUser.ID, "", false); err != nil {
		s.logger.Error("Failed to disable TOTP", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Failed to disable TOTP")
		return
	}

	// Delete recovery codes
	if err := s.store.DeleteRecoveryCodes(ctx, targetUser.ID); err != nil {
		s.logger.Warn("Failed to delete recovery codes", zap.Error(err))
	}

	// Audit log
	s.logAudit(r, "admin_totp_disable", "security",
		"admin: "+admin.Username+", target: "+req.Username+", reason: "+req.Reason, "success")

	s.logger.Info("TOTP disabled by admin",
		zap.String("admin", admin.Username),
		zap.String("target_user", req.Username),
		zap.String("reason", req.Reason))

	s.writeJSON(w, http.StatusOK, map[string]string{
		"message": "TOTP disabled successfully",
	})
}
