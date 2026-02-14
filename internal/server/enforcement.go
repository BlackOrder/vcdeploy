// Package server provides enforcement middleware for security policies.
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// Role hierarchy: admin > user > viewer
// admin can do everything
// user can read and write (create/update/delete non-admin resources)
// viewer can only read

// RoleLevel returns the numeric level for a role (higher = more permissions).
func RoleLevel(role string) int {
	switch role {
	case "admin":
		return 3
	case "user":
		return 2
	case "viewer":
		return 1
	default:
		return 0
	}
}

// APIScope represents an API operation scope.
type APIScope string

const (
	// ScopeRead allows read-only operations.
	ScopeRead APIScope = "read"
	// ScopeWrite allows create/update/delete operations.
	ScopeWrite APIScope = "write"
	// ScopeAdmin allows all operations including user management.
	ScopeAdmin APIScope = "admin"
)

// Context key for API key object
const contextKeyAPIKey contextKey = "apiKey"

// WithAPIKeyContext adds the API key to the request context.
func WithAPIKeyContext(ctx context.Context, key *storage.APIKey) context.Context {
	return context.WithValue(ctx, contextKeyAPIKey, key)
}

// GetAPIKeyFromContext retrieves the API key from context.
func GetAPIKeyFromContext(ctx context.Context) (*storage.APIKey, bool) {
	key, ok := ctx.Value(contextKeyAPIKey).(*storage.APIKey)
	return key, ok
}

// parseScopes parses the scopes JSON from an API key.
func parseScopes(key *storage.APIKey) ([]string, error) {
	if key == nil || key.Scopes == "" {
		return nil, nil
	}
	var scopes []string
	if err := json.Unmarshal([]byte(key.Scopes), &scopes); err != nil {
		return nil, err
	}
	return scopes, nil
}

// hasScope checks if the API key has the required scope.
func hasScope(key *storage.APIKey, required APIScope) bool {
	scopes, err := parseScopes(key)
	if err != nil {
		return false
	}

	// Empty scopes means full access (for backward compatibility)
	if len(scopes) == 0 {
		return true
	}

	// Wildcard "*" scope grants all permissions
	if slices.Contains(scopes, "*") {
		return true
	}

	// admin scope implies all other scopes
	if slices.Contains(scopes, string(ScopeAdmin)) {
		return true
	}

	// write scope implies read scope
	if required == ScopeRead && slices.Contains(scopes, string(ScopeWrite)) {
		return true
	}

	return slices.Contains(scopes, string(required))
}

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
			WriteJSONError(w, http.StatusServiceUnavailable, "api is disabled")
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
			WriteJSONError(w, http.StatusServiceUnavailable, "api is disabled")
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
			m.logger.Error("Failed to get user for 2FA check", zap.String("userID", userID), zap.Error(err))
			WriteJSONError(w, http.StatusInternalServerError, "internal server error")
			return
		}

		// Check if user is admin and requires 2FA
		if user.Role == "admin" && !user.TOTPEnabled {
			m.logger.Warn("Admin user without 2FA attempted access",
				zap.String("username", user.Username),
				zap.String("path", r.URL.Path),
			)
			WriteJSONError(w, http.StatusForbidden, "2FA is required for admin users")
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
			m.logger.Error("Failed to get user for 2FA check", zap.String("userID", userID), zap.Error(err))
			WriteJSONError(w, http.StatusInternalServerError, "internal server error")
			return
		}

		// Check if user is admin and requires 2FA
		if user.Role == "admin" && !user.TOTPEnabled {
			m.logger.Warn("Admin user without 2FA attempted access",
				zap.String("username", user.Username),
				zap.String("path", r.URL.Path),
			)
			WriteJSONError(w, http.StatusForbidden, "2FA is required for admin users")
			return
		}

		next(w, r)
	}
}

// RequireRole returns middleware that requires an exact role.
// Use RequireMinRole for hierarchical role checks.
func (m *EnforcementMiddleware) RequireRole(role string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			userID, ok := GetUserIDFromContext(r.Context())
			if !ok {
				WriteJSONError(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			user, err := m.userService.GetByID(r.Context(), userID)
			if err != nil {
				m.logger.Error("Failed to get user for role check", zap.String("userID", userID), zap.Error(err))
				WriteJSONError(w, http.StatusInternalServerError, "internal server error")
				return
			}

			if user.Role != role {
				m.logger.Warn("User role mismatch",
					zap.String("username", user.Username),
					zap.String("required", role),
					zap.String("actual", user.Role),
					zap.String("path", r.URL.Path),
				)
				WriteJSONError(w, http.StatusForbidden, "forbidden: insufficient permissions")
				return
			}

			// Add user to context for downstream handlers
			ctx := WithUserContext(r.Context(), user)
			next(w, r.WithContext(ctx))
		}
	}
}

// RequireMinRole returns middleware that requires at least the specified role level.
// Role hierarchy: admin > user > viewer
func (m *EnforcementMiddleware) RequireMinRole(minRole string) func(http.HandlerFunc) http.HandlerFunc {
	minLevel := RoleLevel(minRole)
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			userID, ok := GetUserIDFromContext(r.Context())
			if !ok {
				WriteJSONError(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			user, err := m.userService.GetByID(r.Context(), userID)
			if err != nil {
				m.logger.Error("Failed to get user for role check", zap.String("userID", userID), zap.Error(err))
				WriteJSONError(w, http.StatusInternalServerError, "internal server error")
				return
			}

			userLevel := RoleLevel(user.Role)
			if userLevel < minLevel {
				m.logger.Warn("User role insufficient",
					zap.String("username", user.Username),
					zap.String("minRequired", minRole),
					zap.String("actual", user.Role),
					zap.String("path", r.URL.Path),
				)
				WriteJSONError(w, http.StatusForbidden, "forbidden: insufficient permissions")
				return
			}

			// Add user to context for downstream handlers
			ctx := WithUserContext(r.Context(), user)
			next(w, r.WithContext(ctx))
		}
	}
}

