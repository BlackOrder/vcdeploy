package recipes

import (
	"context"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
)

func TestPlaybookService_Create_Valid(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a component first (required for step references)
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

	svc := NewPlaybookService(db)

	playbook := &storage.Playbook{
		Namespace:     storage.NamespaceUser,
		Slug:          "test-playbook",
		Version:       "v1.0.0",
		Name:          "Test Playbook",
		Description:   "A test playbook",
		FrameworkType: "generic",
		Steps: []storage.PlaybookStep{
			{
				ComponentRef: "user:test-cmd:v1.0.0",
				Phase:        storage.PhaseDeploy,
				Order:        1,
			},
		},
		KeepReleases: 5,
	}

	err := svc.Create(ctx, playbook)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Verify
	got, err := svc.Get(ctx, storage.NamespaceUser, "test-playbook", "v1.0.0")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Name != "Test Playbook" {
		t.Errorf("Name = %v, want %v", got.Name, "Test Playbook")
	}
}

func TestPlaybookService_Create_InvalidComponentRef(t *testing.T) {
	db := setupTestDB(t)
	svc := NewPlaybookService(db)
	ctx := context.Background()

	playbook := &storage.Playbook{
		Namespace:     storage.NamespaceUser,
		Slug:          "test-playbook",
		Version:       "v1.0.0",
		Name:          "Test Playbook",
		FrameworkType: "generic",
		Steps: []storage.PlaybookStep{
			{
				ComponentRef: "user:nonexistent:v1.0.0", // Component doesn't exist
				Phase:        storage.PhaseDeploy,
				Order:        1,
			},
		},
	}

	err := svc.Create(ctx, playbook)
	if err == nil {
		t.Fatal("Create() expected error for invalid component reference")
	}
}

func TestPlaybookService_GetLatest(t *testing.T) {
	db := setupTestDB(t)
	svc := NewPlaybookService(db)
	ctx := context.Background()

	// Create multiple versions
	versions := []string{"v1.0.0", "v2.0.0", "v1.5.0"}
	for _, v := range versions {
		playbook := &storage.Playbook{
			Namespace:     storage.NamespaceUser,
			Slug:          "versioned-playbook",
			Version:       v,
			Name:          "Version " + v,
			FrameworkType: "generic",
			CreatedAt:     time.Now(),
		}
		if err := db.CreatePlaybook(ctx, playbook); err != nil {
			t.Fatalf("CreatePlaybook() error = %v", err)
		}
	}

	latest, err := svc.GetLatest(ctx, storage.NamespaceUser, "versioned-playbook")
	if err != nil {
		t.Fatalf("GetLatest() error = %v", err)
	}
	if latest.Version != "v2.0.0" {
		t.Errorf("GetLatest() Version = %v, want v2.0.0", latest.Version)
	}
}

func TestPlaybookService_CustomizeFromSeed(t *testing.T) {
	db := setupTestDB(t)
	svc := NewPlaybookService(db)
	ctx := context.Background()

	// Create seed playbook
	seed := &storage.Playbook{
		Namespace:     storage.NamespaceSeed,
		Slug:          "laravel-standard",
		Version:       "v1.0.0",
		Name:          "Laravel Standard",
		Description:   "Standard Laravel deployment",
		FrameworkType: "laravel",
		SharedDirs:    []string{"storage"},
		WritableDirs:  []string{"storage/logs"},
		KeepReleases:  5,
		IsSeed:        true,
		CreatedAt:     time.Now(),
	}
	if err := db.CreatePlaybook(ctx, seed); err != nil {
		t.Fatalf("CreatePlaybook() error = %v", err)
	}

	// Customize
	copy, err := svc.CustomizeFromSeed(ctx, seed.ID, "my-laravel", "v1.0.0")
	if err != nil {
		t.Fatalf("CustomizeFromSeed() error = %v", err)
	}

	if copy.Namespace != storage.NamespaceUser {
		t.Errorf("Namespace = %v, want user", copy.Namespace)
	}
	if copy.IsSeed {
		t.Error("IsSeed = true, want false")
	}
	if copy.ParentID == nil || *copy.ParentID != seed.ID {
		t.Error("ParentID not set correctly")
	}
	if copy.ParentVersion != "v1.0.0" {
		t.Errorf("ParentVersion = %v, want v1.0.0", copy.ParentVersion)
	}
}

