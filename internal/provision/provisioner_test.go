package provision

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// mockAgentService implements services.AgentServicer for testing.
type mockAgentService struct {
	agents map[string]*storage.Agent
}

func newMockAgentService() *mockAgentService {
	return &mockAgentService{
		agents: make(map[string]*storage.Agent),
	}
}

func (m *mockAgentService) GetByID(_ context.Context, id string) (*storage.Agent, error) {
	agent, ok := m.agents[id]
	if !ok {
		return nil, services.NotFound("GetByID", "agent", id)
	}
	return agent, nil
}

func (m *mockAgentService) Upsert(_ context.Context, agent *storage.Agent) error {
	m.agents[agent.ID] = agent
	return nil
}

func (m *mockAgentService) Delete(_ context.Context, id string) error {
	delete(m.agents, id)
	return nil
}

func (m *mockAgentService) List(_ context.Context) ([]*storage.Agent, error) {
	agents := make([]*storage.Agent, 0, len(m.agents))
	for _, a := range m.agents {
		agents = append(agents, a)
	}
	return agents, nil
}

func (m *mockAgentService) Count(_ context.Context) (int64, error) {
	return int64(len(m.agents)), nil
}

func (m *mockAgentService) CountByStatus(_ context.Context) (map[string]int64, error) {
	result := make(map[string]int64)
	for _, a := range m.agents {
		result[a.Status]++
	}
	return result, nil
}

func (m *mockAgentService) MarkStale(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func setupTestProvisioner(t *testing.T) (*Provisioner, *mockAgentService) {
	t.Helper()

	logger := zap.NewNop()
	mockSvc := newMockAgentService()

	var registeredTokens = make(map[string]string)
	cfg := ProvisionerConfig{
		MasterURL: "https://master.example.com:8443",
		TokenCallback: func(agentID, token string) {
			registeredTokens[agentID] = token
		},
	}

	p := NewProvisioner(mockSvc, nil, nil, logger, cfg)

	return p, mockSvc
}

func TestProvisioner_Provision(t *testing.T) {
	p, _ := setupTestProvisioner(t)
	ctx := context.Background()

	req := &ProvisionRequest{
		AgentID:  "agent-1",
		Hostname: "server1.example.com",
		Labels: map[string]string{
			"env":  "production",
			"role": "web",
		},
	}

	result, err := p.Provision(ctx, req)
	if err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	if result.AgentID != "agent-1" {
		t.Errorf("Expected AgentID 'agent-1', got %q", result.AgentID)
	}

	if result.Token == "" {
		t.Error("Expected token to be generated")
	}

	if len(result.Token) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("Expected 64-char token, got %d chars", len(result.Token))
	}

	if result.InstallScript == "" {
		t.Error("Expected install script to be generated")
	}

	// Check script contains expected values
	if !strings.Contains(result.InstallScript, "agent-1") {
		t.Error("Install script should contain agent ID")
	}
	if !strings.Contains(result.InstallScript, "master.example.com") {
		t.Error("Install script should contain master URL")
	}
	if !strings.Contains(result.InstallScript, result.Token) {
		t.Error("Install script should contain token")
	}
}

