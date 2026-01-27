// Package server provides enforcement middleware for security policies.
package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// EnforcementMiddleware provides policy enforcement for HTTP handlers.
type EnforcementMiddleware struct {
	config      *config.MasterConfig
	userService services.UserServicer
	logger      *zap.Logger
}

// NewEnforcementMiddleware creates a new enforcement middleware.
func NewEnforcementMiddleware(cfg *config.MasterConfig, userSvc services.UserServicer, logger *zap.Logger) *EnforcementMiddleware {
	return &EnforcementMiddleware{
		config:      cfg,
		userService: userSvc,
		logger:      logger,
	}
}

// RequireAPIEnabled returns middleware that rejects requests when API is disabled.
func (m *EnforcementMiddleware) RequireAPIEnabled(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.config.API.Enabled {
			m.logger.Debug("API request rejected: API is disabled",
				zap.String("path", r.URL.Path),
				zap.String("method", r.Method),
			)
			http.Error(w, "API is disabled", http.StatusServiceUnavailable)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAPIEnabledFunc returns a HandlerFunc wrapper that rejects requests when API is disabled.
func (m *EnforcementMiddleware) RequireAPIEnabledFunc(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !m.config.API.Enabled {
			m.logger.Debug("API request rejected: API is disabled",
				zap.String("path", r.URL.Path),
				zap.String("method", r.Method),
			)
			http.Error(w, "API is disabled", http.StatusServiceUnavailable)
			return
		}
		next(w, r)
	}
}

// Require2FAForAdmin returns middleware that enforces 2FA for admin users.
// This should be applied after authentication middleware has set the user ID in context.
func (m *EnforcementMiddleware) Require2FAForAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip if 2FA for admin is not required
		if !m.config.Security.Require2FAAdmin {
			next.ServeHTTP(w, r)
			return
		}

		// Get user ID from context
		userID, ok := GetUserIDFromContext(r.Context())
		if !ok {
			// No user in context, let downstream handler deal with auth
			next.ServeHTTP(w, r)
			return
		}

		// Get user details
		user, err := m.userService.GetByID(r.Context(), userID)
		if err != nil {
			m.logger.Error("Failed to get user for 2FA check", zap.Int64("userID", userID), zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Check if user is admin and requires 2FA
		if user.Role == "admin" && !user.TOTPEnabled {
			m.logger.Warn("Admin user without 2FA attempted access",
				zap.String("username", user.Username),
				zap.String("path", r.URL.Path),
			)
			http.Error(w, "2FA is required for admin users", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Require2FAForAdminFunc returns a HandlerFunc wrapper that enforces 2FA for admin users.
func (m *EnforcementMiddleware) Require2FAForAdminFunc(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Skip if 2FA for admin is not required
		if !m.config.Security.Require2FAAdmin {
			next(w, r)
			return
		}

		// Get user ID from context
		userID, ok := GetUserIDFromContext(r.Context())
		if !ok {
			// No user in context, let downstream handler deal with auth
			next(w, r)
			return
		}

		// Get user details
		user, err := m.userService.GetByID(r.Context(), userID)
		if err != nil {
			m.logger.Error("Failed to get user for 2FA check", zap.Int64("userID", userID), zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Check if user is admin and requires 2FA
		if user.Role == "admin" && !user.TOTPEnabled {
			m.logger.Warn("Admin user without 2FA attempted access",
				zap.String("username", user.Username),
				zap.String("path", r.URL.Path),
			)
			http.Error(w, "2FA is required for admin users", http.StatusForbidden)
			return
		}

		next(w, r)
	}
}

// LogSizeEnforcer enforces deployment log size limits.
type LogSizeEnforcer struct {
	maxSizeBytes int64
	logger       *zap.Logger
}

// NewLogSizeEnforcer creates a new log size enforcer.
func NewLogSizeEnforcer(maxSizeMB int, logger *zap.Logger) *LogSizeEnforcer {
	maxBytes := int64(maxSizeMB) * 1024 * 1024
	if maxBytes <= 0 {
		maxBytes = 100 * 1024 * 1024 // Default 100MB
	}
	return &LogSizeEnforcer{
		maxSizeBytes: maxBytes,
		logger:       logger,
	}
}

// CheckSize returns true if the current size is within limits.
func (e *LogSizeEnforcer) CheckSize(currentSize int64) bool {
	return currentSize < e.maxSizeBytes
}

// MaxSize returns the maximum allowed size in bytes.
func (e *LogSizeEnforcer) MaxSize() int64 {
	return e.maxSizeBytes
}

// TruncateLog truncates the log content to fit within the size limit.
// Returns the truncated content and true if truncation occurred.
func (e *LogSizeEnforcer) TruncateLog(content string) (string, bool) {
	contentBytes := int64(len(content))
	if contentBytes <= e.maxSizeBytes {
		return content, false
	}

	// Truncate to max size with a message
	truncateMarker := "\n\n... [LOG TRUNCATED - SIZE LIMIT EXCEEDED] ...\n"
	markerLen := int64(len(truncateMarker))
	maxContentLen := e.maxSizeBytes - markerLen

	if maxContentLen <= 0 {
		return truncateMarker, true
	}

	// Find a good break point (newline) near the truncation point
	truncatedContent := content[:maxContentLen]
	if lastNewline := strings.LastIndex(truncatedContent, "\n"); lastNewline > 0 {
		truncatedContent = truncatedContent[:lastNewline]
	}

	return truncatedContent + truncateMarker, true
}

// --- Helper middleware combiners ---

// ChainMiddleware chains multiple middleware functions.
func ChainMiddleware(middlewares ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(final http.Handler) http.Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			final = middlewares[i](final)
		}
		return final
	}
}

// WithUserContext adds user information to the request context for use by middleware.
func WithUserContext(ctx context.Context, user *storage.User) context.Context {
	return context.WithValue(ctx, contextKeyUser, user)
}

// GetUserFromContext retrieves the full user object from context.
func GetUserFromContext(ctx context.Context) (*storage.User, bool) {
	user, ok := ctx.Value(contextKeyUser).(*storage.User)
	return user, ok
}

// Context key for full user object
const contextKeyUser contextKey = "user"