func TestPlaybookService_CheckNewerVersionAvailable(t *testing.T) {
	db := setupTestDB(t)
	svc := NewPlaybookService(db)
	ctx := context.Background()

	// Create seed playbook v1.0.0
	seed := &storage.Playbook{
		Namespace:     storage.NamespaceSeed,
		Slug:          "seed-playbook",
		Version:       "v1.0.0",
		Name:          "Seed Playbook",
		FrameworkType: "generic",
		IsSeed:        true,
		CreatedAt:     time.Now(),
	}
	if err := db.CreatePlaybook(ctx, seed); err != nil {
		t.Fatalf("CreatePlaybook() error = %v", err)
	}

	// Create user copy
	copy, err := svc.CustomizeFromSeed(ctx, seed.ID, "my-copy", "v1.0.0")
	if err != nil {
		t.Fatalf("CustomizeFromSeed() error = %v", err)
	}

	// No newer version yet
	hasNewer, _, err := svc.CheckNewerVersionAvailable(ctx, copy.ID)
	if err != nil {
		t.Fatalf("CheckNewerVersionAvailable() error = %v", err)
	}
	if hasNewer {
		t.Error("CheckNewerVersionAvailable() = true, want false")
	}

	// Create seed v2.0.0
	seedV2 := &storage.Playbook{
		Namespace:     storage.NamespaceSeed,
		Slug:          "seed-playbook",
		Version:       "v2.0.0",
		Name:          "Seed Playbook v2",
		FrameworkType: "generic",
		IsSeed:        true,
		CreatedAt:     time.Now(),
	}
	if err := db.CreatePlaybook(ctx, seedV2); err != nil {
		t.Fatalf("CreatePlaybook() error = %v", err)
	}

	// Now should have newer version
	hasNewer, newVersion, err := svc.CheckNewerVersionAvailable(ctx, copy.ID)
	if err != nil {
		t.Fatalf("CheckNewerVersionAvailable() error = %v", err)
	}
	if !hasNewer {
		t.Error("CheckNewerVersionAvailable() = false, want true")
	}
	if newVersion != "v2.0.0" {
		t.Errorf("newVersion = %v, want v2.0.0", newVersion)
	}
}

func TestPlaybookService_Delete_SeedBlocked(t *testing.T) {
	db := setupTestDB(t)
	svc := NewPlaybookService(db)
	ctx := context.Background()

	// Create seed playbook
	seed := &storage.Playbook{
		Namespace:     storage.NamespaceSeed,
		Slug:          "protected-seed",
		Version:       "v1.0.0",
		Name:          "Protected Seed",
		FrameworkType: "generic",
		IsSeed:        true,
		CreatedAt:     time.Now(),
	}
	if err := db.CreatePlaybook(ctx, seed); err != nil {
		t.Fatalf("CreatePlaybook() error = %v", err)
	}

	err := svc.Delete(ctx, seed.ID)
	if err == nil {
		t.Fatal("Delete() expected error for seed playbook")
	}
}

func TestParseComponentRef(t *testing.T) {
	tests := []struct {
		ref       string
		namespace string
		slug      string
		version   string
		wantErr   bool
	}{
		{"seed:laravel-migrate:v1.0.0", "seed", "laravel-migrate", "v1.0.0", false},
		{"user:my-component:v2.1.0", "user", "my-component", "v2.1.0", false},
		{"invalid", "", "", "", true},
		{"only:two", "", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			ns, slug, ver, err := ParseComponentRef(tt.ref)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseComponentRef() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if ns != tt.namespace {
				t.Errorf("namespace = %v, want %v", ns, tt.namespace)
			}
			if slug != tt.slug {
				t.Errorf("slug = %v, want %v", slug, tt.slug)
			}
			if ver != tt.version {
				t.Errorf("version = %v, want %v", ver, tt.version)
			}
		})
	}
}

func TestBuildComponentRef(t *testing.T) {
	ref := BuildComponentRef("seed", "laravel-migrate", "v1.0.0")
	expected := "seed:laravel-migrate:v1.0.0"
	if ref != expected {
		t.Errorf("BuildComponentRef() = %v, want %v", ref, expected)
	}
}
