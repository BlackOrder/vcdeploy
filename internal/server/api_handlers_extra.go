// Package server provides HTTP and gRPC server implementations.
package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services"
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
			s.jsonError(w, status, msg)
			return
		}

		keys, err := s.hostKeyService.List(ctx)
		if err != nil {
			s.logger.Error("Failed to list host keys", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		// Apply pagination
		p := parsePagination(r)
		totalCount := len(keys)

		// Apply offset
		if p.Offset >= totalCount {
			keys = []*storage.SSHHostKey{}
		} else {
			keys = keys[p.Offset:]
			// Apply limit
			if p.Limit > 0 && p.Limit < len(keys) {
				keys = keys[:p.Limit]
			}
		}

		s.jsonResponse(w, map[string]interface{}{
			"items":      keys,
			"totalCount": totalCount,
			"limit":      p.Limit,
			"offset":     p.Offset,
		})

	case http.MethodPost:
		// Check admin access for creating host keys
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		var input struct {
			Hostname    string `json:"hostname"`
			Port        int    `json:"port"`
			KeyType     string `json:"keyType"`
			PublicKey   string `json:"publicKey"`
			Fingerprint string `json:"fingerprint"`
			Trusted     bool   `json:"trusted"`
		}

		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			s.jsonError(w, http.StatusBadRequest, "Invalid JSON body")
			return
		}

		if input.Hostname == "" {
			s.jsonError(w, http.StatusBadRequest, "hostname is required")
			return
		}
		if input.Port == 0 {
			input.Port = 22
		}
		if input.KeyType == "" {
			s.jsonError(w, http.StatusBadRequest, "key_type is required")
			return
		}
		if input.PublicKey == "" {
			s.jsonError(w, http.StatusBadRequest, "public_key is required")
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
			s.jsonError(w, http.StatusInternalServerError, "Failed to create host key")
			return
		}

		s.writeJSON(w, http.StatusCreated, key)

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleHostKey handles operations on a specific host key:
// GET /api/v1/host-keys/{id} - get details
// PUT /api/v1/host-keys/{id} - update trust status
// DELETE /api/v1/host-keys/{id} - delete
func (s *MasterServer) handleHostKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/host-keys/")
	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, "Invalid host key ID")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		// List all and find by ID (service doesn't have GetByID)
		keys, err := s.hostKeyService.List(ctx)
		if err != nil {
			s.logger.Error("Failed to list host keys", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
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
			s.jsonError(w, http.StatusNotFound, "Host key not found")
			return
		}

		s.jsonResponse(w, found)

	case http.MethodPut:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		var input struct {
			Trusted bool `json:"trusted"`
		}

		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			s.jsonError(w, http.StatusBadRequest, "Invalid JSON body")
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
			s.jsonError(w, http.StatusInternalServerError, "Failed to update host key")
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))

	case http.MethodDelete:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		if err := s.hostKeyService.Delete(ctx, id); err != nil {
			s.logger.Error("Failed to delete host key", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Failed to delete host key")
			return
		}

		w.WriteHeader(http.StatusNoContent)

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// --- SSH Jump Server API Handlers ---

// handleJumpServers handles GET /api/v1/jumpservers (list) and POST /api/v1/jumpservers (create).
func (s *MasterServer) handleJumpServers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		servers, err := s.store.ListJumpServers(ctx)
		if err != nil {
			s.logger.Error("Failed to list jump servers", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		// Apply pagination
		p := parsePagination(r)
		totalCount := len(servers)

		// Apply offset
		if p.Offset >= totalCount {
			servers = []*storage.SSHJumpServer{}
		} else {
			servers = servers[p.Offset:]
			// Apply limit
			if p.Limit > 0 && p.Limit < len(servers) {
				servers = servers[:p.Limit]
			}
		}

		s.jsonResponse(w, map[string]interface{}{
			"items":      servers,
			"totalCount": totalCount,
			"limit":      p.Limit,
			"offset":     p.Offset,
		})

	case http.MethodPost:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		var input struct {
			Name     string `json:"name"`
			Host     string `json:"host"`
			Port     int    `json:"port"`
			Username string `json:"username"`
			SSHKeyID *int64 `json:"sshKeyId,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			s.jsonError(w, http.StatusBadRequest, "Invalid JSON body")
			return
		}

		if input.Name == "" {
			s.jsonError(w, http.StatusBadRequest, "name is required")
			return
		}
		if input.Host == "" {
			s.jsonError(w, http.StatusBadRequest, "host is required")
			return
		}
		if input.Port == 0 {
			input.Port = 22
		}
		if input.Username == "" {
			s.jsonError(w, http.StatusBadRequest, "username is required")
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

		if err := s.store.CreateJumpServer(ctx, server); err != nil {
			s.logger.Error("Failed to create jump server", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Failed to create jump server")
			return
		}

		s.writeJSON(w, http.StatusCreated, server)

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleJumpServer handles operations on a specific jump server.
func (s *MasterServer) handleJumpServer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/jump-servers/")
	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, "Invalid jump server ID")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if msg, status, ok := s.enforcementMiddleware.CheckReadAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		server, err := s.store.GetJumpServer(ctx, id)
		if err != nil {
			if services.IsNotFound(err) {
				s.jsonError(w, http.StatusNotFound, "Jump server not found")
				return
			}
			s.logger.Error("Failed to get jump server", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		s.jsonResponse(w, server)

	case http.MethodPut:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		var input struct {
			Name     string `json:"name"`
			Host     string `json:"host"`
			Port     int    `json:"port"`
			Username string `json:"username"`
			SSHKeyID *int64 `json:"sshKeyId,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			s.jsonError(w, http.StatusBadRequest, "Invalid JSON body")
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

		if err := s.store.UpdateJumpServer(ctx, server); err != nil {
			s.logger.Error("Failed to update jump server", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Failed to update jump server")
			return
		}

		s.jsonResponse(w, server)

	case http.MethodDelete:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		if err := s.store.DeleteJumpServer(ctx, id); err != nil {
			s.logger.Error("Failed to delete jump server", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Failed to delete jump server")
			return
		}

		w.WriteHeader(http.StatusNoContent)

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// --- Blocked IPs API Handlers ---

// handleBlockedIPs handles GET /api/v1/blocked (list) and POST /api/v1/blocked (block IP).
func (s *MasterServer) handleBlockedIPs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		// M5 FIX: Use query params for pagination instead of hardcoded values
		limit := 100
		offset := 0
		if l := r.URL.Query().Get("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
				limit = parsed
			}
		}
		if o := r.URL.Query().Get("offset"); o != "" {
			if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
				offset = parsed
			}
		}

		blocked, total, err := s.store.ListBlockedIPs(ctx, limit, offset)
		if err != nil {
			s.logger.Error("Failed to list blocked IPs", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		// Return paginated response with total count
		response := map[string]interface{}{
			"items":  blocked,
			"total":  total,
			"limit":  limit,
			"offset": offset,
		}

		s.jsonResponse(w, response)

	case http.MethodPost:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		var input struct {
			IPAddress string `json:"ipAddress"`
			Reason    string `json:"reason"`
			Duration  string `json:"duration"` // e.g., "24h", "7d"
		}

		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			s.jsonError(w, http.StatusBadRequest, "Invalid JSON body")
			return
		}

		if input.IPAddress == "" {
			s.jsonError(w, http.StatusBadRequest, "ipAddress is required")
			return
		}

		duration := 24 * time.Hour // Default 24 hours
		if input.Duration != "" {
			d, err := time.ParseDuration(input.Duration)
			if err != nil {
				s.jsonError(w, http.StatusBadRequest, "Invalid duration format (use Go duration syntax like '24h', '7d')")
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

		if err := s.store.BlockIP(ctx, blocked); err != nil {
			s.logger.Error("Failed to block IP", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Failed to block IP")
			return
		}

		s.writeJSON(w, http.StatusCreated, blocked)

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleBlockedIP handles DELETE /api/v1/blocked/{ip} (unblock).
func (s *MasterServer) handleBlockedIP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract IP from path
	ip := strings.TrimPrefix(r.URL.Path, "/api/v1/blocked/")
	if ip == "" {
		s.jsonError(w, http.StatusBadRequest, "IP address required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		blocked, err := s.store.GetBlockedIP(ctx, ip)
		if err != nil {
			if services.IsNotFound(err) {
				s.jsonError(w, http.StatusNotFound, "IP not blocked")
				return
			}
			s.logger.Error("Failed to get blocked IP", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		s.jsonResponse(w, blocked)

	case http.MethodDelete:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		if err := s.store.UnblockIP(ctx, ip); err != nil {
			s.logger.Error("Failed to unblock IP", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Failed to unblock IP")
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"unblocked"}`))

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// --- Provision API Handlers ---

// handleProvisionJobs handles GET /api/v1/provision (list) and POST /api/v1/provision (create job).
func (s *MasterServer) handleProvisionJobs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		// List pending jobs (most common use case)
		jobs, err := s.provisionService.ListPending(ctx)
		if err != nil {
			s.logger.Error("Failed to list provision jobs", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		// Apply pagination
		p := parsePagination(r)
		totalCount := len(jobs)

		// Apply offset
		if p.Offset >= totalCount {
			jobs = []*storage.ProvisionJob{}
		} else {
			jobs = jobs[p.Offset:]
			// Apply limit
			if p.Limit > 0 && p.Limit < len(jobs) {
				jobs = jobs[:p.Limit]
			}
		}

		s.jsonResponse(w, map[string]interface{}{
			"items":      jobs,
			"totalCount": totalCount,
			"limit":      p.Limit,
			"offset":     p.Offset,
		})

	case http.MethodPost:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		var input struct {
			TargetHost    string `json:"targetHost"`
			TargetPort    int    `json:"targetPort"`
			TargetUser    string `json:"targetUser"`
			SSHKeyID      *int64 `json:"sshKeyId,omitempty"`
			AgentBinaryID *int64 `json:"agentBinaryId,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			s.jsonError(w, http.StatusBadRequest, "Invalid JSON body")
			return
		}

		if input.TargetHost == "" {
			s.jsonError(w, http.StatusBadRequest, "targetHost is required")
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
			s.jsonError(w, http.StatusInternalServerError, "Failed to create provision job")
			return
		}

		s.writeJSON(w, http.StatusCreated, job)

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleProvisionJob handles operations on a specific provision job.
func (s *MasterServer) handleProvisionJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract ID from path
	jobID := strings.TrimPrefix(r.URL.Path, "/api/v1/provision/")
	if jobID == "" {
		s.jsonError(w, http.StatusBadRequest, "Job ID required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		job, err := s.provisionService.GetJob(ctx, jobID)
		if err != nil {
			s.logger.Error("Failed to get provision job", zap.Error(err))
			s.jsonError(w, http.StatusNotFound, "Provision job not found")
			return
		}

		s.jsonResponse(w, job)

	case http.MethodDelete:
		if msg, status, ok := s.enforcementMiddleware.CheckAdminAccess(ctx); !ok {
			s.jsonError(w, status, msg)
			return
		}

		// Cancel the job (marks as cancelled rather than actual deletion)
		if err := s.provisionService.Cancel(ctx, jobID); err != nil {
			s.logger.Error("Failed to cancel provision job", zap.Error(err))
			s.jsonError(w, http.StatusInternalServerError, "Failed to cancel provision job")
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"cancelled"}`))

	default:
		s.jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}
