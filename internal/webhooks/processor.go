// Package webhooks provides webhook handling for Git providers.
package webhooks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

// DeploymentOrchestrator is the interface for triggering deployments.
type DeploymentOrchestrator interface {
	TriggerDeploy(ctx context.Context, projectID, branch, commit, triggeredBy, triggerSource string) (string, error)
}

// ProjectStore retrieves project information.
type ProjectStore interface {
	GetProjectWebhookConfig(projectID string) (*WebhookConfig, error)
	GetProjectByID(ctx context.Context, projectID string) (*Project, error)
}

// WebhookConfig contains per-project webhook configuration.
type WebhookConfig struct {
	Enabled             bool
	AutoDeploy          bool
	AutoDeployBranches  []string
	AutoDeployOnTag     bool
	AutoDeployOnPR      bool
	PRDeployEnvironment string
}

// Project represents a project.
type Project struct {
	ID         string
	Name       string
	Repository string
	Branch     string
	Targets    []string
}

// EventProcessorConfig configures the event processor.
type EventProcessorConfig struct {
	Logger       *zap.Logger
	Projects     ProjectStore
	Orchestrator DeploymentOrchestrator
	// DefaultBranch is the default branch to deploy when not specified
	DefaultBranch string
}

// DefaultEventProcessor implements EventProcessor.
type DefaultEventProcessor struct {
	logger        *zap.Logger
	projects      ProjectStore
	orchestrator  DeploymentOrchestrator
	defaultBranch string
}

// NewEventProcessor creates a new event processor.
func NewEventProcessor(cfg EventProcessorConfig) *DefaultEventProcessor {
	defaultBranch := cfg.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	return &DefaultEventProcessor{
		logger:        cfg.Logger,
		projects:      cfg.Projects,
		orchestrator:  cfg.Orchestrator,
		defaultBranch: defaultBranch,
	}
}

// ProcessPush handles push events.
func (p *DefaultEventProcessor) ProcessPush(event *PushEvent) error {
	p.logger.Info("Processing push event",
		zap.String("project", event.ProjectID),
		zap.String("branch", event.Branch),
		zap.String("commit", event.Commit),
		zap.String("provider", event.Provider),
	)

	// Skip deleted branches
	if event.Deleted {
		p.logger.Info("Skipping deleted branch", zap.String("branch", event.Branch))
		return nil
	}

	// Get project configuration
	cfg, err := p.projects.GetProjectWebhookConfig(event.ProjectID)
	if err != nil {
		return fmt.Errorf("getting webhook config: %w", err)
	}

	if cfg == nil || !cfg.Enabled || !cfg.AutoDeploy {
		p.logger.Debug("Auto-deploy disabled for project", zap.String("project", event.ProjectID))
		return nil
	}

	// Check if branch should auto-deploy
	if !p.shouldDeployBranch(event.Branch, cfg.AutoDeployBranches) {
		p.logger.Debug("Branch not configured for auto-deploy",
			zap.String("branch", event.Branch),
			zap.Strings("allowed", cfg.AutoDeployBranches),
		)
		return nil
	}

	// Trigger deployment
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	triggeredBy := fmt.Sprintf("webhook:%s", event.Provider)
	triggerSource := fmt.Sprintf("push by %s: %s", event.Author, truncateMessage(event.Message, 50))

	deploymentID, err := p.orchestrator.TriggerDeploy(ctx, event.ProjectID, event.Branch, event.Commit, triggeredBy, triggerSource)
	if err != nil {
		return fmt.Errorf("triggering deployment: %w", err)
	}

	p.logger.Info("Triggered deployment from push",
		zap.String("deployment_id", deploymentID),
		zap.String("project", event.ProjectID),
		zap.String("branch", event.Branch),
		zap.String("commit", event.Commit),
	)

	return nil
}

