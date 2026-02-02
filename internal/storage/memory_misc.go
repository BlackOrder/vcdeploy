package storage

import (
	"context"
	"time"
)

// --- SSHHostKey methods ---

// CreateSSHHostKey creates a new SSH host key record.
func (s *MemoryStore) CreateSSHHostKey(ctx context.Context, key *SSHHostKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for duplicate
	for _, k := range s.sshHostKeys {
		if k.Hostname == key.Hostname && k.Port == key.Port && k.KeyType == key.KeyType {
			return ErrDuplicate
		}
	}

	key.ID = nextID(&s.nextSSHHostKeyID)
	now := time.Now()
	key.CreatedAt = now
	key.UpdatedAt = now

	// Copy-on-store
	stored := *key
	s.sshHostKeys[key.ID] = &stored

	s.queueWrite(s.provisionWrites, NewWriteOp(WriteOpInsert, "ssh_host_keys", &stored))
	return nil
}

// GetSSHHostKey retrieves an SSH host key by hostname, port, and key type.
func (s *MemoryStore) GetSSHHostKey(ctx context.Context, hostname string, port int, keyType string) (*SSHHostKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, k := range s.sshHostKeys {
		if k.Hostname == hostname && k.Port == port && k.KeyType == keyType {
			result := *k
			return &result, nil
		}
	}
	return nil, ErrNotFound
}

// GetSSHHostKeysByHost retrieves all SSH host keys for a host/port combination.
func (s *MemoryStore) GetSSHHostKeysByHost(ctx context.Context, hostname string, port int) ([]*SSHHostKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*SSHHostKey
	for _, k := range s.sshHostKeys {
		if k.Hostname == hostname && k.Port == port {
			cp := *k
			result = append(result, &cp)
		}
	}
	return result, nil
}

// ListSSHHostKeys returns all SSH host keys.
func (s *MemoryStore) ListSSHHostKeys(ctx context.Context) ([]*SSHHostKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*SSHHostKey, 0, len(s.sshHostKeys))
	for _, k := range s.sshHostKeys {
		cp := *k
		result = append(result, &cp)
	}
	return result, nil
}

// UpdateSSHHostKeyTrust updates the trust status of an SSH host key.
func (s *MemoryStore) UpdateSSHHostKeyTrust(ctx context.Context, id int64, trusted bool, verifiedBy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key, ok := s.sshHostKeys[id]
	if !ok {
		return ErrNotFound
	}

	key.Trusted = trusted
	now := time.Now()
	key.VerifiedAt = &now
	key.UpdatedAt = now
	key.AddedBy = verifiedBy

	s.queueWrite(s.provisionWrites, NewWriteOp(WriteOpUpdate, "ssh_host_keys", key))
	return nil
}

// DeleteSSHHostKey removes an SSH host key by ID.
func (s *MemoryStore) DeleteSSHHostKey(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sshHostKeys[id]; !ok {
		return ErrNotFound
	}

	delete(s.sshHostKeys, id)
	s.queueWrite(s.provisionWrites, NewWriteOp(WriteOpDelete, "ssh_host_keys", id))
	return nil
}

// DeleteSSHHostKeysByHost removes all SSH host keys for a host/port combination.
func (s *MemoryStore) DeleteSSHHostKeysByHost(ctx context.Context, hostname string, port int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var count int64
	for id, k := range s.sshHostKeys {
		if k.Hostname == hostname && k.Port == port {
			delete(s.sshHostKeys, id)
			s.queueWrite(s.provisionWrites, NewWriteOp(WriteOpDelete, "ssh_host_keys", id))
			count++
		}
	}
	return count, nil
}

// --- SSHJumpServer methods ---

// CreateJumpServer creates a new jump server record.
func (s *MemoryStore) CreateJumpServer(ctx context.Context, js *SSHJumpServer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for duplicate name
	for _, j := range s.jumpServers {
		if j.Name == js.Name {
			return ErrDuplicate
		}
	}

	js.ID = nextID(&s.nextJumpServerID)
	js.CreatedAt = time.Now()

	// Copy-on-store
	stored := *js
	s.jumpServers[js.ID] = &stored

	s.queueWrite(s.provisionWrites, NewWriteOp(WriteOpInsert, "ssh_jump_servers", &stored))
	return nil
}

