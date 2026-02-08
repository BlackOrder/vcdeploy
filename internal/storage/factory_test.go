package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewCachedStore(t *testing.T) {
	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "vcdeploy-factory-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	logger := zap.NewNop()

	// Create cached store
	store, err := NewCachedStore(dbPath, logger)
	if err != nil {
		t.Fatalf("NewCachedStore failed: %v", err)
	}
	defer store.Close()

	// Verify we can access the underlying DB
	if store.UnderlyingDB() == nil {
		t.Error("UnderlyingDB() returned nil")
	}

	// Verify we can perform operations through the MemoryStore
	ctx := context.Background()

	// Create a user
	user := &User{
		Username:     "testuser",
		PasswordHash: "hash123",
		Email:        "test@example.com",
		Role:         "admin",
	}
	if err := store.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Verify we can read it back (from memory)
	readUser, err := store.GetUserByUsername(ctx, "testuser")
	if err != nil {
		t.Fatalf("GetUserByUsername failed: %v", err)
	}
	if readUser.Email != "test@example.com" {
		t.Errorf("Expected email test@example.com, got %s", readUser.Email)
	}
}

func TestNewCachedStoreLoadsExistingData(t *testing.T) {
	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "vcdeploy-factory-load-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	logger := zap.NewNop()

	// First, create data using direct DB access
	db, err := New(dbPath, logger)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx := context.Background()
	user := &User{
		Username:     "preexisting",
		PasswordHash: "hash456",
		Email:        "pre@example.com",
		Role:         "viewer",
	}
	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	db.Close()

	// Now open with CachedStore and verify data is loaded
	store, err := NewCachedStore(dbPath, logger)
	if err != nil {
		t.Fatalf("NewCachedStore failed: %v", err)
	}
	defer store.Close()

	// Data should be in memory now
	readUser, err := store.GetUserByUsername(ctx, "preexisting")
	if err != nil {
		t.Fatalf("GetUserByUsername failed: %v", err)
	}
	if readUser.Email != "pre@example.com" {
		t.Errorf("Expected email pre@example.com, got %s", readUser.Email)
	}
}

func TestNewCachedStoreWriteThrough(t *testing.T) {
	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "vcdeploy-factory-writethrough-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	logger := zap.NewNop()

	// Create cached store
	store, err := NewCachedStore(dbPath, logger)
	if err != nil {
		t.Fatalf("NewCachedStore failed: %v", err)
	}

	ctx := context.Background()

	// Create data through cached store
	user := &User{
		Username:     "writethrough",
		PasswordHash: "hash789",
		Email:        "wt@example.com",
		Role:         "admin",
	}
	if err := store.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Close the store (this should flush pending writes)
	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Give background workers time to flush
	time.Sleep(200 * time.Millisecond)

	// Open directly with DB to verify data was persisted
	db, err := New(dbPath, logger)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer db.Close()

	readUser, err := db.GetUserByUsername(ctx, "writethrough")
	if err != nil {
		t.Fatalf("GetUserByUsername from DB failed: %v", err)
	}
	if readUser.Email != "wt@example.com" {
		t.Errorf("Expected email wt@example.com, got %s", readUser.Email)
	}
}

func TestNewCachedStoreInvalidPath(t *testing.T) {
	// Try to create store with invalid path
	_, err := NewCachedStore("/nonexistent/directory/test.db", nil)
	if err == nil {
		t.Error("Expected error for invalid path, got nil")
	}
}

