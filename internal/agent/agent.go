// Package agent provides the agent daemon implementation.
package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/deploy"
	pb "github.com/BlackOrder/vcdeploy/internal/proto"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Version information (set at build time via ldflags)
var (
	AgentVersion   = "dev"
	AgentCommit    = "unknown"
	AgentBuildTime = "unknown"
)

// SetVersionInfo sets the version information for the agent.
func SetVersionInfo(version, commit, buildTime string) {
	AgentVersion = version
	AgentCommit = commit
	AgentBuildTime = buildTime
}

// Agent is the deployment agent daemon.
type Agent struct {
	config   *config.AgentConfig
	logger   *zap.Logger
	strategy *deploy.SymlinkStrategy
	runner   *LocalRunner
	store    *AgentStore // Encrypted local storage

	// gRPC connection to master
	conn   *grpc.ClientConn
	client pb.AgentServiceClient
	stream pb.AgentService_ConnectClient
	mu     sync.RWMutex

	// HTTP client for health checks (reused across requests)
	httpClient *http.Client

	// Active deployments tracking
	activeDeployments map[string]*activeDeployment
	deployMu          sync.RWMutex

	// Self-update state
	updateInProgress bool
	updateMu         sync.RWMutex

	// State
	running  bool
	shutdown chan struct{}
	wg       sync.WaitGroup
}

// activeDeployment tracks an in-progress deployment.
type activeDeployment struct {
	ID        string
	Project   string
	StartTime time.Time
	State     pb.DeploymentState
	Cancel    context.CancelFunc
	// cancelDone is closed when the deployment has acknowledged cancellation
	cancelDone chan struct{}
}

// NewAgent creates a new agent instance.
func NewAgent(cfg *config.AgentConfig, logger *zap.Logger) (*Agent, error) {
	// Create a local command runner
	runner := NewLocalRunner(logger)

	// Create the deployment strategy
	strategy := deploy.NewSymlinkStrategy(runner)

	// Create the local encrypted store
	store, err := NewAgentStore(cfg.Paths.Data)
	if err != nil {
		return nil, fmt.Errorf("create agent store: %w", err)
	}

	// Initialize schema
	if err := store.InitSchema(context.Background()); err != nil {
		store.Close()
		return nil, fmt.Errorf("initialize store schema: %w", err)
	}

	// Create HTTP client for health checks (reused for connection pooling)
	httpClient := &http.Client{
		Timeout: 30 * time.Second, // Default timeout, can be overridden per-request
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Allow up to 10 redirects
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 5,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	return &Agent{
		config:            cfg,
		logger:            logger,
		strategy:          strategy,
		runner:            runner,
		store:             store,
		httpClient:        httpClient,
		activeDeployments: make(map[string]*activeDeployment),
		shutdown:          make(chan struct{}),
	}, nil
}

// Start starts the agent daemon.
func (a *Agent) Start(ctx context.Context) error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return fmt.Errorf("agent already running")
	}
	a.running = true
	a.mu.Unlock()

	a.logger.Info("Starting agent",
		zap.String("id", a.config.Agent.ID),
		zap.String("master", a.config.Master.Address),
	)

	// Connect to master
	if err := a.connect(ctx); err != nil {
		return fmt.Errorf("connecting to master: %w", err)
	}

	// Auto-register if token is provided and we haven't registered yet
	if a.config.Master.Token != "" {
		a.logger.Info("Registration token provided, attempting auto-registration")

		// Retry registration a few times in case of transient errors (e.g., database lock)
		var registrationErr error
		for attempt := 1; attempt <= 5; attempt++ {
			if _, _, registrationErr = a.Register(ctx, a.config.Master.Token); registrationErr == nil {
				a.logger.Info("Successfully registered with master")
				break
			}
			a.logger.Warn("Registration attempt failed, retrying...",
				zap.Int("attempt", attempt),
				zap.Error(registrationErr))
			// Wait before retrying (exponential backoff)
			time.Sleep(time.Duration(attempt*500) * time.Millisecond)
		}
		if registrationErr != nil {
			a.logger.Warn("Auto-registration failed after retries (agent may already be registered)",
				zap.Error(registrationErr))
		}
	}

	// Start heartbeat loop
	a.wg.Go(func() {
		a.heartbeatLoop(ctx)
	})

	// Start command listener
	a.wg.Go(func() {
		a.commandLoop(ctx)
	})

	// Wait for shutdown signal
	select {
	case <-ctx.Done():
		return a.Shutdown(context.Background())
	case <-a.shutdown:
		return nil
	}
}

func (a *Agent) connect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.conn != nil {
		return nil
	}

	var opts []grpc.DialOption

	// Try to use mTLS if we have stored certificates
	tlsConfig, err := a.buildTLSConfig(ctx)
	if err != nil {
		a.logger.Debug("Could not build mTLS config, falling back", zap.Error(err))
	}

	if tlsConfig != nil {
		// Use mTLS with stored certificates
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
		a.logger.Info("Using mTLS with stored certificates")
	} else if a.config.Master.Cert != "" {
		// Use TLS with CA cert (server-only auth for registration)
		creds, err := credentials.NewClientTLSFromFile(a.config.Master.Cert, "")
		if err != nil {
			return fmt.Errorf("loading CA cert: %w", err)
		}
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else if a.config.Master.AllowInsecure {
		// Explicit insecure mode - warn user
		a.logger.Warn("Using insecure connection to master - TLS is disabled. This is NOT recommended for production!")
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		return fmt.Errorf("TLS certificate required: set master.cert or use master.allow_insecure=true (not recommended)")
	}

	// Use the modern grpc.NewClient API
	conn, err := grpc.NewClient(a.config.Master.Address, opts...)
	if err != nil {
		return fmt.Errorf("creating grpc client: %w", err)
	}

	// NewClient is lazy - verify connection with a timeout
	connectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	conn.Connect()
	if !conn.WaitForStateChange(connectCtx, connectivity.Idle) {
		conn.Close()
		return fmt.Errorf("connection timeout to master")
	}

	a.conn = conn
	a.client = pb.NewAgentServiceClient(conn)
	a.logger.Info("Connected to master", zap.String("address", a.config.Master.Address))

	return nil
}

