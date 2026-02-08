package recipes

import (
	"context"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
)

func TestRawApprovalService_Approve_Success(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a RAW component
	component := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "raw-command",
		Version:       "v1.0.0",
		Name:          "Raw Command",
		ComponentType: storage.ComponentTypeCommand,
		IsRaw:         true,
		CreatedAt:     time.Now(),
	}
	if err := db.CreateRecipeComponent(ctx, component); err != nil {
		t.Fatalf("CreateRecipeComponent() error = %v", err)
	}

	svc := NewRawApprovalService(db)

	// Approve
	err := svc.Approve(ctx, component.ID, "user-123", "Reviewed and verified safe")
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}

	// Verify approval exists
	status, err := svc.GetApprovalStatus(ctx, component.ID)
	if err != nil {
		t.Fatalf("GetApprovalStatus() error = %v", err)
	}
	if !status.IsApproved {
		t.Error("IsApproved = false, want true")
	}
	if status.ApprovedBy != "user-123" {
		t.Errorf("ApprovedBy = %v, want user-123", status.ApprovedBy)
	}
	if status.ApprovalNote != "Reviewed and verified safe" {
		t.Errorf("ApprovalNote = %v, want 'Reviewed and verified safe'", status.ApprovalNote)
	}
}

func TestRawApprovalService_Approve_NotRaw(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a non-RAW component
	component := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "normal-command",
		Version:       "v1.0.0",
		Name:          "Normal Command",
		ComponentType: storage.ComponentTypeCommand,
		IsRaw:         false,
		CreatedAt:     time.Now(),
	}
	if err := db.CreateRecipeComponent(ctx, component); err != nil {
		t.Fatalf("CreateRecipeComponent() error = %v", err)
	}

	svc := NewRawApprovalService(db)

	// Attempt to approve - should fail
	err := svc.Approve(ctx, component.ID, "user-123", "test")
	if err == nil {
		t.Fatal("Approve() expected error for non-RAW component")
	}
}

func TestRawApprovalService_Approve_AlreadyApproved(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a RAW component
	component := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "already-approved",
		Version:       "v1.0.0",
		Name:          "Already Approved",
		ComponentType: storage.ComponentTypeCommand,
		IsRaw:         true,
		CreatedAt:     time.Now(),
	}
	if err := db.CreateRecipeComponent(ctx, component); err != nil {
		t.Fatalf("CreateRecipeComponent() error = %v", err)
	}

	svc := NewRawApprovalService(db)

	// First approval
	err := svc.Approve(ctx, component.ID, "user-123", "first approval")
	if err != nil {
		t.Fatalf("First Approve() error = %v", err)
	}

	// Second approval should fail
	err = svc.Approve(ctx, component.ID, "user-456", "second approval")
	if err == nil {
		t.Fatal("Second Approve() expected error")
	}
}

func TestRawApprovalService_RevokeApproval(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Create and approve a RAW component
	component := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "revoke-test",
		Version:       "v1.0.0",
		Name:          "Revoke Test",
		ComponentType: storage.ComponentTypeCommand,
		IsRaw:         true,
		CreatedAt:     time.Now(),
	}
	if err := db.CreateRecipeComponent(ctx, component); err != nil {
		t.Fatalf("CreateRecipeComponent() error = %v", err)
	}

	svc := NewRawApprovalService(db)

	// Approve first
	err := svc.Approve(ctx, component.ID, "user-123", "test")
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}

	// Revoke
	err = svc.RevokeApproval(ctx, component.ID, "user-456")
	if err != nil {
		t.Fatalf("RevokeApproval() error = %v", err)
	}

	// Verify revoked
	status, err := svc.GetApprovalStatus(ctx, component.ID)
	if err != nil {
		t.Fatalf("GetApprovalStatus() error = %v", err)
	}
	if status.IsApproved {
		t.Error("IsApproved = true, want false after revocation")
	}
}

func TestRawApprovalService_RequiresApproval(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a RAW component (not approved)
	rawComponent := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "needs-approval",
		Version:       "v1.0.0",
		Name:          "Needs Approval",
		ComponentType: storage.ComponentTypeCommand,
		IsRaw:         true,
		CreatedAt:     time.Now(),
	}
	if err := db.CreateRecipeComponent(ctx, rawComponent); err != nil {
		t.Fatalf("CreateRecipeComponent() error = %v", err)
	}

	// Create a non-RAW component
	normalComponent := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "no-approval",
		Version:       "v1.0.0",
		Name:          "No Approval Needed",
		ComponentType: storage.ComponentTypeCommand,
		IsRaw:         false,
		CreatedAt:     time.Now(),
	}
	if err := db.CreateRecipeComponent(ctx, normalComponent); err != nil {
		t.Fatalf("CreateRecipeComponent() error = %v", err)
	}

	svc := NewRawApprovalService(db)

	// RAW component should require approval
	requires, err := svc.RequiresApproval(ctx, rawComponent.ID)
	if err != nil {
		t.Fatalf("RequiresApproval() error = %v", err)
	}
	if !requires {
		t.Error("RequiresApproval() = false for unapproved RAW component")
	}

	// Non-RAW component should not require approval
	requires, err = svc.RequiresApproval(ctx, normalComponent.ID)
	if err != nil {
		t.Fatalf("RequiresApproval() error = %v", err)
	}
	if requires {
		t.Error("RequiresApproval() = true for non-RAW component")
	}

	// Approve RAW component
	err = svc.Approve(ctx, rawComponent.ID, "user-123", "approved")
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}

	// Now should not require approval
	requires, err = svc.RequiresApproval(ctx, rawComponent.ID)
	if err != nil {
		t.Fatalf("RequiresApproval() error = %v", err)
	}
	if requires {
		t.Error("RequiresApproval() = true for approved RAW component")
	}
}

func TestRawApprovalService_ListPendingApprovals(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Create RAW components
	raw1 := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "pending1",
		Version:       "v1.0.0",
		Name:          "Pending 1",
		ComponentType: storage.ComponentTypeCommand,
		IsRaw:         true,
		CreatedAt:     time.Now(),
	}
	raw2 := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "pending2",
		Version:       "v1.0.0",
		Name:          "Pending 2",
		ComponentType: storage.ComponentTypeCommand,
		IsRaw:         true,
		CreatedAt:     time.Now(),
	}
	normal := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "not-raw",
		Version:       "v1.0.0",
		Name:          "Not Raw",
		ComponentType: storage.ComponentTypeCommand,
		IsRaw:         false,
		CreatedAt:     time.Now(),
	}

	for _, c := range []*storage.RecipeComponent{raw1, raw2, normal} {
		if err := db.CreateRecipeComponent(ctx, c); err != nil {
			t.Fatalf("CreateRecipeComponent() error = %v", err)
		}
	}

	svc := NewRawApprovalService(db)

	// All RAW components should be pending
	pending, err := svc.ListPendingApprovals(ctx)
	if err != nil {
		t.Fatalf("ListPendingApprovals() error = %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("len(pending) = %v, want 2", len(pending))
	}

	// Approve one
	err = svc.Approve(ctx, raw1.ID, "user-123", "approved")
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}

	// Now only one should be pending
	pending, err = svc.ListPendingApprovals(ctx)
	if err != nil {
		t.Fatalf("ListPendingApprovals() error = %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("len(pending) = %v, want 1", len(pending))
	}
}
