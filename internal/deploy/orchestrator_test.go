package deploy

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/xid"
)

func newTestOrchestrator() (*Orchestrator, *testCallbacks) {
	cb := &testCallbacks{
		statusChanges: make(map[string][]DeploymentStatus),
		logs:          make(map[string][]LogEntry),
	}

	orch := NewOrchestrator(OrchestratorConfig{
		OnStatusChange: func(deploymentID string, status DeploymentStatus) {
			cb.mutex.Lock()
			cb.statusChanges[deploymentID] = append(cb.statusChanges[deploymentID], status)
			cb.mutex.Unlock()
		},
		OnLog: func(deploymentID string, entry LogEntry) {
			cb.mutex.Lock()
			cb.logs[deploymentID] = append(cb.logs[deploymentID], entry)
			cb.mutex.Unlock()
		},
		SendCommand: func(agentID string, cmd *DeployCommand) error {
			cb.mutex.Lock()
			cb.sentCommands = append(cb.sentCommands, cmd)
			cb.mutex.Unlock()
			return cb.sendError
		},
	})

	return orch, cb
}

type testCallbacks struct {
	mutex         sync.Mutex
	statusChanges map[string][]DeploymentStatus
	logs          map[string][]LogEntry
	sentCommands  []*DeployCommand
	sendError     error
}

func TestOrchestrator_Deploy(t *testing.T) {
	orch, cb := newTestOrchestrator()
	ctx := context.Background()

	req := &DeployRequest{
		ProjectID:     "my-project",
		TargetID:      "production",
		AgentID:       "agent-1",
		Repository:    "git@github.com:example/repo.git",
		Branch:        "main",
		Commit:        "abc123",
		Path:          "/var/www/my-project",
		TriggeredBy:   "user1",
		TriggerSource: "api",
	}

	deployment, err := orch.Deploy(ctx, req)
	if err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}

	if deployment.ID == "" {
		t.Error("Expected deployment ID")
	}

	if deployment.ProjectID != "my-project" {
		t.Errorf("Expected project 'my-project', got %q", deployment.ProjectID)
	}

	if deployment.Status != StatusQueued {
		t.Errorf("Expected status 'queued', got %q", deployment.Status)
	}

	// Check command was sent
	cb.mutex.Lock()
	if len(cb.sentCommands) != 1 {
		t.Errorf("Expected 1 sent command, got %d", len(cb.sentCommands))
	}
	if len(cb.sentCommands) > 0 && cb.sentCommands[0].DeploymentID != deployment.ID {
		t.Errorf("Command deployment ID mismatch")
	}
	cb.mutex.Unlock()

	// Check status changes
	cb.mutex.Lock()
	changes := cb.statusChanges[deployment.ID]
	cb.mutex.Unlock()
	if len(changes) != 2 { // pending, queued
		t.Errorf("Expected 2 status changes, got %d", len(changes))
	}
}

