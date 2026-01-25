// Package server provides background cleanup tasks.
package server

import (
	"context"
	"sync"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// CleanupTask handles periodic cleanup of old data.
type CleanupTask struct {
	db                     *storage.DB
	logger                 *zap.Logger
	stopCh                 chan struct{}
	wg                     sync.WaitGroup
	interval               time.Duration
	deploymentRetention    time.Duration
	auditLogRetention      time.Duration
	sessionExpiry          time.Duration
	staleAgentThreshold    time.Duration
	deploymentLogRetention time.Duration
}

// CleanupConfig holds configuration for the cleanup task.
type CleanupConfig struct {
	Interval               time.Duration // How often to run cleanup (default: 1 hour)
	DeploymentRetention    time.Duration // How long to keep deployment records (default: 30 days)
	AuditLogRetention      time.Duration // How long to keep audit logs (default: 90 days)
	SessionExpiry          time.Duration // Session expiry time (default: 24 hours)
	StaleAgentThreshold    time.Duration // Mark agents stale after this time (default: 5 minutes)
	DeploymentLogRetention time.Duration // How long to keep deployment logs (default: 7 days)
}

// DefaultCleanupConfig returns the default cleanup configuration.
func DefaultCleanupConfig() CleanupConfig {
	return CleanupConfig{
		Interval:               1 * time.Hour,
		DeploymentRetention:    30 * 24 * time.Hour,
		AuditLogRetention:      90 * 24 * time.Hour,
		SessionExpiry:          24 * time.Hour,
		StaleAgentThreshold:    5 * time.Minute,
		DeploymentLogRetention: 7 * 24 * time.Hour,
	}
}

// NewCleanupTask creates a new cleanup task.
func NewCleanupTask(db *storage.DB, logger *zap.Logger, cfg CleanupConfig) *CleanupTask {
	if cfg.Interval == 0 {
		cfg.Interval = time.Hour
	}
	if cfg.DeploymentRetention == 0 {
		cfg.DeploymentRetention = 30 * 24 * time.Hour
	}
	if cfg.AuditLogRetention == 0 {
		cfg.AuditLogRetention = 90 * 24 * time.Hour
	}
	if cfg.SessionExpiry == 0 {
		cfg.SessionExpiry = 24 * time.Hour
	}
	if cfg.StaleAgentThreshold == 0 {
		cfg.StaleAgentThreshold = 5 * time.Minute
	}
	if cfg.DeploymentLogRetention == 0 {
		cfg.DeploymentLogRetention = 7 * 24 * time.Hour
	}

	return &CleanupTask{
		db:                     db,
		logger:                 logger,
		stopCh:                 make(chan struct{}),
		interval:               cfg.Interval,
		deploymentRetention:    cfg.DeploymentRetention,
		auditLogRetention:      cfg.AuditLogRetention,
		sessionExpiry:          cfg.SessionExpiry,
		staleAgentThreshold:    cfg.StaleAgentThreshold,
		deploymentLogRetention: cfg.DeploymentLogRetention,
	}
}

// Start starts the cleanup task in a background goroutine.
func (c *CleanupTask) Start() {
	c.wg.Go(c.run)
	c.logger.Info("Cleanup task started",
		zap.Duration("interval", c.interval),
		zap.Duration("deploymentRetention", c.deploymentRetention),
		zap.Duration("auditLogRetention", c.auditLogRetention),
	)
}

// Stop stops the cleanup task.
func (c *CleanupTask) Stop() {
	close(c.stopCh)
	c.wg.Wait()
	c.logger.Info("Cleanup task stopped")
}

func (c *CleanupTask) run() {
	// Run cleanup immediately on start
	c.runCleanup()

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.runCleanup()
		case <-c.stopCh:
			return
		}
	}
}

func (c *CleanupTask) runCleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	c.logger.Debug("Running cleanup tasks")

	// Clean up expired sessions
	if count, err := c.cleanExpiredSessions(ctx); err != nil {
		c.logger.Error("Failed to clean expired sessions", zap.Error(err))
	} else if count > 0 {
		c.logger.Info("Cleaned expired sessions", zap.Int64("count", count))
	}

	// Clean up old deployments
	if count, err := c.cleanOldDeployments(ctx); err != nil {
		c.logger.Error("Failed to clean old deployments", zap.Error(err))
	} else if count > 0 {
		c.logger.Info("Cleaned old deployments", zap.Int64("count", count))
	}

	// Clean up old deployment logs
	if count, err := c.cleanOldDeploymentLogs(ctx); err != nil {
		c.logger.Error("Failed to clean old deployment logs", zap.Error(err))
	} else if count > 0 {
		c.logger.Info("Cleaned old deployment logs", zap.Int64("count", count))
	}

	// Clean up old audit logs
	if count, err := c.cleanOldAuditLogs(ctx); err != nil {
		c.logger.Error("Failed to clean old audit logs", zap.Error(err))
	} else if count > 0 {
		c.logger.Info("Cleaned old audit logs", zap.Int64("count", count))
	}

	// Mark stale agents
	if count, err := c.markStaleAgents(ctx); err != nil {
		c.logger.Error("Failed to mark stale agents", zap.Error(err))
	} else if count > 0 {
		c.logger.Info("Marked agents as stale", zap.Int64("count", count))
	}

	// Clean up expired API keys
	if count, err := c.cleanExpiredAPIKeys(ctx); err != nil {
		c.logger.Error("Failed to clean expired API keys", zap.Error(err))
	} else if count > 0 {
		c.logger.Info("Cleaned expired API keys", zap.Int64("count", count))
	}

	// Clean up orphaned webhook secrets (webhooks for deleted projects)
	if count, err := c.cleanOrphanedWebhooks(ctx); err != nil {
		c.logger.Error("Failed to clean orphaned webhooks", zap.Error(err))
	} else if count > 0 {
		c.logger.Info("Cleaned orphaned webhooks", zap.Int64("count", count))
	}

	c.logger.Debug("Cleanup tasks completed")
}

// cleanExpiredSessions removes sessions that have expired.
func (c *CleanupTask) cleanExpiredSessions(ctx context.Context) (int64, error) {
	return c.db.CleanupExpiredSessions(ctx, time.Now().Add(-c.sessionExpiry))
}

// cleanOldDeployments removes deployment records older than the retention period.
func (c *CleanupTask) cleanOldDeployments(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-c.deploymentRetention)
	return c.db.CleanupOldDeployments(ctx, cutoff)
}

// cleanOldDeploymentLogs removes deployment logs older than the retention period.
func (c *CleanupTask) cleanOldDeploymentLogs(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-c.deploymentLogRetention)
	return c.db.CleanupOldDeploymentLogs(ctx, cutoff)
}

// cleanOldAuditLogs removes audit log entries older than the retention period.
func (c *CleanupTask) cleanOldAuditLogs(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-c.auditLogRetention)
	return c.db.CleanupOldAuditLogs(ctx, cutoff)
}

// markStaleAgents marks agents that haven't been seen recently as stale.
func (c *CleanupTask) markStaleAgents(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-c.staleAgentThreshold)
	return c.db.MarkStaleAgents(ctx, cutoff)
}

// cleanExpiredAPIKeys removes API keys that have expired.
func (c *CleanupTask) cleanExpiredAPIKeys(ctx context.Context) (int64, error) {
	return c.db.CleanupExpiredAPIKeys(ctx, time.Now())
}

// cleanOrphanedWebhooks removes webhook configs for deleted projects.
func (c *CleanupTask) cleanOrphanedWebhooks(ctx context.Context) (int64, error) {
	return c.db.CleanupOrphanedWebhooks(ctx)
}
