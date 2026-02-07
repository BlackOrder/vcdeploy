// Package deploy provides deployment orchestration and execution.
package deploy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/xid"
)

// Orchestrator coordinates deployments across agents.
type Orchestrator struct {
	deployments     map[string]*Deployment
	deploymentMutex sync.RWMutex

	// callbacks for deployment events
	onStatusChange func(deploymentID string, status DeploymentStatus)
	onLog          func(deploymentID string, entry LogEntry)
	sendCommand    func(agentID string, cmd *DeployCommand) error
}

// DeploymentStatus represents the state of a deployment.
type DeploymentStatus string

const (
	StatusPending     DeploymentStatus = "pending"
	StatusQueued      DeploymentStatus = "queued"
	StatusPreparing   DeploymentStatus = "preparing"
	StatusCloning     DeploymentStatus = "cloning"
	StatusBuilding    DeploymentStatus = "building"
	StatusDeploying   DeploymentStatus = "deploying"
	StatusVerifying   DeploymentStatus = "verifying"
	StatusCompleted   DeploymentStatus = "completed"
	StatusFailed      DeploymentStatus = "failed"
	StatusCancelled   DeploymentStatus = "cancelled"
	StatusRollingBack DeploymentStatus = "rolling_back"
)

// Deployment represents an active deployment.
type Deployment struct {
	ID            string
	ProjectID     string
	TargetID      string
	AgentID       string
	Status        DeploymentStatus
	Progress      int
	CurrentStep   string
	ReleaseNumber int

	// Source
	Repository string
	Branch     string
	Commit     string

	// Timestamps
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time

	// Trigger info
	TriggeredBy   string
	TriggerSource string

	// Results
	Error error
	Logs  []LogEntry

	// Mutex for concurrent access
	mutex sync.RWMutex
}

// OrchestratorConfig contains configuration for the orchestrator.
type OrchestratorConfig struct {
	// OnStatusChange is called when a deployment status changes
	OnStatusChange func(deploymentID string, status DeploymentStatus)
	// OnLog is called when a deployment log entry is added
	OnLog func(deploymentID string, entry LogEntry)
	// SendCommand sends a deploy command to an agent
	SendCommand func(agentID string, cmd *DeployCommand) error
}

// NewOrchestrator creates a new deployment orchestrator.
func NewOrchestrator(cfg OrchestratorConfig) *Orchestrator {
	return &Orchestrator{
		deployments:    make(map[string]*Deployment),
		onStatusChange: cfg.OnStatusChange,
		onLog:          cfg.OnLog,
		sendCommand:    cfg.SendCommand,
	}
}

// DeployRequest contains the information needed to start a deployment.
//
//nolint:revive // Keeping explicit naming for clarity
type DeployRequest struct {
	ProjectID       string
	TargetID        string
	AgentID         string
	Repository      string
	Branch          string
	Commit          string
	Path            string
	Settings        DeploySettings
	EnvVars         map[string]string
	EnvFileContent  []byte
	PreDeployHooks  []string
	PostDeployHooks []string
	ReloadServices  []ServiceReload
	TriggeredBy     string
	TriggerSource   string
}

// Deploy starts a new deployment.
func (o *Orchestrator) Deploy(ctx context.Context, req *DeployRequest) (*Deployment, error) {
	if req.ProjectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	if req.AgentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	if req.Repository == "" {
		return nil, fmt.Errorf("repository is required")
	}

	// Generate deployment ID
	deploymentID := xid.New().String()

	// Create deployment record
	deployment := &Deployment{
		ID:            deploymentID,
		ProjectID:     req.ProjectID,
		TargetID:      req.TargetID,
		AgentID:       req.AgentID,
		Status:        StatusPending,
		Repository:    req.Repository,
		Branch:        req.Branch,
		Commit:        req.Commit,
		CreatedAt:     time.Now(),
		TriggeredBy:   req.TriggeredBy,
		TriggerSource: req.TriggerSource,
	}

	// Store deployment
	o.deploymentMutex.Lock()
	o.deployments[deploymentID] = deployment
	o.deploymentMutex.Unlock()

	// Notify status change
	o.notifyStatusChange(deploymentID, StatusPending)

	// Build deploy command
	cmd := &DeployCommand{
		DeploymentID:    deploymentID,
		Project:         req.ProjectID,
		Target:          req.TargetID,
		Repository:      req.Repository,
		Branch:          req.Branch,
		Commit:          req.Commit,
		Path:            req.Path,
		Settings:        req.Settings,
		EnvVars:         req.EnvVars,
		EnvFileContent:  req.EnvFileContent,
		PreDeployHooks:  req.PreDeployHooks,
		PostDeployHooks: req.PostDeployHooks,
		ReloadServices:  req.ReloadServices,
	}

	// Send command to agent
	if o.sendCommand != nil {
		o.updateStatus(deploymentID, StatusQueued)
		if err := o.sendCommand(req.AgentID, cmd); err != nil {
			o.updateStatusWithError(deploymentID, StatusFailed, err)
			return deployment, fmt.Errorf("sending deploy command: %w", err)
		}
	}

	return deployment, nil
}