// buildTLSConfig creates a TLS config using stored certificates for mTLS.
func (a *Agent) buildTLSConfig(ctx context.Context) (*tls.Config, error) {
	// Load agent certificate (our identity)
	agentCert, err := a.store.GetTLSCertificate(ctx, "agent")
	if err != nil {
		return nil, fmt.Errorf("load agent certificate: %w", err)
	}

	// Load CA certificate
	caCertRecord, err := a.store.GetCertificate(ctx, "ca")
	if err != nil {
		return nil, fmt.Errorf("load CA certificate: %w", err)
	}

	// Parse CA certificate for verification
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCertRecord.Certificate) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{*agentCert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// Register registers the agent with the master server using a token.
// Returns the mTLS certificate and CA certificate on success.
// Certificates are automatically stored for future mTLS connections.
func (a *Agent) Register(ctx context.Context, token string) (cert []byte, caCert []byte, err error) {
	// Connect to master
	if err := a.connect(ctx); err != nil {
		return nil, nil, fmt.Errorf("connecting to master: %w", err)
	}

	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil {
		return nil, nil, fmt.Errorf("not connected")
	}

	hostname, _ := os.Hostname()

	// Build capabilities
	caps := &pb.AgentCapabilities{
		CanUseNamespaces: a.config.Execution.UseNamespaces,
		AllowedUsers:     []string{a.config.Execution.User},
	}

	// Get disk info
	if diskStat, err := disk.Usage(a.config.Paths.Releases); err == nil {
		caps.DiskSpaceBytes = int64(diskStat.Total) // #nosec G115 - uint64->int64 safe: overflow impossible (>9EB)
	}

	// Get memory info
	if vmStat, err := mem.VirtualMemory(); err == nil {
		caps.MemoryBytes = int64(vmStat.Total) // #nosec G115 - uint64->int64 safe: overflow impossible (>9EB)
	}

	req := &pb.RegisterRequest{
		AgentId:      a.config.Agent.ID,
		Token:        token,
		Hostname:     hostname,
		Labels:       a.config.Agent.Labels,
		Capabilities: caps,
	}

	a.logger.Info("Registering with master",
		zap.String("agent_id", a.config.Agent.ID),
		zap.String("hostname", hostname),
	)

	resp, err := client.Register(ctx, req)
	if err != nil {
		// Log audit event for failed registration
		a.store.LogAuditEvent(ctx, AuditEventConnect, fmt.Sprintf("registration failed: %v", err), false)
		return nil, nil, fmt.Errorf("registration RPC failed: %w", err)
	}

	if !resp.Success {
		a.store.LogAuditEvent(ctx, AuditEventConnect, fmt.Sprintf("registration rejected: %s", resp.Error), false)
		return nil, nil, fmt.Errorf("registration rejected: %s", resp.Error)
	}

	// Store the certificates
	if len(resp.Certificate) > 0 {
		// Parse certificate to get expiry
		notAfter := time.Now().AddDate(1, 0, 0) // Default 1 year
		if block, _ := pem.Decode(resp.Certificate); block != nil {
			if parsed, err := x509.ParseCertificate(block.Bytes); err == nil {
				notAfter = parsed.NotAfter
			}
		}

		// Extract private key if present (agent cert includes key)
		certPEM, keyPEM := splitCertAndKey(resp.Certificate)

		if err := a.store.SaveCertificate(ctx, "agent", certPEM, keyPEM, notAfter); err != nil {
			a.logger.Warn("Failed to save agent certificate", zap.Error(err))
		} else {
			a.logger.Info("Stored agent certificate", zap.Time("expires", notAfter))
		}
	}

	if len(resp.CaCertificate) > 0 {
		// CA cert has no private key
		notAfter := time.Now().AddDate(10, 0, 0) // Default 10 years for CA
		if block, _ := pem.Decode(resp.CaCertificate); block != nil {
			if parsed, err := x509.ParseCertificate(block.Bytes); err == nil {
				notAfter = parsed.NotAfter
			}
		}

		if err := a.store.SaveCertificate(ctx, "ca", resp.CaCertificate, nil, notAfter); err != nil {
			a.logger.Warn("Failed to save CA certificate", zap.Error(err))
		} else {
			a.logger.Info("Stored CA certificate", zap.Time("expires", notAfter))
		}
	}

	// Log successful registration
	a.store.LogAuditEvent(ctx, AuditEventCertIssued, "agent registered successfully", true)

	a.logger.Info("Registration successful")

	return resp.Certificate, resp.CaCertificate, nil
}

func (a *Agent) heartbeatLoop(ctx context.Context) {
	interval := a.config.Master.Reconnect.HeartbeatInterval
	if interval == 0 {
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := a.sendHeartbeat(ctx); err != nil {
				a.logger.Error("Heartbeat failed", zap.Error(err))
				// Try to reconnect
				if err := a.reconnect(ctx); err != nil {
					a.logger.Error("Reconnect failed", zap.Error(err))
				}
			}
		case <-a.shutdown:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (a *Agent) sendHeartbeat(ctx context.Context) error {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("not connected")
	}

	// Gather system stats
	stats := a.collectStats()

	// Gather active deployments status
	activeStatuses := a.getActiveDeploymentStatuses()

	req := &pb.HeartbeatRequest{
		AgentId:           a.config.Agent.ID,
		Timestamp:         time.Now().Unix(),
		Stats:             stats,
		ActiveDeployments: activeStatuses,
		Version:           AgentVersion,
		Os:                runtime.GOOS,
		Arch:              runtime.GOARCH,
	}

	hostname, _ := os.Hostname()
	a.logger.Debug("Sending heartbeat",
		zap.String("hostname", hostname),
		zap.Int("active_deployments", len(activeStatuses)),
		zap.String("version", AgentVersion),
	)

	resp, err := client.Heartbeat(ctx, req)
	if err != nil {
		return fmt.Errorf("sending heartbeat: %w", err)
	}

	if !resp.Ok {
		return fmt.Errorf("heartbeat rejected by master")
	}

	// Check for update notification
	if resp.UpdateAvailable != nil {
		a.handleUpdateNotification(resp.UpdateAvailable)
	}

	return nil
}

// collectStats gathers system resource statistics.
func (a *Agent) collectStats() *pb.AgentStats {
	stats := &pb.AgentStats{}

	// CPU usage
	if cpuPercent, err := cpu.Percent(0, false); err == nil && len(cpuPercent) > 0 {
		stats.CpuPercent = cpuPercent[0]
	}

	// Memory usage
	if vmStat, err := mem.VirtualMemory(); err == nil {
		stats.MemoryPercent = vmStat.UsedPercent
	}

	// Disk usage (check the releases path)
	if diskStat, err := disk.Usage(a.config.Paths.Releases); err == nil {
		stats.DiskPercent = diskStat.UsedPercent
		stats.DiskFreeBytes = int64(diskStat.Free) // #nosec G115 - uint64->int64 safe: overflow impossible (>9EB)
	}

	return stats
}

// getActiveDeploymentStatuses returns status of all active deployments.
func (a *Agent) getActiveDeploymentStatuses() []*pb.DeploymentStatus {
	a.deployMu.RLock()
	defer a.deployMu.RUnlock()

	statuses := make([]*pb.DeploymentStatus, 0, len(a.activeDeployments))
	for _, ad := range a.activeDeployments {
		statuses = append(statuses, &pb.DeploymentStatus{
			DeploymentId: ad.ID,
			State:        ad.State,
			Timestamp:    time.Now().Unix(),
		})
	}
	return statuses
}

func (a *Agent) reconnect(ctx context.Context) error {
	a.mu.Lock()
	if a.conn != nil {
		a.conn.Close()
		a.conn = nil
	}
	a.mu.Unlock()

	// Exponential backoff
	backoff := a.config.Master.Reconnect.InitialDelay
	if backoff == 0 {
		backoff = time.Second
	}
	maxBackoff := a.config.Master.Reconnect.MaxDelay
	if maxBackoff == 0 {
		maxBackoff = 5 * time.Minute
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-a.shutdown:
			return fmt.Errorf("shutdown requested")
		default:
		}

		if err := a.connect(ctx); err != nil {
			a.logger.Warn("Reconnect attempt failed",
				zap.Error(err),
				zap.Duration("retry_in", backoff),
			)
			// Use select to respect context cancellation during backoff
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-a.shutdown:
				return fmt.Errorf("shutdown requested")
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		return nil
	}
}

func (a *Agent) commandLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.shutdown:
			return
		default:
		}

		// Establish bidirectional stream
		if err := a.runCommandStream(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			a.logger.Error("Command stream error", zap.Error(err))
			// Use select to respect context cancellation during backoff
			select {
			case <-ctx.Done():
				return
			case <-a.shutdown:
				return
			case <-time.After(5 * time.Second):
			}
		}
	}
}