func TestOrchestrator_Deploy_ValidationErrors(t *testing.T) {
	orch, _ := newTestOrchestrator()
	ctx := context.Background()

	tests := []struct {
		name    string
		req     *DeployRequest
		wantErr string
	}{
		{
			name:    "missing project_id",
			req:     &DeployRequest{AgentID: "agent-1", Repository: "repo"},
			wantErr: "project_id is required",
		},
		{
			name:    "missing agent_id",
			req:     &DeployRequest{ProjectID: "proj", Repository: "repo"},
			wantErr: "agent_id is required",
		},
		{
			name:    "missing repository",
			req:     &DeployRequest{ProjectID: "proj", AgentID: "agent-1"},
			wantErr: "repository is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := orch.Deploy(ctx, tt.req)
			if err == nil {
				t.Fatal("Expected error")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("Expected error %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestOrchestrator_Deploy_SendCommandError(t *testing.T) {
	orch, cb := newTestOrchestrator()
	cb.sendError = errors.New("agent not connected")
	ctx := context.Background()

	req := &DeployRequest{
		ProjectID:  "my-project",
		AgentID:    "agent-1",
		Repository: "git@github.com:example/repo.git",
	}

	deployment, err := orch.Deploy(ctx, req)
	if err == nil {
		t.Fatal("Expected error")
	}

	// Deployment should exist with failed status
	if deployment == nil {
		t.Fatal("Expected deployment even on error")
	}
	if deployment.Status != StatusFailed {
		t.Errorf("Expected status 'failed', got %q", deployment.Status)
	}
}

func TestOrchestrator_Rollback(t *testing.T) {
	orch, _ := newTestOrchestrator()
	ctx := context.Background()

	req := &RollbackRequest{
		ProjectID:     "my-project",
		TargetID:      "production",
		AgentID:       "agent-1",
		Path:          "/var/www/my-project",
		ReleaseNumber: 5,
		TriggeredBy:   "user1",
	}

	deployment, err := orch.Rollback(ctx, req)
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	if deployment.ID == "" {
		t.Error("Expected deployment ID")
	}

	if deployment.Status != StatusRollingBack {
		t.Errorf("Expected status 'rolling_back', got %q", deployment.Status)
	}
}

func TestOrchestrator_Cancel(t *testing.T) {
	orch, _ := newTestOrchestrator()
	ctx := context.Background()

	req := &DeployRequest{
		ProjectID:  "my-project",
		AgentID:    "agent-1",
		Repository: "git@github.com:example/repo.git",
	}

	deployment, _ := orch.Deploy(ctx, req)

	// Cancel it
	err := orch.Cancel(deployment.ID, "user requested")
	if err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	// Check status
	d, ok := orch.GetDeployment(deployment.ID)
	if !ok {
		t.Fatal("Deployment not found")
	}
	if d.Status != StatusCancelled {
		t.Errorf("Expected status 'cancelled', got %q", d.Status)
	}
}

func TestOrchestrator_Cancel_NotFound(t *testing.T) {
	orch, _ := newTestOrchestrator()

	err := orch.Cancel("nonexistent", "reason")
	if err == nil {
		t.Fatal("Expected error")
	}
}

func TestOrchestrator_Cancel_AlreadyCompleted(t *testing.T) {
	orch, _ := newTestOrchestrator()
	ctx := context.Background()

	req := &DeployRequest{
		ProjectID:  "my-project",
		AgentID:    "agent-1",
		Repository: "git@github.com:example/repo.git",
	}

	deployment, _ := orch.Deploy(ctx, req)

	// Mark as completed
	orch.UpdateDeploymentStatus(deployment.ID, StatusCompleted, 100, "done")

	// Try to cancel
	err := orch.Cancel(deployment.ID, "reason")
	if err == nil {
		t.Fatal("Expected error")
	}
}

func TestOrchestrator_GetDeployment(t *testing.T) {
	orch, _ := newTestOrchestrator()
	ctx := context.Background()

	// Not found
	_, ok := orch.GetDeployment("nonexistent")
	if ok {
		t.Error("Expected not found")
	}

	// Create deployment
	req := &DeployRequest{
		ProjectID:  "my-project",
		AgentID:    "agent-1",
		Repository: "git@github.com:example/repo.git",
	}
	created, _ := orch.Deploy(ctx, req)

	// Found
	d, ok := orch.GetDeployment(created.ID)
	if !ok {
		t.Fatal("Expected to find deployment")
	}
	if d.ID != created.ID {
		t.Errorf("ID mismatch")
	}
}

func TestOrchestrator_ListDeployments(t *testing.T) {
	orch, _ := newTestOrchestrator()
	ctx := context.Background()

	// Empty list
	deployments := orch.ListDeployments()
	if len(deployments) != 0 {
		t.Errorf("Expected 0 deployments, got %d", len(deployments))
	}

	// Create some deployments
	for i := 0; i < 3; i++ {
		req := &DeployRequest{
			ProjectID:  "my-project",
			AgentID:    "agent-1",
			Repository: "git@github.com:example/repo.git",
		}
		_, _ = orch.Deploy(ctx, req)
	}

	deployments = orch.ListDeployments()
	if len(deployments) != 3 {
		t.Errorf("Expected 3 deployments, got %d", len(deployments))
	}
}

func TestOrchestrator_ListProjectDeployments(t *testing.T) {
	orch, _ := newTestOrchestrator()
	ctx := context.Background()

	// Create deployments for different projects
	for i := 0; i < 3; i++ {
		_, _ = orch.Deploy(ctx, &DeployRequest{
			ProjectID:  "project-a",
			AgentID:    "agent-1",
			Repository: "repo",
		})
	}
	for i := 0; i < 2; i++ {
		_, _ = orch.Deploy(ctx, &DeployRequest{
			ProjectID:  "project-b",
			AgentID:    "agent-1",
			Repository: "repo",
		})
	}

	// List project-a
	deploymentsA := orch.ListProjectDeployments("project-a")
	if len(deploymentsA) != 3 {
		t.Errorf("Expected 3 deployments for project-a, got %d", len(deploymentsA))
	}

	// List project-b
	deploymentsB := orch.ListProjectDeployments("project-b")
	if len(deploymentsB) != 2 {
		t.Errorf("Expected 2 deployments for project-b, got %d", len(deploymentsB))
	}

	// List nonexistent
	deploymentsC := orch.ListProjectDeployments("project-c")
	if len(deploymentsC) != 0 {
		t.Errorf("Expected 0 deployments for project-c, got %d", len(deploymentsC))
	}
}

func TestOrchestrator_UpdateDeploymentStatus(t *testing.T) {
	orch, cb := newTestOrchestrator()
	ctx := context.Background()

	req := &DeployRequest{
		ProjectID:  "my-project",
		AgentID:    "agent-1",
		Repository: "repo",
	}
	deployment, _ := orch.Deploy(ctx, req)

	// Update status
	orch.UpdateDeploymentStatus(deployment.ID, StatusDeploying, 50, "copying files")

	d, _ := orch.GetDeployment(deployment.ID)
	if d.Status != StatusDeploying {
		t.Errorf("Expected status 'deploying', got %q", d.Status)
	}
	if d.Progress != 50 {
		t.Errorf("Expected progress 50, got %d", d.Progress)
	}
	if d.CurrentStep != "copying files" {
		t.Errorf("Expected step 'copying files', got %q", d.CurrentStep)
	}
	if d.StartedAt == nil {
		t.Error("Expected StartedAt to be set")
	}

	// Complete
	orch.UpdateDeploymentStatus(deployment.ID, StatusCompleted, 100, "done")

	d, _ = orch.GetDeployment(deployment.ID)
	if d.CompletedAt == nil {
		t.Error("Expected CompletedAt to be set")
	}

	// Check callbacks
	cb.mutex.Lock()
	changes := cb.statusChanges[deployment.ID]
	cb.mutex.Unlock()
	// pending, queued, deploying, completed
	if len(changes) < 4 {
		t.Errorf("Expected at least 4 status changes, got %d", len(changes))
	}
}

func TestOrchestrator_AddDeploymentLog(t *testing.T) {
	orch, cb := newTestOrchestrator()
	ctx := context.Background()

	req := &DeployRequest{
		ProjectID:  "my-project",
		AgentID:    "agent-1",
		Repository: "repo",
	}
	deployment, _ := orch.Deploy(ctx, req)

	// Add log entry
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     LogInfo,
		Message:   "Test message",
		Source:    "test",
	}
	orch.AddDeploymentLog(deployment.ID, entry)

	// Check deployment logs
	d, _ := orch.GetDeployment(deployment.ID)
	if len(d.Logs) != 1 {
		t.Errorf("Expected 1 log entry, got %d", len(d.Logs))
	}
	if d.Logs[0].Message != "Test message" {
		t.Errorf("Log message mismatch")
	}

	// Check callback
	cb.mutex.Lock()
	logs := cb.logs[deployment.ID]
	cb.mutex.Unlock()
	if len(logs) != 1 {
		t.Errorf("Expected 1 log callback, got %d", len(logs))
	}
}

func TestOrchestrator_SetReleaseNumber(t *testing.T) {
	orch, _ := newTestOrchestrator()
	ctx := context.Background()

	req := &DeployRequest{
		ProjectID:  "my-project",
		AgentID:    "agent-1",
		Repository: "repo",
	}
	deployment, _ := orch.Deploy(ctx, req)

	orch.SetReleaseNumber(deployment.ID, 42)

	d, _ := orch.GetDeployment(deployment.ID)
	if d.ReleaseNumber != 42 {
		t.Errorf("Expected release number 42, got %d", d.ReleaseNumber)
	}
}

func TestOrchestrator_SetError(t *testing.T) {
	orch, _ := newTestOrchestrator()
	ctx := context.Background()

	req := &DeployRequest{
		ProjectID:  "my-project",
		AgentID:    "agent-1",
		Repository: "repo",
	}
	deployment, _ := orch.Deploy(ctx, req)

	testErr := errors.New("deployment failed")
	orch.SetError(deployment.ID, testErr)

	d, _ := orch.GetDeployment(deployment.ID)
	if d.Status != StatusFailed {
		t.Errorf("Expected status 'failed', got %q", d.Status)
	}
	if d.Error == nil || d.Error.Error() != "deployment failed" {
		t.Error("Expected error to be set")
	}
}

func TestOrchestrator_CleanupOldDeployments(t *testing.T) {
	orch, _ := newTestOrchestrator()
	ctx := context.Background()

	// Create some deployments
	for i := 0; i < 5; i++ {
		req := &DeployRequest{
			ProjectID:  "my-project",
			AgentID:    "agent-1",
			Repository: "repo",
		}
		d, _ := orch.Deploy(ctx, req)

		// Mark some as completed
		if i < 3 {
			orch.UpdateDeploymentStatus(d.ID, StatusCompleted, 100, "done")
			// Simulate old completion time
			deployment, _ := orch.GetDeployment(d.ID)
			oldTime := time.Now().Add(-2 * time.Hour)
			deployment.CompletedAt = &oldTime
		}
	}

	// Should have 5 deployments
	if len(orch.ListDeployments()) != 5 {
		t.Errorf("Expected 5 deployments")
	}

	// Cleanup deployments older than 1 hour
	removed := orch.CleanupOldDeployments(1 * time.Hour)
	if removed != 3 {
		t.Errorf("Expected 3 removed, got %d", removed)
	}

	// Should have 2 remaining
	if len(orch.ListDeployments()) != 2 {
		t.Errorf("Expected 2 deployments remaining")
	}
}

func TestDeploymentIDIsXID(t *testing.T) {
	ids := make(map[string]bool)

	for i := 0; i < 100; i++ {
		id := xid.New().String()

		// XID should be exactly 20 characters
		if len(id) != 20 {
			t.Errorf("Expected 20-char XID, got %d chars: %q", len(id), id)
		}

		if ids[id] {
			t.Error("Generated duplicate ID")
		}
		ids[id] = true
	}
}

// --- Additional tests for improved coverage ---

func TestOrchestrator_Rollback_ValidationErrors(t *testing.T) {
	orch, _ := newTestOrchestrator()
	ctx := context.Background()

	tests := []struct {
		name    string
		req     *RollbackRequest
		wantErr string
	}{
		{
			name:    "missing project_id",
			req:     &RollbackRequest{AgentID: "agent-1"},
			wantErr: "project_id is required",
		},
		{
			name:    "missing agent_id",
			req:     &RollbackRequest{ProjectID: "proj"},
			wantErr: "agent_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := orch.Rollback(ctx, tt.req)
			if err == nil {
				t.Fatal("Expected error")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("Expected error %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestOrchestrator_UpdateNonExistent(t *testing.T) {
	orch, _ := newTestOrchestrator()

	// These should not panic or error, just be no-ops
	orch.UpdateDeploymentStatus("nonexistent", StatusDeploying, 50, "step")
	orch.AddDeploymentLog("nonexistent", LogEntry{Message: "test"})
	orch.SetReleaseNumber("nonexistent", 42)
	orch.SetError("nonexistent", errors.New("test"))
}

func TestOrchestrator_StatusTransitions(t *testing.T) {
	orch, cb := newTestOrchestrator()
	ctx := context.Background()

	req := &DeployRequest{
		ProjectID:  "my-project",
		AgentID:    "agent-1",
		Repository: "repo",
	}
	deployment, _ := orch.Deploy(ctx, req)

	// Track all status transitions
	statuses := []DeploymentStatus{
		StatusPreparing,
		StatusCloning,
		StatusBuilding,
		StatusDeploying,
		StatusVerifying,
		StatusCompleted,
	}

	for i, status := range statuses {
		orch.UpdateDeploymentStatus(deployment.ID, status, (i+1)*20, string(status))
	}

	d, _ := orch.GetDeployment(deployment.ID)

	if d.Status != StatusCompleted {
		t.Errorf("Final status = %q, want completed", d.Status)
	}
	if d.Progress != 120 { // 6 * 20
		t.Errorf("Progress = %d, want 120", d.Progress)
	}

	// Verify callbacks were called
	cb.mutex.Lock()
	changes := cb.statusChanges[deployment.ID]
	cb.mutex.Unlock()

	// pending, queued, then all 6 status updates
	if len(changes) < 8 {
		t.Errorf("Expected at least 8 status changes, got %d", len(changes))
	}
}

func TestOrchestrator_DeployWithAllOptions(t *testing.T) {
	orch, cb := newTestOrchestrator()
	ctx := context.Background()

	req := &DeployRequest{
		ProjectID:  "full-project",
		TargetID:   "production",
		AgentID:    "agent-1",
		Repository: "git@github.com:example/repo.git",
		Branch:     "main",
		Commit:     "abc123",
		Path:       "/var/www/app",
		Settings: DeploySettings{
			Strategy:       "symlink",
			KeepReleases:   5,
			SharedDirs:     []string{"storage", "logs"},
			SharedFiles:    []string{".env"},
			WritableDirs:   []string{"cache"},
			ExecutionUser:  "www-data",
			ExecutionGroup: "www-data",
			Timeout:        5 * time.Minute,
		},
		EnvVars: map[string]string{
			"APP_ENV": "production",
		},
		EnvFileContent:  []byte("SECRET=value"),
		PreDeployHooks:  []string{"composer install"},
		PostDeployHooks: []string{"php artisan migrate"},
		ReloadServices: []ServiceReload{
			{Service: "php-fpm", Action: "reload"},
		},
		TriggeredBy:   "user@example.com",
		TriggerSource: "webhook",
	}

	deployment, err := orch.Deploy(ctx, req)
	if err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}

	if deployment.ProjectID != "full-project" {
		t.Errorf("ProjectID = %q, want full-project", deployment.ProjectID)
	}
	if deployment.TargetID != "production" {
		t.Errorf("TargetID = %q, want production", deployment.TargetID)
	}
	if deployment.TriggeredBy != "user@example.com" {
		t.Errorf("TriggeredBy = %q, want user@example.com", deployment.TriggeredBy)
	}
	if deployment.TriggerSource != "webhook" {
		t.Errorf("TriggerSource = %q, want webhook", deployment.TriggerSource)
	}

	// Verify command was sent with all options
	cb.mutex.Lock()
	if len(cb.sentCommands) != 1 {
		t.Errorf("Expected 1 command, got %d", len(cb.sentCommands))
	}
	if len(cb.sentCommands) > 0 {
		cmd := cb.sentCommands[0]
		if cmd.Repository != "git@github.com:example/repo.git" {
			t.Error("Command repository mismatch")
		}
		if len(cmd.Settings.SharedDirs) != 2 {
			t.Error("Command shared dirs mismatch")
		}
		if len(cmd.PreDeployHooks) != 1 {
			t.Error("Command pre-deploy hooks mismatch")
		}
	}
	cb.mutex.Unlock()
}

func TestOrchestrator_RollbackWithAllOptions(t *testing.T) {
	orch, _ := newTestOrchestrator()
	ctx := context.Background()

	req := &RollbackRequest{
		ProjectID:     "my-project",
		TargetID:      "production",
		AgentID:       "agent-1",
		Path:          "/var/www/app",
		ReleaseNumber: 3,
		RollbackHooks: []string{"php artisan down", "php artisan up"},
		ReloadServices: []ServiceReload{
			{Service: "php-fpm", Action: "reload"},
			{Service: "nginx", Action: "restart"},
		},
		TriggeredBy: "admin@example.com",
	}

	deployment, err := orch.Rollback(ctx, req)
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	if deployment.Status != StatusRollingBack {
		t.Errorf("Status = %q, want rolling_back", deployment.Status)
	}
	if deployment.TriggerSource != "rollback" {
		t.Errorf("TriggerSource = %q, want rollback", deployment.TriggerSource)
	}
	if deployment.TriggeredBy != "admin@example.com" {
		t.Errorf("TriggeredBy = %q, want admin@example.com", deployment.TriggeredBy)
	}
}

func TestOrchestrator_ConcurrentOperations(t *testing.T) {
	orch, _ := newTestOrchestrator()
	ctx := context.Background()

	var wg sync.WaitGroup
	deploymentCount := 20

	// Create many deployments concurrently
	for i := 0; i < deploymentCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req := &DeployRequest{
				ProjectID:  "concurrent-project",
				AgentID:    "agent-1",
				Repository: "repo",
			}
			_, _ = orch.Deploy(ctx, req)
		}(i)
	}

	wg.Wait()

	deployments := orch.ListDeployments()
	if len(deployments) != deploymentCount {
		t.Errorf("Expected %d deployments, got %d", deploymentCount, len(deployments))
	}
}

func TestOrchestrator_ConcurrentStatusUpdates(t *testing.T) {
	orch, _ := newTestOrchestrator()
	ctx := context.Background()

	req := &DeployRequest{
		ProjectID:  "my-project",
		AgentID:    "agent-1",
		Repository: "repo",
	}
	deployment, _ := orch.Deploy(ctx, req)

	var wg sync.WaitGroup
	updateCount := 50

	// Concurrent status updates
	for i := 0; i < updateCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			orch.UpdateDeploymentStatus(deployment.ID, StatusDeploying, idx, "step")
		}(i)
	}

	// Concurrent log additions
	for i := 0; i < updateCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			orch.AddDeploymentLog(deployment.ID, LogEntry{
				Message: "Log message",
				Level:   LogInfo,
			})
		}(i)
	}

	wg.Wait()

	d, _ := orch.GetDeployment(deployment.ID)
	if len(d.Logs) != updateCount {
		t.Errorf("Expected %d logs, got %d", updateCount, len(d.Logs))
	}
}

