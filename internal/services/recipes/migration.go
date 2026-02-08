// Package recipes provides YAML to playbook migration tools.
package recipes

import (
	"context"
	"fmt"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// MigrationService handles YAML to playbook migration.
type MigrationService struct {
	store            storage.Store
	componentService *ComponentService
	playbookService  *PlaybookService
	activationSvc    *ActivationService
}

// NewMigrationService creates a new migration service.
func NewMigrationService(store storage.Store) *MigrationService {
	return &MigrationService{
		store:            store,
		componentService: NewComponentService(store),
		playbookService:  NewPlaybookService(store),
		activationSvc:    NewActivationService(store),
	}
}

// MigrationOptions configures the migration process.
type MigrationOptions struct {
	// CreateComponents creates individual components from hooks
	CreateComponents bool
	// PlaybookName is the name for the generated playbook
	PlaybookName string
	// PlaybookVersion is the version for the generated playbook
	PlaybookVersion string
	// ActivatePlaybook automatically activates the playbook for the project
	ActivatePlaybook bool
	// UserID is the user performing the migration (for audit)
	UserID *string
}

// MigrationResult contains the results of a migration.
type MigrationResult struct {
	// Components created during migration
	Components []*storage.RecipeComponent
	// Playbook created during migration
	Playbook *storage.Playbook
	// Activation created if ActivatePlaybook was true
	Activation *storage.PlaybookActivation
	// Warnings about aspects that couldn't be migrated
	Warnings []string
}

// MigrateProjectConfig migrates a project YAML configuration to a playbook.
func (s *MigrationService) MigrateProjectConfig(ctx context.Context, projectID string, cfg *config.ProjectConfig, opts MigrationOptions) (*MigrationResult, error) {
	result := &MigrationResult{
		Warnings: []string{},
	}

	// Set defaults
	if opts.PlaybookName == "" {
		opts.PlaybookName = fmt.Sprintf("%s-playbook", cfg.Name)
	}
	if opts.PlaybookVersion == "" {
		opts.PlaybookVersion = "v1.0.0"
	}

	// Create components from hooks if requested
	var steps []storage.PlaybookStep
	stepOrder := 1

	if opts.CreateComponents {
		// Pre-deploy hooks -> components
		for i, hook := range cfg.Hooks.PreDeploy {
			comp, err := s.createComponentFromHook(ctx, cfg.Name, "pre_deploy", i, hook)
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to create pre-deploy component: %v", err))
				continue
			}
			result.Components = append(result.Components, comp)

			steps = append(steps, storage.PlaybookStep{
				Order:        stepOrder,
				ComponentRef: fmt.Sprintf("user:%s:v1.0.0", comp.Slug),
				Phase:        "pre_deploy",
			})
			stepOrder++
		}

		// Post-deploy hooks -> components
		for i, hook := range cfg.Hooks.PostDeploy {
			comp, err := s.createComponentFromHook(ctx, cfg.Name, "post_deploy", i, hook)
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to create post-deploy component: %v", err))
				continue
			}
			result.Components = append(result.Components, comp)

			steps = append(steps, storage.PlaybookStep{
				Order:        stepOrder,
				ComponentRef: fmt.Sprintf("user:%s:v1.0.0", comp.Slug),
				Phase:        "post_deploy",
			})
			stepOrder++
		}

		// Service reloads -> components
		for i, reload := range cfg.Hooks.Reload {
			comp, err := s.createComponentFromReload(ctx, cfg.Name, i, reload)
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to create reload component: %v", err))
				continue
			}
			result.Components = append(result.Components, comp)

			steps = append(steps, storage.PlaybookStep{
				Order:        stepOrder,
				ComponentRef: fmt.Sprintf("user:%s:v1.0.0", comp.Slug),
				Phase:        "post_deploy",
			})
			stepOrder++
		}

		// Rollback hooks -> components
		for i, hook := range cfg.Hooks.Rollback {
			comp, err := s.createComponentFromHook(ctx, cfg.Name, "rollback", i, hook)
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to create rollback component: %v", err))
				continue
			}
			result.Components = append(result.Components, comp)

			steps = append(steps, storage.PlaybookStep{
				Order:        stepOrder,
				ComponentRef: fmt.Sprintf("user:%s:v1.0.0", comp.Slug),
				Phase:        "rollback",
			})
			stepOrder++
		}
	} else {
		// Create inline steps without components
		for i, hook := range cfg.Hooks.PreDeploy {
			steps = append(steps, storage.PlaybookStep{
				Order:        stepOrder,
				ComponentRef: fmt.Sprintf("inline:pre-deploy-%d:v1.0.0", i),
				Phase:        "pre_deploy",
				VariableBindings: map[string]string{
					"command": hook,
				},
			})
			stepOrder++
		}
		// Note: This approach requires inline component support
		result.Warnings = append(result.Warnings, "Inline steps created without component storage")
	}

	// Warn about non-migratable features
	if cfg.Watch.Guards.RequireCIPass {
		result.Warnings = append(result.Warnings, "CI requirement guard cannot be migrated to playbook")
	}
	if cfg.Health.URL != "" {
		result.Warnings = append(result.Warnings, "Health checks should be configured separately")
	}
	if len(cfg.Env.RequiredKeys) > 0 {
		result.Warnings = append(result.Warnings, "Environment variable requirements should be added as playbook variables")
	}

	// Create playbook
	playbook := &storage.Playbook{
		Namespace:     "user",
		Slug:          slugify(opts.PlaybookName),
		Version:       opts.PlaybookVersion,
		Name:          opts.PlaybookName,
		Description:   fmt.Sprintf("Migrated from YAML configuration for project %s", cfg.Name),
		FrameworkType: cfg.Type,
		Steps:         steps,
		CreatedAt:     time.Now(),
	}

	if err := s.store.CreatePlaybook(ctx, playbook); err != nil {
		return nil, fmt.Errorf("failed to create playbook: %w", err)
	}
	result.Playbook = playbook

	// Activate playbook if requested
	if opts.ActivatePlaybook && projectID != "" {
		activation, err := s.activationSvc.Activate(ctx, projectID, playbook.ID, nil, opts.UserID)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to activate playbook: %v", err))
		} else {
			result.Activation = activation
		}
	}

	return result, nil
}