// runCommandStream establishes and maintains the bidirectional gRPC stream.
func (a *Agent) runCommandStream(ctx context.Context) error {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("not connected")
	}

	stream, err := client.Connect(ctx)
	if err != nil {
		return fmt.Errorf("establishing stream: %w", err)
	}

	a.mu.Lock()
	a.stream = stream
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		a.stream = nil
		a.mu.Unlock()
	}()

	// Send initial ready message
	err = stream.Send(&pb.AgentMessage{
		Message: &pb.AgentMessage_AgentReady{
			AgentReady: &pb.AgentReady{
				AgentId:   a.config.Agent.ID,
				Timestamp: time.Now().Unix(),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("sending ready message: %w", err)
	}

	a.logger.Info("Command stream established")

	// Receive and process commands
	for {
		msg, err := stream.Recv()
		if err != nil {
			return fmt.Errorf("receiving command: %w", err)
		}

		switch cmd := msg.Message.(type) {
		case *pb.MasterMessage_DeployCommand:
			go a.handleDeployCommand(ctx, stream, cmd.DeployCommand)

		case *pb.MasterMessage_RollbackCommand:
			go a.handleRollbackCommand(ctx, stream, cmd.RollbackCommand)

		case *pb.MasterMessage_CancelCommand:
			a.handleCancelCommand(cmd.CancelCommand)

		case *pb.MasterMessage_HealthCheckCommand:
			go a.handleHealthCheckCommand(ctx, stream, cmd.HealthCheckCommand)
		}
	}
}

// handleDeployCommand executes a deployment command.
func (a *Agent) handleDeployCommand(ctx context.Context, stream pb.AgentService_ConnectClient, cmd *pb.DeployCommand) {
	deployCtx, cancel := context.WithCancel(ctx)
	cancelDone := make(chan struct{})

	// Register active deployment
	a.deployMu.Lock()
	a.activeDeployments[cmd.DeploymentId] = &activeDeployment{
		ID:         cmd.DeploymentId,
		Project:    cmd.Project,
		StartTime:  time.Now(),
		State:      pb.DeploymentState_DEPLOYMENT_STATE_PREPARING,
		Cancel:     cancel,
		cancelDone: cancelDone,
	}
	a.deployMu.Unlock()

	defer func() {
		// Signal that we're done (handles cancellation acknowledgment)
		close(cancelDone)

		a.deployMu.Lock()
		delete(a.activeDeployments, cmd.DeploymentId)
		a.deployMu.Unlock()
		cancel()
	}()

	a.logger.Info("Received deploy command",
		zap.String("deployment_id", cmd.DeploymentId),
		zap.String("project", cmd.Project),
		zap.String("commit", cmd.Commit),
	)

	// Send status update
	a.sendDeploymentStatus(stream, cmd.DeploymentId, pb.DeploymentState_DEPLOYMENT_STATE_PREPARING, "Starting deployment", 10)

	// Convert proto command to internal deploy command
	deployCmd := a.protoToDeployCommand(cmd)

	// Create log channel
	logCh := make(chan deploy.LogEntry, 100)
	go a.forwardLogs(stream, cmd.DeploymentId, logCh)

	a.updateDeploymentState(cmd.DeploymentId, pb.DeploymentState_DEPLOYMENT_STATE_DEPLOYING)
	a.sendDeploymentStatus(stream, cmd.DeploymentId, pb.DeploymentState_DEPLOYMENT_STATE_DEPLOYING, "Executing deployment", 50)

	result, err := a.strategy.Deploy(deployCtx, deployCmd, logCh)
	close(logCh)

	if err != nil {
		// Check if this was a cancellation
		if deployCtx.Err() == context.Canceled {
			a.updateDeploymentState(cmd.DeploymentId, pb.DeploymentState_DEPLOYMENT_STATE_CANCELLED)
			a.sendDeploymentStatus(stream, cmd.DeploymentId, pb.DeploymentState_DEPLOYMENT_STATE_CANCELLED, "Deployment cancelled", 100)
			return
		}
		a.updateDeploymentState(cmd.DeploymentId, pb.DeploymentState_DEPLOYMENT_STATE_FAILED)
		a.sendDeploymentStatus(stream, cmd.DeploymentId, pb.DeploymentState_DEPLOYMENT_STATE_FAILED, err.Error(), 100)
		return
	}

	a.updateDeploymentState(cmd.DeploymentId, pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED)
	a.sendDeploymentStatus(stream, cmd.DeploymentId, pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED,
		fmt.Sprintf("Deployed to %s", result.ReleasePath), 100)
}

// handleRollbackCommand executes a rollback command.
func (a *Agent) handleRollbackCommand(ctx context.Context, stream pb.AgentService_ConnectClient, cmd *pb.RollbackCommand) {
	a.logger.Info("Received rollback command",
		zap.String("deployment_id", cmd.DeploymentId),
		zap.String("project", cmd.Project),
		zap.Int32("release", cmd.ReleaseNumber),
	)

	a.sendDeploymentStatus(stream, cmd.DeploymentId, pb.DeploymentState_DEPLOYMENT_STATE_ROLLING_BACK, "Rolling back", 50)

	rollbackCmd := &deploy.RollbackCommand{
		Project:       cmd.Project,
		Target:        cmd.Target,
		Path:          cmd.Path,
		ReleaseNumber: int(cmd.ReleaseNumber),
	}

	result, err := a.strategy.Rollback(ctx, rollbackCmd)
	if err != nil {
		a.sendDeploymentStatus(stream, cmd.DeploymentId, pb.DeploymentState_DEPLOYMENT_STATE_FAILED, err.Error(), 100)
		return
	}

	a.sendDeploymentStatus(stream, cmd.DeploymentId, pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED,
		fmt.Sprintf("Rolled back to %s", result.ReleasePath), 100)
}

// handleCancelCommand cancels an in-progress deployment.
// It uses channel-based coordination to safely cancel and wait for acknowledgment.
func (a *Agent) handleCancelCommand(cmd *pb.CancelCommand) {
	a.logger.Info("Received cancel command",
		zap.String("deployment_id", cmd.DeploymentId),
		zap.String("reason", cmd.Reason),
	)

	// Hold lock while reading AND calling Cancel to avoid race with deployment cleanup
	a.deployMu.Lock()
	ad, exists := a.activeDeployments[cmd.DeploymentId]
	if !exists {
		a.deployMu.Unlock()
		a.logger.Warn("Cannot cancel deployment: not found",
			zap.String("deployment_id", cmd.DeploymentId),
		)
		return
	}

	if ad.Cancel == nil {
		a.deployMu.Unlock()
		a.logger.Warn("Cannot cancel deployment: no cancel function",
			zap.String("deployment_id", cmd.DeploymentId),
		)
		return
	}

	// Get the cancelDone channel while still holding the lock
	cancelDone := ad.cancelDone

	// Call cancel while holding the lock to prevent the deployment
	// from being deleted between our check and the cancel call
	ad.Cancel()
	a.deployMu.Unlock()

	a.logger.Info("Sent cancellation signal, waiting for acknowledgment",
		zap.String("deployment_id", cmd.DeploymentId),
	)

	// Wait for the deployment to acknowledge cancellation with timeout
	select {
	case <-cancelDone:
		a.logger.Info("Deployment cancellation acknowledged",
			zap.String("deployment_id", cmd.DeploymentId),
		)
	case <-time.After(30 * time.Second):
		a.logger.Warn("Deployment cancellation timed out waiting for acknowledgment",
			zap.String("deployment_id", cmd.DeploymentId),
		)
	}
}

// handleHealthCheckCommand performs a health check by making an HTTP request to the specified URL.
func (a *Agent) handleHealthCheckCommand(ctx context.Context, stream pb.AgentService_ConnectClient, cmd *pb.HealthCheckCommand) {
	a.logger.Info("Received health check command",
		zap.String("deployment_id", cmd.DeploymentId),
		zap.String("url", cmd.Url),
		zap.String("method", cmd.Method),
		zap.Int32("expected_status", cmd.ExpectedStatus),
		zap.Bool("trigger_rollback", cmd.TriggerRollback),
	)

	// Perform actual HTTP health check with retries
	retries := cmd.Retries
	if retries <= 0 {
		retries = 1
	}

	retryDelay := cmd.RetryDelaySeconds
	if retryDelay <= 0 {
		retryDelay = 5
	}

	var result *pb.HealthCheckResult
	var attempt int32

	for attempt = 1; attempt <= retries; attempt++ {
		result = a.performHealthCheckWithConfig(ctx, cmd)
		result.RetryCount = attempt

		if result.Success {
			break
		}

		if attempt < retries {
			a.logger.Info("Health check failed, retrying",
				zap.Int32("attempt", attempt),
				zap.Int32("max_retries", retries),
				zap.String("error", result.Error),
			)
			// Sleep between retries
			time.Sleep(time.Duration(retryDelay) * time.Second)
		}
	}

	// Set rollback trigger based on command and result
	result.TriggerRollback = cmd.TriggerRollback && !result.Success

	// Send health check result
	a.sendHealthCheckResult(stream, result)

	// Also send command result for backward compatibility
	exitCode := int32(0)
	statusMsg := fmt.Sprintf("Health check passed: status %d (in %dms)", result.StatusCode, result.ResponseTimeMs)
	if !result.Success {
		exitCode = 1
		statusMsg = result.Error
	}
	a.sendCommandResult(stream, cmd.DeploymentId, "health_check", exitCode, statusMsg, "")
}

// performHealthCheckWithConfig makes an HTTP request using the health check command configuration.
func (a *Agent) performHealthCheckWithConfig(ctx context.Context, cmd *pb.HealthCheckCommand) *pb.HealthCheckResult {
	result := &pb.HealthCheckResult{
		DeploymentId: cmd.DeploymentId,
	}

	// Set default timeout
	timeoutDuration := 10 * time.Second
	if cmd.TimeoutSeconds > 0 {
		timeoutDuration = time.Duration(cmd.TimeoutSeconds) * time.Second
	}

	// Create context with timeout for this specific request
	reqCtx, cancel := context.WithTimeout(ctx, timeoutDuration)
	defer cancel()

	// Determine HTTP method
	method := cmd.Method
	if method == "" {
		method = http.MethodGet
	}

	// Create request body if provided
	var bodyReader io.Reader
	if cmd.Body != "" {
		bodyReader = strings.NewReader(cmd.Body)
	}

	// Create request with context
	req, err := http.NewRequestWithContext(reqCtx, method, cmd.Url, bodyReader)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to create request: %v", err)
		return result
	}

	// Set user agent
	req.Header.Set("User-Agent", "vcdeploy-health-check/1.0")

	// Add custom headers
	for key, value := range cmd.Headers {
		req.Header.Set(key, value)
	}

	// Set content type if body is provided and not already set
	if cmd.Body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Perform the request using the reusable HTTP client
	start := time.Now()
	resp, err := a.httpClient.Do(req)
	duration := time.Since(start)
	result.ResponseTimeMs = duration.Milliseconds()

	if err != nil {
		result.Error = fmt.Sprintf("Health check failed: %v (after %v)", err, duration)
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = int32(resp.StatusCode) //nolint:gosec // G115: HTTP status codes are always small positive integers

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to read response body: %v", err)
		return result
	}

	// Check expected status code
	expectedStatus := cmd.ExpectedStatus
	if expectedStatus == 0 {
		expectedStatus = 200
	}

	if resp.StatusCode != int(expectedStatus) {
		result.Error = fmt.Sprintf("Health check failed: got status %d, expected %d (after %v)",
			resp.StatusCode, expectedStatus, duration)
		return result
	}

	// Check body contains if specified
	if cmd.BodyContains != "" && !strings.Contains(string(body), cmd.BodyContains) {
		result.Error = fmt.Sprintf("Health check failed: response body does not contain '%s'", cmd.BodyContains)
		return result
	}

	result.Success = true
	return result
}

// sendHealthCheckResult sends a health check result back to the master.
func (a *Agent) sendHealthCheckResult(stream pb.AgentService_ConnectClient, result *pb.HealthCheckResult) {
	msg := &pb.AgentMessage{
		Message: &pb.AgentMessage_HealthCheckResult{
			HealthCheckResult: result,
		},
	}

	if err := stream.Send(msg); err != nil {
		a.logger.Error("Failed to send health check result",
			zap.String("deployment_id", result.DeploymentId),
			zap.Error(err),
		)
	}
}

// performHealthCheck makes an HTTP request and validates the response (legacy method).
// This method accepts any 2xx status code for backward compatibility.
func (a *Agent) performHealthCheck(ctx context.Context, url string, timeout int32) (bool, string) {
	// Set default timeout
	timeoutDuration := 10 * time.Second
	if timeout > 0 {
		timeoutDuration = time.Duration(timeout) * time.Second
	}

	// Create context with timeout for this specific request
	reqCtx, cancel := context.WithTimeout(ctx, timeoutDuration)
	defer cancel()

	// Create request with context
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return false, fmt.Sprintf("Failed to create request: %v", err)
	}

	// Set user agent
	req.Header.Set("User-Agent", "vcdeploy-health-check/1.0")

	// Perform the request using the reusable HTTP client
	start := time.Now()
	resp, err := a.httpClient.Do(req)
	duration := time.Since(start)

	if err != nil {
		return false, fmt.Sprintf("Health check failed: %v (after %v)", err, duration)
	}
	defer resp.Body.Close()

	// Accept any 2xx status code for backward compatibility
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, fmt.Sprintf("Health check passed: status %d (in %v)", resp.StatusCode, duration)
	}

	return false, fmt.Sprintf("Health check failed: got status %d, expected 2xx (after %v)", resp.StatusCode, duration)
}

