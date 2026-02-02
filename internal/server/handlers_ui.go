// Package server provides UI page handlers for the master server.
package server

import (
	"context"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// handleDashboard renders the main dashboard page.
func (s *MasterServer) handleDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Fetch stats
	stats := map[string]interface{}{
		"TotalProjects":    0,
		"DeploymentsToday": 0,
		"ConnectedAgents":  0,
		"SuccessRate":      0,
	}

	// Get projects count
	projects, err := s.projectService.List(ctx)
	if err == nil {
		stats["TotalProjects"] = len(projects)
	}

	// Get agents and count connected
	agents, err := s.agentService.List(ctx)
	var agentData []map[string]interface{}
	if err == nil {
		var connectedCount int
		for _, a := range agents {
			if a.Status == "connected" {
				connectedCount++
			}
			agentData = append(agentData, map[string]interface{}{
				"Name":     a.Hostname,
				"Hostname": a.Hostname,
				"Status":   a.Status,
			})
		}
		stats["ConnectedAgents"] = connectedCount
	}

	// Get recent deployments and calculate stats (limited to 5 for dashboard display)
	deployments, err := s.deploymentService.ListRecent(ctx, 5)
	var recentDeployments []map[string]interface{}
	if err == nil {
		var successCount, totalCount int
		today := time.Now().Truncate(24 * time.Hour)
		var deploymentsToday int

		for _, d := range deployments {
			if d.StartedAt.After(today) {
				deploymentsToday++
			}
			if d.Status == "success" {
				successCount++
			}
			totalCount++

			recentDeployments = append(recentDeployments, map[string]interface{}{
				"ID":          d.ID,
				"ProjectName": d.Project,
				"Branch":      d.Branch,
				"Status":      d.Status,
				"CreatedAt":   d.StartedAt,
			})
		}

		stats["DeploymentsToday"] = deploymentsToday
		if totalCount > 0 {
			stats["SuccessRate"] = (successCount * 100) / totalCount
		}
	}

	// Get recent audit logs (limited to 5 for dashboard)
	auditLogs, err := s.auditService.List(ctx, 5, 0)
	var recentActivity []map[string]interface{}
	if err == nil {
		for _, log := range auditLogs {
			recentActivity = append(recentActivity, map[string]interface{}{
				"Action":    log.Action,
				"Username":  log.User,
				"CreatedAt": log.Timestamp,
			})
		}
	}

	data := s.withCommonData(r, map[string]interface{}{
		"Title":             "Dashboard",
		"Active":            "dashboard",
		"Stats":             stats,
		"RecentDeployments": recentDeployments,
		"Agents":            agentData,
		"RecentActivity":    recentActivity,
	})

	s.renderTemplate(w, "dashboard", data)
}

// handleProjectsUI renders the projects list page.
func (s *MasterServer) handleProjectsUI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get projects
	var projectsList []map[string]interface{}
	projects, err := s.projectService.List(ctx)
	if err != nil {
		s.logger.Error("Failed to list projects", zap.Error(err))
	} else {
		for _, p := range projects {
			projectsList = append(projectsList, map[string]interface{}{
				"ID":         p.ID,
				"Name":       p.Name,
				"Type":       p.Type,
				"Repository": p.Repository,
				"Branch":     p.Branch,
				"Path":       p.DeployPath,
				"CreatedAt":  p.CreatedAt,
			})
		}
	}

	s.renderTemplate(w, "projects", s.withCommonData(r, map[string]interface{}{
		"Title":    "Projects",
		"Active":   "projects",
		"Projects": projectsList,
	}))
}

// handleDeploymentsUI renders the deployments list page.
func (s *MasterServer) handleDeploymentsUI(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, "deployments", s.withCommonData(r, map[string]interface{}{"Title": "Deployments", "Active": "deployments"}))
}

