// Package recipes provides the playbook resolver for deployment integration.
package recipes

import (
	"context"
	"fmt"
	"strings"

	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// PlaybookResolver resolves activated playbooks into deployment steps.
type PlaybookResolver struct {
	store            storage.Store
	componentService *ComponentService
	activationSvc    *ActivationService
}

// NewPlaybookResolver creates a new playbook resolver.
func NewPlaybookResolver(store storage.Store) *PlaybookResolver {
	return &PlaybookResolver{
		store:            store,
		componentService: NewComponentService(store),
		activationSvc:    NewActivationService(store),
	}
}

// ResolvedStep represents a deployment step with resolved component content.
type ResolvedStep struct {
	Order          int
	Phase          string // pre_deploy, deploy, post_deploy, rollback
	ComponentRef   string
	ComponentName  string
	ComponentType  string // shell, template, file_sync
	IsRaw          bool
	Content        string            // Resolved template/script content
	Variables      map[string]string // Resolved variable values
	Condition      string
	TargetPath     string // For file_sync components
	TemplateSource string // For template components
}

// ResolvedPlaybook contains the fully resolved deployment steps.
type ResolvedPlaybook struct {
	PlaybookID      string
	PlaybookName    string
	PlaybookVersion string
	ProjectID       string
	PreDeploySteps  []*ResolvedStep
	DeploySteps     []*ResolvedStep
	PostDeploySteps []*ResolvedStep
	RollbackSteps   []*ResolvedStep
	SharedDirs      []string
}

// DeployHooks contains the resolved hooks for traditional deployment.
type DeployHooks struct {
	PreDeployHooks  []string
	PostDeployHooks []string
	RollbackHooks   []string
}

// Resolve resolves an activated playbook for deployment.
func (r *PlaybookResolver) Resolve(ctx context.Context, projectID string, envGetter func(string) string, secretGetter func(context.Context, string) (string, error)) (*ResolvedPlaybook, error) {
	// Get activation
	activation, playbook, err := r.activationSvc.GetActiveWithPlaybook(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("no active playbook: %w", err)
	}

	// Resolve variables
	resolvedVars, err := r.activationSvc.ResolveVariables(ctx, activation.ID, envGetter, secretGetter)
	if err != nil {
		return nil, fmt.Errorf("resolving variables: %w", err)
	}

	resolved := &ResolvedPlaybook{
		PlaybookID:      playbook.ID,
		PlaybookName:    playbook.Name,
		PlaybookVersion: playbook.Version,
		ProjectID:       projectID,
		SharedDirs:      playbook.SharedDirs,
	}

	// Resolve each step
	for _, step := range playbook.Steps {
		resolvedStep, err := r.resolveStep(ctx, step, resolvedVars)
		if err != nil {
			return nil, fmt.Errorf("resolving step %d: %w", step.Order, err)
		}

		switch step.Phase {
		case "pre_deploy":
			resolved.PreDeploySteps = append(resolved.PreDeploySteps, resolvedStep)
		case "deploy":
			resolved.DeploySteps = append(resolved.DeploySteps, resolvedStep)
		case "post_deploy":
			resolved.PostDeploySteps = append(resolved.PostDeploySteps, resolvedStep)
		case "rollback":
			resolved.RollbackSteps = append(resolved.RollbackSteps, resolvedStep)
		}
	}

	return resolved, nil
}

// resolveStep resolves a single playbook step.
func (r *PlaybookResolver) resolveStep(ctx context.Context, step storage.PlaybookStep, vars map[string]string) (*ResolvedStep, error) {
	// Parse component reference: namespace:slug:version
	parts := strings.Split(step.ComponentRef, ":")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid component reference: %s", step.ComponentRef)
	}

	namespace, slug, version := parts[0], parts[1], parts[2]

	// Get component
	component, err := r.componentService.Get(ctx, namespace, slug, version)
	if err != nil {
		return nil, fmt.Errorf("component not found: %s", step.ComponentRef)
	}

	resolved := &ResolvedStep{
		Order:         step.Order,
		Phase:         step.Phase,
		ComponentRef:  step.ComponentRef,
		ComponentName: component.Name,
		ComponentType: component.ComponentType,
		IsRaw:         component.IsRaw,
		Condition:     step.Condition,
		Variables:     make(map[string]string),
	}

	// Apply step variable overrides
	for k, v := range vars {
		resolved.Variables[k] = v
	}
	for k, v := range step.VariableBindings {
		resolved.Variables[k] = v
	}

	// Resolve content with variables
	// Convert commands to a single script content
	var contentLines []string
	for _, cmd := range component.Content.Commands {
		resolvedCmd := cmd
		for varName, varValue := range resolved.Variables {
			// Replace {{variable}} style placeholders
			placeholder := "{{" + varName + "}}"
			resolvedCmd = strings.ReplaceAll(resolvedCmd, placeholder, varValue)
			// Also replace ${variable} style
			placeholder = "${" + varName + "}"
			resolvedCmd = strings.ReplaceAll(resolvedCmd, placeholder, varValue)
		}
		contentLines = append(contentLines, resolvedCmd)
	}
	resolved.Content = strings.Join(contentLines, "\n")

	// Set component-specific fields
	if component.ComponentType == "file_op" && component.Content.DestPath != "" {
		resolved.TargetPath = component.Content.DestPath
	}

	return resolved, nil
}

