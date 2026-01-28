// Package server provides the HTTP server implementation.
package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/proto"
	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/BlackOrder/vcdeploy/internal/validation"
	"go.uber.org/zap"
)

// --- Agent Binaries API ---

// handleAgentBinaries handles GET /api/v1/binaries and POST /api/v1/binaries
func (s *MasterServer) handleAgentBinaries(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		// Read access: viewer role + read scope
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		binaries, err := s.db.ListAgentBinaries(ctx)
		if err != nil {
			s.logger.Error("Failed to list agent binaries", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		s.jsonResponse(w, binaries)

	case http.MethodPost:
		// Admin-only: uploading binaries
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		// Handle multipart form upload
		if err := r.ParseMultipartForm(100 << 20); err != nil { // 100MB max
			http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
			return
		}

		version := r.FormValue("version")
		osType := r.FormValue("os")
		arch := r.FormValue("arch")
		setAsCurrent := r.FormValue("set_current") == "true"

		if version == "" || osType == "" || arch == "" {
			http.Error(w, "version, os, and arch are required", http.StatusBadRequest)
			return
		}

		// Get the uploaded file
		file, header, err := r.FormFile("binary")
		if err != nil {
			http.Error(w, "binary file is required: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()

		// Create binaries directory if it doesn't exist
		sysCfg := config.MustGetSystemConfig()
		binDir := filepath.Join(sysCfg.Paths.DataDir, "binaries")
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			s.logger.Error("Failed to create binaries directory", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Generate unique filename
		filename := fmt.Sprintf("vcdeploy-agent-%s-%s-%s", version, osType, arch)
		destPath := filepath.Join(binDir, filename)

		// Create destination file
		dest, err := os.Create(destPath)
		if err != nil {
			s.logger.Error("Failed to create binary file", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Copy and calculate checksum
		hasher := sha256.New()
		multiWriter := io.MultiWriter(dest, hasher)
		size, err := io.Copy(multiWriter, file)
		dest.Close()
		if err != nil {
			os.Remove(destPath)
			s.logger.Error("Failed to save binary file", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		checksum := hex.EncodeToString(hasher.Sum(nil))

		// Make binary executable
		if err := os.Chmod(destPath, 0o755); err != nil {
			s.logger.Error("Failed to make binary executable", zap.Error(err))
		}

		// Create database record
		binary := &storage.AgentBinary{
			Version:        version,
			OS:             osType,
			Arch:           arch,
			Path:           destPath,
			ChecksumSHA256: checksum,
			SizeBytes:      size,
			UploadedAt:     time.Now(),
			IsCurrent:      setAsCurrent,
		}

		if err := s.db.CreateAgentBinary(ctx, binary); err != nil {
			os.Remove(destPath)
			s.logger.Error("Failed to create binary record", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// If set_current, mark this as the current binary
		if setAsCurrent {
			if err := s.db.SetCurrentAgentBinary(ctx, binary.ID); err != nil {
				s.logger.Error("Failed to set current binary", zap.Error(err))
			}
		}

		s.logAudit(r, "create", "agent_binary", fmt.Sprintf("Uploaded agent binary %s for %s/%s (size: %d, checksum: %s)",
			version, osType, arch, size, checksum[:16]+"..."), "success")

		s.logger.Info("Agent binary uploaded",
			zap.String("version", version),
			zap.String("os", osType),
			zap.String("arch", arch),
			zap.Int64("size", size),
			zap.String("filename", header.Filename),
		)

		w.WriteHeader(http.StatusCreated)
		s.jsonResponse(w, binary)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAgentBinary handles GET/DELETE /api/v1/binaries/{id}
func (s *MasterServer) handleAgentBinary(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/binaries/")
	parts := strings.Split(path, "/")
	idStr := parts[0]

	if idStr == "" {
		http.Error(w, "Binary ID required", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid binary ID", http.StatusBadRequest)
		return
	}

	// Handle sub-resources
	if len(parts) > 1 {
		switch parts[1] {
		case "download":
			s.handleAgentBinaryDownload(w, r, id)
			return
		case "current":
			s.handleSetCurrentBinary(w, r, id)
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		// Read access: viewer role + read scope
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		binary, err := s.db.GetAgentBinary(ctx, id)
		if err != nil {
			if services.IsNotFound(err) {
				http.Error(w, "Binary not found", http.StatusNotFound)
				return
			}
			s.logger.Error("Failed to get binary", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		s.jsonResponse(w, binary)

	case http.MethodDelete:
		// Admin-only: deleting binaries
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		// Get binary to find file path
		binary, err := s.db.GetAgentBinary(ctx, id)
		if err != nil {
			if services.IsNotFound(err) {
				http.Error(w, "Binary not found", http.StatusNotFound)
				return
			}
			s.logger.Error("Failed to get binary", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Delete file
		if binary.Path != "" {
			if err := os.Remove(binary.Path); err != nil && !os.IsNotExist(err) {
				s.logger.Warn("Failed to delete binary file", zap.String("path", binary.Path), zap.Error(err))
			}
		}

		// Delete database record
		if err := s.db.DeleteAgentBinary(ctx, id); err != nil {
			s.logger.Error("Failed to delete binary record", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		s.logAudit(r, "delete", "agent_binary", fmt.Sprintf("Deleted agent binary %s for %s/%s",
			binary.Version, binary.OS, binary.Arch), "success")

		s.jsonResponse(w, map[string]string{"status": "deleted"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAgentBinaryDownload handles GET /api/v1/binaries/{id}/download
func (s *MasterServer) handleAgentBinaryDownload(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// Read access is sufficient for download
	if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
		http.Error(w, msg, status)
		return
	}

	binary, err := s.db.GetAgentBinary(ctx, id)
	if err != nil {
		if services.IsNotFound(err) {
			http.Error(w, "Binary not found", http.StatusNotFound)
			return
		}
		s.logger.Error("Failed to get binary", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Open file
	file, err := os.Open(binary.Path)
	if err != nil {
		s.logger.Error("Failed to open binary file", zap.Error(err))
		http.Error(w, "Binary file not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	// Set headers
	filename := fmt.Sprintf("vcdeploy-agent-%s-%s-%s", binary.Version, binary.OS, binary.Arch)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(binary.SizeBytes, 10))
	w.Header().Set("X-Checksum-SHA256", binary.ChecksumSHA256)

	http.ServeContent(w, r, filename, binary.UploadedAt, file)
}

// handleSetCurrentBinary handles POST /api/v1/binaries/{id}/current
func (s *MasterServer) handleSetCurrentBinary(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Admin-only
	if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
		http.Error(w, msg, status)
		return
	}

	// Verify binary exists
	binary, err := s.db.GetAgentBinary(ctx, id)
	if err != nil {
		if services.IsNotFound(err) {
			http.Error(w, "Binary not found", http.StatusNotFound)
			return
		}
		s.logger.Error("Failed to get binary", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Set as current
	if err := s.db.SetCurrentAgentBinary(ctx, id); err != nil {
		s.logger.Error("Failed to set current binary", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	s.logAudit(r, "update", "agent_binary", fmt.Sprintf("Set agent binary %s for %s/%s as current",
		binary.Version, binary.OS, binary.Arch), "success")

	s.jsonResponse(w, map[string]string{"status": "current", "version": binary.Version})
}

// handleAgentBinaryLatest handles GET /api/v1/binaries/latest?os=xxx&arch=xxx
func (s *MasterServer) handleAgentBinaryLatest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Read access for checking latest version
	if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
		http.Error(w, msg, status)
		return
	}

	osType := r.URL.Query().Get("os")
	arch := r.URL.Query().Get("arch")

	if osType == "" || arch == "" {
		http.Error(w, "os and arch query parameters are required", http.StatusBadRequest)
		return
	}

	binary, err := s.db.GetCurrentAgentBinary(ctx, osType, arch)
	if err != nil {
		if services.IsNotFound(err) {
			http.Error(w, "No current binary found for this platform", http.StatusNotFound)
			return
		}
		s.logger.Error("Failed to get current binary", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, binary)
}

// --- Agent Update Configuration API ---

// handleAgentUpdateConfig handles GET/PUT /api/v1/agents/{id}/update-config
func (s *MasterServer) handleAgentUpdateConfig(w http.ResponseWriter, r *http.Request, agentID string) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		// Read access
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		agent, err := s.agentService.GetByID(ctx, agentID)
		if err != nil {
			s.logger.Error("Failed to get agent", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if agent == nil {
			http.Error(w, "Agent not found", http.StatusNotFound)
			return
		}

		config := map[string]interface{}{
			"update_policy":       agent.UpdatePolicy,
			"update_window_start": agent.UpdateWindowStart,
			"update_window_end":   agent.UpdateWindowEnd,
			"current_version":     agent.Version,
			"last_update_at":      agent.LastUpdateAt,
			"last_update_error":   agent.LastUpdateError,
		}
		s.jsonResponse(w, config)

	case http.MethodPut:
		// Write access
		if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		var req struct {
			UpdatePolicy      string `json:"update_policy"`
			UpdateWindowStart string `json:"update_window_start"`
			UpdateWindowEnd   string `json:"update_window_end"`
		}

		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Validate policy
		if req.UpdatePolicy != "" {
			switch req.UpdatePolicy {
			case storage.AgentUpdatePolicyImmediate, storage.AgentUpdatePolicyScheduled, storage.AgentUpdatePolicyManual:
				// Valid
			default:
				http.Error(w, "Invalid update_policy. Must be 'immediate', 'scheduled', or 'manual'", http.StatusBadRequest)
				return
			}
		}

		// Validate time windows if policy is scheduled
		if req.UpdatePolicy == storage.AgentUpdatePolicyScheduled {
			if req.UpdateWindowStart == "" || req.UpdateWindowEnd == "" {
				http.Error(w, "update_window_start and update_window_end are required for scheduled policy", http.StatusBadRequest)
				return
			}
			// Simple HH:MM validation
			for _, t := range []string{req.UpdateWindowStart, req.UpdateWindowEnd} {
				if len(t) != 5 || t[2] != ':' {
					http.Error(w, "Time windows must be in HH:MM format", http.StatusBadRequest)
					return
				}
			}
		}

		if err := s.db.UpdateAgentUpdatePolicy(ctx, agentID, req.UpdatePolicy, req.UpdateWindowStart, req.UpdateWindowEnd); err != nil {
			s.logger.Error("Failed to update agent update policy", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		s.logAudit(r, "update", "agent", fmt.Sprintf("Updated update policy for agent %s to %s", agentID, req.UpdatePolicy), "success")

		s.jsonResponse(w, map[string]string{"status": "updated"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAgentUpdateHistory handles GET /api/v1/agents/{id}/update-history
func (s *MasterServer) handleAgentUpdateHistory(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Read access
	if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
		http.Error(w, msg, status)
		return
	}

	// Parse pagination
	p := parsePaginationWithDefaults(r, 20)

	history, total, err := s.db.ListAgentUpdateHistory(ctx, agentID, p.Limit, p.Offset)
	if err != nil {
		s.logger.Error("Failed to get agent update history", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, map[string]interface{}{
		"items":  history,
		"total":  total,
		"limit":  p.Limit,
		"offset": p.Offset,
	})
}

// handleTriggerAgentUpdate handles POST /api/v1/agents/{id}/update
func (s *MasterServer) handleTriggerAgentUpdate(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Admin-only: triggering updates
	if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
		http.Error(w, msg, status)
		return
	}

	var req struct {
		Force bool `json:"force"`
	}
	if r.Body != nil && r.ContentLength > 0 {
		// Decode is optional - ignore errors for missing/invalid body
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req)
	}

	// Get agent
	agent, err := s.agentService.GetByID(ctx, agentID)
	if err != nil {
		if services.IsNotFound(err) {
			http.Error(w, "Agent not found", http.StatusNotFound)
			return
		}
		s.logger.Error("Failed to get agent", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if agent == nil {
		http.Error(w, "Agent not found", http.StatusNotFound)
		return
	}

	// Check agent has OS/arch info
	if agent.OS == "" || agent.Arch == "" {
		http.Error(w, "Agent has not reported its OS/arch yet", http.StatusBadRequest)
		return
	}

	// Get current binary for agent's platform
	binary, err := s.db.GetCurrentAgentBinary(ctx, agent.OS, agent.Arch)
	if err != nil {
		if services.IsNotFound(err) {
			http.Error(w, "No current binary available for agent's platform", http.StatusNotFound)
			return
		}
		s.logger.Error("Failed to get current binary", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Check if already up to date
	if agent.Version == binary.Version && !req.Force {
		s.jsonResponse(w, map[string]interface{}{
			"status":          "up_to_date",
			"current_version": agent.Version,
		})
		return
	}

	// Create update history record
	history := &storage.AgentUpdateHistory{
		AgentID:     agentID,
		FromVersion: agent.Version,
		ToVersion:   binary.Version,
		Status:      "pending",
		StartedAt:   time.Now(),
	}
	if err := s.db.CreateAgentUpdateHistory(ctx, history); err != nil {
		s.logger.Error("Failed to create update history", zap.Error(err))
		// Continue anyway - update history is not critical
	}

	// Build download URL using request host
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		host = fwdHost
	}
	downloadURL := fmt.Sprintf("%s://%s/api/v1/binaries/%d/download", scheme, host, binary.ID)

	// Try to send update command via gRPC if agent is connected
	updateDelivery := "heartbeat"
	if s.agentServer != nil && s.agentServer.IsAgentConnected(agentID) {
		updateCmd := &proto.UpdateCommand{
			Version:        binary.Version,
			DownloadUrl:    downloadURL,
			ChecksumSha256: binary.ChecksumSHA256,
			SizeBytes:      binary.SizeBytes,
			Force:          req.Force,
		}
		if err := s.agentServer.SendUpdateCommand(agentID, updateCmd); err != nil {
			s.logger.Warn("Failed to send update command via gRPC, will use heartbeat fallback",
				zap.String("agent_id", agentID),
				zap.Error(err))
		} else {
			updateDelivery = "grpc_push"
			s.logger.Info("Sent update command via gRPC",
				zap.String("agent_id", agentID),
				zap.String("version", binary.Version))
		}
	}

	s.logAudit(r, "trigger", "agent_update", fmt.Sprintf("Triggered update for agent %s from %s to %s (delivery: %s)",
		agentID, agent.Version, binary.Version, updateDelivery), "success")

	s.jsonResponse(w, map[string]interface{}{
		"status":       "pending",
		"from_version": agent.Version,
		"to_version":   binary.Version,
		"force":        req.Force,
		"update_id":    history.ID,
		"delivery":     updateDelivery,
	})
}

// handleAgentsNeedingUpdate handles GET /api/v1/agents/updates/pending
func (s *MasterServer) handleAgentsNeedingUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Read access
	if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
		http.Error(w, msg, status)
		return
	}

	agents, err := s.db.ListAgentsNeedingUpdate(ctx)
	if err != nil {
		s.logger.Error("Failed to list agents needing update", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Enhance with current version info
	type agentUpdate struct {
		*storage.Agent
		TargetVersion string `json:"target_version"`
	}

	results := make([]agentUpdate, 0, len(agents))
	for _, agent := range agents {
		binary, err := s.db.GetCurrentAgentBinary(ctx, agent.OS, agent.Arch)
		if err != nil {
			continue
		}
		results = append(results, agentUpdate{
			Agent:         agent,
			TargetVersion: binary.Version,
		})
	}

	s.jsonResponse(w, results)
}