func TestOrchestrator_CleanupOnlyFinished(t *testing.T) {
	orch, _ := newTestOrchestrator()
	ctx := context.Background()

	// Create some deployments
	for i := 0; i < 5; i++ {
		req := &DeployRequest{
			ProjectID:  "my-project",
			AgentID:    "agent-1",
			Repository: "repo",
		}
		d, _ := orch.Deploy(ctx, req)

		// Mark some as finished (completed/failed/cancelled)
		switch i {
		case 0:
			orch.UpdateDeploymentStatus(d.ID, StatusCompleted, 100, "done")
		case 1:
			orch.SetError(d.ID, errors.New("failed"))
		case 2:
			orch.Cancel(d.ID, "cancelled")
			// 3 and 4 remain in progress
		}
	}

	// Set old completion times on finished deployments
	for _, d := range orch.ListDeployments() {
		d.mutex.Lock()
		if d.CompletedAt != nil {
			oldTime := time.Now().Add(-2 * time.Hour)
			d.CompletedAt = &oldTime
		}
		d.mutex.Unlock()
	}

	// Should have 5 deployments
	if len(orch.ListDeployments()) != 5 {
		t.Errorf("Expected 5 deployments, got %d", len(orch.ListDeployments()))
	}

	// Cleanup old deployments
	removed := orch.CleanupOldDeployments(1 * time.Hour)
	if removed != 3 {
		t.Errorf("Expected 3 removed (completed, failed, cancelled), got %d", removed)
	}

	// Should have 2 remaining (in progress)
	if len(orch.ListDeployments()) != 2 {
		t.Errorf("Expected 2 deployments remaining, got %d", len(orch.ListDeployments()))
	}
}

