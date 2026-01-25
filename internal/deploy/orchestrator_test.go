package deploy

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
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

func TestGenerateDeploymentID(t *testing.T) {
	ids := make(map[string]bool)

	for i := 0; i < 100; i++ {
		id, err := generateDeploymentID()
		if err != nil {
			t.Fatalf("generateDeploymentID failed: %v", err)
		}

		if !hasPrefix(id, "deploy-") {
			t.Errorf("Expected prefix 'deploy-', got %q", id)
		}

		if ids[id] {
			t.Error("Generated duplicate ID")
		}
		ids[id] = true
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
