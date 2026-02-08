// Package server provides authentication handlers for the master server.
package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// setupRequiredMiddleware redirects to /setup when system requires initial configuration.
// Allows: /setup, /static/*, /favicon.ico, health endpoints
func (s *MasterServer) setupRequiredMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.requiresSetup {
			next.ServeHTTP(w, r)
			return
		}

		// Allow these paths even during setup
		path := r.URL.Path
		if path == "/setup" ||
			strings.HasPrefix(path, "/static/") ||
			path == "/favicon.ico" ||
			path == "/healthz" ||
			path == "/livez" ||
			path == "/readyz" ||
			path == "/api/v1/health" {
			next.ServeHTTP(w, r)
			return
		}

		// Redirect everything else to setup
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
	})
}

// handleSetup handles the first-run setup wizard.
func (s *MasterServer) handleSetup(w http.ResponseWriter, r *http.Request) {
	// If setup not required, redirect to dashboard or login
	if !s.requiresSetup {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if r.Method == http.MethodGet {
		s.renderTemplate(w, "setup", nil)
		return
	}

	if r.Method == http.MethodPost {
		username := strings.TrimSpace(r.FormValue("username"))
		email := strings.TrimSpace(r.FormValue("email"))
		password := r.FormValue("password")
		confirmPassword := r.FormValue("confirm_password")

		// Validation
		if username == "" {
			s.renderTemplate(w, "setup", map[string]interface{}{"Error": "Username is required"})
			return
		}
		if email == "" {
			s.renderTemplate(w, "setup", map[string]interface{}{"Error": "Email is required"})
			return
		}
		if password == "" {
			s.renderTemplate(w, "setup", map[string]interface{}{"Error": "Password is required"})
			return
		}
		if password != confirmPassword {
			s.renderTemplate(w, "setup", map[string]interface{}{"Error": "Passwords do not match"})
			return
		}

		// Create admin user
		ctx := r.Context()
		user, err := s.userService.Create(ctx, username, password, email, "admin")
		if err != nil {
			s.logger.Error("Failed to create admin user during setup", zap.Error(err))
			s.renderTemplate(w, "setup", map[string]interface{}{"Error": fmt.Sprintf("Failed to create user: %v", err)})
			return
		}

		// Log audit event
		s.logAudit(r, "setup", "user", fmt.Sprintf("Initial admin user '%s' created via setup wizard", username), "success")

		s.logger.Info("Initial admin user created via setup wizard",
			zap.String("username", user.Username),
			zap.String("email", user.Email))

		// Mark setup as complete
		s.requiresSetup = false

		// Create session and redirect to dashboard
		session, err := s.sessionService.Create(ctx, user.ID, extractClientIP(r), r.UserAgent(), 7*24*time.Hour)
		if err != nil {
			s.logger.Error("Failed to create session after setup", zap.Error(err))
			// Still redirect to login - user can log in manually
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    session.ID,
			Path:     "/",
			HttpOnly: true,
			Secure:   s.config.Server.TLS.Enabled,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   86400 * 7, // 7 days
		})

		http.Redirect(w, r, "/stats", http.StatusSeeOther)
		return
	}

	s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
}

// handleIndex redirects the root path to the stats page.
func (s *MasterServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/stats", http.StatusSeeOther)
}