func TestOrchestrator_CleanupRecentDeployments(t *testing.T) {
	orch, _ := newTestOrchestrator()
	ctx := context.Background()

	// Create completed deployments
	for i := 0; i < 3; i++ {
		req := &DeployRequest{
			ProjectID:  "my-project",
			AgentID:    "agent-1",
			Repository: "repo",
		}
		d, _ := orch.Deploy(ctx, req)
		orch.UpdateDeploymentStatus(d.ID, StatusCompleted, 100, "done")
		// These are recent - CompletedAt is set to now
	}

	// Cleanup with 1 hour max age - none should be removed (all recent)
	removed := orch.CleanupOldDeployments(1 * time.Hour)
	if removed != 0 {
		t.Errorf("Expected 0 removed (all recent), got %d", removed)
	}

	if len(orch.ListDeployments()) != 3 {
		t.Errorf("Expected 3 deployments remaining, got %d", len(orch.ListDeployments()))
	}
}

func TestOrchestrator_NilCallbacks(t *testing.T) {
	// Create orchestrator with no callbacks
	orch := NewOrchestrator(OrchestratorConfig{})
	ctx := context.Background()

	req := &DeployRequest{
		ProjectID:  "my-project",
		AgentID:    "agent-1",
		Repository: "repo",
	}

	// Should not panic with nil callbacks
	deployment, err := orch.Deploy(ctx, req)
	if err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}

	orch.UpdateDeploymentStatus(deployment.ID, StatusDeploying, 50, "step")
	orch.AddDeploymentLog(deployment.ID, LogEntry{Message: "test"})

	d, _ := orch.GetDeployment(deployment.ID)
	if d.Status != StatusDeploying {
		t.Errorf("Status = %q, want deploying", d.Status)
	}
}