// GetJumpServer retrieves a jump server by ID.
func (s *MemoryStore) GetJumpServer(ctx context.Context, id int64) (*SSHJumpServer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	js, ok := s.jumpServers[id]
	if !ok {
		return nil, ErrNotFound
	}

	result := *js
	return &result, nil
}

// GetJumpServerByName retrieves a jump server by name.
func (s *MemoryStore) GetJumpServerByName(ctx context.Context, name string) (*SSHJumpServer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, js := range s.jumpServers {
		if js.Name == name {
			result := *js
			return &result, nil
		}
	}
	return nil, ErrNotFound
}

// ListJumpServers returns all jump servers.
func (s *MemoryStore) ListJumpServers(ctx context.Context) ([]*SSHJumpServer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*SSHJumpServer, 0, len(s.jumpServers))
	for _, js := range s.jumpServers {
		cp := *js
		result = append(result, &cp)
	}
	return result, nil
}

// UpdateJumpServer updates a jump server.
func (s *MemoryStore) UpdateJumpServer(ctx context.Context, js *SSHJumpServer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.jumpServers[js.ID]
	if !ok {
		return ErrNotFound
	}

	existing.Name = js.Name
	existing.Host = js.Host
	existing.Port = js.Port
	existing.Username = js.Username
	existing.SSHKeyID = js.SSHKeyID

	s.queueWrite(s.provisionWrites, NewWriteOp(WriteOpUpdate, "ssh_jump_servers", existing))
	return nil
}

// DeleteJumpServer removes a jump server by ID.
func (s *MemoryStore) DeleteJumpServer(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.jumpServers[id]; !ok {
		return ErrNotFound
	}

	delete(s.jumpServers, id)
	s.queueWrite(s.provisionWrites, NewWriteOp(WriteOpDelete, "ssh_jump_servers", id))
	return nil
}

// --- ProvisionJob methods ---

// CreateProvisionJob creates a new provision job.
func (s *MemoryStore) CreateProvisionJob(ctx context.Context, job *ProvisionJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.provisionJobs[job.ID]; exists {
		return ErrDuplicate
	}

	if job.StartedAt.IsZero() {
		job.StartedAt = time.Now()
	}
	if job.Status == "" {
		job.Status = "pending"
	}

	// Copy-on-store
	stored := *job
	s.provisionJobs[job.ID] = &stored

	s.queueWrite(s.provisionWrites, NewWriteOp(WriteOpInsert, "provision_jobs", &stored))
	return nil
}

// GetProvisionJob retrieves a provision job by ID.
func (s *MemoryStore) GetProvisionJob(ctx context.Context, id string) (*ProvisionJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	job, ok := s.provisionJobs[id]
	if !ok {
		return nil, ErrNotFound
	}

	result := *job
	return &result, nil
}

// UpdateProvisionJobStatus updates the status of a provision job.
func (s *MemoryStore) UpdateProvisionJobStatus(ctx context.Context, id, status, stage, errorMessage string, progress int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.provisionJobs[id]
	if !ok {
		return ErrNotFound
	}

	job.Status = status
	job.Stage = stage
	job.ErrorMessage = errorMessage
	job.Progress = progress

	if status == "completed" || status == "failed" {
		now := time.Now()
		job.CompletedAt = &now
	}

	s.queueWrite(s.provisionWrites, NewWriteOp(WriteOpUpdate, "provision_jobs", job))
	return nil
}

// ListPendingProvisionJobs returns all pending provision jobs.
func (s *MemoryStore) ListPendingProvisionJobs(ctx context.Context) ([]*ProvisionJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*ProvisionJob
	for _, job := range s.provisionJobs {
		if job.Status == "pending" || job.Status == "in_progress" {
			cp := *job
			result = append(result, &cp)
		}
	}
	return result, nil
}

// ListProvisionJobsByHost returns provision jobs for a host with pagination.
func (s *MemoryStore) ListProvisionJobsByHost(ctx context.Context, host string, limit, offset int) ([]*ProvisionJob, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Collect all jobs for host
	var all []*ProvisionJob
	for _, job := range s.provisionJobs {
		if job.TargetHost == host {
			cp := *job
			all = append(all, &cp)
		}
	}

	// Sort by StartedAt descending (newest first)
	for i := 0; i < len(all)-1; i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].StartedAt.After(all[i].StartedAt) {
				all[i], all[j] = all[j], all[i]
			}
		}
	}

	total := int64(len(all))

	// Apply pagination
	if offset >= len(all) {
		return []*ProvisionJob{}, total, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}

	return all[offset:end], total, nil
}