// RequireScope returns middleware that validates API key scope.
// This should be applied after authentication middleware has set the API key in context.
func (m *EnforcementMiddleware) RequireScope(scope APIScope) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Get API key from context (set by auth middleware)
			apiKey, ok := GetAPIKeyFromContext(r.Context())
			if !ok {
				// No API key in context - might be session auth, allow through
				// Role-based checks will handle authorization
				next(w, r)
				return
			}

			if !hasScope(apiKey, scope) {
				m.logger.Warn("API key scope insufficient",
					zap.String("keyPrefix", apiKey.KeyPrefix),
					zap.String("required", string(scope)),
					zap.String("path", r.URL.Path),
				)
				WriteJSONError(w, http.StatusForbidden, "forbidden: API key lacks required scope")
				return
			}

			next(w, r)
		}
	}
}

// RequireReadScope returns middleware requiring read scope.
func (m *EnforcementMiddleware) RequireReadScope(next http.HandlerFunc) http.HandlerFunc {
	return m.RequireScope(ScopeRead)(next)
}

// RequireWriteScope returns middleware requiring write scope.
func (m *EnforcementMiddleware) RequireWriteScope(next http.HandlerFunc) http.HandlerFunc {
	return m.RequireScope(ScopeWrite)(next)
}

// RequireAdminScope returns middleware requiring admin scope.
func (m *EnforcementMiddleware) RequireAdminScope(next http.HandlerFunc) http.HandlerFunc {
	return m.RequireScope(ScopeAdmin)(next)
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

// --- In-handler authorization helpers ---

// CheckMinRole checks if the user in context has at least the specified role.
// Returns an error message and HTTP status code if unauthorized.
func (m *EnforcementMiddleware) CheckMinRole(ctx context.Context, minRole string) (string, int, bool) {
	userID, ok := GetUserIDFromContext(ctx)
	if !ok {
		return "Unauthorized", http.StatusUnauthorized, false
	}

	user, err := m.userService.GetByID(ctx, userID)
	if err != nil {
		m.logger.Error("Failed to get user for role check", zap.String("userID", userID), zap.Error(err))
		return "Internal server error", http.StatusInternalServerError, false
	}

	if RoleLevel(user.Role) < RoleLevel(minRole) {
		m.logger.Warn("User role insufficient",
			zap.String("username", user.Username),
			zap.String("minRequired", minRole),
			zap.String("actual", user.Role),
		)
		return "Forbidden: insufficient permissions", http.StatusForbidden, false
	}

	return "", 0, true
}

// CheckScope checks if the API key in context has the required scope.
// Returns an error message and HTTP status code if unauthorized.
func (m *EnforcementMiddleware) CheckScope(ctx context.Context, scope APIScope) (string, int, bool) {
	apiKey, ok := GetAPIKeyFromContext(ctx)
	if !ok {
		// No API key in context - might be session auth, allow through
		return "", 0, true
	}

	if !hasScope(apiKey, scope) {
		m.logger.Warn("API key scope insufficient",
			zap.String("keyPrefix", apiKey.KeyPrefix),
			zap.String("required", string(scope)),
		)
		return "Forbidden: API key lacks required scope", http.StatusForbidden, false
	}

	return "", 0, true
}

// CheckWriteAccess checks if the request has write access (user role + API scope).
// Use for POST, PUT, PATCH, DELETE operations.
func (m *EnforcementMiddleware) CheckWriteAccess(ctx context.Context) (string, int, bool) {
	// Check API key scope
	if msg, status, ok := m.CheckScope(ctx, ScopeWrite); !ok {
		return msg, status, false
	}

	// Check user role (must be at least "user" to write)
	if msg, status, ok := m.CheckMinRole(ctx, "user"); !ok {
		return msg, status, false
	}

	return "", 0, true
}

// CheckAdminAccess checks if the request has admin access (admin role + admin scope).
// Use for user management and system configuration.
func (m *EnforcementMiddleware) CheckAdminAccess(ctx context.Context) (string, int, bool) {
	// Check API key scope
	if msg, status, ok := m.CheckScope(ctx, ScopeAdmin); !ok {
		return msg, status, false
	}

	// Check user role
	if msg, status, ok := m.CheckMinRole(ctx, "admin"); !ok {
		return msg, status, false
	}

	return "", 0, true
}

// CheckReadAccess checks if the request has read access (viewer role + read scope).
// Use for GET operations.
func (m *EnforcementMiddleware) CheckReadAccess(ctx context.Context) (string, int, bool) {
	// Check API key scope
	if msg, status, ok := m.CheckScope(ctx, ScopeRead); !ok {
		return msg, status, false
	}

	// Check user role (viewer can read)
	if msg, status, ok := m.CheckMinRole(ctx, "viewer"); !ok {
		return msg, status, false
	}

	return "", 0, true
}
