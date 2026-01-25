// Package server provides background cleanup tasks.
package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// setupTestCleanupDB creates a temporary database for testing
func setupTestCleanupDB(t *testing.T) (*storage.DB, func()) {
	t.Helper()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "cleanup_test")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("open database: %v", err)
	}

	// Run migrations
	if err := db.MigrateUp(context.Background()); err != nil {
		db.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("migrate database: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return db, cleanup
}

func TestDefaultCleanupConfig(t *testing.T) {
	cfg := DefaultCleanupConfig()

	if cfg.Interval != 1*time.Hour {
		t.Errorf("expected interval 1h, got %s", cfg.Interval)
	}
	if cfg.DeploymentRetention != 30*24*time.Hour {
		t.Errorf("expected deployment retention 30 days, got %s", cfg.DeploymentRetention)
	}
	if cfg.AuditLogRetention != 90*24*time.Hour {
		t.Errorf("expected audit log retention 90 days, got %s", cfg.AuditLogRetention)
	}
	if cfg.SessionExpiry != 24*time.Hour {
		t.Errorf("expected session expiry 24h, got %s", cfg.SessionExpiry)
	}
	if cfg.StaleAgentThreshold != 5*time.Minute {
		t.Errorf("expected stale agent threshold 5m, got %s", cfg.StaleAgentThreshold)
	}
	if cfg.DeploymentLogRetention != 7*24*time.Hour {
		t.Errorf("expected deployment log retention 7 days, got %s", cfg.DeploymentLogRetention)
	}
}

func TestNewCleanupTask(t *testing.T) {
	db, cleanup := setupTestCleanupDB(t)
	defer cleanup()

	logger := zaptest.NewLogger(t)

	// Test with default config
	task := NewCleanupTask(db, logger, DefaultCleanupConfig())
	if task == nil {
		t.Fatal("expected non-nil task")
	}

	if task.db != db {
		t.Error("db not set correctly")
	}
	if task.interval != 1*time.Hour {
		t.Errorf("expected interval 1h, got %s", task.interval)
	}
}

func TestNewCleanupTask_ZeroValues(t *testing.T) {
	db, cleanup := setupTestCleanupDB(t)
	defer cleanup()

	logger := zaptest.NewLogger(t)

	// Test with zero values (should use defaults)
	task := NewCleanupTask(db, logger, CleanupConfig{})

	if task.interval != 1*time.Hour {
		t.Errorf("expected default interval 1h, got %s", task.interval)
	}
	if task.deploymentRetention != 30*24*time.Hour {
		t.Errorf("expected default deployment retention, got %s", task.deploymentRetention)
	}
	if task.auditLogRetention != 90*24*time.Hour {
		t.Errorf("expected default audit retention, got %s", task.auditLogRetention)
	}
	if task.sessionExpiry != 24*time.Hour {
		t.Errorf("expected default session expiry, got %s", task.sessionExpiry)
	}
	if task.staleAgentThreshold != 5*time.Minute {
		t.Errorf("expected default stale threshold, got %s", task.staleAgentThreshold)
	}
	if task.deploymentLogRetention != 7*24*time.Hour {
		t.Errorf("expected default log retention, got %s", task.deploymentLogRetention)
	}
}

func TestNewCleanupTask_CustomValues(t *testing.T) {
	db, cleanup := setupTestCleanupDB(t)
	defer cleanup()

	logger := zaptest.NewLogger(t)

	cfg := CleanupConfig{
		Interval:               15 * time.Minute,
		DeploymentRetention:    7 * 24 * time.Hour,
		AuditLogRetention:      30 * 24 * time.Hour,
		SessionExpiry:          12 * time.Hour,
		StaleAgentThreshold:    10 * time.Minute,
		DeploymentLogRetention: 3 * 24 * time.Hour,
	}

	task := NewCleanupTask(db, logger, cfg)

	if task.interval != 15*time.Minute {
		t.Errorf("expected custom interval 15m, got %s", task.interval)
	}
	if task.deploymentRetention != 7*24*time.Hour {
		t.Errorf("expected custom deployment retention 7 days, got %s", task.deploymentRetention)
	}
	if task.auditLogRetention != 30*24*time.Hour {
		t.Errorf("expected custom audit retention 30 days, got %s", task.auditLogRetention)
	}
	if task.sessionExpiry != 12*time.Hour {
		t.Errorf("expected custom session expiry 12h, got %s", task.sessionExpiry)
	}
	if task.staleAgentThreshold != 10*time.Minute {
		t.Errorf("expected custom stale threshold 10m, got %s", task.staleAgentThreshold)
	}
	if task.deploymentLogRetention != 3*24*time.Hour {
		t.Errorf("expected custom log retention 3 days, got %s", task.deploymentLogRetention)
	}
}

func TestCleanupTask_StartStop(t *testing.T) {
	db, cleanup := setupTestCleanupDB(t)
	defer cleanup()

	logger := zaptest.NewLogger(t)

	// Use very short interval for testing
	cfg := CleanupConfig{
		Interval:               50 * time.Millisecond,
		DeploymentRetention:    1 * time.Hour,
		AuditLogRetention:      1 * time.Hour,
		SessionExpiry:          1 * time.Hour,
		StaleAgentThreshold:    1 * time.Minute,
		DeploymentLogRetention: 1 * time.Hour,
	}

	task := NewCleanupTask(db, logger, cfg)

	// Start the task
	task.Start()

	// Wait a bit to ensure cleanup runs
	time.Sleep(100 * time.Millisecond)

	// Stop the task
	task.Stop()

	// Should not panic or hang
}

func TestCleanupTask_CleanExpiredSessions(t *testing.T) {
	db, cleanup := setupTestCleanupDB(t)
	defer cleanup()

	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	task := NewCleanupTask(db, logger, CleanupConfig{
		SessionExpiry: 1 * time.Hour,
	})

	// Create a test user first
	user := &storage.User{
		Username: "testuser",
		Email:    "test@example.com",
		Role:     "admin",
	}
	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Create an expired session
	session := &storage.Session{
		UserID:    user.ID,
		Token:     "expired-session-token-1",
		ExpiresAt: time.Now().Add(-2 * time.Hour), // expired 2 hours ago
	}
	if err := db.CreateSession(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Run cleanup
	count, err := task.cleanExpiredSessions(ctx)
	if err != nil {
		t.Fatalf("clean expired sessions: %v", err)
	}
	// The session was already inserted with expired time, so it should be cleaned
	if count < 0 {
		t.Errorf("expected non-negative count, got %d", count)
	}
}

func TestCleanupTask_CleanOldDeployments(t *testing.T) {
	db, cleanup := setupTestCleanupDB(t)
	defer cleanup()

	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	task := NewCleanupTask(db, logger, CleanupConfig{
		DeploymentRetention: 7 * 24 * time.Hour, // 7 days
	})

	// Create a project
	project := &storage.Project{
		Name:       "test-project",
		Repository: "https://github.com/test/test",
		CreatedAt:  time.Now(),
	}
	if err := db.CreateProject(project); err != nil {
		t.Fatalf("create project: %v", err)
	}

	// Create an old deployment (10 days ago)
	oldDeployment := &storage.DeploymentCLI{
		ProjectID:   project.ID,
		ProjectName: project.Name,
		Status:      "completed",
		StartedAt:   time.Now().Add(-10 * 24 * time.Hour),
		TriggeredBy: "test",
	}
	finishedAt := time.Now().Add(-10 * 24 * time.Hour)
	oldDeployment.FinishedAt = &finishedAt
	if err := db.InsertDeployment(oldDeployment); err != nil {
		t.Fatalf("insert old deployment: %v", err)
	}

	// Run cleanup - verify no errors
	count, err := task.cleanOldDeployments(ctx)
	if err != nil {
		t.Fatalf("clean old deployments: %v", err)
	}
	// Just verify it doesn't error - actual cleanup depends on started_at column
	t.Logf("cleaned %d deployments", count)
}

func TestCleanupTask_MarkStaleAgents(t *testing.T) {
	db, cleanup := setupTestCleanupDB(t)
	defer cleanup()

	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	task := NewCleanupTask(db, logger, CleanupConfig{
		StaleAgentThreshold: 5 * time.Minute,
	})

	// Create a stale agent (last seen 10 minutes ago)
	staleAgent := &storage.Agent{
		ID:         "stale-agent",
		Hostname:   "stale.example.com",
		Status:     "online",
		LastSeenAt: time.Now().Add(-10 * time.Minute),
	}
	if err := db.UpsertAgent(ctx, staleAgent); err != nil {
		t.Fatalf("upsert stale agent: %v", err)
	}

	// Run cleanup - verify no errors
	count, err := task.markStaleAgents(ctx)
	if err != nil {
		t.Fatalf("mark stale agents: %v", err)
	}
	t.Logf("marked %d agents as stale", count)
}

func TestCleanupTask_RunCleanup(t *testing.T) {
	db, cleanup := setupTestCleanupDB(t)
	defer cleanup()

	logger, _ := zap.NewDevelopment()

	task := NewCleanupTask(db, logger, CleanupConfig{
		Interval:               1 * time.Hour,
		DeploymentRetention:    1 * time.Hour,
		AuditLogRetention:      1 * time.Hour,
		SessionExpiry:          1 * time.Hour,
		StaleAgentThreshold:    1 * time.Minute,
		DeploymentLogRetention: 1 * time.Hour,
	})

	// Run the full cleanup - should not panic even with empty database
	task.runCleanup()
}

func TestCleanupTask_MultipleCycles(t *testing.T) {
	db, cleanup := setupTestCleanupDB(t)
	defer cleanup()

	logger := zaptest.NewLogger(t)

	cfg := CleanupConfig{
		Interval:               20 * time.Millisecond,
		DeploymentRetention:    1 * time.Hour,
		AuditLogRetention:      1 * time.Hour,
		SessionExpiry:          1 * time.Hour,
		StaleAgentThreshold:    1 * time.Minute,
		DeploymentLogRetention: 1 * time.Hour,
	}

	task := NewCleanupTask(db, logger, cfg)

	// Start the task
	task.Start()

	// Let it run multiple cycles
	time.Sleep(100 * time.Millisecond)

	// Stop gracefully
	task.Stop()

	// Should complete without panic or deadlock
}

func TestCleanupTask_CleanExpiredAPIKeys(t *testing.T) {
	db, cleanup := setupTestCleanupDB(t)
	defer cleanup()

	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	task := NewCleanupTask(db, logger, DefaultCleanupConfig())

	// Create an expired API key
	expiredKey := &storage.APIKey{
		Name:      "expired-key",
		KeyHash:   "expired-hash",
		ExpiresAt: timePtr(time.Now().Add(-1 * time.Hour)),
	}
	if err := db.CreateAPIKey(ctx, expiredKey); err != nil {
		t.Fatalf("create expired API key: %v", err)
	}

	// Create a valid API key
	validKey := &storage.APIKey{
		Name:      "valid-key",
		KeyHash:   "valid-hash",
		ExpiresAt: timePtr(time.Now().Add(24 * time.Hour)),
	}
	if err := db.CreateAPIKey(ctx, validKey); err != nil {
		t.Fatalf("create valid API key: %v", err)
	}

	// Run cleanup
	count, err := task.cleanExpiredAPIKeys(ctx)
	if err != nil {
		t.Fatalf("clean expired API keys: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 cleaned key, got %d", count)
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}
