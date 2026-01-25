// Package projects provides project management functionality.
package projects

import (
	"context"
	"fmt"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// Ensure Service implements the interface.
var _ services.ProjectServicer = (*Service)(nil)

// Service handles project management.
type Service struct {
	db *storage.DB
}

// New creates a new projects Service.
func New(db *storage.DB) *Service {
	return &Service{db: db}
}

// Create creates a new project.
func (s *Service) Create(ctx context.Context, name, repository, branch, deployPath, projectType string) (*storage.Project, error) {
	if name == "" {
		return nil, fmt.Errorf("project name is required")
	}

	// Set defaults
	if branch == "" {
		branch = "main"
	}
	if projectType == "" {
		projectType = "generic"
	}

	project := &storage.Project{
		Name:       name,
		Repository: repository,
		Branch:     branch,
		DeployPath: deployPath,
		Type:       projectType,
		CreatedAt:  time.Now(),
	}

	if err := s.db.CreateProject(project); err != nil {
		return nil, fmt.Errorf("creating project: %w", err)
	}

	return project, nil
}

// GetByName retrieves a project by name.
func (s *Service) GetByName(ctx context.Context, name string) (*storage.Project, error) {
	project, err := s.db.GetProjectByName(ctx, name)
	if err != nil {
		return nil, err // Returns ErrNotFound if not found
	}
	return project, nil
}

// List returns all projects.
func (s *Service) List(ctx context.Context) ([]*storage.Project, error) {
	return s.db.ListProjects()
}

// Update updates a project.
func (s *Service) Update(ctx context.Context, project *storage.Project) error {
	if err := s.db.UpdateProjectByName(ctx, project); err != nil {
		return fmt.Errorf("updating project: %w", err)
	}
	return nil
}

// Delete removes a project by name.
func (s *Service) Delete(ctx context.Context, name string) error {
	if err := s.db.DeleteProject(name); err != nil {
		return fmt.Errorf("deleting project: %w", err)
	}
	return nil
}