func TestDeployment_Fields(t *testing.T) {
	t.Parallel()

	deployment := &Deployment{
		ID:            "deploy-123",
		Progress:      50,
		ReleaseNumber: 42,
		Logs: []LogEntry{
			{Message: "Log 1", Level: LogInfo},
			{Message: "Log 2", Level: LogWarn},
		},
	}

	if deployment.ID != "deploy-123" {
		t.Errorf("ID = %q, want deploy-123", deployment.ID)
	}
	if deployment.Progress != 50 {
		t.Errorf("Progress = %d, want 50", deployment.Progress)
	}
	if deployment.ReleaseNumber != 42 {
		t.Errorf("ReleaseNumber = %d, want 42", deployment.ReleaseNumber)
	}
	if len(deployment.Logs) != 2 {
		t.Errorf("Logs count = %d, want 2", len(deployment.Logs))
	}
}

func TestDeploymentStatus_Values(t *testing.T) {
	t.Parallel()

	statuses := []DeploymentStatus{
		StatusPending,
		StatusQueued,
		StatusPreparing,
		StatusCloning,
		StatusBuilding,
		StatusDeploying,
		StatusVerifying,
		StatusCompleted,
		StatusFailed,
		StatusCancelled,
		StatusRollingBack,
	}

	expected := []string{
		"pending",
		"queued",
		"preparing",
		"cloning",
		"building",
		"deploying",
		"verifying",
		"completed",
		"failed",
		"cancelled",
		"rolling_back",
	}

	for i, status := range statuses {
		if string(status) != expected[i] {
			t.Errorf("Status %d = %q, want %q", i, status, expected[i])
		}
	}
}

