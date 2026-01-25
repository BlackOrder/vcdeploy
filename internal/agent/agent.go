// Package agent provides the agent daemon implementation.
package agent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
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
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Agent is the deployment agent daemon.
type Agent struct {
	config   *config.AgentConfig
	logger   *zap.Logger
	strategy *deploy.SymlinkStrategy
	runner   *LocalRunner

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

	if a.config.Master.Cert != "" {
		// Use TLS with CA cert
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

	// Add retry options
	opts = append(opts,
		grpc.WithBlock(),
	)

	dialCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(dialCtx, a.config.Master.Address, opts...)
	if err != nil {
		return fmt.Errorf("dialing master: %w", err)
	}

	a.conn = conn
	a.client = pb.NewAgentServiceClient(conn)
	a.logger.Info("Connected to master", zap.String("address", a.config.Master.Address))

	return nil
}

// Register registers the agent with the master server using a token.
// Returns the mTLS certificate and CA certificate on success.
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
		caps.DiskSpaceBytes = int64(diskStat.Total)
	}

	// Get memory info
	if vmStat, err := mem.VirtualMemory(); err == nil {
		caps.MemoryBytes = int64(vmStat.Total)
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
		return nil, nil, fmt.Errorf("registration RPC failed: %w", err)
	}

	if !resp.Success {
		return nil, nil, fmt.Errorf("registration rejected: %s", resp.Error)
	}

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
	}

	hostname, _ := os.Hostname()
	a.logger.Debug("Sending heartbeat",
		zap.String("hostname", hostname),
		zap.Int("active_deployments", len(activeStatuses)),
	)

	resp, err := client.Heartbeat(ctx, req)
	if err != nil {
		return fmt.Errorf("sending heartbeat: %w", err)
	}

	if !resp.Ok {
		return fmt.Errorf("heartbeat rejected by master")
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
		stats.DiskFreeBytes = int64(diskStat.Free)
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
			time.Sleep(backoff)
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
			time.Sleep(5 * time.Second)
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
	)

	// Perform actual HTTP health check with retries
	retries := cmd.Retries
	if retries <= 0 {
		retries = 1
	}

	var result bool
	var statusMsg string

	for attempt := int32(1); attempt <= retries; attempt++ {
		result, statusMsg = a.performHealthCheck(ctx, cmd.Url, cmd.TimeoutSeconds)
		if result {
			break
		}
		if attempt < retries {
			a.logger.Info("Health check failed, retrying",
				zap.Int32("attempt", attempt),
				zap.Int32("max_retries", retries),
			)
			time.Sleep(time.Second)
		}
	}

	exitCode := int32(0)
	if !result {
		exitCode = 1
	}

	a.sendCommandResult(stream, cmd.DeploymentId, "health_check", exitCode, statusMsg, "")
}

// performHealthCheck makes an HTTP request and validates the response.
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
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Sprintf("Failed to create request: %v", err)
	}

	// Set a user agent
	req.Header.Set("User-Agent", "vcdeploy-health-check/1.0")

	// Perform the request using the reusable HTTP client
	start := time.Now()
	resp, err := a.httpClient.Do(req)
	duration := time.Since(start)

	if err != nil {
		return false, fmt.Sprintf("Health check failed: %v (after %v)", err, duration)
	}
	defer resp.Body.Close()

	// Read and discard the body (to allow connection reuse)
	_, _ = io.Copy(io.Discard, resp.Body)

	// Check status code (accept 2xx as success)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Sprintf("Health check failed: got status %d (after %v)",
			resp.StatusCode, duration)
	}

	return true, fmt.Sprintf("Health check passed: status %d (in %v)", resp.StatusCode, duration)
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
