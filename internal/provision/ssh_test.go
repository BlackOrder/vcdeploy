package provision

import (
	"context"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// mockAgentServicer implements services.AgentServicer for testing.
type mockAgentServicer struct {
	agents map[string]*storage.Agent
}

func newMockAgentServicer() *mockAgentServicer {
	return &mockAgentServicer{
		agents: make(map[string]*storage.Agent),
	}
}

func (m *mockAgentServicer) GetByID(ctx context.Context, id string) (*storage.Agent, error) {
	if agent, ok := m.agents[id]; ok {
		return agent, nil
	}
	return nil, services.ErrNotFound
}

func (m *mockAgentServicer) GetByHostname(ctx context.Context, hostname string) (*storage.Agent, error) {
	for _, agent := range m.agents {
		if agent.Hostname == hostname {
			return agent, nil
		}
	}
	return nil, services.ErrNotFound
}

func (m *mockAgentServicer) List(ctx context.Context) ([]*storage.Agent, error) {
	var result []*storage.Agent
	for _, agent := range m.agents {
		result = append(result, agent)
	}
	return result, nil
}

func (m *mockAgentServicer) ListPaginated(ctx context.Context, p services.Pagination) (*services.ListResult[*storage.Agent], error) {
	agents, _ := m.List(ctx)
	return &services.ListResult[*storage.Agent]{
		Items:      agents,
		TotalCount: int64(len(agents)),
	}, nil
}

func (m *mockAgentServicer) Count(ctx context.Context) (int64, error) {
	return int64(len(m.agents)), nil
}

func (m *mockAgentServicer) CountByStatus(ctx context.Context) (map[string]int64, error) {
	result := make(map[string]int64)
	for _, agent := range m.agents {
		result[agent.Status]++
	}
	return result, nil
}

func (m *mockAgentServicer) Upsert(ctx context.Context, agent *storage.Agent) error {
	m.agents[agent.ID] = agent
	return nil
}

func (m *mockAgentServicer) Delete(ctx context.Context, id string) error {
	delete(m.agents, id)
	return nil
}

func (m *mockAgentServicer) MarkStale(ctx context.Context, cutoff time.Time) (int64, error) {
	var count int64
	for _, agent := range m.agents {
		if !agent.LastSeenAt.IsZero() && agent.LastSeenAt.Before(cutoff) {
			agent.Status = "stale"
			count++
		}
	}
	return count, nil
}

func (m *mockAgentServicer) UpdateStatus(ctx context.Context, id, status string, ts time.Time) error {
	if agent, ok := m.agents[id]; ok {
		agent.Status = status
		agent.LastSeenAt = ts
	}
	return nil
}

func TestNewSSHProvisioner(t *testing.T) {
	t.Parallel()

	logger, _ := zap.NewDevelopment()
	provisioner := NewProvisioner(newMockAgentServicer(), nil, nil, logger, ProvisionerConfig{
		MasterURL: "https://master.example.com:8443",
	})

	cfg := SSHProvisionerConfig{
		MasterURL:         "https://master.example.com:8443",
		BinaryURL:         "https://master.example.com:8443/api/v1/agent/binary",
		ConnectionTimeout: 30 * time.Second,
		ExecutionTimeout:  5 * time.Minute,
	}

	sshProv := NewSSHProvisioner(nil, provisioner, logger, cfg)
	if sshProv == nil {
		t.Fatal("NewSSHProvisioner returned nil")
	}

	if sshProv.connTimeout != 30*time.Second {
		t.Errorf("expected connTimeout 30s, got %v", sshProv.connTimeout)
	}

	if sshProv.execTimeout != 5*time.Minute {
		t.Errorf("expected execTimeout 5m, got %v", sshProv.execTimeout)
	}
}

func TestSSHProvisionRequest_Validation(t *testing.T) {
	t.Parallel()

	logger, _ := zap.NewDevelopment()
	provisioner := NewProvisioner(newMockAgentServicer(), nil, nil, logger, ProvisionerConfig{
		MasterURL: "https://master.example.com:8443",
	})

	sshProv := NewSSHProvisioner(nil, provisioner, logger, SSHProvisionerConfig{
		MasterURL: "https://master.example.com:8443",
	})

	ctx := context.Background()

	tests := []struct {
		name    string
		req     *SSHProvisionRequest
		wantErr string
	}{
		{
			name:    "missing agent_id",
			req:     &SSHProvisionRequest{TargetHost: "host", SSHUser: "user", SSHKeyName: "key"},
			wantErr: "agent_id is required",
		},
		{
			name:    "missing target_host",
			req:     &SSHProvisionRequest{AgentID: "test", SSHUser: "user", SSHKeyName: "key"},
			wantErr: "target_host is required",
		},
		{
			name:    "missing ssh_user",
			req:     &SSHProvisionRequest{AgentID: "test", TargetHost: "host", SSHKeyName: "key"},
			wantErr: "ssh_user is required",
		},
		{
			name:    "missing ssh_key_name",
			req:     &SSHProvisionRequest{AgentID: "test", TargetHost: "host", SSHUser: "user"},
			wantErr: "ssh_key_name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := sshProv.SSHProvision(ctx, tt.req)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("expected error %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestSSHProvisionRequest_Defaults(t *testing.T) {
	t.Parallel()

	req := &SSHProvisionRequest{
		AgentID:    "test-agent",
		TargetHost: "192.168.1.100",
		SSHUser:    "admin",
		SSHKeyName: "default",
	}

	// Test defaults before they're applied
	if req.TargetPort != 0 {
		t.Errorf("expected TargetPort 0 before defaults, got %d", req.TargetPort)
	}
	if req.InstallPath != "" {
		t.Errorf("expected InstallPath empty before defaults, got %s", req.InstallPath)
	}
	if req.ServiceUser != "" {
		t.Errorf("expected ServiceUser empty before defaults, got %s", req.ServiceUser)
	}
	if req.ServiceGroup != "" {
		t.Errorf("expected ServiceGroup empty before defaults, got %s", req.ServiceGroup)
	}
}

func TestSSHProvisionerConfig_Defaults(t *testing.T) {
	t.Parallel()

	logger, _ := zap.NewDevelopment()
	provisioner := NewProvisioner(newMockAgentServicer(), nil, nil, logger, ProvisionerConfig{
		MasterURL: "https://master.example.com:8443",
	})

	// Empty config should get defaults
	cfg := SSHProvisionerConfig{}
	sshProv := NewSSHProvisioner(nil, provisioner, logger, cfg)

	if sshProv.connTimeout != 30*time.Second {
		t.Errorf("expected default connTimeout 30s, got %v", sshProv.connTimeout)
	}

	if sshProv.execTimeout != 5*time.Minute {
		t.Errorf("expected default execTimeout 5m, got %v", sshProv.execTimeout)
	}
}

func TestSSHProvisionResult_Fields(t *testing.T) {
	t.Parallel()

	startTime := time.Now()
	result := &SSHProvisionResult{
		AgentID:     "test-agent",
		TargetHost:  "192.168.1.100",
		Status:      "provisioned",
		Token:       "secret-token",
		Output:      "Installation successful",
		StartedAt:   startTime,
		CompletedAt: startTime.Add(30 * time.Second),
	}

	if result.AgentID != "test-agent" {
		t.Errorf("expected AgentID 'test-agent', got '%s'", result.AgentID)
	}

	if result.Status != "provisioned" {
		t.Errorf("expected Status 'provisioned', got '%s'", result.Status)
	}

	duration := result.CompletedAt.Sub(result.StartedAt)
	if duration != 30*time.Second {
		t.Errorf("expected duration 30s, got %v", duration)
	}
}

func TestSSHProvisioner_Errors(t *testing.T) {
	t.Parallel()

	// Test error types
	if ErrAgentAlreadyInstalled.Error() != "agent already installed on target" {
		t.Errorf("unexpected error message: %s", ErrAgentAlreadyInstalled.Error())
	}

	if ErrSSHConnectionFailed.Error() != "SSH connection failed" {
		t.Errorf("unexpected error message: %s", ErrSSHConnectionFailed.Error())
	}

	if ErrInstallationFailed.Error() != "agent installation failed" {
		t.Errorf("unexpected error message: %s", ErrInstallationFailed.Error())
	}

	if ErrSSHKeyNotFound.Error() != "SSH key not found" {
		t.Errorf("unexpected error message: %s", ErrSSHKeyNotFound.Error())
	}
}

// Note: Full integration tests would require a test SSH server.
// These unit tests cover the request validation and configuration logic.
// Integration tests for actual SSH provisioning should be run against
// a test environment with a real or mock SSH server.

// TestMockAgentServicer verifies the mock implementation works correctly.
func TestMockAgentServicer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mock := newMockAgentServicer()

	// Test empty list
	agents, err := mock.List(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("expected empty list, got %d agents", len(agents))
	}

	// Test not found
	_, err = mock.GetByID(ctx, "nonexistent")
	if err != services.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Test upsert and get
	agent := &storage.Agent{
		ID:       "test-agent",
		Hostname: "test-host",
		Status:   "pending",
	}
	if err := mock.Upsert(ctx, agent); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := mock.GetByID(ctx, "test-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Hostname != "test-host" {
		t.Errorf("expected hostname 'test-host', got '%s'", got.Hostname)
	}

	// Test GetByHostname
	got, err = mock.GetByHostname(ctx, "test-host")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "test-agent" {
		t.Errorf("expected ID 'test-agent', got '%s'", got.ID)
	}

	// Test delete
	if err := mock.Delete(ctx, "test-agent"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = mock.GetByID(ctx, "test-agent")
	if err != services.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

// Verify the mock satisfies the interface.
var _ services.AgentServicer = (*mockAgentServicer)(nil)