func TestOrchestrator_UpdateStatus_Private(t *testing.T) {
	orch, cb := newTestOrchestrator()
	ctx := context.Background()

	req := &DeployRequest{
		ProjectID:  "my-project",
		AgentID:    "agent-1",
		Repository: "repo",
	}
	deployment, _ := orch.Deploy(ctx, req)

	// Use public SetError which internally uses updateStatusWithError
	testErr := errors.New("test failure")
	orch.SetError(deployment.ID, testErr)

	d, _ := orch.GetDeployment(deployment.ID)
	if d.Status != StatusFailed {
		t.Errorf("Status = %q, want failed", d.Status)
	}
	if d.Error == nil || d.Error.Error() != "test failure" {
		t.Error("Error should be set")
	}
	if d.CompletedAt == nil {
		t.Error("CompletedAt should be set on failure")
	}

	// Verify callback was called
	cb.mutex.Lock()
	changes := cb.statusChanges[deployment.ID]
	cb.mutex.Unlock()

	found := false
	for _, status := range changes {
		if status == StatusFailed {
			found = true
			break
		}
	}
	if !found {
		t.Error("StatusFailed should be in status changes")
	}
}

func TestOrchestrator_Cancel_Idempotent(t *testing.T) {
	orch, _ := newTestOrchestrator()
	ctx := context.Background()

	req := &DeployRequest{
		ProjectID:  "my-project",
		AgentID:    "agent-1",
		Repository: "repo",
	}
	deployment, _ := orch.Deploy(ctx, req)

	// First cancel should succeed
	err := orch.Cancel(deployment.ID, "first cancel")
	if err != nil {
		t.Fatalf("First Cancel failed: %v", err)
	}

	// Second cancel should fail (already cancelled)
	err = orch.Cancel(deployment.ID, "second cancel")
	if err == nil {
		t.Error("Second Cancel should fail")
	}
}

func TestDeploymentIDUniqueness(t *testing.T) {
	t.Parallel()

	ids := make(map[string]bool)
	iterations := 1000

	for i := 0; i < iterations; i++ {
		id := xid.New().String()

		if ids[id] {
			t.Fatalf("Duplicate ID generated after %d iterations: %s", i, id)
		}
		ids[id] = true
	}
}

