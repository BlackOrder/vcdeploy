// Package server provides the master daemon HTTP and gRPC servers.
package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/proto"
	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AgentServer implements the AgentServiceServer interface for agent-master communication.
type AgentServer struct {
	proto.UnimplementedAgentServiceServer

	db     *storage.DB
	ca     *security.CAManager
	logger *zap.Logger

	// Service layer
	agentService      services.AgentServicer
	deploymentService services.DeploymentServicer

	// tokens maps agent IDs to their registration tokens
	tokens     map[string]string
	tokenMutex sync.RWMutex

	// connections tracks active agent connections
	connections     map[string]*GRPCAgentConnection
	connectionMutex sync.RWMutex

	// pendingCommands holds commands waiting to be sent to agents
	pendingCommands     map[string]chan *proto.MasterMessage
	pendingCommandMutex sync.RWMutex
}

// GRPCAgentConnection represents an active agent gRPC connection.
type GRPCAgentConnection struct {
	AgentID     string
	Hostname    string
	Stream      proto.AgentService_ConnectServer
	ConnectedAt time.Time
	LastMessage time.Time
	Cancel      context.CancelFunc
	CleanupDone chan struct{} // Closed when connection cleanup completes
}

// NewAgentServer creates a new agent gRPC server.
func NewAgentServer(db *storage.DB, ca *security.CAManager, logger *zap.Logger) *AgentServer {
	return &AgentServer{
		db:              db,
		ca:              ca,
		logger:          logger.Named("agent-grpc"),
		tokens:          make(map[string]string),
		connections:     make(map[string]*GRPCAgentConnection),
		pendingCommands: make(map[string]chan *proto.MasterMessage),
	}
}

// SetServices sets the service layer dependencies for the AgentServer.
func (s *AgentServer) SetServices(agentSvc services.AgentServicer, deploymentSvc services.DeploymentServicer) {
	s.agentService = agentSvc
	s.deploymentService = deploymentSvc
}

// RegisterToken adds a registration token for an agent.
// This is called by the master when provisioning a new agent.
func (s *AgentServer) RegisterToken(agentID, token string) {
	s.tokenMutex.Lock()
	defer s.tokenMutex.Unlock()
	s.tokens[agentID] = token
	s.logger.Debug("Registered agent token", zap.String("agent_id", agentID))
}

// RevokeToken removes a registration token.
func (s *AgentServer) RevokeToken(agentID string) {
	s.tokenMutex.Lock()
	defer s.tokenMutex.Unlock()
	delete(s.tokens, agentID)
}

// Register handles agent registration requests.
// The agent provides a one-time token and receives a certificate for mTLS.
func (s *AgentServer) Register(ctx context.Context, req *proto.RegisterRequest) (*proto.RegisterResponse, error) {
	if req.AgentId == "" || req.Token == "" {
		return &proto.RegisterResponse{
			Success: false,
			Error:   "agent_id and token are required",
		}, nil
	}

	// Validate the registration token
	if !s.validateToken(req.AgentId, req.Token) {
		s.logger.Warn("Invalid registration token",
			zap.String("agent_id", req.AgentId),
			zap.String("hostname", req.Hostname),
		)
		return &proto.RegisterResponse{
			Success: false,
			Error:   "invalid registration token",
		}, nil
	}

	s.logger.Info("Agent registration request",
		zap.String("agent_id", req.AgentId),
		zap.String("hostname", req.Hostname),
	)

	// Issue a certificate for the agent
	cert, err := s.ca.IssueAgentCertificate(ctx, req.AgentId, req.Hostname)
	if err != nil {
		s.logger.Error("Failed to issue agent certificate",
			zap.String("agent_id", req.AgentId),
			zap.Error(err),
		)
		return &proto.RegisterResponse{
			Success: false,
			Error:   "failed to issue certificate",
		}, nil
	}

	// Store agent information
	capabilities, err := json.Marshal(req.Capabilities)
	if err != nil {
		s.logger.Error("Failed to marshal capabilities", zap.Error(err))
		capabilities = []byte("{}")
	}
	agent := &storage.Agent{
		ID:           req.AgentId,
		Hostname:     req.Hostname,
		Labels:       req.Labels,
		Capabilities: string(capabilities),
		Status:       "registered",
		LastSeenAt:   time.Now(),
		Certificate:  cert.SerialNumber,
	}

	if err := s.agentService.Upsert(ctx, agent); err != nil {
		s.logger.Error("Failed to store agent",
			zap.String("agent_id", req.AgentId),
			zap.Error(err),
		)
		return &proto.RegisterResponse{
			Success: false,
			Error:   "failed to store agent information",
		}, nil
	}

	// Get CA certificate for the response
	currentCA := s.ca.GetCurrentCA()
	var caCertPEM []byte
	if currentCA != nil && currentCA.Certificate != nil {
		caCertPEM = []byte(currentCA.CertificatePEM)
	}

	// Revoke the registration token (one-time use)
	s.RevokeToken(req.AgentId)

	s.logger.Info("Agent registered successfully",
		zap.String("agent_id", req.AgentId),
		zap.String("hostname", req.Hostname),
		zap.String("certificate_serial", cert.SerialNumber),
	)

	return &proto.RegisterResponse{
		Success:       true,
		Certificate:   []byte(cert.CertificatePEM + cert.PrivateKeyPEM),
		CaCertificate: caCertPEM,
	}, nil
}

