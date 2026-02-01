// Package projects provides project management functionality.
package projects

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// Ensure Service implements the interface.
var _ services.ProjectServicer = (*Service)(nil)

// Service handles project management.
type Service struct {
	store storage.Store
}

// New creates a new projects Service.
func New(store storage.Store) *Service {
	return &Service{store: store}
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

	if err := s.store.CreateProject(project); err != nil {
		return nil, fmt.Errorf("creating project: %w", err)
	}

	return project, nil
}

// GetByID retrieves a project by ID.
func (s *Service) GetByID(ctx context.Context, id int64) (*storage.Project, error) {
	project, err := s.store.GetProjectByID(ctx, id)
	if err != nil {
		return nil, err // Returns ErrNotFound if not found
	}
	return project, nil
}

// GetByName retrieves a project by name.
func (s *Service) GetByName(ctx context.Context, name string) (*storage.Project, error) {
	project, err := s.store.GetProjectByName(ctx, name)
	if err != nil {
		return nil, err // Returns ErrNotFound if not found
	}
	return project, nil
}

// List returns all projects.
func (s *Service) List(ctx context.Context) ([]*storage.Project, error) {
	return s.store.ListProjects()
}

// ListPaginated returns projects with pagination support.
func (s *Service) ListPaginated(ctx context.Context, p services.Pagination) (*services.ListResult[*storage.Project], error) {
	projects, err := s.store.ListProjectsPaginated(ctx, p.Limit, p.Offset)
	if err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}
	count, err := s.store.CountProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("counting projects: %w", err)
	}
	return &services.ListResult[*storage.Project]{
		Items:      projects,
		TotalCount: count,
		Pagination: p,
	}, nil
}

// Count returns the total number of projects.
func (s *Service) Count(ctx context.Context) (int64, error) {
	count, err := s.store.CountProjects(ctx)
	if err != nil {
		return 0, fmt.Errorf("counting projects: %w", err)
	}
	return count, nil
}

// UpdateByID updates a project by ID.
func (s *Service) UpdateByID(ctx context.Context, project *storage.Project) error {
	if err := s.store.UpdateProjectByID(ctx, project); err != nil {
		return fmt.Errorf("updating project: %w", err)
	}
	return nil
}

// Update updates a project.
func (s *Service) Update(ctx context.Context, project *storage.Project) error {
	if err := s.store.UpdateProjectByName(ctx, project); err != nil {
		return fmt.Errorf("updating project: %w", err)
	}
	return nil
}

// DeleteByID removes a project by ID.
func (s *Service) DeleteByID(ctx context.Context, id int64) error {
	if err := s.store.DeleteProjectByID(ctx, id); err != nil {
		return fmt.Errorf("deleting project: %w", err)
	}
	return nil
}

// Delete removes a project by name.
func (s *Service) Delete(ctx context.Context, name string) error {
	if err := s.store.DeleteProject(name); err != nil {
		return fmt.Errorf("deleting project: %w", err)
	}
	return nil
}

// DeleteWithCleanup deletes a project and all associated data (webhooks, secrets, deployments) in a transaction.
func (s *Service) DeleteWithCleanup(ctx context.Context, name string) error {
	// First get the project to find its ID
	project, err := s.store.GetProjectByName(ctx, name)
	if err != nil {
		return fmt.Errorf("getting project: %w", err)
	}

	return s.store.RunInTransaction(ctx, func(tx *sql.Tx) error {
		// Delete project webhooks
		if _, err := tx.ExecContext(ctx, `DELETE FROM project_webhooks WHERE project_id = ?`, project.ID); err != nil {
			return fmt.Errorf("deleting project webhooks: %w", err)
		}

		// Delete project secrets
		if _, err := tx.ExecContext(ctx, `DELETE FROM secrets WHERE project = ?`, name); err != nil {
			return fmt.Errorf("deleting project secrets: %w", err)
		}

		// Delete deployment logs for this project's deployments
		if _, err := tx.ExecContext(ctx, `DELETE FROM deployment_logs WHERE deployment_id IN (SELECT id FROM deployments WHERE project = ?)`, name); err != nil {
			return fmt.Errorf("deleting deployment logs: %w", err)
		}

		// Delete deployments
		if _, err := tx.ExecContext(ctx, `DELETE FROM deployments WHERE project = ?`, name); err != nil {
			return fmt.Errorf("deleting deployments: %w", err)
		}

		// Cancel any pending scheduled deployments (scheduled_at is on deployments table)
		if _, err := tx.ExecContext(ctx, `UPDATE deployments SET status = 'cancelled' WHERE project = ? AND scheduled_at IS NOT NULL AND status IN ('scheduled', 'pending')`, name); err != nil {
			return fmt.Errorf("cancelling scheduled deployments: %w", err)
		}

		// Delete the project
		result, err := tx.ExecContext(ctx, `DELETE FROM projects WHERE name = ?`, name)
		if err != nil {
			return fmt.Errorf("deleting project: %w", err)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("checking rows affected: %w", err)
		}
		if rowsAffected == 0 {
			return storage.ErrNotFound
		}

		return nil
	})
}
