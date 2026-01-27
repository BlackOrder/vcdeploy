// Package deployments provides deployment management functionality.
package deployments

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// Ensure Service implements the interface.
var _ services.DeploymentServicer = (*Service)(nil)

// Service handles deployment management.
type Service struct {
	db *storage.DB
}

// New creates a new deployments Service.
func New(db *storage.DB) *Service {
	return &Service{db: db}
}

// Create creates a new deployment record.
func (s *Service) Create(ctx context.Context, deployment *storage.DeploymentRecord) error {
	if err := s.db.CreateDeployment(ctx, deployment); err != nil {
		return fmt.Errorf("creating deployment: %w", err)
	}
	return nil
}

// GetByID retrieves a deployment by ID.
func (s *Service) GetByID(ctx context.Context, id string) (*storage.DeploymentRecord, error) {
	deployment, err := s.db.GetDeployment(ctx, id)
	if err != nil {
		return nil, err // Returns ErrNotFound if not found
	}
	return deployment, nil
}

// Update updates a deployment record.
func (s *Service) Update(ctx context.Context, deployment *storage.DeploymentRecord) error {
	if err := s.db.UpdateDeployment(ctx, deployment); err != nil {
		return fmt.Errorf("updating deployment: %w", err)
	}
	return nil
}

// ListRecent returns recent deployments.
func (s *Service) ListRecent(ctx context.Context, limit int) ([]*storage.DeploymentRecord, error) {
	deployments, err := s.db.ListDeploymentsRecent(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("listing deployments: %w", err)
	}
	return deployments, nil
}

// CountByStatus returns deployment counts grouped by status.
func (s *Service) CountByStatus(ctx context.Context) (map[string]int64, error) {
	counts, err := s.db.CountDeploymentsByStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("counting deployments by status: %w", err)
	}
	return counts, nil
}

// Cancel marks a deployment as cancelled.
func (s *Service) Cancel(ctx context.Context, id string) error {
	deployment, err := s.db.GetDeployment(ctx, id)
	if err != nil {
		return fmt.Errorf("getting deployment: %w", err)
	}

	if deployment.Status != "pending" && deployment.Status != "running" {
		return fmt.Errorf("cannot cancel deployment in status: %s", deployment.Status)
	}

	now := time.Now()
	deployment.Status = "cancelled"
	deployment.CompletedAt = &now
	deployment.ErrorMessage = "Cancelled by user"

	if err := s.db.UpdateDeployment(ctx, deployment); err != nil {
		return fmt.Errorf("updating deployment: %w", err)
	}

	return nil
}

// --- Log operations ---

// CreateLog creates a deployment log entry.
func (s *Service) CreateLog(ctx context.Context, log *storage.DeploymentLog) error {
	if err := s.db.CreateDeploymentLog(ctx, log); err != nil {
		return fmt.Errorf("creating deployment log: %w", err)
	}
	return nil
}

// ListLogs returns logs for a deployment.
func (s *Service) ListLogs(ctx context.Context, deploymentID string) ([]*storage.DeploymentLog, error) {
	logs, err := s.db.ListDeploymentLogs(ctx, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("listing deployment logs: %w", err)
	}
	return logs, nil
}

// ListLogsAfter returns logs for a deployment after a specific log ID.
func (s *Service) ListLogsAfter(ctx context.Context, deploymentID string, afterID int64) ([]*storage.DeploymentLog, error) {
	logs, err := s.db.ListDeploymentLogsAfter(ctx, deploymentID, afterID)
	if err != nil {
		return nil, fmt.Errorf("listing deployment logs: %w", err)
	}
	return logs, nil
}

// --- Scheduled deployment operations ---

// CreateScheduled creates a scheduled deployment.
func (s *Service) CreateScheduled(ctx context.Context, id, project, target, branch string, scheduledAt time.Time, scheduledBy string) error {
	if err := s.db.CreateScheduledDeployment(ctx, id, project, target, branch, scheduledAt, scheduledBy); err != nil {
		return fmt.Errorf("creating scheduled deployment: %w", err)
	}
	return nil
}

// ListPendingScheduled returns deployments that are due to run.
func (s *Service) ListPendingScheduled(ctx context.Context) ([]*storage.ScheduledDeployment, error) {
	deployments, err := s.db.ListPendingScheduledDeployments(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing scheduled deployments: %w", err)
	}
	return deployments, nil
}

// CancelScheduled cancels a scheduled deployment.
func (s *Service) CancelScheduled(ctx context.Context, id string) error {
	if err := s.db.CancelScheduledDeployment(ctx, id); err != nil {
		return fmt.Errorf("cancelling scheduled deployment: %w", err)
	}
	return nil
}

// --- Cleanup operations ---

// CleanupOld removes completed deployment records older than the cutoff.
func (s *Service) CleanupOld(ctx context.Context, cutoff time.Time) (int64, error) {
	count, err := s.db.CleanupOldDeployments(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("cleaning up old deployments: %w", err)
	}
	return count, nil
}

// CleanupOldLogs removes deployment logs older than the cutoff.
func (s *Service) CleanupOldLogs(ctx context.Context, cutoff time.Time) (int64, error) {
	count, err := s.db.CleanupOldDeploymentLogs(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("cleaning up old deployment logs: %w", err)
	}
	return count, nil
}

// CreateLogsBatch creates multiple deployment log entries in a single transaction.
// This is more efficient than creating logs one at a time for bulk operations.
func (s *Service) CreateLogsBatch(ctx context.Context, deploymentID string, logs []*storage.DeploymentLog) error {
	if len(logs) == 0 {
		return nil
	}

	return s.db.RunInTransaction(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO deployment_logs (deployment_id, level, message, source, created_at)
			VALUES (?, ?, ?, ?, ?)
		`)
		if err != nil {
			return fmt.Errorf("preparing statement: %w", err)
		}
		defer stmt.Close()

		for _, log := range logs {
			if log.DeploymentID == "" {
				log.DeploymentID = deploymentID
			}
			if log.Level == "" {
				log.Level = "info"
			}
			if log.CreatedAt.IsZero() {
				log.CreatedAt = time.Now()
			}

			_, err := stmt.ExecContext(ctx,
				log.DeploymentID,
				log.Level,
				log.Message,
				log.Source,
				log.CreatedAt,
			)
			if err != nil {
				return fmt.Errorf("inserting log: %w", err)
			}
		}
		return nil
	})
}
