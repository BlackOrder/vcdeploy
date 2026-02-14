package storage

import (
	"context"
	"time"

	"github.com/rs/xid"
)

// --- DeploymentRecord methods ---

// CreateDeployment creates a new deployment record.
func (s *MemoryStore) CreateDeployment(ctx context.Context, d *DeploymentRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for duplicate ID
	if _, exists := s.deployments[d.ID]; exists {
		return ErrDuplicate
	}

	if d.StartedAt.IsZero() {
		d.StartedAt = time.Now()
	}
	if d.Status == "" {
		d.Status = "pending"
	}

	// Copy-on-store
	stored := *d
	s.deployments[d.ID] = &stored

	s.queueWrite(s.deploymentsWrites, NewWriteOp(WriteOpInsert, "deployments", &stored))
	return nil
}

// GetDeployment retrieves a deployment by ID.
func (s *MemoryStore) GetDeployment(ctx context.Context, id string) (*DeploymentRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	d, ok := s.deployments[id]
	if !ok {
		return nil, ErrNotFound
	}

	result := *d
	return &result, nil
}

// UpdateDeployment updates a deployment record.
func (s *MemoryStore) UpdateDeployment(ctx context.Context, d *DeploymentRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.deployments[d.ID]
	if !ok {
		return ErrNotFound
	}

	existing.Status = d.Status
	existing.CompletedAt = d.CompletedAt
	existing.ErrorMessage = d.ErrorMessage
	existing.CommitHash = d.CommitHash
	existing.ReleaseNumber = d.ReleaseNumber

	s.queueWrite(s.deploymentsWrites, NewWriteOp(WriteOpUpdate, "deployments", existing))
	return nil
}

// ListDeploymentsRecent returns the most recent deployments.
func (s *MemoryStore) ListDeploymentsRecent(ctx context.Context, limit int) ([]*DeploymentRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Collect all deployments
	all := make([]*DeploymentRecord, 0, len(s.deployments))
	for _, d := range s.deployments {
		cp := *d
		all = append(all, &cp)
	}

	// Sort by StartedAt descending (newest first)
	for i := 0; i < len(all)-1; i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].StartedAt.After(all[i].StartedAt) {
				all[i], all[j] = all[j], all[i]
			}
		}
	}

	// Apply limit
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}

	return all, nil
}

// CountDeploymentsByStatus returns a count of deployments grouped by status.
func (s *MemoryStore) CountDeploymentsByStatus(ctx context.Context) (map[string]int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	counts := make(map[string]int64)
	for _, d := range s.deployments {
		counts[d.Status.String()]++
	}
	return counts, nil
}

// ListDeploymentsPaginated returns deployments with pagination support.
func (s *MemoryStore) ListDeploymentsPaginated(ctx context.Context, limit, offset int) ([]*DeploymentRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Collect all deployments
	all := make([]*DeploymentRecord, 0, len(s.deployments))
	for _, d := range s.deployments {
		cp := *d
		all = append(all, &cp)
	}

	// Sort by StartedAt descending (newest first)
	for i := 0; i < len(all)-1; i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].StartedAt.After(all[i].StartedAt) {
				all[i], all[j] = all[j], all[i]
			}
		}
	}

	// Apply offset
	if offset >= len(all) {
		return []*DeploymentRecord{}, nil
	}
	all = all[offset:]

	// Apply limit
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}

	return all, nil
}

// CountDeployments returns the total number of deployments.
func (s *MemoryStore) CountDeployments(ctx context.Context) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return int64(len(s.deployments)), nil
}

// --- DeploymentLog methods ---

// CreateDeploymentLog creates a new deployment log entry.
func (s *MemoryStore) CreateDeploymentLog(ctx context.Context, log *DeploymentLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if log.ID == "" {
		log.ID = xid.New().String()
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}

	// Copy-on-store
	stored := *log
	s.deploymentLogs[log.DeploymentID] = append(s.deploymentLogs[log.DeploymentID], &stored)

	s.queueWrite(s.deploymentsWrites, NewWriteOp(WriteOpInsert, "deployment_logs", &stored))
	return nil
}

// ListDeploymentLogs returns all log entries for a deployment.
func (s *MemoryStore) ListDeploymentLogs(ctx context.Context, deploymentID string) ([]*DeploymentLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	logs := s.deploymentLogs[deploymentID]
	result := make([]*DeploymentLog, len(logs))
	for i, l := range logs {
		cp := *l
		result[i] = &cp
	}

	return result, nil
}

// ListDeploymentLogsAfter returns log entries after a given ID.
func (s *MemoryStore) ListDeploymentLogsAfter(ctx context.Context, deploymentID string, afterID string) ([]*DeploymentLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	logs := s.deploymentLogs[deploymentID]
	var result []*DeploymentLog
	for _, l := range logs {
		if l.ID > afterID {
			cp := *l
			result = append(result, &cp)
		}
	}

	return result, nil
}