// handleAgentsUI renders the agents list page.
func (s *MasterServer) handleAgentsUI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get agents from database
	var agentsList []map[string]interface{}
	agents, err := s.agentService.List(ctx)
	if err != nil {
		s.logger.Error("Failed to list agents", zap.Error(err))
	} else {
		for _, a := range agents {
			agentsList = append(agentsList, map[string]interface{}{
				"ID":         a.ID,
				"Hostname":   a.Hostname,
				"Status":     a.Status,
				"Version":    a.Version,
				"OS":         a.OS,
				"Arch":       a.Arch,
				"LastSeenAt": a.LastSeenAt,
			})
		}
	}

	s.renderTemplate(w, "agents", s.withCommonData(r, map[string]interface{}{
		"Title":  "Agents",
		"Active": "agents",
		"Agents": agentsList,
	}))
}

// handleSettingsUI renders the settings page.
func (s *MasterServer) handleSettingsUI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Build settings map for template
	settings := map[string]interface{}{}

	if s.settingsSvc != nil {
		// Server settings
		settings["MasterURL"], _ = s.settingsSvc.GetString(ctx, "server", "master_url", "")
		settings["LogLevel"], _ = s.settingsSvc.GetString(ctx, "logs", "app_level", "info")
		settings["RetentionDays"], _ = s.settingsSvc.GetInt(ctx, "logs", "deploy_retention", 90)
		settings["MaxConcurrent"], _ = s.settingsSvc.GetInt(ctx, "deployments", "max_concurrent", 10)

		// Security settings
		settings["Require2FA"], _ = s.settingsSvc.GetBool(ctx, "security", "require_2fa", false)
		settings["ForceHTTPS"], _ = s.settingsSvc.GetBool(ctx, "security", "force_https", false)
		settings["AuditLog"], _ = s.settingsSvc.GetBool(ctx, "security", "audit_log", true)
		settings["SessionTimeout"], _ = s.settingsSvc.GetInt(ctx, "security", "session_timeout", 60)

		// Notification settings
		settings["SlackWebhook"], _ = s.settingsSvc.GetString(ctx, "notifications", "slack_webhook", "")
		settings["SlackChannel"], _ = s.settingsSvc.GetString(ctx, "notifications", "slack_channel", "")
		settings["SMTPHost"], _ = s.settingsSvc.GetString(ctx, "notifications", "smtp_host", "")
		settings["SMTPPort"], _ = s.settingsSvc.GetInt(ctx, "notifications", "smtp_port", 587)
		settings["SMTPUser"], _ = s.settingsSvc.GetString(ctx, "notifications", "smtp_user", "")
		settings["SMTPFrom"], _ = s.settingsSvc.GetString(ctx, "notifications", "smtp_from", "")

		// Appearance settings
		settings["DarkMode"], _ = s.settingsSvc.GetBool(ctx, "appearance", "dark_mode", true)
		settings["ThemeColor"], _ = s.settingsSvc.GetString(ctx, "appearance", "theme_color", "green")
	}

	data := s.withCommonData(r, map[string]interface{}{
		"Title":  "Settings",
		"Active": "settings",
	})
	data["Settings"] = settings

	// Add current user's TOTP status for admin disable modal
	if userID, ok := GetUserIDFromContext(r.Context()); ok {
		if user, err := s.userService.GetByID(ctx, userID); err == nil {
			data["CurrentUserTOTPEnabled"] = user.TOTPEnabled
		}
	}

	s.renderTemplate(w, "settings", data)
}

// handleSecretsUI renders the secrets management page.
func (s *MasterServer) handleSecretsUI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get projects for filter dropdown
	projects, err := s.projectService.List(ctx)
	if err != nil {
		s.logger.Error("Failed to list projects for secrets UI", zap.Error(err))
		projects = nil
	}

	// Get secrets
	var secretsList []map[string]interface{}
	if s.secretService != nil {
		secrets, err := s.secretService.ListAll(ctx)
		if err != nil {
			s.logger.Error("Failed to list secrets", zap.Error(err))
		} else {
			for _, sec := range secrets {
				secretsList = append(secretsList, map[string]interface{}{
					"ID":        sec.ID,
					"Project":   sec.Project,
					"Scope":     sec.Scope,
					"Key":       sec.Key,
					"CreatedAt": sec.CreatedAt,
				})
			}
		}
	}

	data := s.withCommonData(r, map[string]interface{}{
		"Title":   "Secrets",
		"Active":  "secrets",
		"Secrets": secretsList,
	})
	data["Projects"] = projects
	s.renderTemplate(w, "secrets", data)
}

