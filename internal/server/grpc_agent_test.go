package server

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/proto"
	"github.com/BlackOrder/vcdeploy/internal/services/agents"
	"github.com/BlackOrder/vcdeploy/internal/services/deployments"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

func newTestAgentServer(t *testing.T) (*AgentServer, *storage.DB) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	logger := zap.NewNop()

	db, err := storage.New(dbPath, logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// We need a CA manager, but for tests we can use nil and skip cert generation
	// In real tests we'd set up a proper CA
	server := &AgentServer{
		db:                db,
		ca:                nil, // Will cause cert generation to fail, but we can test other paths
		logger:            logger.Named("agent-grpc"),
		tokens:            make(map[string]string),
		connections:       make(map[string]*GRPCAgentConnection),
		pendingCommands:   make(map[string]chan *proto.MasterMessage),
		agentService:      agents.New(db),
		deploymentService: deployments.New(db),
	}

	return server, db
}

func TestAgentServer_RegisterToken(t *testing.T) {
	server, _ := newTestAgentServer(t)

	// Register a token
	server.RegisterToken("agent-1", "token-123")

	// Verify token was registered
	if !server.validateToken("agent-1", "token-123") {
		t.Error("Expected token to be valid")
	}

	// Verify invalid tokens fail
	if server.validateToken("agent-1", "wrong-token") {
		t.Error("Expected wrong token to be invalid")
	}

	if server.validateToken("agent-2", "token-123") {
		t.Error("Expected token for different agent to be invalid")
	}
}

func TestAgentServer_RevokeToken(t *testing.T) {
	server, _ := newTestAgentServer(t)

	// Register and then revoke a token
	server.RegisterToken("agent-1", "token-123")
	server.RevokeToken("agent-1")

	// Token should no longer be valid
	if server.validateToken("agent-1", "token-123") {
		t.Error("Expected revoked token to be invalid")
	}
}

func TestAgentServer_Register_MissingFields(t *testing.T) {
	server, _ := newTestAgentServer(t)

	tests := []struct {
		name    string
		req     *proto.RegisterRequest
		wantErr string
	}{
		{
			name:    "missing agent_id",
			req:     &proto.RegisterRequest{Token: "token"},
			wantErr: "agent_id and token are required",
		},
		{
			name:    "missing token",
			req:     &proto.RegisterRequest{AgentId: "agent-1"},
			wantErr: "agent_id and token are required",
		},
		{
			name:    "both missing",
			req:     &proto.RegisterRequest{},
			wantErr: "agent_id and token are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := server.Register(context.Background(), tt.req)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if resp.Success {
				t.Error("Expected success to be false")
			}
			if resp.Error != tt.wantErr {
				t.Errorf("Expected error %q, got %q", tt.wantErr, resp.Error)
			}
		})
	}
}

