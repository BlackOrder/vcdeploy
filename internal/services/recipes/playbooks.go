package recipes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/BlackOrder/vcdeploy/internal/validation"
)

// PlaybookService handles playbook operations.
type PlaybookService struct {
	store            storage.Store
	componentService *ComponentService
}

// NewPlaybookService creates a new playbook service.
func NewPlaybookService(store storage.Store) *PlaybookService {
	return &PlaybookService{
		store:            store,
		componentService: NewComponentService(store),
	}
}

// List returns playbooks filtered by namespace and framework.
func (s *PlaybookService) List(ctx context.Context, namespace, frameworkType string, includeDeprecated bool) ([]*storage.Playbook, error) {
	return s.store.ListPlaybooks(ctx, namespace, frameworkType, includeDeprecated)
}

// Get retrieves a specific playbook version.
func (s *PlaybookService) Get(ctx context.Context, namespace, slug, version string) (*storage.Playbook, error) {
	normalizedVersion := validation.NormalizeSemver(version)
	return s.store.GetPlaybook(ctx, namespace, slug, normalizedVersion)
}

// GetByID retrieves a playbook by its ID.
func (s *PlaybookService) GetByID(ctx context.Context, id int64) (*storage.Playbook, error) {
	return s.store.GetPlaybookByID(ctx, id)
}

// GetLatest retrieves the highest semver version.
func (s *PlaybookService) GetLatest(ctx context.Context, namespace, slug string) (*storage.Playbook, error) {
	versions, err := s.store.ListPlaybookVersions(ctx, namespace, slug)
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("playbook not found: %s:%s", namespace, slug)
	}

	var versionStrings []string
	versionMap := make(map[string]*storage.Playbook)
	for _, v := range versions {
		versionStrings = append(versionStrings, v.Version)
		versionMap[v.Version] = v
	}

	latest := validation.LatestSemver(versionStrings)
	return versionMap[latest], nil
}

// GetVersions returns all versions sorted by semver descending.
func (s *PlaybookService) GetVersions(ctx context.Context, namespace, slug string) ([]*storage.Playbook, error) {
	versions, err := s.store.ListPlaybookVersions(ctx, namespace, slug)
	if err != nil {
		return nil, err
	}

	sortPlaybooksBySemverDesc(versions)
	return versions, nil
}

// Create creates a new user playbook.
func (s *PlaybookService) Create(ctx context.Context, playbook *storage.Playbook) error {
	if err := validation.ValidateSemver(playbook.Version); err != nil {
		return fmt.Errorf("invalid version: %w", err)
	}
	playbook.Version = validation.NormalizeSemver(playbook.Version)

	if playbook.Namespace != storage.NamespaceUser {
		return fmt.Errorf("can only create playbooks in 'user' namespace")
	}

	playbook.IsSeed = false
	playbook.CreatedAt = time.Now()

	// Validate all step component references exist
	if err := s.validateSteps(ctx, playbook.Steps); err != nil {
		return err
	}

	return s.store.CreatePlaybook(ctx, playbook)
}

// Update updates an existing user playbook.
func (s *PlaybookService) Update(ctx context.Context, playbook *storage.Playbook) error {
	existing, err := s.store.GetPlaybookByID(ctx, playbook.ID)
	if err != nil {
		return err
	}
	if existing.IsSeed {
		return fmt.Errorf("cannot update seed playbooks")
	}

	// Validate semver if changed
	if playbook.Version != existing.Version {
		if err := validation.ValidateSemver(playbook.Version); err != nil {
			return fmt.Errorf("invalid version: %w", err)
		}
		playbook.Version = validation.NormalizeSemver(playbook.Version)
	}

	// Validate steps if changed
	if err := s.validateSteps(ctx, playbook.Steps); err != nil {
		return err
	}

	return s.store.UpdatePlaybook(ctx, playbook)
}

// CustomizeFromSeed creates a user copy of a seed playbook.
func (s *PlaybookService) CustomizeFromSeed(ctx context.Context, seedID int64, newSlug, newVersion string) (*storage.Playbook, error) {
	seed, err := s.store.GetPlaybookByID(ctx, seedID)
	if err != nil {
		return nil, fmt.Errorf("seed playbook not found: %w", err)
	}
	if !seed.IsSeed {
		return nil, fmt.Errorf("source playbook is not a seed")
	}

	if err := validation.ValidateSemver(newVersion); err != nil {
		return nil, fmt.Errorf("invalid version: %w", err)
	}

	cloned := &storage.Playbook{
		Namespace:       storage.NamespaceUser,
		Slug:            newSlug,
		Version:         validation.NormalizeSemver(newVersion),
		Name:            seed.Name + " (Custom)",
		Description:     seed.Description,
		FrameworkType:   seed.FrameworkType,
		Steps:           seed.Steps,
		SharedDirs:      seed.SharedDirs,
		SharedFiles:     seed.SharedFiles,
		WritableDirs:    seed.WritableDirs,
		KeepReleases:    seed.KeepReleases,
		ValidationRules: seed.ValidationRules,
		IsSeed:          false,
		IsDeprecated:    false,
		ParentID:        &seedID,
		ParentVersion:   seed.Version,
		CreatedAt:       time.Now(),
	}

	if err := s.store.CreatePlaybook(ctx, cloned); err != nil {
		return nil, err
	}

	return cloned, nil
}