// HasActivePlaybook checks if a project has an active playbook.
func (r *PlaybookResolver) HasActivePlaybook(ctx context.Context, projectID string) bool {
	activation, err := r.activationSvc.GetActive(ctx, projectID)
	return err == nil && activation != nil
}

// ToDeployHooks converts resolved playbook to traditional hook format.
// This is a compatibility layer for the existing deployment system.
func (r *PlaybookResolver) ToDeployHooks(resolved *ResolvedPlaybook) *DeployHooks {
	hooks := &DeployHooks{}

	// Convert pre-deploy steps to hooks
	for _, step := range resolved.PreDeploySteps {
		if step.ComponentType == "shell" {
			hooks.PreDeployHooks = append(hooks.PreDeployHooks, step.Content)
		}
	}

	// Convert post-deploy steps to hooks
	for _, step := range resolved.PostDeploySteps {
		if step.ComponentType == "shell" {
			hooks.PostDeployHooks = append(hooks.PostDeployHooks, step.Content)
		}
	}

	// Convert rollback steps to hooks
	for _, step := range resolved.RollbackSteps {
		if step.ComponentType == "shell" {
			hooks.RollbackHooks = append(hooks.RollbackHooks, step.Content)
		}
	}

	return hooks
}

// GetSharedDirs returns shared directories from the playbook.
func (r *PlaybookResolver) GetSharedDirs(resolved *ResolvedPlaybook) []string {
	return resolved.SharedDirs
}

// ValidateForDeployment validates that a playbook is ready for deployment.
func (r *PlaybookResolver) ValidateForDeployment(ctx context.Context, projectID string) error {
	activation, err := r.activationSvc.GetActive(ctx, projectID)
	if err != nil {
		return fmt.Errorf("no active playbook")
	}

	// Check for RAW components
	playbook, err := r.store.GetPlaybookByID(ctx, activation.PlaybookID)
	if err != nil {
		return fmt.Errorf("playbook not found")
	}

	for _, step := range playbook.Steps {
		parts := strings.Split(step.ComponentRef, ":")
		if len(parts) != 3 {
			continue
		}

		component, err := r.componentService.Get(ctx, parts[0], parts[1], parts[2])
		if err != nil {
			return fmt.Errorf("component not found: %s", step.ComponentRef)
		}

		if component.IsRaw {
			// Check RAW approval
			approval, err := r.store.GetRawApproval(ctx, component.ID)
			if err != nil || approval == nil {
				return fmt.Errorf("RAW component %s requires admin approval", component.Name)
			}
		}
	}

	return nil
}

// DeploymentConfig holds additional configuration needed for deployment integration.
type DeploymentConfig struct {
	EnvVars        map[string]string
	EnvFileContent []byte
	ReloadServices []ServiceReloadConfig
}

// ServiceReloadConfig defines a service to reload after deployment.
type ServiceReloadConfig struct {
	Service string
	Action  string // reload, restart, start, stop
}

// BuildDeployRequest builds deployment parameters from a resolved playbook.
// This is used by the deployment orchestrator to execute playbook-based deployments.
func (r *PlaybookResolver) BuildDeployRequest(resolved *ResolvedPlaybook, cfg *DeploymentConfig) *DeploymentRequest {
	req := &DeploymentRequest{
		PlaybookID:      resolved.PlaybookID,
		PlaybookName:    resolved.PlaybookName,
		PlaybookVersion: resolved.PlaybookVersion,
		SharedDirs:      resolved.SharedDirs,
	}

	// Convert pre-deploy steps to hooks
	for _, step := range resolved.PreDeploySteps {
		if step.Content != "" {
			req.PreDeployHooks = append(req.PreDeployHooks, step.Content)
		}
	}

	// Convert post-deploy steps to hooks
	for _, step := range resolved.PostDeploySteps {
		if step.Content != "" {
			req.PostDeployHooks = append(req.PostDeployHooks, step.Content)
		}
	}

	// Convert rollback steps to hooks
	for _, step := range resolved.RollbackSteps {
		if step.Content != "" {
			req.RollbackHooks = append(req.RollbackHooks, step.Content)
		}
	}

	// Add env vars and reload services from config
	if cfg != nil {
		req.EnvVars = cfg.EnvVars
		req.EnvFileContent = cfg.EnvFileContent
		for _, svc := range cfg.ReloadServices {
			req.ReloadServices = append(req.ReloadServices, ServiceReloadConfig{
				Service: svc.Service,
				Action:  svc.Action,
			})
		}
	}

	return req
}

// DeploymentRequest contains the resolved deployment parameters.
type DeploymentRequest struct {
	PlaybookID      string
	PlaybookName    string
	PlaybookVersion string
	PreDeployHooks  []string
	PostDeployHooks []string
	RollbackHooks   []string
	SharedDirs      []string
	EnvVars         map[string]string
	EnvFileContent  []byte
	ReloadServices  []ServiceReloadConfig
}
