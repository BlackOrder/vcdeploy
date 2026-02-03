package recipes

import (
	"context"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
)

func TestActivationService_Activate(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a component
	component := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "test-cmd",
		Version:       "v1.0.0",
		Name:          "Test Command",
		ComponentType: storage.ComponentTypeCommand,
		CreatedAt:     time.Now(),
	}
	if err := db.CreateRecipeComponent(ctx, component); err != nil {
		t.Fatalf("CreateRecipeComponent() error = %v", err)
	}

	// Create a playbook
	playbook := &storage.Playbook{
		Namespace:     storage.NamespaceUser,
		Slug:          "test-playbook",
		Version:       "v1.0.0",
		Name:          "Test Playbook",
		FrameworkType: "generic",
		Steps: []storage.PlaybookStep{
			{
				ComponentRef: "user:test-cmd:v1.0.0",
				Phase:        storage.PhaseDeploy,
				Order:        1,
			},
		},
		CreatedAt: time.Now(),
	}
	if err := db.CreatePlaybook(ctx, playbook); err != nil {
		t.Fatalf("CreatePlaybook() error = %v", err)
	}

	svc := NewActivationService(db)

	// Activate the playbook for a project
	activation, err := svc.Activate(ctx, 1, playbook.ID, map[string]VariableBinding{}, nil)
	if err != nil {
		t.Fatalf("Activate() error = %v", err)
	}

	if activation.ProjectID != 1 {
		t.Errorf("ProjectID = %v, want 1", activation.ProjectID)
	}
	if activation.PlaybookID != playbook.ID {
		t.Errorf("PlaybookID = %v, want %v", activation.PlaybookID, playbook.ID)
	}
}

func TestActivationService_Activate_MissingVariable(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a component with required variable
	component := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "needs-var",
		Version:       "v1.0.0",
		Name:          "Needs Variable",
		ComponentType: storage.ComponentTypeCommand,
		Variables: []storage.VariableDefinition{
			{Name: "DB_HOST", Type: "string", Required: true},
		},
		CreatedAt: time.Now(),
	}
	if err := db.CreateRecipeComponent(ctx, component); err != nil {
		t.Fatalf("CreateRecipeComponent() error = %v", err)
	}

	// Create a playbook using it
	playbook := &storage.Playbook{
		Namespace:     storage.NamespaceUser,
		Slug:          "needs-var-playbook",
		Version:       "v1.0.0",
		Name:          "Needs Variable Playbook",
		FrameworkType: "generic",
		Steps: []storage.PlaybookStep{
			{
				ComponentRef: "user:needs-var:v1.0.0",
				Phase:        storage.PhaseDeploy,
				Order:        1,
			},
		},
		CreatedAt: time.Now(),
	}
	if err := db.CreatePlaybook(ctx, playbook); err != nil {
		t.Fatalf("CreatePlaybook() error = %v", err)
	}

	svc := NewActivationService(db)

	// Activate without providing required variable should fail
	_, err := svc.Activate(ctx, 1, playbook.ID, map[string]VariableBinding{}, nil)
	if err == nil {
		t.Fatal("Activate() expected error for missing required variable")
	}

	// Activate with variable should succeed
	_, err = svc.Activate(ctx, 1, playbook.ID, map[string]VariableBinding{
		"DB_HOST": {SourceType: storage.SourceTypeLiteral, LiteralValue: "localhost"},
	}, nil)
	if err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
}

func TestActivationService_Activate_RAWRequiresApproval(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a RAW component (requires admin approval)
	component := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "raw-cmd",
		Version:       "v1.0.0",
		Name:          "Raw Command",
		ComponentType: storage.ComponentTypeCommand,
		IsRaw:         true,
		CreatedAt:     time.Now(),
	}
	if err := db.CreateRecipeComponent(ctx, component); err != nil {
		t.Fatalf("CreateRecipeComponent() error = %v", err)
	}

	// Create a playbook using it
	playbook := &storage.Playbook{
		Namespace:     storage.NamespaceUser,
		Slug:          "raw-playbook",
		Version:       "v1.0.0",
		Name:          "Raw Playbook",
		FrameworkType: "generic",
		Steps: []storage.PlaybookStep{
			{
				ComponentRef: "user:raw-cmd:v1.0.0",
				Phase:        storage.PhaseDeploy,
				Order:        1,
			},
		},
		CreatedAt: time.Now(),
	}
	if err := db.CreatePlaybook(ctx, playbook); err != nil {
		t.Fatalf("CreatePlaybook() error = %v", err)
	}

	svc := NewActivationService(db)

	// Activate without approval should fail
	_, err := svc.Activate(ctx, 1, playbook.ID, map[string]VariableBinding{}, nil)
	if err == nil {
		t.Fatal("Activate() expected error for RAW component without approval")
	}

	// Add approval
	approval := &storage.RawCommandApproval{
		ComponentID: component.ID,
		ApprovedAt:  time.Now(),
	}
	if err := db.CreateRawApproval(ctx, approval); err != nil {
		t.Fatalf("CreateRawApproval() error = %v", err)
	}

	// Now activation should succeed
	_, err = svc.Activate(ctx, 1, playbook.ID, map[string]VariableBinding{}, nil)
	if err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
}

