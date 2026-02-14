// Package recipes provides services for recipe components and playbooks.
package recipes

import (
	"context"
	"fmt"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/BlackOrder/vcdeploy/internal/validation"
)

// ComponentService handles recipe component operations.
type ComponentService struct {
	store storage.Store
}

// NewComponentService creates a new component service.
func NewComponentService(store storage.Store) *ComponentService {
	return &ComponentService{store: store}
}

// List returns components filtered by namespace.
func (s *ComponentService) List(ctx context.Context, namespace string, includeDeprecated bool) ([]*storage.RecipeComponent, error) {
	return s.store.ListRecipeComponents(ctx, namespace, includeDeprecated)
}

// Get retrieves a specific component version.
func (s *ComponentService) Get(ctx context.Context, namespace, slug, version string) (*storage.RecipeComponent, error) {
	normalizedVersion := validation.NormalizeSemver(version)
	return s.store.GetRecipeComponent(ctx, namespace, slug, normalizedVersion)
}

// GetByID retrieves a component by its ID.
func (s *ComponentService) GetByID(ctx context.Context, id string) (*storage.RecipeComponent, error) {
	return s.store.GetRecipeComponentByID(ctx, id)
}

// GetLatest retrieves the highest semver version of a component.
func (s *ComponentService) GetLatest(ctx context.Context, namespace, slug string) (*storage.RecipeComponent, error) {
	versions, err := s.store.ListRecipeComponentVersions(ctx, namespace, slug)
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("component not found: %s:%s", namespace, slug)
	}

	// Find latest using semver comparison
	var versionStrings []string
	versionMap := make(map[string]*storage.RecipeComponent)
	for _, v := range versions {
		versionStrings = append(versionStrings, v.Version)
		versionMap[v.Version] = v
	}

	latest := validation.LatestSemver(versionStrings)
	return versionMap[latest], nil
}

// GetVersions returns all versions of a component, sorted by semver descending.
func (s *ComponentService) GetVersions(ctx context.Context, namespace, slug string) ([]*storage.RecipeComponent, error) {
	versions, err := s.store.ListRecipeComponentVersions(ctx, namespace, slug)
	if err != nil {
		return nil, err
	}

	// Sort by semver descending (newest first)
	sortComponentsBySemverDesc(versions)
	return versions, nil
}

// Create creates a new user component.
func (s *ComponentService) Create(ctx context.Context, component *storage.RecipeComponent) error {
	// Validate semver
	if err := validation.ValidateSemver(component.Version); err != nil {
		return fmt.Errorf("invalid version: %w", err)
	}
	component.Version = validation.NormalizeSemver(component.Version)

	// Enforce user namespace for creation
	if component.Namespace != storage.NamespaceUser {
		return fmt.Errorf("can only create components in 'user' namespace")
	}

	component.IsSeed = false
	component.CreatedAt = time.Now()

	return s.store.CreateRecipeComponent(ctx, component)
}

// Update updates an existing user component.
func (s *ComponentService) Update(ctx context.Context, component *storage.RecipeComponent) error {
	existing, err := s.store.GetRecipeComponentByID(ctx, component.ID)
	if err != nil {
		return err
	}
	if existing.IsSeed {
		return fmt.Errorf("cannot update seed components")
	}

	// Validate semver if changed
	if component.Version != existing.Version {
		if err := validation.ValidateSemver(component.Version); err != nil {
			return fmt.Errorf("invalid version: %w", err)
		}
		component.Version = validation.NormalizeSemver(component.Version)
	}

	return s.store.UpdateRecipeComponent(ctx, component)
}

// CreateFromSeed creates a user copy of a seed component (copy-on-write).
func (s *ComponentService) CreateFromSeed(ctx context.Context, seedID string, newSlug, newVersion string) (*storage.RecipeComponent, error) {
	seed, err := s.store.GetRecipeComponentByID(ctx, seedID)
	if err != nil {
		return nil, fmt.Errorf("seed component not found: %w", err)
	}
	if !seed.IsSeed {
		return nil, fmt.Errorf("source component is not a seed")
	}

	// Validate new version
	if err := validation.ValidateSemver(newVersion); err != nil {
		return nil, fmt.Errorf("invalid version: %w", err)
	}

	// Create clone in user namespace
	cloned := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          newSlug,
		Version:       validation.NormalizeSemver(newVersion),
		Name:          seed.Name + " (Custom)",
		Description:   seed.Description,
		ComponentType: seed.ComponentType,
		Content:       seed.Content,
		Variables:     seed.Variables,
		IsSeed:        false,
		IsRaw:         seed.IsRaw,
		IsDeprecated:  false,
		CreatedAt:     time.Now(),
	}

	if err := s.store.CreateRecipeComponent(ctx, cloned); err != nil {
		return nil, err
	}

	return cloned, nil
}

// ValidateVariables checks if provided bindings satisfy required variables.
func (s *ComponentService) ValidateVariables(ctx context.Context, componentID string, bindings map[string]string) error {
	component, err := s.store.GetRecipeComponentByID(ctx, componentID)
	if err != nil {
		return err
	}

	for _, v := range component.Variables {
		if v.Required {
			if _, ok := bindings[v.Name]; !ok {
				if v.Default == "" {
					return fmt.Errorf("required variable %q not provided", v.Name)
				}
			}
		}
	}

	return nil
}

// Delete removes a user component.
func (s *ComponentService) Delete(ctx context.Context, id string) error {
	component, err := s.store.GetRecipeComponentByID(ctx, id)
	if err != nil {
		return err
	}
	if component.IsSeed {
		return fmt.Errorf("cannot delete seed components")
	}
	return s.store.DeleteRecipeComponent(ctx, id)
}

// sortComponentsBySemverDesc sorts components by semver in descending order (newest first).
func sortComponentsBySemverDesc(components []*storage.RecipeComponent) {
	// Simple bubble sort for small lists - adequate for version lists
	for i := 0; i < len(components)-1; i++ {
		for j := 0; j < len(components)-i-1; j++ {
			if validation.CompareSemver(components[j].Version, components[j+1].Version) < 0 {
				components[j], components[j+1] = components[j+1], components[j]
			}
		}
	}
}
