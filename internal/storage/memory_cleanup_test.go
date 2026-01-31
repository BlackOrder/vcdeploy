package storage

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStore_CleanupExpiredSessions(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	now := time.Now()
	user := &User{Username: "testuser", Email: "test@example.com", PasswordHash: "hash"}
	s.CreateUser(ctx, user)

	// Create expired and valid sessions
	s.CreateSession(ctx, &Session{UserID: user.ID, Token: "expired", ExpiresAt: now.Add(-time.Hour)})
	s.CreateSession(ctx, &Session{UserID: user.ID, Token: "valid", ExpiresAt: now.Add(time.Hour)})

	count, err := s.CleanupExpiredSessions(ctx, now)
	if err != nil {
		t.Fatalf("CleanupExpiredSessions() error = %v", err)
	}
	if count != 1 {
		t.Errorf("CleanupExpiredSessions() count = %d, want 1", count)
	}

	// Verify expired session is gone
	_, err = s.GetSessionByToken(ctx, "expired")
	if err != ErrNotFound {
		t.Error("expired session should be deleted")
	}

	// Verify valid session remains
	_, err = s.GetSessionByToken(ctx, "valid")
	if err != nil {
		t.Error("valid session should remain")
	}
}

func TestMemoryStore_CleanupOldDeployments(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	now := time.Now()

	// Create old and recent deployments
	s.CreateDeployment(ctx, &DeploymentRecord{ID: "old", StartedAt: now.Add(-48 * time.Hour)})
	s.CreateDeployment(ctx, &DeploymentRecord{ID: "recent", StartedAt: now})

	cutoff := now.Add(-24 * time.Hour)
	count, err := s.CleanupOldDeployments(ctx, cutoff)
	if err != nil {
		t.Fatalf("CleanupOldDeployments() error = %v", err)
	}
	if count != 1 {
		t.Errorf("CleanupOldDeployments() count = %d, want 1", count)
	}

	// Verify old deployment is gone
	_, err = s.GetDeployment(ctx, "old")
	if err != ErrNotFound {
		t.Error("old deployment should be deleted")
	}

	// Verify recent deployment remains
	_, err = s.GetDeployment(ctx, "recent")
	if err != nil {
		t.Error("recent deployment should remain")
	}
}

func TestMemoryStore_CleanupOldDeploymentLogs(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	now := time.Now()
	s.CreateDeployment(ctx, &DeploymentRecord{ID: "deploy1"})

	// Create old and recent logs
	s.CreateDeploymentLog(ctx, &DeploymentLog{DeploymentID: "deploy1", Message: "old", CreatedAt: now.Add(-48 * time.Hour)})
	s.CreateDeploymentLog(ctx, &DeploymentLog{DeploymentID: "deploy1", Message: "recent", CreatedAt: now})

	cutoff := now.Add(-24 * time.Hour)
	count, err := s.CleanupOldDeploymentLogs(ctx, cutoff)
	if err != nil {
		t.Fatalf("CleanupOldDeploymentLogs() error = %v", err)
	}
	if count != 1 {
		t.Errorf("CleanupOldDeploymentLogs() count = %d, want 1", count)
	}

	// Verify only recent log remains
	logs, _ := s.ListDeploymentLogs(ctx, "deploy1")
	if len(logs) != 1 {
		t.Errorf("len(logs) = %d, want 1", len(logs))
	}
	if logs[0].Message != "recent" {
		t.Error("recent log should remain")
	}
}

func TestMemoryStore_CleanupOldAuditLogs(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	now := time.Now()

	// Create old and recent audit entries
	old := &AuditEntry{Action: "old", Timestamp: now.Add(-48 * time.Hour)}
	recent := &AuditEntry{Action: "recent", Timestamp: now}
	s.LogAudit(ctx, old)
	s.LogAudit(ctx, recent)

	cutoff := now.Add(-24 * time.Hour)
	count, err := s.CleanupOldAuditLogs(ctx, cutoff)
	if err != nil {
		t.Fatalf("CleanupOldAuditLogs() error = %v", err)
	}
	if count != 1 {
		t.Errorf("CleanupOldAuditLogs() count = %d, want 1", count)
	}

	// Verify only recent audit log remains
	logs, _ := s.ListAuditLogs(ctx, 100, 0)
	if len(logs) != 1 {
		t.Errorf("len(logs) = %d, want 1", len(logs))
	}
	if logs[0].Action != "recent" {
		t.Error("recent audit entry should remain")
	}
}

func TestMemoryStore_CleanupExpiredBlockedIPs(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	now := time.Now()

	// Create expired and non-expired blocked IPs
	s.BlockIP(ctx, &BlockedIP{IPAddress: "1.1.1.1", ExpiresAt: now.Add(-time.Hour)})
	s.BlockIP(ctx, &BlockedIP{IPAddress: "2.2.2.2", ExpiresAt: now.Add(time.Hour)})

	count, err := s.CleanupExpiredBlockedIPs(ctx)
	if err != nil {
		t.Fatalf("CleanupExpiredBlockedIPs() error = %v", err)
	}
	if count != 1 {
		t.Errorf("CleanupExpiredBlockedIPs() count = %d, want 1", count)
	}

	// Verify expired IP is gone
	blocked, _ := s.IsIPBlocked(ctx, "1.1.1.1")
	if blocked {
		t.Error("expired blocked IP should be cleaned up")
	}

	// Verify non-expired IP remains
	blocked, _ = s.IsIPBlocked(ctx, "2.2.2.2")
	if !blocked {
		t.Error("non-expired blocked IP should remain")
	}
}

