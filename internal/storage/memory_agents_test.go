package storage

import (
	"context"
	"testing"
	"time"
)

// --- Agent tests ---

func TestMemoryStore_UpsertAgent_Create(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	agent := &Agent{
		ID:       "agent-1",
		Hostname: "host1.example.com",
		Labels:   map[string]string{"env": "prod"},
		Status:   "online",
	}
	err := s.UpsertAgent(ctx, agent)
	if err != nil {
		t.Fatalf("UpsertAgent() error = %v", err)
	}

	if agent.RegisteredAt.IsZero() {
		t.Error("UpsertAgent() did not set RegisteredAt")
	}
	if agent.LastSeenAt.IsZero() {
		t.Error("UpsertAgent() did not set LastSeenAt")
	}
}

func TestMemoryStore_UpsertAgent_Update(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	// Create
	s.UpsertAgent(ctx, &Agent{ID: "agent-1", Hostname: "old.example.com"})

	// Update
	err := s.UpsertAgent(ctx, &Agent{ID: "agent-1", Hostname: "new.example.com"})
	if err != nil {
		t.Fatalf("UpsertAgent() error = %v", err)
	}

	found, _ := s.GetAgent(ctx, "agent-1")
	if found.Hostname != "new.example.com" {
		t.Errorf("Hostname = %s, want new.example.com", found.Hostname)
	}
}

func TestMemoryStore_GetAgent(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.UpsertAgent(ctx, &Agent{ID: "agent-1", Hostname: "host1"})

	found, err := s.GetAgent(ctx, "agent-1")
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if found.Hostname != "host1" {
		t.Errorf("Hostname = %s, want host1", found.Hostname)
	}
}

func TestMemoryStore_GetAgent_NotFound(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	_, err := s.GetAgent(context.Background(), "nonexistent")
	if err != ErrNotFound {
		t.Errorf("GetAgent() error = %v, want ErrNotFound", err)
	}
}

func TestMemoryStore_ListAgents(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.UpsertAgent(ctx, &Agent{ID: "agent-1"})
	s.UpsertAgent(ctx, &Agent{ID: "agent-2"})
	s.UpsertAgent(ctx, &Agent{ID: "agent-3"})

	agents, err := s.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents() error = %v", err)
	}
	if len(agents) != 3 {
		t.Errorf("len(agents) = %d, want 3", len(agents))
	}
}