// handleAPILogin handles JSON-based authentication for API clients.
// POST /api/v1/auth/login
// Request: {"username": "...", "password": "...", "totp": "..."}
// Response: {"token": "session_id", "user": {...}}
func (s *MasterServer) handleAPILogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		TOTP     string `json:"totp,omitempty"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.Username == "" || req.Password == "" {
		s.jsonError(w, http.StatusBadRequest, "username and password required")
		return
	}

	ctx := r.Context()

	// Look up user
	user, err := s.userService.GetByUsername(ctx, req.Username)
	if err != nil {
		if services.IsNotFound(err) {
			s.logger.Debug("API login failed: user not found", zap.String("username", req.Username))
			s.logAudit(r, "api_login", "session", "user not found: "+req.Username, "failure")
			s.jsonError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		s.logger.Error("Database error during API login", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Verify password
	if !verifyPassword(user.PasswordHash, req.Password) {
		s.logger.Debug("API login failed: invalid password", zap.String("username", req.Username))
		s.logAudit(r, "api_login", "session", "invalid password for: "+req.Username, "failure")
		s.jsonError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// Check if user must change password
	if user.MustChangePassword {
		s.logger.Debug("API login blocked: password change required", zap.String("username", req.Username))
		s.logAudit(r, "api_login", "session", "password change required for: "+req.Username, "blocked")
		s.jsonError(w, http.StatusForbidden, "password change required. Please login via web UI to change your password.")
		return
	}

	// Verify TOTP if enabled
	if user.TOTPEnabled {
		if req.TOTP == "" {
			s.jsonError(w, http.StatusUnauthorized, "TOTP required")
			return
		}

		// First try as regular TOTP code
		if !verifyTOTP(user.TOTPSecret, req.TOTP) {
			// Try as recovery code
			codes, err := s.store.ListRecoveryCodes(ctx, user.ID)
			if err != nil {
				s.logger.Error("Failed to get recovery codes", zap.Error(err))
				s.jsonError(w, http.StatusInternalServerError, "internal error")
				return
			}

			// Build slice of hashes for verification
			hashes := make([]string, len(codes))
			var codeID string
			normalizedCode := security.NormalizeRecoveryCode(req.TOTP)
			for i, code := range codes {
				if code.UsedAt == nil {
					hashes[i] = code.CodeHash
					// Check if this code matches
					if bcrypt.CompareHashAndPassword([]byte(code.CodeHash), []byte(normalizedCode)) == nil {
						codeID = code.ID
					}
				}
			}

			if codeID == "" {
				s.logger.Debug("API login failed: invalid TOTP", zap.String("username", req.Username))
				s.logAudit(r, "api_login", "session", "invalid TOTP for: "+req.Username, "failure")
				s.jsonError(w, http.StatusUnauthorized, "invalid verification code")
				return
			}

			// Mark recovery code as used
			if err := s.store.UseRecoveryCode(ctx, codeID); err != nil {
				s.logger.Error("Failed to mark recovery code as used", zap.Error(err))
			}

			// Count remaining codes
			remaining, _ := s.store.CountUnusedRecoveryCodes(ctx, user.ID)
			s.logger.Info("Recovery code used for login",
				zap.String("username", req.Username),
				zap.Int("remaining_codes", remaining))
			s.logAudit(r, "recovery_code_used", "auth",
				fmt.Sprintf("user: %s, remaining: %d", req.Username, remaining), "success")
		}
	}

	// Create session using service
	session, err := s.sessionService.Create(ctx, user.ID, extractClientIP(r), r.UserAgent(), 7*24*time.Hour)
	if err != nil {
		s.logger.Error("Failed to create session", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	// Log successful login
	s.logAudit(r, "api_login", "session", fmt.Sprintf("user: %s, IP: %s", req.Username, session.IPAddress), "success")

	s.logger.Info("User logged in via API",
		zap.String("username", req.Username),
		zap.String("ip", session.IPAddress))

	s.jsonResponse(w, LoginResponse{
		Token: session.ID,
		User: UserInfoResponse{
			ID:       user.ID,
			Username: user.Username,
			Email:    user.Email,
			Role:     user.Role,
		},
	})
}

// handleAPICurrentUser handles GET /api/v1/auth/me
// Returns the currently authenticated user's information.
func (s *MasterServer) handleAPICurrentUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Get the user from context (set by withAuth middleware)
	userID, ok := GetUserIDFromContext(r.Context())
	if !ok || userID == "" {
		s.jsonError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	ctx := r.Context()
	user, err := s.userService.GetByID(ctx, userID)
	if err != nil {
		if services.IsNotFound(err) {
			s.jsonError(w, http.StatusNotFound, "user not found")
			return
		}
		s.logger.Error("Failed to get user", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	s.jsonResponse(w, UserInfoResponse{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Role:     user.Role,
	})
}

// handleLogin handles the web UI login page.
func (s *MasterServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		username := r.FormValue("username")
		password := r.FormValue("password")
		totp := r.FormValue("totp")
		s.logger.Debug("Login attempt", zap.String("username", username))

		ctx := r.Context()

		// Look up user
		user, err := s.userService.GetByUsername(ctx, username)
		if err != nil {
			if services.IsNotFound(err) {
				s.logger.Debug("Login failed: user not found", zap.String("username", username))
				s.logAudit(r, "login", "session", "user not found: "+username, "failure")
				s.renderTemplate(w, "login", map[string]interface{}{"Error": "Invalid credentials"})
				return
			}
			s.logger.Error("Database error during login", zap.Error(err))
			s.renderTemplate(w, "login", map[string]interface{}{"Error": "Internal error"})
			return
		}

		// Verify password
		if !verifyPassword(user.PasswordHash, password) {
			s.logger.Debug("Login failed: invalid password", zap.String("username", username))
			s.logAudit(r, "login", "session", "invalid password for: "+username, "failure")
			s.renderTemplate(w, "login", map[string]interface{}{"Error": "Invalid credentials"})
			return
		}

		// Verify TOTP if enabled
		if user.TOTPEnabled {
			if totp == "" {
				s.renderTemplate(w, "login", map[string]interface{}{
					"Username":  username,
					"NeedsTOTP": true,
				})
				return
			}

			// First try as regular TOTP code
			if !verifyTOTP(user.TOTPSecret, totp) {
				// Try as recovery code
				codes, err := s.store.ListRecoveryCodes(ctx, user.ID)
				if err != nil {
					s.logger.Error("Failed to get recovery codes", zap.Error(err))
					s.renderTemplate(w, "login", map[string]interface{}{"Error": "Internal error"})
					return
				}

				// Check if the code matches any unused recovery code
				var codeID string
				normalizedCode := security.NormalizeRecoveryCode(totp)
				for _, code := range codes {
					if code.UsedAt == nil {
						if bcrypt.CompareHashAndPassword([]byte(code.CodeHash), []byte(normalizedCode)) == nil {
							codeID = code.ID
							break
						}
					}
				}

				if codeID == "" {
					s.logger.Debug("Login failed: invalid TOTP", zap.String("username", username))
					s.logAudit(r, "login", "session", "invalid TOTP for: "+username, "failure")
					s.renderTemplate(w, "login", map[string]interface{}{
						"Error":     "Invalid verification code",
						"Username":  username,
						"NeedsTOTP": true,
					})
					return
				}

				// Mark recovery code as used
				if err := s.store.UseRecoveryCode(ctx, codeID); err != nil {
					s.logger.Error("Failed to mark recovery code as used", zap.Error(err))
				}

				// Count remaining codes and log
				remaining, _ := s.store.CountUnusedRecoveryCodes(ctx, user.ID)
				s.logger.Info("Recovery code used for login",
					zap.String("username", username),
					zap.Int("remaining_codes", remaining))
				s.logAudit(r, "recovery_code_used", "auth",
					fmt.Sprintf("user: %s, remaining: %d", username, remaining), "success")
			}
		}

		// Create session using service (even if password change required, we need a session)
		session, err := s.sessionService.Create(ctx, user.ID, extractClientIP(r), r.UserAgent(), 7*24*time.Hour)
		if err != nil {
			s.logger.Error("Failed to create session", zap.Error(err))
			s.renderTemplate(w, "login", map[string]interface{}{"Error": "Internal error"})
			return
		}

		// Set session cookie first (needed for change-password page)
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    session.ID,
			Path:     "/",
			HttpOnly: true,
			Secure:   s.config.Server.TLS.Enabled,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   86400 * 7, // 7 days
		})

		// Check if user must change password
		if user.MustChangePassword {
			s.logger.Info("User must change password", zap.String("username", username))
			s.logAudit(r, "login", "session", fmt.Sprintf("user: %s, password change required", username), "success")
			http.Redirect(w, r, "/change-password", http.StatusSeeOther)
			return
		}

		// Log successful login
		s.logAudit(r, "login", "session", fmt.Sprintf("user: %s, IP: %s", username, session.IPAddress), "success")

		s.logger.Info("User logged in",
			zap.String("username", username),
			zap.String("ip", session.IPAddress))

		http.Redirect(w, r, "/stats", http.StatusSeeOther)
		return
	}
	s.renderTemplate(w, "login", nil)
}

// handleLogout handles user logout.
func (s *MasterServer) handleLogout(w http.ResponseWriter, r *http.Request) {
	// Delete session from database
	if cookie, err := r.Cookie("session"); err == nil && cookie.Value != "" {
		ctx := r.Context()

		// Get user ID for audit log (if session exists) before deleting
		if session, err := s.sessionService.GetByToken(ctx, cookie.Value); err == nil {
			if auditErr := s.auditService.Log(ctx, &storage.AuditEntry{
				Source:    "web",
				User:      fmt.Sprintf("user:%s", session.UserID),
				Action:    "logout",
				Resource:  "session",
				Result:    "success",
				Timestamp: time.Now(),
			}); auditErr != nil {
				s.logger.Error("Failed to log audit entry for logout", zap.Error(auditErr), zap.String("userID", session.UserID))
			}
		}

		if err := s.sessionService.Delete(ctx, cookie.Value); err != nil {
			s.logger.Debug("Failed to delete session", zap.Error(err))
		}
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// handleChangePassword handles the password change page for users who must change their password.
func (s *MasterServer) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user, ok := GetUserFromContext(r.Context())
	if !ok || user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if r.Method == http.MethodPost {
		currentPassword := r.FormValue("current_password")
		newPassword := r.FormValue("new_password")
		confirmPassword := r.FormValue("confirm_password")

		// Validate current password
		if !verifyPassword(user.PasswordHash, currentPassword) {
			s.renderTemplate(w, "change-password", map[string]interface{}{
				"Error":              "Current password is incorrect",
				"MustChangePassword": user.MustChangePassword,
			})
			return
		}

		// Validate new password matches confirmation
		if newPassword != confirmPassword {
			s.renderTemplate(w, "change-password", map[string]interface{}{
				"Error":              "New passwords do not match",
				"MustChangePassword": user.MustChangePassword,
			})
			return
		}

		// Validate new password is different from current
		if currentPassword == newPassword {
			s.renderTemplate(w, "change-password", map[string]interface{}{
				"Error":              "New password must be different from current password",
				"MustChangePassword": user.MustChangePassword,
			})
			return
		}

		// Update password using service (this clears MustChangePassword flag)
		ctx := r.Context()
		if err := s.userService.UpdatePassword(ctx, user.ID, newPassword); err != nil {
			s.logger.Error("Failed to update password", zap.Error(err))
			s.renderTemplate(w, "change-password", map[string]interface{}{
				"Error":              "Failed to update password: " + err.Error(),
				"MustChangePassword": user.MustChangePassword,
			})
			return
		}

		// Invalidate all existing sessions for this user (security best practice)
		if err := s.sessionService.DeleteAllForUser(ctx, user.ID); err != nil {
			s.logger.Error("Failed to invalidate sessions after password change",
				zap.String("user_id", user.ID),
				zap.Error(err))
			// Continue - password was changed successfully, session cleanup is best-effort
		}

		// Log password change
		s.logAudit(r, "password_change", "user", fmt.Sprintf("user: %s", user.Username), "success")

		s.logger.Info("User changed password",
			zap.String("username", user.Username),
			zap.Bool("was_forced", user.MustChangePassword))

		// Redirect to stats
		http.Redirect(w, r, "/stats", http.StatusSeeOther)
		return
	}

	// GET request - show the change password form
	s.renderTemplate(w, "change-password", map[string]interface{}{
		"MustChangePassword": user.MustChangePassword,
	})
}

// --- Auth Helper Functions ---

// verifyPassword verifies a password against a bcrypt hash.
func verifyPassword(hash, password string) bool {
	if hash == "" || password == "" {
		return false
	}
	// Use bcrypt for password verification
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// verifyTOTP verifies a TOTP code against a secret.
func verifyTOTP(secret, code string) bool {
	if secret == "" || code == "" {
		return false
	}
	return security.ValidateTOTP(secret, code, security.DefaultTOTPConfig())
}

// extractClientIP extracts the client IP from a request, handling proxies.
func extractClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (first IP in chain is the client)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
