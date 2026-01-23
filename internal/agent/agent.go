// Package agent provides the agent daemon implementation.
package agent

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/deploy"
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
	conn *grpc.ClientConn
	mu   sync.RWMutex

	// State
	running  bool
	shutdown chan struct{}
	wg       sync.WaitGroup
}

// NewAgent creates a new agent instance.
func NewAgent(cfg *config.AgentConfig, logger *zap.Logger) (*Agent, error) {
	// Create a local command runner
	runner := NewLocalRunner(logger)

	// Create the deployment strategy
	strategy := deploy.NewSymlinkStrategy(runner)

	return &Agent{
		config:   cfg,
		logger:   logger,
		strategy: strategy,
		runner:   runner,
		shutdown: make(chan struct{}),
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
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.heartbeatLoop(ctx)
	}()

	// Start command listener
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.commandLoop(ctx)
	}()

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
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
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
	a.logger.Info("Connected to master", zap.String("address", a.config.Master.Address))

	return nil
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
	conn := a.conn
	a.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	// Get system info for heartbeat
	hostname, _ := os.Hostname()

	a.logger.Debug("Sending heartbeat",
		zap.String("hostname", hostname),
	)

	// TODO: Send actual gRPC heartbeat to master
	// This would use the generated protobuf client

	return nil
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
	// TODO: Implement streaming command receiver from master
	// This would establish a bi-directional gRPC stream and listen
	// for deployment commands

	<-ctx.Done()
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
