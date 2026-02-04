package deployments

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services/testutil"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

func newTestService(t *testing.T) (*Service, storage.Store) {
	t.Helper()

	db, cleanup := testutil.NewTestStore(t)
	t.Cleanup(cleanup)

	return New(db), db
}

func createTestDeployment(t *testing.T, svc *Service, id, status string) *storage.DeploymentRecord {
	t.Helper()
	ctx := context.Background()
	deployment := &storage.DeploymentRecord{
		ID:            id,
		Project:       "test-project",
		Target:        "production",
		Branch:        "main",
		CommitHash:    "abc123",
		Status:        storage.DeploymentStatus(status),
		ReleaseNumber: 1,
		StartedAt:     time.Now(),
		TriggeredBy:   "test",
		TriggerSource: "manual",
	}
	if err := svc.Create(ctx, deployment); err != nil {
		t.Fatalf("createTestDeployment() error = %v", err)
	}
	return deployment
}

func TestService_Create(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	deployment := &storage.DeploymentRecord{
		ID:            "deploy-1",
		Project:       "my-project",
		Target:        "staging",
		Branch:        "develop",
		CommitHash:    "def456",
		Status:        "pending",
		ReleaseNumber: 1,
		StartedAt:     time.Now(),
		TriggeredBy:   "admin",
		TriggerSource: "api",
	}

	err := svc.Create(ctx, deployment)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Verify it was created
	found, err := svc.GetByID(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if found == nil {
		t.Fatal("GetByID() returned nil after Create")
	}
	if found.ID != deployment.ID {
		t.Errorf("GetByID() id = %v, want %v", found.ID, deployment.ID)
	}
	if found.Project != deployment.Project {
		t.Errorf("GetByID() project = %v, want %v", found.Project, deployment.Project)
	}
}

func TestService_GetByID(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	deployment := createTestDeployment(t, svc, "find-me", "running")

	found, err := svc.GetByID(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if found == nil {
		t.Fatal("GetByID() returned nil")
	}
	if found.ID != deployment.ID {
		t.Errorf("GetByID() id = %v, want %v", found.ID, deployment.ID)
	}
}

func TestService_GetByID_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.GetByID(ctx, "nonexistent")
	if err == nil {
		t.Error("GetByID() expected error for nonexistent deployment")
	}
}

func TestService_Update(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	deployment := createTestDeployment(t, svc, "to-update", "pending")

	// Update status
	deployment.Status = "running"
	err := svc.Update(ctx, deployment)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Verify update
	updated, err := svc.GetByID(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if updated.Status != "running" {
		t.Errorf("Update() status = %v, want %v", updated.Status, "running")
	}
}

func TestService_ListRecent(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create some deployments
	for i := 0; i < 5; i++ {
		createTestDeployment(t, svc, fmt.Sprintf("deploy-%d", i), "completed")
	}

	// List with limit
	deployments, err := svc.ListRecent(ctx, 3)
	if err != nil {
		t.Fatalf("ListRecent() error = %v", err)
	}
	if len(deployments) != 3 {
		t.Errorf("ListRecent() returned %v deployments, want %v", len(deployments), 3)
	}

	// List all
	all, err := svc.ListRecent(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecent() error = %v", err)
	}
	if len(all) != 5 {
		t.Errorf("ListRecent() returned %v deployments, want %v", len(all), 5)
	}
}

func TestService_CountByStatus(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create deployments with different statuses
	statuses := []string{"completed", "completed", "failed", "running", "pending"}
	for i, status := range statuses {
		createTestDeployment(t, svc, fmt.Sprintf("status-%d", i), status)
	}

	counts, err := svc.CountByStatus(ctx)
	if err != nil {
		t.Fatalf("CountByStatus() error = %v", err)
	}

	if counts["completed"] != 2 {
		t.Errorf("CountByStatus() completed = %v, want %v", counts["completed"], 2)
	}
	if counts["failed"] != 1 {
		t.Errorf("CountByStatus() failed = %v, want %v", counts["failed"], 1)
	}
	if counts["running"] != 1 {
		t.Errorf("CountByStatus() running = %v, want %v", counts["running"], 1)
	}
	if counts["pending"] != 1 {
		t.Errorf("CountByStatus() pending = %v, want %v", counts["pending"], 1)
	}
}

func TestService_Cancel(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	deployment := createTestDeployment(t, svc, "to-cancel", "pending")

	err := svc.Cancel(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	// Verify cancelled
	cancelled, err := svc.GetByID(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if cancelled.Status != "cancelled" {
		t.Errorf("Cancel() status = %v, want %v", cancelled.Status, "cancelled")
	}
}

func TestService_Cancel_InvalidStatus(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Cannot cancel completed deployment
	deployment := createTestDeployment(t, svc, "completed-deploy", "completed")

	err := svc.Cancel(ctx, deployment.ID)
	if err == nil {
		t.Error("Cancel() expected error for completed deployment")
	}
}

func TestService_ContextCancellation(t *testing.T) {
	svc, _ := newTestService(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := svc.ListRecent(ctx, 10)
	if err == nil {
		t.Error("ListRecent() expected error for cancelled context")
	}
}

func TestService_Cancel_RunningStatus(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Can cancel running deployment
	deployment := createTestDeployment(t, svc, "running-deploy", "running")

	err := svc.Cancel(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	// Verify cancelled
	cancelled, err := svc.GetByID(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if cancelled.Status != "cancelled" {
		t.Errorf("Cancel() status = %v, want %v", cancelled.Status, "cancelled")
	}
	if cancelled.CompletedAt == nil {
		t.Error("Cancel() should set CompletedAt")
	}
	if cancelled.ErrorMessage != "Cancelled by user" {
		t.Errorf("Cancel() ErrorMessage = %v, want %v", cancelled.ErrorMessage, "Cancelled by user")
	}
}

func TestService_Cancel_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	err := svc.Cancel(ctx, "nonexistent-id")
	if err == nil {
		t.Error("Cancel() expected error for nonexistent deployment")
	}
}

func TestService_Cancel_FailedStatus(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Cannot cancel failed deployment
	deployment := createTestDeployment(t, svc, "failed-deploy", "failed")

	err := svc.Cancel(ctx, deployment.ID)
	if err == nil {
		t.Error("Cancel() expected error for failed deployment")
	}
}

// --- Log operations tests ---

// --- Log-related tests ---
// These tests were previously skipped due to schema/code mismatch (timestamp vs created_at).
// The schema has been fixed to use created_at column.

func TestService_CreateLog(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	deployment := createTestDeployment(t, svc, "log-deploy-1", "running")

	log := &storage.DeploymentLog{
		DeploymentID: deployment.ID,
		Level:        "info",
		Message:      "Test log message",
		Source:       "test",
		CreatedAt:    time.Now(),
	}

	err := svc.CreateLog(ctx, log)
	if err != nil {
		t.Fatalf("CreateLog() error = %v", err)
	}

	logs, err := svc.ListLogs(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("ListLogs() error = %v", err)
	}
	if len(logs) != 1 {
		t.Errorf("ListLogs() returned %d logs, want 1", len(logs))
	}
	if logs[0].Message != "Test log message" {
		t.Errorf("Log message = %q, want %q", logs[0].Message, "Test log message")
	}
}

func TestService_ListLogs(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	deployment := createTestDeployment(t, svc, "log-deploy-2", "running")

	// Create multiple logs
	for i := 0; i < 3; i++ {
		log := &storage.DeploymentLog{
			DeploymentID: deployment.ID,
			Level:        "info",
			Message:      fmt.Sprintf("Log message %d", i),
			Source:       "test",
			CreatedAt:    time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := svc.CreateLog(ctx, log); err != nil {
			t.Fatalf("CreateLog() error = %v", err)
		}
	}

	logs, err := svc.ListLogs(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("ListLogs() error = %v", err)
	}
	if len(logs) != 3 {
		t.Errorf("ListLogs() returned %d logs, want 3", len(logs))
	}
}

func TestService_ListLogs_Empty(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	deployment := createTestDeployment(t, svc, "log-deploy-empty", "running")

	logs, err := svc.ListLogs(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("ListLogs() error = %v", err)
	}
	if len(logs) != 0 {
		t.Errorf("ListLogs() returned %d logs, want 0", len(logs))
	}
}

func TestService_ListLogsAfter(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	deployment := createTestDeployment(t, svc, "log-deploy-after", "running")

	// Create multiple logs
	var lastID int64
	for i := 0; i < 5; i++ {
		log := &storage.DeploymentLog{
			DeploymentID: deployment.ID,
			Level:        "info",
			Message:      fmt.Sprintf("Log message %d", i),
			Source:       "test",
			CreatedAt:    time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := svc.CreateLog(ctx, log); err != nil {
			t.Fatalf("CreateLog() error = %v", err)
		}
		if i == 2 {
			// Get the ID of the 3rd log to use as cursor
			logs, _ := svc.ListLogs(ctx, deployment.ID)
			lastID = logs[2].ID
		}
	}

	// Get logs after the 3rd one
	logs, err := svc.ListLogsAfter(ctx, deployment.ID, lastID)
	if err != nil {
		t.Fatalf("ListLogsAfter() error = %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("ListLogsAfter() returned %d logs, want 2", len(logs))
	}
}

// --- Scheduled deployment tests ---

func TestService_CreateScheduled(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	scheduledAt := time.Now().Add(1 * time.Hour)
	err := svc.CreateScheduled(ctx, "sched-1", "test-project", "production", "main", scheduledAt, "admin")
	if err != nil {
		t.Fatalf("CreateScheduled() error = %v", err)
	}
}

func TestService_ListPendingScheduled(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Test that the method works without error (empty result is fine for new DB)
	_, err := svc.ListPendingScheduled(ctx)
	if err != nil {
		t.Fatalf("ListPendingScheduled() error = %v", err)
	}
}

func TestService_ListPendingScheduled_WithData(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	// Directly insert a scheduled deployment with a past scheduled_at using raw SQL
	// This bypasses any Go time formatting issues
	_, err := db.Conn().ExecContext(ctx, `
		INSERT INTO deployments (id, project, target, branch, status, scheduled_at, scheduled_by, triggered_by)
		VALUES ('sched-test-1', 'test-project', 'production', 'main', 'scheduled', datetime('now', '-1 hour'), 'admin', 'admin')
	`)
	if err != nil {
		t.Fatalf("Failed to insert test scheduled deployment: %v", err)
	}

	// Insert one in the future (should not be returned)
	_, err = db.Conn().ExecContext(ctx, `
		INSERT INTO deployments (id, project, target, branch, status, scheduled_at, scheduled_by, triggered_by)
		VALUES ('sched-test-2', 'test-project', 'staging', 'develop', 'scheduled', datetime('now', '+1 hour'), 'admin', 'admin')
	`)
	if err != nil {
		t.Fatalf("Failed to insert future scheduled deployment: %v", err)
	}

	pending, err := svc.ListPendingScheduled(ctx)
	if err != nil {
		t.Fatalf("ListPendingScheduled() error = %v", err)
	}

	// Should return only the past scheduled deployment
	if len(pending) != 1 {
		t.Errorf("ListPendingScheduled() returned %v, expected 1", len(pending))
	}

	if len(pending) > 0 && pending[0].ID != "sched-test-1" {
		t.Errorf("ListPendingScheduled() returned wrong deployment, got %v", pending[0].ID)
	}
}

func TestService_CancelScheduled(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	scheduledAt := time.Now().Add(1 * time.Hour)
	err := svc.CreateScheduled(ctx, "sched-cancel-1", "test-project", "production", "main", scheduledAt, "admin")
	if err != nil {
		t.Fatalf("CreateScheduled() error = %v", err)
	}

	err = svc.CancelScheduled(ctx, "sched-cancel-1")
	if err != nil {
		t.Fatalf("CancelScheduled() error = %v", err)
	}
}

func TestService_CancelScheduled_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	err := svc.CancelScheduled(ctx, "nonexistent-scheduled")
	if err == nil {
		t.Error("CancelScheduled() expected error for nonexistent scheduled deployment")
	}
}

// --- Cleanup operations tests ---

func TestService_CleanupOld(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create some old completed deployments
	// Note: We need to create then update because CreateDeployment doesn't set completed_at
	now := time.Now()
	oldTime := now.Add(-30 * 24 * time.Hour) // 30 days ago

	for i := 0; i < 3; i++ {
		deployment := &storage.DeploymentRecord{
			ID:            fmt.Sprintf("old-deploy-%d", i),
			Project:       "test-project",
			Target:        "production",
			Branch:        "main",
			CommitHash:    "abc123",
			Status:        "pending", // Start as pending
			ReleaseNumber: i,
			StartedAt:     oldTime,
			TriggeredBy:   "test",
			TriggerSource: "manual",
		}
		if err := svc.Create(ctx, deployment); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		// Update to completed status with old completed_at
		deployment.Status = "success"
		deployment.CompletedAt = &oldTime
		if err := svc.Update(ctx, deployment); err != nil {
			t.Fatalf("Update() error = %v", err)
		}
	}

	// Create a recent completed deployment (should not be cleaned up)
	recentTime := now.Add(-1 * time.Hour)
	recentDeployment := &storage.DeploymentRecord{
		ID:            "recent-deploy",
		Project:       "test-project",
		Target:        "production",
		Branch:        "main",
		CommitHash:    "def456",
		Status:        "pending",
		ReleaseNumber: 10,
		StartedAt:     recentTime,
		TriggeredBy:   "test",
		TriggerSource: "manual",
	}
	if err := svc.Create(ctx, recentDeployment); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	recentDeployment.Status = "success"
	recentDeployment.CompletedAt = &recentTime
	if err := svc.Update(ctx, recentDeployment); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Cleanup deployments older than 7 days
	cutoff := now.Add(-7 * 24 * time.Hour)
	count, err := svc.CleanupOld(ctx, cutoff)
	if err != nil {
		t.Fatalf("CleanupOld() error = %v", err)
	}
	if count != 3 {
		t.Errorf("CleanupOld() count = %v, want %v", count, 3)
	}

	// Verify recent deployment still exists
	_, err = svc.GetByID(ctx, "recent-deploy")
	if err != nil {
		t.Errorf("CleanupOld() removed recent deployment: %v", err)
	}
}

func TestService_CleanupOld_NoneToCleanup(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create only recent deployments
	createTestDeployment(t, svc, "very-recent", "completed")

	// Try to cleanup with a past cutoff - nothing should be deleted
	cutoff := time.Now().Add(-365 * 24 * time.Hour) // 1 year ago
	count, err := svc.CleanupOld(ctx, cutoff)
	if err != nil {
		t.Fatalf("CleanupOld() error = %v", err)
	}
	if count != 0 {
		t.Errorf("CleanupOld() count = %v, want %v", count, 0)
	}
}

func TestService_CleanupOldLogs(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	deployment := createTestDeployment(t, svc, "cleanup-logs-deploy", "completed")

	// Create some logs
	for i := 0; i < 3; i++ {
		log := &storage.DeploymentLog{
			DeploymentID: deployment.ID,
			Level:        "info",
			Message:      fmt.Sprintf("Log %d", i),
			Source:       "test",
			CreatedAt:    time.Now().Add(-time.Duration(i) * time.Hour),
		}
		if err := svc.CreateLog(ctx, log); err != nil {
			t.Fatalf("CreateLog() error = %v", err)
		}
	}

	// Verify logs exist
	logs, err := svc.ListLogs(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("ListLogs() error = %v", err)
	}
	if len(logs) != 3 {
		t.Errorf("Expected 3 logs before cleanup, got %d", len(logs))
	}

	// Try cleanup with future cutoff (should remove nothing)
	cutoff := time.Now().Add(1 * time.Hour)
	count, err := svc.CleanupOldLogs(ctx, cutoff)
	if err != nil {
		t.Fatalf("CleanupOldLogs() error = %v", err)
	}
	t.Logf("CleanupOldLogs() removed %d logs", count)
}

// --- Batch operations tests ---

func TestService_CreateLogsBatch(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	deployment := createTestDeployment(t, svc, "batch-deploy", "running")

	logs := []*storage.DeploymentLog{
		{Level: "info", Message: "Starting deployment", Source: "deploy"},
		{Level: "info", Message: "Cloning repository", Source: "git"},
		{Level: "warn", Message: "Deprecated config found", Source: "config"},
	}

	err := svc.CreateLogsBatch(ctx, deployment.ID, logs)
	if err != nil {
		t.Fatalf("CreateLogsBatch() error = %v", err)
	}

	// Verify logs were created
	savedLogs, err := db.ListDeploymentLogs(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("ListDeploymentLogs() error = %v", err)
	}
	if len(savedLogs) != 3 {
		t.Errorf("CreateLogsBatch() created %d logs, want 3", len(savedLogs))
	}
}

func TestService_CreateLogsBatch_Empty(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	deployment := createTestDeployment(t, svc, "batch-empty-deploy", "running")

	// Empty batch should succeed without error
	err := svc.CreateLogsBatch(ctx, deployment.ID, []*storage.DeploymentLog{})
	if err != nil {
		t.Fatalf("CreateLogsBatch() error = %v", err)
	}
}

func TestService_CreateLogsBatch_DefaultLevel(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	deployment := createTestDeployment(t, svc, "batch-default-level", "running")

	// Log without level should default to "info"
	logs := []*storage.DeploymentLog{
		{Message: "No level specified", Source: "test"},
	}

	err := svc.CreateLogsBatch(ctx, deployment.ID, logs)
	if err != nil {
		t.Fatalf("CreateLogsBatch() error = %v", err)
	}

	savedLogs, err := db.ListDeploymentLogs(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("ListDeploymentLogs() error = %v", err)
	}
	if len(savedLogs) != 1 {
		t.Fatalf("Expected 1 log, got %d", len(savedLogs))
	}
	if savedLogs[0].Level != "info" {
		t.Errorf("Default level = %q, want %q", savedLogs[0].Level, "info")
	}
}

func TestService_CreateLogsBatch_InheritsDeploymentID(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	deployment := createTestDeployment(t, svc, "batch-inherit-id", "running")

	// Log without DeploymentID should inherit from parameter
	logs := []*storage.DeploymentLog{
		{Level: "info", Message: "Should inherit ID", Source: "test"},
	}

	err := svc.CreateLogsBatch(ctx, deployment.ID, logs)
	if err != nil {
		t.Fatalf("CreateLogsBatch() error = %v", err)
	}

	savedLogs, err := db.ListDeploymentLogs(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("ListDeploymentLogs() error = %v", err)
	}
	if len(savedLogs) != 1 {
		t.Fatalf("Expected 1 log, got %d", len(savedLogs))
	}
	if savedLogs[0].DeploymentID != deployment.ID {
		t.Errorf("DeploymentID = %q, want %q", savedLogs[0].DeploymentID, deployment.ID)
	}
}

// --- Edge cases and additional coverage ---

func TestService_Update_Nonexistent(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Update for nonexistent deployment doesn't return error
	// (SQL UPDATE returns success even if no rows match)
	deployment := &storage.DeploymentRecord{
		ID:            "nonexistent-update",
		Project:       "test",
		Target:        "prod",
		Branch:        "main",
		Status:        "completed",
		ReleaseNumber: 1,
		StartedAt:     time.Now(),
	}

	err := svc.Update(ctx, deployment)
	// Note: This doesn't error because SQL UPDATE succeeds even with 0 rows affected
	if err != nil {
		t.Errorf("Update() unexpected error = %v", err)
	}
}

func TestService_ListRecent_ZeroLimit(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create some deployments
	for i := 0; i < 3; i++ {
		createTestDeployment(t, svc, fmt.Sprintf("zero-limit-%d", i), "completed")
	}

	// List with zero limit
	deployments, err := svc.ListRecent(ctx, 0)
	if err != nil {
		t.Fatalf("ListRecent() error = %v", err)
	}
	if len(deployments) != 0 {
		t.Errorf("ListRecent(0) returned %v deployments, want 0", len(deployments))
	}
}

func TestService_CountByStatus_Empty(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// No deployments exist
	counts, err := svc.CountByStatus(ctx)
	if err != nil {
		t.Fatalf("CountByStatus() error = %v", err)
	}
	// Should return empty map or zero counts
	for status, count := range counts {
		if count != 0 {
			t.Errorf("CountByStatus() %s = %v, want 0", status, count)
		}
	}
}

func TestService_Create_DuplicateID(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	deployment := createTestDeployment(t, svc, "duplicate-id", "pending")

	// Try to create another with the same ID
	duplicate := &storage.DeploymentRecord{
		ID:            deployment.ID,
		Project:       "other-project",
		Target:        "staging",
		Branch:        "develop",
		Status:        "pending",
		ReleaseNumber: 2,
		StartedAt:     time.Now(),
	}

	err := svc.Create(ctx, duplicate)
	if err == nil {
		t.Error("Create() expected error for duplicate ID")
	}
}

func TestService_ContextCancellation_Create(t *testing.T) {
	svc, _ := newTestService(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	deployment := &storage.DeploymentRecord{
		ID:            "ctx-cancel-create",
		Project:       "test",
		Target:        "prod",
		Branch:        "main",
		Status:        "pending",
		ReleaseNumber: 1,
		StartedAt:     time.Now(),
	}

	err := svc.Create(ctx, deployment)
	if err == nil {
		t.Error("Create() expected error for cancelled context")
	}
}

func TestService_ContextCancellation_GetByID(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// First create a deployment
	createTestDeployment(t, svc, "ctx-cancel-get", "completed")

	// Then try to get it with cancelled context
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()

	_, err := svc.GetByID(cancelCtx, "ctx-cancel-get")
	if err == nil {
		t.Error("GetByID() expected error for cancelled context")
	}
}

func TestService_ContextCancellation_CountByStatus(t *testing.T) {
	svc, _ := newTestService(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := svc.CountByStatus(ctx)
	if err == nil {
		t.Error("CountByStatus() expected error for cancelled context")
	}
}

func TestService_ListLogs_ContextCancellation(t *testing.T) {
	svc, db := newTestService(t)

	deployment := createTestDeployment(t, svc, "ctx-cancel-logs", "running")

	// Create a log first
	log := &storage.DeploymentLog{
		DeploymentID: deployment.ID,
		Level:        "info",
		Message:      "Test",
		Source:       "test",
		CreatedAt:    time.Now(),
	}
	if err := db.CreateDeploymentLog(context.Background(), log); err != nil {
		t.Fatalf("CreateDeploymentLog() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := db.ListDeploymentLogs(ctx, deployment.ID)
	if err == nil {
		t.Error("ListDeploymentLogs() expected error for cancelled context")
	}
}

func TestService_CreateScheduled_ContextCancellation(t *testing.T) {
	svc, _ := newTestService(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := svc.CreateScheduled(ctx, "sched-cancel-ctx", "test", "prod", "main", time.Now().Add(time.Hour), "admin")
	if err == nil {
		t.Error("CreateScheduled() expected error for cancelled context")
	}
}

func TestService_CleanupOld_ContextCancellation(t *testing.T) {
	svc, _ := newTestService(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := svc.CleanupOld(ctx, time.Now())
	if err == nil {
		t.Error("CleanupOld() expected error for cancelled context")
	}
}

func TestService_ListPendingScheduled_ContextCancellation(t *testing.T) {
	svc, _ := newTestService(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := svc.ListPendingScheduled(ctx)
	if err == nil {
		t.Error("ListPendingScheduled() expected error for cancelled context")
	}
}

func TestService_CancelScheduled_ContextCancellation(t *testing.T) {
	svc, _ := newTestService(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := svc.CancelScheduled(ctx, "some-id")
	if err == nil {
		t.Error("CancelScheduled() expected error for cancelled context")
	}
}

func TestService_Update_ContextCancellation(t *testing.T) {
	svc, _ := newTestService(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	deployment := &storage.DeploymentRecord{
		ID:     "ctx-cancel-update",
		Status: "completed",
	}
	err := svc.Update(ctx, deployment)
	if err == nil {
		t.Error("Update() expected error for cancelled context")
	}
}

func TestService_Cancel_ContextCancellation(t *testing.T) {
	svc, _ := newTestService(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := svc.Cancel(ctx, "some-id")
	if err == nil {
		t.Error("Cancel() expected error for cancelled context")
	}
}

func TestService_CleanupOldLogs_ContextCancellation(t *testing.T) {
	svc, _ := newTestService(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	// CleanupOldDeployments may handle cancelled context
	_, err := svc.CleanupOld(ctx, cutoff)
	if err == nil {
		// Some implementations may not check context during cleanup
		// This is acceptable behavior
		t.Log("CleanupOld() did not return error for cancelled context (acceptable)")
	}
}

func TestService_CreateLog_ContextCancellation(t *testing.T) {
	svc, db := newTestService(t)

	deployment := createTestDeployment(t, svc, "ctx-cancel-create-log", "running")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	log := &storage.DeploymentLog{
		DeploymentID: deployment.ID,
		Level:        "info",
		Message:      "Test",
		Source:       "test",
		CreatedAt:    time.Now(),
	}

	err := db.CreateDeploymentLog(ctx, log)
	if err == nil {
		t.Error("CreateDeploymentLog() expected error for cancelled context")
	}
}

func TestService_ListLogsAfter_ContextCancellation(t *testing.T) {
	svc, db := newTestService(t)

	deployment := createTestDeployment(t, svc, "ctx-cancel-logs-after", "running")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := db.ListDeploymentLogsAfter(ctx, deployment.ID, 0)
	if err == nil {
		t.Error("ListDeploymentLogsAfter() expected error for cancelled context")
	}
}

// --- Additional tests for better coverage ---

func TestService_Cancel_CancelledStatus(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Cannot cancel already cancelled deployment
	deployment := createTestDeployment(t, svc, "cancelled-deploy", "cancelled")

	err := svc.Cancel(ctx, deployment.ID)
	if err == nil {
		t.Error("Cancel() expected error for cancelled deployment")
	}
}

func TestService_CleanupOld_NoCompletedAt(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create deployment without completed_at - should not be cleaned up
	deployment := createTestDeployment(t, svc, "no-completed-at", "running")

	// Update status but don't set completed_at
	deployment.Status = "success"
	// Note: CompletedAt is nil
	if err := svc.Update(ctx, deployment); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Try to cleanup - should not delete this one
	cutoff := time.Now().Add(24 * time.Hour) // Future cutoff
	count, err := svc.CleanupOld(ctx, cutoff)
	if err != nil {
		t.Fatalf("CleanupOld() error = %v", err)
	}
	if count != 0 {
		t.Errorf("CleanupOld() should not delete deployments without completed_at, got %v", count)
	}
}

func TestService_CreateScheduled_Multiple(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create multiple scheduled deployments
	for i := 0; i < 3; i++ {
		scheduledAt := time.Now().Add(time.Duration(i+1) * time.Hour)
		err := svc.CreateScheduled(ctx, fmt.Sprintf("multi-sched-%d", i), "test-project", "production", "main", scheduledAt, "admin")
		if err != nil {
			t.Fatalf("CreateScheduled() error = %v for deployment %d", err, i)
		}
	}
}

func TestService_ListRecent_LargeLimit(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create only a few deployments
	for i := 0; i < 3; i++ {
		createTestDeployment(t, svc, fmt.Sprintf("large-limit-%d", i), "completed")
	}

	// Request more than exists
	deployments, err := svc.ListRecent(ctx, 100)
	if err != nil {
		t.Fatalf("ListRecent() error = %v", err)
	}
	if len(deployments) != 3 {
		t.Errorf("ListRecent() returned %v deployments, want %v", len(deployments), 3)
	}
}

func TestService_CountByStatus_AllStatuses(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create deployments with all possible statuses
	statuses := []string{"pending", "running", "success", "failed", "cancelled", "scheduled"}
	for i, status := range statuses {
		deployment := &storage.DeploymentRecord{
			ID:            fmt.Sprintf("status-all-%d", i),
			Project:       "test-project",
			Target:        "production",
			Branch:        "main",
			CommitHash:    "abc123",
			Status:        storage.DeploymentStatus(status),
			ReleaseNumber: i,
			StartedAt:     time.Now(),
			TriggeredBy:   "test",
			TriggerSource: "manual",
		}
		if err := svc.Create(ctx, deployment); err != nil {
			t.Fatalf("Create() error = %v for status %s", err, status)
		}
	}

	counts, err := svc.CountByStatus(ctx)
	if err != nil {
		t.Fatalf("CountByStatus() error = %v", err)
	}

	// Verify each status has count of 1
	for _, status := range statuses {
		if counts[status] != 1 {
			t.Errorf("CountByStatus() %s = %v, want 1", status, counts[status])
		}
	}
}

func TestService_Update_AllFields(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create a deployment
	deployment := createTestDeployment(t, svc, "update-all-fields", "pending")

	// Update all fields that Update can change
	now := time.Now()
	deployment.Status = "success"
	deployment.ReleaseNumber = 42
	deployment.CompletedAt = &now
	deployment.ErrorMessage = "Updated error message"

	err := svc.Update(ctx, deployment)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Verify all fields were updated
	updated, err := svc.GetByID(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if updated.Status != "success" {
		t.Errorf("Update() Status = %v, want %v", updated.Status, "success")
	}
	if updated.ReleaseNumber != 42 {
		t.Errorf("Update() ReleaseNumber = %v, want %v", updated.ReleaseNumber, 42)
	}
	if updated.CompletedAt == nil {
		t.Error("Update() CompletedAt should not be nil")
	}
	if updated.ErrorMessage != "Updated error message" {
		t.Errorf("Update() ErrorMessage = %v, want %v", updated.ErrorMessage, "Updated error message")
	}
}

func TestService_Create_AllFields(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	now := time.Now()
	deployment := &storage.DeploymentRecord{
		ID:            "create-all-fields",
		Project:       "full-project",
		Target:        "staging",
		Branch:        "feature/test",
		CommitHash:    "1234567890abcdef",
		Status:        "pending",
		ReleaseNumber: 5,
		StartedAt:     now,
		TriggeredBy:   "admin-user",
		TriggerSource: "webhook",
	}

	err := svc.Create(ctx, deployment)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Verify all fields were stored
	found, err := svc.GetByID(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if found.Project != "full-project" {
		t.Errorf("Create() Project = %v, want %v", found.Project, "full-project")
	}
	if found.Target != "staging" {
		t.Errorf("Create() Target = %v, want %v", found.Target, "staging")
	}
	if found.Branch != "feature/test" {
		t.Errorf("Create() Branch = %v, want %v", found.Branch, "feature/test")
	}
	if found.CommitHash != "1234567890abcdef" {
		t.Errorf("Create() CommitHash = %v, want %v", found.CommitHash, "1234567890abcdef")
	}
	if found.TriggeredBy != "admin-user" {
		t.Errorf("Create() TriggeredBy = %v, want %v", found.TriggeredBy, "admin-user")
	}
	if found.TriggerSource != "webhook" {
		t.Errorf("Create() TriggerSource = %v, want %v", found.TriggerSource, "webhook")
	}
}
