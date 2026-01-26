// Package provision provides agent provisioning services.
package provision

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// Service handles agent provisioning operations.
type Service struct {
	db *storage.DB
}

// Ensure Service implements ProvisionServicer.
var _ services.ProvisionServicer = (*Service)(nil)

// New creates a new provision service.
func New(db *storage.DB) *Service {
	return &Service{db: db}
}

// CreateJob creates a new provisioning job.
func (s *Service) CreateJob(ctx context.Context, job *storage.ProvisionJob) error {
	const op = "provision.CreateJob"

	if job.TargetHost == "" {
		return &services.ServiceError{
			Op:  op,
			Err: fmt.Errorf("target_host is required: %w", services.ErrInvalidInput),
		}
	}

	if job.ID == "" {
		job.ID = generateID()
	}
	if job.Status == "" {
		job.Status = "pending"
	}
	if job.TargetPort == 0 {
		job.TargetPort = 22
	}
	job.StartedAt = time.Now()

	if err := s.db.CreateProvisionJob(ctx, job); err != nil {
		return &services.ServiceError{
			Op:  op,
			Err: err,
		}
	}

	return nil
}

// GetJob retrieves a provisioning job by ID.
func (s *Service) GetJob(ctx context.Context, id string) (*storage.ProvisionJob, error) {
	const op = "provision.GetJob"

	job, err := s.db.GetProvisionJob(ctx, id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, &services.ServiceError{
				Op:       op,
				Err:      services.ErrNotFound,
				Resource: "provision_job",
				ID:       id,
			}
		}
		return nil, &services.ServiceError{
			Op:  op,
			Err: err,
		}
	}

	return job, nil
}

// UpdateStatus updates the status of a provisioning job.
func (s *Service) UpdateStatus(ctx context.Context, id, status, stage, errorMessage string, progress int) error {
	const op = "provision.UpdateStatus"

	validStatuses := map[string]bool{
		"pending":   true,
		"running":   true,
		"completed": true,
		"failed":    true,
		"cancelled": true,
	}

	if !validStatuses[status] {
		return &services.ServiceError{
			Op:  op,
			Err: fmt.Errorf("invalid status: %s: %w", status, services.ErrInvalidInput),
		}
	}

	if err := s.db.UpdateProvisionJobStatus(ctx, id, status, stage, errorMessage, progress); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return &services.ServiceError{
				Op:       op,
				Err:      services.ErrNotFound,
				Resource: "provision_job",
				ID:       id,
			}
		}
		return &services.ServiceError{
			Op:  op,
			Err: err,
		}
	}

	return nil
}

// ListPending returns all pending provisioning jobs.
func (s *Service) ListPending(ctx context.Context) ([]*storage.ProvisionJob, error) {
	const op = "provision.ListPending"

	jobs, err := s.db.ListPendingProvisionJobs(ctx)
	if err != nil {
		return nil, &services.ServiceError{
			Op:  op,
			Err: err,
		}
	}

	return jobs, nil
}

// ListByHost returns provisioning jobs for a specific host.
func (s *Service) ListByHost(ctx context.Context, host string, pagination services.Pagination) (*services.ListResult[*storage.ProvisionJob], error) {
	const op = "provision.ListByHost"

	pagination = services.NewPagination(pagination.Limit, pagination.Offset)

	jobs, total, err := s.db.ListProvisionJobsByHost(ctx, host, pagination.Limit, pagination.Offset)
	if err != nil {
		return nil, &services.ServiceError{
			Op:  op,
			Err: err,
		}
	}

	return &services.ListResult[*storage.ProvisionJob]{
		Items:      jobs,
		TotalCount: total,
		Pagination: pagination,
	}, nil
}

// Cancel cancels a pending provisioning job.
func (s *Service) Cancel(ctx context.Context, id string) error {
	const op = "provision.Cancel"

	// Get current job to check status
	job, err := s.GetJob(ctx, id)
	if err != nil {
		return fmt.Errorf("%s: get job: %w", op, err)
	}

	if job.Status != "pending" && job.Status != "running" {
		return &services.ServiceError{
			Op:  op,
			Err: fmt.Errorf("cannot cancel job with status %s: %w", job.Status, services.ErrInvalidInput),
		}
	}

	return s.UpdateStatus(ctx, id, "cancelled", "", "Cancelled by user", job.Progress)
}

// Cleanup removes old completed/failed jobs.
func (s *Service) Cleanup(ctx context.Context, before time.Time) (int64, error) {
	const op = "provision.Cleanup"

	count, err := s.db.CleanupOldProvisionJobs(ctx, before)
	if err != nil {
		return 0, &services.ServiceError{
			Op:  op,
			Err: err,
		}
	}

	return count, nil
}

// generateID generates a unique ID for provision jobs.
func generateID() string {
	return fmt.Sprintf("prov_%d", time.Now().UnixNano())
}
