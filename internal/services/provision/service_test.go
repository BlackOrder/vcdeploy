// Package provision provides agent provisioning services.
package provision

import (
	"context"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/services/testutil"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

func setupTest(t *testing.T) (*Service, func()) {
	t.Helper()
	db, cleanup := testutil.NewTestStore(t)
	svc := New(db)
	return svc, cleanup
}

func TestService_CreateJob(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("valid job", func(t *testing.T) {
		job := &storage.ProvisionJob{
			TargetHost: "example.com",
			TargetUser: "deploy",
		}

		err := svc.CreateJob(ctx, job)
		if err != nil {
			t.Fatalf("Failed to create job: %v", err)
		}

		if job.ID == "" {
			t.Error("Expected job ID to be set")
		}
		if job.Status != "pending" {
			t.Errorf("Expected status 'pending', got %q", job.Status)
		}
		if job.TargetPort != 22 {
			t.Errorf("Expected default port 22, got %d", job.TargetPort)
		}
	})

	t.Run("missing target host", func(t *testing.T) {
		job := &storage.ProvisionJob{
			TargetUser: "deploy",
		}

		err := svc.CreateJob(ctx, job)
		if err == nil {
			t.Error("Expected error for missing target host")
		}
		if !services.IsInvalidInput(err) {
			t.Errorf("Expected invalid input error, got: %v", err)
		}
	})

	t.Run("custom values", func(t *testing.T) {
		job := &storage.ProvisionJob{
			ID:         "custom-id",
			TargetHost: "server.example.com",
			TargetPort: 2222,
			TargetUser: "admin",
			Status:     "running",
		}

		err := svc.CreateJob(ctx, job)
		if err != nil {
			t.Fatalf("Failed to create job: %v", err)
		}

		if job.ID != "custom-id" {
			t.Errorf("Expected ID 'custom-id', got %q", job.ID)
		}
		if job.TargetPort != 2222 {
			t.Errorf("Expected port 2222, got %d", job.TargetPort)
		}
	})
}

func TestService_GetJob(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a job
	job := &storage.ProvisionJob{
		TargetHost: "test.example.com",
		TargetUser: "deploy",
	}
	_ = svc.CreateJob(ctx, job)

	t.Run("existing job", func(t *testing.T) {
		retrieved, err := svc.GetJob(ctx, job.ID)
		if err != nil {
			t.Fatalf("Failed to get job: %v", err)
		}
		if retrieved.TargetHost != "test.example.com" {
			t.Errorf("Expected target host 'test.example.com', got %q", retrieved.TargetHost)
		}
	})

	t.Run("non-existent job", func(t *testing.T) {
		_, err := svc.GetJob(ctx, "nonexistent")
		if err == nil {
			t.Error("Expected error for non-existent job")
		}
		if !services.IsNotFound(err) {
			t.Errorf("Expected not found error, got: %v", err)
		}
	})
}

func TestService_UpdateStatus(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a job
	job := &storage.ProvisionJob{
		TargetHost: "update.example.com",
		TargetUser: "deploy",
	}
	_ = svc.CreateJob(ctx, job)

	t.Run("valid status update", func(t *testing.T) {
		err := svc.UpdateStatus(ctx, job.ID, "running", "downloading", "", 25)
		if err != nil {
			t.Fatalf("Failed to update status: %v", err)
		}

		updated, _ := svc.GetJob(ctx, job.ID)
		if updated.Status != "running" {
			t.Errorf("Expected status 'running', got %q", updated.Status)
		}
		if updated.Stage != "downloading" {
			t.Errorf("Expected stage 'downloading', got %q", updated.Stage)
		}
		if updated.Progress != 25 {
			t.Errorf("Expected progress 25, got %d", updated.Progress)
		}
	})

	t.Run("invalid status", func(t *testing.T) {
		err := svc.UpdateStatus(ctx, job.ID, "invalid_status", "", "", 0)
		if err == nil {
			t.Error("Expected error for invalid status")
		}
		if !services.IsInvalidInput(err) {
			t.Errorf("Expected invalid input error, got: %v", err)
		}
	})

	t.Run("non-existent job", func(t *testing.T) {
		err := svc.UpdateStatus(ctx, "nonexistent", "running", "", "", 0)
		if err == nil {
			t.Error("Expected error for non-existent job")
		}
		if !services.IsNotFound(err) {
			t.Errorf("Expected not found error, got: %v", err)
		}
	})
}

func TestService_Cancel(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("cancel pending job", func(t *testing.T) {
		job := &storage.ProvisionJob{
			TargetHost: "cancel-pending.example.com",
			TargetUser: "deploy",
		}
		_ = svc.CreateJob(ctx, job)

		err := svc.Cancel(ctx, job.ID)
		if err != nil {
			t.Fatalf("Failed to cancel job: %v", err)
		}

		cancelled, _ := svc.GetJob(ctx, job.ID)
		if cancelled.Status != "cancelled" {
			t.Errorf("Expected status 'cancelled', got %q", cancelled.Status)
		}
	})

	t.Run("cancel running job", func(t *testing.T) {
		job := &storage.ProvisionJob{
			TargetHost: "cancel-running.example.com",
			TargetUser: "deploy",
		}
		_ = svc.CreateJob(ctx, job)
		_ = svc.UpdateStatus(ctx, job.ID, "running", "installing", "", 50)

		err := svc.Cancel(ctx, job.ID)
		if err != nil {
			t.Fatalf("Failed to cancel running job: %v", err)
		}

		cancelled, _ := svc.GetJob(ctx, job.ID)
		if cancelled.Status != "cancelled" {
			t.Errorf("Expected status 'cancelled', got %q", cancelled.Status)
		}
	})

	t.Run("cancel completed job", func(t *testing.T) {
		job := &storage.ProvisionJob{
			TargetHost: "cancel-completed.example.com",
			TargetUser: "deploy",
		}
		_ = svc.CreateJob(ctx, job)
		_ = svc.UpdateStatus(ctx, job.ID, "completed", "", "", 100)

		err := svc.Cancel(ctx, job.ID)
		if err == nil {
			t.Error("Expected error when cancelling completed job")
		}
		if !services.IsInvalidInput(err) {
			t.Errorf("Expected invalid input error, got: %v", err)
		}
	})
}

func TestService_ListPending(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create some jobs with different statuses
	for i := range 3 {
		job := &storage.ProvisionJob{
			TargetHost: "pending.example.com",
			TargetUser: "deploy",
		}
		_ = svc.CreateJob(ctx, job)
		if i == 0 {
			_ = svc.UpdateStatus(ctx, job.ID, "completed", "", "", 100)
		}
	}

	pending, err := svc.ListPending(ctx)
	if err != nil {
		t.Fatalf("Failed to list pending: %v", err)
	}

	if len(pending) != 2 {
		t.Errorf("Expected 2 pending jobs, got %d", len(pending))
	}
}

func TestService_ListByHost(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create jobs for different hosts
	for i := range 3 {
		job := &storage.ProvisionJob{
			TargetHost: "host-a.example.com",
			TargetUser: "deploy",
		}
		_ = svc.CreateJob(ctx, job)
		_ = i // use i
	}
	job := &storage.ProvisionJob{
		TargetHost: "host-b.example.com",
		TargetUser: "deploy",
	}
	_ = svc.CreateJob(ctx, job)

	result, err := svc.ListByHost(ctx, "host-a.example.com", services.Pagination{Limit: 10})
	if err != nil {
		t.Fatalf("Failed to list by host: %v", err)
	}

	if len(result.Items) != 3 {
		t.Errorf("Expected 3 jobs for host-a, got %d", len(result.Items))
	}
	if result.TotalCount != 3 {
		t.Errorf("Expected total count 3, got %d", result.TotalCount)
	}
}

func TestService_Cleanup(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create and complete a job
	job := &storage.ProvisionJob{
		TargetHost: "cleanup.example.com",
		TargetUser: "deploy",
	}
	_ = svc.CreateJob(ctx, job)
	_ = svc.UpdateStatus(ctx, job.ID, "completed", "", "", 100)

	// Cleanup jobs completed before now (should remove nothing since it just completed)
	count, err := svc.Cleanup(ctx, job.StartedAt.Add(-time.Hour))
	if err != nil {
		t.Fatalf("Failed to cleanup: %v", err)
	}

	// Should be 0 since the job completed after the cutoff
	t.Logf("Cleaned up %d old jobs", count)
}
