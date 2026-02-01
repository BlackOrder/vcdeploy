// Package server provides API endpoint handlers.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/proto"
	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/BlackOrder/vcdeploy/internal/validation"
	"go.uber.org/zap"
)

// WriteJSONError writes a JSON error response to the ResponseWriter.
// This is a standalone helper for use by middleware that don't have access to *MasterServer.
// For handlers with MasterServer access, use s.jsonError() instead which includes logging.
func WriteJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// Best-effort encoding - if this fails, there's nothing more we can do
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error:   true,
		Message: message,
	})
}

// --- Stats API ---

// handleStats returns dashboard statistics.
func (s *MasterServer) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	// Gather statistics - log warnings on errors but continue with zero values
	projects, err := s.projectService.List(ctx)
	if err != nil {
		s.logger.Warn("failed to list projects for stats", zap.Error(err))
		projects = nil
	}
	agents, err := s.agentService.List(ctx)
	if err != nil {
		s.logger.Warn("failed to list agents for stats", zap.Error(err))
		agents = nil
	}
	deployments, err := s.deploymentService.ListRecent(ctx, 100)
	if err != nil {
		s.logger.Warn("failed to list deployments for stats", zap.Error(err))
		deployments = nil
	}

	// Count deployment stats
	var successCount, failedCount, runningCount int
	for _, d := range deployments {
		switch d.Status {
		case "success":
			successCount++
		case "failed":
			failedCount++
		case "running", "pending":
			runningCount++
		}
	}

	// Count connected agents
	var connectedAgents int
	for _, a := range agents {
		if a.Status == "connected" {
			connectedAgents++
		}
	}

	s.jsonResponse(w, map[string]interface{}{
		"projects": map[string]interface{}{
			"total": len(projects),
		},
		"agents": map[string]interface{}{
			"total":     len(agents),
			"connected": connectedAgents,
		},
		"deployments": map[string]interface{}{
			"success": successCount,
			"failed":  failedCount,
			"running": runningCount,
			"total":   len(deployments),
		},
		"timestamp": time.Now().UTC(),
	})
}

// --- Users API ---