// --- ProvisionLog methods ---

// SaveProvisionLog saves a log entry for a provisioning job.
func (s *MemoryStore) SaveProvisionLog(ctx context.Context, jobID, level, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	log := &ProvisionLog{
		ID:        int64(len(s.provisionLogs[jobID]) + 1),
		JobID:     jobID,
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
	}

	s.provisionLogs[jobID] = append(s.provisionLogs[jobID], log)
	return nil
}

// GetProvisionLogs retrieves all logs for a provisioning job.
func (s *MemoryStore) GetProvisionLogs(ctx context.Context, jobID string) ([]*ProvisionLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	logs := s.provisionLogs[jobID]
	if logs == nil {
		return []*ProvisionLog{}, nil
	}

	// Copy-on-read
	result := make([]*ProvisionLog, len(logs))
	for i, log := range logs {
		cp := *log
		result[i] = &cp
	}
	return result, nil
}

// --- HealthCheckConfig methods ---

// CreateHealthCheckConfig creates a new health check configuration.
func (s *MemoryStore) CreateHealthCheckConfig(ctx context.Context, config *HealthCheckConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	config.ID = nextID(&s.nextHealthCheckID)
	now := time.Now()
	config.CreatedAt = now
	config.UpdatedAt = now

	// Copy-on-store
	stored := *config
	s.healthCheckConfigs[config.ID] = &stored

	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpInsert, "health_check_configs", &stored))
	return nil
}

// GetHealthCheckConfig retrieves a health check configuration by ID.
func (s *MemoryStore) GetHealthCheckConfig(ctx context.Context, id int64) (*HealthCheckConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	config, ok := s.healthCheckConfigs[id]
	if !ok {
		return nil, ErrNotFound
	}

	result := *config
	return &result, nil
}

// GetGlobalHealthCheckConfig retrieves the global health check configuration.
func (s *MemoryStore) GetGlobalHealthCheckConfig(ctx context.Context) (*HealthCheckConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, config := range s.healthCheckConfigs {
		if config.IsGlobal {
			result := *config
			return &result, nil
		}
	}
	return nil, ErrNotFound
}

// GetHealthCheckConfigForProject retrieves the health check configuration for a project.
func (s *MemoryStore) GetHealthCheckConfigForProject(ctx context.Context, projectID int64) (*HealthCheckConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, config := range s.healthCheckConfigs {
		if config.ProjectID != nil && *config.ProjectID == projectID {
			result := *config
			return &result, nil
		}
	}
	return nil, ErrNotFound
}

// UpdateHealthCheckConfig updates a health check configuration.
func (s *MemoryStore) UpdateHealthCheckConfig(ctx context.Context, config *HealthCheckConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.healthCheckConfigs[config.ID]
	if !ok {
		return ErrNotFound
	}

	existing.Name = config.Name
	existing.URL = config.URL
	existing.Method = config.Method
	existing.ExpectedStatus = config.ExpectedStatus
	existing.TimeoutSeconds = config.TimeoutSeconds
	existing.Retries = config.Retries
	existing.RetryDelaySeconds = config.RetryDelaySeconds
	existing.Headers = config.Headers
	existing.Body = config.Body
	existing.BodyContains = config.BodyContains
	existing.Enabled = config.Enabled
	existing.UpdatedAt = time.Now()

	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpUpdate, "health_check_configs", existing))
	return nil
}

// ListHealthCheckConfigs returns all health check configurations.
func (s *MemoryStore) ListHealthCheckConfigs(ctx context.Context) ([]*HealthCheckConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*HealthCheckConfig, 0, len(s.healthCheckConfigs))
	for _, config := range s.healthCheckConfigs {
		cp := *config
		result = append(result, &cp)
	}
	return result, nil
}

// DeleteHealthCheckConfig removes a health check configuration by ID.
func (s *MemoryStore) DeleteHealthCheckConfig(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.healthCheckConfigs[id]; !ok {
		return ErrNotFound
	}

	delete(s.healthCheckConfigs, id)
	s.queueWrite(s.coreWrites, NewWriteOp(WriteOpDelete, "health_check_configs", id))
	return nil
}

// --- HasSettings method ---

// HasSettings checks if any settings exist.
func (s *MemoryStore) HasSettings(ctx context.Context) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.settings) > 0, nil
}