func TestActivationService_GetActive(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a component
	component := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "get-active-test",
		Version:       "v1.0.0",
		Name:          "Test",
		ComponentType: storage.ComponentTypeCommand,
		CreatedAt:     time.Now(),
	}
	if err := db.CreateRecipeComponent(ctx, component); err != nil {
		t.Fatalf("CreateRecipeComponent() error = %v", err)
	}

	// Create a playbook
	playbook := &storage.Playbook{
		Namespace:     storage.NamespaceUser,
		Slug:          "get-active-playbook",
		Version:       "v1.0.0",
		Name:          "Test Playbook",
		FrameworkType: "generic",
		Steps: []storage.PlaybookStep{
			{
				ComponentRef: "user:get-active-test:v1.0.0",
				Phase:        storage.PhaseDeploy,
				Order:        1,
			},
		},
		CreatedAt: time.Now(),
	}
	if err := db.CreatePlaybook(ctx, playbook); err != nil {
		t.Fatalf("CreatePlaybook() error = %v", err)
	}

	svc := NewActivationService(db)

	// Activate with bindings
	bindings := map[string]VariableBinding{
		"TEST_VAR": {SourceType: storage.SourceTypeLiteral, LiteralValue: "test_value"},
	}
	_, err := svc.Activate(ctx, 123, playbook.ID, bindings, nil)
	if err != nil {
		t.Fatalf("Activate() error = %v", err)
	}

	// Get active should return the activation with bindings
	activation, err := svc.GetActive(ctx, 123)
	if err != nil {
		t.Fatalf("GetActive() error = %v", err)
	}

	if activation.ProjectID != 123 {
		t.Errorf("ProjectID = %v, want 123", activation.ProjectID)
	}
	if len(activation.Bindings) != 1 {
		t.Errorf("len(Bindings) = %v, want 1", len(activation.Bindings))
	}
}

func TestActivationService_Deactivate(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a component
	component := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "deactivate-test",
		Version:       "v1.0.0",
		Name:          "Test",
		ComponentType: storage.ComponentTypeCommand,
		CreatedAt:     time.Now(),
	}
	if err := db.CreateRecipeComponent(ctx, component); err != nil {
		t.Fatalf("CreateRecipeComponent() error = %v", err)
	}

	// Create a playbook
	playbook := &storage.Playbook{
		Namespace:     storage.NamespaceUser,
		Slug:          "deactivate-playbook",
		Version:       "v1.0.0",
		Name:          "Test Playbook",
		FrameworkType: "generic",
		Steps: []storage.PlaybookStep{
			{
				ComponentRef: "user:deactivate-test:v1.0.0",
				Phase:        storage.PhaseDeploy,
				Order:        1,
			},
		},
		CreatedAt: time.Now(),
	}
	if err := db.CreatePlaybook(ctx, playbook); err != nil {
		t.Fatalf("CreatePlaybook() error = %v", err)
	}

	svc := NewActivationService(db)

	// Activate
	_, err := svc.Activate(ctx, 456, playbook.ID, map[string]VariableBinding{}, nil)
	if err != nil {
		t.Fatalf("Activate() error = %v", err)
	}

	// Deactivate
	err = svc.Deactivate(ctx, 456)
	if err != nil {
		t.Fatalf("Deactivate() error = %v", err)
	}

	// GetActive should now fail
	_, err = svc.GetActive(ctx, 456)
	if err == nil {
		t.Fatal("GetActive() expected error after deactivation")
	}
}