// handleUsers handles user list and creation.
func (s *MasterServer) handleUsers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		// Admin-only: listing all users
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		// H6: Add pagination support for users endpoint
		p := parsePagination(r)
		result, err := s.userService.ListPaginated(ctx, p)
		if err != nil {
			s.logger.Error("Failed to list users", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		// Sanitize - remove password hashes
		users := make([]map[string]interface{}, 0, len(result.Items))
		for _, u := range result.Items {
			users = append(users, map[string]interface{}{
				"id":        u.ID,
				"username":  u.Username,
				"email":     u.Email,
				"role":      u.Role,
				"createdAt": u.CreatedAt,
			})
		}
		// Return paginated response with metadata
		s.jsonResponse(w, map[string]interface{}{
			"items":      users,
			"totalCount": result.TotalCount,
			"limit":      result.Pagination.Limit,
			"offset":     result.Pagination.Offset,
		})

	case http.MethodPost:
		// Admin-only: creating users
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		var req struct {
			Username    string `json:"username"`
			Email       string `json:"email"`
			Password    string `json:"password"`
			Role        string `json:"role"`
			TOTPEnabled bool   `json:"totpEnabled"`
			TOTPSecret  string `json:"totpSecret"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
			s.jsonError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		// Validate
		if req.Username == "" || req.Password == "" {
			s.jsonError(w, http.StatusBadRequest, "username and password required")
			return
		}
		if req.Email != "" {
			if err := services.ValidateEmail(req.Email); err != nil {
				s.jsonError(w, http.StatusBadRequest, "invalid email format")
				return
			}
		}
		if req.Role == "" {
			req.Role = "user"
		}

		// Validate role
		if err := services.ValidateRole(req.Role); err != nil {
			s.jsonError(w, http.StatusBadRequest, "role must be admin, user, or viewer")
			return
		}

		// Build create options
		var createOpts []services.CreateUserOption
		if req.TOTPEnabled && req.TOTPSecret != "" {
			createOpts = append(createOpts, services.WithTOTP(req.TOTPSecret))
		}

		// Create user through service (handles password validation and hashing)
		user, err := s.userService.Create(ctx, req.Username, req.Password, req.Email, req.Role, createOpts...)
		if err != nil {
			s.logger.Error("Failed to create user", zap.Error(err))
			// Check if it's a password validation error (should return 400)
			if strings.Contains(err.Error(), "password validation failed") {
				s.jsonError(w, http.StatusBadRequest, err.Error())
				return
			}
			// Otherwise it's likely a duplicate user (return 409)
			s.jsonError(w, http.StatusConflict, err.Error())
			return
		}

		s.logAudit(r, "create", "user", fmt.Sprintf("Created user: %s", req.Username), "success")

		w.WriteHeader(http.StatusCreated)
		s.jsonResponse(w, UserCreateResponse{
			ID:        user.ID,
			Username:  user.Username,
			Email:     user.Email,
			Role:      user.Role,
			CreatedAt: user.CreatedAt,
		})

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleUser handles individual user operations.
func (s *MasterServer) handleUser(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from path: /api/v1/users/{id}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
	if path == "" {
		s.jsonError(w, http.StatusBadRequest, "User ID required")
		return
	}

	userID, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		// Admin-only: viewing other user details
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		user, err := s.userService.GetByID(ctx, userID)
		if err != nil {
			if services.IsNotFound(err) {
				s.jsonError(w, http.StatusNotFound, "User not found")
				return
			}
			s.logger.Error("Failed to get user", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		s.jsonResponse(w, UserResponse{
			ID:        user.ID,
			Username:  user.Username,
			Email:     user.Email,
			Role:      user.Role,
			CreatedAt: user.CreatedAt,
		})

	case http.MethodPut:
		// Admin-only: updating users
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		var req struct {
			Email    string `json:"email"`
			Role     string `json:"role"`
			Password string `json:"password,omitempty"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
			s.jsonError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		user, err := s.userService.GetByID(ctx, userID)
		if err != nil {
			if services.IsNotFound(err) {
				s.jsonError(w, http.StatusNotFound, "User not found")
				return
			}
			s.logger.Error("Failed to get user", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		// Update fields
		if req.Email != "" {
			user.Email = req.Email
		}
		if req.Role != "" {
			user.Role = req.Role
		}

		// Handle password update via service
		if req.Password != "" {
			if err := s.userService.UpdatePassword(ctx, userID, req.Password); err != nil {
				s.logger.Error("Failed to update password", zap.Error(err))
				s.jsonError(w, http.StatusBadRequest, err.Error())
				return
			}
		}

		if err := s.userService.Update(ctx, user); err != nil {
			s.logger.Error("Failed to update user", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		s.logAudit(r, "update", "user", fmt.Sprintf("Updated user: %s", user.Username), "success")
		s.jsonResponse(w, StatusResponse{Status: "updated"})

	case http.MethodDelete:
		// Admin-only: deleting users
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		user, err := s.userService.GetByID(ctx, userID)
		if err != nil {
			if services.IsNotFound(err) {
				s.jsonError(w, http.StatusNotFound, "User not found")
				return
			}
			s.logger.Error("Failed to get user", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		if err := s.userService.Delete(ctx, userID); err != nil {
			s.logger.Error("Failed to delete user", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		// Log with snapshot - omit password hash for security
		userSnapshot := map[string]any{
			"id":       user.ID,
			"username": user.Username,
			"email":    user.Email,
			"role":     user.Role,
		}
		s.logAuditWithSnapshot(r, "delete", "user", fmt.Sprintf("%d", user.ID), userSnapshot, fmt.Sprintf("Deleted user: %s", user.Username), "success")
		s.jsonResponse(w, StatusResponse{Status: "deleted"})

	case http.MethodPatch:
		// Admin-only: partial updates (e.g., password change)
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		var req struct {
			Email    string `json:"email,omitempty"`
			Role     string `json:"role,omitempty"`
			Password string `json:"password,omitempty"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
			s.jsonError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		user, err := s.userService.GetByID(ctx, userID)
		if err != nil {
			if services.IsNotFound(err) {
				s.jsonError(w, http.StatusNotFound, "User not found")
				return
			}
			s.logger.Error("Failed to get user", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		// Update only provided fields
		updated := false
		if req.Email != "" {
			user.Email = req.Email
			updated = true
		}
		if req.Role != "" {
			user.Role = req.Role
			updated = true
		}

		// Handle password update via service
		if req.Password != "" {
			if err := s.userService.UpdatePassword(ctx, userID, req.Password); err != nil {
				s.logger.Error("Failed to update password", zap.Error(err))
				if strings.Contains(err.Error(), "password validation failed") {
					s.jsonError(w, http.StatusBadRequest, err.Error())
					return
				}
				s.jsonError(w, http.StatusInternalServerError, "Internal server error")
				return
			}
			updated = true
		}

		if updated && (req.Email != "" || req.Role != "") {
			if err := s.userService.Update(ctx, user); err != nil {
				s.logger.Error("Failed to update user", zap.Error(err))
				s.jsonError(w, http.StatusInternalServerError, "Internal server error")
				return
			}
		}

		s.logAudit(r, "update", "user", fmt.Sprintf("Updated user: %s", user.Username), "success")
		s.jsonResponse(w, StatusResponse{Status: "updated"})

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// --- Settings API ---

// handleSettingsCategory handles settings operations for a category.
func (s *MasterServer) handleSettingsCategory(w http.ResponseWriter, r *http.Request) {
	// Extract category from path: /api/v1/settings/{category}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/settings/")
	category := strings.Split(path, "/")[0]

	if category == "" {
		s.jsonError(w, http.StatusBadRequest, "Category required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		// Read access: viewer role + read scope
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		if s.settingsSvc == nil {
			s.jsonError(w, http.StatusInternalServerError, "Settings service not configured")
			return
		}

		settings, err := s.settingsSvc.ListByCategory(ctx, category)
		if err != nil {
			s.logger.Error("Failed to list settings", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		result := make(map[string]interface{})
		for _, setting := range settings {
			result[setting.Key] = setting.Value
		}
		s.jsonResponse(w, result)

	case http.MethodPut:
		// Admin-only: changing settings
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		if s.settingsSvc == nil {
			s.jsonError(w, http.StatusInternalServerError, "Settings service not configured")
			return
		}

		var req map[string]interface{}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
			s.jsonError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		for key, rawValue := range req {
			// Type coercion: convert non-string values to strings
			var value string
			switch v := rawValue.(type) {
			case string:
				value = v
			case bool:
				if v {
					value = "true"
				} else {
					value = "false"
				}
			case float64:
				// JSON numbers are float64; format as integer if whole number
				if v == float64(int64(v)) {
					value = strconv.FormatInt(int64(v), 10)
				} else {
					value = strconv.FormatFloat(v, 'f', -1, 64)
				}
			case nil:
				value = ""
			default:
				s.jsonError(w, http.StatusBadRequest, fmt.Sprintf("invalid type for setting %s", key))
				return
			}

			if err := s.settingsSvc.Set(ctx, category, key, value, false); err != nil {
				s.logger.Error("Failed to set setting", zap.String("key", key), zap.Error(err))
				s.jsonError(w, http.StatusInternalServerError, "Internal server error")
				return
			}
		}

		s.logAudit(r, "update", "settings", fmt.Sprintf("Updated settings category: %s", category), "success")
		s.jsonResponse(w, StatusResponse{Status: "updated"})

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleSettingsExport exports all settings as JSON.
func (s *MasterServer) handleSettingsExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	// Admin-only: exporting settings
	if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	if s.settingsSvc == nil {
		s.jsonError(w, http.StatusInternalServerError, "Settings service not configured")
		return
	}

	settings, err := s.settingsSvc.ListAll(ctx)
	if err != nil {
		s.logger.Error("Failed to list settings", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// Group by category
	result := make(map[string]map[string]interface{})
	for _, setting := range settings {
		if result[setting.Category] == nil {
			result[setting.Category] = make(map[string]interface{})
		}
		result[setting.Category][setting.Key] = map[string]interface{}{
			"value":     setting.Value,
			"type":      setting.ValueType,
			"encrypted": setting.Encrypted,
		}
	}

	w.Header().Set("Content-Disposition", "attachment; filename=settings.json")
	s.jsonResponse(w, result)
}

// handleSettingsImport imports settings from JSON.
func (s *MasterServer) handleSettingsImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutLong)
	defer cancel()

	// Admin-only: importing settings
	if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	if s.settingsSvc == nil {
		s.jsonError(w, http.StatusServiceUnavailable, "Settings service not configured")
		return
	}

	var req map[string]map[string]struct {
		Value     string `json:"value"`
		Type      string `json:"type"`
		Encrypted bool   `json:"encrypted"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 10<<20)).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	var count int
	for category, settings := range req {
		for key, setting := range settings {
			valueType := setting.Type
			if valueType == "" {
				valueType = "string"
			}
			if err := s.settingsSvc.SetRaw(ctx, category, key, setting.Value, valueType, setting.Encrypted); err != nil {
				s.logger.Error("Failed to import setting", zap.String("key", key), zap.Error(err))
				continue
			}
			count++
		}
	}

	s.logAudit(r, "import", "settings", fmt.Sprintf("Imported %d settings", count), "success")
	s.jsonResponse(w, SettingsImportResponse{
		Status:   "imported",
		Imported: count,
	})
}

// --- Projects API (enhanced) ---

// handleProjectsAPI handles project list and creation with full implementation.
func (s *MasterServer) handleProjectsAPI(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		// Read access: viewer role + read scope
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		p := parsePagination(r)
		result, err := s.projectService.ListPaginated(ctx, p)
		if err != nil {
			s.logger.Error("Failed to list projects", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		s.jsonResponse(w, map[string]interface{}{
			"items":      result.Items,
			"totalCount": result.TotalCount,
			"limit":      result.Pagination.Limit,
			"offset":     result.Pagination.Offset,
		})

	case http.MethodPost:
		// Write access: user role + write scope
		if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		var req struct {
			Name       string `json:"name"`
			Repository string `json:"repository"`
			Branch     string `json:"branch"`
			DeployPath string `json:"deployPath"`
			Type       string `json:"type"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
			s.jsonError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		if req.Name == "" {
			s.jsonError(w, http.StatusBadRequest, "name is required")
			return
		}
		if err := services.ValidateProjectName(req.Name); err != nil {
			s.jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		if req.Repository == "" {
			s.jsonError(w, http.StatusBadRequest, "repository is required")
			return
		}
		if req.DeployPath == "" {
			s.jsonError(w, http.StatusBadRequest, "deployPath is required")
			return
		}
		if req.Branch == "" {
			req.Branch = "main"
		}

		project, err := s.projectService.Create(ctx, req.Name, req.Repository, req.Branch, req.DeployPath, req.Type)
		if err != nil {
			s.logger.Error("Failed to create project", zap.Error(err))
			s.jsonError(w, http.StatusConflict, "project already exists")
			return
		}

		s.logAudit(r, "create", "project", fmt.Sprintf("Created project: %s", req.Name), "success")
		w.WriteHeader(http.StatusCreated)
		s.jsonResponse(w, project)

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleProjectAPI handles individual project operations.
func (s *MasterServer) handleProjectAPI(w http.ResponseWriter, r *http.Request) {
	// Extract project ID from path: /api/v1/projects/{id}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/projects/")
	parts := strings.Split(path, "/")
	projectIDStr := parts[0]

	if projectIDStr == "" {
		s.jsonError(w, http.StatusBadRequest, "Project ID required")
		return
	}

	projectID, err := strconv.ParseInt(projectIDStr, 10, 64)
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, "Invalid project ID: must be a number")
		return
	}

	// Check for sub-resources
	if len(parts) > 1 {
		switch parts[1] {
		case "webhooks":
			s.handleProjectWebhooksByID(w, r, projectID)
			return
		case "deploy":
			s.handleProjectDeployByID(w, r, projectID)
			return
		case "health-config":
			s.handleProjectHealthConfig(w, r, projectID)
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		// Read access: viewer role + read scope
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		project, err := s.projectService.GetByID(ctx, projectID)
		if err != nil {
			s.jsonError(w, http.StatusNotFound, "Project not found")
			return
		}
		s.jsonResponse(w, project)

	case http.MethodPut:
		// Write access: user role + write scope
		if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		var req struct {
			Name       string `json:"name"`
			Repository string `json:"repository"`
			Branch     string `json:"branch"`
			DeployPath string `json:"deployPath"`
			Type       string `json:"type"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
			s.jsonError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		project, err := s.projectService.GetByID(ctx, projectID)
		if err != nil {
			s.jsonError(w, http.StatusNotFound, "Project not found")
			return
		}

		// Update fields
		if req.Name != "" {
			project.Name = req.Name
		}
		if req.Repository != "" {
			project.Repository = req.Repository
		}
		if req.Branch != "" {
			project.Branch = req.Branch
		}
		if req.DeployPath != "" {
			project.DeployPath = req.DeployPath
		}
		if req.Type != "" {
			project.Type = req.Type
		}

		if err := s.projectService.UpdateByID(ctx, project); err != nil {
			s.logger.Error("Failed to update project", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		s.logAudit(r, "update", "project", fmt.Sprintf("Updated project: %d", projectID), "success")
		s.jsonResponse(w, project)

	case http.MethodDelete:
		// Write access: user role + write scope
		if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		// Fetch project before deletion to capture snapshot for audit
		project, err := s.projectService.GetByID(ctx, projectID)
		if err != nil {
			if services.IsNotFound(err) {
				s.jsonError(w, http.StatusNotFound, "Project not found")
				return
			}
			s.logger.Error("Failed to get project", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		if err := s.projectService.DeleteByID(ctx, projectID); err != nil {
			s.logger.Error("Failed to delete project", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		// Log with snapshot of deleted resource
		s.logAuditWithSnapshot(r, "delete", "project", fmt.Sprintf("%d", project.ID), project, fmt.Sprintf("Deleted project: %s (ID: %d)", project.Name, projectID), "success")
		s.jsonResponse(w, StatusResponse{Status: "deleted"})

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleProjectWebhooks handles webhook configuration for a project.
func (s *MasterServer) handleProjectWebhooks(w http.ResponseWriter, r *http.Request, projectName string) {
	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	project, err := s.projectService.GetByName(ctx, projectName)
	if err != nil {
		s.jsonError(w, http.StatusNotFound, "Project not found")
		return
	}

	s.handleProjectWebhooksInternal(ctx, w, r, project)
}

// handleProjectWebhooksByID handles webhook configuration for a project by ID.
func (s *MasterServer) handleProjectWebhooksByID(w http.ResponseWriter, r *http.Request, projectID int64) {
	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	project, err := s.projectService.GetByID(ctx, projectID)
	if err != nil {
		s.jsonError(w, http.StatusNotFound, "Project not found")
		return
	}

	s.handleProjectWebhooksInternal(ctx, w, r, project)
}

// handleProjectWebhooksInternal is the shared implementation for webhook handling.
func (s *MasterServer) handleProjectWebhooksInternal(ctx context.Context, w http.ResponseWriter, r *http.Request, project *storage.Project) {
	switch r.Method {
	case http.MethodGet:
		// Read access: viewer role + read scope
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		// Get all webhooks for this project
		webhooksList := make([]map[string]interface{}, 0)
		for _, provider := range []string{"github", "gitlab", "bitbucket"} {
			wh, err := s.webhookService.Get(ctx, project.ID, provider)
			if err == nil && wh != nil {
				webhooksList = append(webhooksList, map[string]interface{}{
					"provider": provider,
					"enabled":  wh.Enabled,
				})
			}
		}
		s.jsonResponse(w, webhooksList)

	case http.MethodPost:
		// Write access: user role + write scope
		if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		var req struct {
			Provider      string `json:"provider"`
			Secret        string `json:"secret"`
			Enabled       bool   `json:"enabled"`
			RequireSecret *bool  `json:"requireSecret"` // Pointer to detect if field was provided
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
			s.jsonError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		if req.Provider == "" || req.Secret == "" {
			s.jsonError(w, http.StatusBadRequest, "provider and secret are required")
			return
		}

		// Default requireSecret to true for security
		requireSecret := true
		if req.RequireSecret != nil {
			requireSecret = *req.RequireSecret
		}

		if err := s.webhookService.Set(ctx, project.ID, req.Provider, []byte(req.Secret), req.Enabled, requireSecret); err != nil {
			s.logger.Error("Failed to set webhook", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		s.logAudit(r, "create", "webhook", fmt.Sprintf("Configured %s webhook for project: %s", req.Provider, project.Name), "success")
		w.WriteHeader(http.StatusCreated)
		s.jsonResponse(w, StatusResponse{Status: "created"})

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleProjectDeploy triggers a deployment for a project.
func (s *MasterServer) handleProjectDeploy(w http.ResponseWriter, r *http.Request, projectName string) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	project, err := s.projectService.GetByName(ctx, projectName)
	if err != nil {
		s.jsonError(w, http.StatusNotFound, "Project not found")
		return
	}

	s.handleProjectDeployInternal(ctx, w, r, project)
}

// handleProjectDeployByID triggers a deployment for a project by ID.
func (s *MasterServer) handleProjectDeployByID(w http.ResponseWriter, r *http.Request, projectID int64) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	project, err := s.projectService.GetByID(ctx, projectID)
	if err != nil {
		s.jsonError(w, http.StatusNotFound, "Project not found")
		return
	}

	s.handleProjectDeployInternal(ctx, w, r, project)
}

// handleProjectDeployInternal is the shared implementation for deployment triggering.
func (s *MasterServer) handleProjectDeployInternal(ctx context.Context, w http.ResponseWriter, r *http.Request, project *storage.Project) {
	var req struct {
		Branch      string `json:"branch"`
		Target      string `json:"target"`
		ScheduledAt string `json:"scheduledAt,omitempty"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
		// Empty body is OK - use defaults
		req.Branch = project.Branch
		req.Target = "production"
	}

	if req.Branch == "" {
		req.Branch = project.Branch
	}
	if req.Target == "" {
		req.Target = "production"
	}

	// Validate target exists as a registered agent (skip for default "production" target)
	if req.Target != "production" && s.agentService != nil {
		if _, err := s.agentService.GetByID(ctx, req.Target); err != nil {
			if services.IsNotFound(err) {
				s.jsonError(w, http.StatusBadRequest, fmt.Sprintf("target agent %q not found", req.Target))
				return
			}
			s.logger.Error("Failed to validate target agent", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
	}

	// Get username from context
	username := "api"
	if userID, ok := GetUserIDFromContext(r.Context()); ok {
		if user, err := s.userService.GetByID(ctx, userID); err == nil && user != nil {
			username = user.Username
		}
	}

	deploymentID := fmt.Sprintf("deploy-%d", time.Now().UnixNano())

	// Check if scheduled
	if req.ScheduledAt != "" {
		scheduledTime, err := time.Parse(time.RFC3339, req.ScheduledAt)
		if err != nil {
			s.jsonError(w, http.StatusBadRequest, "invalid scheduled_at format (use RFC3339)")
			return
		}

		if err := s.deploymentService.CreateScheduled(ctx, deploymentID, project.Name, req.Target, req.Branch, scheduledTime, username); err != nil {
			s.logger.Error("Failed to create scheduled deployment", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		s.logAudit(r, "schedule", "deployment", fmt.Sprintf("Scheduled deployment for %s at %s", project.Name, scheduledTime), "success")
		w.WriteHeader(http.StatusAccepted)
		s.jsonResponse(w, ScheduledDeploymentResponse{
			ID:          deploymentID,
			Status:      "scheduled",
			ScheduledAt: scheduledTime,
		})
		return
	}

	// Create immediate deployment
	deployment := &storage.DeploymentRecord{
		ID:          deploymentID,
		Project:     project.Name,
		Target:      req.Target,
		Branch:      req.Branch,
		Status:      "pending",
		TriggeredBy: username,
		StartedAt:   time.Now(),
	}

	if err := s.deploymentService.Create(ctx, deployment); err != nil {
		s.logger.Error("Failed to create deployment", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.logAudit(r, "trigger", "deployment", fmt.Sprintf("Triggered deployment for %s", project.Name), "success")
	w.WriteHeader(http.StatusAccepted)
	s.jsonResponse(w, deployment)
}

// --- Agents API (enhanced) ---

// handleAgentsAPI handles agent list.
func (s *MasterServer) handleAgentsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	// Read access: viewer role + read scope
	if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	p := parsePagination(r)
	result, err := s.agentService.ListPaginated(ctx, p)
	if err != nil {
		s.logger.Error("Failed to list agents", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	s.jsonResponse(w, map[string]interface{}{
		"items":      result.Items,
		"totalCount": result.TotalCount,
		"limit":      result.Pagination.Limit,
		"offset":     result.Pagination.Offset,
	})
}

// handleAgentAPI handles individual agent operations.
func (s *MasterServer) handleAgentAPI(w http.ResponseWriter, r *http.Request) {
	// Extract agent ID from path: /api/v1/agents/{id} or /api/v1/agents/{id}/token
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/agents/")
	parts := strings.Split(path, "/")
	agentID := parts[0]

	// Handle special paths that don't require agent ID
	if agentID == "updates" && len(parts) > 1 && parts[1] == "pending" {
		s.handleAgentsNeedingUpdate(w, r)
		return
	}

	if agentID == "updates" && len(parts) > 1 && parts[1] == "history" {
		s.handleAllAgentUpdateHistory(w, r)
		return
	}

	if agentID == "" {
		s.jsonError(w, http.StatusBadRequest, "Agent ID required")
		return
	}

	// Handle token sub-resource: POST /api/v1/agents/{id}/token
	if len(parts) > 1 && parts[1] == "token" {
		s.handleAgentToken(w, r, agentID)
		return
	}

	// Handle update-config sub-resource
	if len(parts) > 1 && parts[1] == "update-config" {
		s.handleAgentUpdateConfig(w, r, agentID)
		return
	}

	// Handle update-history sub-resource
	if len(parts) > 1 && parts[1] == "update-history" {
		s.handleAgentUpdateHistory(w, r, agentID)
		return
	}

	// Handle update trigger: POST /api/v1/agents/{id}/update
	if len(parts) > 1 && parts[1] == "update" {
		s.handleTriggerAgentUpdate(w, r, agentID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		// Read access: viewer role + read scope
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		agent, err := s.agentService.GetByID(ctx, agentID)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				s.jsonError(w, http.StatusNotFound, "Agent not found")
				return
			}
			s.logger.Error("Failed to get agent", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		if agent == nil {
			s.jsonError(w, http.StatusNotFound, "Agent not found")
			return
		}
		s.jsonResponse(w, agent)

	case http.MethodPut:
		// Write access: user role + write scope
		if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		var req struct {
			Labels map[string]string `json:"labels"`
			Status string            `json:"status"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
			s.jsonError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		agent, err := s.agentService.GetByID(ctx, agentID)
		if err != nil || agent == nil {
			s.jsonError(w, http.StatusNotFound, "Agent not found")
			return
		}

		if req.Labels != nil {
			agent.Labels = req.Labels
		}
		if req.Status != "" {
			// Validate agent status
			validStatuses := map[string]bool{"online": true, "offline": true, "maintenance": true, "connected": true, "disconnected": true}
			if !validStatuses[req.Status] {
				s.jsonError(w, http.StatusBadRequest, "status must be one of: online, offline, maintenance, connected, disconnected")
				return
			}
			agent.Status = req.Status
		}

		if err := s.agentService.Upsert(ctx, agent); err != nil {
			s.logger.Error("Failed to update agent", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		s.logAudit(r, "update", "agent", fmt.Sprintf("Updated agent: %s", agentID), "success")
		s.jsonResponse(w, agent)

	case http.MethodDelete:
		// Admin-only: deleting agents
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		// Fetch agent before deletion for snapshot
		agent, err := s.agentService.GetByID(ctx, agentID)
		if err != nil {
			if services.IsNotFound(err) {
				s.jsonError(w, http.StatusNotFound, "Agent not found")
				return
			}
			s.logger.Error("Failed to get agent", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		if err := s.agentService.Delete(ctx, agentID); err != nil {
			s.logger.Error("Failed to delete agent", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		// Log with snapshot - omit certificate for security
		agentSnapshot := map[string]any{
			"id":         agent.ID,
			"hostname":   agent.Hostname,
			"labels":     agent.Labels,
			"status":     agent.Status,
			"version":    agent.Version,
			"os":         agent.OS,
			"arch":       agent.Arch,
			"lastSeenAt": agent.LastSeenAt,
		}
		s.logAuditWithSnapshot(r, "delete", "agent", agentID, agentSnapshot, fmt.Sprintf("Deleted agent: %s", agentID), "success")
		s.jsonResponse(w, StatusResponse{Status: "deleted"})

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleAgentToken handles POST /api/v1/agents/{id}/token to generate a registration token.
func (s *MasterServer) handleAgentToken(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Generate a secure random token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		s.logger.Error("Failed to generate token", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}
	token := hex.EncodeToString(tokenBytes)

	// Register the token with the agent server
	if s.agentServer != nil {
		s.agentServer.RegisterToken(agentID, token)
	}

	s.logAudit(r, "create", "agent_token", fmt.Sprintf("Generated token for agent: %s", agentID), "success")

	s.jsonResponse(w, AgentTokenResponse{
		AgentID: agentID,
		Token:   token,
		Expires: "30m", // Token expires after 30 minutes if not used
	})
}

// --- Deployments API (enhanced) ---

// handleDeploymentsAPI handles deployment list and creation.
func (s *MasterServer) handleDeploymentsAPI(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		// Read access: viewer role + read scope
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		// Parse pagination
		p := parsePagination(r)

		deployments, err := s.deploymentService.ListRecent(ctx, p.Limit)
		if err != nil {
			s.logger.Error("Failed to list deployments", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		s.jsonResponse(w, deployments)

	case http.MethodPost:
		// Write access: user role + write scope
		if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		var req struct {
			Project     string `json:"project"`
			Branch      string `json:"branch"`
			Target      string `json:"target"`
			ScheduledAt string `json:"scheduledAt,omitempty"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
			s.jsonError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		if req.Project == "" {
			s.jsonError(w, http.StatusBadRequest, "project is required")
			return
		}

		// Forward to project deploy handler
		s.handleProjectDeploy(w, r, req.Project)

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleDeploymentAPI handles individual deployment operations.
func (s *MasterServer) handleDeploymentAPI(w http.ResponseWriter, r *http.Request) {
	// Extract deployment ID and action from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/deployments/")
	parts := strings.Split(path, "/")
	deploymentID := parts[0]

	if deploymentID == "" {
		s.jsonError(w, http.StatusBadRequest, "Deployment ID required")
		return
	}

	// Check for actions
	if len(parts) > 1 {
		switch parts[1] {
		case "cancel":
			s.handleDeploymentCancel(w, r, deploymentID)
			return
		case "rollback":
			s.handleDeploymentRollback(w, r, deploymentID)
			return
		case "logs":
			// Check for streaming request
			if r.URL.Query().Get("stream") == "true" {
				s.handleDeploymentLogsStream(w, r, deploymentID)
				return
			}
			s.handleDeploymentLogs(w, r, deploymentID)
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		deployment, err := s.deploymentService.GetByID(ctx, deploymentID)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				s.jsonError(w, http.StatusNotFound, "Deployment not found")
				return
			}
			s.logger.Error("Failed to get deployment", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		if deployment == nil {
			s.jsonError(w, http.StatusNotFound, "Deployment not found")
			return
		}
		s.jsonResponse(w, deployment)

	case http.MethodDelete:
		// Cancel if running, otherwise just acknowledge
		deployment, err := s.deploymentService.GetByID(ctx, deploymentID)
		if err != nil || deployment == nil {
			s.jsonError(w, http.StatusNotFound, "Deployment not found")
			return
		}

		if deployment.Status == "scheduled" {
			if err := s.deploymentService.CancelScheduled(ctx, deploymentID); err != nil {
				s.logger.Error("Failed to cancel deployment", zap.Error(err))
				s.jsonError(w, http.StatusInternalServerError, "Internal server error")
				return
			}
		}

		s.logAudit(r, "cancel", "deployment", fmt.Sprintf("Cancelled deployment: %s", deploymentID), "success")
		s.jsonResponse(w, StatusResponse{Status: "cancelled"})

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleDeploymentCancel cancels a running deployment.
func (s *MasterServer) handleDeploymentCancel(w http.ResponseWriter, r *http.Request, deploymentID string) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	deployment, err := s.deploymentService.GetByID(ctx, deploymentID)
	if err != nil || deployment == nil {
		s.jsonError(w, http.StatusNotFound, "Deployment not found")
		return
	}

	if deployment.Status != "running" && deployment.Status != "pending" && deployment.Status != "scheduled" {
		s.jsonError(w, http.StatusBadRequest, "deployment cannot be cancelled (not running)")
		return
	}

	// Try to send cancel command to agent if deployment is running
	if deployment.Status == "running" && s.agentServer != nil {
		// Determine target agent
		agentID := deployment.Target
		if agentID == "" {
			// Try to find the agent handling this deployment
			connectedAgents := s.agentServer.GetConnectedAgents()
			if len(connectedAgents) > 0 {
				agentID = connectedAgents[0]
			}
		}

		if agentID != "" && s.agentServer.IsAgentConnected(agentID) {
			cancelCmd := &proto.CancelCommand{
				DeploymentId: deploymentID,
				Reason:       "Cancelled by user via API",
			}
			if err := s.agentServer.SendCancelCommand(agentID, cancelCmd); err != nil {
				s.logger.Warn("Failed to send cancel command to agent",
					zap.String("agent", agentID),
					zap.Error(err),
				)
			} else {
				s.logger.Info("Sent cancel command to agent",
					zap.String("deployment_id", deploymentID),
					zap.String("agent", agentID),
				)
			}
		}
	}

	// Update status
	now := time.Now()
	deployment.Status = "cancelled"
	deployment.CompletedAt = &now
	if err := s.deploymentService.Update(ctx, deployment); err != nil {
		s.logger.Error("Failed to cancel deployment", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.logAudit(r, "cancel", "deployment", fmt.Sprintf("Cancelled deployment: %s", deploymentID), "success")
	s.jsonResponse(w, StatusResponse{Status: "cancelled"})
}

// handleDeploymentRollback triggers a rollback for a deployment.
func (s *MasterServer) handleDeploymentRollback(w http.ResponseWriter, r *http.Request, deploymentID string) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	deployment, err := s.deploymentService.GetByID(ctx, deploymentID)
	if err != nil || deployment == nil {
		s.jsonError(w, http.StatusNotFound, "Deployment not found")
		return
	}

	// Get username from context
	username := "api"
	if userID, ok := GetUserIDFromContext(r.Context()); ok {
		if user, err := s.userService.GetByID(ctx, userID); err == nil && user != nil {
			username = user.Username
		}
	}

	// Create rollback deployment record
	rollbackID := fmt.Sprintf("rollback-%d", time.Now().UnixNano())
	rollback := &storage.DeploymentRecord{
		ID:            rollbackID,
		Project:       deployment.Project,
		Target:        deployment.Target,
		Branch:        deployment.Branch,
		Status:        "pending",
		TriggeredBy:   username,
		TriggerSource: "rollback:" + deploymentID,
		StartedAt:     time.Now(),
	}

	if err := s.deploymentService.Create(ctx, rollback); err != nil {
		s.logger.Error("Failed to create rollback", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// Try to send rollback command to agent
	if s.agentServer != nil {
		// Determine target agent
		agentID := deployment.Target
		if agentID == "" {
			connectedAgents := s.agentServer.GetConnectedAgents()
			if len(connectedAgents) > 0 {
				agentID = connectedAgents[0]
			}
		}

		if agentID != "" && s.agentServer.IsAgentConnected(agentID) {
			// Get project details for rollback
			project, err := s.projectService.GetByName(ctx, deployment.Project)
			if err == nil && project != nil {
				rollbackCmd := &proto.RollbackCommand{
					DeploymentId:  rollbackID,
					Project:       deployment.Project,
					Target:        deployment.Target,
					Path:          project.DeployPath,
					ReleaseNumber: 0, // 0 means previous release
				}

				// Update status to running
				rollback.Status = "running"
				if err := s.deploymentService.Update(ctx, rollback); err != nil {
					s.logger.Error("Failed to update deployment status to running", zap.Error(err))
				}

				if err := s.agentServer.SendRollbackCommand(agentID, rollbackCmd); err != nil {
					s.logger.Error("Failed to send rollback command to agent",
						zap.String("agent", agentID),
						zap.Error(err),
					)
					// Mark as failed
					rollback.Status = "failed"
					now := time.Now()
					rollback.CompletedAt = &now
					if err := s.deploymentService.Update(ctx, rollback); err != nil {
						s.logger.Error("Failed to update deployment status to failed", zap.Error(err))
					}
				} else {
					s.logger.Info("Sent rollback command to agent",
						zap.String("deployment_id", rollbackID),
						zap.String("agent", agentID),
					)
				}
			}
		} else {
			s.logger.Warn("No agent available for rollback",
				zap.String("deployment_id", rollbackID),
			)
		}
	}

	s.logAudit(r, "rollback", "deployment", fmt.Sprintf("Triggered rollback for: %s", deploymentID), "success")
	w.WriteHeader(http.StatusAccepted)
	s.jsonResponse(w, rollback)
}

// handleDeploymentLogs returns logs for a deployment.
func (s *MasterServer) handleDeploymentLogs(w http.ResponseWriter, r *http.Request, deploymentID string) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	logs, err := s.deploymentService.ListLogs(ctx, deploymentID)
	if err != nil {
		s.logger.Error("Failed to get deployment logs", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	s.jsonResponse(w, logs)
}

// handleDeploymentLogsStream streams deployment logs using Server-Sent Events (SSE).
// This allows real-time log streaming without WebSocket dependencies.
func (s *MasterServer) handleDeploymentLogsStream(w http.ResponseWriter, r *http.Request, deploymentID string) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.jsonError(w, http.StatusInternalServerError, "Streaming not supported")
		return
	}

	// Send initial logs
	ctx := r.Context()
	logs, err := s.deploymentService.ListLogs(ctx, deploymentID)
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return
	}

	// Send existing logs
	for _, log := range logs {
		logJSON, err := json.Marshal(log)
		if err != nil {
			s.logger.Error("Failed to marshal log", zap.Error(err))
			continue
		}
		fmt.Fprintf(w, "data: %s\n\n", logJSON)
	}
	flusher.Flush()

	// Track the last log ID we've seen
	lastID := int64(0)
	if len(logs) > 0 {
		lastID = logs[len(logs)-1].ID
	}

	// Poll for new logs until deployment completes or client disconnects
	// Max streaming duration prevents resource exhaustion from abandoned connections
	const maxStreamDuration = 30 * time.Minute
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	timeout := time.NewTimer(maxStreamDuration)
	defer timeout.Stop()

	for {
		select {
		case <-ctx.Done():
			// Client disconnected
			return
		case <-timeout.C:
			// Max streaming duration reached
			fmt.Fprintf(w, "event: timeout\ndata: {\"message\":\"Max streaming duration reached\"}\n\n")
			flusher.Flush()
			return
		case <-ticker.C:
			// Check for new logs
			newLogs, err := s.deploymentService.ListLogsAfter(ctx, deploymentID, lastID)
			if err != nil {
				s.logger.Error("Failed to poll logs", zap.Error(err))
				continue
			}

			for _, log := range newLogs {
				logJSON, err := json.Marshal(log)
				if err != nil {
					s.logger.Error("Failed to marshal log", zap.Error(err))
					continue
				}
				fmt.Fprintf(w, "data: %s\n\n", logJSON)
				lastID = log.ID
			}
			flusher.Flush()

			// Check if deployment is complete
			deployment, err := s.deploymentService.GetByID(ctx, deploymentID)
			if err != nil {
				continue
			}
			if deployment != nil && (deployment.Status == "success" || deployment.Status == "failed" || deployment.Status == "cancelled") {
				// Send completion event
				fmt.Fprintf(w, "event: complete\ndata: {\"status\":\"%s\"}\n\n", deployment.Status)
				flusher.Flush()
				return
			}
		}
	}
}

// --- API Keys API ---

// handleAPIKeys handles API key list and creation.
func (s *MasterServer) handleAPIKeys(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	// Get current user
	userID, ok := GetUserIDFromContext(r.Context())
	if !ok {
		s.jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Read access: viewer role + read scope (users can view their own keys)
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		keys, err := s.apiKeyService.List(ctx, userID)
		if err != nil {
			s.logger.Error("Failed to list API keys", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		// Sanitize - don't return the hash
		result := make([]map[string]interface{}, 0, len(keys))
		for _, k := range keys {
			result = append(result, map[string]interface{}{
				"id":         k.ID,
				"name":       k.Name,
				"createdAt":  k.CreatedAt,
				"expiresAt":  k.ExpiresAt,
				"lastUsedAt": k.LastUsedAt,
			})
		}
		s.jsonResponse(w, result)

	case http.MethodPost:
		// Write access: user role + write scope
		if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		var req struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Scopes      []string `json:"scopes"`
			ExpiresIn   int      `json:"expiresInDays"` // 0 = no expiry
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
			s.jsonError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		if req.Name == "" {
			s.jsonError(w, http.StatusBadRequest, "name is required")
			return
		}

		// Validate scopes before creating the key
		if err := services.ValidateAPIKeyScopes(req.Scopes); err != nil {
			s.jsonError(w, http.StatusBadRequest, err.Error())
			return
		}

		// Default to wildcard scope if not specified
		scopes := req.Scopes
		if len(scopes) == 0 {
			scopes = []string{"*"}
		}

		var expiresAt *time.Time
		if req.ExpiresIn > 0 {
			exp := time.Now().AddDate(0, 0, req.ExpiresIn)
			expiresAt = &exp
		}

		// Create API key using service (handles generation and hashing)
		rawKey, apiKey, err := s.apiKeyService.Create(ctx, userID, req.Name, scopes, expiresAt)
		if err != nil {
			s.logger.Error("Failed to create API key", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		s.logAudit(r, "create", "apikey", fmt.Sprintf("Created API key: %s", req.Name), "success")

		// Return the raw key (only time it's visible)
		w.WriteHeader(http.StatusCreated)
		s.jsonResponse(w, APIKeyCreateResponse{
			ID:        apiKey.ID,
			Name:      apiKey.Name,
			Key:       rawKey, // Only returned on creation!
			Scopes:    scopes,
			ExpiresAt: expiresAt,
			CreatedAt: apiKey.CreatedAt,
		})

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleAPIKey handles individual API key operations.
func (s *MasterServer) handleAPIKey(w http.ResponseWriter, r *http.Request) {
	// Extract key ID from path: /api/v1/api-keys/{id}
	// Also handles /api/v1/api-keys/{id}/revoke
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/api-keys/")

	// Check for /revoke suffix (POST to revoke is treated like DELETE)
	isRevoke := false
	if strings.HasSuffix(path, "/revoke") {
		path = strings.TrimSuffix(path, "/revoke")
		isRevoke = true
	}

	keyID, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, "Invalid key ID")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		// Get API key by ID
		key, err := s.apiKeyService.GetByID(ctx, keyID)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				s.jsonError(w, http.StatusNotFound, "API key not found")
				return
			}
			s.logger.Error("Failed to get API key", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		s.jsonResponse(w, key)

	case http.MethodPost:
		// POST to /revoke endpoint
		if !isRevoke {
			s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		// Fall through to delete/revoke the key
		fallthrough

	case http.MethodDelete:
		if err := s.apiKeyService.Delete(ctx, keyID); err != nil {
			s.logger.Error("Failed to revoke API key", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		s.logAudit(r, "revoke", "apikey", fmt.Sprintf("Revoked API key ID: %d", keyID), "success")
		s.jsonResponse(w, StatusResponse{Status: "revoked"})

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// --- Helper methods ---

// jsonError sends a JSON error response.
// H12 FIX: Properly handles encoder errors instead of ignoring them.
func (s *MasterServer) jsonError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// H14 FIX: Use the ErrorResponse type instead of inline map
	if err := json.NewEncoder(w).Encode(ErrorResponse{
		Error:   true,
		Message: message,
	}); err != nil {
		s.logger.Error("Failed to encode JSON error response",
			zap.Error(err),
			zap.String("message", message),
			zap.Int("status", status),
		)
	}
}
