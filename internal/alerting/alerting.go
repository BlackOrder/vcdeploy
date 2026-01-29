// Package alerting provides system alerting capabilities for vcdeploy.
// It monitors system health and sends alerts via the notification system.
package alerting

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/notify"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// AlertType represents the type of alert.
type AlertType string

const (
	// AlertAgentDisconnected is sent when an agent disconnects unexpectedly.
	AlertAgentDisconnected AlertType = "agent_disconnected"
	// AlertAgentReconnected is sent when a previously disconnected agent reconnects.
	AlertAgentReconnected AlertType = "agent_reconnected"
	// AlertDiskWarning is sent when agent disk usage exceeds threshold.
	AlertDiskWarning AlertType = "disk_warning"
	// AlertDiskCritical is sent when agent disk usage is critical.
	AlertDiskCritical AlertType = "disk_critical"
	// AlertHighMemory is sent when agent memory usage exceeds threshold.
	AlertHighMemory AlertType = "high_memory"
	// AlertHighCPU is sent when agent CPU usage exceeds threshold.
	AlertHighCPU AlertType = "high_cpu"
	// AlertDeploymentStuck is sent when a deployment exceeds timeout.
	AlertDeploymentStuck AlertType = "deployment_stuck"
	// AlertHighErrorRate is sent when error rate exceeds threshold.
	AlertHighErrorRate AlertType = "high_error_rate"
)

// Thresholds defines alert thresholds.
type Thresholds struct {
	// DiskWarningPercent triggers a warning alert (default: 80)
	DiskWarningPercent float64 `yaml:"disk_warning_percent"`
	// DiskCriticalPercent triggers a critical alert (default: 90)
	DiskCriticalPercent float64 `yaml:"disk_critical_percent"`
	// MemoryWarningPercent triggers a memory warning (default: 85)
	MemoryWarningPercent float64 `yaml:"memory_warning_percent"`
	// CPUWarningPercent triggers a CPU warning (default: 90)
	CPUWarningPercent float64 `yaml:"cpu_warning_percent"`
	// DeploymentTimeout triggers stuck deployment alert (default: 30m)
	DeploymentTimeout time.Duration `yaml:"deployment_timeout"`
	// AlertCooldown prevents alert storms (default: 15m)
	AlertCooldown time.Duration `yaml:"alert_cooldown"`
}

// DefaultThresholds returns sensible default thresholds.
func DefaultThresholds() Thresholds {
	return Thresholds{
		DiskWarningPercent:   80,
		DiskCriticalPercent:  90,
		MemoryWarningPercent: 85,
		CPUWarningPercent:    90,
		DeploymentTimeout:    30 * time.Minute,
		AlertCooldown:        15 * time.Minute,
	}
}

// Manager handles system alerts and sends notifications.
type Manager struct {
	notifier   *notify.Manager
	logger     *zap.Logger
	thresholds Thresholds
	cooldowns  map[string]time.Time
	mu         sync.RWMutex
}

// NewManager creates a new alert manager.
func NewManager(notifier *notify.Manager, logger *zap.Logger, thresholds Thresholds) *Manager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Manager{
		notifier:   notifier,
		logger:     logger,
		thresholds: thresholds,
		cooldowns:  make(map[string]time.Time),
	}
}

// canAlert checks if an alert can be sent (respects cooldown).
func (m *Manager) canAlert(key string) bool {
	m.mu.RLock()
	lastAlert, exists := m.cooldowns[key]
	m.mu.RUnlock()

	if exists && time.Since(lastAlert) < m.thresholds.AlertCooldown {
		return false
	}

	m.mu.Lock()
	m.cooldowns[key] = time.Now()
	m.mu.Unlock()

	return true
}

// sendAlert sends an alert via the notification system.
func (m *Manager) sendAlert(ctx context.Context, alertType AlertType, title, message string) {
	event := notify.Event{
		Type:      string(alertType),
		Message:   message,
		Status:    "alert",
		Timestamp: time.Now(),
	}

	m.logger.Warn("alert triggered",
		zap.String("type", string(alertType)),
		zap.String("title", title),
		zap.String("message", message),
	)

	if m.notifier != nil {
		m.notifier.Notify(ctx, event)
	}
}

// CheckAgentDisconnect sends an alert when an agent disconnects.
func (m *Manager) CheckAgentDisconnect(ctx context.Context, agent *storage.Agent) {
	key := fmt.Sprintf("agent_disconnect_%s", agent.ID)
	if !m.canAlert(key) {
		return
	}

	title := fmt.Sprintf("Agent Disconnected: %s", agent.Hostname)
	message := fmt.Sprintf("Agent %s (%s) has disconnected. Last seen: %s",
		agent.ID, agent.Hostname, agent.LastSeenAt.Format(time.RFC3339))

	m.sendAlert(ctx, AlertAgentDisconnected, title, message)
}