// Connect handles bi-directional streaming between agent and master.
func (s *AgentServer) Connect(stream proto.AgentService_ConnectServer) error {
	ctx := stream.Context()

	// Wait for the AgentReady message to identify the agent
	msg, err := stream.Recv()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return status.Errorf(codes.Internal, "failed to receive initial message: %v", err)
	}

	readyMsg := msg.GetAgentReady()
	if readyMsg == nil {
		return status.Error(codes.InvalidArgument, "first message must be AgentReady")
	}

	agentID := readyMsg.AgentId
	if agentID == "" {
		return status.Error(codes.InvalidArgument, "agent_id is required")
	}

	// Verify agent exists and is registered
	agent, err := s.agentService.GetByID(ctx, agentID)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to lookup agent: %v", err)
	}
	if agent == nil {
		return status.Error(codes.NotFound, "agent not registered")
	}

	// Create cancellable context for this connection
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Register the connection
	conn := &GRPCAgentConnection{
		AgentID:     agentID,
		Hostname:    agent.Hostname,
		Stream:      stream,
		ConnectedAt: time.Now(),
		LastMessage: time.Now(),
		Cancel:      cancel,
		CleanupDone: make(chan struct{}),
	}

	s.connectionMutex.Lock()
	// Close existing connection if any
	if existing, ok := s.connections[agentID]; ok {
		s.logger.Info("Replacing existing agent connection",
			zap.String("agent_id", agentID),
			zap.Time("old_connected_at", existing.ConnectedAt),
		)
		existing.Cancel()
		cleanupChan := existing.CleanupDone
		// Wait for old connection cleanup to complete (with timeout)
		// This prevents command channel conflicts
		s.connectionMutex.Unlock()
		select {
		case <-cleanupChan:
			// Cleanup completed
		case <-time.After(500 * time.Millisecond):
			s.logger.Warn("Timeout waiting for old connection cleanup",
				zap.String("agent_id", agentID))
		}
		s.connectionMutex.Lock()
	}
	s.connections[agentID] = conn

	// Create command channel for this agent
	cmdChan := make(chan *proto.MasterMessage, 100)
	s.pendingCommandMutex.Lock()
	s.pendingCommands[agentID] = cmdChan
	s.pendingCommandMutex.Unlock()
	s.connectionMutex.Unlock()

	defer s.cleanupConnection(agentID)

	// Update agent status
	agent.Status = "online"
	agent.LastSeenAt = time.Now()
	if err := s.agentService.Upsert(ctx, agent); err != nil {
		s.logger.Warn("Failed to update agent status", zap.Error(err))
	}

	s.logger.Info("Agent connected",
		zap.String("agent_id", agentID),
		zap.String("hostname", agent.Hostname),
	)

	// Start goroutine to send commands to agent
	go s.sendCommands(connCtx, agentID, stream, cmdChan)

	// Process incoming messages from agent
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			s.logger.Info("Agent disconnected (EOF)", zap.String("agent_id", agentID))
			return nil
		}
		if err != nil {
			s.logger.Error("Error receiving from agent",
				zap.String("agent_id", agentID),
				zap.Error(err),
			)
			return err
		}

		// Update last message time
		s.connectionMutex.Lock()
		if conn, ok := s.connections[agentID]; ok {
			conn.LastMessage = time.Now()
		}
		s.connectionMutex.Unlock()

		// Process the message
		if err := s.processAgentMessage(ctx, agentID, msg); err != nil {
			s.logger.Error("Error processing agent message",
				zap.String("agent_id", agentID),
				zap.Error(err),
			)
		}
	}
}