func TestMemoryStore_CountAgents(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.UpsertAgent(ctx, &Agent{ID: "agent-1"})
	s.UpsertAgent(ctx, &Agent{ID: "agent-2"})

	count, err := s.CountAgents(ctx)
	if err != nil {
		t.Fatalf("CountAgents() error = %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}

func TestMemoryStore_CountAgentsByStatus(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.UpsertAgent(ctx, &Agent{ID: "agent-1", Status: "online"})
	s.UpsertAgent(ctx, &Agent{ID: "agent-2", Status: "online"})
	s.UpsertAgent(ctx, &Agent{ID: "agent-3", Status: "offline"})

	counts, err := s.CountAgentsByStatus(ctx)
	if err != nil {
		t.Fatalf("CountAgentsByStatus() error = %v", err)
	}
	if counts["online"] != 2 {
		t.Errorf("online count = %d, want 2", counts["online"])
	}
	if counts["offline"] != 1 {
		t.Errorf("offline count = %d, want 1", counts["offline"])
	}
}

func TestMemoryStore_DeleteAgent(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.UpsertAgent(ctx, &Agent{ID: "delete-me"})

	err := s.DeleteAgent(ctx, "delete-me")
	if err != nil {
		t.Fatalf("DeleteAgent() error = %v", err)
	}

	_, err = s.GetAgent(ctx, "delete-me")
	if err != ErrNotFound {
		t.Error("Agent still exists after delete")
	}
}

func TestMemoryStore_DeleteAgent_CascadesHistory(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.UpsertAgent(ctx, &Agent{ID: "cascade-agent"})
	s.CreateAgentUpdateHistory(ctx, &AgentUpdateHistory{AgentID: "cascade-agent", FromVersion: "1.0", ToVersion: "2.0"})

	s.DeleteAgent(ctx, "cascade-agent")

	_, err := s.GetLatestAgentUpdateHistory(ctx, "cascade-agent")
	if err != ErrNotFound {
		t.Error("Update history still exists after agent delete")
	}
}

func TestMemoryStore_UpdateAgentVersion(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.UpsertAgent(ctx, &Agent{ID: "agent-1", Version: "1.0.0"})

	err := s.UpdateAgentVersion(ctx, "agent-1", "2.0.0")
	if err != nil {
		t.Fatalf("UpdateAgentVersion() error = %v", err)
	}

	found, _ := s.GetAgent(ctx, "agent-1")
	if found.Version != "2.0.0" {
		t.Errorf("Version = %s, want 2.0.0", found.Version)
	}
	if found.LastUpdateAt == nil {
		t.Error("LastUpdateAt not set")
	}
}

func TestMemoryStore_UpdateAgentUpdateError(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.UpsertAgent(ctx, &Agent{ID: "agent-1"})

	err := s.UpdateAgentUpdateError(ctx, "agent-1", "download failed")
	if err != nil {
		t.Fatalf("UpdateAgentUpdateError() error = %v", err)
	}

	found, _ := s.GetAgent(ctx, "agent-1")
	if found.LastUpdateError != "download failed" {
		t.Errorf("LastUpdateError = %s, want 'download failed'", found.LastUpdateError)
	}
}

func TestMemoryStore_UpdateAgentUpdatePolicy(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.UpsertAgent(ctx, &Agent{ID: "agent-1"})

	err := s.UpdateAgentUpdatePolicy(ctx, "agent-1", AgentUpdatePolicyScheduled, "02:00", "06:00")
	if err != nil {
		t.Fatalf("UpdateAgentUpdatePolicy() error = %v", err)
	}

	found, _ := s.GetAgent(ctx, "agent-1")
	if found.UpdatePolicy != AgentUpdatePolicyScheduled {
		t.Errorf("UpdatePolicy = %s, want scheduled", found.UpdatePolicy)
	}
	if found.UpdateWindowStart != "02:00" {
		t.Errorf("UpdateWindowStart = %s, want 02:00", found.UpdateWindowStart)
	}
}

func TestMemoryStore_ListAgentsNeedingUpdate(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.UpsertAgent(ctx, &Agent{ID: "immediate", UpdatePolicy: AgentUpdatePolicyImmediate})
	s.UpsertAgent(ctx, &Agent{ID: "manual", UpdatePolicy: AgentUpdatePolicyManual})
	// Scheduled agent with valid window - will depend on current time

	agents, err := s.ListAgentsNeedingUpdate(ctx)
	if err != nil {
		t.Fatalf("ListAgentsNeedingUpdate() error = %v", err)
	}

	// Should include immediate, not manual
	found := false
	for _, a := range agents {
		if a.ID == "immediate" {
			found = true
		}
		if a.ID == "manual" {
			t.Error("Manual agent should not be in needing update list")
		}
	}
	if !found {
		t.Error("Immediate agent not found in needing update list")
	}
}

// --- AgentBinary tests ---

func TestMemoryStore_CreateAgentBinary(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	binary := &AgentBinary{
		Version:        "1.0.0",
		OS:             "linux",
		Arch:           "amd64",
		Path:           "/binaries/agent-1.0.0-linux-amd64",
		ChecksumSHA256: "abc123",
		SizeBytes:      1024000,
	}
	err := s.CreateAgentBinary(ctx, binary)
	if err != nil {
		t.Fatalf("CreateAgentBinary() error = %v", err)
	}

	if binary.ID == "" {
		t.Error("CreateAgentBinary() did not assign ID")
	}
}

func TestMemoryStore_CreateAgentBinary_Duplicate(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateAgentBinary(ctx, &AgentBinary{Version: "1.0.0", OS: "linux", Arch: "amd64"})

	err := s.CreateAgentBinary(ctx, &AgentBinary{Version: "1.0.0", OS: "linux", Arch: "amd64"})
	if err != ErrDuplicate {
		t.Errorf("CreateAgentBinary() error = %v, want ErrDuplicate", err)
	}
}

func TestMemoryStore_GetAgentBinary(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	binary := &AgentBinary{Version: "1.0.0", OS: "linux", Arch: "amd64", Path: "/path"}
	s.CreateAgentBinary(ctx, binary)

	found, err := s.GetAgentBinary(ctx, binary.ID)
	if err != nil {
		t.Fatalf("GetAgentBinary() error = %v", err)
	}
	if found.Path != "/path" {
		t.Errorf("Path = %s, want /path", found.Path)
	}
}

func TestMemoryStore_GetAgentBinaryByVersion(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateAgentBinary(ctx, &AgentBinary{Version: "1.0.0", OS: "linux", Arch: "amd64"})
	s.CreateAgentBinary(ctx, &AgentBinary{Version: "1.0.0", OS: "darwin", Arch: "amd64"})

	found, err := s.GetAgentBinaryByVersion(ctx, "1.0.0", "darwin", "amd64")
	if err != nil {
		t.Fatalf("GetAgentBinaryByVersion() error = %v", err)
	}
	if found.OS != "darwin" {
		t.Errorf("OS = %s, want darwin", found.OS)
	}
}

func TestMemoryStore_GetCurrentAgentBinary(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateAgentBinary(ctx, &AgentBinary{Version: "1.0.0", OS: "linux", Arch: "amd64"})
	b2 := &AgentBinary{Version: "2.0.0", OS: "linux", Arch: "amd64"}
	s.CreateAgentBinary(ctx, b2)
	s.SetCurrentAgentBinary(ctx, b2.ID)

	found, err := s.GetCurrentAgentBinary(ctx, "linux", "amd64")
	if err != nil {
		t.Fatalf("GetCurrentAgentBinary() error = %v", err)
	}
	if found.Version != "2.0.0" {
		t.Errorf("Version = %s, want 2.0.0", found.Version)
	}
}

func TestMemoryStore_ListAgentBinaries(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateAgentBinary(ctx, &AgentBinary{Version: "1.0.0", OS: "linux", Arch: "amd64"})
	s.CreateAgentBinary(ctx, &AgentBinary{Version: "1.0.0", OS: "darwin", Arch: "amd64"})

	binaries, err := s.ListAgentBinaries(ctx)
	if err != nil {
		t.Fatalf("ListAgentBinaries() error = %v", err)
	}
	if len(binaries) != 2 {
		t.Errorf("len(binaries) = %d, want 2", len(binaries))
	}
}

func TestMemoryStore_SetCurrentAgentBinary(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	b1 := &AgentBinary{Version: "1.0.0", OS: "linux", Arch: "amd64"}
	s.CreateAgentBinary(ctx, b1)
	s.SetCurrentAgentBinary(ctx, b1.ID)

	b2 := &AgentBinary{Version: "2.0.0", OS: "linux", Arch: "amd64"}
	s.CreateAgentBinary(ctx, b2)
	s.SetCurrentAgentBinary(ctx, b2.ID)

	// b1 should no longer be current
	found1, _ := s.GetAgentBinary(ctx, b1.ID)
	if found1.IsCurrent {
		t.Error("b1 should no longer be current")
	}

	// b2 should be current
	found2, _ := s.GetAgentBinary(ctx, b2.ID)
	if !found2.IsCurrent {
		t.Error("b2 should be current")
	}
}

func TestMemoryStore_DeleteAgentBinary(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	binary := &AgentBinary{Version: "1.0.0", OS: "linux", Arch: "amd64"}
	s.CreateAgentBinary(ctx, binary)

	err := s.DeleteAgentBinary(ctx, binary.ID)
	if err != nil {
		t.Fatalf("DeleteAgentBinary() error = %v", err)
	}

	_, err = s.GetAgentBinary(ctx, binary.ID)
	if err != ErrNotFound {
		t.Error("Binary still exists after delete")
	}
}

// --- AgentUpdateHistory tests ---

func TestMemoryStore_CreateAgentUpdateHistory(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	history := &AgentUpdateHistory{
		AgentID:     "agent-1",
		FromVersion: "1.0.0",
		ToVersion:   "2.0.0",
	}
	err := s.CreateAgentUpdateHistory(ctx, history)
	if err != nil {
		t.Fatalf("CreateAgentUpdateHistory() error = %v", err)
	}

	if history.ID == "" {
		t.Error("CreateAgentUpdateHistory() did not assign ID")
	}
	if history.Status != "pending" {
		t.Errorf("Status = %s, want pending", history.Status)
	}
}

func TestMemoryStore_GetAgentUpdateHistory(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	history := &AgentUpdateHistory{AgentID: "agent-1", FromVersion: "1.0", ToVersion: "2.0"}
	s.CreateAgentUpdateHistory(ctx, history)

	found, err := s.GetAgentUpdateHistory(ctx, history.ID)
	if err != nil {
		t.Fatalf("GetAgentUpdateHistory() error = %v", err)
	}
	if found.FromVersion != "1.0" {
		t.Errorf("FromVersion = %s, want 1.0", found.FromVersion)
	}
}

func TestMemoryStore_UpdateAgentUpdateHistory(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	history := &AgentUpdateHistory{AgentID: "agent-1", FromVersion: "1.0", ToVersion: "2.0"}
	s.CreateAgentUpdateHistory(ctx, history)

	now := time.Now()
	history.Status = "completed"
	history.CompletedAt = &now

	err := s.UpdateAgentUpdateHistory(ctx, history)
	if err != nil {
		t.Fatalf("UpdateAgentUpdateHistory() error = %v", err)
	}

	found, _ := s.GetAgentUpdateHistory(ctx, history.ID)
	if found.Status != "completed" {
		t.Errorf("Status = %s, want completed", found.Status)
	}
}

func TestMemoryStore_ListAgentUpdateHistory(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateAgentUpdateHistory(ctx, &AgentUpdateHistory{AgentID: "agent-1", FromVersion: "1.0", ToVersion: "2.0"})
	time.Sleep(time.Millisecond) // Ensure different timestamps
	s.CreateAgentUpdateHistory(ctx, &AgentUpdateHistory{AgentID: "agent-1", FromVersion: "2.0", ToVersion: "3.0"})
	s.CreateAgentUpdateHistory(ctx, &AgentUpdateHistory{AgentID: "agent-2", FromVersion: "1.0", ToVersion: "2.0"})

	list, total, err := s.ListAgentUpdateHistory(ctx, "agent-1", 10, 0)
	if err != nil {
		t.Fatalf("ListAgentUpdateHistory() error = %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(list) != 2 {
		t.Errorf("len(list) = %d, want 2", len(list))
	}
	// Should be sorted by newest first
	if list[0].ToVersion != "3.0" {
		t.Errorf("First item ToVersion = %s, want 3.0 (newest first)", list[0].ToVersion)
	}
}

func TestMemoryStore_ListAgentUpdateHistory_Pagination(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		s.CreateAgentUpdateHistory(ctx, &AgentUpdateHistory{AgentID: "agent-1"})
		time.Sleep(time.Millisecond)
	}

	list, total, _ := s.ListAgentUpdateHistory(ctx, "agent-1", 2, 2)
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(list) != 2 {
		t.Errorf("len(list) = %d, want 2", len(list))
	}
}

func TestMemoryStore_GetLatestAgentUpdateHistory(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateAgentUpdateHistory(ctx, &AgentUpdateHistory{AgentID: "agent-1", ToVersion: "1.0"})
	time.Sleep(time.Millisecond)
	s.CreateAgentUpdateHistory(ctx, &AgentUpdateHistory{AgentID: "agent-1", ToVersion: "2.0"})

	latest, err := s.GetLatestAgentUpdateHistory(ctx, "agent-1")
	if err != nil {
		t.Fatalf("GetLatestAgentUpdateHistory() error = %v", err)
	}
	if latest.ToVersion != "2.0" {
		t.Errorf("ToVersion = %s, want 2.0", latest.ToVersion)
	}
}

func TestMemoryStore_GetLatestAgentUpdateHistory_NotFound(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	_, err := s.GetLatestAgentUpdateHistory(context.Background(), "nonexistent")
	if err != ErrNotFound {
		t.Errorf("GetLatestAgentUpdateHistory() error = %v, want ErrNotFound", err)
	}
}

// --- isInUpdateWindow tests ---

func TestIsInUpdateWindow(t *testing.T) {
	tests := []struct {
		name   string
		now    time.Time
		start  string
		end    string
		expect bool
	}{
		{"in window", time.Date(2024, 1, 1, 3, 0, 0, 0, time.UTC), "02:00", "06:00", true},
		{"before window", time.Date(2024, 1, 1, 1, 0, 0, 0, time.UTC), "02:00", "06:00", false},
		{"after window", time.Date(2024, 1, 1, 7, 0, 0, 0, time.UTC), "02:00", "06:00", false},
		{"overnight in window", time.Date(2024, 1, 1, 23, 0, 0, 0, time.UTC), "22:00", "06:00", true},
		{"overnight early morning", time.Date(2024, 1, 1, 3, 0, 0, 0, time.UTC), "22:00", "06:00", true},
		{"overnight outside", time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC), "22:00", "06:00", false},
		{"empty window", time.Date(2024, 1, 1, 3, 0, 0, 0, time.UTC), "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isInUpdateWindow(tt.now, tt.start, tt.end)
			if result != tt.expect {
				t.Errorf("isInUpdateWindow(%v, %s, %s) = %v, want %v", tt.now, tt.start, tt.end, result, tt.expect)
			}
		})
	}
}
