// Package seeds provides seed data infrastructure for built-in recipes and playbooks.
package seeds

import (
	"context"
	"fmt"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// LoadSeeds inserts all seed data that doesn't already exist.
// Never updates or deletes existing seeds - idempotent operation.
func (l *Loader) LoadSeeds(ctx context.Context, logger *zap.Logger) error {
	if !l.HasSeeds() {
		logger.Info("no seed data defined, skipping seed loading")
		return nil
	}

	logger.Info("loading seed data",
		zap.Int("components", len(SeedComponents)),
		zap.Int("playbooks", len(SeedPlaybooks)),
	)

	// Load components
	for idx := range SeedComponents {
		sc := &SeedComponents[idx]
		if err := l.loadComponent(ctx, *sc, logger); err != nil {
			return fmt.Errorf("failed to load component %s:%s: %w", sc.Slug, sc.Version, err)
		}
	}

	// Load playbooks
	for idx := range SeedPlaybooks {
		sp := &SeedPlaybooks[idx]
		if err := l.loadPlaybook(ctx, *sp, logger); err != nil {
			return fmt.Errorf("failed to load playbook %s:%s: %w", sp.Slug, sp.Version, err)
		}
	}

	logger.Info("seed data loading complete")
	return nil
}

func (l *Loader) loadComponent(ctx context.Context, sc SeedComponent, logger *zap.Logger) error {
	// Check if already exists
	existing, err := l.store.GetRecipeComponent(ctx, storage.NamespaceSeed, sc.Slug, sc.Version)
	if err != nil {
		return fmt.Errorf("check existing component: %w", err)
	}
	if existing != nil {
		logger.Debug("seed component already exists, skipping",
			zap.String("slug", sc.Slug),
			zap.String("version", sc.Version),
		)
		return nil
	}

	// Insert new seed component
	component := &storage.RecipeComponent{
		Namespace:     storage.NamespaceSeed,
		Slug:          sc.Slug,
		Version:       sc.Version,
		Name:          sc.Name,
		Description:   sc.Description,
		ComponentType: sc.Type,
		Content:       sc.Content,
		Variables:     sc.Variables,
		IsSeed:        true,
		IsRaw:         false,
		IsDeprecated:  false,
		CreatedAt:     time.Now(),
	}

	if err := l.store.CreateRecipeComponent(ctx, component); err != nil {
		return fmt.Errorf("create component: %w", err)
	}

	logger.Info("loaded seed component",
		zap.String("slug", sc.Slug),
		zap.String("version", sc.Version),
	)
	return nil
}

func (l *Loader) loadPlaybook(ctx context.Context, sp SeedPlaybook, logger *zap.Logger) error {
	// Check if already exists
	existing, err := l.store.GetPlaybook(ctx, storage.NamespaceSeed, sp.Slug, sp.Version)
	if err != nil {
		return fmt.Errorf("check existing playbook: %w", err)
	}
	if existing != nil {
		logger.Debug("seed playbook already exists, skipping",
			zap.String("slug", sp.Slug),
			zap.String("version", sp.Version),
		)
		return nil
	}

	// Insert new seed playbook
	keepReleases := sp.KeepReleases
	if keepReleases == 0 {
		keepReleases = 5
	}

	playbook := &storage.Playbook{
		Namespace:     storage.NamespaceSeed,
		Slug:          sp.Slug,
		Version:       sp.Version,
		Name:          sp.Name,
		Description:   sp.Description,
		FrameworkType: sp.FrameworkType,
		Steps:         sp.Steps,
		SharedDirs:    sp.SharedDirs,
		SharedFiles:   sp.SharedFiles,
		WritableDirs:  sp.WritableDirs,
		KeepReleases:  keepReleases,
		IsSeed:        true,
		IsDeprecated:  false,
		CreatedAt:     time.Now(),
	}

	if err := l.store.CreatePlaybook(ctx, playbook); err != nil {
		return fmt.Errorf("create playbook: %w", err)
	}

	logger.Info("loaded seed playbook",
		zap.String("slug", sp.Slug),
		zap.String("version", sp.Version),
	)
	return nil
}

// CountSeeds returns the count of defined seed components and playbooks.
func (l *Loader) CountSeeds() (components, playbooks int) {
	return len(SeedComponents), len(SeedPlaybooks)
}