// Heartbeat handles periodic health updates from agents.
func (s *AgentServer) Heartbeat(ctx context.Context, req *proto.HeartbeatRequest) (*proto.HeartbeatResponse, error) {
	if req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id is required")
	}

	// Get agent
	agent, err := s.agentService.GetByID(ctx, req.AgentId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to lookup agent: %v", err)
	}
	if agent == nil {
		return nil, status.Error(codes.NotFound, "agent not found")
	}

	// Update agent status and stats
	agent.LastSeenAt = time.Now()
	if req.Stats != nil {
		// Store stats as capabilities JSON for now
		stats := map[string]interface{}{
			"cpu_percent":        req.Stats.CpuPercent,
			"memory_percent":     req.Stats.MemoryPercent,
			"disk_percent":       req.Stats.DiskPercent,
			"disk_free_bytes":    req.Stats.DiskFreeBytes,
			"active_connections": req.Stats.ActiveConnections,
		}
		statsJSON, err := json.Marshal(stats)
		if err != nil {
			s.logger.Warn("Failed to marshal agent stats", zap.Error(err))
		} else {
			agent.Capabilities = string(statsJSON)
		}
	}

	if err := s.agentService.Upsert(ctx, agent); err != nil {
		s.logger.Warn("Failed to update agent", zap.Error(err))
	}

	// Update deployment statuses if provided
	for _, deployStatus := range req.ActiveDeployments {
		if err := s.updateDeploymentStatus(ctx, deployStatus); err != nil {
			s.logger.Warn("Failed to update deployment status",
				zap.String("deployment_id", deployStatus.DeploymentId),
				zap.Error(err),
			)
		}
	}

	return &proto.HeartbeatResponse{
		Ok:              true,
		ServerTimestamp: time.Now().Unix(),
	}, nil
}

// SendCommand sends a command to a connected agent.
func (s *AgentServer) SendCommand(agentID string, msg *proto.MasterMessage) error {
	s.pendingCommandMutex.RLock()
	cmdChan, ok := s.pendingCommands[agentID]
	s.pendingCommandMutex.RUnlock()

	if !ok {
		return fmt.Errorf("agent %s is not connected", agentID)
	}

	select {
	case cmdChan <- msg:
		return nil
	default:
		return fmt.Errorf("command queue full for agent %s", agentID)
	}
}

// SendDeployCommand sends a deployment command to an agent.
func (s *AgentServer) SendDeployCommand(agentID string, cmd *proto.DeployCommand) error {
	return s.SendCommand(agentID, &proto.MasterMessage{
		Message: &proto.MasterMessage_DeployCommand{
			DeployCommand: cmd,
		},
	})
}

// SendRollbackCommand sends a rollback command to an agent.
func (s *AgentServer) SendRollbackCommand(agentID string, cmd *proto.RollbackCommand) error {
	return s.SendCommand(agentID, &proto.MasterMessage{
		Message: &proto.MasterMessage_RollbackCommand{
			RollbackCommand: cmd,
		},
	})
}

// SendCancelCommand sends a cancel command to an agent.
func (s *AgentServer) SendCancelCommand(agentID string, cmd *proto.CancelCommand) error {
	return s.SendCommand(agentID, &proto.MasterMessage{
		Message: &proto.MasterMessage_CancelCommand{
			CancelCommand: cmd,
		},
	})
}

// IsAgentConnected checks if an agent is currently connected.
func (s *AgentServer) IsAgentConnected(agentID string) bool {
	s.connectionMutex.RLock()
	defer s.connectionMutex.RUnlock()
	_, ok := s.connections[agentID]
	return ok
}