// TestCachedStore_Recipe_Integration verifies that recipe operations work
// correctly through CachedStore (which embeds MemoryStore).
func TestCachedStore_Recipe_Integration(t *testing.T) {
	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "vcdeploy-cachedstore-recipe-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	logger := zap.NewNop()

	// Create cached store
	store, err := NewCachedStore(dbPath, logger)
	if err != nil {
		t.Fatalf("NewCachedStore failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Test 1: Create a component
	component := &RecipeComponent{
		Name:          "test-component",
		Slug:          "test-component",
		Version:       "v1.0.0",
		Description:   "Test component for CachedStore integration",
		ComponentType: "command",
		Content: ComponentContent{
			Commands: []string{"echo hello"},
		},
		Namespace: "user",
	}
	if err := store.CreateRecipeComponent(ctx, component); err != nil {
		t.Fatalf("CreateRecipeComponent failed: %v", err)
	}
	if component.ID == "" {
		t.Error("Component ID should be set after creation")
	}

	// Test 2: Read the component back by ID
	readComponent, err := store.GetRecipeComponentByID(ctx, component.ID)
	if err != nil {
		t.Fatalf("GetRecipeComponentByID failed: %v", err)
	}
	if readComponent.Name != "test-component" {
		t.Errorf("Expected name 'test-component', got '%s'", readComponent.Name)
	}

	// Test 3: List components
	components, err := store.ListRecipeComponents(ctx, "user", false)
	if err != nil {
		t.Fatalf("ListRecipeComponents failed: %v", err)
	}
	if len(components) < 1 {
		t.Error("Expected at least 1 component")
	}

	// Test 4: Create a playbook
	playbook := &Playbook{
		Name:        "test-playbook",
		Slug:        "test-playbook",
		Version:     "v1.0.0",
		Description: "Test playbook for CachedStore integration",
		Namespace:   "user",
		Steps: []PlaybookStep{
			{
				Order:        1,
				ComponentRef: "user:test-component:v1.0.0",
				Phase:        "deploy",
			},
		},
	}
	if err := store.CreatePlaybook(ctx, playbook); err != nil {
		t.Fatalf("CreatePlaybook failed: %v", err)
	}
	if playbook.ID == "" {
		t.Error("Playbook ID should be set after creation")
	}

	// Test 5: Get playbook by ID
	readPlaybook, err := store.GetPlaybookByID(ctx, playbook.ID)
	if err != nil {
		t.Fatalf("GetPlaybookByID failed: %v", err)
	}
	if readPlaybook.Name != "test-playbook" {
		t.Errorf("Expected name 'test-playbook', got '%s'", readPlaybook.Name)
	}

	// Test 6: List playbooks
	playbooks, err := store.ListPlaybooks(ctx, "user", "", false)
	if err != nil {
		t.Fatalf("ListPlaybooks failed: %v", err)
	}
	if len(playbooks) < 1 {
		t.Error("Expected at least 1 playbook")
	}

	// Test 7: Create a project for activation testing
	project := &Project{
		Name:       "test-project",
		Repository: "https://github.com/test/repo",
		Branch:     "main",
		DeployPath: "/var/www/test",
	}
	if err := store.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	// Test 8: Create playbook activation
	activation := &PlaybookActivation{
		ProjectID:  project.ID,
		PlaybookID: playbook.ID,
	}
	if err := store.CreatePlaybookActivation(ctx, activation); err != nil {
		t.Fatalf("CreatePlaybookActivation failed: %v", err)
	}

	// Test 9: Get activation for project
	readActivation, err := store.GetPlaybookActivation(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetPlaybookActivation failed: %v", err)
	}
	if readActivation.PlaybookID != playbook.ID {
		t.Errorf("Expected playbook ID %s, got %s", playbook.ID, readActivation.PlaybookID)
	}

	// Test 10: Delete activation
	if err := store.DeletePlaybookActivation(ctx, activation.ID); err != nil {
		t.Fatalf("DeletePlaybookActivation failed: %v", err)
	}

	// Test 11: Verify activation was deleted
	_, err = store.GetPlaybookActivation(ctx, project.ID)
	if err == nil {
		t.Error("Expected error getting deleted activation")
	}

	// Test 12: Update component
	component.Description = "Updated description"
	if err := store.UpdateRecipeComponent(ctx, component); err != nil {
		t.Fatalf("UpdateRecipeComponent failed: %v", err)
	}

	// Verify update
	readComponent, err = store.GetRecipeComponentByID(ctx, component.ID)
	if err != nil {
		t.Fatalf("GetRecipeComponentByID after update failed: %v", err)
	}
	if readComponent.Description != "Updated description" {
		t.Errorf("Expected updated description, got '%s'", readComponent.Description)
	}

	// Test 13: Delete playbook first (before component)
	if err := store.DeletePlaybook(ctx, playbook.ID); err != nil {
		t.Fatalf("DeletePlaybook failed: %v", err)
	}

	// Test 14: Delete the component
	if err := store.DeleteRecipeComponent(ctx, component.ID); err != nil {
		t.Fatalf("DeleteRecipeComponent failed: %v", err)
	}

	// Verify component was deleted
	_, err = store.GetRecipeComponentByID(ctx, component.ID)
	if err == nil {
		t.Error("Expected error getting deleted component")
	}

	t.Log("CachedStore recipe integration test passed - all operations work correctly")
}