func TestMemoryStore_CleanupRateLimitRecords(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	now := time.Now()
	oldStart := now.Add(-48 * time.Hour)
	recentStart := now.Add(-1 * time.Hour)

	// Create old and recent rate limit records with proper args
	s.RecordRateLimitRequest(ctx, "old", "minute", oldStart, oldStart.Add(time.Minute))
	s.RecordRateLimitRequest(ctx, "recent", "minute", recentStart, recentStart.Add(time.Minute))

	cutoff := now.Add(-24 * time.Hour)
	count, err := s.CleanupRateLimitRecords(ctx, cutoff)
	if err != nil {
		t.Fatalf("CleanupRateLimitRecords() error = %v", err)
	}
	if count != 1 {
		t.Errorf("CleanupRateLimitRecords() count = %d, want 1", count)
	}
}

func TestMemoryStore_CleanupOldProvisionJobs(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	now := time.Now()

	// Create old and recent provision jobs
	s.CreateProvisionJob(ctx, &ProvisionJob{ID: "old", StartedAt: now.Add(-48 * time.Hour)})
	s.CreateProvisionJob(ctx, &ProvisionJob{ID: "recent", StartedAt: now})

	cutoff := now.Add(-24 * time.Hour)
	count, err := s.CleanupOldProvisionJobs(ctx, cutoff)
	if err != nil {
		t.Fatalf("CleanupOldProvisionJobs() error = %v", err)
	}
	if count != 1 {
		t.Errorf("CleanupOldProvisionJobs() count = %d, want 1", count)
	}

	// Verify old job is gone
	_, err = s.GetProvisionJob(ctx, "old")
	if err != ErrNotFound {
		t.Error("old provision job should be deleted")
	}

	// Verify recent job remains
	_, err = s.GetProvisionJob(ctx, "recent")
	if err != nil {
		t.Error("recent provision job should remain")
	}
}

func TestMemoryStore_CleanupExpiredAPIKeys(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	now := time.Now()
	expiredAt := now.Add(-time.Hour)
	validAt := now.Add(time.Hour)

	user := &User{Username: "testuser", Email: "test@example.com", PasswordHash: "hash"}
	s.CreateUser(ctx, user)

	// Create expired and valid API keys
	s.CreateAPIKey(ctx, &APIKey{UserID: user.ID, KeyHash: "expired", ExpiresAt: &expiredAt})
	s.CreateAPIKey(ctx, &APIKey{UserID: user.ID, KeyHash: "valid", ExpiresAt: &validAt})

	count, err := s.CleanupExpiredAPIKeys(ctx, now)
	if err != nil {
		t.Fatalf("CleanupExpiredAPIKeys() error = %v", err)
	}
	if count != 1 {
		t.Errorf("CleanupExpiredAPIKeys() count = %d, want 1", count)
	}

	// Verify expired key is gone
	_, err = s.GetAPIKeyByHash(ctx, "expired")
	if err != ErrNotFound {
		t.Error("expired API key should be deleted")
	}

	// Verify valid key remains
	_, err = s.GetAPIKeyByHash(ctx, "valid")
	if err != nil {
		t.Error("valid API key should remain")
	}
}

func TestMemoryStore_CleanupOrphanedWebhooks(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	// Create a project
	project := &Project{Name: "test-project"}
	s.CreateProject(project)

	// Create webhook for the project
	s.SetProjectWebhook(ctx, project.ID, "github", []byte("secret"), true, true)

	// Create orphaned webhook (for non-existent project) directly in the map
	s.mu.Lock()
	orphanedID := nextID(&s.nextWebhookID)
	s.webhooks[orphanedID] = &ProjectWebhook{ID: orphanedID, ProjectID: 99999, Provider: "gitlab"}
	s.mu.Unlock()

	count, err := s.CleanupOrphanedWebhooks(ctx)
	if err != nil {
		t.Fatalf("CleanupOrphanedWebhooks() error = %v", err)
	}
	if count != 1 {
		t.Errorf("CleanupOrphanedWebhooks() count = %d, want 1", count)
	}

	// Verify orphaned webhook is gone but valid webhook remains
	webhook, _ := s.GetProjectWebhook(ctx, project.ID, "github")
	if webhook == nil {
		t.Error("valid webhook should remain")
	}
}

func TestMemoryStore_MarkStaleAgents(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	now := time.Now()
	oldTime := now.Add(-48 * time.Hour)

	// Create agents
	s.UpsertAgent(ctx, &Agent{ID: "stale", Hostname: "stale-host", Status: "online"})
	s.UpsertAgent(ctx, &Agent{ID: "active", Hostname: "active-host", Status: "online"})
	s.UpsertAgent(ctx, &Agent{ID: "already-stale", Hostname: "already-stale", Status: "stale"})

	// Manually update LastSeenAt for testing (UpsertAgent sets it to now)
	s.mu.Lock()
	s.agents["stale"].LastSeenAt = oldTime
	s.agents["already-stale"].LastSeenAt = oldTime
	s.mu.Unlock()

	cutoff := now.Add(-24 * time.Hour)
	count, err := s.MarkStaleAgents(ctx, cutoff)
	if err != nil {
		t.Fatalf("MarkStaleAgents() error = %v", err)
	}
	if count != 1 {
		t.Errorf("MarkStaleAgents() count = %d, want 1", count)
	}

	// Verify stale agent is marked
	agent, _ := s.GetAgent(ctx, "stale")
	if agent.Status != "stale" {
		t.Errorf("stale agent Status = %s, want stale", agent.Status)
	}

	// Verify active agent is not marked
	agent, _ = s.GetAgent(ctx, "active")
	if agent.Status != "online" {
		t.Errorf("active agent Status = %s, want online", agent.Status)
	}
}

func TestMemoryStore_Backup(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	err := s.Backup("/tmp/backup.db")
	if err != ErrNotImplemented {
		t.Errorf("Backup() error = %v, want ErrNotImplemented", err)
	}
}
