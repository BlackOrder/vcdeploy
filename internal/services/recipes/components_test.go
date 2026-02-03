package recipes

import (
	"context"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// setupTestDB creates a test database for services tests.
func setupTestDB(t *testing.T) storage.Store {
	t.Helper()
	db, err := storage.New(":memory:", zap.NewNop())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestComponentService_Create_ValidSemver(t *testing.T) {
	db := setupTestDB(t)
	svc := NewComponentService(db)
	ctx := context.Background()

	component := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "test-component",
		Version:       "v1.0.0",
		Name:          "Test Component",
		Description:   "A test component",
		ComponentType: storage.ComponentTypeCommand,
		Content: storage.ComponentContent{
			Commands: []string{"echo hello"},
		},
	}

	err := svc.Create(ctx, component)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Verify it was created
	got, err := svc.Get(ctx, storage.NamespaceUser, "test-component", "v1.0.0")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Name != "Test Component" {
		t.Errorf("Name = %v, want %v", got.Name, "Test Component")
	}
}

func TestComponentService_Create_InvalidSemver(t *testing.T) {
	db := setupTestDB(t)
	svc := NewComponentService(db)
	ctx := context.Background()

	component := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "test-component",
		Version:       "invalid-version",
		Name:          "Test Component",
		ComponentType: storage.ComponentTypeCommand,
	}

	err := svc.Create(ctx, component)
	if err == nil {
		t.Fatal("Create() expected error for invalid semver")
	}
}

func TestComponentService_Create_WrongNamespace(t *testing.T) {
	db := setupTestDB(t)
	svc := NewComponentService(db)
	ctx := context.Background()

	component := &storage.RecipeComponent{
		Namespace:     storage.NamespaceSeed, // Cannot create in seed namespace
		Slug:          "test-component",
		Version:       "v1.0.0",
		Name:          "Test Component",
		ComponentType: storage.ComponentTypeCommand,
	}

	err := svc.Create(ctx, component)
	if err == nil {
		t.Fatal("Create() expected error for seed namespace")
	}
}

func TestComponentService_GetLatest_MultipleVersions(t *testing.T) {
	db := setupTestDB(t)
	svc := NewComponentService(db)
	ctx := context.Background()

	// Create multiple versions
	versions := []string{"v1.0.0", "v2.0.0", "v1.5.0", "v2.1.0"}
	for _, v := range versions {
		component := &storage.RecipeComponent{
			Namespace:     storage.NamespaceUser,
			Slug:          "multi-version",
			Version:       v,
			Name:          "Version " + v,
			ComponentType: storage.ComponentTypeCommand,
		}
		if err := svc.Create(ctx, component); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	// Get latest should return v2.1.0
	latest, err := svc.GetLatest(ctx, storage.NamespaceUser, "multi-version")
	if err != nil {
		t.Fatalf("GetLatest() error = %v", err)
	}
	if latest.Version != "v2.1.0" {
		t.Errorf("GetLatest() Version = %v, want v2.1.0", latest.Version)
	}
}

func TestComponentService_GetVersions_Sorted(t *testing.T) {
	db := setupTestDB(t)
	svc := NewComponentService(db)
	ctx := context.Background()

	// Create versions in random order
	versions := []string{"v1.0.0", "v3.0.0", "v2.0.0"}
	for _, v := range versions {
		component := &storage.RecipeComponent{
			Namespace:     storage.NamespaceUser,
			Slug:          "sorted-test",
			Version:       v,
			Name:          "Version " + v,
			ComponentType: storage.ComponentTypeCommand,
		}
		if err := svc.Create(ctx, component); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	// Get versions should be sorted descending
	got, err := svc.GetVersions(ctx, storage.NamespaceUser, "sorted-test")
	if err != nil {
		t.Fatalf("GetVersions() error = %v", err)
	}

	expected := []string{"v3.0.0", "v2.0.0", "v1.0.0"}
	for i, v := range got {
		if v.Version != expected[i] {
			t.Errorf("GetVersions()[%d] = %v, want %v", i, v.Version, expected[i])
		}
	}
}

func TestComponentService_CreateFromSeed(t *testing.T) {
	db := setupTestDB(t)
	svc := NewComponentService(db)
	ctx := context.Background()

	// Create a seed component directly in DB (bypassing service)
	seed := &storage.RecipeComponent{
		Namespace:     storage.NamespaceSeed,
		Slug:          "laravel-migrate",
		Version:       "v1.0.0",
		Name:          "Laravel Migrate",
		Description:   "Run artisan migrate",
		ComponentType: storage.ComponentTypeCommand,
		Content: storage.ComponentContent{
			Commands: []string{"php artisan migrate --force"},
		},
		IsSeed:    true,
		CreatedAt: time.Now(),
	}
	if err := db.CreateRecipeComponent(ctx, seed); err != nil {
		t.Fatalf("CreateRecipeComponent() error = %v", err)
	}

	// Create copy from seed
	copy, err := svc.CreateFromSeed(ctx, seed.ID, "my-migrate", "v1.0.0")
	if err != nil {
		t.Fatalf("CreateFromSeed() error = %v", err)
	}

	if copy.Namespace != storage.NamespaceUser {
		t.Errorf("Namespace = %v, want user", copy.Namespace)
	}
	if copy.IsSeed {
		t.Error("IsSeed = true, want false")
	}
	if copy.Name != "Laravel Migrate (Custom)" {
		t.Errorf("Name = %v, want 'Laravel Migrate (Custom)'", copy.Name)
	}
}

func TestComponentService_Delete_SeedBlocked(t *testing.T) {
	db := setupTestDB(t)
	svc := NewComponentService(db)
	ctx := context.Background()

	// Create a seed component
	seed := &storage.RecipeComponent{
		Namespace:     storage.NamespaceSeed,
		Slug:          "protected-seed",
		Version:       "v1.0.0",
		Name:          "Protected Seed",
		ComponentType: storage.ComponentTypeCommand,
		IsSeed:        true,
		CreatedAt:     time.Now(),
	}
	if err := db.CreateRecipeComponent(ctx, seed); err != nil {
		t.Fatalf("CreateRecipeComponent() error = %v", err)
	}

	// Try to delete - should fail
	err := svc.Delete(ctx, seed.ID)
	if err == nil {
		t.Fatal("Delete() expected error for seed component")
	}
}

func TestComponentService_ValidateVariables_Required(t *testing.T) {
	db := setupTestDB(t)
	svc := NewComponentService(db)
	ctx := context.Background()

	// Create component with required variable
	component := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "with-vars",
		Version:       "v1.0.0",
		Name:          "With Variables",
		ComponentType: storage.ComponentTypeCommand,
		Variables: []storage.VariableDefinition{
			{Name: "DATABASE_URL", Type: "string", Required: true},
			{Name: "DEBUG", Type: "boolean", Required: false, Default: "false"},
		},
		CreatedAt: time.Now(),
	}
	if err := svc.Create(ctx, component); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Missing required variable
	err := svc.ValidateVariables(ctx, component.ID, map[string]string{})
	if err == nil {
		t.Fatal("ValidateVariables() expected error for missing required variable")
	}

	// With required variable
	err = svc.ValidateVariables(ctx, component.ID, map[string]string{
		"DATABASE_URL": "postgres://localhost/test",
	})
	if err != nil {
		t.Fatalf("ValidateVariables() error = %v", err)
	}
}
