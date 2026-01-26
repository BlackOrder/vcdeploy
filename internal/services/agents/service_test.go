package agents

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

func newTestService(t *testing.T) (*Service, *storage.DB) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	logger := zap.NewNop()
	db, err := storage.New(dbPath, logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return New(db), db
}

func createTestAgent(t *testing.T, svc *Service, id string) *storage.Agent {
	t.Helper()
	ctx := context.Background()
	agent := &storage.Agent{
		ID:           id,
		Hostname:     "host-" + id,
		Labels:       map[string]string{"env": "test"},
		Status:       "connected",
		RegisteredAt: time.Now(),
		LastSeenAt:   time.Now(),
	}
	if err := svc.Upsert(ctx, agent); err != nil {
		t.Fatalf("createTestAgent() error = %v", err)
	}
	return agent
}

func TestService_Upsert(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	agent := &storage.Agent{
		ID:           "test-agent-1",
		Hostname:     "test-host",
		Labels:       map[string]string{"env": "production"},
		Status:       "connected",
		RegisteredAt: time.Now(),
		LastSeenAt:   time.Now(),
	}

	err := svc.Upsert(ctx, agent)
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	// Verify it was created
	found, err := svc.GetByID(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if found == nil {
		t.Fatal("GetByID() returned nil after Upsert")
	}
	if found.ID != agent.ID {
		t.Errorf("GetByID() id = %v, want %v", found.ID, agent.ID)
	}
	if found.Hostname != agent.Hostname {
		t.Errorf("GetByID() hostname = %v, want %v", found.Hostname, agent.Hostname)
	}
}

func TestService_Upsert_Update(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	agent := createTestAgent(t, svc, "update-agent")

	// Update the agent
	agent.Status = "disconnected"
	agent.Hostname = "new-hostname"
	err := svc.Upsert(ctx, agent)
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	// Verify update
	updated, err := svc.GetByID(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if updated.Status != "disconnected" {
		t.Errorf("Upsert() status = %v, want %v", updated.Status, "disconnected")
	}
	if updated.Hostname != "new-hostname" {
		t.Errorf("Upsert() hostname = %v, want %v", updated.Hostname, "new-hostname")
	}
}

func TestService_GetByID(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create an agent
	agent := createTestAgent(t, svc, "find-me")

	// Get by ID
	found, err := svc.GetByID(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if found == nil {
		t.Fatal("GetByID() returned nil")
	}
	if found.ID != agent.ID {
		t.Errorf("GetByID() id = %v, want %v", found.ID, agent.ID)
	}
}

func TestService_GetByID_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.GetByID(ctx, "nonexistent-agent")
	if err == nil {
		t.Error("GetByID() expected error for nonexistent agent")
	}
}

func TestService_List(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create some agents
	for i := 0; i < 3; i++ {
		createTestAgent(t, svc, fmt.Sprintf("agent-%d", i))
	}

	agents, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(agents) != 3 {
		t.Errorf("List() returned %v agents, want %v", len(agents), 3)
	}
}

func TestService_Count(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create some agents
	for i := 0; i < 5; i++ {
		createTestAgent(t, svc, fmt.Sprintf("count-agent-%d", i))
	}

	count, err := svc.Count(ctx)
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 5 {
		t.Errorf("Count() = %v, want %v", count, 5)
	}
}

func TestService_Delete(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create an agent
	agent := createTestAgent(t, svc, "to-delete")

	// Delete the agent
	err := svc.Delete(ctx, agent.ID)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deleted
	_, err = svc.GetByID(ctx, agent.ID)
	if err == nil {
		t.Error("Delete() agent still exists")
	}
}

func TestService_CountByStatus(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create agents with different statuses
	statuses := []string{"connected", "connected", "disconnected", "stale"}
	for i, status := range statuses {
		agent := &storage.Agent{
			ID:           fmt.Sprintf("status-agent-%d", i),
			Hostname:     fmt.Sprintf("host-%d", i),
			Status:       status,
			RegisteredAt: time.Now(),
			LastSeenAt:   time.Now(),
		}
		if err := svc.Upsert(ctx, agent); err != nil {
			t.Fatalf("Upsert() error = %v", err)
		}
	}

	counts, err := svc.CountByStatus(ctx)
	if err != nil {
		t.Fatalf("CountByStatus() error = %v", err)
	}

	if counts["connected"] != 2 {
		t.Errorf("CountByStatus() connected = %v, want %v", counts["connected"], 2)
	}
	if counts["disconnected"] != 1 {
		t.Errorf("CountByStatus() disconnected = %v, want %v", counts["disconnected"], 1)
	}
	if counts["stale"] != 1 {
		t.Errorf("CountByStatus() stale = %v, want %v", counts["stale"], 1)
	}
}

func TestService_MarkStale(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create an agent with old last_seen
	oldTime := time.Now().Add(-2 * time.Hour)
	agent := &storage.Agent{
		ID:           "stale-agent",
		Hostname:     "stale-host",
		Status:       "connected",
		RegisteredAt: time.Now(),
		LastSeenAt:   oldTime,
	}
	if err := svc.Upsert(ctx, agent); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	// Mark agents stale if not seen in last hour
	cutoff := time.Now().Add(-1 * time.Hour)
	count, err := svc.MarkStale(ctx, cutoff)
	if err != nil {
		t.Fatalf("MarkStale() error = %v", err)
	}
	if count != 1 {
		t.Errorf("MarkStale() count = %v, want %v", count, 1)
	}

	// Verify agent is now stale/disconnected
	updated, err := svc.GetByID(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if updated.Status != "disconnected" {
		t.Errorf("MarkStale() status = %v, want %v", updated.Status, "disconnected")
	}
}

func TestService_ContextCancellation(t *testing.T) {
	svc, _ := newTestService(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := svc.List(ctx)
	if err == nil {
		t.Error("List() expected error for cancelled context")
	}
}

func TestService_UpdateStatus(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create an agent first
	agent := createTestAgent(t, svc, "status-update-agent")

	// Update status
	err := svc.UpdateStatus(ctx, agent.ID, "disconnected")
	if err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}

	// Verify the status was updated
	updated, err := svc.GetByID(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if updated.Status != "disconnected" {
		t.Errorf("UpdateStatus() status = %v, want %v", updated.Status, "disconnected")
	}
	// LastSeenAt should be updated
	if updated.LastSeenAt.Before(agent.LastSeenAt) {
		t.Error("UpdateStatus() should have updated LastSeenAt")
	}
}

func TestService_UpdateStatus_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	err := svc.UpdateStatus(ctx, "nonexistent-agent", "connected")
	if err == nil {
		t.Error("UpdateStatus() expected error for nonexistent agent")
	}
}

func TestService_List_Empty(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	agents, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("List() returned %v agents, want 0", len(agents))
	}
}

func TestService_Count_Empty(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	count, err := svc.Count(ctx)
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 0 {
		t.Errorf("Count() = %v, want 0", count)
	}
}

func TestService_CountByStatus_Empty(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	counts, err := svc.CountByStatus(ctx)
	if err != nil {
		t.Fatalf("CountByStatus() error = %v", err)
	}
	if len(counts) != 0 {
		t.Errorf("CountByStatus() returned %v entries, want 0", len(counts))
	}
}

func TestService_Delete_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Deleting a nonexistent agent returns an error
	err := svc.Delete(ctx, "nonexistent-agent")
	if err == nil {
		t.Error("Delete() expected error for nonexistent agent")
	}
}

func TestService_Upsert_WithAllFields(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	agent := &storage.Agent{
		ID:           "full-agent",
		Hostname:     "full-host.example.com",
		Labels:       map[string]string{"env": "production", "region": "us-east-1"},
		Capabilities: `{"docker": true, "k8s": false}`,
		Status:       "connected",
		RegisteredAt: time.Now().Add(-24 * time.Hour),
		LastSeenAt:   time.Now(),
		Certificate:  "-----BEGIN CERTIFICATE-----\nMIIBkTCB+wIJAKHBfpw...\n-----END CERTIFICATE-----",
	}

	err := svc.Upsert(ctx, agent)
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	// Verify fields (note: capabilities is not returned by storage layer)
	found, err := svc.GetByID(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if found.Hostname != agent.Hostname {
		t.Errorf("GetByID() hostname = %v, want %v", found.Hostname, agent.Hostname)
	}
	if found.Certificate != agent.Certificate {
		t.Errorf("GetByID() certificate = %v, want %v", found.Certificate, agent.Certificate)
	}
	if len(found.Labels) != 2 {
		t.Errorf("GetByID() labels count = %v, want 2", len(found.Labels))
	}
}

func TestService_Upsert_EmptyLabels(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	agent := &storage.Agent{
		ID:           "empty-labels-agent",
		Hostname:     "no-labels.example.com",
		Labels:       nil,
		Status:       "connected",
		RegisteredAt: time.Now(),
		LastSeenAt:   time.Now(),
	}

	err := svc.Upsert(ctx, agent)
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	found, err := svc.GetByID(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if found.ID != agent.ID {
		t.Errorf("GetByID() id = %v, want %v", found.ID, agent.ID)
	}
}

func TestService_MarkStale_NoStaleAgents(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create an agent with recent last_seen
	recentTime := time.Now()
	agent := &storage.Agent{
		ID:           "recent-agent",
		Hostname:     "recent-host",
		Status:       "connected",
		RegisteredAt: time.Now(),
		LastSeenAt:   recentTime,
	}
	if err := svc.Upsert(ctx, agent); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	// Mark agents stale if not seen in last hour (none should be stale)
	cutoff := time.Now().Add(-1 * time.Hour)
	count, err := svc.MarkStale(ctx, cutoff)
	if err != nil {
		t.Fatalf("MarkStale() error = %v", err)
	}
	if count != 0 {
		t.Errorf("MarkStale() count = %v, want 0", count)
	}

	// Verify agent is still connected
	updated, err := svc.GetByID(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if updated.Status != "connected" {
		t.Errorf("MarkStale() status = %v, want %v", updated.Status, "connected")
	}
}

func TestService_MarkStale_MultipleAgents(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	now := time.Now()
	oldTime := now.Add(-2 * time.Hour)

	// Create stale agents
	for i := 0; i < 3; i++ {
		agent := &storage.Agent{
			ID:           fmt.Sprintf("stale-multi-%d", i),
			Hostname:     fmt.Sprintf("stale-host-%d", i),
			Status:       "connected",
			RegisteredAt: now,
			LastSeenAt:   oldTime,
		}
		if err := svc.Upsert(ctx, agent); err != nil {
			t.Fatalf("Upsert() error = %v", err)
		}
	}

	// Create fresh agents
	for i := 0; i < 2; i++ {
		agent := &storage.Agent{
			ID:           fmt.Sprintf("fresh-multi-%d", i),
			Hostname:     fmt.Sprintf("fresh-host-%d", i),
			Status:       "connected",
			RegisteredAt: now,
			LastSeenAt:   now,
		}
		if err := svc.Upsert(ctx, agent); err != nil {
			t.Fatalf("Upsert() error = %v", err)
		}
	}

	// Mark agents stale if not seen in last hour
	cutoff := now.Add(-1 * time.Hour)
	count, err := svc.MarkStale(ctx, cutoff)
	if err != nil {
		t.Fatalf("MarkStale() error = %v", err)
	}
	if count != 3 {
		t.Errorf("MarkStale() count = %v, want 3", count)
	}

	// Verify counts
	counts, err := svc.CountByStatus(ctx)
	if err != nil {
		t.Fatalf("CountByStatus() error = %v", err)
	}
	if counts["disconnected"] != 3 {
		t.Errorf("CountByStatus() disconnected = %v, want 3", counts["disconnected"])
	}
	if counts["connected"] != 2 {
		t.Errorf("CountByStatus() connected = %v, want 2", counts["connected"])
	}
}

func TestService_MarkStale_AlreadyDisconnected(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create an agent that's already disconnected
	oldTime := time.Now().Add(-2 * time.Hour)
	agent := &storage.Agent{
		ID:           "already-disconnected",
		Hostname:     "disconnected-host",
		Status:       "disconnected",
		RegisteredAt: time.Now(),
		LastSeenAt:   oldTime,
	}
	if err := svc.Upsert(ctx, agent); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	// MarkStale should still count it (or skip, depends on implementation)
	cutoff := time.Now().Add(-1 * time.Hour)
	_, err := svc.MarkStale(ctx, cutoff)
	if err != nil {
		t.Fatalf("MarkStale() error = %v", err)
	}
}

func TestService_Upsert_UpdateLabels(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create agent with initial labels
	agent := &storage.Agent{
		ID:           "label-update-agent",
		Hostname:     "label-host",
		Labels:       map[string]string{"env": "dev"},
		Status:       "connected",
		RegisteredAt: time.Now(),
		LastSeenAt:   time.Now(),
	}
	if err := svc.Upsert(ctx, agent); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	// Update labels
	agent.Labels = map[string]string{"env": "production", "region": "eu-west-1"}
	if err := svc.Upsert(ctx, agent); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	// Verify labels were updated
	found, err := svc.GetByID(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if found.Labels["env"] != "production" {
		t.Errorf("GetByID() labels[env] = %v, want %v", found.Labels["env"], "production")
	}
	if found.Labels["region"] != "eu-west-1" {
		t.Errorf("GetByID() labels[region] = %v, want %v", found.Labels["region"], "eu-west-1")
	}
}

func TestService_ContextCancellation_GetByID(t *testing.T) {
	svc, _ := newTestService(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := svc.GetByID(ctx, "any-id")
	if err == nil {
		t.Error("GetByID() expected error for cancelled context")
	}
}

func TestService_ContextCancellation_Count(t *testing.T) {
	svc, _ := newTestService(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := svc.Count(ctx)
	if err == nil {
		t.Error("Count() expected error for cancelled context")
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

func TestService_ContextCancellation_Upsert(t *testing.T) {
	svc, _ := newTestService(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	agent := &storage.Agent{
		ID:       "cancelled-agent",
		Hostname: "cancelled-host",
		Status:   "connected",
	}
	err := svc.Upsert(ctx, agent)
	if err == nil {
		t.Error("Upsert() expected error for cancelled context")
	}
}

func TestService_ContextCancellation_Delete(t *testing.T) {
	svc, _ := newTestService(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := svc.Delete(ctx, "any-id")
	if err == nil {
		t.Error("Delete() expected error for cancelled context")
	}
}

func TestService_ContextCancellation_MarkStale(t *testing.T) {
	svc, _ := newTestService(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := svc.MarkStale(ctx, time.Now())
	if err == nil {
		t.Error("MarkStale() expected error for cancelled context")
	}
}

func TestService_ContextCancellation_UpdateStatus(t *testing.T) {
	svc, _ := newTestService(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := svc.UpdateStatus(ctx, "any-id", "connected")
	if err == nil {
		t.Error("UpdateStatus() expected error for cancelled context")
	}
}

func TestNew(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	logger := zap.NewNop()
	db, err := storage.New(dbPath, logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	svc := New(db)
	if svc == nil {
		t.Fatal("New() returned nil")
	}
	if svc.db != db {
		t.Error("New() did not set db correctly")
	}
}

func TestService_UpdateStatus_MultipleUpdates(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	agent := createTestAgent(t, svc, "multi-status-agent")

	statuses := []string{"disconnected", "connected", "stale", "connected"}
	for _, status := range statuses {
		err := svc.UpdateStatus(ctx, agent.ID, status)
		if err != nil {
			t.Fatalf("UpdateStatus() error = %v", err)
		}

		updated, err := svc.GetByID(ctx, agent.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if updated.Status != status {
			t.Errorf("UpdateStatus() status = %v, want %v", updated.Status, status)
		}
	}
}

func TestService_ListVerifyAgentOrder(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create agents in specific order
	ids := []string{"agent-c", "agent-a", "agent-b"}
	for _, id := range ids {
		createTestAgent(t, svc, id)
	}

	agents, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(agents) != 3 {
		t.Errorf("List() returned %v agents, want 3", len(agents))
	}

	// Verify all agents are present (order may vary by implementation)
	foundIDs := make(map[string]bool)
	for _, a := range agents {
		foundIDs[a.ID] = true
	}
	for _, id := range ids {
		if !foundIDs[id] {
			t.Errorf("List() missing agent %v", id)
		}
	}
}

func TestService_CountByStatus_AllSameStatus(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create all connected agents
	for i := 0; i < 5; i++ {
		agent := &storage.Agent{
			ID:           fmt.Sprintf("same-status-agent-%d", i),
			Hostname:     fmt.Sprintf("host-%d", i),
			Status:       "connected",
			RegisteredAt: time.Now(),
			LastSeenAt:   time.Now(),
		}
		if err := svc.Upsert(ctx, agent); err != nil {
			t.Fatalf("Upsert() error = %v", err)
		}
	}

	counts, err := svc.CountByStatus(ctx)
	if err != nil {
		t.Fatalf("CountByStatus() error = %v", err)
	}

	if counts["connected"] != 5 {
		t.Errorf("CountByStatus() connected = %v, want 5", counts["connected"])
	}
	// Other statuses should not exist or be 0
	if counts["disconnected"] != 0 {
		t.Errorf("CountByStatus() disconnected = %v, want 0", counts["disconnected"])
	}
}

func TestService_DeleteThenRecreate(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	agentID := "recreate-agent"

	// Create
	agent := createTestAgent(t, svc, agentID)

	// Delete
	err := svc.Delete(ctx, agentID)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deleted
	_, err = svc.GetByID(ctx, agentID)
	if err == nil {
		t.Error("GetByID() expected error after delete")
	}

	// Recreate with same ID
	agent.Hostname = "new-hostname"
	err = svc.Upsert(ctx, agent)
	if err != nil {
		t.Fatalf("Upsert() after delete error = %v", err)
	}

	// Verify recreated
	found, err := svc.GetByID(ctx, agentID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if found.Hostname != "new-hostname" {
		t.Errorf("Upsert() after delete hostname = %v, want %v", found.Hostname, "new-hostname")
	}
}
