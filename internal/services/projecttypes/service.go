// Package projecttypes provides project type management functionality.
package projecttypes

import (
	"context"
	"fmt"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// Ensure Service implements the interface.
var _ services.ProjectTypeServicer = (*Service)(nil)

// Service handles project type management.
type Service struct {
	store storage.Store
}

// New creates a new project types Service.
func New(store storage.Store) *Service {
	return &Service{store: store}
}

// Create creates a new project type.
func (s *Service) Create(ctx context.Context, name, description, buildCmd string) (*storage.ProjectType, error) {
	if name == "" {
		return nil, fmt.Errorf("project type name is required")
	}

	pt := &storage.ProjectType{
		Name:        name,
		Description: description,
		BuildCmd:    buildCmd,
		CreatedAt:   time.Now(),
	}

	if err := s.store.CreateProjectType(ctx, pt); err != nil {
		return nil, fmt.Errorf("creating project type: %w", err)
	}

	return pt, nil
}

// GetByName retrieves a project type by name.
func (s *Service) GetByName(ctx context.Context, name string) (*storage.ProjectType, error) {
	pt, err := s.store.GetProjectTypeByName(ctx, name)
	if err != nil {
		return nil, err // Returns ErrNotFound if not found
	}
	return pt, nil
}

// List returns all project types.
func (s *Service) List(ctx context.Context) ([]*storage.ProjectType, error) {
	return s.store.ListProjectTypes(ctx)
}

// Update updates a project type.
func (s *Service) Update(ctx context.Context, pt *storage.ProjectType) error {
	if err := s.store.UpdateProjectTypeByName(ctx, pt); err != nil {
		return fmt.Errorf("updating project type: %w", err)
	}
	return nil
}

// Delete removes a project type by name.
func (s *Service) Delete(ctx context.Context, name string) error {
	if err := s.store.DeleteProjectType(ctx, name); err != nil {
		return fmt.Errorf("deleting project type: %w", err)
	}
	return nil
}
