package proto

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
)

// mockAgentServiceServer implements AgentServiceServer for testing.
type mockAgentServiceServer struct {
	UnimplementedAgentServiceServer
	registerCalled  bool
	heartbeatCalled bool
	connectCalled   bool
}

func (m *mockAgentServiceServer) Register(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error) {
	m.registerCalled = true
	return &RegisterResponse{
		Success: true,
	}, nil
}

func (m *mockAgentServiceServer) Heartbeat(ctx context.Context, req *HeartbeatRequest) (*HeartbeatResponse, error) {
	m.heartbeatCalled = true
	return &HeartbeatResponse{
		Ok:              true,
		ServerTimestamp: time.Now().Unix(),
	}, nil
}

func (m *mockAgentServiceServer) Connect(stream AgentService_ConnectServer) error {
	m.connectCalled = true
	return nil
}

func TestAgentServiceServer_Register(t *testing.T) {
	t.Parallel()

	mock := &mockAgentServiceServer{}
	ctx := context.Background()

	req := &RegisterRequest{
		AgentId:  "agent-001",
		Token:    "secret-token",
		Hostname: "agent-host.example.com",
		Labels: map[string]string{
			"env":    "production",
			"region": "us-west",
		},
		Capabilities: &AgentCapabilities{
			CanUseNamespaces: true,
			DiskSpaceBytes:   1024 * 1024 * 1024,
			MemoryBytes:      4 * 1024 * 1024 * 1024,
		},
	}

	resp, err := mock.Register(ctx, req)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if !resp.Success {
		t.Error("Register() should succeed")
	}
	if !mock.registerCalled {
		t.Error("Register() was not called")
	}
}

func TestAgentServiceServer_Heartbeat(t *testing.T) {
	t.Parallel()

	mock := &mockAgentServiceServer{}
	ctx := context.Background()

	req := &HeartbeatRequest{
		AgentId:   "agent-001",
		Timestamp: time.Now().Unix(),
		Stats: &AgentStats{
			CpuPercent:    25.5,
			MemoryPercent: 50.0,
			DiskPercent:   60.0,
		},
	}

	resp, err := mock.Heartbeat(ctx, req)
	if err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}

	if !resp.Ok {
		t.Error("Heartbeat() should succeed")
	}
	if resp.ServerTimestamp == 0 {
		t.Error("Heartbeat() should return server timestamp")
	}
	if !mock.heartbeatCalled {
		t.Error("Heartbeat() was not called")
	}
}

func TestRegisterAgentServiceServer(t *testing.T) {
	t.Parallel()

	server := grpc.NewServer()
	mock := &mockAgentServiceServer{}

	// Should not panic
	RegisterAgentServiceServer(server, mock)
	server.Stop()
}

func TestUnimplementedAgentServiceServer(t *testing.T) {
	t.Parallel()

	server := UnimplementedAgentServiceServer{}

	// Register should return unimplemented error
	resp, err := server.Register(context.Background(), &RegisterRequest{})
	if err == nil {
		t.Error("Unimplemented Register() should return error")
	}
	if resp != nil {
		t.Error("Unimplemented Register() should return nil response")
	}

	// Heartbeat should return unimplemented error
	hbResp, err := server.Heartbeat(context.Background(), &HeartbeatRequest{})
	if err == nil {
		t.Error("Unimplemented Heartbeat() should return error")
	}
	if hbResp != nil {
		t.Error("Unimplemented Heartbeat() should return nil response")
	}
}

