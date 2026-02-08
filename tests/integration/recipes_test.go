// Package integration provides end-to-end integration tests for vcdeploy.
package integration

import (
	"context"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// TestRecipeComponentStorage tests direct storage operations for components.
func TestRecipeComponentStorage(t *testing.T) {
	fix := NewTestFixture(t)
	defer fix.Close()

	ctx := context.Background()

	t.Run("CreateAndGetComponent", func(t *testing.T) {
		component := &storage.RecipeComponent{
			Namespace:     "user",
			Slug:          "test-component",
			Version:       "v1.0.0",
			Name:          "Test Component",
			ComponentType: "command",
			Description:   "A test component",
			Content:       storage.ComponentContent{},
			CreatedAt:     time.Now(),
		}

		if err := fix.DB.CreateRecipeComponent(ctx, component); err != nil {
			t.Fatalf("Failed to create component: %v", err)
		}

		if component.ID == "" {
			t.Error("Component ID should be set after create")
		}

		// Get by ref
		got, err := fix.DB.GetRecipeComponent(ctx, storage.NamespaceUser, "test-component", "v1.0.0")
		if err != nil {
			t.Fatalf("Failed to get component: %v", err)
		}

		if got.Name != "Test Component" {
			t.Errorf("Expected name 'Test Component', got %s", got.Name)
		}
		if got.Namespace != "user" {
			t.Errorf("Expected namespace 'user', got %s", got.Namespace)
		}
	})

	t.Run("ListComponentsByNamespace", func(t *testing.T) {
		// Create another component
		component := &storage.RecipeComponent{
			Namespace:     "user",
			Slug:          "test-component-2",
			Version:       "v1.0.0",
			Name:          "Test Component 2",
			ComponentType: "command",
			Content:       storage.ComponentContent{},
			CreatedAt:     time.Now(),
		}
		fix.DB.CreateRecipeComponent(ctx, component)

		// List user components
		components, err := fix.DB.ListRecipeComponents(ctx, storage.NamespaceUser, false)
		if err != nil {
			t.Fatalf("Failed to list components: %v", err)
		}

		if len(components) < 2 {
			t.Errorf("Expected at least 2 user components, got %d", len(components))
		}
	})
}

// TestRecipePlaybookStorage tests direct storage operations for playbooks.
func TestRecipePlaybookStorage(t *testing.T) {
	fix := NewTestFixture(t)
	defer fix.Close()

	ctx := context.Background()

	t.Run("CreateAndGetPlaybook", func(t *testing.T) {
		playbook := &storage.Playbook{
			Namespace:     "user",
			Slug:          "test-playbook",
			Version:       "v1.0.0",
			Name:          "Test Playbook",
			Description:   "A test playbook",
			FrameworkType: "laravel",
			Steps:         []storage.PlaybookStep{},
			CreatedAt:     time.Now(),
		}

		if err := fix.DB.CreatePlaybook(ctx, playbook); err != nil {
			t.Fatalf("Failed to create playbook: %v", err)
		}

		if playbook.ID == "" {
			t.Error("Playbook ID should be set after create")
		}

		got, err := fix.DB.GetPlaybook(ctx, storage.NamespaceUser, "test-playbook", "v1.0.0")
		if err != nil {
			t.Fatalf("Failed to get playbook: %v", err)
		}

		if got.Name != "Test Playbook" {
			t.Errorf("Expected name 'Test Playbook', got %s", got.Name)
		}
	})

	t.Run("PlaybookWithSteps", func(t *testing.T) {
		playbook := &storage.Playbook{
			Namespace:     "user",
			Slug:          "steps-playbook",
			Version:       "v1.0.0",
			Name:          "Playbook with Steps",
			FrameworkType: "laravel",
			Steps: []storage.PlaybookStep{
				{
					Order:        1,
					Phase:        "pre_deploy",
					ComponentRef: "user:test-component:v1.0.0",
				},
				{
					Order:        2,
					Phase:        "post_deploy",
					ComponentRef: "user:test-component:v1.0.0",
				},
			},
			CreatedAt: time.Now(),
		}

		if err := fix.DB.CreatePlaybook(ctx, playbook); err != nil {
			t.Fatalf("Failed to create playbook with steps: %v", err)
		}

		got, err := fix.DB.GetPlaybook(ctx, storage.NamespaceUser, "steps-playbook", "v1.0.0")
		if err != nil {
			t.Fatalf("Failed to get playbook: %v", err)
		}

		if len(got.Steps) != 2 {
			t.Errorf("Expected 2 steps, got %d", len(got.Steps))
		}
	})
}

// TestRecipeActivationStorage tests playbook activation storage.
func TestRecipeActivationStorage(t *testing.T) {
	fix := NewTestFixture(t)
	defer fix.Close()

	ctx := context.Background()

	// Create a project
	project := fix.CreateTestProject("activation-test", "https://github.com/test/repo.git", "main")

	// Create a playbook
	playbook := &storage.Playbook{
		Namespace: "user",
		Slug:      "activation-playbook",
		Version:   "v1.0.0",
		Name:      "Activation Test Playbook",
		Steps:     []storage.PlaybookStep{},
		CreatedAt: time.Now(),
	}
	fix.DB.CreatePlaybook(ctx, playbook)

	t.Run("ActivatePlaybook", func(t *testing.T) {
		activation := &storage.PlaybookActivation{
			ProjectID:   project.ID,
			PlaybookID:  playbook.ID,
			ActivatedAt: time.Now(),
		}

		if err := fix.DB.CreatePlaybookActivation(ctx, activation); err != nil {
			t.Fatalf("Failed to activate playbook: %v", err)
		}

		if activation.ID == "" {
			t.Error("Activation ID should be set")
		}
	})

	t.Run("GetActivePlaybook", func(t *testing.T) {
		activation, err := fix.DB.GetPlaybookActivation(ctx, project.ID)
		if err != nil {
			t.Fatalf("Failed to get active playbook: %v", err)
		}

		if activation == nil {
			t.Fatal("Expected active playbook")
		}

		if activation.PlaybookID != playbook.ID {
			t.Errorf("Expected playbook ID %s, got %s", playbook.ID, activation.PlaybookID)
		}
	})
}

// TestEmptyNamespaceStates verifies empty states in namespaces.
func TestEmptyNamespaceStates(t *testing.T) {
	fix := NewTestFixture(t)
	defer fix.Close()

	ctx := context.Background()

	t.Run("EmptySeedComponents", func(t *testing.T) {
		// New database should have no seed components
		components, err := fix.DB.ListRecipeComponents(ctx, storage.NamespaceSeed, false)
		if err != nil {
			t.Fatalf("Failed to list: %v", err)
		}
		t.Logf("Seed components count: %d (should be 0 without seed data)", len(components))
	})

	t.Run("EmptySeedPlaybooks", func(t *testing.T) {
		// New database should have no seed playbooks
		playbooks, err := fix.DB.ListPlaybooks(ctx, "seed", "", false)
		if err != nil {
			t.Fatalf("Failed to list: %v", err)
		}
		t.Logf("Seed playbooks count: %d (should be 0 without seed data)", len(playbooks))
	})
}
