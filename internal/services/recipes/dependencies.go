package recipes

import (
	"context"
	"fmt"

	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// DependencyService tracks resource usage in playbooks.
type DependencyService struct {
	store storage.Store
}

// NewDependencyService creates a new dependency service.
func NewDependencyService(store storage.Store) *DependencyService {
	return &DependencyService{store: store}
}

// ResourceUsage describes how a resource is used.
type ResourceUsage struct {
	ProjectID    int64  `json:"project_id"`
	ProjectName  string `json:"project_name"`
	PlaybookID   int64  `json:"playbook_id"`
	PlaybookName string `json:"playbook_name"`
	VariableName string `json:"variable_name"`
}

// FindSecretUsages finds all playbook activations using a secret.
func (s *DependencyService) FindSecretUsages(ctx context.Context, secretName string) ([]*ResourceUsage, error) {
	bindings, err := s.store.FindBindingsBySourceRef(ctx, storage.SourceTypeSecret, secretName)
	if err != nil {
		return nil, err
	}

	return s.bindingsToUsages(ctx, bindings)
}

// FindEnvUsages finds all playbook activations using an env variable.
func (s *DependencyService) FindEnvUsages(ctx context.Context, envKey string) ([]*ResourceUsage, error) {
	bindings, err := s.store.FindBindingsBySourceRef(ctx, storage.SourceTypeEnv, envKey)
	if err != nil {
		return nil, err
	}

	return s.bindingsToUsages(ctx, bindings)
}

// FindAllUsages finds all usages of a resource by type and reference.
func (s *DependencyService) FindAllUsages(ctx context.Context, sourceType, sourceRef string) ([]*ResourceUsage, error) {
	bindings, err := s.store.FindBindingsBySourceRef(ctx, sourceType, sourceRef)
	if err != nil {
		return nil, err
	}

	return s.bindingsToUsages(ctx, bindings)
}

func (s *DependencyService) bindingsToUsages(ctx context.Context, bindings []*storage.PlaybookVariableBinding) ([]*ResourceUsage, error) {
	var usages []*ResourceUsage

	for _, b := range bindings {
		activation, err := s.store.GetPlaybookActivationByID(ctx, b.ActivationID)
		if err != nil {
			continue
		}

		playbook, err := s.store.GetPlaybookByID(ctx, activation.PlaybookID)
		if err != nil {
			continue
		}

		// Get project name - try to load from store if available
		projectName := s.getProjectName(ctx, activation.ProjectID)

		usages = append(usages, &ResourceUsage{
			ProjectID:    activation.ProjectID,
			ProjectName:  projectName,
			PlaybookID:   activation.PlaybookID,
			PlaybookName: playbook.Name,
			VariableName: b.VariableName,
		})
	}

	return usages, nil
}

func (s *DependencyService) getProjectName(ctx context.Context, projectID int64) string {
	// Try to get project from store
	project, err := s.store.GetProjectByID(ctx, projectID)
	if err != nil || project == nil {
		return fmt.Sprintf("Project %d", projectID)
	}
	return project.Name
}

// CheckDeletionSafe checks if a resource can be safely deleted.
func (s *DependencyService) CheckDeletionSafe(ctx context.Context, resourceType, resourceName string) error {
	var usages []*ResourceUsage
	var err error

	switch resourceType {
	case storage.SourceTypeSecret:
		usages, err = s.FindSecretUsages(ctx, resourceName)
	case storage.SourceTypeEnv:
		usages, err = s.FindEnvUsages(ctx, resourceName)
	default:
		return fmt.Errorf("unknown resource type: %s", resourceType)
	}

	if err != nil {
		return fmt.Errorf("failed to check usages: %w", err)
	}

	if len(usages) > 0 {
		return &DeletionBlockedError{
			ResourceType: resourceType,
			ResourceName: resourceName,
			Usages:       usages,
		}
	}

	return nil
}

// CheckSecretDeletionSafe is a convenience method for checking secret deletion.
func (s *DependencyService) CheckSecretDeletionSafe(ctx context.Context, secretName string) error {
	return s.CheckDeletionSafe(ctx, "secret", secretName)
}

// CheckEnvDeletionSafe is a convenience method for checking env variable deletion.
func (s *DependencyService) CheckEnvDeletionSafe(ctx context.Context, envKey string) error {
	return s.CheckDeletionSafe(ctx, "env", envKey)
}

// DeletionBlockedError indicates a resource cannot be deleted due to usage.
type DeletionBlockedError struct {
	ResourceType string
	ResourceName string
	Usages       []*ResourceUsage
}

func (e *DeletionBlockedError) Error() string {
	return fmt.Sprintf("cannot delete %s %q: used by %d playbook(s)", e.ResourceType, e.ResourceName, len(e.Usages))
}

// IsDeletionBlockedError checks if an error is a DeletionBlockedError.
func IsDeletionBlockedError(err error) bool {
	_, ok := err.(*DeletionBlockedError)
	return ok
}

// GetDeletionBlockedError extracts the DeletionBlockedError from an error.
func GetDeletionBlockedError(err error) *DeletionBlockedError {
	if e, ok := err.(*DeletionBlockedError); ok {
		return e
	}
	return nil
}

// GetUsageDetails returns a formatted description of resource usages.
func (e *DeletionBlockedError) GetUsageDetails() []string {
	var details []string
	for _, u := range e.Usages {
		details = append(details, fmt.Sprintf("Project %q uses variable %q in playbook %q",
			u.ProjectName, u.VariableName, u.PlaybookName))
	}
	return details
}