// protoToDeployCommand converts a protobuf deploy command to internal format.
func (a *Agent) protoToDeployCommand(cmd *pb.DeployCommand) *deploy.DeployCommand {
	deployCmd := &deploy.DeployCommand{
		DeploymentID:    cmd.DeploymentId,
		Project:         cmd.Project,
		Target:          cmd.Target,
		Repository:      cmd.Repository,
		Branch:          cmd.Branch,
		Commit:          cmd.Commit,
		Path:            cmd.Path,
		EnvVars:         cmd.EnvVars,
		EnvFileContent:  cmd.EnvFileContent,
		PreDeployHooks:  cmd.PreDeployHooks,
		PostDeployHooks: cmd.PostDeployHooks,
	}

	if cmd.Settings != nil {
		deployCmd.Settings.Strategy = cmd.Settings.Strategy
		deployCmd.Settings.KeepReleases = int(cmd.Settings.KeepReleases)
		deployCmd.Settings.SharedDirs = cmd.Settings.SharedDirs
		deployCmd.Settings.SharedFiles = cmd.Settings.SharedFiles
		deployCmd.Settings.WritableDirs = cmd.Settings.WritableDirs
		deployCmd.Settings.ExecutionUser = cmd.Settings.ExecutionUser
		deployCmd.Settings.ExecutionGroup = cmd.Settings.ExecutionGroup
		deployCmd.Settings.Timeout = time.Duration(cmd.Settings.TimeoutSeconds) * time.Second
	}

	// Convert service reloads
	for _, sr := range cmd.ReloadServices {
		deployCmd.ReloadServices = append(deployCmd.ReloadServices, deploy.ServiceReload{
			Service: sr.Service,
			Action:  sr.Action,
		})
	}

	return deployCmd
}