// createComponentFromHook creates a component from a shell hook.
func (s *MigrationService) createComponentFromHook(ctx context.Context, projectName, phase string, index int, command string) (*storage.RecipeComponent, error) {
	slug := fmt.Sprintf("%s-%s-%d", slugify(projectName), phase, index)

	component := &storage.RecipeComponent{
		Namespace:     "user",
		Slug:          slug,
		Version:       "v1.0.0",
		Name:          fmt.Sprintf("%s %s hook %d", projectName, phase, index+1),
		Description:   fmt.Sprintf("Migrated from YAML %s hook", phase),
		ComponentType: "hook",
		Content: storage.ComponentContent{
			Commands: []string{command},
		},
		IsRaw:     isRawCommand(command),
		CreatedAt: time.Now(),
	}

	if err := s.componentService.Create(ctx, component); err != nil {
		return nil, err
	}

	return component, nil
}

// createComponentFromReload creates a component from a service reload action.
func (s *MigrationService) createComponentFromReload(ctx context.Context, projectName string, index int, reload config.ServiceAction) (*storage.RecipeComponent, error) {
	slug := fmt.Sprintf("%s-reload-%d", slugify(projectName), index)

	component := &storage.RecipeComponent{
		Namespace:     "user",
		Slug:          slug,
		Version:       "v1.0.0",
		Name:          fmt.Sprintf("%s service %s", reload.Service, reload.Action),
		Description:   fmt.Sprintf("Migrated from YAML reload hook for %s", reload.Service),
		ComponentType: "service_reload",
		Content: storage.ComponentContent{
			ServiceName:   reload.Service,
			ServiceAction: reload.Action,
		},
		CreatedAt: time.Now(),
	}

	if err := s.componentService.Create(ctx, component); err != nil {
		return nil, err
	}

	return component, nil
}

