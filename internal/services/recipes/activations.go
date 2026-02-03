package recipes

import (
	"context"
	"fmt"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// ActivationService handles playbook activation for projects.
type ActivationService struct {
	store            storage.Store
	playbookService  *PlaybookService
	componentService *ComponentService
}

// NewActivationService creates a new activation service.
func NewActivationService(store storage.Store) *ActivationService {
	return &ActivationService{
		store:            store,
		playbookService:  NewPlaybookService(store),
		componentService: NewComponentService(store),
	}
}

// VariableBinding represents a variable value source.
type VariableBinding struct {
	SourceType   string // literal, env, secret
	SourceRef    string // for env/secret
	LiteralValue string // for literal
}

// Activate activates a playbook for a project.
func (s *ActivationService) Activate(ctx context.Context, projectID, playbookID int64, bindings map[string]VariableBinding, userID *int64) (*storage.PlaybookActivation, error) {
	// Validate playbook exists
	playbook, err := s.store.GetPlaybookByID(ctx, playbookID)
	if err != nil {
		return nil, fmt.Errorf("playbook not found: %w", err)
	}

	// Validate all bindings satisfy requirements
	if err := s.validateBindings(ctx, playbook, bindings); err != nil {
		return nil, err
	}

	// Check RAW component approvals
	if err := s.checkRawApprovals(ctx, playbook); err != nil {
		return nil, err
	}

	// Delete existing activation if any
	existing, _ := s.store.GetPlaybookActivation(ctx, projectID)
	if existing != nil {
		if err := s.store.DeletePlaybookActivation(ctx, existing.ID); err != nil {
			return nil, fmt.Errorf("failed to remove existing activation: %w", err)
		}
	}

	// Create new activation
	activation := &storage.PlaybookActivation{
		ProjectID:   projectID,
		PlaybookID:  playbookID,
		ActivatedAt: time.Now(),
		ActivatedBy: userID,
	}

	if err := s.store.CreatePlaybookActivation(ctx, activation); err != nil {
		return nil, err
	}

	// Create variable bindings
	for name, binding := range bindings {
		vb := &storage.PlaybookVariableBinding{
			ActivationID: activation.ID,
			VariableName: name,
			SourceType:   binding.SourceType,
			SourceRef:    binding.SourceRef,
			LiteralValue: binding.LiteralValue,
		}
		if err := s.store.CreateVariableBinding(ctx, vb); err != nil {
			return nil, fmt.Errorf("failed to create binding for %q: %w", name, err)
		}
	}

	return activation, nil
}

// Deactivate removes the playbook activation for a project.
func (s *ActivationService) Deactivate(ctx context.Context, projectID int64) error {
	existing, err := s.store.GetPlaybookActivation(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to get activation: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("no activation found for project %d", projectID)
	}
	return s.store.DeletePlaybookActivation(ctx, existing.ID)
}

// GetActive returns the current activation for a project.
func (s *ActivationService) GetActive(ctx context.Context, projectID int64) (*storage.PlaybookActivation, error) {
	activation, err := s.store.GetPlaybookActivation(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if activation == nil {
		return nil, fmt.Errorf("no activation found for project %d", projectID)
	}

	// Load bindings
	bindings, err := s.store.GetVariableBindings(ctx, activation.ID)
	if err != nil {
		return nil, err
	}
	activation.Bindings = make([]storage.PlaybookVariableBinding, len(bindings))
	for i, b := range bindings {
		activation.Bindings[i] = *b
	}

	return activation, nil
}

// GetActiveWithPlaybook returns the activation along with the associated playbook.
func (s *ActivationService) GetActiveWithPlaybook(ctx context.Context, projectID int64) (*storage.PlaybookActivation, *storage.Playbook, error) {
	activation, err := s.GetActive(ctx, projectID)
	if err != nil {
		return nil, nil, err
	}

	playbook, err := s.store.GetPlaybookByID(ctx, activation.PlaybookID)
	if err != nil {
		return nil, nil, fmt.Errorf("playbook not found: %w", err)
	}

	return activation, playbook, nil
}

// ListByPlaybook returns all activations using a specific playbook.
func (s *ActivationService) ListByPlaybook(ctx context.Context, playbookID int64) ([]*storage.PlaybookActivation, error) {
	return s.store.ListActivationsByPlaybook(ctx, playbookID)
}

// UpdateBindings updates the variable bindings for an activation.
func (s *ActivationService) UpdateBindings(ctx context.Context, activationID int64, bindings map[string]VariableBinding) error {
	activation, err := s.store.GetPlaybookActivationByID(ctx, activationID)
	if err != nil {
		return fmt.Errorf("activation not found: %w", err)
	}

	playbook, err := s.store.GetPlaybookByID(ctx, activation.PlaybookID)
	if err != nil {
		return fmt.Errorf("playbook not found: %w", err)
	}

	// Validate new bindings
	if err := s.validateBindings(ctx, playbook, bindings); err != nil {
		return err
	}

	// Delete existing bindings
	existingBindings, err := s.store.GetVariableBindings(ctx, activationID)
	if err != nil {
		return err
	}
	for _, b := range existingBindings {
		if err := s.store.DeleteVariableBinding(ctx, b.ID); err != nil {
			return fmt.Errorf("failed to delete binding %d: %w", b.ID, err)
		}
	}

	// Create new bindings
	for name, binding := range bindings {
		vb := &storage.PlaybookVariableBinding{
			ActivationID: activationID,
			VariableName: name,
			SourceType:   binding.SourceType,
			SourceRef:    binding.SourceRef,
			LiteralValue: binding.LiteralValue,
		}
		if err := s.store.CreateVariableBinding(ctx, vb); err != nil {
			return fmt.Errorf("failed to create binding for %q: %w", name, err)
		}
	}

	return nil
}

// ResolveVariables resolves all variable bindings to their actual values.
// Returns a map of variable name to resolved value.
func (s *ActivationService) ResolveVariables(ctx context.Context, activationID int64, envGetter func(string) string, secretGetter func(context.Context, string) (string, error)) (map[string]string, error) {
	bindings, err := s.store.GetVariableBindings(ctx, activationID)
	if err != nil {
		return nil, err
	}

	resolved := make(map[string]string)
	for _, b := range bindings {
		switch b.SourceType {
		case storage.SourceTypeLiteral:
			resolved[b.VariableName] = b.LiteralValue
		case storage.SourceTypeEnv:
			if envGetter != nil {
				resolved[b.VariableName] = envGetter(b.SourceRef)
			}
		case storage.SourceTypeSecret:
			if secretGetter != nil {
				value, err := secretGetter(ctx, b.SourceRef)
				if err != nil {
					return nil, fmt.Errorf("failed to resolve secret %q: %w", b.SourceRef, err)
				}
				resolved[b.VariableName] = value
			}
		default:
			return nil, fmt.Errorf("unknown source type: %s", b.SourceType)
		}
	}

	return resolved, nil
}

func (s *ActivationService) validateBindings(ctx context.Context, playbook *storage.Playbook, bindings map[string]VariableBinding) error {
	// Collect all required variables from all steps
	required := make(map[string]bool)

	for _, step := range playbook.Steps {
		namespace, slug, version, err := ParseComponentRef(step.ComponentRef)
		if err != nil {
			continue
		}

		component, err := s.store.GetRecipeComponent(ctx, namespace, slug, version)
		if err != nil {
			continue
		}

		for _, v := range component.Variables {
			if v.Required && v.Default == "" {
				required[v.Name] = true
			}
		}
	}

	// Check all required variables have bindings
	for name := range required {
		if _, ok := bindings[name]; !ok {
			return fmt.Errorf("required variable %q not bound", name)
		}
	}

	// Validate binding source types
	for name, b := range bindings {
		switch b.SourceType {
		case storage.SourceTypeLiteral:
			// OK
		case storage.SourceTypeEnv, storage.SourceTypeSecret:
			if b.SourceRef == "" {
				return fmt.Errorf("variable %q: source reference required for type %s", name, b.SourceType)
			}
		default:
			return fmt.Errorf("variable %q: invalid source type %q", name, b.SourceType)
		}
	}

	return nil
}

func (s *ActivationService) checkRawApprovals(ctx context.Context, playbook *storage.Playbook) error {
	for _, step := range playbook.Steps {
		namespace, slug, version, err := ParseComponentRef(step.ComponentRef)
		if err != nil {
			continue
		}

		component, err := s.store.GetRecipeComponent(ctx, namespace, slug, version)
		if err != nil {
			continue
		}

		if component.IsRaw {
			approval, err := s.store.GetRawApproval(ctx, component.ID)
			if err != nil || approval == nil {
				return fmt.Errorf("RAW component %q requires admin approval before activation", step.ComponentRef)
			}
		}
	}
	return nil
}