// ProcessPullRequest handles pull request events.
func (p *DefaultEventProcessor) ProcessPullRequest(event *PullRequestEvent) error {
	p.logger.Info("Processing pull request event",
		zap.String("project", event.ProjectID),
		zap.String("action", event.Action),
		zap.Int("number", event.Number),
		zap.String("provider", event.Provider),
	)

	// Get project configuration
	cfg, err := p.projects.GetProjectWebhookConfig(event.ProjectID)
	if err != nil {
		return fmt.Errorf("getting webhook config: %w", err)
	}

	if cfg == nil || !cfg.Enabled || !cfg.AutoDeployOnPR {
		p.logger.Debug("PR deploy disabled for project", zap.String("project", event.ProjectID))
		return nil
	}

	// Only deploy on certain actions
	switch event.Action {
	case "opened", "synchronize", "reopened":
		// Continue to deploy
	case "closed", "merged":
		p.logger.Debug("PR closed/merged, no deploy needed")
		return nil
	default:
		p.logger.Debug("Ignoring PR action", zap.String("action", event.Action))
		return nil
	}

	// Trigger deployment to PR environment
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	triggeredBy := fmt.Sprintf("webhook:%s", event.Provider)
	triggerSource := fmt.Sprintf("PR #%d: %s by %s", event.Number, event.Title, event.Author)

	// Use source branch for PR deployments
	deploymentID, err := p.orchestrator.TriggerDeploy(ctx, event.ProjectID, event.SourceBranch, "", triggeredBy, triggerSource)
	if err != nil {
		return fmt.Errorf("triggering deployment: %w", err)
	}

	p.logger.Info("Triggered deployment from PR",
		zap.String("deployment_id", deploymentID),
		zap.String("project", event.ProjectID),
		zap.Int("pr_number", event.Number),
	)

	return nil
}

// ProcessTag handles tag events.
func (p *DefaultEventProcessor) ProcessTag(event *TagEvent) error {
	p.logger.Info("Processing tag event",
		zap.String("project", event.ProjectID),
		zap.String("tag", event.Tag),
		zap.Bool("deleted", event.Deleted),
		zap.String("provider", event.Provider),
	)

	// Skip deleted tags
	if event.Deleted {
		p.logger.Info("Skipping deleted tag", zap.String("tag", event.Tag))
		return nil
	}

	// Get project configuration
	cfg, err := p.projects.GetProjectWebhookConfig(event.ProjectID)
	if err != nil {
		return fmt.Errorf("getting webhook config: %w", err)
	}

	if cfg == nil || !cfg.Enabled || !cfg.AutoDeployOnTag {
		p.logger.Debug("Tag deploy disabled for project", zap.String("project", event.ProjectID))
		return nil
	}

	// Trigger deployment
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	triggeredBy := fmt.Sprintf("webhook:%s", event.Provider)
	triggerSource := fmt.Sprintf("tag %s by %s", event.Tag, event.Author)

	// Use tag as branch name (or resolve to commit)
	branch := fmt.Sprintf("refs/tags/%s", event.Tag)
	commit := event.Commit

	deploymentID, err := p.orchestrator.TriggerDeploy(ctx, event.ProjectID, branch, commit, triggeredBy, triggerSource)
	if err != nil {
		return fmt.Errorf("triggering deployment: %w", err)
	}

	p.logger.Info("Triggered deployment from tag",
		zap.String("deployment_id", deploymentID),
		zap.String("project", event.ProjectID),
		zap.String("tag", event.Tag),
	)

	return nil
}

// shouldDeployBranch checks if a branch should trigger auto-deploy.
func (p *DefaultEventProcessor) shouldDeployBranch(branch string, allowedBranches []string) bool {
	// If no branches configured, deploy only the default branch
	if len(allowedBranches) == 0 {
		return branch == p.defaultBranch || branch == "master"
	}

	for _, allowed := range allowedBranches {
		// Support wildcard patterns
		if strings.HasSuffix(allowed, "*") {
			prefix := strings.TrimSuffix(allowed, "*")
			if strings.HasPrefix(branch, prefix) {
				return true
			}
		} else if allowed == branch {
			return true
		}
	}

	return false
}

// truncateMessage truncates a string to the specified length.
func truncateMessage(s string, maxLen int) string {
	// Remove newlines first
	s = strings.Split(s, "\n")[0]
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}