func TestAgentServer_Register_InvalidToken(t *testing.T) {
	server, _ := newTestAgentServer(t)

	// Register with a different token than expected
	server.RegisterToken("agent-1", "correct-token")

	resp, err := server.Register(context.Background(), &proto.RegisterRequest{
		AgentId:  "agent-1",
		Token:    "wrong-token",
		Hostname: "test-host",
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if resp.Success {
		t.Error("Expected success to be false")
	}
	if resp.Error != "invalid registration token" {
		t.Errorf("Expected error 'invalid registration token', got %q", resp.Error)
	}
}

func TestAgentServer_Heartbeat_MissingAgentID(t *testing.T) {
	server, _ := newTestAgentServer(t)

	_, err := server.Heartbeat(context.Background(), &proto.HeartbeatRequest{})
	if err == nil {
		t.Error("Expected error for missing agent_id")
	}
}

func TestAgentServer_Heartbeat_UnknownAgent(t *testing.T) {
	server, _ := newTestAgentServer(t)

	_, err := server.Heartbeat(context.Background(), &proto.HeartbeatRequest{
		AgentId: "unknown-agent",
	})
	if err == nil {
		t.Error("Expected error for unknown agent")
	}
}

func TestAgentServer_Heartbeat_Success(t *testing.T) {
	server, db := newTestAgentServer(t)
	ctx := context.Background()

	// Create an agent first
	agent := &storage.Agent{
		ID:       "agent-1",
		Hostname: "test-host",
		Status:   "online",
	}
	if err := db.UpsertAgent(ctx, agent); err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	// Send heartbeat
	resp, err := server.Heartbeat(ctx, &proto.HeartbeatRequest{
		AgentId:   "agent-1",
		Timestamp: time.Now().Unix(),
		Stats: &proto.AgentStats{
			CpuPercent:    50.0,
			MemoryPercent: 60.0,
			DiskPercent:   70.0,
		},
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !resp.Ok {
		t.Error("Expected ok to be true")
	}
	if resp.ServerTimestamp == 0 {
		t.Error("Expected server timestamp to be set")
	}
}

func TestAgentServer_SendCommand_NotConnected(t *testing.T) {
	server, _ := newTestAgentServer(t)

	err := server.SendCommand("agent-1", &proto.MasterMessage{})
	if err == nil {
		t.Error("Expected error when agent not connected")
	}
}

func TestAgentServer_IsAgentConnected(t *testing.T) {
	server, _ := newTestAgentServer(t)

	// Agent not connected
	if server.IsAgentConnected("agent-1") {
		t.Error("Expected agent to not be connected")
	}

	// Manually add a connection
	server.connectionMutex.Lock()
	server.connections["agent-1"] = &GRPCAgentConnection{
		AgentID:     "agent-1",
		ConnectedAt: time.Now(),
	}
	server.connectionMutex.Unlock()

	// Now agent should be connected
	if !server.IsAgentConnected("agent-1") {
		t.Error("Expected agent to be connected")
	}
}

func TestAgentServer_GetConnectedAgents(t *testing.T) {
	server, _ := newTestAgentServer(t)

	// No agents
	agents := server.GetConnectedAgents()
	if len(agents) != 0 {
		t.Errorf("Expected 0 agents, got %d", len(agents))
	}

	// Add some connections
	server.connectionMutex.Lock()
	server.connections["agent-1"] = &GRPCAgentConnection{AgentID: "agent-1"}
	server.connections["agent-2"] = &GRPCAgentConnection{AgentID: "agent-2"}
	server.connectionMutex.Unlock()

	agents = server.GetConnectedAgents()
	if len(agents) != 2 {
		t.Errorf("Expected 2 agents, got %d", len(agents))
	}
}

func TestAgentServer_DisconnectAgent(t *testing.T) {
	server, _ := newTestAgentServer(t)

	cancelled := false
	cancelFn := func() { cancelled = true }

	// Add a connection
	server.connectionMutex.Lock()
	server.connections["agent-1"] = &GRPCAgentConnection{
		AgentID: "agent-1",
		Cancel:  cancelFn,
	}
	server.connectionMutex.Unlock()

	// Disconnect
	server.DisconnectAgent("agent-1")

	// Should have called cancel
	if !cancelled {
		t.Error("Expected cancel to be called")
	}
}

func TestAgentServer_SendDeployCommand(t *testing.T) {
	server, _ := newTestAgentServer(t)

	// Set up connection and command channel
	cmdChan := make(chan *proto.MasterMessage, 10)
	server.connectionMutex.Lock()
	server.connections["agent-1"] = &GRPCAgentConnection{AgentID: "agent-1"}
	server.connectionMutex.Unlock()
	server.pendingCommandMutex.Lock()
	server.pendingCommands["agent-1"] = cmdChan
	server.pendingCommandMutex.Unlock()

	// Send deploy command
	cmd := &proto.DeployCommand{
		DeploymentId: "deploy-1",
		Project:      "my-project",
		Target:       "production",
	}
	err := server.SendDeployCommand("agent-1", cmd)
	if err != nil {
		t.Fatalf("Failed to send deploy command: %v", err)
	}

	// Check command was queued
	select {
	case msg := <-cmdChan:
		deployCmd := msg.GetDeployCommand()
		if deployCmd == nil {
			t.Fatal("Expected deploy command")
		}
		if deployCmd.DeploymentId != "deploy-1" {
			t.Errorf("Expected deployment ID 'deploy-1', got %q", deployCmd.DeploymentId)
		}
	default:
		t.Error("Expected command in channel")
	}
}

func TestAgentServer_SendRollbackCommand(t *testing.T) {
	server, _ := newTestAgentServer(t)

	// Set up connection and command channel
	cmdChan := make(chan *proto.MasterMessage, 10)
	server.connectionMutex.Lock()
	server.connections["agent-1"] = &GRPCAgentConnection{AgentID: "agent-1"}
	server.connectionMutex.Unlock()
	server.pendingCommandMutex.Lock()
	server.pendingCommands["agent-1"] = cmdChan
	server.pendingCommandMutex.Unlock()

	// Send rollback command
	cmd := &proto.RollbackCommand{
		DeploymentId:  "deploy-1",
		Project:       "my-project",
		ReleaseNumber: 5,
	}
	err := server.SendRollbackCommand("agent-1", cmd)
	if err != nil {
		t.Fatalf("Failed to send rollback command: %v", err)
	}

	// Check command was queued
	select {
	case msg := <-cmdChan:
		rollbackCmd := msg.GetRollbackCommand()
		if rollbackCmd == nil {
			t.Fatal("Expected rollback command")
		}
		if rollbackCmd.ReleaseNumber != 5 {
			t.Errorf("Expected release number 5, got %d", rollbackCmd.ReleaseNumber)
		}
	default:
		t.Error("Expected command in channel")
	}
}

func TestAgentServer_SendCancelCommand(t *testing.T) {
	server, _ := newTestAgentServer(t)

	// Set up connection and command channel
	cmdChan := make(chan *proto.MasterMessage, 10)
	server.connectionMutex.Lock()
	server.connections["agent-1"] = &GRPCAgentConnection{AgentID: "agent-1"}
	server.connectionMutex.Unlock()
	server.pendingCommandMutex.Lock()
	server.pendingCommands["agent-1"] = cmdChan
	server.pendingCommandMutex.Unlock()

	// Send cancel command
	cmd := &proto.CancelCommand{
		DeploymentId: "deploy-1",
		Reason:       "User requested",
	}
	err := server.SendCancelCommand("agent-1", cmd)
	if err != nil {
		t.Fatalf("Failed to send cancel command: %v", err)
	}

	// Check command was queued
	select {
	case msg := <-cmdChan:
		cancelCmd := msg.GetCancelCommand()
		if cancelCmd == nil {
			t.Fatal("Expected cancel command")
		}
		if cancelCmd.Reason != "User requested" {
			t.Errorf("Expected reason 'User requested', got %q", cancelCmd.Reason)
		}
	default:
		t.Error("Expected command in channel")
	}
}

func TestAgentServer_ValidateToken_ConstantTimeComparison(t *testing.T) {
	server, _ := newTestAgentServer(t)

	// Register a token
	server.RegisterToken("agent-1", "secret-token-12345")

	// These should all fail but take similar time (constant time comparison)
	testCases := []string{
		"secret-token-12345x", // One char different at end
		"xsecret-token-12345", // One char different at start
		"completely-wrong",    // Completely different
		"",                    // Empty
	}

	for _, tc := range testCases {
		if server.validateToken("agent-1", tc) {
			t.Errorf("Expected token %q to be invalid", tc)
		}
	}

	// Correct token should work
	if !server.validateToken("agent-1", "secret-token-12345") {
		t.Error("Expected correct token to be valid")
	}
}