// RollbackRequest contains the information needed to rollback a deployment.
type RollbackRequest struct {
	ProjectID      string
	TargetID       string
	AgentID        string
	Path           string
	ReleaseNumber  int
	RollbackHooks  []string
	ReloadServices []ServiceReload
	TriggeredBy    string
}

// Rollback starts a rollback operation.
func (o *Orchestrator) Rollback(ctx context.Context, req *RollbackRequest) (*Deployment, error) {
	if req.ProjectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	if req.AgentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}

	// Generate deployment ID for the rollback
	deploymentID := xid.New().String()

	// Create deployment record
	deployment := &Deployment{
		ID:            deploymentID,
		ProjectID:     req.ProjectID,
		TargetID:      req.TargetID,
		AgentID:       req.AgentID,
		Status:        StatusRollingBack,
		CreatedAt:     time.Now(),
		TriggeredBy:   req.TriggeredBy,
		TriggerSource: "rollback",
	}

	// Store deployment
	o.deploymentMutex.Lock()
	o.deployments[deploymentID] = deployment
	o.deploymentMutex.Unlock()

	o.notifyStatusChange(deploymentID, StatusRollingBack)

	return deployment, nil
}

// Cancel cancels an in-progress deployment.
func (o *Orchestrator) Cancel(deploymentID, reason string) error {
	o.deploymentMutex.RLock()
	deployment, ok := o.deployments[deploymentID]
	o.deploymentMutex.RUnlock()

	if !ok {
		return fmt.Errorf("deployment %s not found", deploymentID)
	}

	deployment.mutex.Lock()
	if deployment.Status == StatusCompleted || deployment.Status == StatusFailed || deployment.Status == StatusCancelled {
		deployment.mutex.Unlock()
		return fmt.Errorf("deployment %s is already finished", deploymentID)
	}
	deployment.mutex.Unlock()

	o.updateStatusWithError(deploymentID, StatusCancelled, fmt.Errorf("cancelled: %s", reason))
	return nil
}

// GetDeployment returns a deployment by ID.
func (o *Orchestrator) GetDeployment(deploymentID string) (*Deployment, bool) {
	o.deploymentMutex.RLock()
	defer o.deploymentMutex.RUnlock()
	d, ok := o.deployments[deploymentID]
	return d, ok
}

// ListDeployments returns all deployments.
func (o *Orchestrator) ListDeployments() []*Deployment {
	o.deploymentMutex.RLock()
	defer o.deploymentMutex.RUnlock()

	result := make([]*Deployment, 0, len(o.deployments))
	for _, d := range o.deployments {
		result = append(result, d)
	}
	return result
}

// ListProjectDeployments returns deployments for a specific project.
func (o *Orchestrator) ListProjectDeployments(projectID string) []*Deployment {
	o.deploymentMutex.RLock()
	defer o.deploymentMutex.RUnlock()

	var result []*Deployment
	for _, d := range o.deployments {
		if d.ProjectID == projectID {
			result = append(result, d)
		}
	}
	return result
}

