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
			s.jsonError(w, status, msg)
			return
		}

		binaries, err := s.store.ListAgentBinaries(ctx)
		if err != nil {
			s.logger.Error("Failed to list agent binaries", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}

		// Apply pagination
		p := parsePagination(r)
		totalCount := len(binaries)

		// Apply offset
		if p.Offset >= totalCount {
			binaries = []*storage.AgentBinary{}
		} else {
			binaries = binaries[p.Offset:]
			// Apply limit
			if p.Limit > 0 && p.Limit < len(binaries) {
				binaries = binaries[:p.Limit]
			}
		}

		s.jsonResponse(w, PaginatedResponse{
			Items:      binaries,
			TotalCount: int64(totalCount),
			Limit:      p.Limit,
			Offset:     p.Offset,
		})

	case http.MethodPost:
		// Admin-only: uploading binaries
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		// Handle multipart form upload
		if err := r.ParseMultipartForm(100 << 20); err != nil { // 100MB max
			s.jsonError(w, http.StatusBadRequest, "failed to parse form: "+err.Error())
			return
		}

		version := r.FormValue("version")
		osType := r.FormValue("os")
		arch := r.FormValue("arch")
		setAsCurrent := r.FormValue("set_current") == "true"

		if version == "" || osType == "" || arch == "" {
			s.jsonError(w, http.StatusBadRequest, "version, os, and arch are required")
			return
		}

		// Path traversal validation (C4 security fix)
		if err := validation.ValidateBinaryPathComponent(version); err != nil {
			s.jsonError(w, http.StatusBadRequest, "invalid version: "+err.Error())
			return
		}
		if err := validation.ValidateBinaryPathComponent(osType); err != nil {
			s.jsonError(w, http.StatusBadRequest, "invalid os: "+err.Error())
			return
		}
		if err := validation.ValidateBinaryPathComponent(arch); err != nil {
			s.jsonError(w, http.StatusBadRequest, "invalid arch: "+err.Error())
			return
		}

		// Whitelist validation for os and arch
		validOS := map[string]bool{"linux": true, "darwin": true, "windows": true, "freebsd": true}
		validArch := map[string]bool{"amd64": true, "arm64": true, "arm": true, "386": true}
		if !validOS[osType] {
			s.jsonError(w, http.StatusBadRequest, "unsupported os: must be one of linux, darwin, windows, freebsd")
			return
		}
		if !validArch[arch] {
			s.jsonError(w, http.StatusBadRequest, "unsupported arch: must be one of amd64, arm64, arm, 386")
			return
		}

		// Get the uploaded file
		file, header, err := r.FormFile("binary")
		if err != nil {
			s.jsonError(w, http.StatusBadRequest, "binary file is required: "+err.Error())
			return
		}
		defer file.Close()

		// Create binaries directory if it doesn't exist
		sysCfg, err := config.GetSystemConfig()
		if err != nil {
			s.logger.Error("Failed to load system config", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		binDir := filepath.Join(sysCfg.Paths.DataDir, "binaries")
		// #nosec G301 - Binary directory needs world-execute for agent binary execution
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			s.logger.Error("Failed to create binaries directory", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}

		// Generate unique filename
		filename := fmt.Sprintf("vcdeploy-agent-%s-%s-%s", version, osType, arch)
		destPath := filepath.Join(binDir, filename)

		// Create destination file
		dest, err := os.Create(destPath) // #nosec G304 - destPath is constructed from server-controlled binDir and validated version/os/arch
		if err != nil {
			s.logger.Error("Failed to create binary file", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}

		// Copy and calculate checksum
		hasher := sha256.New()
		multiWriter := io.MultiWriter(dest, hasher)
		size, err := io.Copy(multiWriter, file)
		_ = dest.Close() // #nosec G104 - best effort close after write
		if err != nil {
			_ = os.Remove(destPath) // #nosec G104 - best effort cleanup
			s.logger.Error("Failed to save binary file", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}

		checksum := hex.EncodeToString(hasher.Sum(nil))

		// Make binary executable
		// #nosec G302 - Binary needs execute permission
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

		if err := s.store.CreateAgentBinary(ctx, binary); err != nil {
			_ = os.Remove(destPath) // #nosec G104 - best effort cleanup
			s.logger.Error("Failed to create binary record", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}

		// If set_current, mark this as the current binary
		if setAsCurrent {
			if err := s.store.SetCurrentAgentBinary(ctx, binary.ID); err != nil {
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
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleAgentBinary handles GET/DELETE /api/v1/binaries/{id}
func (s *MasterServer) handleAgentBinary(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/binaries/")
	parts := strings.Split(path, "/")
	idStr := parts[0]

	if idStr == "" {
		s.jsonError(w, http.StatusBadRequest, "binary ID required")
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid binary ID")
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

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		// Read access: viewer role + read scope
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		binary, err := s.store.GetAgentBinary(ctx, id)
		if err != nil {
			if services.IsNotFound(err) {
				s.jsonError(w, http.StatusNotFound, "binary not found")
				return
			}
			s.logger.Error("Failed to get binary", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		s.jsonResponse(w, binary)

	case http.MethodDelete:
		// Admin-only: deleting binaries
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		// Get binary to find file path
		binary, err := s.store.GetAgentBinary(ctx, id)
		if err != nil {
			if services.IsNotFound(err) {
				s.jsonError(w, http.StatusNotFound, "binary not found")
				return
			}
			s.logger.Error("Failed to get binary", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}

		// Delete file
		if binary.Path != "" {
			if err := os.Remove(binary.Path); err != nil && !os.IsNotExist(err) {
				s.logger.Warn("Failed to delete binary file", zap.String("path", binary.Path), zap.Error(err))
			}
		}

		// Delete database record
		if err := s.store.DeleteAgentBinary(ctx, id); err != nil {
			s.logger.Error("Failed to delete binary record", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}

		s.logAudit(r, "delete", "agent_binary", fmt.Sprintf("Deleted agent binary %s for %s/%s",
			binary.Version, binary.OS, binary.Arch), "success")

		w.WriteHeader(http.StatusNoContent)

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleAgentBinaryDownload handles GET /api/v1/binaries/{id}/download
func (s *MasterServer) handleAgentBinaryDownload(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// Read access is sufficient for download
	if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	binary, err := s.store.GetAgentBinary(ctx, id)
	if err != nil {
		if services.IsNotFound(err) {
			s.jsonError(w, http.StatusNotFound, "binary not found")
			return
		}
		s.logger.Error("Failed to get binary", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Open file
	file, err := os.Open(binary.Path)
	if err != nil {
		s.logger.Error("Failed to open binary file", zap.Error(err))
		s.jsonError(w, http.StatusNotFound, "binary file not found")
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
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	// Admin-only
	if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	// Verify binary exists
	binary, err := s.store.GetAgentBinary(ctx, id)
	if err != nil {
		if services.IsNotFound(err) {
			s.jsonError(w, http.StatusNotFound, "binary not found")
			return
		}
		s.logger.Error("Failed to get binary", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Set as current
	if err := s.store.SetCurrentAgentBinary(ctx, id); err != nil {
		s.logger.Error("Failed to set current binary", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	s.logAudit(r, "update", "agent_binary", fmt.Sprintf("Set agent binary %s for %s/%s as current",
		binary.Version, binary.OS, binary.Arch), "success")

	s.jsonResponse(w, map[string]string{"status": "current", "version": binary.Version})
}

// handleAgentBinaryLatest handles GET /api/v1/binaries/latest?os=xxx&arch=xxx
func (s *MasterServer) handleAgentBinaryLatest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	// Read access for checking latest version
	if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	osType := r.URL.Query().Get("os")
	arch := r.URL.Query().Get("arch")

	if osType == "" || arch == "" {
		s.jsonError(w, http.StatusBadRequest, "os and arch query parameters are required")
		return
	}

	binary, err := s.store.GetCurrentAgentBinary(ctx, osType, arch)
	if err != nil {
		if services.IsNotFound(err) {
			s.jsonError(w, http.StatusNotFound, "no current binary found for this platform")
			return
		}
		s.logger.Error("Failed to get current binary", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	s.jsonResponse(w, binary)
}

// --- Agent Update Configuration API ---

// handleAgentUpdateConfig handles GET/PUT /api/v1/agents/{id}/update-config
func (s *MasterServer) handleAgentUpdateConfig(w http.ResponseWriter, r *http.Request, agentID string) {
	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		// Read access
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		agent, err := s.agentService.GetByID(ctx, agentID)
		if err != nil {
			s.logger.Error("Failed to get agent", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		if agent == nil {
			s.jsonError(w, http.StatusNotFound, "agent not found")
			return
		}

		config := map[string]interface{}{
			"updatePolicy":      agent.UpdatePolicy,
			"updateWindowStart": agent.UpdateWindowStart,
			"updateWindowEnd":   agent.UpdateWindowEnd,
			"currentVersion":    agent.Version,
			"lastUpdateAt":      agent.LastUpdateAt,
			"lastUpdateError":   agent.LastUpdateError,
		}
		s.jsonResponse(w, config)

	case http.MethodPut:
		// Write access
		if msg, status, ok := s.enforcementMiddleware.CheckWriteAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		var req struct {
			UpdatePolicy      string `json:"updatePolicy"`
			UpdateWindowStart string `json:"updateWindowStart"`
			UpdateWindowEnd   string `json:"updateWindowEnd"`
		}

		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
			s.jsonError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		// Validate policy
		if req.UpdatePolicy != "" {
			switch req.UpdatePolicy {
			case storage.AgentUpdatePolicyImmediate, storage.AgentUpdatePolicyScheduled, storage.AgentUpdatePolicyManual:
				// Valid
			default:
				s.jsonError(w, http.StatusBadRequest, "invalid updatePolicy. Must be 'immediate', 'scheduled', or 'manual'")
				return
			}
		}

		// Validate time windows if policy is scheduled
		if req.UpdatePolicy == storage.AgentUpdatePolicyScheduled {
			if req.UpdateWindowStart == "" || req.UpdateWindowEnd == "" {
				s.jsonError(w, http.StatusBadRequest, "update_window_start and update_window_end are required for scheduled policy")
				return
			}
			// Simple HH:MM validation
			for _, t := range []string{req.UpdateWindowStart, req.UpdateWindowEnd} {
				if len(t) != 5 || t[2] != ':' {
					s.jsonError(w, http.StatusBadRequest, "time windows must be in HH:MM format")
					return
				}
			}
		}

		if err := s.store.UpdateAgentUpdatePolicy(ctx, agentID, req.UpdatePolicy, req.UpdateWindowStart, req.UpdateWindowEnd); err != nil {
			s.logger.Error("Failed to update agent update policy", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}

		s.logAudit(r, "update", "agent", fmt.Sprintf("Updated update policy for agent %s to %s", agentID, req.UpdatePolicy), "success")

		s.jsonResponse(w, map[string]string{"status": "updated"})

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleAgentUpdateHistory handles GET /api/v1/agents/{id}/update-history
func (s *MasterServer) handleAgentUpdateHistory(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	// Read access
	if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	// Parse pagination
	p := parsePaginationWithDefaults(r, 20)

	history, total, err := s.store.ListAgentUpdateHistory(ctx, agentID, p.Limit, p.Offset)
	if err != nil {
		s.logger.Error("Failed to get agent update history", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	s.jsonResponse(w, RollbackListResponse{
		Items:      history,
		TotalCount: total,
		Limit:      p.Limit,
		Offset:     p.Offset,
	})
}

// handleTriggerAgentUpdate handles POST /api/v1/agents/{id}/update
func (s *MasterServer) handleTriggerAgentUpdate(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodPost {
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	// Admin-only: triggering updates
	if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	var req struct {
		Force bool `json:"force"`
	}
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validation.DefaultMaxBodySize)).Decode(&req); err != nil {
			s.logger.Debug("Failed to decode request body, using defaults", zap.Error(err))
			// Continue with default values - Force=false
		}
	}

	// Get agent
	agent, err := s.agentService.GetByID(ctx, agentID)
	if err != nil {
		if services.IsNotFound(err) {
			s.jsonError(w, http.StatusNotFound, "agent not found")
			return
		}
		s.logger.Error("Failed to get agent", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if agent == nil {
		s.jsonError(w, http.StatusNotFound, "agent not found")
		return
	}

	// Check agent has OS/arch info
	if agent.OS == "" || agent.Arch == "" {
		s.jsonError(w, http.StatusBadRequest, "agent has not reported its OS/arch yet")
		return
	}

	// Get current binary for agent's platform
	binary, err := s.store.GetCurrentAgentBinary(ctx, agent.OS, agent.Arch)
	if err != nil {
		if services.IsNotFound(err) {
			s.jsonError(w, http.StatusNotFound, "no current binary available for agent's platform")
			return
		}
		s.logger.Error("Failed to get current binary", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Check if already up to date
	if agent.Version == binary.Version && !req.Force {
		s.jsonResponse(w, AgentUpToDateResponse{
			Status:         "up_to_date",
			CurrentVersion: agent.Version,
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
	if err := s.store.CreateAgentUpdateHistory(ctx, history); err != nil {
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

	s.jsonResponse(w, AgentUpdateTriggerResponse{
		Status:      "pending",
		FromVersion: agent.Version,
		ToVersion:   binary.Version,
		Force:       req.Force,
		UpdateID:    history.ID,
		Delivery:    updateDelivery,
	})
}

// handleAgentsNeedingUpdate handles GET /api/v1/agents/updates/pending
func (s *MasterServer) handleAgentsNeedingUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	// Read access
	if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	agents, err := s.store.ListAgentsNeedingUpdate(ctx)
	if err != nil {
		s.logger.Error("Failed to list agents needing update", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Enhance with current version info
	type agentUpdate struct {
		*storage.Agent
		TargetVersion string `json:"targetVersion"`
	}

	results := make([]agentUpdate, 0, len(agents))
	for _, agent := range agents {
		binary, err := s.store.GetCurrentAgentBinary(ctx, agent.OS, agent.Arch)
		if err != nil {
			continue
		}
		results = append(results, agentUpdate{
			Agent:         agent,
			TargetVersion: binary.Version,
		})
	}

	// Apply pagination
	p := parsePagination(r)
	totalCount := len(results)

	// Apply offset
	if p.Offset >= totalCount {
		results = []agentUpdate{}
	} else {
		results = results[p.Offset:]
		// Apply limit
		if p.Limit > 0 && p.Limit < len(results) {
			results = results[:p.Limit]
		}
	}

	s.jsonResponse(w, PaginatedResponse{
		Items:      results,
		TotalCount: int64(totalCount),
		Limit:      p.Limit,
		Offset:     p.Offset,
	})
}

// handleAllAgentUpdateHistory handles GET /api/v1/agents/updates/history
func (s *MasterServer) handleAllAgentUpdateHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), TimeoutDefault)
	defer cancel()

	// Read access
	if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
		s.jsonError(w, status, msg)
		return
	}

	// Parse pagination
	p := parsePaginationWithDefaults(r, 20)

	history, total, err := s.store.ListAllAgentUpdateHistory(ctx, p.Limit, p.Offset)
	if err != nil {
		s.logger.Error("Failed to get all agent update history", zap.Error(err))
		s.jsonError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	s.jsonResponse(w, RollbackListResponse{
		Items:      history,
		TotalCount: total,
		Limit:      p.Limit,
		Offset:     p.Offset,
	})
}