// sendDeploymentStatus sends a deployment status update to master.
func (a *Agent) sendDeploymentStatus(stream pb.AgentService_ConnectClient, deploymentID string, state pb.DeploymentState, message string, progress int32) {
	if stream == nil {
		return
	}

	err := stream.Send(&pb.AgentMessage{
		Message: &pb.AgentMessage_DeploymentStatus{
			DeploymentStatus: &pb.DeploymentStatus{
				DeploymentId:    deploymentID,
				State:           state,
				Message:         message,
				Timestamp:       time.Now().Unix(),
				ProgressPercent: progress,
			},
		},
	})
	if err != nil {
		a.logger.Warn("Failed to send deployment status to master",
			zap.String("deployment_id", deploymentID),
			zap.String("state", state.String()),
			zap.Error(err),
		)
	}
}

// sendCommandResult sends a command result to master.
func (a *Agent) sendCommandResult(stream pb.AgentService_ConnectClient, deploymentID, command string, exitCode int32, stdout, stderr string) {
	if stream == nil {
		return
	}

	err := stream.Send(&pb.AgentMessage{
		Message: &pb.AgentMessage_CommandResult{
			CommandResult: &pb.CommandResult{
				DeploymentId: deploymentID,
				Command:      command,
				ExitCode:     exitCode,
				Stdout:       stdout,
				Stderr:       stderr,
			},
		},
	})
	if err != nil {
		a.logger.Warn("Failed to send command result to master",
			zap.String("deployment_id", deploymentID),
			zap.String("command", command),
			zap.Error(err),
		)
	}
}

