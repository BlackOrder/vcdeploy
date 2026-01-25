// Package server provides API endpoint handlers.
package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/proto"
	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/BlackOrder/vcdeploy/internal/validation"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// --- Stats API ---

// handleStats returns dashboard statistics.
func (s *MasterServer) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Gather statistics - log warnings on errors but continue with zero values
	projects, err := s.db.ListProjects()
	if err != nil {
		s.logger.Warn("failed to list projects for stats", zap.Error(err))
		projects = nil
	}
	agents, err := s.db.ListAgents(ctx)
	if err != nil {
		s.logger.Warn("failed to list agents for stats", zap.Error(err))
		agents = nil
	}
	deployments, err := s.db.ListDeploymentsRecent(ctx, 100)
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
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		users, err := s.db.ListUsers(ctx)
		if err != nil {
			s.logger.Error("Failed to list users", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Sanitize - remove password hashes
		result := make([]map[string]interface{}, 0, len(users))
		for _, u := range users {
			result = append(result, map[string]interface{}{
				"id":        u.ID,
				"username":  u.Username,
				"email":     u.Email,
				"role":      u.Role,
				"createdAt": u.CreatedAt,
			})
		}
		s.jsonResponse(w, result)

	case http.MethodPost:
		var req struct {
			Username string `json:"username"`
			Email    string `json:"email"`
			Password string `json:"password"`
			Role     string `json:"role"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Validate
		if req.Username == "" || req.Password == "" {
			s.jsonError(w, http.StatusBadRequest, "username and password required")
			return
		}
		if req.Role == "" {
			req.Role = "user"
		}

		// Validate password complexity
		if err := security.ValidatePassword(req.Password); err != nil {
			s.jsonError(w, http.StatusBadRequest, err.Error())
			return
		}

		// Hash password
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			s.logger.Error("Failed to hash password", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		user := &storage.User{
			Username:     req.Username,
			Email:        req.Email,
			PasswordHash: string(hash),
			Role:         req.Role,
			CreatedAt:    time.Now(),
		}

		if err := s.db.CreateUser(ctx, user); err != nil {
			s.logger.Error("Failed to create user", zap.Error(err))
			s.jsonError(w, http.StatusConflict, "user already exists or database error")
			return
		}

		s.logAudit(r, "create", "user", fmt.Sprintf("Created user: %s", req.Username), "success")

		s.jsonResponse(w, map[string]interface{}{
			"id":       user.ID,
			"username": user.Username,
			"email":    user.Email,
			"role":     user.Role,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleUser handles individual user operations.
func (s *MasterServer) handleUser(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from path: /api/v1/users/{id}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
	if path == "" {
		http.Error(w, "User ID required", http.StatusBadRequest)
		return
	}

	userID, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		user, err := s.db.GetUserByID(ctx, userID)
		if errors.Is(err, storage.ErrNotFound) {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		if err != nil {
			s.logger.Error("Failed to get user", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		s.jsonResponse(w, map[string]interface{}{
			"id":        user.ID,
			"username":  user.Username,
			"email":     user.Email,
			"role":      user.Role,
			"createdAt": user.CreatedAt,
		})

	case http.MethodPut:
		var req struct {
			Email    string `json:"email"`
			Role     string `json:"role"`
			Password string `json:"password,omitempty"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		user, err := s.db.GetUserByID(ctx, userID)
		if errors.Is(err, storage.ErrNotFound) {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		if err != nil {
			s.logger.Error("Failed to get user", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Update fields
		if req.Email != "" {
			user.Email = req.Email
		}
		if req.Role != "" {
			user.Role = req.Role
		}
		if req.Password != "" {
			// Validate password complexity
			if err := security.ValidatePassword(req.Password); err != nil {
				s.jsonError(w, http.StatusBadRequest, err.Error())
				return
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			user.PasswordHash = string(hash)
		}

		if err := s.db.UpdateUserByID(ctx, user); err != nil {
			s.logger.Error("Failed to update user", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		s.logAudit(r, "update", "user", fmt.Sprintf("Updated user: %s", user.Username), "success")
		s.jsonResponse(w, map[string]string{"status": "updated"})

	case http.MethodDelete:
		user, err := s.db.GetUserByID(ctx, userID)
		if errors.Is(err, storage.ErrNotFound) {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		if err != nil {
			s.logger.Error("Failed to get user", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if err := s.db.DeleteUser(ctx, userID); err != nil {
			s.logger.Error("Failed to delete user", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		s.logAudit(r, "delete", "user", fmt.Sprintf("Deleted user: %s", user.Username), "success")
		s.jsonResponse(w, map[string]string{"status": "deleted"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- Settings API ---

// handleSettingsCategory handles settings operations for a category.
func (s *MasterServer) handleSettingsCategory(w http.ResponseWriter, r *http.Request) {
	// Extract category from path: /api/v1/settings/{category}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/settings/")
	category := strings.Split(path, "/")[0]

	if category == "" {
		http.Error(w, "Category required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		settings, err := s.db.ListSettingsByCategory(ctx, category)
		if err != nil {
			s.logger.Error("Failed to list settings", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		result := make(map[string]interface{})
		for _, setting := range settings {
			result[setting.Key] = setting.Value
		}
		s.jsonResponse(w, result)

	case http.MethodPut:
		var req map[string]string
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		for key, value := range req {
			if err := s.db.SetSetting(ctx, category, key, value, "string", false); err != nil {
				s.logger.Error("Failed to set setting", zap.String("key", key), zap.Error(err))
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		}

		s.logAudit(r, "update", "settings", fmt.Sprintf("Updated settings category: %s", category), "success")
		s.jsonResponse(w, map[string]string{"status": "updated"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSettingsExport exports all settings as JSON.
func (s *MasterServer) handleSettingsExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	settings, err := s.db.ListAllSettings(ctx)
	if err != nil {
		s.logger.Error("Failed to list settings", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
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
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	var req map[string]map[string]struct {
		Value     string `json:"value"`
		Type      string `json:"type"`
		Encrypted bool   `json:"encrypted"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 10<<20)).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	var count int
	for category, settings := range req {
		for key, setting := range settings {
			valueType := setting.Type
			if valueType == "" {
				valueType = "string"
			}
			if err := s.db.SetSetting(ctx, category, key, setting.Value, valueType, setting.Encrypted); err != nil {
				s.logger.Error("Failed to import setting", zap.String("key", key), zap.Error(err))
				continue
			}
			count++
		}
	}

	s.logAudit(r, "import", "settings", fmt.Sprintf("Imported %d settings", count), "success")
	s.jsonResponse(w, map[string]interface{}{
		"status":   "imported",
		"imported": count,
	})
}

// --- Projects API (enhanced) ---

// handleProjectsAPI handles project list and creation with full implementation.
func (s *MasterServer) handleProjectsAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		projects, err := s.db.ListProjects()
		if err != nil {
			s.logger.Error("Failed to list projects", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		s.jsonResponse(w, projects)

	case http.MethodPost:
		var req struct {
			Name       string `json:"name"`
			Repository string `json:"repository"`
			Branch     string `json:"branch"`
			DeployPath string `json:"deploy_path"`
			Type       string `json:"type"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if req.Name == "" {
			s.jsonError(w, http.StatusBadRequest, "name is required")
			return
		}
		if req.Branch == "" {
			req.Branch = "main"
		}

		project := &storage.Project{
			Name:       req.Name,
			Repository: req.Repository,
			Branch:     req.Branch,
			DeployPath: req.DeployPath,
			Type:       req.Type,
			CreatedAt:  time.Now(),
		}

		if err := s.db.CreateProject(project); err != nil {
			s.logger.Error("Failed to create project", zap.Error(err))
			s.jsonError(w, http.StatusConflict, "project already exists")
			return
		}

		s.logAudit(r, "create", "project", fmt.Sprintf("Created project: %s", req.Name), "success")
		w.WriteHeader(http.StatusCreated)
		s.jsonResponse(w, project)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleProjectAPI handles individual project operations.
func (s *MasterServer) handleProjectAPI(w http.ResponseWriter, r *http.Request) {
	// Extract project name from path: /api/v1/projects/{name}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/projects/")
	parts := strings.Split(path, "/")
	projectName := parts[0]

	if projectName == "" {
		http.Error(w, "Project name required", http.StatusBadRequest)
		return
	}

	// Check for sub-resources
	if len(parts) > 1 {
		switch parts[1] {
		case "webhooks":
			s.handleProjectWebhooks(w, r, projectName)
			return
		case "deploy":
			s.handleProjectDeploy(w, r, projectName)
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		project, err := s.db.GetProjectByName(ctx, projectName)
		if err != nil {
			http.Error(w, "Project not found", http.StatusNotFound)
			return
		}
		s.jsonResponse(w, project)

	case http.MethodPut:
		var req struct {
			Repository string `json:"repository"`
			Branch     string `json:"branch"`
			DeployPath string `json:"deploy_path"`
			Type       string `json:"type"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		project, err := s.db.GetProjectByName(ctx, projectName)
		if err != nil {
			http.Error(w, "Project not found", http.StatusNotFound)
			return
		}

		// Update fields
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

		if err := s.db.UpdateProjectByName(ctx, project); err != nil {
			s.logger.Error("Failed to update project", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		s.logAudit(r, "update", "project", fmt.Sprintf("Updated project: %s", projectName), "success")
		s.jsonResponse(w, project)

	case http.MethodDelete:
		if err := s.db.DeleteProject(projectName); err != nil {
			s.logger.Error("Failed to delete project", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		s.logAudit(r, "delete", "project", fmt.Sprintf("Deleted project: %s", projectName), "success")
		s.jsonResponse(w, map[string]string{"status": "deleted"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleProjectWebhooks handles webhook configuration for a project.
func (s *MasterServer) handleProjectWebhooks(w http.ResponseWriter, r *http.Request, projectName string) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	project, err := s.db.GetProjectByName(ctx, projectName)
	if err != nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Get all webhooks for this project
		webhooks := make([]map[string]interface{}, 0)
		for _, provider := range []string{"github", "gitlab", "bitbucket"} {
			wh, err := s.db.GetProjectWebhook(ctx, project.ID, provider)
			if err == nil && wh != nil {
				webhooks = append(webhooks, map[string]interface{}{
					"provider": provider,
					"enabled":  wh.Enabled,
				})
			}
		}
		s.jsonResponse(w, webhooks)

	case http.MethodPost:
		var req struct {
			Provider      string `json:"provider"`
			Secret        string `json:"secret"`
			Enabled       bool   `json:"enabled"`
			RequireSecret *bool  `json:"require_secret"` // Pointer to detect if field was provided
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
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

		if err := s.db.SetProjectWebhook(ctx, project.ID, req.Provider, []byte(req.Secret), req.Enabled, requireSecret); err != nil {
			s.logger.Error("Failed to set webhook", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		s.logAudit(r, "create", "webhook", fmt.Sprintf("Configured %s webhook for project: %s", req.Provider, projectName), "success")
		w.WriteHeader(http.StatusCreated)
		s.jsonResponse(w, map[string]string{"status": "created"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleProjectDeploy triggers a deployment for a project.
func (s *MasterServer) handleProjectDeploy(w http.ResponseWriter, r *http.Request, projectName string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	project, err := s.db.GetProjectByName(ctx, projectName)
	if err != nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	var req struct {
		Branch      string `json:"branch"`
		Target      string `json:"target"`
		ScheduledAt string `json:"scheduled_at,omitempty"`
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

	// Get username from context
	username := "api"
	if userID, ok := GetUserIDFromContext(r.Context()); ok {
		if user, err := s.db.GetUserByID(ctx, userID); err == nil && user != nil {
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

		if err := s.db.CreateScheduledDeployment(ctx, deploymentID, project.Name, req.Target, req.Branch, scheduledTime, username); err != nil {
			s.logger.Error("Failed to create scheduled deployment", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		s.logAudit(r, "schedule", "deployment", fmt.Sprintf("Scheduled deployment for %s at %s", projectName, scheduledTime), "success")
		w.WriteHeader(http.StatusAccepted)
		s.jsonResponse(w, map[string]interface{}{
			"id":           deploymentID,
			"status":       "scheduled",
			"scheduled_at": scheduledTime,
		})
		return
	}

	// Create immediate deployment
	deployment := &storage.Deployment{
		ID:          deploymentID,
		Project:     project.Name,
		Target:      req.Target,
		Branch:      req.Branch,
		Status:      "pending",
		TriggeredBy: username,
		StartedAt:   time.Now(),
	}

	if err := s.db.CreateDeployment(ctx, deployment); err != nil {
		s.logger.Error("Failed to create deployment", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	s.logAudit(r, "trigger", "deployment", fmt.Sprintf("Triggered deployment for %s", projectName), "success")
	w.WriteHeader(http.StatusAccepted)
	s.jsonResponse(w, deployment)
}

// --- Agents API (enhanced) ---

// handleAgentsAPI handles agent list.
func (s *MasterServer) handleAgentsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	agents, err := s.db.ListAgents(ctx)
	if err != nil {
		s.logger.Error("Failed to list agents", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	s.jsonResponse(w, agents)
}

// handleAgentAPI handles individual agent operations.
func (s *MasterServer) handleAgentAPI(w http.ResponseWriter, r *http.Request) {
	// Extract agent ID from path: /api/v1/agents/{id} or /api/v1/agents/{id}/token
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/agents/")
	parts := strings.Split(path, "/")
	agentID := parts[0]

	if agentID == "" {
		http.Error(w, "Agent ID required", http.StatusBadRequest)
		return
	}

	// Handle token sub-resource: POST /api/v1/agents/{id}/token
	if len(parts) > 1 && parts[1] == "token" {
		s.handleAgentToken(w, r, agentID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		agent, err := s.db.GetAgent(ctx, agentID)
		if err != nil {
			s.logger.Error("Failed to get agent", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if agent == nil {
			http.Error(w, "Agent not found", http.StatusNotFound)
			return
		}
		s.jsonResponse(w, agent)

	case http.MethodPut:
		var req struct {
			Labels map[string]string `json:"labels"`
			Status string            `json:"status"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		agent, err := s.db.GetAgent(ctx, agentID)
		if err != nil || agent == nil {
			http.Error(w, "Agent not found", http.StatusNotFound)
			return
		}

		if req.Labels != nil {
			agent.Labels = req.Labels
		}
		if req.Status != "" {
			agent.Status = req.Status
		}

		if err := s.db.UpsertAgent(ctx, agent); err != nil {
			s.logger.Error("Failed to update agent", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		s.logAudit(r, "update", "agent", fmt.Sprintf("Updated agent: %s", agentID), "success")
		s.jsonResponse(w, agent)

	case http.MethodDelete:
		if err := s.db.DeleteAgent(ctx, agentID); err != nil {
			s.logger.Error("Failed to delete agent", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		s.logAudit(r, "delete", "agent", fmt.Sprintf("Deleted agent: %s", agentID), "success")
		s.jsonResponse(w, map[string]string{"status": "deleted"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAgentToken handles POST /api/v1/agents/{id}/token to generate a registration token.
func (s *MasterServer) handleAgentToken(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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

	s.jsonResponse(w, map[string]string{
		"agent_id": agentID,
		"token":    token,
		"expires":  "30m", // Token expires after 30 minutes if not used
	})
}

// --- Deployments API (enhanced) ---

// handleDeploymentsAPI handles deployment list and creation.
func (s *MasterServer) handleDeploymentsAPI(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		// Parse query params
		limit := 50
		if l := r.URL.Query().Get("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
				limit = parsed
			}
		}

		deployments, err := s.db.ListDeploymentsRecent(ctx, limit)
		if err != nil {
			s.logger.Error("Failed to list deployments", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		s.jsonResponse(w, deployments)

	case http.MethodPost:
		var req struct {
			Project     string `json:"project"`
			Branch      string `json:"branch"`
			Target      string `json:"target"`
			ScheduledAt string `json:"scheduled_at,omitempty"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if req.Project == "" {
			s.jsonError(w, http.StatusBadRequest, "project is required")
			return
		}

		// Forward to project deploy handler
		s.handleProjectDeploy(w, r, req.Project)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleDeploymentAPI handles individual deployment operations.
func (s *MasterServer) handleDeploymentAPI(w http.ResponseWriter, r *http.Request) {
	// Extract deployment ID and action from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/deployments/")
	parts := strings.Split(path, "/")
	deploymentID := parts[0]

	if deploymentID == "" {
		http.Error(w, "Deployment ID required", http.StatusBadRequest)
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

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		deployment, err := s.db.GetDeployment(ctx, deploymentID)
		if err != nil {
			s.logger.Error("Failed to get deployment", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if deployment == nil {
			http.Error(w, "Deployment not found", http.StatusNotFound)
			return
		}
		s.jsonResponse(w, deployment)

	case http.MethodDelete:
		// Cancel if running, otherwise just acknowledge
		deployment, err := s.db.GetDeployment(ctx, deploymentID)
		if err != nil || deployment == nil {
			http.Error(w, "Deployment not found", http.StatusNotFound)
			return
		}

		if deployment.Status == "scheduled" {
			if err := s.db.CancelScheduledDeployment(ctx, deploymentID); err != nil {
				s.logger.Error("Failed to cancel deployment", zap.Error(err))
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		}

		s.logAudit(r, "cancel", "deployment", fmt.Sprintf("Cancelled deployment: %s", deploymentID), "success")
		s.jsonResponse(w, map[string]string{"status": "cancelled"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleDeploymentCancel cancels a running deployment.
func (s *MasterServer) handleDeploymentCancel(w http.ResponseWriter, r *http.Request, deploymentID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	deployment, err := s.db.GetDeployment(ctx, deploymentID)
	if err != nil || deployment == nil {
		http.Error(w, "Deployment not found", http.StatusNotFound)
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
	if err := s.db.UpdateDeployment(ctx, deployment); err != nil {
		s.logger.Error("Failed to cancel deployment", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	s.logAudit(r, "cancel", "deployment", fmt.Sprintf("Cancelled deployment: %s", deploymentID), "success")
	s.jsonResponse(w, map[string]string{"status": "cancelled"})
}

// handleDeploymentRollback triggers a rollback for a deployment.
func (s *MasterServer) handleDeploymentRollback(w http.ResponseWriter, r *http.Request, deploymentID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	deployment, err := s.db.GetDeployment(ctx, deploymentID)
	if err != nil || deployment == nil {
		http.Error(w, "Deployment not found", http.StatusNotFound)
		return
	}

	// Get username from context
	username := "api"
	if userID, ok := GetUserIDFromContext(r.Context()); ok {
		if user, err := s.db.GetUserByID(ctx, userID); err == nil && user != nil {
			username = user.Username
		}
	}

	// Create rollback deployment record
	rollbackID := fmt.Sprintf("rollback-%d", time.Now().UnixNano())
	rollback := &storage.Deployment{
		ID:            rollbackID,
		Project:       deployment.Project,
		Target:        deployment.Target,
		Branch:        deployment.Branch,
		Status:        "pending",
		TriggeredBy:   username,
		TriggerSource: "rollback:" + deploymentID,
		StartedAt:     time.Now(),
	}

	if err := s.db.CreateDeployment(ctx, rollback); err != nil {
		s.logger.Error("Failed to create rollback", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
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
			project, err := s.db.GetProjectByName(ctx, deployment.Project)
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
				if err := s.db.UpdateDeployment(ctx, rollback); err != nil {
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
					if err := s.db.UpdateDeployment(ctx, rollback); err != nil {
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
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	logs, err := s.db.ListDeploymentLogs(ctx, deploymentID)
	if err != nil {
		s.logger.Error("Failed to get deployment logs", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	s.jsonResponse(w, logs)
}

// handleDeploymentLogsStream streams deployment logs using Server-Sent Events (SSE).
// This allows real-time log streaming without WebSocket dependencies.
func (s *MasterServer) handleDeploymentLogsStream(w http.ResponseWriter, r *http.Request, deploymentID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send initial logs
	ctx := r.Context()
	logs, err := s.db.ListDeploymentLogs(ctx, deploymentID)
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
			newLogs, err := s.db.ListDeploymentLogsAfter(ctx, deploymentID, lastID)
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
			deployment, err := s.db.GetDeployment(ctx, deploymentID)
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
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Get current user
	userID, ok := GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		keys, err := s.db.ListAPIKeys(ctx, userID)
		if err != nil {
			s.logger.Error("Failed to list API keys", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
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
		var req struct {
			Name      string `json:"name"`
			ExpiresIn int    `json:"expires_in_days"` // 0 = no expiry
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if req.Name == "" {
			s.jsonError(w, http.StatusBadRequest, "name is required")
			return
		}

		// Generate secure API key
		rawKey, err := security.GenerateSecureToken(32)
		if err != nil {
			s.logger.Error("Failed to generate API key", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Hash it for storage
		hash := sha256.Sum256([]byte(rawKey))
		keyHash := hex.EncodeToString(hash[:])

		var expiresAt *time.Time
		if req.ExpiresIn > 0 {
			exp := time.Now().AddDate(0, 0, req.ExpiresIn)
			expiresAt = &exp
		}

		apiKey := &storage.APIKey{
			UserID:    userID,
			Name:      req.Name,
			KeyHash:   keyHash,
			CreatedAt: time.Now(),
			ExpiresAt: expiresAt,
		}

		if err := s.db.CreateAPIKey(ctx, apiKey); err != nil {
			s.logger.Error("Failed to create API key", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		s.logAudit(r, "create", "apikey", fmt.Sprintf("Created API key: %s", req.Name), "success")

		// Return the raw key (only time it's visible)
		w.WriteHeader(http.StatusCreated)
		s.jsonResponse(w, map[string]interface{}{
			"id":        apiKey.ID,
			"name":      apiKey.Name,
			"key":       rawKey, // Only returned on creation!
			"expiresAt": expiresAt,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAPIKey handles individual API key operations.
func (s *MasterServer) handleAPIKey(w http.ResponseWriter, r *http.Request) {
	// Extract key ID from path: /api/v1/apikeys/{id}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/apikeys/")
	keyID, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		http.Error(w, "Invalid key ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	switch r.Method {
	case http.MethodDelete:
		if err := s.db.DeleteAPIKey(ctx, keyID); err != nil {
			s.logger.Error("Failed to revoke API key", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		s.logAudit(r, "revoke", "apikey", fmt.Sprintf("Revoked API key ID: %d", keyID), "success")
		s.jsonResponse(w, map[string]string{"status": "revoked"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- Helper methods ---

// jsonError sends a JSON error response.
func (s *MasterServer) jsonError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error":   true,
		"message": message,
	})
}
