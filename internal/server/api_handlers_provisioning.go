// Package server provides API endpoint handlers.
package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// --- Provisioning API Handlers ---

// ProvisionAgentRequest represents a request to provision an agent via SSH.
type ProvisionAgentRequest struct {
	AgentID    string `json:"agent_id"`
	TargetHost string `json:"target_host"`
	SSHUser    string `json:"ssh_user"`
	SSHKeyID   int64  `json:"ssh_key_id"`
	SSHPort    int    `json:"ssh_port,omitempty"`
}

// Validate validates the provision request.
func (r *ProvisionAgentRequest) Validate() error {
	if r.AgentID == "" {
		return services.NewInputError("agent_id is required", "agent_id")
	}
	if r.TargetHost == "" {
		return services.NewInputError("target_host is required", "target_host")
	}
	if r.SSHUser == "" {
		return services.NewInputError("ssh_user is required", "ssh_user")
	}
	if r.SSHKeyID <= 0 {
		return services.NewInputError("ssh_key_id is required", "ssh_key_id")
	}
	return nil
}

// handleProvisionAgent handles /api/v1/agents/provision endpoints.
func (s *MasterServer) handleProvisionAgent(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/agents/provision")

	if path == "" || path == "/" {
		if r.Method == http.MethodPost {
			s.handleStartProvisioning(w, r)
			return
		}
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Extract provision job ID from path
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	jobID := parts[0]

	if len(parts) == 2 {
		switch parts[1] {
		case "status":
			s.handleGetProvisionStatus(w, r, jobID)
			return
		case "logs":
			s.handleGetProvisionLogs(w, r, jobID)
			return
		}
	}

	s.jsonError(w, http.StatusNotFound, "Not found")
}

// handleStartProvisioning starts a new provisioning job.
func (s *MasterServer) handleStartProvisioning(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req ProvisionAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := req.Validate(); err != nil {
		if inputErr, ok := err.(*services.InputError); ok {
			s.jsonError(w, http.StatusBadRequest, inputErr.Message)
			return
		}
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Set default SSH port
	if req.SSHPort == 0 {
		req.SSHPort = 22
	}

	// Verify SSH key exists
	_, err := s.store.GetSSHKey(ctx, req.SSHKeyID)
	if err != nil {
		if services.IsNotFound(err) || strings.Contains(err.Error(), "not found") {
			s.jsonError(w, http.StatusBadRequest, "SSH key not found")
			return
		}
		s.logger.Error("Failed to get SSH key", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// Get initiated by user
	initiatedBy := "system"
	if userID, ok := GetUserIDFromContext(ctx); ok && userID > 0 {
		user, err := s.userService.GetByID(ctx, userID)
		if err == nil {
			initiatedBy = user.Username
		}
	}

	// Create provision job
	jobID := uuid.New().String()
	keyID := req.SSHKeyID // Convert to pointer
	job := &storage.ProvisionJob{
		ID:         jobID,
		TargetHost: req.TargetHost,
		TargetPort: req.SSHPort,
		TargetUser: req.SSHUser,
		SSHKeyID:   &keyID,
		Status:     "pending",
		Stage:      "initialized",
		Progress:   0,
		StartedAt:  time.Now(),
	}

	if err := s.provisionService.CreateJob(ctx, job); err != nil {
		s.logger.Error("Failed to create provision job", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// TODO: Trigger async provisioning worker
	// For now, we just create the job and let it be picked up by a background worker

	s.logAudit(r, "provision", "agent", "Started provisioning agent: "+req.AgentID+" on "+req.TargetHost+" by "+initiatedBy, "success")

	s.jsonResponse(w, map[string]interface{}{
		"job_id":      jobID,
		"agent_id":    req.AgentID,
		"target_host": req.TargetHost,
		"status":      "pending",
		"message":     "Provisioning job created",
	})
}

// handleGetProvisionStatus gets the status of a provisioning job.
func (s *MasterServer) handleGetProvisionStatus(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx := r.Context()

	job, err := s.provisionService.GetJob(ctx, jobID)
	if err != nil {
		if services.IsNotFound(err) || strings.Contains(err.Error(), "not found") {
			s.jsonError(w, http.StatusNotFound, "Provision job not found")
			return
		}
		s.logger.Error("Failed to get provision job", zap.Error(err), zap.String("job_id", jobID))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.jsonResponse(w, map[string]interface{}{
		"id":            job.ID,
		"target_host":   job.TargetHost,
		"target_port":   job.TargetPort,
		"target_user":   job.TargetUser,
		"status":        job.Status,
		"stage":         job.Stage,
		"progress":      job.Progress,
		"error_message": job.ErrorMessage,
		"started_at":    job.StartedAt,
		"completed_at":  job.CompletedAt,
	})
}

// handleGetProvisionLogs gets the logs of a provisioning job.
func (s *MasterServer) handleGetProvisionLogs(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx := r.Context()

	// Verify job exists
	job, err := s.provisionService.GetJob(ctx, jobID)
	if err != nil {
		if services.IsNotFound(err) || strings.Contains(err.Error(), "not found") {
			s.jsonError(w, http.StatusNotFound, "Provision job not found")
			return
		}
		s.logger.Error("Failed to get provision job", zap.Error(err), zap.String("job_id", jobID))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// For now, return basic job info
	// In a full implementation, logs would be stored separately with streaming support
	s.jsonResponse(w, map[string]interface{}{
		"job_id": job.ID,
		"status": job.Status,
		"stage":  job.Stage,
		"logs":   []string{}, // TODO: Implement detailed log storage
	})
}

// handleProvisionJobsList lists provisioning jobs.
func (s *MasterServer) handleProvisionJobsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx := r.Context()

	// Check for host filter
	host := r.URL.Query().Get("host")

	if host != "" {
		p := parsePagination(r)
		result, err := s.provisionService.ListByHost(ctx, host, p)
		if err != nil {
			s.logger.Error("Failed to list provision jobs by host", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		s.jsonResponse(w, map[string]interface{}{
			"items":      result.Items,
			"totalCount": result.TotalCount,
			"limit":      result.Pagination.Limit,
			"offset":     result.Pagination.Offset,
		})
		return
	}

	// List pending jobs
	jobs, err := s.provisionService.ListPending(ctx)
	if err != nil {
		s.logger.Error("Failed to list pending provision jobs", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.jsonResponse(w, map[string]interface{}{
		"items": jobs,
		"count": len(jobs),
	})
}