// forwardLogs forwards deployment logs to the master.
func (a *Agent) forwardLogs(stream pb.AgentService_ConnectClient, deploymentID string, logCh <-chan deploy.LogEntry) {
	for entry := range logCh {
		if stream == nil {
			continue
		}

		level := pb.LogLevel_LOG_LEVEL_INFO
		switch entry.Level {
		case deploy.LogDebug:
			level = pb.LogLevel_LOG_LEVEL_DEBUG
		case deploy.LogWarn:
			level = pb.LogLevel_LOG_LEVEL_WARN
		case deploy.LogError:
			level = pb.LogLevel_LOG_LEVEL_ERROR
		}

		err := stream.Send(&pb.AgentMessage{
			Message: &pb.AgentMessage_DeploymentLog{
				DeploymentLog: &pb.DeploymentLog{
					DeploymentId: deploymentID,
					Timestamp:    time.Now().Unix(),
					Level:        level,
					Message:      entry.Message,
					Source:       entry.Source,
				},
			},
		})
		if err != nil {
			a.logger.Debug("Failed to forward log to master",
				zap.String("deployment_id", deploymentID),
				zap.Error(err),
			)
			// Continue processing logs even if send fails
		}
	}
}

// updateDeploymentState updates the state of an active deployment.
func (a *Agent) updateDeploymentState(deploymentID string, state pb.DeploymentState) {
	a.deployMu.Lock()
	if ad, exists := a.activeDeployments[deploymentID]; exists {
		ad.State = state
	}
	a.deployMu.Unlock()
}

