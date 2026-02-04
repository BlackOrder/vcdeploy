// Package server provides the master daemon HTTP and gRPC servers.
package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/alerting"
	"github.com/BlackOrder/vcdeploy/internal/git"
	"github.com/BlackOrder/vcdeploy/internal/metrics"
	"github.com/BlackOrder/vcdeploy/internal/proto"
	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// AgentServer implements the AgentServiceServer interface for agent-master communication.
type AgentServer struct {
	proto.UnimplementedAgentServiceServer

	store  storage.Store
	ca     *security.CAManager
	logger *zap.Logger

	// Service layer
	agentService      services.AgentServicer
	deploymentService services.DeploymentServicer

	// Git service for repository operations
	gitService *git.Service

	// Alert manager for system alerts
	alertManager *alerting.Manager

	// allowAutoRegister enables agents to register without a pre-generated token (for testing)
	allowAutoRegister bool

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
func NewAgentServer(store storage.Store, ca *security.CAManager, logger *zap.Logger) *AgentServer {
	return &AgentServer{
		store:           store,
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

// SetAlertManager sets the alert manager for system alerts.
func (s *AgentServer) SetAlertManager(alertMgr *alerting.Manager) {
	s.alertManager = alertMgr
}

// SetGitService sets the git service for repository operations.
func (s *AgentServer) SetGitService(gitSvc *git.Service) {
	s.gitService = gitSvc
}

// SetAllowAutoRegister enables or disables auto-registration without pre-generated tokens.
// This should only be enabled for testing environments.
func (s *AgentServer) SetAllowAutoRegister(allow bool) {
	s.allowAutoRegister = allow
	if allow {
		s.logger.Warn("Auto-registration enabled - agents can register without pre-generated tokens")
	}
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
	if req.AgentId == "" {
		return &proto.RegisterResponse{
			Success: false,
			Error:   "agent_id is required",
		}, nil
	}

	// Validate agent ID format
	if err := security.ValidateAgentID(req.AgentId); err != nil {
		s.logger.Warn("Invalid agent ID format",
			zap.String("agent_id", req.AgentId),
			zap.Error(err),
		)
		return &proto.RegisterResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	// Extract client IP for audit logging
	clientIP := ExtractGRPCClientIP(ctx)

	// In auto-register mode, allow agents to register without a pre-generated token
	// This is only for testing environments
	tokenValid := false
	if s.allowAutoRegister && req.Token != "" {
		// In auto-register mode, accept any non-empty token
		s.logger.Info("Auto-registration mode: accepting agent registration",
			zap.String("agent_id", req.AgentId),
			zap.String("hostname", req.Hostname),
			zap.String("client_ip", clientIP),
		)
		tokenValid = true
	} else if req.Token != "" {
		// Normal mode: validate the pre-generated token
		// Try database token first, fall back to in-memory
		tokenValid = s.validateDatabaseToken(ctx, req.AgentId, req.Token)
		if !tokenValid {
			tokenValid = s.validateToken(req.AgentId, req.Token)
		}
	}

	if !tokenValid {
		s.logger.Warn("Invalid registration token",
			zap.String("agent_id", req.AgentId),
			zap.String("hostname", req.Hostname),
			zap.String("client_ip", clientIP),
		)
		return &proto.RegisterResponse{
			Success: false,
			Error:   "invalid registration token",
		}, nil
	}

	s.logger.Info("Agent registration request",
		zap.String("agent_id", req.AgentId),
		zap.String("hostname", req.Hostname),
		zap.String("client_ip", clientIP),
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

	// Generate HMAC secret for re-authentication
	hmacSecret := make([]byte, 32)
	if _, err := rand.Read(hmacSecret); err != nil {
		s.logger.Error("Failed to generate HMAC secret",
			zap.String("agent_id", req.AgentId),
			zap.Error(err),
		)
		return &proto.RegisterResponse{
			Success: false,
			Error:   "failed to generate HMAC secret",
		}, nil
	}

	agent := &storage.Agent{
		ID:           req.AgentId,
		Hostname:     req.Hostname,
		Labels:       req.Labels,
		Capabilities: string(capabilities),
		Status:       "registered",
		LastSeenAt:   time.Now(),
		Certificate:  cert.SerialNumber,
		HMACSecret:   hmacSecret,
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
		HmacSecret:    hmacSecret,
	}, nil
}

// Connect handles bi-directional streaming between agent and master.
func (s *AgentServer) Connect(stream proto.AgentService_ConnectServer) error {
	ctx := stream.Context()

	// Wait for the AgentReady message to identify the agent
	msg, err := stream.Recv()
	if errors.Is(err, io.EOF) {
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

	// Verify agent certificate matches claimed ID (mTLS verification)
	if err := s.verifyAgentCert(ctx, agentID); err != nil {
		return err
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

	// Update metrics
	metrics.AgentConnected.WithLabelValues(agentID, agent.Hostname).Set(1)

	// Send reconnection alert if alerting is enabled
	if s.alertManager != nil {
		s.alertManager.CheckAgentReconnect(ctx, agent)
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
		if errors.Is(err, io.EOF) {
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

	// Verify agent certificate matches claimed ID (mTLS verification)
	if err := s.verifyAgentCert(ctx, req.AgentId); err != nil {
		return nil, err
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
		// Update Prometheus metrics
		metrics.AgentCPUPercent.WithLabelValues(req.AgentId).Set(req.Stats.CpuPercent)
		metrics.AgentMemoryPercent.WithLabelValues(req.AgentId).Set(req.Stats.MemoryPercent)
		metrics.AgentDiskPercent.WithLabelValues(req.AgentId).Set(req.Stats.DiskPercent)

		// Check alert thresholds
		if s.alertManager != nil {
			s.alertManager.CheckAgentMetrics(ctx, req.AgentId, agent.Hostname,
				req.Stats.CpuPercent, req.Stats.MemoryPercent, req.Stats.DiskPercent)
		}

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

// GetCATrustBundle returns the current CA trust bundle for certificate validation.
func (s *AgentServer) GetCATrustBundle(ctx context.Context, req *proto.GetCATrustBundleRequest) (*proto.GetCATrustBundleResponse, error) {
	// Verify agent from mTLS cert
	if err := s.verifyAgentCert(ctx, ""); err != nil {
		// Allow unauthenticated requests - agents need this before they have valid certs
		s.logger.Debug("GetCATrustBundle called without valid cert, allowing for CA sync")
	}

	// Get current trust pool version
	version := s.ca.GetTrustPoolVersion(ctx)

	// If client already has current version, return minimal response
	if req.CurrentVersion == version && req.CurrentVersion != "" {
		return &proto.GetCATrustBundleResponse{
			Version: version,
		}, nil
	}

	// Get all CA certificates
	certs, err := s.ca.GetAllCACertificates(ctx)
	if err != nil {
		return nil, fmt.Errorf("get CA certificates: %w", err)
	}

	// Get current CA ID
	currentCA := s.ca.GetCurrentCA()
	var currentCAID string
	if currentCA != nil {
		currentCAID = currentCA.ID
	}

	s.logger.Debug("Returning CA trust bundle",
		zap.String("version", version),
		zap.Int("ca_count", len(certs)),
		zap.String("current_ca", currentCAID))

	return &proto.GetCATrustBundleResponse{
		CaCertificates: certs,
		CurrentCaId:    currentCAID,
		Version:        version,
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

// SendUpdateCommand sends an update command to an agent.
func (s *AgentServer) SendUpdateCommand(agentID string, cmd *proto.UpdateCommand) error {
	return s.SendCommand(agentID, &proto.MasterMessage{
		Message: &proto.MasterMessage_UpdateCommand{
			UpdateCommand: cmd,
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

// validateToken checks if a registration token is valid for an agent (in-memory).
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

// validateDatabaseToken checks if a registration token is valid using the database.
// It also marks the token as used if valid.
func (s *AgentServer) validateDatabaseToken(ctx context.Context, agentID, token string) bool {
	// Get token from database
	dbToken, err := s.store.GetRegistrationToken(ctx, token)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			s.logger.Error("Failed to get registration token from database",
				zap.String("agent_id", agentID),
				zap.Error(err),
			)
		}
		return false
	}

	// Check if token is expired
	if time.Now().After(dbToken.ExpiresAt) {
		s.logger.Warn("Registration token expired",
			zap.String("agent_id", agentID),
			zap.Time("expired_at", dbToken.ExpiresAt),
		)
		return false
	}

	// Check if token was already used
	if dbToken.UsedAt != nil {
		s.logger.Warn("Registration token already used",
			zap.String("agent_id", agentID),
			zap.Time("used_at", *dbToken.UsedAt),
		)
		return false
	}

	// Check if token is for a specific agent
	if dbToken.AgentID != "" && dbToken.AgentID != agentID {
		s.logger.Warn("Registration token not valid for this agent",
			zap.String("agent_id", agentID),
			zap.String("token_agent_id", dbToken.AgentID),
		)
		return false
	}

	// Mark token as used
	if err := s.store.MarkTokenUsed(ctx, token); err != nil {
		s.logger.Error("Failed to mark token as used",
			zap.String("agent_id", agentID),
			zap.Error(err),
		)
		// Continue anyway - the token was valid
	}

	return true
}

// extractAgentIDFromCert extracts the agent ID from the client certificate in mTLS.
// Returns empty string if no client certificate is present (e.g., during registration or tests).
func extractAgentIDFromCert(ctx context.Context) (string, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		// No peer info - this happens in tests without TLS
		return "", nil
	}

	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		// Not using TLS - this is okay during registration
		return "", nil
	}

	if len(tlsInfo.State.PeerCertificates) == 0 {
		// No client certificate - this is okay during registration
		return "", nil
	}

	cert := tlsInfo.State.PeerCertificates[0]
	// Agent ID is in the Common Name
	return cert.Subject.CommonName, nil
}

// verifyAgentCert verifies the client certificate matches the claimed agent ID.
// Used for authenticated RPCs like Heartbeat and Connect.
func (s *AgentServer) verifyAgentCert(ctx context.Context, claimedAgentID string) error {
	certAgentID, err := extractAgentIDFromCert(ctx)
	if err != nil {
		return err
	}

	// If we got an agent ID from the cert, verify it matches
	if certAgentID != "" && certAgentID != claimedAgentID {
		s.logger.Warn("Agent ID mismatch between certificate and request",
			zap.String("cert_agent_id", certAgentID),
			zap.String("claimed_agent_id", claimedAgentID),
		)
		return status.Errorf(codes.PermissionDenied, "agent ID mismatch: certificate says %s, request says %s", certAgentID, claimedAgentID)
	}

	return nil
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

		// Update metrics
		metrics.AgentConnected.WithLabelValues(agentID, agent.Hostname).Set(0)

		// Send disconnect alert if alerting is enabled
		if s.alertManager != nil {
			s.alertManager.CheckAgentDisconnect(ctx, agent)
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

	deployment.Status = storage.DeploymentStatus(dbStatus)
	if status.State == proto.DeploymentState_DEPLOYMENT_STATE_COMPLETED ||
		status.State == proto.DeploymentState_DEPLOYMENT_STATE_FAILED ||
		status.State == proto.DeploymentState_DEPLOYMENT_STATE_CANCELLED {
		now := time.Now()
		deployment.CompletedAt = &now
	}

	return s.deploymentService.Update(ctx, deployment)
}

// StreamRepoArchive streams a repository archive to an agent.
// The master clones the repository using stored credentials and streams the archive
// to the agent. This ensures credentials never leave the master.
func (s *AgentServer) StreamRepoArchive(req *proto.StreamRepoRequest, stream proto.AgentService_StreamRepoArchiveServer) error {
	ctx := stream.Context()

	// Verify agent from certificate
	agentID, err := extractAgentIDFromCert(ctx)
	if err != nil {
		return err
	}

	if agentID == "" {
		return status.Error(codes.Unauthenticated, "no client certificate")
	}

	s.logger.Info("StreamRepoArchive request",
		zap.String("agent_id", agentID),
		zap.String("deployment_id", req.DeploymentId),
		zap.String("ref", req.Ref),
	)

	// Verify agent is assigned to this deployment (multi-agent deployment support)
	if req.DeploymentId != "" {
		deployment, err := s.deploymentService.GetByID(ctx, req.DeploymentId)
		if err != nil {
			return status.Errorf(codes.NotFound, "deployment not found: %v", err)
		}
		if deployment == nil {
			return status.Error(codes.NotFound, "deployment not found")
		}

		// Check if this is a multi-agent deployment (has assignments in deployment_agents)
		agents, err := s.store.GetDeploymentAgents(ctx, req.DeploymentId)
		if err != nil {
			return status.Errorf(codes.Internal, "failed to check deployment agents: %v", err)
		}

		// If deployment_agents has entries, verify this agent is assigned
		if len(agents) > 0 {
			assigned, err := s.store.IsAgentAssignedToDeployment(ctx, req.DeploymentId, agentID)
			if err != nil {
				return status.Errorf(codes.Internal, "failed to verify agent assignment: %v", err)
			}
			if !assigned {
				return status.Errorf(codes.PermissionDenied, "agent %s not assigned to deployment %s", agentID, req.DeploymentId)
			}
		}
		// For single-agent deployments (no entries in deployment_agents), allow any agent
		// This maintains backward compatibility with existing single-agent deployments
	}

	// Check if git service is configured
	if s.gitService == nil {
		return status.Error(codes.Unavailable, "git service not configured")
	}

	// Clone and create archive
	archive, err := s.gitService.CloneAndArchive(ctx, req.RepoUrl, req.Ref)
	if err != nil {
		s.logger.Error("Failed to clone and archive repository",
			zap.String("agent_id", agentID),
			zap.String("repo_url", req.RepoUrl),
			zap.Error(err),
		)
		return status.Errorf(codes.Internal, "failed to clone repository: %v", err)
	}
	defer os.Remove(archive.Path) // Clean up archive after streaming

	// Open archive file for streaming
	file, err := os.Open(archive.Path)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to open archive: %v", err)
	}
	defer file.Close()

	// Stream archive to agent in 64KB chunks
	const chunkSize = 64 * 1024
	buf := make([]byte, chunkSize)
	var bytesRead int64

	for {
		n, err := file.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			return status.Errorf(codes.Internal, "failed to read archive: %v", err)
		}

		bytesRead += int64(n)
		isLast := bytesRead >= archive.Size

		chunk := &proto.RepoChunk{
			Data:      buf[:n],
			TotalSize: archive.Size,
			IsLast:    isLast,
		}

		// Include checksum only in last chunk
		if isLast {
			chunk.Checksum = archive.Checksum
		}

		if err := stream.Send(chunk); err != nil {
			s.logger.Error("Failed to send chunk",
				zap.String("agent_id", agentID),
				zap.Int64("bytes_sent", bytesRead),
				zap.Error(err),
			)
			return err
		}
	}

	s.logger.Info("StreamRepoArchive completed",
		zap.String("agent_id", agentID),
		zap.Int64("bytes_sent", bytesRead),
		zap.String("checksum", archive.Checksum[:16]+"..."),
	)

	return nil
}

// Reauthenticate handles re-authentication requests.
// On the main mTLS port, this returns an error directing agents to use the dedicated re-auth port.
// The actual re-authentication is handled by ReauthOnlyServer on port 9444.
func (s *AgentServer) Reauthenticate(ctx context.Context, req *proto.ReauthRequest) (*proto.ReauthResponse, error) {
	// Re-authentication should use the dedicated port (9444) which doesn't require client certs
	return nil, status.Error(codes.FailedPrecondition, "use the dedicated re-auth port (:9444) for re-authentication")
}
