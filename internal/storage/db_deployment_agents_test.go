package storage

import (
	"context"
	"path/filepath"
	"testing"
)

func TestAssignAgentToDeployment(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath, nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Create a deployment first
	dep := &DeploymentRecord{
		ID:            "dep-001",
		Project:       "test-project",
		Target:        "test-agent",
		Branch:        "main",
		Status:        DeploymentStatusPending,
		TriggeredBy:   "test",
		TriggerSource: "manual",
	}
	if err := db.CreateDeployment(ctx, dep); err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	// Create an agent
	agent := &Agent{
		ID:       "agent-001",
		Hostname: "test-host",
		Status:   AgentStatusOnline,
	}
	if err := db.UpsertAgent(ctx, agent); err != nil {
		t.Fatalf("UpsertAgent failed: %v", err)
	}

	// Assign agent to deployment
	err = db.AssignAgentToDeployment(ctx, "dep-001", "agent-001")
	if err != nil {
		t.Errorf("AssignAgentToDeployment failed: %v", err)
	}

	// Verify assignment
	assigned, err := db.IsAgentAssignedToDeployment(ctx, "dep-001", "agent-001")
	if err != nil {
		t.Errorf("IsAgentAssignedToDeployment failed: %v", err)
	}
	if !assigned {
		t.Error("Expected agent to be assigned to deployment")
	}
}

func TestAssignAgentToDeployment_Duplicate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath, nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Setup
	dep := &DeploymentRecord{
		ID:            "dep-002",
		Project:       "test-project",
		Target:        "test-agent",
		Branch:        "main",
		Status:        DeploymentStatusPending,
		TriggeredBy:   "test",
		TriggerSource: "manual",
	}
	if err := db.CreateDeployment(ctx, dep); err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	agent := &Agent{
		ID:       "agent-002",
		Hostname: "test-host",
		Status:   AgentStatusOnline,
	}
	if err := db.UpsertAgent(ctx, agent); err != nil {
		t.Fatalf("UpsertAgent failed: %v", err)
	}

	// First assignment should succeed
	err = db.AssignAgentToDeployment(ctx, "dep-002", "agent-002")
	if err != nil {
		t.Fatalf("First AssignAgentToDeployment failed: %v", err)
	}

	// Second assignment should fail (duplicate)
	err = db.AssignAgentToDeployment(ctx, "dep-002", "agent-002")
	if err == nil {
		t.Error("Expected duplicate assignment to fail")
	}
}

func TestGetDeploymentAgents_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath, nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	agents, err := db.GetDeploymentAgents(ctx, "nonexistent")
	if err != nil {
		t.Errorf("GetDeploymentAgents failed: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("Expected empty list, got %d agents", len(agents))
	}
}

func TestGetDeploymentAgents_Multiple(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath, nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Setup deployment
	dep := &DeploymentRecord{
		ID:            "dep-003",
		Project:       "test-project",
		Target:        "multi-agent",
		Branch:        "main",
		Status:        DeploymentStatusPending,
		TriggeredBy:   "test",
		TriggerSource: "manual",
	}
	if err := db.CreateDeployment(ctx, dep); err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	// Create multiple agents
	for i, agentID := range []string{"agent-a", "agent-b", "agent-c"} {
		agent := &Agent{
			ID:       agentID,
			Hostname: "host-" + string(rune('a'+i)),
			Status:   AgentStatusOnline,
		}
		if err := db.UpsertAgent(ctx, agent); err != nil {
			t.Fatalf("UpsertAgent failed: %v", err)
		}
	}

	// Assign all agents
	err = db.AssignAgentsToDeployment(ctx, "dep-003", []string{"agent-a", "agent-b", "agent-c"})
	if err != nil {
		t.Fatalf("AssignAgentsToDeployment failed: %v", err)
	}

	// Get all agents
	agents, err := db.GetDeploymentAgents(ctx, "dep-003")
	if err != nil {
		t.Errorf("GetDeploymentAgents failed: %v", err)
	}
	if len(agents) != 3 {
		t.Errorf("Expected 3 agents, got %d", len(agents))
	}

	// All should be pending
	for _, agent := range agents {
		if agent.Status != DeploymentStatusPending {
			t.Errorf("Expected status pending, got %s for agent %s", agent.Status, agent.AgentID)
		}
	}
}

