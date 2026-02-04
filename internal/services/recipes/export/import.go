package export

import (
	"context"
	"fmt"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/BlackOrder/vcdeploy/internal/validation"
)

// Importer handles recipe import operations.
type Importer struct {
	store storage.Store
}

// NewImporter creates a new importer.
func NewImporter(store storage.Store) *Importer {
	return &Importer{store: store}
}

// ImportResult contains the results of an import operation.
type ImportResult struct {
	ComponentsImported int      `json:"components_imported"`
	ComponentsSkipped  int      `json:"components_skipped"`
	PlaybooksImported  int      `json:"playbooks_imported"`
	PlaybooksSkipped   int      `json:"playbooks_skipped"`
	Errors             []string `json:"errors,omitempty"`
}

// ValidateBundle validates an export bundle format.
func (i *Importer) ValidateBundle(bundle *ExportBundle) error {
	if bundle == nil {
		return fmt.Errorf("bundle is nil")
	}

	if bundle.FormatVersion == "" {
		return fmt.Errorf("missing format_version")
	}

	if err := validation.ValidateSemver(bundle.FormatVersion); err != nil {
		return fmt.Errorf("invalid format_version: %s", bundle.FormatVersion)
	}

	// Check format version compatibility (we can import older versions)
	if validation.CompareSemver(bundle.FormatVersion, FormatVersion) > 0 {
		return fmt.Errorf("format version %s is newer than supported %s", bundle.FormatVersion, FormatVersion)
	}

	// Validate components
	for idx := range bundle.Components {
		c := &bundle.Components[idx]
		if c.Slug == "" {
			return fmt.Errorf("component[%d]: missing slug", idx)
		}
		if c.Version == "" {
			return fmt.Errorf("component[%d] %s: missing version", idx, c.Slug)
		}
		if err := validation.ValidateSemver(c.Version); err != nil {
			return fmt.Errorf("component[%d] %s: invalid version %s", idx, c.Slug, c.Version)
		}
	}

	// Validate playbooks
	for idx := range bundle.Playbooks {
		p := &bundle.Playbooks[idx]
		if p.Slug == "" {
			return fmt.Errorf("playbook[%d]: missing slug", idx)
		}
		if p.Version == "" {
			return fmt.Errorf("playbook[%d] %s: missing version", idx, p.Slug)
		}
		if err := validation.ValidateSemver(p.Version); err != nil {
			return fmt.Errorf("playbook[%d] %s: invalid version %s", idx, p.Slug, p.Version)
		}
	}

	return nil
}

// Import imports a bundle with the specified conflict strategy.
func (i *Importer) Import(ctx context.Context, bundle *ExportBundle, strategy ConflictStrategy) (*ImportResult, error) {
	if err := i.ValidateBundle(bundle); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	if !strategy.IsValid() {
		return nil, fmt.Errorf("invalid conflict strategy: %s", strategy)
	}

	result := &ImportResult{}

	// Import components first (playbooks may depend on them)
	for idx := range bundle.Components {
		c := &bundle.Components[idx]
		imported, err := i.importComponent(ctx, *c, strategy)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("component %s:%s: %v", c.Slug, c.Version, err))
			continue
		}
		if imported {
			result.ComponentsImported++
		} else {
			result.ComponentsSkipped++
		}
	}

	// Import playbooks
	for idx := range bundle.Playbooks {
		p := &bundle.Playbooks[idx]
		imported, err := i.importPlaybook(ctx, *p, strategy)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("playbook %s:%s: %v", p.Slug, p.Version, err))
			continue
		}
		if imported {
			result.PlaybooksImported++
		} else {
			result.PlaybooksSkipped++
		}
	}

	return result, nil
}