func TestOrchestratorConfig_AllCallbacks(t *testing.T) {
	statusChanges := make(map[string][]DeploymentStatus)
	logs := make(map[string][]LogEntry)
	var sentCommands []*DeployCommand
	var mu sync.Mutex

	cfg := OrchestratorConfig{
		OnStatusChange: func(deploymentID string, status DeploymentStatus) {
			mu.Lock()
			statusChanges[deploymentID] = append(statusChanges[deploymentID], status)
			mu.Unlock()
		},
		OnLog: func(deploymentID string, entry LogEntry) {
			mu.Lock()
			logs[deploymentID] = append(logs[deploymentID], entry)
			mu.Unlock()
		},
		SendCommand: func(agentID string, cmd *DeployCommand) error {
			mu.Lock()
			sentCommands = append(sentCommands, cmd)
			mu.Unlock()
			return nil
		},
	}

	orch := NewOrchestrator(cfg)
	ctx := context.Background()

	req := &DeployRequest{
		ProjectID:  "test-project",
		AgentID:    "agent-1",
		Repository: "repo",
	}
	deployment, _ := orch.Deploy(ctx, req)

	// Add a log
	orch.AddDeploymentLog(deployment.ID, LogEntry{Message: "test log", Level: LogInfo})

	mu.Lock()
	defer mu.Unlock()

	if len(statusChanges[deployment.ID]) < 2 {
		t.Errorf("Expected at least 2 status changes, got %d", len(statusChanges[deployment.ID]))
	}
	if len(logs[deployment.ID]) != 1 {
		t.Errorf("Expected 1 log, got %d", len(logs[deployment.ID]))
	}
	if len(sentCommands) != 1 {
		t.Errorf("Expected 1 sent command, got %d", len(sentCommands))
	}
}

func TestOrchestrator_Rollback_Success(t *testing.T) {
	orch, _ := newTestOrchestrator()
	ctx := context.Background()

	req := &RollbackRequest{
		ProjectID:     "my-project",
		TargetID:      "staging",
		AgentID:       "agent-1",
		Path:          "/var/www/app",
		ReleaseNumber: 5,
		TriggeredBy:   "admin",
	}

	deployment, err := orch.Rollback(ctx, req)
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	if deployment == nil {
		t.Fatal("Expected deployment object")
	}

	if deployment.ID == "" {
		t.Error("Deployment ID should be set")
	}

	if deployment.Status != StatusRollingBack {
		t.Errorf("Status = %q, want rolling_back", deployment.Status)
	}

	if deployment.ProjectID != "my-project" {
		t.Errorf("ProjectID = %q, want my-project", deployment.ProjectID)
	}

	if deployment.TargetID != "staging" {
		t.Errorf("TargetID = %q, want staging", deployment.TargetID)
	}

	if deployment.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want agent-1", deployment.AgentID)
	}

	if deployment.TriggeredBy != "admin" {
		t.Errorf("TriggeredBy = %q, want admin", deployment.TriggeredBy)
	}

	if deployment.TriggerSource != "rollback" {
		t.Errorf("TriggerSource = %q, want rollback", deployment.TriggerSource)
	}
}

func TestOrchestrator_Deploy_WithSendCommand(t *testing.T) {
	var sentCommands []*DeployCommand
	var mu sync.Mutex

	cfg := OrchestratorConfig{
		SendCommand: func(agentID string, cmd *DeployCommand) error {
			mu.Lock()
			sentCommands = append(sentCommands, cmd)
			mu.Unlock()
			return nil
		},
	}

	orch := NewOrchestrator(cfg)
	ctx := context.Background()

	req := &DeployRequest{
		ProjectID:       "my-project",
		TargetID:        "production",
		AgentID:         "agent-1",
		Repository:      "git@github.com:org/repo.git",
		Branch:          "main",
		Commit:          "abc123",
		Path:            "/var/www/app",
		TriggeredBy:     "user@example.com",
		TriggerSource:   "api",
		PreDeployHooks:  []string{"npm install"},
		PostDeployHooks: []string{"npm run build"},
		Settings: DeploySettings{
			KeepReleases: 3,
		},
	}

	deployment, err := orch.Deploy(ctx, req)
	if err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(sentCommands) != 1 {
		t.Fatalf("Expected 1 sent command, got %d", len(sentCommands))
	}

	cmd := sentCommands[0]
	if cmd.DeploymentID != deployment.ID {
		t.Error("Command deployment ID should match")
	}
	if cmd.Project != "my-project" {
		t.Errorf("Command Project = %q, want my-project", cmd.Project)
	}
	if cmd.Target != "production" {
		t.Errorf("Command Target = %q, want production", cmd.Target)
	}
	if cmd.Repository != "git@github.com:org/repo.git" {
		t.Errorf("Command Repository = %q", cmd.Repository)
	}
	if cmd.Branch != "main" {
		t.Errorf("Command Branch = %q, want main", cmd.Branch)
	}
	if cmd.Commit != "abc123" {
		t.Errorf("Command Commit = %q, want abc123", cmd.Commit)
	}
	if len(cmd.PreDeployHooks) != 1 {
		t.Errorf("Command PreDeployHooks count = %d, want 1", len(cmd.PreDeployHooks))
	}
	if cmd.Settings.KeepReleases != 3 {
		t.Errorf("Command Settings.KeepReleases = %d, want 3", cmd.Settings.KeepReleases)
	}
}

func TestOrchestrator_UpdateStatusNonexistent(t *testing.T) {
	t.Parallel()

	callbackCalled := false
	orch := NewOrchestrator(OrchestratorConfig{
		OnStatusChange: func(id string, status DeploymentStatus) {
			callbackCalled = true
		},
	})

	// Try to update status for nonexistent deployment - should not panic
	orch.updateStatus("nonexistent-id", StatusPreparing)

	// Callback should not have been called
	if callbackCalled {
		t.Error("updateStatus() callback should not be called for nonexistent deployment")
	}
}