// GetConnectedAgents returns a list of connected agent IDs.
func (s *AgentServer) GetConnectedAgents() []string {
	s.connectionMutex.RLock()
	defer s.connectionMutex.RUnlock()

	agents := make([]string, 0, len(s.connections))
	for id := range s.connections {
		agents = append(agents, id)
	}
	return agents
}

// DisconnectAgent forcefully disconnects an agent.
func (s *AgentServer) DisconnectAgent(agentID string) {
	s.connectionMutex.Lock()
	if conn, ok := s.connections[agentID]; ok {
		conn.Cancel()
	}
	s.connectionMutex.Unlock()
}

// validateToken checks if a registration token is valid for an agent.
func (s *AgentServer) validateToken(agentID, token string) bool {
	s.tokenMutex.RLock()
	defer s.tokenMutex.RUnlock()

	expectedToken, ok := s.tokens[agentID]
	if !ok {
		return false
	}

	// Constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare([]byte(expectedToken), []byte(token)) == 1
}

// sendCommands sends pending commands to an agent over the stream.
func (s *AgentServer) sendCommands(ctx context.Context, agentID string, stream proto.AgentService_ConnectServer, cmdChan chan *proto.MasterMessage) {
	for {
		select {
		case <-ctx.Done():
			return
		case cmd := <-cmdChan:
			if err := stream.Send(cmd); err != nil {
				s.logger.Error("Failed to send command to agent",
					zap.String("agent_id", agentID),
					zap.Error(err),
				)
				return
			}
		}
	}
}

// cleanupConnection removes an agent connection and updates status.
func (s *AgentServer) cleanupConnection(agentID string) {
	s.connectionMutex.Lock()
	conn := s.connections[agentID]
	delete(s.connections, agentID)
	s.connectionMutex.Unlock()

	// Signal that cleanup is complete (for connection replacement)
	if conn != nil && conn.CleanupDone != nil {
		close(conn.CleanupDone)
	}

	s.pendingCommandMutex.Lock()
	if ch, ok := s.pendingCommands[agentID]; ok {
		close(ch)
		delete(s.pendingCommands, agentID)
	}
	s.pendingCommandMutex.Unlock()

	// Update agent status to offline
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	agent, err := s.agentService.GetByID(ctx, agentID)
	if err == nil && agent != nil {
		agent.Status = "offline"
		if err := s.agentService.Upsert(ctx, agent); err != nil {
			s.logger.Warn("Failed to update agent status to offline",
				zap.String("agent_id", agentID),
				zap.Error(err))
		}
	}

	s.logger.Info("Agent connection cleaned up", zap.String("agent_id", agentID))
}

// processAgentMessage processes messages received from an agent.
func (s *AgentServer) processAgentMessage(ctx context.Context, agentID string, msg *proto.AgentMessage) error {
	switch m := msg.Message.(type) {
	case *proto.AgentMessage_DeploymentStatus:
		return s.handleDeploymentStatus(ctx, agentID, m.DeploymentStatus)
	case *proto.AgentMessage_DeploymentLog:
		return s.handleDeploymentLog(ctx, agentID, m.DeploymentLog)
	case *proto.AgentMessage_CommandResult:
		return s.handleCommandResult(ctx, agentID, m.CommandResult)
	case *proto.AgentMessage_AgentReady:
		// Already handled during connection setup
		return nil
	default:
		s.logger.Warn("Unknown message type from agent",
			zap.String("agent_id", agentID),
		)
		return nil
	}
}

// handleDeploymentStatus processes deployment status updates.
func (s *AgentServer) handleDeploymentStatus(ctx context.Context, agentID string, status *proto.DeploymentStatus) error {
	if status == nil {
		return nil
	}

	s.logger.Debug("Deployment status update",
		zap.String("agent_id", agentID),
		zap.String("deployment_id", status.DeploymentId),
		zap.String("state", status.State.String()),
		zap.Int32("progress", status.ProgressPercent),
	)

	return s.updateDeploymentStatus(ctx, status)
}