// DryRun validates import and returns what would be imported without making changes.
func (i *Importer) DryRun(ctx context.Context, bundle *ExportBundle, strategy ConflictStrategy) (*ImportResult, error) {
	if err := i.ValidateBundle(bundle); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	if !strategy.IsValid() {
		return nil, fmt.Errorf("invalid conflict strategy: %s", strategy)
	}

	result := &ImportResult{}

	// Check components
	for idx := range bundle.Components {
		c := &bundle.Components[idx]
		version := validation.NormalizeSemver(c.Version)
		existing, _ := i.store.GetRecipeComponent(ctx, storage.NamespaceUser, c.Slug, version)

		if existing != nil {
			switch strategy {
			case ConflictSkip:
				result.ComponentsSkipped++
			case ConflictOverwrite, ConflictRename:
				result.ComponentsImported++
			}
		} else {
			result.ComponentsImported++
		}
	}

	// Check playbooks
	for idx := range bundle.Playbooks {
		p := &bundle.Playbooks[idx]
		version := validation.NormalizeSemver(p.Version)
		existing, _ := i.store.GetPlaybook(ctx, storage.NamespaceUser, p.Slug, version)

		if existing != nil {
			switch strategy {
			case ConflictSkip:
				result.PlaybooksSkipped++
			case ConflictOverwrite, ConflictRename:
				result.PlaybooksImported++
			}
		} else {
			result.PlaybooksImported++
		}
	}

	return result, nil
}

func (i *Importer) importComponent(ctx context.Context, ce ComponentExport, strategy ConflictStrategy) (bool, error) {
	version := validation.NormalizeSemver(ce.Version)
	slug := ce.Slug

	existing, _ := i.store.GetRecipeComponent(ctx, storage.NamespaceUser, slug, version)
	if existing != nil {
		switch strategy {
		case ConflictSkip:
			return false, nil
		case ConflictOverwrite:
			if err := i.store.DeleteRecipeComponent(ctx, existing.ID); err != nil {
				return false, fmt.Errorf("failed to delete existing: %w", err)
			}
		case ConflictRename:
			slug += "-imported"
			// Check if renamed version also exists
			renamed, _ := i.store.GetRecipeComponent(ctx, storage.NamespaceUser, slug, version)
			if renamed != nil {
				return false, fmt.Errorf("renamed slug %s already exists", slug)
			}
		}
	}

	component := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          slug,
		Version:       version,
		Name:          ce.Name,
		Description:   ce.Description,
		ComponentType: ce.ComponentType,
		Content:       ce.Content,
		Variables:     ce.Variables,
		IsSeed:        false,
		IsRaw:         ce.IsRaw,
		IsDeprecated:  false,
		CreatedAt:     time.Now(),
	}

	if err := i.store.CreateRecipeComponent(ctx, component); err != nil {
		return false, err
	}

	return true, nil
}

func (i *Importer) importPlaybook(ctx context.Context, pe PlaybookExport, strategy ConflictStrategy) (bool, error) {
	version := validation.NormalizeSemver(pe.Version)
	slug := pe.Slug

	existing, _ := i.store.GetPlaybook(ctx, storage.NamespaceUser, slug, version)
	if existing != nil {
		switch strategy {
		case ConflictSkip:
			return false, nil
		case ConflictOverwrite:
			if err := i.store.DeletePlaybook(ctx, existing.ID); err != nil {
				return false, fmt.Errorf("failed to delete existing: %w", err)
			}
		case ConflictRename:
			slug += "-imported"
			// Check if renamed version also exists
			renamed, _ := i.store.GetPlaybook(ctx, storage.NamespaceUser, slug, version)
			if renamed != nil {
				return false, fmt.Errorf("renamed slug %s already exists", slug)
			}
		}
	}

	playbook := &storage.Playbook{
		Namespace:       storage.NamespaceUser,
		Slug:            slug,
		Version:         version,
		Name:            pe.Name,
		Description:     pe.Description,
		FrameworkType:   pe.FrameworkType,
		Steps:           pe.Steps,
		SharedDirs:      pe.SharedDirs,
		SharedFiles:     pe.SharedFiles,
		WritableDirs:    pe.WritableDirs,
		KeepReleases:    pe.KeepReleases,
		ValidationRules: pe.ValidationRules,
		IsSeed:          false,
		IsDeprecated:    false,
		CreatedAt:       time.Now(),
	}

	if err := i.store.CreatePlaybook(ctx, playbook); err != nil {
		return false, err
	}

	return true, nil
}
