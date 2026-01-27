// Package server provides HTTP and gRPC server implementations.
package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// --- SSH Host Key API Handlers ---

// handleHostKeys handles GET /api/v1/hostkeys (list) and POST /api/v1/hostkeys (create).
func (s *MasterServer) handleHostKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		// Check read access
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		keys, err := s.hostKeyService.List(ctx)
		if err != nil {
			s.logger.Error("Failed to list host keys", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(keys); err != nil {
			s.logger.Error("Failed to encode host keys", zap.Error(err))
		}

	case http.MethodPost:
		// Check admin access for creating host keys
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		var input struct {
			Hostname    string `json:"hostname"`
			Port        int    `json:"port"`
			KeyType     string `json:"key_type"`
			PublicKey   string `json:"public_key"`
			Fingerprint string `json:"fingerprint"`
			Trusted     bool   `json:"trusted"`
		}

		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}

		if input.Hostname == "" {
			http.Error(w, "hostname is required", http.StatusBadRequest)
			return
		}
		if input.Port == 0 {
			input.Port = 22
		}
		if input.KeyType == "" {
			http.Error(w, "key_type is required", http.StatusBadRequest)
			return
		}
		if input.PublicKey == "" {
			http.Error(w, "public_key is required", http.StatusBadRequest)
			return
		}

		userID, _ := GetUserIDFromContext(ctx)
		addedBy := "system"
		if userID > 0 {
			if user, err := s.userService.GetByID(ctx, userID); err == nil {
				addedBy = user.Username
			}
		}

		key := &storage.SSHHostKey{
			Hostname:    input.Hostname,
			Port:        input.Port,
			KeyType:     input.KeyType,
			PublicKey:   input.PublicKey,
			Fingerprint: input.Fingerprint,
			Trusted:     input.Trusted,
			AddedBy:     addedBy,
			CreatedAt:   time.Now(),
		}

		if err := s.hostKeyService.Create(ctx, key); err != nil {
			s.logger.Error("Failed to create host key", zap.Error(err))
			http.Error(w, "Failed to create host key", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(key); err != nil {
			s.logger.Error("Failed to encode host key", zap.Error(err))
		}

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleHostKey handles operations on a specific host key:
// GET /api/v1/hostkeys/{id} - get details
// PUT /api/v1/hostkeys/{id} - update trust status
// DELETE /api/v1/hostkeys/{id} - delete
func (s *MasterServer) handleHostKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/hostkeys/")
	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		http.Error(w, "Invalid host key ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		// List all and find by ID (service doesn't have GetByID)
		keys, err := s.hostKeyService.List(ctx)
		if err != nil {
			s.logger.Error("Failed to list host keys", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		var found *storage.SSHHostKey
		for _, k := range keys {
			if k.ID == id {
				found = k
				break
			}
		}

		if found == nil {
			http.Error(w, "Host key not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(found); err != nil {
			s.logger.Error("Failed to encode host key", zap.Error(err))
		}

	case http.MethodPut:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		var input struct {
			Trusted bool `json:"trusted"`
		}

		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}

		userID, _ := GetUserIDFromContext(ctx)
		verifiedBy := "system"
		if userID > 0 {
			if user, err := s.userService.GetByID(ctx, userID); err == nil {
				verifiedBy = user.Username
			}
		}

		if err := s.hostKeyService.UpdateTrust(ctx, id, input.Trusted, verifiedBy); err != nil {
			s.logger.Error("Failed to update host key trust", zap.Error(err))
			http.Error(w, "Failed to update host key", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))

	case http.MethodDelete:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		if err := s.hostKeyService.Delete(ctx, id); err != nil {
			s.logger.Error("Failed to delete host key", zap.Error(err))
			http.Error(w, "Failed to delete host key", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"deleted"}`))

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- SSH Jump Server API Handlers ---

// handleJumpServers handles GET /api/v1/jumpservers (list) and POST /api/v1/jumpservers (create).
func (s *MasterServer) handleJumpServers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		servers, err := s.db.ListJumpServers(ctx)
		if err != nil {
			s.logger.Error("Failed to list jump servers", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(servers); err != nil {
			s.logger.Error("Failed to encode jump servers", zap.Error(err))
		}

	case http.MethodPost:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		var input struct {
			Name     string `json:"name"`
			Host     string `json:"host"`
			Port     int    `json:"port"`
			Username string `json:"username"`
			SSHKeyID *int64 `json:"ssh_key_id,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}

		if input.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		if input.Host == "" {
			http.Error(w, "host is required", http.StatusBadRequest)
			return
		}
		if input.Port == 0 {
			input.Port = 22
		}
		if input.Username == "" {
			http.Error(w, "username is required", http.StatusBadRequest)
			return
		}

		server := &storage.SSHJumpServer{
			Name:      input.Name,
			Host:      input.Host,
			Port:      input.Port,
			Username:  input.Username,
			SSHKeyID:  input.SSHKeyID,
			CreatedAt: time.Now(),
		}

		if err := s.db.CreateJumpServer(ctx, server); err != nil {
			s.logger.Error("Failed to create jump server", zap.Error(err))
			http.Error(w, "Failed to create jump server", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(server); err != nil {
			s.logger.Error("Failed to encode jump server", zap.Error(err))
		}

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleJumpServer handles operations on a specific jump server.
func (s *MasterServer) handleJumpServer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/jumpservers/")
	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		http.Error(w, "Invalid jump server ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		server, err := s.db.GetJumpServer(ctx, id)
		if err != nil {
			if err == storage.ErrNotFound {
				http.Error(w, "Jump server not found", http.StatusNotFound)
				return
			}
			s.logger.Error("Failed to get jump server", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(server); err != nil {
			s.logger.Error("Failed to encode jump server", zap.Error(err))
		}

	case http.MethodPut:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		var input struct {
			Name     string `json:"name"`
			Host     string `json:"host"`
			Port     int    `json:"port"`
			Username string `json:"username"`
			SSHKeyID *int64 `json:"ssh_key_id,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}

		server := &storage.SSHJumpServer{
			ID:       id,
			Name:     input.Name,
			Host:     input.Host,
			Port:     input.Port,
			Username: input.Username,
			SSHKeyID: input.SSHKeyID,
		}

		if err := s.db.UpdateJumpServer(ctx, server); err != nil {
			s.logger.Error("Failed to update jump server", zap.Error(err))
			http.Error(w, "Failed to update jump server", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(server); err != nil {
			s.logger.Error("Failed to encode jump server", zap.Error(err))
		}

	case http.MethodDelete:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		if err := s.db.DeleteJumpServer(ctx, id); err != nil {
			s.logger.Error("Failed to delete jump server", zap.Error(err))
			http.Error(w, "Failed to delete jump server", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"deleted"}`))

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- Blocked IPs API Handlers ---

// handleBlockedIPs handles GET /api/v1/blocked (list) and POST /api/v1/blocked (block IP).
func (s *MasterServer) handleBlockedIPs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		// Use pagination with reasonable defaults
		blocked, _, err := s.db.ListBlockedIPs(ctx, 100, 0)
		if err != nil {
			s.logger.Error("Failed to list blocked IPs", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(blocked); err != nil {
			s.logger.Error("Failed to encode blocked IPs", zap.Error(err))
		}

	case http.MethodPost:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		var input struct {
			IPAddress string `json:"ip_address"`
			Reason    string `json:"reason"`
			Duration  string `json:"duration"` // e.g., "24h", "7d"
		}

		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}

		if input.IPAddress == "" {
			http.Error(w, "ip_address is required", http.StatusBadRequest)
			return
		}

		duration := 24 * time.Hour // Default 24 hours
		if input.Duration != "" {
			d, err := time.ParseDuration(input.Duration)
			if err != nil {
				http.Error(w, "Invalid duration format (use Go duration syntax like '24h', '7d')", http.StatusBadRequest)
				return
			}
			duration = d
		}

		userID, _ := GetUserIDFromContext(ctx)
		blockedBy := "system"
		if userID > 0 {
			if user, err := s.userService.GetByID(ctx, userID); err == nil {
				blockedBy = user.Username
			}
		}

		blocked := &storage.BlockedIP{
			IPAddress: input.IPAddress,
			Reason:    input.Reason,
			BlockedAt: time.Now(),
			ExpiresAt: time.Now().Add(duration),
			BlockedBy: blockedBy,
		}

		if err := s.db.BlockIP(ctx, blocked); err != nil {
			s.logger.Error("Failed to block IP", zap.Error(err))
			http.Error(w, "Failed to block IP", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(blocked); err != nil {
			s.logger.Error("Failed to encode blocked IP", zap.Error(err))
		}

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleBlockedIP handles DELETE /api/v1/blocked/{ip} (unblock).
func (s *MasterServer) handleBlockedIP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract IP from path
	ip := strings.TrimPrefix(r.URL.Path, "/api/v1/blocked/")
	if ip == "" {
		http.Error(w, "IP address required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		blocked, err := s.db.GetBlockedIP(ctx, ip)
		if err != nil {
			if err == storage.ErrNotFound {
				http.Error(w, "IP not blocked", http.StatusNotFound)
				return
			}
			s.logger.Error("Failed to get blocked IP", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(blocked); err != nil {
			s.logger.Error("Failed to encode blocked IP", zap.Error(err))
		}

	case http.MethodDelete:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		if err := s.db.UnblockIP(ctx, ip); err != nil {
			s.logger.Error("Failed to unblock IP", zap.Error(err))
			http.Error(w, "Failed to unblock IP", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"unblocked"}`))

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- Provision API Handlers ---

// handleProvisionJobs handles GET /api/v1/provision (list) and POST /api/v1/provision (create job).
func (s *MasterServer) handleProvisionJobs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		// List pending jobs (most common use case)
		jobs, err := s.provisionService.ListPending(ctx)
		if err != nil {
			s.logger.Error("Failed to list provision jobs", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(jobs); err != nil {
			s.logger.Error("Failed to encode provision jobs", zap.Error(err))
		}

	case http.MethodPost:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		var input struct {
			TargetHost    string `json:"target_host"`
			TargetPort    int    `json:"target_port"`
			TargetUser    string `json:"target_user"`
			SSHKeyID      *int64 `json:"ssh_key_id,omitempty"`
			AgentBinaryID *int64 `json:"agent_binary_id,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}

		if input.TargetHost == "" {
			http.Error(w, "target_host is required", http.StatusBadRequest)
			return
		}
		if input.TargetPort == 0 {
			input.TargetPort = 22
		}
		if input.TargetUser == "" {
			input.TargetUser = "root"
		}

		job := &storage.ProvisionJob{
			TargetHost:    input.TargetHost,
			TargetPort:    input.TargetPort,
			TargetUser:    input.TargetUser,
			SSHKeyID:      input.SSHKeyID,
			AgentBinaryID: input.AgentBinaryID,
			Status:        "pending",
		}

		if err := s.provisionService.CreateJob(ctx, job); err != nil {
			s.logger.Error("Failed to create provision job", zap.Error(err))
			http.Error(w, "Failed to create provision job", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(job); err != nil {
			s.logger.Error("Failed to encode provision job", zap.Error(err))
		}

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleProvisionJob handles operations on a specific provision job.
func (s *MasterServer) handleProvisionJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract ID from path
	jobID := strings.TrimPrefix(r.URL.Path, "/api/v1/provision/")
	if jobID == "" {
		http.Error(w, "Job ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		job, err := s.provisionService.GetJob(ctx, jobID)
		if err != nil {
			s.logger.Error("Failed to get provision job", zap.Error(err))
			http.Error(w, "Provision job not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(job); err != nil {
			s.logger.Error("Failed to encode provision job", zap.Error(err))
		}

	case http.MethodDelete:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			http.Error(w, msg, status)
			return
		}

		// Cancel the job (marks as cancelled rather than actual deletion)
		if err := s.provisionService.Cancel(ctx, jobID); err != nil {
			s.logger.Error("Failed to cancel provision job", zap.Error(err))
			http.Error(w, "Failed to cancel provision job", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"cancelled"}`))

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}