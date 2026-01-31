package storage

import (
	"context"
	"errors"
	"time"
)

// ErrNotImplemented is returned when an operation is not supported.
var ErrNotImplemented = errors.New("not implemented")

// CleanupExpiredSessions removes sessions that expired before the given cutoff time.
func (s *MemoryStore) CleanupExpiredSessions(ctx context.Context, cutoff time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var deleted int64
	for token, sess := range s.sessions {
		if sess.ExpiresAt.Before(cutoff) {
			delete(s.sessions, token)
			s.queueWrite(s.coreWrites, NewWriteOp(WriteOpDelete, "sessions", sess))
			deleted++
		}
	}
	return deleted, nil
}

// CleanupOldDeployments removes deployment records created before the given cutoff time.
// Uses StartedAt since DeploymentRecord doesn't have CreatedAt.
func (s *MemoryStore) CleanupOldDeployments(ctx context.Context, cutoff time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var deleted int64
	for id, dep := range s.deployments {
		if dep.StartedAt.Before(cutoff) {
			delete(s.deployments, id)
			s.queueWrite(s.deploymentsWrites, NewWriteOp(WriteOpDelete, "deployments", dep))
			deleted++
		}
	}
	return deleted, nil
}

// CleanupOldDeploymentLogs removes deployment logs created before the given cutoff time.
func (s *MemoryStore) CleanupOldDeploymentLogs(ctx context.Context, cutoff time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var deleted int64
	// deploymentLogs is map[string][]*DeploymentLog keyed by deploymentID
	for deploymentID, logs := range s.deploymentLogs {
		var kept []*DeploymentLog
		for _, log := range logs {
			if log.CreatedAt.Before(cutoff) {
				s.queueWrite(s.deploymentsWrites, NewWriteOp(WriteOpDelete, "deployment_logs", log))
				deleted++
			} else {
				kept = append(kept, log)
			}
		}
		if len(kept) == 0 {
			delete(s.deploymentLogs, deploymentID)
		} else {
			s.deploymentLogs[deploymentID] = kept
		}
	}
	return deleted, nil
}

// CleanupOldAuditLogs removes audit entries created before the given cutoff time.
func (s *MemoryStore) CleanupOldAuditLogs(ctx context.Context, cutoff time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var deleted int64
	var kept []*AuditEntry
	for _, entry := range s.auditLogs {
		if entry.Timestamp.Before(cutoff) {
			s.queueWrite(s.auditWrites, NewWriteOp(WriteOpDelete, "audit_logs", entry))
			deleted++
		} else {
			kept = append(kept, entry)
		}
	}
	s.auditLogs = kept
	return deleted, nil
}

// CleanupExpiredBlockedIPs removes blocked IP entries that have expired.
func (s *MemoryStore) CleanupExpiredBlockedIPs(ctx context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	var deleted int64
	for ip, blocked := range s.blockedIPs {
		// ExpiresAt is time.Time (not pointer), check if it's in the past and non-zero
		if !blocked.ExpiresAt.IsZero() && blocked.ExpiresAt.Before(now) {
			delete(s.blockedIPs, ip)
			s.queueWrite(s.ratelimitWrites, NewWriteOp(WriteOpDelete, "blocked_ips", blocked))
			deleted++
		}
	}
	return deleted, nil
}

// CleanupRateLimitRecords removes rate limit records from before the given time.
func (s *MemoryStore) CleanupRateLimitRecords(ctx context.Context, before time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var deleted int64
	for key, record := range s.rateLimits {
		if record.WindowStart.Before(before) {
			delete(s.rateLimits, key)
			s.queueWrite(s.ratelimitWrites, NewWriteOp(WriteOpDelete, "rate_limits", record))
			deleted++
		}
	}
	return deleted, nil
}

// CleanupOldProvisionJobs removes provision jobs started before the given time.
func (s *MemoryStore) CleanupOldProvisionJobs(ctx context.Context, before time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var deleted int64
	for id, job := range s.provisionJobs {
		if job.StartedAt.Before(before) {
			delete(s.provisionJobs, id)
			s.queueWrite(s.provisionWrites, NewWriteOp(WriteOpDelete, "provision_jobs", job))
			deleted++
		}
	}
	return deleted, nil
}

// CleanupExpiredAPIKeys removes API keys that have expired before the given time.
func (s *MemoryStore) CleanupExpiredAPIKeys(ctx context.Context, now time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var deleted int64
	// apiKeys is map[string]*APIKey keyed by hash
	for hash, key := range s.apiKeys {
		if key.ExpiresAt != nil && key.ExpiresAt.Before(now) {
			delete(s.apiKeys, hash)
			// Also remove from apiKeysByUser
			if keys, ok := s.apiKeysByUser[key.UserID]; ok {
				var kept []*APIKey
				for _, k := range keys {
					if k.KeyHash != hash {
						kept = append(kept, k)
					}
				}
				if len(kept) == 0 {
					delete(s.apiKeysByUser, key.UserID)
				} else {
					s.apiKeysByUser[key.UserID] = kept
				}
			}
			s.queueWrite(s.coreWrites, NewWriteOp(WriteOpDelete, "api_keys", key))
			deleted++
		}
	}
	return deleted, nil
}

// CleanupOrphanedWebhooks removes webhooks that reference non-existent projects.
func (s *MemoryStore) CleanupOrphanedWebhooks(ctx context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var deleted int64
	for id, webhook := range s.webhooks {
		if _, exists := s.projects[webhook.ProjectID]; !exists {
			delete(s.webhooks, id)
			s.queueWrite(s.projectsWrites, NewWriteOp(WriteOpDelete, "webhooks", webhook))
			deleted++
		}
	}
	return deleted, nil
}

// MarkStaleAgents marks agents as stale if they haven't been seen since the cutoff time.
func (s *MemoryStore) MarkStaleAgents(ctx context.Context, cutoff time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var marked int64
	for id, agent := range s.agents {
		// Only mark non-stale agents that haven't been seen recently
		// Agent uses LastSeenAt field (not LastSeen)
		if agent.Status == "stale" || !agent.LastSeenAt.Before(cutoff) {
			continue
		}
		// Create a copy for update
		updated := *agent
		// Deep copy Labels map if present
		if agent.Labels != nil {
			updated.Labels = make(map[string]string, len(agent.Labels))
			for k, v := range agent.Labels {
				updated.Labels[k] = v
			}
		}
		updated.Status = "stale"
		s.agents[id] = &updated
		s.queueWrite(s.agentsWrites, NewWriteOp(WriteOpUpdate, "agents", &updated))
		marked++
	}
	return marked, nil
}

// Backup is not supported by MemoryStore.
func (s *MemoryStore) Backup(destPath string) error {
	return ErrNotImplemented
}