func TestUpdateDeploymentAgentStatus(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath, nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Setup
	dep := &DeploymentRecord{
		ID:            "dep-004",
		Project:       "test-project",
		Target:        "test-agent",
		Branch:        "main",
		Status:        DeploymentStatusPending,
		TriggeredBy:   "test",
		TriggerSource: "manual",
	}
	if err := db.CreateDeployment(ctx, dep); err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	agent := &Agent{
		ID:       "agent-004",
		Hostname: "test-host",
		Status:   AgentStatusOnline,
	}
	if err := db.UpsertAgent(ctx, agent); err != nil {
		t.Fatalf("UpsertAgent failed: %v", err)
	}

	if err := db.AssignAgentToDeployment(ctx, "dep-004", "agent-004"); err != nil {
		t.Fatalf("AssignAgentToDeployment failed: %v", err)
	}

	// Update status to running
	err = db.UpdateDeploymentAgentStatus(ctx, "dep-004", "agent-004", DeploymentStatusRunning, "")
	if err != nil {
		t.Errorf("UpdateDeploymentAgentStatus (running) failed: %v", err)
	}

	// Verify
	daStatus, err := db.GetDeploymentAgentStatus(ctx, "dep-004", "agent-004")
	if err != nil {
		t.Errorf("GetDeploymentAgentStatus failed: %v", err)
	}
	if daStatus.Status != DeploymentStatusRunning {
		t.Errorf("Expected running status, got %s", daStatus.Status)
	}

	// Update to success (terminal)
	err = db.UpdateDeploymentAgentStatus(ctx, "dep-004", "agent-004", DeploymentStatusSuccess, "")
	if err != nil {
		t.Errorf("UpdateDeploymentAgentStatus (success) failed: %v", err)
	}

	// Verify completed_at is set
	daStatus, err = db.GetDeploymentAgentStatus(ctx, "dep-004", "agent-004")
	if err != nil {
		t.Errorf("GetDeploymentAgentStatus failed: %v", err)
	}
	if daStatus.CompletedAt == nil {
		t.Error("Expected completed_at to be set for terminal status")
	}
}

func TestIsAgentAssignedToDeployment(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath, nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Not assigned
	assigned, err := db.IsAgentAssignedToDeployment(ctx, "nonexistent", "nonexistent")
	if err != nil {
		t.Errorf("IsAgentAssignedToDeployment failed: %v", err)
	}
	if assigned {
		t.Error("Expected not assigned for nonexistent")
	}

	// Setup and assign
	dep := &DeploymentRecord{
		ID:            "dep-005",
		Project:       "test-project",
		Target:        "test-agent",
		Branch:        "main",
		Status:        DeploymentStatusPending,
		TriggeredBy:   "test",
		TriggerSource: "manual",
	}
	if err := db.CreateDeployment(ctx, dep); err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	agent := &Agent{
		ID:       "agent-005",
		Hostname: "test-host",
		Status:   AgentStatusOnline,
	}
	if err := db.UpsertAgent(ctx, agent); err != nil {
		t.Fatalf("UpsertAgent failed: %v", err)
	}

	if err := db.AssignAgentToDeployment(ctx, "dep-005", "agent-005"); err != nil {
		t.Fatalf("AssignAgentToDeployment failed: %v", err)
	}

	// Now should be assigned
	assigned, err = db.IsAgentAssignedToDeployment(ctx, "dep-005", "agent-005")
	if err != nil {
		t.Errorf("IsAgentAssignedToDeployment failed: %v", err)
	}
	if !assigned {
		t.Error("Expected agent to be assigned")
	}
}