// ListDeploymentLogsPaginated returns deployment logs with pagination support.
func (s *MemoryStore) ListDeploymentLogsPaginated(ctx context.Context, deploymentID string, limit, offset int) ([]*DeploymentLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	logs := s.deploymentLogs[deploymentID]
	if logs == nil {
		return []*DeploymentLog{}, nil
	}

	// Apply offset
	if offset >= len(logs) {
		return []*DeploymentLog{}, nil
	}
	logs = logs[offset:]

	// Apply limit
	if limit > 0 && limit < len(logs) {
		logs = logs[:limit]
	}

	// Copy results
	result := make([]*DeploymentLog, len(logs))
	for i, l := range logs {
		cp := *l
		result[i] = &cp
	}
	return result, nil
}

// CountDeploymentLogs returns the total number of logs for a deployment.
func (s *MemoryStore) CountDeploymentLogs(ctx context.Context, deploymentID string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return int64(len(s.deploymentLogs[deploymentID])), nil
}

// --- DeploymentRollback methods ---

// CreateDeploymentRollback creates a new rollback record.
func (s *MemoryStore) CreateDeploymentRollback(ctx context.Context, rollback *DeploymentRollback) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rollback.ID == "" {
		rollback.ID = xid.New().String()
	}
	if rollback.StartedAt.IsZero() {
		rollback.StartedAt = time.Now()
	}
	if rollback.Status == "" {
		rollback.Status = "pending"
	}

	// Copy-on-store
	stored := *rollback
	s.deploymentRollbacks[rollback.ID] = &stored

	s.queueWrite(s.deploymentsWrites, NewWriteOp(WriteOpInsert, "deployment_rollbacks", &stored))
	return nil
}

// GetDeploymentRollback retrieves a rollback by ID.
func (s *MemoryStore) GetDeploymentRollback(ctx context.Context, id string) (*DeploymentRollback, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rollback, ok := s.deploymentRollbacks[id]
	if !ok {
		return nil, ErrNotFound
	}

	result := *rollback
	return &result, nil
}

// UpdateDeploymentRollback updates a rollback record.
func (s *MemoryStore) UpdateDeploymentRollback(ctx context.Context, rollback *DeploymentRollback) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.deploymentRollbacks[rollback.ID]
	if !ok {
		return ErrNotFound
	}

	existing.Status = rollback.Status
	existing.ErrorMessage = rollback.ErrorMessage
	existing.CompletedAt = rollback.CompletedAt
	existing.HealthCheckError = rollback.HealthCheckError

	s.queueWrite(s.deploymentsWrites, NewWriteOp(WriteOpUpdate, "deployment_rollbacks", existing))
	return nil
}

// ListDeploymentRollbacks returns rollbacks for a project with pagination.
func (s *MemoryStore) ListDeploymentRollbacks(ctx context.Context, projectName string, limit, offset int) ([]*DeploymentRollback, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Collect all rollbacks for project
	var all []*DeploymentRollback
	for _, r := range s.deploymentRollbacks {
		if r.ProjectName == projectName {
			cp := *r
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
		return []*DeploymentRollback{}, total, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}

	return all[offset:end], total, nil
}

// GetLatestRollbackForDeployment returns the most recent rollback for a deployment.
func (s *MemoryStore) GetLatestRollbackForDeployment(ctx context.Context, deploymentID string) (*DeploymentRollback, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var latest *DeploymentRollback
	for _, r := range s.deploymentRollbacks {
		if r.DeploymentID == deploymentID {
			if latest == nil || r.StartedAt.After(latest.StartedAt) {
				latest = r
			}
		}
	}

	if latest == nil {
		return nil, ErrNotFound
	}

	result := *latest
	return &result, nil
}

// --- ScheduledDeployment methods ---

// CreateScheduledDeployment creates a new scheduled deployment.
func (s *MemoryStore) CreateScheduledDeployment(ctx context.Context, id, project, target, branch string, scheduledAt time.Time, scheduledBy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.scheduledDeploys[id]; exists {
		return ErrDuplicate
	}

	scheduled := &ScheduledDeployment{
		ID:          id,
		Project:     project,
		Target:      target,
		Branch:      branch,
		ScheduledAt: scheduledAt,
		ScheduledBy: scheduledBy,
		Status:      "pending",
	}

	s.scheduledDeploys[id] = scheduled
	s.queueWrite(s.deploymentsWrites, NewWriteOp(WriteOpInsert, "scheduled_deployments", scheduled))
	return nil
}

// ListPendingScheduledDeployments returns all pending scheduled deployments.
func (s *MemoryStore) ListPendingScheduledDeployments(ctx context.Context) ([]*ScheduledDeployment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*ScheduledDeployment
	now := time.Now()

	for _, sd := range s.scheduledDeploys {
		if sd.Status == "pending" && sd.ScheduledAt.Before(now) {
			cp := *sd
			result = append(result, &cp)
		}
	}

	return result, nil
}

// CancelScheduledDeployment cancels a scheduled deployment.
func (s *MemoryStore) CancelScheduledDeployment(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sd, ok := s.scheduledDeploys[id]
	if !ok {
		return ErrNotFound
	}

	sd.Status = "cancelled"
	s.queueWrite(s.deploymentsWrites, NewWriteOp(WriteOpUpdate, "scheduled_deployments", sd))
	return nil
}