// CheckAgentReconnect sends an alert when a previously disconnected agent reconnects.
func (m *Manager) CheckAgentReconnect(ctx context.Context, agent *storage.Agent) {
	// Clear the disconnect cooldown so future disconnects will alert immediately
	key := fmt.Sprintf("agent_disconnect_%s", agent.ID)
	m.mu.Lock()
	delete(m.cooldowns, key)
	m.mu.Unlock()

	// Send reconnect notification (no cooldown for reconnects)
	title := fmt.Sprintf("Agent Reconnected: %s", agent.Hostname)
	message := fmt.Sprintf("Agent %s (%s) has reconnected",
		agent.ID, agent.Hostname)

	m.sendAlert(ctx, AlertAgentReconnected, title, message)
}

// CheckDiskUsage checks disk usage and sends alerts if thresholds are exceeded.
func (m *Manager) CheckDiskUsage(ctx context.Context, agentID, hostname string, diskPercent float64) {
	if diskPercent >= m.thresholds.DiskCriticalPercent {
		key := fmt.Sprintf("disk_critical_%s", agentID)
		if m.canAlert(key) {
			title := fmt.Sprintf("CRITICAL: Disk Space on %s", hostname)
			message := fmt.Sprintf("Agent %s disk usage is critical at %.1f%% (threshold: %.1f%%)",
				agentID, diskPercent, m.thresholds.DiskCriticalPercent)
			m.sendAlert(ctx, AlertDiskCritical, title, message)
		}
	} else if diskPercent >= m.thresholds.DiskWarningPercent {
		key := fmt.Sprintf("disk_warning_%s", agentID)
		if m.canAlert(key) {
			title := fmt.Sprintf("Warning: Disk Space on %s", hostname)
			message := fmt.Sprintf("Agent %s disk usage is at %.1f%% (warning threshold: %.1f%%)",
				agentID, diskPercent, m.thresholds.DiskWarningPercent)
			m.sendAlert(ctx, AlertDiskWarning, title, message)
		}
	}
}

// CheckMemoryUsage checks memory usage and sends alerts if threshold is exceeded.
func (m *Manager) CheckMemoryUsage(ctx context.Context, agentID, hostname string, memPercent float64) {
	if memPercent >= m.thresholds.MemoryWarningPercent {
		key := fmt.Sprintf("memory_warning_%s", agentID)
		if m.canAlert(key) {
			title := fmt.Sprintf("Warning: High Memory on %s", hostname)
			message := fmt.Sprintf("Agent %s memory usage is at %.1f%% (threshold: %.1f%%)",
				agentID, memPercent, m.thresholds.MemoryWarningPercent)
			m.sendAlert(ctx, AlertHighMemory, title, message)
		}
	}
}

// CheckCPUUsage checks CPU usage and sends alerts if threshold is exceeded.
func (m *Manager) CheckCPUUsage(ctx context.Context, agentID, hostname string, cpuPercent float64) {
	if cpuPercent >= m.thresholds.CPUWarningPercent {
		key := fmt.Sprintf("cpu_warning_%s", agentID)
		if m.canAlert(key) {
			title := fmt.Sprintf("Warning: High CPU on %s", hostname)
			message := fmt.Sprintf("Agent %s CPU usage is at %.1f%% (threshold: %.1f%%)",
				agentID, cpuPercent, m.thresholds.CPUWarningPercent)
			m.sendAlert(ctx, AlertHighCPU, title, message)
		}
	}
}

// CheckDeploymentStuck checks if a deployment has exceeded timeout.
func (m *Manager) CheckDeploymentStuck(ctx context.Context, deploymentID, projectName string, startTime time.Time) {
	if time.Since(startTime) < m.thresholds.DeploymentTimeout {
		return
	}

	key := fmt.Sprintf("deployment_stuck_%s", deploymentID)
	if !m.canAlert(key) {
		return
	}

	title := fmt.Sprintf("Deployment Stuck: %s", projectName)
	message := fmt.Sprintf("Deployment %s for project %s has been running for %s (timeout: %s)",
		deploymentID, projectName,
		time.Since(startTime).Round(time.Second),
		m.thresholds.DeploymentTimeout)

	m.sendAlert(ctx, AlertDeploymentStuck, title, message)
}

// CheckAgentMetrics is a convenience method to check all agent metrics at once.
func (m *Manager) CheckAgentMetrics(ctx context.Context, agentID, hostname string, cpuPercent, memPercent, diskPercent float64) {
	m.CheckCPUUsage(ctx, agentID, hostname, cpuPercent)
	m.CheckMemoryUsage(ctx, agentID, hostname, memPercent)
	m.CheckDiskUsage(ctx, agentID, hostname, diskPercent)
}

// ClearCooldown clears a specific cooldown (useful for testing or manual reset).
func (m *Manager) ClearCooldown(key string) {
	m.mu.Lock()
	delete(m.cooldowns, key)
	m.mu.Unlock()
}

// ClearAllCooldowns clears all cooldowns.
func (m *Manager) ClearAllCooldowns() {
	m.mu.Lock()
	m.cooldowns = make(map[string]time.Time)
	m.mu.Unlock()
}