func TestOrchestrator_DeploySendCommandError(t *testing.T) {
	t.Parallel()

	orch := NewOrchestrator(OrchestratorConfig{
		SendCommand: func(agentID string, cmd *DeployCommand) error {
			return errors.New("agent unavailable")
		},
	})

	ctx := context.Background()
	req := &DeployRequest{
		ProjectID:  "project",
		AgentID:    "agent",
		Repository: "https://github.com/test/repo.git",
	}

	dep, err := orch.Deploy(ctx, req)

	if err == nil {
		t.Error("Deploy() should return error when sendCommand fails")
	}
	if !strings.Contains(err.Error(), "sending deploy command") {
		t.Errorf("Error should mention 'sending deploy command', got: %v", err)
	}

	// Deployment should still be returned
	if dep == nil {
		t.Fatal("Deployment should still be returned even on error")
	}

	// And it should be marked as failed
	if dep.Status != StatusFailed {
		t.Errorf("Deployment status = %q, want failed", dep.Status)
	}
}

func TestOrchestrator_UpdateStatusWithErrorNonexistent(t *testing.T) {
	t.Parallel()

	callbackCalled := false
	orch := NewOrchestrator(OrchestratorConfig{
		OnStatusChange: func(id string, status DeploymentStatus) {
			callbackCalled = true
		},
	})

	// Try to update status with error for nonexistent deployment - should not panic
	orch.updateStatusWithError("nonexistent-id", StatusFailed, errors.New("test error"))

	// Callback should not have been called
	if callbackCalled {
		t.Error("updateStatusWithError() callback should not be called for nonexistent deployment")
	}
}

func TestOrchestrator_SetReleaseNumberNonexistent(t *testing.T) {
	t.Parallel()

	orch := NewOrchestrator(OrchestratorConfig{})

	// Try to set release number for nonexistent deployment - should not panic
	orch.SetReleaseNumber("nonexistent-id", 5)

	// Try to get it - should return nil deployment or just not panic
	dep, ok := orch.GetDeployment("nonexistent-id")
	if ok || dep != nil {
		t.Error("GetDeployment() should return nil and false for nonexistent deployment")
	}
}

func TestOrchestrator_AddDeploymentLogNonexistent(t *testing.T) {
	t.Parallel()

	orch := NewOrchestrator(OrchestratorConfig{})
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     LogInfo,
		Message:   "test message",
	}

	// Try to add log entry for nonexistent deployment - should not panic
	orch.AddDeploymentLog("nonexistent-id", entry)
}

func TestOrchestrator_UpdateDeploymentStatusNonexistent(t *testing.T) {
	t.Parallel()

	callbackCalled := false
	orch := NewOrchestrator(OrchestratorConfig{
		OnStatusChange: func(id string, status DeploymentStatus) {
			callbackCalled = true
		},
	})

	// Try to update deployment status for nonexistent - should not panic
	orch.UpdateDeploymentStatus("nonexistent-id", StatusCompleted, 100, "done")

	if callbackCalled {
		t.Error("UpdateDeploymentStatus() callback should not be called for nonexistent deployment")
	}
}

func TestOrchestrator_DeployIDIsXID(t *testing.T) {
	t.Parallel()

	// Verify that Deploy() produces valid XID-format IDs
	orch := NewOrchestrator(OrchestratorConfig{})
	ctx := context.Background()

	req := &DeployRequest{
		ProjectID:  "project",
		AgentID:    "agent",
		Repository: "https://github.com/test/repo.git",
		Branch:     "main",
	}

	// Normal deploy should work and produce XID-format ID
	dep, err := orch.Deploy(ctx, req)
	if err != nil {
		t.Errorf("Deploy() error = %v", err)
	}
	if dep == nil {
		t.Fatal("Deploy() returned nil deployment")
	}
	if len(dep.ID) != 20 {
		t.Errorf("Deploy() ID = %q, want 20-char XID", dep.ID)
	}
}

func TestOrchestrator_CleanupWithCompletedAt(t *testing.T) {
	t.Parallel()

	orch := NewOrchestrator(OrchestratorConfig{})
	ctx := context.Background()

	// Create deployment
	req := &DeployRequest{
		ProjectID:  "project",
		AgentID:    "agent",
		Repository: "https://github.com/test/repo.git",
	}

	dep, _ := orch.Deploy(ctx, req)

	// Set it as completed (simulating what happens after deploy finishes)
	completedTime := time.Now().Add(-2 * time.Hour)
	orch.deploymentMutex.RLock()
	d := orch.deployments[dep.ID]
	orch.deploymentMutex.RUnlock()

	d.mutex.Lock()
	d.CompletedAt = &completedTime
	d.Status = StatusCompleted
	d.mutex.Unlock()

	// Cleanup with 1 hour max age should remove it
	removed := orch.CleanupOldDeployments(1 * time.Hour)
	if removed != 1 {
		t.Errorf("CleanupOldDeployments() removed = %d, want 1", removed)
	}

	// Should be gone
	_, ok := orch.GetDeployment(dep.ID)
	if ok {
		t.Error("Deployment should be removed after cleanup")
	}
}