// UpdateDeploymentStatus updates a deployment status (called by agent callbacks).
func (o *Orchestrator) UpdateDeploymentStatus(deploymentID string, status DeploymentStatus, progress int, currentStep string) {
	o.deploymentMutex.RLock()
	deployment, ok := o.deployments[deploymentID]
	o.deploymentMutex.RUnlock()

	if !ok {
		return
	}

	deployment.mutex.Lock()
	deployment.Status = status
	deployment.Progress = progress
	deployment.CurrentStep = currentStep

	if status == StatusDeploying && deployment.StartedAt == nil {
		now := time.Now()
		deployment.StartedAt = &now
	}

	if status == StatusCompleted || status == StatusFailed || status == StatusCancelled {
		now := time.Now()
		deployment.CompletedAt = &now
	}
	deployment.mutex.Unlock()

	o.notifyStatusChange(deploymentID, status)
}

// AddDeploymentLog adds a log entry to a deployment.
func (o *Orchestrator) AddDeploymentLog(deploymentID string, entry LogEntry) {
	o.deploymentMutex.RLock()
	deployment, ok := o.deployments[deploymentID]
	o.deploymentMutex.RUnlock()

	if !ok {
		return
	}

	deployment.mutex.Lock()
	deployment.Logs = append(deployment.Logs, entry)
	deployment.mutex.Unlock()

	if o.onLog != nil {
		o.onLog(deploymentID, entry)
	}
}

// SetReleaseNumber sets the release number for a deployment.
func (o *Orchestrator) SetReleaseNumber(deploymentID string, releaseNumber int) {
	o.deploymentMutex.RLock()
	deployment, ok := o.deployments[deploymentID]
	o.deploymentMutex.RUnlock()

	if !ok {
		return
	}

	deployment.mutex.Lock()
	deployment.ReleaseNumber = releaseNumber
	deployment.mutex.Unlock()
}

// SetError sets an error for a deployment.
func (o *Orchestrator) SetError(deploymentID string, err error) {
	o.updateStatusWithError(deploymentID, StatusFailed, err)
}

// updateStatus updates a deployment's status.
func (o *Orchestrator) updateStatus(deploymentID string, status DeploymentStatus) {
	o.deploymentMutex.RLock()
	deployment, ok := o.deployments[deploymentID]
	o.deploymentMutex.RUnlock()

	if !ok {
		return
	}

	deployment.mutex.Lock()
	deployment.Status = status
	deployment.mutex.Unlock()

	o.notifyStatusChange(deploymentID, status)
}

// updateStatusWithError updates a deployment's status and error.
func (o *Orchestrator) updateStatusWithError(deploymentID string, status DeploymentStatus, err error) {
	o.deploymentMutex.RLock()
	deployment, ok := o.deployments[deploymentID]
	o.deploymentMutex.RUnlock()

	if !ok {
		return
	}

	deployment.mutex.Lock()
	deployment.Status = status
	deployment.Error = err
	now := time.Now()
	deployment.CompletedAt = &now
	deployment.mutex.Unlock()

	o.notifyStatusChange(deploymentID, status)
}

// notifyStatusChange calls the status change callback.
func (o *Orchestrator) notifyStatusChange(deploymentID string, status DeploymentStatus) {
	if o.onStatusChange != nil {
		o.onStatusChange(deploymentID, status)
	}
}

// CleanupOldDeployments removes completed deployments older than the given duration.
func (o *Orchestrator) CleanupOldDeployments(maxAge time.Duration) int {
	cutoff := time.Now().Add(-maxAge)
	var toRemove []string

	o.deploymentMutex.RLock()
	for id, d := range o.deployments {
		d.mutex.RLock()
		isFinished := d.Status == StatusCompleted || d.Status == StatusFailed || d.Status == StatusCancelled
		completedAt := d.CompletedAt
		d.mutex.RUnlock()

		if isFinished && completedAt != nil && completedAt.Before(cutoff) {
			toRemove = append(toRemove, id)
		}
	}
	o.deploymentMutex.RUnlock()

	if len(toRemove) > 0 {
		o.deploymentMutex.Lock()
		for _, id := range toRemove {
			delete(o.deployments, id)
		}
		o.deploymentMutex.Unlock()
	}

	return len(toRemove)
}