func TestProvisioner_Provision_MissingFields(t *testing.T) {
	p, _ := setupTestProvisioner(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		req     *ProvisionRequest
		wantErr string
	}{
		{
			name:    "missing agent_id",
			req:     &ProvisionRequest{Hostname: "host"},
			wantErr: "agent_id is required",
		},
		{
			name:    "missing hostname",
			req:     &ProvisionRequest{AgentID: "agent-1"},
			wantErr: "hostname is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.Provision(ctx, tt.req)
			if err == nil {
				t.Fatal("Expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestProvisioner_Provision_DuplicateAgent(t *testing.T) {
	p, _ := setupTestProvisioner(t)
	ctx := context.Background()

	req := &ProvisionRequest{
		AgentID:  "agent-1",
		Hostname: "server1.example.com",
	}

	// First provision should succeed
	_, err := p.Provision(ctx, req)
	if err != nil {
		t.Fatalf("First provision failed: %v", err)
	}

	// Second provision should fail
	_, err = p.Provision(ctx, req)
	if err == nil {
		t.Fatal("Expected error for duplicate agent")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("Expected 'already exists' error, got: %v", err)
	}
}

func TestProvisioner_Deprovision(t *testing.T) {
	p, mockSvc := setupTestProvisioner(t)
	ctx := context.Background()

	// Provision first
	req := &ProvisionRequest{
		AgentID:  "agent-1",
		Hostname: "server1.example.com",
	}
	_, err := p.Provision(ctx, req)
	if err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	// Verify agent exists
	agent, ok := mockSvc.agents["agent-1"]
	if !ok {
		t.Fatal("Agent should exist in mock")
	}
	if agent == nil {
		t.Fatal("Agent should exist")
	}

	// Deprovision
	err = p.Deprovision(ctx, "agent-1")
	if err != nil {
		t.Fatalf("Deprovision failed: %v", err)
	}

	// Verify agent is gone
	_, ok = mockSvc.agents["agent-1"]
	if ok {
		t.Error("Agent should be deleted")
	}
}

func TestProvisioner_Deprovision_NotFound(t *testing.T) {
	p, _ := setupTestProvisioner(t)
	ctx := context.Background()

	err := p.Deprovision(ctx, "nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent agent")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

func TestProvisioner_ListAgents(t *testing.T) {
	p, _ := setupTestProvisioner(t)
	ctx := context.Background()

	// List empty
	agents, err := p.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents failed: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("Expected 0 agents, got %d", len(agents))
	}

	// Provision some agents
	for i := 1; i <= 3; i++ {
		req := &ProvisionRequest{
			AgentID:  "agent-" + string(rune('0'+i)),
			Hostname: "server" + string(rune('0'+i)) + ".example.com",
		}
		_, err := p.Provision(ctx, req)
		if err != nil {
			t.Fatalf("Provision failed: %v", err)
		}
	}

	// List again
	agents, err = p.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents failed: %v", err)
	}
	if len(agents) != 3 {
		t.Errorf("Expected 3 agents, got %d", len(agents))
	}
}

func TestProvisioner_GetAgent(t *testing.T) {
	p, _ := setupTestProvisioner(t)
	ctx := context.Background()

	// Get nonexistent - should return not found error
	agent, err := p.GetAgent(ctx, "agent-1")
	if !services.IsNotFound(err) {
		t.Fatalf("GetAgent expected not found error, got: %v", err)
	}
	if agent != nil {
		t.Error("Expected nil for nonexistent agent")
	}

	// Provision
	req := &ProvisionRequest{
		AgentID:  "agent-1",
		Hostname: "server1.example.com",
		Labels:   map[string]string{"env": "test"},
	}
	_, err = p.Provision(ctx, req)
	if err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	// Get existing
	agent, err = p.GetAgent(ctx, "agent-1")
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if agent == nil {
		t.Fatal("Expected agent")
	}
	if agent.Hostname != "server1.example.com" {
		t.Errorf("Expected hostname 'server1.example.com', got %q", agent.Hostname)
	}
}

func TestProvisioner_UpdateAgentLabels(t *testing.T) {
	p, _ := setupTestProvisioner(t)
	ctx := context.Background()

	// Provision
	req := &ProvisionRequest{
		AgentID:  "agent-1",
		Hostname: "server1.example.com",
		Labels:   map[string]string{"env": "test"},
	}
	_, err := p.Provision(ctx, req)
	if err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	// Update labels
	newLabels := map[string]string{
		"env":    "production",
		"region": "us-east-1",
	}
	err = p.UpdateAgentLabels(ctx, "agent-1", newLabels)
	if err != nil {
		t.Fatalf("UpdateAgentLabels failed: %v", err)
	}

	// Verify
	agent, err := p.GetAgent(ctx, "agent-1")
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if agent.Labels["env"] != "production" {
		t.Errorf("Expected env 'production', got %q", agent.Labels["env"])
	}
	if agent.Labels["region"] != "us-east-1" {
		t.Errorf("Expected region 'us-east-1', got %q", agent.Labels["region"])
	}
}

func TestProvisioner_UpdateAgentLabels_NotFound(t *testing.T) {
	p, _ := setupTestProvisioner(t)
	ctx := context.Background()

	err := p.UpdateAgentLabels(ctx, "nonexistent", map[string]string{"key": "value"})
	if err == nil {
		t.Fatal("Expected error for nonexistent agent")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

func TestProvisioner_RegenerateToken(t *testing.T) {
	p, _ := setupTestProvisioner(t)
	ctx := context.Background()

	// Provision
	req := &ProvisionRequest{
		AgentID:  "agent-1",
		Hostname: "server1.example.com",
	}
	result, err := p.Provision(ctx, req)
	if err != nil {
		t.Fatalf("Provision failed: %v", err)
	}
	originalToken := result.Token

	// Regenerate token
	newToken, err := p.RegenerateToken(ctx, "agent-1")
	if err != nil {
		t.Fatalf("RegenerateToken failed: %v", err)
	}

	if newToken == "" {
		t.Error("Expected new token")
	}
	if newToken == originalToken {
		t.Error("New token should be different from original")
	}
}

func TestProvisioner_RegenerateToken_AlreadyRegistered(t *testing.T) {
	p, mockSvc := setupTestProvisioner(t)
	ctx := context.Background()

	// Provision
	req := &ProvisionRequest{
		AgentID:  "agent-1",
		Hostname: "server1.example.com",
	}
	_, err := p.Provision(ctx, req)
	if err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	// Simulate agent registration
	agent := mockSvc.agents["agent-1"]
	agent.Status = "online"
	mockSvc.agents["agent-1"] = agent

	// Try to regenerate - should fail
	_, err = p.RegenerateToken(ctx, "agent-1")
	if err == nil {
		t.Fatal("Expected error for already registered agent")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Errorf("Expected 'already registered' error, got: %v", err)
	}
}

func TestProvisioner_GetInstallScript(t *testing.T) {
	p, _ := setupTestProvisioner(t)
	ctx := context.Background()

	// Provision
	req := &ProvisionRequest{
		AgentID:  "agent-1",
		Hostname: "server1.example.com",
	}
	_, err := p.Provision(ctx, req)
	if err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	// Get install script
	script, err := p.GetInstallScript(ctx, "agent-1", "new-token-123")
	if err != nil {
		t.Fatalf("GetInstallScript failed: %v", err)
	}

	if !strings.Contains(script, "agent-1") {
		t.Error("Script should contain agent ID")
	}
	if !strings.Contains(script, "new-token-123") {
		t.Error("Script should contain provided token")
	}
}

func TestProvisioner_GetInstallScript_NotFound(t *testing.T) {
	p, _ := setupTestProvisioner(t)
	ctx := context.Background()

	_, err := p.GetInstallScript(ctx, "nonexistent", "token")
	if err == nil {
		t.Fatal("Expected error for nonexistent agent")
	}
}

func TestGenerateSecureToken(t *testing.T) {
	// Generate multiple tokens
	tokens := make(map[string]bool)
	for i := 0; i < 100; i++ {
		token, err := security.GenerateSecureToken(32)
		if err != nil {
			t.Fatalf("GenerateSecureToken failed: %v", err)
		}
		if len(token) != 64 { // 32 bytes = 64 hex chars
			t.Errorf("Expected 64-char token, got %d chars", len(token))
		}
		if tokens[token] {
			t.Error("Generated duplicate token")
		}
		tokens[token] = true
	}
}

func TestInstallScriptDefaults(t *testing.T) {
	p, _ := setupTestProvisioner(t)
	ctx := context.Background()

	// Provision with minimal fields
	req := &ProvisionRequest{
		AgentID:  "agent-1",
		Hostname: "server1.example.com",
	}

	result, err := p.Provision(ctx, req)
	if err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	// Check defaults are in the script
	if !strings.Contains(result.InstallScript, "/usr/local/bin/vcdeploy-agent") {
		t.Error("Script should contain default install path")
	}
	if !strings.Contains(result.InstallScript, "vcdeploy") { // Service user/group
		t.Error("Script should contain default service user")
	}
}