// CheckNewerVersionAvailable compares playbook against latest seed version.
func (s *PlaybookService) CheckNewerVersionAvailable(ctx context.Context, playbookID int64) (bool, string, error) {
	playbook, err := s.store.GetPlaybookByID(ctx, playbookID)
	if err != nil {
		return false, "", err
	}

	if playbook.ParentID == nil {
		// Not derived from seed
		return false, "", nil
	}

	parent, err := s.store.GetPlaybookByID(ctx, *playbook.ParentID)
	if err != nil {
		return false, "", nil //nolint:nilerr // Parent deleted is not an error - no update available
	}

	// Get latest version of parent's slug
	latest, err := s.GetLatest(ctx, parent.Namespace, parent.Slug)
	if err != nil {
		return false, "", nil //nolint:nilerr // Can't get latest is not an error - no update available
	}

	if validation.CompareSemver(latest.Version, playbook.ParentVersion) > 0 {
		return true, latest.Version, nil
	}

	return false, "", nil
}

// Validate checks if playbook is valid for activation.
func (s *PlaybookService) Validate(ctx context.Context, playbookID int64) error {
	playbook, err := s.store.GetPlaybookByID(ctx, playbookID)
	if err != nil {
		return err
	}

	return s.validateSteps(ctx, playbook.Steps)
}

// GetAllRequiredVariables collects all required variables from all steps.
func (s *PlaybookService) GetAllRequiredVariables(ctx context.Context, playbookID int64) ([]storage.VariableDefinition, error) {
	playbook, err := s.store.GetPlaybookByID(ctx, playbookID)
	if err != nil {
		return nil, err
	}

	var variables []storage.VariableDefinition
	seen := make(map[string]bool)

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
			if !seen[v.Name] {
				seen[v.Name] = true
				variables = append(variables, v)
			}
		}
	}

	return variables, nil
}

func (s *PlaybookService) validateSteps(ctx context.Context, steps []storage.PlaybookStep) error {
	for _, step := range steps {
		// Parse component reference: namespace:slug:version
		namespace, slug, version, err := ParseComponentRef(step.ComponentRef)
		if err != nil {
			return fmt.Errorf("invalid component reference %q: %w", step.ComponentRef, err)
		}

		component, err := s.store.GetRecipeComponent(ctx, namespace, slug, version)
		if err != nil {
			return fmt.Errorf("component lookup failed: %s: %w", step.ComponentRef, err)
		}
		if component == nil {
			return fmt.Errorf("component not found: %s", step.ComponentRef)
		}
	}
	return nil
}

// Delete removes a user playbook.
func (s *PlaybookService) Delete(ctx context.Context, id int64) error {
	playbook, err := s.store.GetPlaybookByID(ctx, id)
	if err != nil {
		return err
	}
	if playbook.IsSeed {
		return fmt.Errorf("cannot delete seed playbooks")
	}
	return s.store.DeletePlaybook(ctx, id)
}

// sortPlaybooksBySemverDesc sorts playbooks by semver in descending order (newest first).
func sortPlaybooksBySemverDesc(playbooks []*storage.Playbook) {
	for i := 0; i < len(playbooks)-1; i++ {
		for j := 0; j < len(playbooks)-i-1; j++ {
			if validation.CompareSemver(playbooks[j].Version, playbooks[j+1].Version) < 0 {
				playbooks[j], playbooks[j+1] = playbooks[j+1], playbooks[j]
			}
		}
	}
}

// ParseComponentRef parses a component reference string.
// Expected format: namespace:slug:version
// e.g., "seed:laravel-artisan-migrate:v1.0.0"
func ParseComponentRef(ref string) (namespace, slug, version string, err error) {
	parts := strings.SplitN(ref, ":", 3)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("expected format namespace:slug:version")
	}
	return parts[0], parts[1], parts[2], nil
}

// BuildComponentRef creates a component reference string.
func BuildComponentRef(namespace, slug, version string) string {
	return namespace + ":" + slug + ":" + version
}
