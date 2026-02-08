package recipes

import (
	"context"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
)

func TestDependencyService_FindSecretUsages(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a component
	component := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "dep-test-cmd",
		Version:       "v1.0.0",
		Name:          "Dependency Test",
		ComponentType: storage.ComponentTypeCommand,
		Variables: []storage.VariableDefinition{
			{Name: "DB_PASSWORD", Type: "string", Required: true, Sensitive: true},
		},
		CreatedAt: time.Now(),
	}
	if err := db.CreateRecipeComponent(ctx, component); err != nil {
		t.Fatalf("CreateRecipeComponent() error = %v", err)
	}

	// Create a playbook
	playbook := &storage.Playbook{
		Namespace:     storage.NamespaceUser,
		Slug:          "dep-test-playbook",
		Version:       "v1.0.0",
		Name:          "Dependency Test Playbook",
		FrameworkType: "generic",
		Steps: []storage.PlaybookStep{
			{
				ComponentRef: "user:dep-test-cmd:v1.0.0",
				Phase:        storage.PhaseDeploy,
				Order:        1,
			},
		},
		CreatedAt: time.Now(),
	}
	if err := db.CreatePlaybook(ctx, playbook); err != nil {
		t.Fatalf("CreatePlaybook() error = %v", err)
	}

	// Activate with a secret binding
	actSvc := NewActivationService(db)
	_, err := actSvc.Activate(ctx, "test-project-id", playbook.ID, map[string]VariableBinding{
		"DB_PASSWORD": {SourceType: storage.SourceTypeSecret, SourceRef: "my-db-password"},
	}, nil)
	if err != nil {
		t.Fatalf("Activate() error = %v", err)
	}

	// Check for secret usages
	depSvc := NewDependencyService(db)
	usages, err := depSvc.FindSecretUsages(ctx, "my-db-password")
	if err != nil {
		t.Fatalf("FindSecretUsages() error = %v", err)
	}

	if len(usages) != 1 {
		t.Errorf("len(usages) = %v, want 1", len(usages))
	}
	if usages[0].VariableName != "DB_PASSWORD" {
		t.Errorf("VariableName = %v, want DB_PASSWORD", usages[0].VariableName)
	}
}

func TestDependencyService_CheckDeletionSafe(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Create component
	component := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "safe-test-cmd",
		Version:       "v1.0.0",
		Name:          "Safe Test",
		ComponentType: storage.ComponentTypeCommand,
		Variables: []storage.VariableDefinition{
			{Name: "API_KEY", Type: "string", Required: true},
		},
		CreatedAt: time.Now(),
	}
	if err := db.CreateRecipeComponent(ctx, component); err != nil {
		t.Fatalf("CreateRecipeComponent() error = %v", err)
	}

	// Create playbook
	playbook := &storage.Playbook{
		Namespace:     storage.NamespaceUser,
		Slug:          "safe-test-playbook",
		Version:       "v1.0.0",
		Name:          "Safe Test Playbook",
		FrameworkType: "generic",
		Steps: []storage.PlaybookStep{
			{
				ComponentRef: "user:safe-test-cmd:v1.0.0",
				Phase:        storage.PhaseDeploy,
				Order:        1,
			},
		},
		CreatedAt: time.Now(),
	}
	if err := db.CreatePlaybook(ctx, playbook); err != nil {
		t.Fatalf("CreatePlaybook() error = %v", err)
	}

	// Activate with secret binding
	actSvc := NewActivationService(db)
	_, err := actSvc.Activate(ctx, "test-project-id-2", playbook.ID, map[string]VariableBinding{
		"API_KEY": {SourceType: storage.SourceTypeSecret, SourceRef: "production-api-key"},
	}, nil)
	if err != nil {
		t.Fatalf("Activate() error = %v", err)
	}

	depSvc := NewDependencyService(db)

	// Check deletion - should be blocked
	err = depSvc.CheckDeletionSafe(ctx, storage.SourceTypeSecret, "production-api-key")
	if err == nil {
		t.Fatal("CheckDeletionSafe() expected error for used secret")
	}

	if !IsDeletionBlockedError(err) {
		t.Errorf("expected DeletionBlockedError, got %T", err)
	}

	// Check deletion of unused secret - should be safe
	err = depSvc.CheckDeletionSafe(ctx, storage.SourceTypeSecret, "unused-secret")
	if err != nil {
		t.Fatalf("CheckDeletionSafe() error = %v for unused secret", err)
	}
}

func TestDeletionBlockedError_GetUsageDetails(t *testing.T) {
	err := &DeletionBlockedError{
		ResourceType: "secret",
		ResourceName: "my-secret",
		Usages: []*ResourceUsage{
			{
				ProjectID:    "project-1",
				ProjectName:  "Project Alpha",
				PlaybookID:   "playbook-10",
				PlaybookName: "Deploy Alpha",
				VariableName: "DB_PASSWORD",
			},
			{
				ProjectID:    "project-2",
				ProjectName:  "Project Beta",
				PlaybookID:   "playbook-20",
				PlaybookName: "Deploy Beta",
				VariableName: "DB_PASSWORD",
			},
		},
	}

	details := err.GetUsageDetails()
	if len(details) != 2 {
		t.Errorf("len(details) = %v, want 2", len(details))
	}

	// Check error message
	msg := err.Error()
	if msg != `cannot delete secret "my-secret": used by 2 playbook(s)` {
		t.Errorf("Error() = %v", msg)
	}
}
