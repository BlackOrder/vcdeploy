package export

import (
	"context"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// Exporter handles recipe export operations.
type Exporter struct {
	store   storage.Store
	version string // vcdeploy version
}

// NewExporter creates a new exporter.
func NewExporter(store storage.Store, version string) *Exporter {
	return &Exporter{store: store, version: version}
}

// ExportAll exports all user components and playbooks.
func (e *Exporter) ExportAll(ctx context.Context) (*ExportBundle, error) {
	bundle := &ExportBundle{
		FormatVersion:   FormatVersion,
		ExportedAt:      time.Now(),
		VCDeployVersion: e.version,
		Components:      []ComponentExport{},
		Playbooks:       []PlaybookExport{},
	}

	// Export user components
	components, err := e.store.ListRecipeComponents(ctx, storage.NamespaceUser, true)
	if err != nil {
		return nil, err
	}

	for _, c := range components {
		bundle.Components = append(bundle.Components, ComponentExport{
			Slug:          c.Slug,
			Version:       c.Version,
			Name:          c.Name,
			Description:   c.Description,
			ComponentType: c.ComponentType,
			Content:       c.Content,
			Variables:     c.Variables,
			IsRaw:         c.IsRaw,
		})
	}

	// Export user playbooks
	playbooks, err := e.store.ListPlaybooks(ctx, storage.NamespaceUser, "", true)
	if err != nil {
		return nil, err
	}

	for _, p := range playbooks {
		bundle.Playbooks = append(bundle.Playbooks, PlaybookExport{
			Slug:            p.Slug,
			Version:         p.Version,
			Name:            p.Name,
			Description:     p.Description,
			FrameworkType:   p.FrameworkType,
			Steps:           p.Steps,
			SharedDirs:      p.SharedDirs,
			SharedFiles:     p.SharedFiles,
			WritableDirs:    p.WritableDirs,
			KeepReleases:    p.KeepReleases,
			ValidationRules: p.ValidationRules,
		})
	}

	return bundle, nil
}

// ExportComponents exports specific components by slug.
func (e *Exporter) ExportComponents(ctx context.Context, slugs []string) (*ExportBundle, error) {
	bundle := &ExportBundle{
		FormatVersion:   FormatVersion,
		ExportedAt:      time.Now(),
		VCDeployVersion: e.version,
		Components:      []ComponentExport{},
		Playbooks:       []PlaybookExport{},
	}

	for _, slug := range slugs {
		versions, err := e.store.ListRecipeComponentVersions(ctx, storage.NamespaceUser, slug)
		if err != nil {
			continue
		}

		for _, c := range versions {
			bundle.Components = append(bundle.Components, ComponentExport{
				Slug:          c.Slug,
				Version:       c.Version,
				Name:          c.Name,
				Description:   c.Description,
				ComponentType: c.ComponentType,
				Content:       c.Content,
				Variables:     c.Variables,
				IsRaw:         c.IsRaw,
			})
		}
	}

	return bundle, nil
}

// ExportPlaybooks exports specific playbooks by slug.
func (e *Exporter) ExportPlaybooks(ctx context.Context, slugs []string) (*ExportBundle, error) {
	bundle := &ExportBundle{
		FormatVersion:   FormatVersion,
		ExportedAt:      time.Now(),
		VCDeployVersion: e.version,
		Components:      []ComponentExport{},
		Playbooks:       []PlaybookExport{},
	}

	for _, slug := range slugs {
		versions, err := e.store.ListPlaybookVersions(ctx, storage.NamespaceUser, slug)
		if err != nil {
			continue
		}

		for _, p := range versions {
			bundle.Playbooks = append(bundle.Playbooks, PlaybookExport{
				Slug:            p.Slug,
				Version:         p.Version,
				Name:            p.Name,
				Description:     p.Description,
				FrameworkType:   p.FrameworkType,
				Steps:           p.Steps,
				SharedDirs:      p.SharedDirs,
				SharedFiles:     p.SharedFiles,
				WritableDirs:    p.WritableDirs,
				KeepReleases:    p.KeepReleases,
				ValidationRules: p.ValidationRules,
			})
		}
	}

	return bundle, nil
}
