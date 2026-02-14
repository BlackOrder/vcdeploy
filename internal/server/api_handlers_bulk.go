// Package server provides bulk operation API handlers.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/proto"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/BlackOrder/vcdeploy/internal/validation"
	"github.com/rs/xid"
	"go.uber.org/zap"
)

// BulkDeployRequest represents a request to deploy multiple projects.
type BulkDeployRequest struct {
	Projects []BulkDeployItem `json:"projects"`
}

// BulkDeployItem represents a single project deployment in a bulk request.
type BulkDeployItem struct {
	Project string `json:"project"`
	Branch  string `json:"branch,omitempty"`
	Target  string `json:"target,omitempty"`
}

// BulkDeployResponse represents the response from a bulk deploy operation.
type BulkDeployResponse struct {
	Deployments []BulkDeployResult `json:"deployments"`
	Succeeded   int                `json:"succeeded"`
	Failed      int                `json:"failed"`
}

// BulkDeployResult represents the result of a single deployment in a bulk operation.
type BulkDeployResult struct {
	Project      string `json:"project"`
	DeploymentID string `json:"deployment_id,omitempty"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
}

// BulkCancelRequest represents a request to cancel multiple deployments.
type BulkCancelRequest struct {
	DeploymentIDs []string `json:"deployment_ids"`
}

// BulkCancelResponse represents the response from a bulk cancel operation.
type BulkCancelResponse struct {
	Results   []BulkCancelResult `json:"results"`
	Succeeded int                `json:"succeeded"`
	Failed    int                `json:"failed"`
}

// BulkCancelResult represents the result of a single cancellation in a bulk operation.
type BulkCancelResult struct {
	DeploymentID string `json:"deployment_id"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
}

// BulkSecretsRequest represents a request to import multiple secrets.
type BulkSecretsRequest struct {
	Secrets []BulkSecretItem `json:"secrets"`
}

// BulkSecretItem represents a single secret in a bulk import.
type BulkSecretItem struct {
	Project string `json:"project"`
	Scope   string `json:"scope"`
	Key     string `json:"key"`
	Value   string `json:"value"`
}

// BulkSecretsResponse represents the response from a bulk secrets operation.
type BulkSecretsResponse struct {
	Results   []BulkSecretResult `json:"results"`
	Succeeded int                `json:"succeeded"`
	Failed    int                `json:"failed"`
}