// MigrateFromYAMLFile reads a project YAML file and migrates it.
func (s *MigrationService) MigrateFromYAMLFile(ctx context.Context, projectID string, yamlPath string, opts MigrationOptions) (*MigrationResult, error) {
	cfg, err := config.LoadProjectConfig(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load YAML: %w", err)
	}

	return s.MigrateProjectConfig(ctx, projectID, cfg, opts)
}

// PreviewMigration returns a preview of what would be migrated without making changes.
func (s *MigrationService) PreviewMigration(cfg *config.ProjectConfig) *MigrationPreview {
	preview := &MigrationPreview{
		ProjectName:     cfg.Name,
		ProjectType:     cfg.Type,
		PreDeployHooks:  len(cfg.Hooks.PreDeploy),
		PostDeployHooks: len(cfg.Hooks.PostDeploy),
		ReloadActions:   len(cfg.Hooks.Reload),
		RollbackHooks:   len(cfg.Hooks.Rollback),
		TotalComponents: 0,
		HasRawCommands:  false,
		Warnings:        []string{},
	}

	// Count total components
	preview.TotalComponents = preview.PreDeployHooks + preview.PostDeployHooks + preview.ReloadActions + preview.RollbackHooks

	// Check for RAW commands
	for _, hook := range cfg.Hooks.PreDeploy {
		if isRawCommand(hook) {
			preview.HasRawCommands = true
			break
		}
	}
	for _, hook := range cfg.Hooks.PostDeploy {
		if isRawCommand(hook) {
			preview.HasRawCommands = true
			break
		}
	}
	for _, hook := range cfg.Hooks.Rollback {
		if isRawCommand(hook) {
			preview.HasRawCommands = true
			break
		}
	}

	// Collect warnings
	if cfg.Watch.Guards.RequireCIPass {
		preview.Warnings = append(preview.Warnings, "CI guard cannot be migrated")
	}
	if cfg.Health.URL != "" {
		preview.Warnings = append(preview.Warnings, "Health checks require separate configuration")
	}
	if len(cfg.Env.RequiredKeys) > 0 {
		preview.Warnings = append(preview.Warnings, fmt.Sprintf("%d environment variables should be defined as playbook variables", len(cfg.Env.RequiredKeys)))
	}
	if preview.HasRawCommands {
		preview.Warnings = append(preview.Warnings, "RAW commands detected - will require admin approval")
	}

	return preview
}

// MigrationPreview shows what would be migrated.
type MigrationPreview struct {
	ProjectName     string
	ProjectType     string
	PreDeployHooks  int
	PostDeployHooks int
	ReloadActions   int
	RollbackHooks   int
	TotalComponents int
	HasRawCommands  bool
	Warnings        []string
}

// slugify converts a name to a slug format.
func slugify(name string) string {
	// Simple slugification - replace spaces with dashes, lowercase
	result := ""
	for _, c := range name {
		if c >= 'a' && c <= 'z' {
			result += string(c)
		} else if c >= 'A' && c <= 'Z' {
			result += string(c + 32) // lowercase
		} else if c >= '0' && c <= '9' {
			result += string(c)
		} else if c == ' ' || c == '_' || c == '-' {
			if result != "" && result[len(result)-1] != '-' {
				result += "-"
			}
		}
	}
	// Trim trailing dash
	for result != "" && result[len(result)-1] == '-' {
		result = result[:len(result)-1]
	}
	return result
}

// isRawCommand checks if a command contains potentially dangerous patterns.
func isRawCommand(cmd string) bool {
	// Commands with shell redirections, pipes, or variable expansions might be RAW
	dangerousPatterns := []string{
		"$(", "`", "&&", "||", "|", ">", "<", ";",
		"rm -rf", "sudo ", "chmod 777", "eval ",
	}
	for _, pattern := range dangerousPatterns {
		if containsString(cmd, pattern) {
			return true
		}
	}
	return false
}

// containsString is a simple contains check.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || s != "" && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
