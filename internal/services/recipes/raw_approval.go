package recipes

import (
	"context"
	"fmt"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// RawApprovalService handles RAW command approvals.
type RawApprovalService struct {
	store        storage.Store
	auditService services.AuditServicer
}

// NewRawApprovalService creates a new approval service.
func NewRawApprovalService(store storage.Store) *RawApprovalService {
	return &RawApprovalService{store: store}
}

// NewRawApprovalServiceWithAudit creates a new approval service with audit logging.
func NewRawApprovalServiceWithAudit(store storage.Store, auditService services.AuditServicer) *RawApprovalService {
	return &RawApprovalService{
		store:        store,
		auditService: auditService,
	}
}

// ApprovalStatus represents the approval state of a component.
type ApprovalStatus struct {
	ComponentID  int64      `json:"component_id"`
	IsRaw        bool       `json:"is_raw"`
	IsApproved   bool       `json:"is_approved"`
	ApprovedBy   int64      `json:"approved_by,omitempty"`
	ApprovedAt   *time.Time `json:"approved_at,omitempty"`
	ApprovalNote string     `json:"approval_note,omitempty"`
}

// RequiresApproval checks if a component needs approval.
func (s *RawApprovalService) RequiresApproval(ctx context.Context, componentID int64) (bool, error) {
	component, err := s.store.GetRecipeComponentByID(ctx, componentID)
	if err != nil {
		return false, err
	}
	if component == nil {
		return false, fmt.Errorf("component not found: %d", componentID)
	}

	if !component.IsRaw {
		return false, nil
	}

	approval, err := s.store.GetRawApproval(ctx, componentID)
	if err != nil {
		// Treat errors as "not approved" for safety
		return true, nil //nolint:nilerr // Errors mean approval unknown - treat as needing approval
	}
	if approval == nil {
		return true, nil
	}

	return false, nil
}

// Approve records admin approval for a RAW component.
func (s *RawApprovalService) Approve(ctx context.Context, componentID, adminUserID int64, note string) error {
	component, err := s.store.GetRecipeComponentByID(ctx, componentID)
	if err != nil {
		return fmt.Errorf("component not found: %w", err)
	}
	if component == nil {
		return fmt.Errorf("component not found: %d", componentID)
	}

	if !component.IsRaw {
		return fmt.Errorf("component is not marked as RAW")
	}

	// Check if already approved
	existing, _ := s.store.GetRawApproval(ctx, componentID)
	if existing != nil {
		return fmt.Errorf("component already approved")
	}

	approval := &storage.RawCommandApproval{
		ComponentID:  componentID,
		ApprovedBy:   adminUserID,
		ApprovedAt:   time.Now(),
		ApprovalNote: note,
	}

	if err := s.store.CreateRawApproval(ctx, approval); err != nil {
		return fmt.Errorf("failed to create approval: %w", err)
	}

	// Audit logging for RAW component approval
	if s.auditService != nil {
		_ = s.auditService.Log(ctx, &storage.AuditEntry{
			Source:     "raw_approval",
			Action:     "raw_component_approved",
			Resource:   "component",
			ResourceID: fmt.Sprintf("%d", componentID),
			Details:    fmt.Sprintf("user_id=%d, component=%s:%s, note=%s", adminUserID, component.Namespace, component.Slug, note),
		})
	}

	return nil
}

// RevokeApproval removes approval from a RAW component.
func (s *RawApprovalService) RevokeApproval(ctx context.Context, componentID, adminUserID int64) error {
	// Verify component exists and is RAW
	component, err := s.store.GetRecipeComponentByID(ctx, componentID)
	if err != nil {
		return fmt.Errorf("component not found: %w", err)
	}
	if component == nil {
		return fmt.Errorf("component not found: %d", componentID)
	}

	if !component.IsRaw {
		return fmt.Errorf("component is not marked as RAW")
	}

	// Verify approval exists
	existing, err := s.store.GetRawApproval(ctx, componentID)
	if err != nil || existing == nil {
		return fmt.Errorf("no approval found for component %d", componentID)
	}

	if err := s.store.DeleteRawApproval(ctx, componentID); err != nil {
		return fmt.Errorf("failed to delete approval: %w", err)
	}

	// Audit logging for RAW component approval revocation
	if s.auditService != nil {
		_ = s.auditService.Log(ctx, &storage.AuditEntry{
			Source:     "raw_approval",
			Action:     "raw_component_revoked",
			Resource:   "component",
			ResourceID: fmt.Sprintf("%d", componentID),
			Details:    fmt.Sprintf("user_id=%d, component=%s:%s", adminUserID, component.Namespace, component.Slug),
		})
	}

	return nil
}

// GetApprovalStatus returns current approval status.
func (s *RawApprovalService) GetApprovalStatus(ctx context.Context, componentID int64) (*ApprovalStatus, error) {
	component, err := s.store.GetRecipeComponentByID(ctx, componentID)
	if err != nil {
		return nil, err
	}
	if component == nil {
		return nil, fmt.Errorf("component not found: %d", componentID)
	}

	status := &ApprovalStatus{
		ComponentID: componentID,
		IsRaw:       component.IsRaw,
		IsApproved:  false,
	}

	if !component.IsRaw {
		return status, nil
	}

	approval, err := s.store.GetRawApproval(ctx, componentID)
	if err == nil && approval != nil {
		status.IsApproved = true
		status.ApprovedBy = approval.ApprovedBy
		status.ApprovedAt = &approval.ApprovedAt
		status.ApprovalNote = approval.ApprovalNote
	}

	return status, nil
}

// ListPendingApprovals returns all RAW components that need approval.
func (s *RawApprovalService) ListPendingApprovals(ctx context.Context) ([]*storage.RecipeComponent, error) {
	// Get all components (both seed and user namespaces)
	seedComponents, err := s.store.ListRecipeComponents(ctx, storage.NamespaceSeed, true)
	if err != nil {
		return nil, err
	}
	userComponents, err := s.store.ListRecipeComponents(ctx, storage.NamespaceUser, true)
	if err != nil {
		return nil, err
	}

	allComponents := make([]*storage.RecipeComponent, 0, len(seedComponents)+len(userComponents))
	allComponents = append(allComponents, seedComponents...)
	allComponents = append(allComponents, userComponents...)
	var pending []*storage.RecipeComponent

	for _, c := range allComponents {
		if !c.IsRaw {
			continue
		}

		approval, _ := s.store.GetRawApproval(ctx, c.ID)
		if approval == nil {
			pending = append(pending, c)
		}
	}

	return pending, nil
}

// ListAllApprovals returns all current RAW command approvals.
func (s *RawApprovalService) ListAllApprovals(ctx context.Context) ([]*storage.RawCommandApproval, error) {
	return s.store.ListRawApprovals(ctx)
}
