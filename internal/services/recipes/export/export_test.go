package export

import (
	"context"
	"testing"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// setupTestDB creates a test database for export tests.
func setupTestDB(t *testing.T) storage.Store {
	t.Helper()
	db, err := storage.New(":memory:", zap.NewNop())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestExporter_ExportAll(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	// Create test data
	comp := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "my-comp",
		Version:       "v1.0.0",
		Name:          "My Component",
		ComponentType: storage.ComponentTypeCommand,
		Content: storage.ComponentContent{
			Commands: []string{"echo test"},
		},
	}
	if err := store.CreateRecipeComponent(ctx, comp); err != nil {
		t.Fatalf("failed to create component: %v", err)
	}

	playbook := &storage.Playbook{
		Namespace:     storage.NamespaceUser,
		Slug:          "my-playbook",
		Version:       "v1.0.0",
		Name:          "My Playbook",
		FrameworkType: "generic",
		Steps:         []storage.PlaybookStep{},
	}
	if err := store.CreatePlaybook(ctx, playbook); err != nil {
		t.Fatalf("failed to create playbook: %v", err)
	}

	exporter := NewExporter(store, "v0.1.0")

	bundle, err := exporter.ExportAll(ctx)
	if err != nil {
		t.Fatalf("ExportAll failed: %v", err)
	}

	// Verify bundle structure
	if bundle.FormatVersion != FormatVersion {
		t.Errorf("expected format version %s, got %s", FormatVersion, bundle.FormatVersion)
	}
	if bundle.VCDeployVersion != "v0.1.0" {
		t.Errorf("expected vcdeploy version 'v0.1.0', got '%s'", bundle.VCDeployVersion)
	}
	if bundle.ExportedAt.IsZero() {
		t.Error("expected non-zero exported_at")
	}
	if len(bundle.Components) != 1 {
		t.Errorf("expected 1 component, got %d", len(bundle.Components))
	}
	if len(bundle.Playbooks) != 1 {
		t.Errorf("expected 1 playbook, got %d", len(bundle.Playbooks))
	}
}

func TestExporter_ExportComponents(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	// Create test components
	comp1 := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "test-comp",
		Version:       "v1.0.0",
		Name:          "Test Component",
		Description:   "A test component",
		ComponentType: storage.ComponentTypeCommand,
		Content: storage.ComponentContent{
			Commands: []string{"echo hello"},
		},
		Variables: []storage.VariableDefinition{
			{Name: "VAR1", Type: "string", Required: true},
		},
	}
	comp2 := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "test-comp",
		Version:       "v1.1.0",
		Name:          "Test Component v1.1",
		Description:   "Updated test component",
		ComponentType: storage.ComponentTypeCommand,
		Content: storage.ComponentContent{
			Commands: []string{"echo hello v1.1"},
		},
	}

	if err := store.CreateRecipeComponent(ctx, comp1); err != nil {
		t.Fatalf("failed to create comp1: %v", err)
	}
	if err := store.CreateRecipeComponent(ctx, comp2); err != nil {
		t.Fatalf("failed to create comp2: %v", err)
	}

	exporter := NewExporter(store, "v0.1.0")

	bundle, err := exporter.ExportComponents(ctx, []string{"test-comp"})
	if err != nil {
		t.Fatalf("ExportComponents failed: %v", err)
	}

	if len(bundle.Components) != 2 {
		t.Errorf("expected 2 exports, got %d", len(bundle.Components))
	}

	// Check export content
	found := make(map[string]bool)
	for _, exp := range bundle.Components {
		found[exp.Slug+":"+exp.Version] = true
		if exp.Slug != "test-comp" {
			t.Errorf("unexpected slug: %s", exp.Slug)
		}
	}

	if !found["test-comp:v1.0.0"] {
		t.Error("missing v1.0.0 export")
	}
	if !found["test-comp:v1.1.0"] {
		t.Error("missing v1.1.0 export")
	}
}

func TestExporter_ExportPlaybooks(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	// Create a component first (for playbook steps)
	comp := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "deploy-script",
		Version:       "v1.0.0",
		Name:          "Deploy Script",
		ComponentType: storage.ComponentTypeCommand,
		Content: storage.ComponentContent{
			Commands: []string{"echo deploy"},
		},
	}
	if err := store.CreateRecipeComponent(ctx, comp); err != nil {
		t.Fatalf("failed to create component: %v", err)
	}

	// Create test playbook
	playbook := &storage.Playbook{
		Namespace:     storage.NamespaceUser,
		Slug:          "test-playbook",
		Version:       "v1.0.0",
		Name:          "Test Playbook",
		Description:   "A test playbook",
		FrameworkType: "generic",
		Steps: []storage.PlaybookStep{
			{Order: 1, ComponentRef: "user:deploy-script:v1.0.0", Phase: "deploy"},
		},
		SharedDirs:   []string{"/shared"},
		SharedFiles:  []string{},
		WritableDirs: []string{"/writable"},
		KeepReleases: 5,
	}

	if err := store.CreatePlaybook(ctx, playbook); err != nil {
		t.Fatalf("failed to create playbook: %v", err)
	}

	exporter := NewExporter(store, "v0.1.0")

	bundle, err := exporter.ExportPlaybooks(ctx, []string{"test-playbook"})
	if err != nil {
		t.Fatalf("ExportPlaybooks failed: %v", err)
	}

	if len(bundle.Playbooks) != 1 {
		t.Errorf("expected 1 export, got %d", len(bundle.Playbooks))
	}

	exp := bundle.Playbooks[0]
	if exp.Slug != "test-playbook" {
		t.Errorf("expected slug 'test-playbook', got '%s'", exp.Slug)
	}
	if exp.Version != "v1.0.0" {
		t.Errorf("expected version 'v1.0.0', got '%s'", exp.Version)
	}
	if len(exp.Steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(exp.Steps))
	}
}

func TestExporter_ExcludesSeedComponents(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	// Create user component
	userComp := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "user-comp",
		Version:       "v1.0.0",
		Name:          "User Component",
		ComponentType: storage.ComponentTypeCommand,
		Content: storage.ComponentContent{
			Commands: []string{"echo user"},
		},
	}

	// Create seed component
	seedComp := &storage.RecipeComponent{
		Namespace:     storage.NamespaceSeed,
		Slug:          "seed-comp",
		Version:       "v1.0.0",
		Name:          "Seed Component",
		ComponentType: storage.ComponentTypeCommand,
		Content: storage.ComponentContent{
			Commands: []string{"echo seed"},
		},
		IsSeed: true,
	}

	if err := store.CreateRecipeComponent(ctx, userComp); err != nil {
		t.Fatalf("failed to create user component: %v", err)
	}
	if err := store.CreateRecipeComponent(ctx, seedComp); err != nil {
		t.Fatalf("failed to create seed component: %v", err)
	}

	exporter := NewExporter(store, "v0.1.0")

	bundle, err := exporter.ExportAll(ctx)
	if err != nil {
		t.Fatalf("ExportAll failed: %v", err)
	}

	// Should only export user component, not seed
	if len(bundle.Components) != 1 {
		t.Errorf("expected 1 export (user only), got %d", len(bundle.Components))
	}
	if len(bundle.Components) > 0 && bundle.Components[0].Slug != "user-comp" {
		t.Errorf("expected 'user-comp', got '%s'", bundle.Components[0].Slug)
	}
}