// ExecuteDeploy runs a deployment on this agent.
func (a *Agent) ExecuteDeploy(ctx context.Context, cmd *deploy.DeployCommand) (*deploy.DeployResult, error) {
	a.logger.Info("Executing deployment",
		zap.String("project", cmd.Project),
		zap.String("commit", cmd.Commit),
	)

	// Create log channel for deployment output
	logCh := make(chan deploy.LogEntry, 100)

	// Log entries asynchronously
	go func() {
		defer func() {
			if r := recover(); r != nil {
				a.logger.Error("panic in deployment log handler", zap.Any("panic", r))
			}
		}()
		for entry := range logCh {
			a.logger.Info(entry.Message,
				zap.String("source", entry.Source),
				zap.String("level", entry.Level.String()),
			)
		}
	}()

	result, err := a.strategy.Deploy(ctx, cmd, logCh)
	close(logCh)

	if err != nil {
		a.logger.Error("Deployment failed",
			zap.String("project", cmd.Project),
			zap.Error(err),
		)
		return result, err
	}

	a.logger.Info("Deployment completed",
		zap.String("project", cmd.Project),
		zap.String("release", result.ReleasePath),
		zap.Duration("duration", result.CompletedAt.Sub(result.StartedAt)),
	)

	return result, nil
}

// ExecuteRollback rolls back a project on this agent.
func (a *Agent) ExecuteRollback(ctx context.Context, cmd *deploy.RollbackCommand) (*deploy.DeployResult, error) {
	a.logger.Info("Executing rollback",
		zap.String("project", cmd.Project),
		zap.Int("release", cmd.ReleaseNumber),
	)

	result, err := a.strategy.Rollback(ctx, cmd)
	if err != nil {
		a.logger.Error("Rollback failed",
			zap.String("project", cmd.Project),
			zap.Error(err),
		)
		return result, err
	}

	a.logger.Info("Rollback completed",
		zap.String("project", cmd.Project),
		zap.String("release", result.ReleasePath),
	)

	return result, nil
}

// Shutdown gracefully stops the agent.
func (a *Agent) Shutdown(ctx context.Context) error {
	a.logger.Info("Shutting down agent")

	close(a.shutdown)

	a.mu.Lock()
	if a.conn != nil {
		a.conn.Close()
		a.conn = nil
	}
	a.running = false
	a.mu.Unlock()

	// Close the store
	if a.store != nil {
		if err := a.store.Close(); err != nil {
			a.logger.Warn("Error closing agent store", zap.Error(err))
		}
	}

	// Wait for goroutines with timeout
	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				a.logger.Error("panic in shutdown wait handler", zap.Any("panic", r))
			}
		}()
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Status returns the current agent status.
func (a *Agent) Status() string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.running {
		return "stopped"
	}
	if a.conn == nil {
		return "disconnected"
	}
	return "connected"
}

// --- Self-Update Methods ---

// handleUpdateNotification processes an update notification from the master.
func (a *Agent) handleUpdateNotification(notification *pb.UpdateNotification) {
	if notification == nil {
		return
	}

	// Check if we're already updating
	a.updateMu.RLock()
	if a.updateInProgress {
		a.updateMu.RUnlock()
		a.logger.Debug("Update already in progress, ignoring notification")
		return
	}
	a.updateMu.RUnlock()

	a.logger.Info("Update available",
		zap.String("current_version", AgentVersion),
		zap.String("new_version", notification.Version),
		zap.Bool("force", notification.Force),
	)

	// Skip if same version
	if notification.Version == AgentVersion && !notification.Force {
		a.logger.Debug("Already at target version")
		return
	}

	// Check update policy from config
	updatePolicy := a.config.Agent.UpdatePolicy
	if updatePolicy == "" {
		updatePolicy = "immediate" // Default to immediate
	}

	switch updatePolicy {
	case "immediate":
		// Update immediately
		go a.performSelfUpdate(notification)

	case "scheduled":
		// Check if within update window
		if a.isWithinUpdateWindow() {
			go a.performSelfUpdate(notification)
		} else {
			a.logger.Info("Update deferred - outside maintenance window",
				zap.String("window_start", a.config.Agent.UpdateWindowStart),
				zap.String("window_end", a.config.Agent.UpdateWindowEnd),
			)
		}

	case "manual":
		// Only update if force flag is set
		if notification.Force {
			go a.performSelfUpdate(notification)
		} else {
			a.logger.Info("Update available but policy is manual - skipping")
		}
	}
}

// isWithinUpdateWindow checks if current time is within the configured update window.
func (a *Agent) isWithinUpdateWindow() bool {
	start := a.config.Agent.UpdateWindowStart
	end := a.config.Agent.UpdateWindowEnd

	if start == "" || end == "" {
		return true // No window configured, allow updates anytime
	}

	now := time.Now()
	nowMinutes := now.Hour()*60 + now.Minute()

	// Parse HH:MM format
	var startH, startM, endH, endM int
	if _, err := fmt.Sscanf(start, "%d:%d", &startH, &startM); err != nil {
		return true
	}
	if _, err := fmt.Sscanf(end, "%d:%d", &endH, &endM); err != nil {
		return true
	}

	startMinutes := startH*60 + startM
	endMinutes := endH*60 + endM

	// Handle window crossing midnight
	if startMinutes <= endMinutes {
		return nowMinutes >= startMinutes && nowMinutes <= endMinutes
	}
	return nowMinutes >= startMinutes || nowMinutes <= endMinutes
}

