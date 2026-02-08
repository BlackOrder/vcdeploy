package storage

import (
	"context"
	"testing"
	"time"
)

// --- DeploymentRecord tests ---

func TestMemoryStore_CreateDeployment(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	d := &DeploymentRecord{
		ID:      "deploy-1",
		Project: "myproject",
		Target:  "production",
		Branch:  "main",
	}
	err := s.CreateDeployment(ctx, d)
	if err != nil {
		t.Fatalf("CreateDeployment() error = %v", err)
	}

	if d.StartedAt.IsZero() {
		t.Error("CreateDeployment() did not set StartedAt")
	}
	if d.Status != "pending" {
		t.Errorf("Status = %s, want pending", d.Status)
	}
}

func TestMemoryStore_CreateDeployment_Duplicate(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateDeployment(ctx, &DeploymentRecord{ID: "duplicate"})

	err := s.CreateDeployment(ctx, &DeploymentRecord{ID: "duplicate"})
	if err != ErrDuplicate {
		t.Errorf("CreateDeployment() error = %v, want ErrDuplicate", err)
	}
}

func TestMemoryStore_GetDeployment(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateDeployment(ctx, &DeploymentRecord{ID: "find-me", Project: "proj1"})

	found, err := s.GetDeployment(ctx, "find-me")
	if err != nil {
		t.Fatalf("GetDeployment() error = %v", err)
	}
	if found.Project != "proj1" {
		t.Errorf("Project = %s, want proj1", found.Project)
	}
}

func TestMemoryStore_GetDeployment_NotFound(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	_, err := s.GetDeployment(context.Background(), "nonexistent")
	if err != ErrNotFound {
		t.Errorf("GetDeployment() error = %v, want ErrNotFound", err)
	}
}

func TestMemoryStore_UpdateDeployment(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateDeployment(ctx, &DeploymentRecord{ID: "update-me", Status: "pending"})

	now := time.Now()
	err := s.UpdateDeployment(ctx, &DeploymentRecord{ID: "update-me", Status: "completed", CompletedAt: &now})
	if err != nil {
		t.Fatalf("UpdateDeployment() error = %v", err)
	}

	found, _ := s.GetDeployment(ctx, "update-me")
	if found.Status != "completed" {
		t.Errorf("Status = %s, want completed", found.Status)
	}
}

func TestMemoryStore_ListDeploymentsRecent(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	now := time.Now()
	s.CreateDeployment(ctx, &DeploymentRecord{ID: "d1", StartedAt: now.Add(-2 * time.Hour)})
	s.CreateDeployment(ctx, &DeploymentRecord{ID: "d2", StartedAt: now.Add(-1 * time.Hour)})
	s.CreateDeployment(ctx, &DeploymentRecord{ID: "d3", StartedAt: now})

	list, err := s.ListDeploymentsRecent(ctx, 2)
	if err != nil {
		t.Fatalf("ListDeploymentsRecent() error = %v", err)
	}
	if len(list) != 2 {
		t.Errorf("len(list) = %d, want 2", len(list))
	}
	// Should be newest first
	if list[0].ID != "d3" {
		t.Errorf("First item ID = %s, want d3", list[0].ID)
	}
}

func TestMemoryStore_CountDeploymentsByStatus(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateDeployment(ctx, &DeploymentRecord{ID: "d1", Status: "completed"})
	s.CreateDeployment(ctx, &DeploymentRecord{ID: "d2", Status: "completed"})
	s.CreateDeployment(ctx, &DeploymentRecord{ID: "d3", Status: "failed"})

	counts, err := s.CountDeploymentsByStatus(ctx)
	if err != nil {
		t.Fatalf("CountDeploymentsByStatus() error = %v", err)
	}
	if counts["completed"] != 2 {
		t.Errorf("completed count = %d, want 2", counts["completed"])
	}
	if counts["failed"] != 1 {
		t.Errorf("failed count = %d, want 1", counts["failed"])
	}
}