// handleDeploymentLog processes deployment log messages.
func (s *AgentServer) handleDeploymentLog(ctx context.Context, agentID string, log *proto.DeploymentLog) error {
	if log == nil {
		return nil
	}

	// Log to our logger
	logLevel := zap.InfoLevel
	switch log.Level {
	case proto.LogLevel_LOG_LEVEL_DEBUG:
		logLevel = zap.DebugLevel
	case proto.LogLevel_LOG_LEVEL_WARN:
		logLevel = zap.WarnLevel
	case proto.LogLevel_LOG_LEVEL_ERROR:
		logLevel = zap.ErrorLevel
	}

	if ce := s.logger.Check(logLevel, "Deployment log"); ce != nil {
		ce.Write(
			zap.String("agent_id", agentID),
			zap.String("deployment_id", log.DeploymentId),
			zap.String("source", log.Source),
			zap.String("message", log.Message),
		)
	}

	// Store in deployment logs
	return s.deploymentService.CreateLog(ctx, &storage.DeploymentLog{
		DeploymentID: log.DeploymentId,
		Level:        log.Level.String(),
		Message:      log.Message,
		Source:       log.Source,
		CreatedAt:    time.Unix(log.Timestamp, 0),
	})
}

// handleCommandResult processes command execution results.
func (s *AgentServer) handleCommandResult(ctx context.Context, agentID string, result *proto.CommandResult) error {
	if result == nil {
		return nil
	}

	s.logger.Debug("Command result",
		zap.String("agent_id", agentID),
		zap.String("deployment_id", result.DeploymentId),
		zap.String("command", result.Command),
		zap.Int32("exit_code", result.ExitCode),
		zap.Int64("duration_ms", result.DurationMs),
	)

	// Create a log entry for the command result
	level := "INFO"
	if result.ExitCode != 0 {
		level = "ERROR"
	}

	message := fmt.Sprintf("Command '%s' completed with exit code %d (took %dms)",
		result.Command, result.ExitCode, result.DurationMs)

	if result.Stdout != "" {
		message += fmt.Sprintf("\nStdout: %s", result.Stdout)
	}
	if result.Stderr != "" {
		message += fmt.Sprintf("\nStderr: %s", result.Stderr)
	}

	return s.deploymentService.CreateLog(ctx, &storage.DeploymentLog{
		DeploymentID: result.DeploymentId,
		Level:        level,
		Message:      message,
		Source:       "command",
		CreatedAt:    time.Now(),
	})
}

// updateDeploymentStatus updates a deployment's status in the database.
func (s *AgentServer) updateDeploymentStatus(ctx context.Context, status *proto.DeploymentStatus) error {
	if status == nil || status.DeploymentId == "" {
		return nil
	}

	deployment, err := s.deploymentService.GetByID(ctx, status.DeploymentId)
	if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	}
	if deployment == nil {
		s.logger.Warn("Deployment not found for status update",
			zap.String("deployment_id", status.DeploymentId),
		)
		return nil
	}

	// Map proto state to storage status
	var dbStatus string
	switch status.State {
	case proto.DeploymentState_DEPLOYMENT_STATE_PENDING:
		dbStatus = "pending"
	case proto.DeploymentState_DEPLOYMENT_STATE_PREPARING,
		proto.DeploymentState_DEPLOYMENT_STATE_CLONING,
		proto.DeploymentState_DEPLOYMENT_STATE_BUILDING,
		proto.DeploymentState_DEPLOYMENT_STATE_DEPLOYING,
		proto.DeploymentState_DEPLOYMENT_STATE_VERIFYING:
		dbStatus = "running"
	case proto.DeploymentState_DEPLOYMENT_STATE_COMPLETED:
		dbStatus = "success"
	case proto.DeploymentState_DEPLOYMENT_STATE_FAILED:
		dbStatus = "failed"
	case proto.DeploymentState_DEPLOYMENT_STATE_CANCELLED:
		dbStatus = "cancelled"
	case proto.DeploymentState_DEPLOYMENT_STATE_ROLLING_BACK:
		dbStatus = "rolling_back"
	default:
		dbStatus = "unknown"
	}

	deployment.Status = dbStatus
	if status.State == proto.DeploymentState_DEPLOYMENT_STATE_COMPLETED ||
		status.State == proto.DeploymentState_DEPLOYMENT_STATE_FAILED ||
		status.State == proto.DeploymentState_DEPLOYMENT_STATE_CANCELLED {
		now := time.Now()
		deployment.CompletedAt = &now
	}

	return s.deploymentService.Update(ctx, deployment)
}
