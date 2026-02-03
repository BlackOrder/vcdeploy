package export

import (
	"context"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// setupTestDB creates a test database for import tests.
func setupTestDBImport(t *testing.T) storage.Store {
	t.Helper()
	db, err := storage.New(":memory:", zap.NewNop())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestImporter_ValidateBundle(t *testing.T) {
	store := setupTestDBImport(t)
	importer := NewImporter(store)

	tests := []struct {
		name    string
		bundle  *ExportBundle
		wantErr bool
	}{
		{
			name:    "nil bundle",
			bundle:  nil,
			wantErr: true,
		},
		{
			name:    "missing format version",
			bundle:  &ExportBundle{},
			wantErr: true,
		},
		{
			name:    "invalid format version",
			bundle:  &ExportBundle{FormatVersion: "not-semver"},
			wantErr: true,
		},
		{
			name: "format version too new",
			bundle: &ExportBundle{
				FormatVersion: "v99.0.0",
			},
			wantErr: true,
		},
		{
			name: "valid empty bundle",
			bundle: &ExportBundle{
				FormatVersion: FormatVersion,
				ExportedAt:    time.Now(),
			},
			wantErr: false,
		},
		{
			name: "component missing slug",
			bundle: &ExportBundle{
				FormatVersion: FormatVersion,
				Components:    []ComponentExport{{Version: "v1.0.0"}},
			},
			wantErr: true,
		},
		{
			name: "component missing version",
			bundle: &ExportBundle{
				FormatVersion: FormatVersion,
				Components:    []ComponentExport{{Slug: "test"}},
			},
			wantErr: true,
		},
		{
			name: "component invalid version",
			bundle: &ExportBundle{
				FormatVersion: FormatVersion,
				Components:    []ComponentExport{{Slug: "test", Version: "bad"}},
			},
			wantErr: true,
		},
		{
			name: "playbook missing slug",
			bundle: &ExportBundle{
				FormatVersion: FormatVersion,
				Playbooks:     []PlaybookExport{{Version: "v1.0.0"}},
			},
			wantErr: true,
		},
		{
			name: "valid bundle with data",
			bundle: &ExportBundle{
				FormatVersion: FormatVersion,
				ExportedAt:    time.Now(),
				Components: []ComponentExport{
					{Slug: "comp1", Version: "v1.0.0", Name: "Test"},
				},
				Playbooks: []PlaybookExport{
					{Slug: "play1", Version: "v1.0.0", Name: "Test"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := importer.ValidateBundle(tt.bundle)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBundle() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestImporter_ImportComponents(t *testing.T) {
	store := setupTestDBImport(t)
	importer := NewImporter(store)
	ctx := context.Background()

	bundle := &ExportBundle{
		FormatVersion: FormatVersion,
		ExportedAt:    time.Now(),
		Components: []ComponentExport{
			{
				Slug:          "imported-comp",
				Version:       "v1.0.0",
				Name:          "Imported Component",
				Description:   "A component from import",
				ComponentType: storage.ComponentTypeCommand,
				Content: storage.ComponentContent{
					Commands: []string{"echo imported"},
				},
			},
		},
	}

	result, err := importer.Import(ctx, bundle, ConflictSkip)
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if result.ComponentsImported != 1 {
		t.Errorf("expected 1 component imported, got %d", result.ComponentsImported)
	}

	// Verify component was created
	comp, err := store.GetRecipeComponent(ctx, storage.NamespaceUser, "imported-comp", "v1.0.0")
	if err != nil {
		t.Fatalf("failed to get imported component: %v", err)
	}
	if comp.Name != "Imported Component" {
		t.Errorf("expected name 'Imported Component', got '%s'", comp.Name)
	}
	if comp.IsSeed {
		t.Error("imported component should not be marked as seed")
	}
}

func TestImporter_ConflictSkip(t *testing.T) {
	store := setupTestDBImport(t)
	importer := NewImporter(store)
	ctx := context.Background()

	// Create existing component
	existing := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "existing-comp",
		Version:       "v1.0.0",
		Name:          "Original Name",
		ComponentType: storage.ComponentTypeCommand,
		Content: storage.ComponentContent{
			Commands: []string{"echo original"},
		},
	}
	if err := store.CreateRecipeComponent(ctx, existing); err != nil {
		t.Fatalf("failed to create existing: %v", err)
	}

	// Try to import same component
	bundle := &ExportBundle{
		FormatVersion: FormatVersion,
		ExportedAt:    time.Now(),
		Components: []ComponentExport{
			{
				Slug:          "existing-comp",
				Version:       "v1.0.0",
				Name:          "New Name",
				ComponentType: storage.ComponentTypeCommand,
				Content: storage.ComponentContent{
					Commands: []string{"echo new"},
				},
			},
		},
	}

	result, err := importer.Import(ctx, bundle, ConflictSkip)
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if result.ComponentsSkipped != 1 {
		t.Errorf("expected 1 component skipped, got %d", result.ComponentsSkipped)
	}

	// Verify original was not changed
	comp, _ := store.GetRecipeComponent(ctx, storage.NamespaceUser, "existing-comp", "v1.0.0")
	if comp.Name != "Original Name" {
		t.Errorf("expected 'Original Name', got '%s'", comp.Name)
	}
}

func TestImporter_ConflictOverwrite(t *testing.T) {
	store := setupTestDBImport(t)
	importer := NewImporter(store)
	ctx := context.Background()

	// Create existing component
	existing := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "existing-comp",
		Version:       "v1.0.0",
		Name:          "Original Name",
		ComponentType: storage.ComponentTypeCommand,
		Content: storage.ComponentContent{
			Commands: []string{"echo original"},
		},
	}
	if err := store.CreateRecipeComponent(ctx, existing); err != nil {
		t.Fatalf("failed to create existing: %v", err)
	}

	// Import with overwrite
	bundle := &ExportBundle{
		FormatVersion: FormatVersion,
		ExportedAt:    time.Now(),
		Components: []ComponentExport{
			{
				Slug:          "existing-comp",
				Version:       "v1.0.0",
				Name:          "Overwritten Name",
				ComponentType: storage.ComponentTypeCommand,
				Content: storage.ComponentContent{
					Commands: []string{"echo overwritten"},
				},
			},
		},
	}

	result, err := importer.Import(ctx, bundle, ConflictOverwrite)
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if result.ComponentsImported != 1 {
		t.Errorf("expected 1 component imported, got %d", result.ComponentsImported)
	}

	// Verify component was overwritten
	comp, _ := store.GetRecipeComponent(ctx, storage.NamespaceUser, "existing-comp", "v1.0.0")
	if comp.Name != "Overwritten Name" {
		t.Errorf("expected 'Overwritten Name', got '%s'", comp.Name)
	}
}

func TestImporter_ConflictRename(t *testing.T) {
	store := setupTestDBImport(t)
	importer := NewImporter(store)
	ctx := context.Background()

	// Create existing component
	existing := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "existing-comp",
		Version:       "v1.0.0",
		Name:          "Original Name",
		ComponentType: storage.ComponentTypeCommand,
		Content: storage.ComponentContent{
			Commands: []string{"echo original"},
		},
	}
	if err := store.CreateRecipeComponent(ctx, existing); err != nil {
		t.Fatalf("failed to create existing: %v", err)
	}

	// Import with rename
	bundle := &ExportBundle{
		FormatVersion: FormatVersion,
		ExportedAt:    time.Now(),
		Components: []ComponentExport{
			{
				Slug:          "existing-comp",
				Version:       "v1.0.0",
				Name:          "Renamed Name",
				ComponentType: storage.ComponentTypeCommand,
				Content: storage.ComponentContent{
					Commands: []string{"echo renamed"},
				},
			},
		},
	}

	result, err := importer.Import(ctx, bundle, ConflictRename)
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if result.ComponentsImported != 1 {
		t.Errorf("expected 1 component imported, got %d", result.ComponentsImported)
	}

	// Verify original still exists
	orig, _ := store.GetRecipeComponent(ctx, storage.NamespaceUser, "existing-comp", "v1.0.0")
	if orig.Name != "Original Name" {
		t.Errorf("expected original to have 'Original Name', got '%s'", orig.Name)
	}

	// Verify renamed was created
	renamed, err := store.GetRecipeComponent(ctx, storage.NamespaceUser, "existing-comp-imported", "v1.0.0")
	if err != nil {
		t.Fatalf("failed to get renamed component: %v", err)
	}
	if renamed.Name != "Renamed Name" {
		t.Errorf("expected 'Renamed Name', got '%s'", renamed.Name)
	}
}

func TestImporter_DryRun(t *testing.T) {
	store := setupTestDBImport(t)
	importer := NewImporter(store)
	ctx := context.Background()

	// Create existing component
	existing := &storage.RecipeComponent{
		Namespace:     storage.NamespaceUser,
		Slug:          "existing-comp",
		Version:       "v1.0.0",
		Name:          "Original",
		ComponentType: storage.ComponentTypeCommand,
		Content: storage.ComponentContent{
			Commands: []string{"echo original"},
		},
	}
	if err := store.CreateRecipeComponent(ctx, existing); err != nil {
		t.Fatalf("failed to create existing: %v", err)
	}

	bundle := &ExportBundle{
		FormatVersion: FormatVersion,
		ExportedAt:    time.Now(),
		Components: []ComponentExport{
			{Slug: "existing-comp", Version: "v1.0.0", Name: "New1"},
			{Slug: "new-comp", Version: "v1.0.0", Name: "New2"},
		},
	}

	result, err := importer.DryRun(ctx, bundle, ConflictSkip)
	if err != nil {
		t.Fatalf("DryRun failed: %v", err)
	}

	if result.ComponentsSkipped != 1 {
		t.Errorf("expected 1 skipped, got %d", result.ComponentsSkipped)
	}
	if result.ComponentsImported != 1 {
		t.Errorf("expected 1 would be imported, got %d", result.ComponentsImported)
	}

	// Verify no actual changes were made (new-comp shouldn't exist)
	comp, err := store.GetRecipeComponent(ctx, storage.NamespaceUser, "new-comp", "v1.0.0")
	if err != nil {
		t.Fatalf("GetRecipeComponent error: %v", err)
	}
	if comp != nil {
		t.Error("expected new-comp to not exist after dry run")
	}
}

func TestImporter_ImportPlaybooks(t *testing.T) {
	store := setupTestDBImport(t)
	importer := NewImporter(store)
	ctx := context.Background()

	bundle := &ExportBundle{
		FormatVersion: FormatVersion,
		ExportedAt:    time.Now(),
		Playbooks: []PlaybookExport{
			{
				Slug:          "imported-playbook",
				Version:       "v1.0.0",
				Name:          "Imported Playbook",
				Description:   "A playbook from import",
				FrameworkType: "generic",
				Steps:         []storage.PlaybookStep{},
				KeepReleases:  5,
			},
		},
	}

	result, err := importer.Import(ctx, bundle, ConflictSkip)
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if result.PlaybooksImported != 1 {
		t.Errorf("expected 1 playbook imported, got %d", result.PlaybooksImported)
	}

	// Verify playbook was created
	playbook, err := store.GetPlaybook(ctx, storage.NamespaceUser, "imported-playbook", "v1.0.0")
	if err != nil {
		t.Fatalf("failed to get imported playbook: %v", err)
	}
	if playbook.Name != "Imported Playbook" {
		t.Errorf("expected name 'Imported Playbook', got '%s'", playbook.Name)
	}
	if playbook.IsSeed {
		t.Error("imported playbook should not be marked as seed")
	}
}

func TestConflictStrategy_IsValid(t *testing.T) {
	tests := []struct {
		strategy ConflictStrategy
		valid    bool
	}{
		{ConflictSkip, true},
		{ConflictOverwrite, true},
		{ConflictRename, true},
		{ConflictStrategy("invalid"), false},
		{ConflictStrategy(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.strategy), func(t *testing.T) {
			if got := tt.strategy.IsValid(); got != tt.valid {
				t.Errorf("IsValid() = %v, want %v", got, tt.valid)
			}
		})
	}
}