// --- DeploymentLog tests ---

func TestMemoryStore_CreateDeploymentLog(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	log := &DeploymentLog{
		DeploymentID: "deploy-1",
		Level:        "info",
		Message:      "Deployment started",
		Source:       "server",
	}
	err := s.CreateDeploymentLog(ctx, log)
	if err != nil {
		t.Fatalf("CreateDeploymentLog() error = %v", err)
	}

	if log.ID == "" {
		t.Error("CreateDeploymentLog() did not assign ID")
	}
}

func TestMemoryStore_ListDeploymentLogs(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateDeploymentLog(ctx, &DeploymentLog{DeploymentID: "d1", Message: "log1"})
	s.CreateDeploymentLog(ctx, &DeploymentLog{DeploymentID: "d1", Message: "log2"})
	s.CreateDeploymentLog(ctx, &DeploymentLog{DeploymentID: "d2", Message: "log3"})

	logs, err := s.ListDeploymentLogs(ctx, "d1")
	if err != nil {
		t.Fatalf("ListDeploymentLogs() error = %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("len(logs) = %d, want 2", len(logs))
	}
}

func TestMemoryStore_ListDeploymentLogsAfter(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	log1 := &DeploymentLog{DeploymentID: "d1", Message: "log1"}
	s.CreateDeploymentLog(ctx, log1)
	log2 := &DeploymentLog{DeploymentID: "d1", Message: "log2"}
	s.CreateDeploymentLog(ctx, log2)
	log3 := &DeploymentLog{DeploymentID: "d1", Message: "log3"}
	s.CreateDeploymentLog(ctx, log3)

	logs, err := s.ListDeploymentLogsAfter(ctx, "d1", log1.ID)
	if err != nil {
		t.Fatalf("ListDeploymentLogsAfter() error = %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("len(logs) = %d, want 2", len(logs))
	}
}

// --- DeploymentRollback tests ---

func TestMemoryStore_CreateDeploymentRollback(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	rollback := &DeploymentRollback{
		DeploymentID: "deploy-1",
		ProjectName:  "myproject",
		FromRelease:  2,
		ToRelease:    1,
		Reason:       "Health check failed",
		TriggeredBy:  RollbackTriggerAutoHealthFail,
	}
	err := s.CreateDeploymentRollback(ctx, rollback)
	if err != nil {
		t.Fatalf("CreateDeploymentRollback() error = %v", err)
	}

	if rollback.ID == "" {
		t.Error("CreateDeploymentRollback() did not assign ID")
	}
	if rollback.Status != "pending" {
		t.Errorf("Status = %s, want pending", rollback.Status)
	}
}

func TestMemoryStore_GetDeploymentRollback(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	rollback := &DeploymentRollback{DeploymentID: "d1", ProjectName: "proj1"}
	s.CreateDeploymentRollback(ctx, rollback)

	found, err := s.GetDeploymentRollback(ctx, rollback.ID)
	if err != nil {
		t.Fatalf("GetDeploymentRollback() error = %v", err)
	}
	if found.ProjectName != "proj1" {
		t.Errorf("ProjectName = %s, want proj1", found.ProjectName)
	}
}

func TestMemoryStore_UpdateDeploymentRollback(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	rollback := &DeploymentRollback{DeploymentID: "d1"}
	s.CreateDeploymentRollback(ctx, rollback)

	now := time.Now()
	rollback.Status = "completed"
	rollback.CompletedAt = &now

	err := s.UpdateDeploymentRollback(ctx, rollback)
	if err != nil {
		t.Fatalf("UpdateDeploymentRollback() error = %v", err)
	}

	found, _ := s.GetDeploymentRollback(ctx, rollback.ID)
	if found.Status != "completed" {
		t.Errorf("Status = %s, want completed", found.Status)
	}
}

func TestMemoryStore_ListDeploymentRollbacks(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	now := time.Now()
	s.CreateDeploymentRollback(ctx, &DeploymentRollback{ProjectName: "proj1", StartedAt: now.Add(-time.Hour)})
	s.CreateDeploymentRollback(ctx, &DeploymentRollback{ProjectName: "proj1", StartedAt: now})
	s.CreateDeploymentRollback(ctx, &DeploymentRollback{ProjectName: "proj2"})

	list, total, err := s.ListDeploymentRollbacks(ctx, "proj1", 10, 0)
	if err != nil {
		t.Fatalf("ListDeploymentRollbacks() error = %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(list) != 2 {
		t.Errorf("len(list) = %d, want 2", len(list))
	}
	// Should be sorted newest first
	if list[0].StartedAt.Before(list[1].StartedAt) {
		t.Error("Expected newest first")
	}
}

func TestMemoryStore_GetLatestRollbackForDeployment(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	now := time.Now()
	s.CreateDeploymentRollback(ctx, &DeploymentRollback{DeploymentID: "d1", FromRelease: 1, StartedAt: now.Add(-time.Hour)})
	s.CreateDeploymentRollback(ctx, &DeploymentRollback{DeploymentID: "d1", FromRelease: 2, StartedAt: now})

	latest, err := s.GetLatestRollbackForDeployment(ctx, "d1")
	if err != nil {
		t.Fatalf("GetLatestRollbackForDeployment() error = %v", err)
	}
	if latest.FromRelease != 2 {
		t.Errorf("FromRelease = %d, want 2", latest.FromRelease)
	}
}

func TestMemoryStore_GetLatestRollbackForDeployment_NotFound(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	_, err := s.GetLatestRollbackForDeployment(context.Background(), "nonexistent")
	if err != ErrNotFound {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

// --- ScheduledDeployment tests ---

func TestMemoryStore_CreateScheduledDeployment(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	err := s.CreateScheduledDeployment(ctx, "sched-1", "myproject", "production", "main", time.Now().Add(time.Hour), "user1")
	if err != nil {
		t.Fatalf("CreateScheduledDeployment() error = %v", err)
	}
}

func TestMemoryStore_CreateScheduledDeployment_Duplicate(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateScheduledDeployment(ctx, "duplicate", "proj", "target", "branch", time.Now(), "user")

	err := s.CreateScheduledDeployment(ctx, "duplicate", "proj", "target", "branch", time.Now(), "user")
	if err != ErrDuplicate {
		t.Errorf("error = %v, want ErrDuplicate", err)
	}
}

func TestMemoryStore_ListPendingScheduledDeployments(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	// Past scheduled time (should be returned)
	s.CreateScheduledDeployment(ctx, "s1", "proj", "target", "branch", time.Now().Add(-time.Hour), "user")
	// Future scheduled time (should not be returned yet)
	s.CreateScheduledDeployment(ctx, "s2", "proj", "target", "branch", time.Now().Add(time.Hour), "user")

	pending, err := s.ListPendingScheduledDeployments(ctx)
	if err != nil {
		t.Fatalf("ListPendingScheduledDeployments() error = %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("len(pending) = %d, want 1", len(pending))
	}
	if pending[0].ID != "s1" {
		t.Errorf("ID = %s, want s1", pending[0].ID)
	}
}

func TestMemoryStore_CancelScheduledDeployment(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.CreateScheduledDeployment(ctx, "cancel-me", "proj", "target", "branch", time.Now().Add(-time.Hour), "user")

	err := s.CancelScheduledDeployment(ctx, "cancel-me")
	if err != nil {
		t.Fatalf("CancelScheduledDeployment() error = %v", err)
	}

	// Should no longer be in pending list
	pending, _ := s.ListPendingScheduledDeployments(ctx)
	for _, p := range pending {
		if p.ID == "cancel-me" {
			t.Error("Cancelled deployment still appears in pending list")
		}
	}
}

func TestMemoryStore_CancelScheduledDeployment_NotFound(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	err := s.CancelScheduledDeployment(context.Background(), "nonexistent")
	if err != ErrNotFound {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}
