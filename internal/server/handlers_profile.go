package server

import (
	"encoding/json"
	"net/http"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// handleProfileUI renders the user profile page.
func (s *MasterServer) handleProfileUI(w http.ResponseWriter, r *http.Request) {
	user, ok := GetUserFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	// Get recovery codes count if TOTP is enabled
	var recoveryCodesRemaining int
	if user.TOTPEnabled {
		count, err := s.store.CountUnusedRecoveryCodes(r.Context(), user.ID)
		if err != nil {
			s.logger.Warn("Failed to count recovery codes", zap.Error(err))
		} else {
			recoveryCodesRemaining = count
		}
	}

	data := s.withCommonData(r, map[string]interface{}{
		"Title":                  "Profile",
		"Active":                 "profile",
		"User":                   user,
		"RecoveryCodesRemaining": recoveryCodesRemaining,
	})
	s.renderTemplate(w, "profile", data)
}

// handleTOTPSetup generates a new TOTP secret for the user to set up.
// POST /api/v1/totp/setup
func (s *MasterServer) handleTOTPSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	user, ok := GetUserFromContext(r.Context())
	if !ok {
		s.jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Check if TOTP is already enabled
	if user.TOTPEnabled {
		s.jsonError(w, http.StatusBadRequest, "TOTP is already enabled")
		return
	}

	// Generate new TOTP secret
	secret, err := security.GenerateTOTPSecret()
	if err != nil {
		s.logger.Error("Failed to generate TOTP secret", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Failed to generate TOTP secret")
		return
	}

	// Generate URI for QR code
	config := security.DefaultTOTPConfig()
	uri := security.GenerateTOTPURI(secret, user.Username, config)

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"secret": secret,
		"uri":    uri,
	})
}

// handleTOTPEnable verifies and enables TOTP for the user.
// POST /api/v1/totp/enable
func (s *MasterServer) handleTOTPEnable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	user, ok := GetUserFromContext(r.Context())
	if !ok {
		s.jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Check if TOTP is already enabled
	if user.TOTPEnabled {
		s.jsonError(w, http.StatusBadRequest, "TOTP is already enabled")
		return
	}

	// Parse request
	var req struct {
		Secret   string `json:"secret"`
		TOTPCode string `json:"totp_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Secret == "" {
		s.jsonError(w, http.StatusBadRequest, "Secret is required")
		return
	}
	if req.TOTPCode == "" {
		s.jsonError(w, http.StatusBadRequest, "TOTP code is required")
		return
	}

	// Verify the TOTP code with the provided secret
	if !security.ValidateTOTP(req.Secret, req.TOTPCode, security.DefaultTOTPConfig()) {
		s.logAudit(r, "totp_enable_failed", "security",
			"user: "+user.Username+", reason: invalid code", "failure")
		s.jsonError(w, http.StatusUnauthorized, "Invalid TOTP code")
		return
	}

	ctx := r.Context()

	// Enable TOTP for the user
	if err := s.userService.SetTOTP(ctx, user.ID, req.Secret, true); err != nil {
		s.logger.Error("Failed to enable TOTP", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Failed to enable TOTP")
		return
	}

	// Generate recovery codes
	codes, hashes, err := security.GenerateRecoveryCodes()
	if err != nil {
		s.logger.Error("Failed to generate recovery codes", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Failed to generate recovery codes")
		return
	}

	// Save recovery codes
	recoveryCodes := make([]*storage.RecoveryCode, len(hashes))
	for i, hash := range hashes {
		recoveryCodes[i] = &storage.RecoveryCode{
			UserID:   user.ID,
			CodeHash: hash,
		}
	}
	if err := s.store.SaveRecoveryCodes(ctx, user.ID, recoveryCodes); err != nil {
		s.logger.Error("Failed to save recovery codes", zap.Error(err))
		// TOTP was enabled but codes failed - try to continue anyway
	}

	// Audit log
	s.logAudit(r, "totp_enabled", "security",
		"user: "+user.Username, "success")

	s.logger.Info("TOTP enabled for user",
		zap.String("user", user.Username),
		zap.Int("recovery_codes", len(codes)))

	// Return recovery codes (shown only once)
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":        "TOTP enabled successfully",
		"recovery_codes": security.FormatRecoveryCodes(codes),
	})
}

// handleTOTPDisable disables TOTP for the current user.
// POST /api/v1/totp/disable
func (s *MasterServer) handleTOTPDisable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	user, ok := GetUserFromContext(r.Context())
	if !ok {
		s.jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Check if TOTP is enabled
	if !user.TOTPEnabled {
		s.jsonError(w, http.StatusBadRequest, "TOTP is not enabled")
		return
	}

	// Parse request
	var req struct {
		TOTPCode string `json:"totp_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.TOTPCode == "" {
		s.jsonError(w, http.StatusBadRequest, "TOTP code is required")
		return
	}

	// Verify the TOTP code
	if !security.ValidateTOTP(user.TOTPSecret, req.TOTPCode, security.DefaultTOTPConfig()) {
		s.logAudit(r, "totp_disable_failed", "security",
			"user: "+user.Username+", reason: invalid code", "failure")
		s.jsonError(w, http.StatusUnauthorized, "Invalid TOTP code")
		return
	}

	ctx := r.Context()

	// Disable TOTP for the user
	if err := s.userService.SetTOTP(ctx, user.ID, "", false); err != nil {
		s.logger.Error("Failed to disable TOTP", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Failed to disable TOTP")
		return
	}

	// Delete recovery codes
	if err := s.store.DeleteRecoveryCodes(ctx, user.ID); err != nil {
		s.logger.Warn("Failed to delete recovery codes", zap.Error(err))
	}

	// Audit log
	s.logAudit(r, "totp_disabled", "security",
		"user: "+user.Username+" (self-service)", "success")

	s.logger.Info("TOTP disabled by user",
		zap.String("user", user.Username))

	s.writeJSON(w, http.StatusOK, map[string]string{
		"message": "TOTP disabled successfully",
	})
}