func TestStartDeploymentAgent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath, nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Setup
	dep := &DeploymentRecord{
		ID:            "dep-006",
		Project:       "test-project",
		Target:        "test-agent",
		Branch:        "main",
		Status:        DeploymentStatusPending,
		TriggeredBy:   "test",
		TriggerSource: "manual",
	}
	if err := db.CreateDeployment(ctx, dep); err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	agent := &Agent{
		ID:       "agent-006",
		Hostname: "test-host",
		Status:   AgentStatusOnline,
	}
	if err := db.UpsertAgent(ctx, agent); err != nil {
		t.Fatalf("UpsertAgent failed: %v", err)
	}

	if err := db.AssignAgentToDeployment(ctx, "dep-006", "agent-006"); err != nil {
		t.Fatalf("AssignAgentToDeployment failed: %v", err)
	}

	// Start the agent
	err = db.StartDeploymentAgent(ctx, "dep-006", "agent-006")
	if err != nil {
		t.Errorf("StartDeploymentAgent failed: %v", err)
	}

	// Verify status and started_at
	daStatus, err := db.GetDeploymentAgentStatus(ctx, "dep-006", "agent-006")
	if err != nil {
		t.Errorf("GetDeploymentAgentStatus failed: %v", err)
	}
	if daStatus.Status != DeploymentStatusRunning {
		t.Errorf("Expected running status, got %s", daStatus.Status)
	}
	if daStatus.StartedAt == nil {
		t.Error("Expected started_at to be set")
	}

	// Starting again should fail (not pending)
	err = db.StartDeploymentAgent(ctx, "dep-006", "agent-006")
	if err == nil {
		t.Error("Expected starting non-pending agent to fail")
	}
}

func TestCountDeploymentAgentsByStatus(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath, nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Setup deployment
	dep := &DeploymentRecord{
		ID:            "dep-007",
		Project:       "test-project",
		Target:        "multi-agent",
		Branch:        "main",
		Status:        DeploymentStatusRunning,
		TriggeredBy:   "test",
		TriggerSource: "manual",
	}
	if err := db.CreateDeployment(ctx, dep); err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	// Create and assign agents
	for i, agentID := range []string{"agent-7a", "agent-7b", "agent-7c", "agent-7d"} {
		agent := &Agent{
			ID:       agentID,
			Hostname: "host-" + string(rune('a'+i)),
			Status:   AgentStatusOnline,
		}
		if err := db.UpsertAgent(ctx, agent); err != nil {
			t.Fatalf("UpsertAgent failed: %v", err)
		}
		if err := db.AssignAgentToDeployment(ctx, "dep-007", agentID); err != nil {
			t.Fatalf("AssignAgentToDeployment failed: %v", err)
		}
	}

	// Set different statuses
	db.UpdateDeploymentAgentStatus(ctx, "dep-007", "agent-7a", DeploymentStatusRunning, "")
	db.UpdateDeploymentAgentStatus(ctx, "dep-007", "agent-7b", DeploymentStatusSuccess, "")
	db.UpdateDeploymentAgentStatus(ctx, "dep-007", "agent-7c", DeploymentStatusFailed, "test error")
	// agent-7d stays pending

	// Count by status
	counts, err := db.CountDeploymentAgentsByStatus(ctx, "dep-007")
	if err != nil {
		t.Errorf("CountDeploymentAgentsByStatus failed: %v", err)
	}

	if counts[DeploymentStatusPending] != 1 {
		t.Errorf("Expected 1 pending, got %d", counts[DeploymentStatusPending])
	}
	if counts[DeploymentStatusRunning] != 1 {
		t.Errorf("Expected 1 running, got %d", counts[DeploymentStatusRunning])
	}
	if counts[DeploymentStatusSuccess] != 1 {
		t.Errorf("Expected 1 success, got %d", counts[DeploymentStatusSuccess])
	}
	if counts[DeploymentStatusFailed] != 1 {
		t.Errorf("Expected 1 failed, got %d", counts[DeploymentStatusFailed])
	}
}