// performSelfUpdate downloads and applies the update.
func (a *Agent) performSelfUpdate(notification *pb.UpdateNotification) {
	a.updateMu.Lock()
	if a.updateInProgress {
		a.updateMu.Unlock()
		return
	}
	a.updateInProgress = true
	a.updateMu.Unlock()

	defer func() {
		a.updateMu.Lock()
		a.updateInProgress = false
		a.updateMu.Unlock()
	}()

	a.logger.Info("Starting self-update",
		zap.String("from_version", AgentVersion),
		zap.String("to_version", notification.Version),
	)

	// Get current executable path
	execPath, err := os.Executable()
	if err != nil {
		a.logger.Error("Failed to get executable path", zap.Error(err))
		return
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		a.logger.Error("Failed to resolve executable path", zap.Error(err))
		return
	}

	// Create temp directory for download
	tmpDir, err := os.MkdirTemp("", "vcdeploy-update-*")
	if err != nil {
		a.logger.Error("Failed to create temp directory", zap.Error(err))
		return
	}
	defer os.RemoveAll(tmpDir)

	// Download new binary
	newBinaryPath := filepath.Join(tmpDir, "vcdeploy-agent-new")
	if err := a.downloadBinary(notification.DownloadUrl, newBinaryPath, notification.ChecksumSha256, notification.SizeBytes); err != nil {
		a.logger.Error("Failed to download update", zap.Error(err))
		return
	}

	// Make it executable
	if err := os.Chmod(newBinaryPath, 0o755); err != nil {
		a.logger.Error("Failed to make new binary executable", zap.Error(err))
		return
	}

	// Verify new binary can run
	cmd := exec.Command(newBinaryPath, "version") //nolint:gosec // G204: newBinaryPath is internally constructed from version/OS/arch, not user input
	if output, err := cmd.CombinedOutput(); err != nil {
		a.logger.Error("New binary failed verification",
			zap.Error(err),
			zap.String("output", string(output)),
		)
		return
	}

	// Backup current binary
	backupPath := execPath + ".backup"
	if err := os.Rename(execPath, backupPath); err != nil {
		a.logger.Error("Failed to backup current binary", zap.Error(err))
		return
	}

	// Move new binary into place
	if err := copyFile(newBinaryPath, execPath); err != nil {
		// Rollback
		a.logger.Error("Failed to install new binary, rolling back", zap.Error(err))
		if rbErr := os.Rename(backupPath, execPath); rbErr != nil {
			a.logger.Error("CRITICAL: Rollback failed", zap.Error(rbErr))
		}
		return
	}

	// Make the new binary executable again (in case copyFile didn't preserve permissions)
	if err := os.Chmod(execPath, 0o755); err != nil {
		a.logger.Warn("Failed to set executable permissions", zap.Error(err))
	}

	a.logger.Info("Update installed successfully, requesting restart",
		zap.String("from_version", AgentVersion),
		zap.String("to_version", notification.Version),
	)

	// Clean up backup
	os.Remove(backupPath)

	// Trigger restart - the systemd service or supervisor should restart us
	// We send SIGTERM to ourselves and rely on the process manager to restart
	go func() {
		time.Sleep(2 * time.Second) // Give time for logs to flush
		a.logger.Info("Exiting for restart...")
		os.Exit(0) // Exit with success code so systemd restarts us
	}()
}

// downloadBinary downloads the binary from the URL and verifies the checksum.
func (a *Agent) downloadBinary(url, destPath, expectedChecksum string, expectedSize int64) error {
	a.logger.Info("Downloading update",
		zap.String("url", url),
		zap.Int64("expected_size", expectedSize),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return fmt.Errorf("create download request: %w", err)
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	// Create destination file
	dest, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create destination file: %w", err)
	}
	defer dest.Close()

	// Download and calculate checksum
	hasher := sha256.New()
	multiWriter := io.MultiWriter(dest, hasher)

	written, err := io.Copy(multiWriter, resp.Body)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Verify size
	if expectedSize > 0 && written != expectedSize {
		return fmt.Errorf("size mismatch: got %d, expected %d", written, expectedSize)
	}

	// Verify checksum
	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	if expectedChecksum != "" && actualChecksum != expectedChecksum {
		return fmt.Errorf("checksum mismatch: got %s, expected %s", actualChecksum, expectedChecksum)
	}

	a.logger.Info("Download complete",
		zap.Int64("bytes", written),
		zap.String("checksum", actualChecksum),
	)

	return nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return dstFile.Sync()
}

// splitCertAndKey separates a PEM bundle into certificate and key parts.
// If the input contains both CERTIFICATE and PRIVATE KEY blocks, they are split.
// Otherwise, certPEM equals the input and keyPEM is nil.
func splitCertAndKey(data []byte) (certPEM, keyPEM []byte) {
	var certBlocks [][]byte
	var keyBlocks [][]byte

	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}

		// Re-encode the block to preserve formatting
		encoded := pem.EncodeToMemory(block)

		switch block.Type {
		case "CERTIFICATE":
			certBlocks = append(certBlocks, encoded)
		case "PRIVATE KEY", "RSA PRIVATE KEY", "EC PRIVATE KEY":
			keyBlocks = append(keyBlocks, encoded)
		default:
			// Unknown block type, add to certs
			certBlocks = append(certBlocks, encoded)
		}
	}

	if len(certBlocks) > 0 {
		certPEM = bytes.Join(certBlocks, nil)
	}
	if len(keyBlocks) > 0 {
		keyPEM = bytes.Join(keyBlocks, nil)
	}

	return certPEM, keyPEM
}