// Test proto message types
func TestDeploymentState_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		state    DeploymentState
		expected string
	}{
		{DeploymentState_DEPLOYMENT_STATE_UNSPECIFIED, "DEPLOYMENT_STATE_UNSPECIFIED"},
		{DeploymentState_DEPLOYMENT_STATE_PENDING, "DEPLOYMENT_STATE_PENDING"},
		{DeploymentState_DEPLOYMENT_STATE_COMPLETED, "DEPLOYMENT_STATE_COMPLETED"},
		{DeploymentState_DEPLOYMENT_STATE_FAILED, "DEPLOYMENT_STATE_FAILED"},
		{DeploymentState(999), "999"}, // Proto returns numeric value for unknown enums
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("String() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestLogLevel_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		level    LogLevel
		expected string
	}{
		{LogLevel_LOG_LEVEL_DEBUG, "LOG_LEVEL_DEBUG"},
		{LogLevel_LOG_LEVEL_INFO, "LOG_LEVEL_INFO"},
		{LogLevel_LOG_LEVEL_WARN, "LOG_LEVEL_WARN"},
		{LogLevel_LOG_LEVEL_ERROR, "LOG_LEVEL_ERROR"},
		{LogLevel(999), "999"}, // Proto returns numeric value for unknown enums
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			if got := tt.level.String(); got != tt.expected {
				t.Errorf("String() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestRegisterRequest_Getters(t *testing.T) {
	t.Parallel()

	req := &RegisterRequest{
		AgentId:  "agent-001",
		Token:    "secret",
		Hostname: "host.example.com",
		Labels:   map[string]string{"env": "prod"},
		Capabilities: &AgentCapabilities{
			CanUseNamespaces: true,
			DiskSpaceBytes:   1024,
		},
	}

	if req.GetAgentId() != "agent-001" {
		t.Errorf("GetAgentId() = %s, want agent-001", req.GetAgentId())
	}
	if req.GetToken() != "secret" {
		t.Errorf("GetToken() = %s, want secret", req.GetToken())
	}
	if req.GetHostname() != "host.example.com" {
		t.Errorf("GetHostname() = %s, want host.example.com", req.GetHostname())
	}
	if len(req.GetLabels()) != 1 {
		t.Errorf("GetLabels() len = %d, want 1", len(req.GetLabels()))
	}
	if req.GetCapabilities() == nil {
		t.Error("GetCapabilities() should not be nil")
	}

	// Test nil request
	var nilReq *RegisterRequest
	if nilReq.GetAgentId() != "" {
		t.Error("nil.GetAgentId() should return empty string")
	}
	if nilReq.GetLabels() != nil {
		t.Error("nil.GetLabels() should return nil")
	}
	if nilReq.GetCapabilities() != nil {
		t.Error("nil.GetCapabilities() should return nil")
	}
}

func TestRegisterRequest_Reset(t *testing.T) {
	t.Parallel()

	req := &RegisterRequest{
		AgentId:  "agent-001",
		Token:    "secret",
		Hostname: "host.example.com",
	}

	req.Reset()

	if req.AgentId != "" {
		t.Error("Reset() should clear AgentId")
	}
	if req.Token != "" {
		t.Error("Reset() should clear Token")
	}
}

func TestAgentCapabilities_Getters(t *testing.T) {
	t.Parallel()

	caps := &AgentCapabilities{
		CanUseNamespaces: true,
		AllowedUsers:     []string{"user1", "user2"},
		DiskSpaceBytes:   1024,
		MemoryBytes:      2048,
	}

	if !caps.GetCanUseNamespaces() {
		t.Error("GetCanUseNamespaces() should be true")
	}
	if len(caps.GetAllowedUsers()) != 2 {
		t.Errorf("GetAllowedUsers() len = %d, want 2", len(caps.GetAllowedUsers()))
	}

	// Test nil
	var nilCaps *AgentCapabilities
	if nilCaps.GetCanUseNamespaces() {
		t.Error("nil.GetCanUseNamespaces() should be false")
	}
	if nilCaps.GetAllowedUsers() != nil {
		t.Error("nil.GetAllowedUsers() should be nil")
	}
}

func TestDeploymentStatus_Getters(t *testing.T) {
	t.Parallel()

	status := &DeploymentStatus{
		DeploymentId:    "deploy-001",
		State:           DeploymentState_DEPLOYMENT_STATE_COMPLETED,
		Message:         "Success",
		Timestamp:       time.Now().Unix(),
		ProgressPercent: 100,
		CurrentStep:     "done",
		ReleaseNumber:   5,
	}

	if status.GetDeploymentId() != "deploy-001" {
		t.Errorf("GetDeploymentId() = %s, want deploy-001", status.GetDeploymentId())
	}
	if status.GetState() != DeploymentState_DEPLOYMENT_STATE_COMPLETED {
		t.Error("GetState() should be COMPLETED")
	}
	if status.GetProgressPercent() != 100 {
		t.Errorf("GetProgressPercent() = %d, want 100", status.GetProgressPercent())
	}
}

func TestDeploymentLog_Getters(t *testing.T) {
	t.Parallel()

	log := &DeploymentLog{
		DeploymentId: "deploy-001",
		Timestamp:    time.Now().Unix(),
		Level:        LogLevel_LOG_LEVEL_INFO,
		Message:      "Deployment started",
		Source:       "executor",
	}

	if log.GetDeploymentId() != "deploy-001" {
		t.Errorf("GetDeploymentId() = %s, want deploy-001", log.GetDeploymentId())
	}
	if log.GetLevel() != LogLevel_LOG_LEVEL_INFO {
		t.Error("GetLevel() should be INFO")
	}
	if log.GetSource() != "executor" {
		t.Errorf("GetSource() = %s, want executor", log.GetSource())
	}
}

func TestDeployCommand_Getters(t *testing.T) {
	t.Parallel()

	cmd := &DeployCommand{
		DeploymentId: "deploy-001",
		Project:      "my-project",
		Target:       "production",
		Repository:   "git@github.com:user/repo.git",
		Branch:       "main",
		Commit:       "abc123",
		Path:         "/var/www/app",
		EnvVars:      map[string]string{"ENV": "prod"},
	}

	if cmd.GetDeploymentId() != "deploy-001" {
		t.Errorf("GetDeploymentId() = %s, want deploy-001", cmd.GetDeploymentId())
	}
	if cmd.GetProject() != "my-project" {
		t.Errorf("GetProject() = %s, want my-project", cmd.GetProject())
	}
	if cmd.GetTarget() != "production" {
		t.Errorf("GetTarget() = %s, want production", cmd.GetTarget())
	}
	if cmd.GetBranch() != "main" {
		t.Errorf("GetBranch() = %s, want main", cmd.GetBranch())
	}
	if len(cmd.GetEnvVars()) != 1 {
		t.Errorf("GetEnvVars() len = %d, want 1", len(cmd.GetEnvVars()))
	}
}

func TestCommandResult_Getters(t *testing.T) {
	t.Parallel()

	result := &CommandResult{
		DeploymentId: "deploy-001",
		Command:      "npm install",
		ExitCode:     0,
		Stdout:       "packages installed",
		Stderr:       "",
		DurationMs:   5000,
	}

	if result.GetDeploymentId() != "deploy-001" {
		t.Errorf("GetDeploymentId() = %s, want deploy-001", result.GetDeploymentId())
	}
	if result.GetExitCode() != 0 {
		t.Errorf("GetExitCode() = %d, want 0", result.GetExitCode())
	}
	if result.GetDurationMs() != 5000 {
		t.Errorf("GetDurationMs() = %d, want 5000", result.GetDurationMs())
	}
}

func TestMasterMessage_Types(t *testing.T) {
	t.Parallel()

	t.Run("DeployCommand", func(t *testing.T) {
		t.Parallel()
		msg := &MasterMessage{
			Message: &MasterMessage_DeployCommand{
				DeployCommand: &DeployCommand{
					DeploymentId: "deploy-001",
					Project:      "test-project",
				},
			},
		}

		if msg.GetDeployCommand() == nil {
			t.Error("GetDeployCommand() should not be nil")
		}
		if msg.GetRollbackCommand() != nil {
			t.Error("GetRollbackCommand() should be nil")
		}
	})

	t.Run("RollbackCommand", func(t *testing.T) {
		t.Parallel()
		msg := &MasterMessage{
			Message: &MasterMessage_RollbackCommand{
				RollbackCommand: &RollbackCommand{
					DeploymentId: "deploy-001",
				},
			},
		}

		if msg.GetRollbackCommand() == nil {
			t.Error("GetRollbackCommand() should not be nil")
		}
		if msg.GetDeployCommand() != nil {
			t.Error("GetDeployCommand() should be nil")
		}
	})
}

func TestAgentMessage_Types(t *testing.T) {
	t.Parallel()

	t.Run("DeploymentStatus", func(t *testing.T) {
		t.Parallel()
		msg := &AgentMessage{
			Message: &AgentMessage_DeploymentStatus{
				DeploymentStatus: &DeploymentStatus{
					DeploymentId: "deploy-001",
					State:        DeploymentState_DEPLOYMENT_STATE_COMPLETED,
				},
			},
		}

		if msg.GetDeploymentStatus() == nil {
			t.Error("GetDeploymentStatus() should not be nil")
		}
		if msg.GetDeploymentLog() != nil {
			t.Error("GetDeploymentLog() should be nil")
		}
	})

	t.Run("DeploymentLog", func(t *testing.T) {
		t.Parallel()
		msg := &AgentMessage{
			Message: &AgentMessage_DeploymentLog{
				DeploymentLog: &DeploymentLog{
					DeploymentId: "deploy-001",
					Message:      "Test log",
				},
			},
		}

		if msg.GetDeploymentLog() == nil {
			t.Error("GetDeploymentLog() should not be nil")
		}
		if msg.GetDeploymentStatus() != nil {
			t.Error("GetDeploymentStatus() should be nil")
		}
	})

	t.Run("AgentReady", func(t *testing.T) {
		t.Parallel()
		msg := &AgentMessage{
			Message: &AgentMessage_AgentReady{
				AgentReady: &AgentReady{
					AgentId:   "agent-001",
					Timestamp: time.Now().Unix(),
				},
			},
		}

		if msg.GetAgentReady() == nil {
			t.Error("GetAgentReady() should not be nil")
		}
	})
}