func TestRemoveAgentFromDeployment(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath, nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Setup
	dep := &DeploymentRecord{
		ID:            "dep-008",
		Project:       "test-project",
		Target:        "test-agent",
		Branch:        "main",
		Status:        DeploymentStatusPending,
		TriggeredBy:   "test",
		TriggerSource: "manual",
	}
	if err := db.CreateDeployment(ctx, dep); err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	agent := &Agent{
		ID:       "agent-008",
		Hostname: "test-host",
		Status:   AgentStatusOnline,
	}
	if err := db.UpsertAgent(ctx, agent); err != nil {
		t.Fatalf("UpsertAgent failed: %v", err)
	}

	if err := db.AssignAgentToDeployment(ctx, "dep-008", "agent-008"); err != nil {
		t.Fatalf("AssignAgentToDeployment failed: %v", err)
	}

	// Remove the pending agent
	err = db.RemoveAgentFromDeployment(ctx, "dep-008", "agent-008")
	if err != nil {
		t.Errorf("RemoveAgentFromDeployment failed: %v", err)
	}

	// Should not be assigned anymore
	assigned, err := db.IsAgentAssignedToDeployment(ctx, "dep-008", "agent-008")
	if err != nil {
		t.Errorf("IsAgentAssignedToDeployment failed: %v", err)
	}
	if assigned {
		t.Error("Expected agent to not be assigned after removal")
	}
}

func TestRemoveAgentFromDeployment_NotPending(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath, nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Setup
	dep := &DeploymentRecord{
		ID:            "dep-009",
		Project:       "test-project",
		Target:        "test-agent",
		Branch:        "main",
		Status:        DeploymentStatusRunning,
		TriggeredBy:   "test",
		TriggerSource: "manual",
	}
	if err := db.CreateDeployment(ctx, dep); err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	agent := &Agent{
		ID:       "agent-009",
		Hostname: "test-host",
		Status:   AgentStatusOnline,
	}
	if err := db.UpsertAgent(ctx, agent); err != nil {
		t.Fatalf("UpsertAgent failed: %v", err)
	}

	if err := db.AssignAgentToDeployment(ctx, "dep-009", "agent-009"); err != nil {
		t.Fatalf("AssignAgentToDeployment failed: %v", err)
	}

	// Start the agent (no longer pending)
	db.StartDeploymentAgent(ctx, "dep-009", "agent-009")

	// Try to remove - should fail since not pending
	err = db.RemoveAgentFromDeployment(ctx, "dep-009", "agent-009")
	if err == nil {
		t.Error("Expected removing non-pending agent to fail")
	}
}

func TestGetAgentDeployments(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath, nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Create an agent
	agent := &Agent{
		ID:       "agent-010",
		Hostname: "test-host",
		Status:   AgentStatusOnline,
	}
	if err := db.UpsertAgent(ctx, agent); err != nil {
		t.Fatalf("UpsertAgent failed: %v", err)
	}

	// Create multiple deployments and assign agent
	for i, depID := range []string{"dep-010a", "dep-010b", "dep-010c"} {
		dep := &DeploymentRecord{
			ID:            depID,
			Project:       "project-" + string(rune('a'+i)),
			Target:        "test-agent",
			Branch:        "main",
			Status:        DeploymentStatusPending,
			TriggeredBy:   "test",
			TriggerSource: "manual",
		}
		if err := db.CreateDeployment(ctx, dep); err != nil {
			t.Fatalf("CreateDeployment failed: %v", err)
		}
		if err := db.AssignAgentToDeployment(ctx, depID, "agent-010"); err != nil {
			t.Fatalf("AssignAgentToDeployment failed: %v", err)
		}
	}

	// Get agent's deployments
	deployments, err := db.GetAgentDeployments(ctx, "agent-010")
	if err != nil {
		t.Errorf("GetAgentDeployments failed: %v", err)
	}
	if len(deployments) != 3 {
		t.Errorf("Expected 3 deployments, got %d", len(deployments))
	}
}