// BulkSecretResult represents the result of a single secret in a bulk operation.
type BulkSecretResult struct {
	Project string `json:"project"`
	Scope   string `json:"scope"`
	Key     string `json:"key"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
}

// handleBulkDeploy handles POST /api/v1/deployments/bulk to deploy multiple projects
// and DELETE /api/v1/deployments/bulk to cancel multiple deployments.
func (s *MasterServer) handleBulkDeploy(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleBulkDeployPost(w, r)
	case http.MethodDelete:
		s.handleBulkCancelDeployments(w, r)
	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleBulkDeployPost handles POST /api/v1/deployments/bulk to deploy multiple projects.
func (s *MasterServer) handleBulkDeployPost(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), TimeoutLong)
	defer cancel()

	// Check write access
	if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	var req BulkDeployRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if len(req.Projects) == 0 {
		s.jsonError(w, http.StatusBadRequest, "At least one project is required")
		return
	}

	if len(req.Projects) > 100 {
		s.jsonError(w, http.StatusBadRequest, "Maximum 100 projects per bulk deploy")
		return
	}

	// Get username from context
	username := "api"
	if userID, ok := GetUserIDFromContext(r.Context()); ok {
		if user, err := s.userService.GetByID(ctx, userID); err == nil && user != nil {
			username = user.Username
		}
	}

	response := BulkDeployResponse{
		Deployments: make([]BulkDeployResult, 0, len(req.Projects)),
	}

	for _, item := range req.Projects {
		result := BulkDeployResult{
			Project: item.Project,
		}

		// Get project
		project, err := s.projectService.GetByName(ctx, item.Project)
		if err != nil {
			result.Status = "failed"
			result.Error = "Project not found"
			response.Deployments = append(response.Deployments, result)
			response.Failed++
			continue
		}

		// Set defaults
		branch := item.Branch
		if branch == "" {
			branch = project.Branch
		}
		target := item.Target
		if target == "" {
			target = "production"
		}

		// Create deployment
		deploymentID := xid.New().String()
		deployment := &storage.DeploymentRecord{
			ID:            deploymentID,
			Project:       project.Name,
			Target:        target,
			Branch:        branch,
			Status:        "pending",
			TriggeredBy:   username,
			TriggerSource: "bulk-api",
			StartedAt:     time.Now(),
		}

		if err := s.deploymentService.Create(ctx, deployment); err != nil {
			result.Status = "failed"
			result.Error = "Failed to create deployment"
			response.Deployments = append(response.Deployments, result)
			response.Failed++
			continue
		}

		// Try to send deploy command to agent
		if s.agentServer != nil {
			agentID := target
			if !s.agentServer.IsAgentConnected(agentID) {
				connectedAgents := s.agentServer.GetConnectedAgents()
				if len(connectedAgents) > 0 {
					agentID = connectedAgents[0]
				}
			}

			if agentID != "" && s.agentServer.IsAgentConnected(agentID) {
				deployCmd := &proto.DeployCommand{
					DeploymentId: deploymentID,
					Project:      project.Name,
					Branch:       branch,
					Target:       target,
					Repository:   project.Repository,
					Path:         project.DeployPath,
				}

				deployment.Status = "running"
				_ = s.deploymentService.Update(ctx, deployment)

				if err := s.agentServer.SendDeployCommand(agentID, deployCmd); err != nil {
					s.logger.Error("Failed to send deploy command", zap.String("project", item.Project), zap.Error(err))
				}
			}
		}

		result.DeploymentID = deploymentID
		result.Status = "accepted"
		response.Deployments = append(response.Deployments, result)
		response.Succeeded++

		// Publish SSE event
		s.publishDeploymentEvent(deploymentID, project.Name, string(deployment.Status), target, branch, "Bulk deploy")
	}

	s.logAudit(r, "bulk_deploy", "deployment", fmt.Sprintf("Bulk deploy: %d succeeded, %d failed", response.Succeeded, response.Failed), "success")
	s.jsonResponse(w, response)
}

// handleBulkCancelDeployments handles DELETE /api/v1/deployments/bulk to cancel multiple deployments.
func (s *MasterServer) handleBulkCancelDeployments(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	// Check write access
	if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	var req BulkCancelRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if len(req.DeploymentIDs) == 0 {
		s.jsonError(w, http.StatusBadRequest, "At least one deployment_id is required")
		return
	}

	if len(req.DeploymentIDs) > 100 {
		s.jsonError(w, http.StatusBadRequest, "Maximum 100 deployments per bulk cancel")
		return
	}

	response := BulkCancelResponse{
		Results: make([]BulkCancelResult, 0, len(req.DeploymentIDs)),
	}

	for _, deploymentID := range req.DeploymentIDs {
		result := BulkCancelResult{
			DeploymentID: deploymentID,
		}

		deployment, err := s.deploymentService.GetByID(ctx, deploymentID)
		if err != nil || deployment == nil {
			result.Status = "failed"
			result.Error = "Deployment not found"
			response.Results = append(response.Results, result)
			response.Failed++
			continue
		}

		// Check if cancellable
		if deployment.Status != "pending" && deployment.Status != "running" {
			result.Status = "failed"
			result.Error = "Deployment is not cancellable"
			response.Results = append(response.Results, result)
			response.Failed++
			continue
		}

		// Cancel
		now := time.Now()
		deployment.Status = "cancelled"
		deployment.CompletedAt = &now
		if err := s.deploymentService.Update(ctx, deployment); err != nil {
			result.Status = "failed"
			result.Error = "Failed to cancel"
			response.Results = append(response.Results, result)
			response.Failed++
			continue
		}

		result.Status = "cancelled"
		response.Results = append(response.Results, result)
		response.Succeeded++

		// Publish SSE event
		s.publishDeploymentEvent(deployment.ID, deployment.Project, "cancelled", deployment.Target, deployment.Branch, "Bulk cancel")
	}

	s.logAudit(r, "bulk_cancel", "deployment", fmt.Sprintf("Bulk cancel: %d succeeded, %d failed", response.Succeeded, response.Failed), "success")
	s.jsonResponse(w, response)
}

// handleBulkSecrets handles POST /api/v1/secrets/bulk to import multiple secrets.
func (s *MasterServer) handleBulkSecrets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	// Check write access
	if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	if s.secretService == nil {
		s.jsonError(w, http.StatusInternalServerError, "Secret service not configured")
		return
	}

	var req BulkSecretsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if len(req.Secrets) == 0 {
		s.jsonError(w, http.StatusBadRequest, "At least one secret is required")
		return
	}

	if len(req.Secrets) > 1000 {
		s.jsonError(w, http.StatusBadRequest, "Maximum 1000 secrets per bulk import")
		return
	}

	response := BulkSecretsResponse{
		Results: make([]BulkSecretResult, 0, len(req.Secrets)),
	}

	for _, item := range req.Secrets {
		result := BulkSecretResult{
			Project: item.Project,
			Scope:   item.Scope,
			Key:     item.Key,
		}

		if item.Project == "" || item.Key == "" {
			result.Status = "failed"
			result.Error = "project and key are required"
			response.Results = append(response.Results, result)
			response.Failed++
			continue
		}

		scope := item.Scope
		if scope == "" {
			scope = "default"
		}

		if err := s.secretService.Set(ctx, item.Project, scope, item.Key, item.Value); err != nil {
			result.Status = "failed"
			result.Error = "Failed to store secret"
			response.Results = append(response.Results, result)
			response.Failed++
			continue
		}

		result.Status = "stored"
		response.Results = append(response.Results, result)
		response.Succeeded++
	}

	s.logAudit(r, "bulk_import", "secret", fmt.Sprintf("Bulk import: %d succeeded, %d failed", response.Succeeded, response.Failed), "success")
	s.jsonResponse(w, response)
}

// BulkAgentsRequest represents a request to update multiple agents.
type BulkAgentsRequest struct {
	Agents []BulkAgentItem `json:"agents"`
}

// BulkAgentItem represents a single agent update in a bulk request.
type BulkAgentItem struct {
	ID           string            `json:"id"`
	Labels       map[string]string `json:"labels,omitempty"`
	UpdatePolicy string            `json:"update_policy,omitempty"`
}

// BulkAgentsResponse represents the response from a bulk agents operation.
type BulkAgentsResponse struct {
	Results   []BulkAgentResult `json:"results"`
	Succeeded int               `json:"succeeded"`
	Failed    int               `json:"failed"`
}

// BulkAgentResult represents the result of a single agent update in a bulk operation.
type BulkAgentResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// handleBulkAgents handles POST /api/v1/agents/bulk to update multiple agents.
func (s *MasterServer) handleBulkAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	// Check write access
	if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	var req BulkAgentsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if len(req.Agents) == 0 {
		s.jsonError(w, http.StatusBadRequest, "At least one agent is required")
		return
	}

	if len(req.Agents) > 100 {
		s.jsonError(w, http.StatusBadRequest, "Maximum 100 agents per bulk update")
		return
	}

	response := BulkAgentsResponse{
		Results: make([]BulkAgentResult, 0, len(req.Agents)),
	}

	for _, item := range req.Agents {
		result := BulkAgentResult{
			ID: item.ID,
		}

		if item.ID == "" {
			result.Status = "failed"
			result.Error = "Agent ID is required"
			response.Results = append(response.Results, result)
			response.Failed++
			continue
		}

		agent, err := s.agentService.GetByID(ctx, item.ID)
		if err != nil || agent == nil {
			result.Status = "failed"
			result.Error = "Agent not found"
			response.Results = append(response.Results, result)
			response.Failed++
			continue
		}

		// Apply updates
		if item.Labels != nil {
			agent.Labels = item.Labels
		}
		if item.UpdatePolicy != "" {
			agent.UpdatePolicy = item.UpdatePolicy
		}

		if err := s.agentService.Upsert(ctx, agent); err != nil {
			result.Status = "failed"
			result.Error = "Failed to update agent"
			response.Results = append(response.Results, result)
			response.Failed++
			continue
		}

		result.Status = "updated"
		response.Results = append(response.Results, result)
		response.Succeeded++

		// Publish SSE event
		s.publishAgentEvent(agent.ID, agent.Hostname, "updated", "Bulk update")
	}

	s.logAudit(r, "bulk_update", "agent", fmt.Sprintf("Bulk update: %d succeeded, %d failed", response.Succeeded, response.Failed), "success")
	s.jsonResponse(w, response)
}