// handleProjectTypesUI renders the project types page.
func (s *MasterServer) handleProjectTypesUI(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, "project-types", s.withCommonData(r, map[string]interface{}{
		"Title":  "Project Types",
		"Active": "project-types",
	}))
}

// handleAuditUI renders the audit log page.
func (s *MasterServer) handleAuditUI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get audit logs
	var auditLogs []map[string]interface{}
	entries, err := s.auditService.List(ctx, 100, 0)
	if err != nil {
		s.logger.Error("Failed to list audit logs", zap.Error(err))
	} else {
		for _, entry := range entries {
			auditLogs = append(auditLogs, map[string]interface{}{
				"ID":        entry.ID,
				"Timestamp": entry.Timestamp,
				"User":      entry.User,
				"Action":    entry.Action,
				"Resource":  entry.Resource,
				"Details":   entry.Details,
				"IPAddress": entry.IPAddress,
				"Result":    entry.Result,
			})
		}
	}

	s.renderTemplate(w, "audit", s.withCommonData(r, map[string]interface{}{
		"Title":     "Audit Log",
		"Active":    "audit",
		"AuditLogs": auditLogs,
	}))
}

// handleAPIKeysUI renders the API keys management page.
func (s *MasterServer) handleAPIKeysUI(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, "apikeys", s.withCommonData(r, map[string]interface{}{
		"Title":  "API Keys",
		"Active": "api-keys",
	}))
}

// withCommonData adds common template data like ShowNav, Username, CSPNonce for authenticated pages.
func (s *MasterServer) withCommonData(r *http.Request, data map[string]interface{}) map[string]interface{} {
	if data == nil {
		data = make(map[string]interface{})
	}
	data["ShowNav"] = true

	// Add CSP nonce for inline scripts
	if nonce := GetCSPNonce(r); nonce != "" {
		data["CSPNonce"] = nonce
	}

	// Get username from context
	if userID, ok := GetUserIDFromContext(r.Context()); ok {
		user, err := s.userService.GetByID(r.Context(), userID)
		if err == nil && user != nil {
			data["Username"] = user.Username
		}
	}

	return data
}

// renderTemplate renders a named template with the given data.
func (s *MasterServer) renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	if s.templates == nil {
		http.Error(w, "Templates not loaded", http.StatusInternalServerError)
		return
	}

	tmpl, ok := s.templates[name+".html"]
	if !ok {
		s.logger.Error("Template not found", zap.String("template", name))
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}

	// Convert data to a map and add common theme settings
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		dataMap = map[string]interface{}{}
	}

	// Add appearance settings if settings service is available
	if s.settingsSvc != nil {
		ctx := context.Background()
		darkMode, err := s.settingsSvc.GetBool(ctx, "appearance", "dark_mode", true)
		if err != nil {
			s.logger.Warn("Failed to load dark_mode setting, using default", zap.Error(err))
		}
		themeColor, err := s.settingsSvc.GetString(ctx, "appearance", "theme_color", "green")
		if err != nil {
			s.logger.Warn("Failed to load theme_color setting, using default", zap.Error(err))
		}
		theme, err := s.settingsSvc.GetString(ctx, "appearance", "theme", "dark")
		if err != nil {
			s.logger.Warn("Failed to load theme setting, using default", zap.Error(err))
		}

		dataMap["DarkMode"] = darkMode
		dataMap["ThemeColor"] = themeColor
		dataMap["Theme"] = theme
	} else {
		// Defaults if settings service not available
		dataMap["DarkMode"] = true
		dataMap["ThemeColor"] = "green"
		dataMap["Theme"] = "dark"
	}

	// Execute the page template (which includes base via {{template "base" .}})
	if err := tmpl.ExecuteTemplate(w, name+".html", dataMap); err != nil {
		s.logger.Error("Template render error", zap.String("template", name), zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
