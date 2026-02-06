package recipes

import (
	"context"
	"testing"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

func TestMigrationService_MigrateProjectConfig_Basic(t *testing.T) {
	db := setupTestDB(t)
	svc := NewMigrationService(db)
	ctx := context.Background()

	// Create test project
	project := &storage.Project{
		Name: "test-project",
		Type: "laravel",
	}
	if err := db.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	// Create project config
	cfg := &config.ProjectConfig{
		Name: "test-project",
		Hooks: config.HooksConfig{
			PreDeploy:  []string{"composer install --no-dev"},
			PostDeploy: []string{"php artisan migrate", "php artisan cache:clear"},
		},
	}

	opts := MigrationOptions{
		CreateComponents: true,
		PlaybookName:     "test-playbook",
		PlaybookVersion:  "v1.0.0",
		ActivatePlaybook: false,
	}

	result, err := svc.MigrateProjectConfig(ctx, project.ID, cfg, opts)
	if err != nil {
		t.Fatalf("MigrateProjectConfig() error = %v", err)
	}

	// Verify components were created
	if len(result.Components) != 3 { // 1 pre-deploy + 2 post-deploy
		t.Errorf("Components count = %d, want 3", len(result.Components))
	}

	// Verify playbook was created
	if result.Playbook == nil {
		t.Fatal("Playbook should not be nil")
	}
	if result.Playbook.Name != "test-playbook" {
		t.Errorf("Playbook.Name = %v, want test-playbook", result.Playbook.Name)
	}
}

func TestMigrationService_MigrateProjectConfig_NoComponents(t *testing.T) {
	db := setupTestDB(t)
	svc := NewMigrationService(db)
	ctx := context.Background()

	// Create test project
	project := &storage.Project{
		Name: "test-project",
		Type: "static",
	}
	if err := db.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	// Empty hooks
	cfg := &config.ProjectConfig{
		Name:  "test-project",
		Hooks: config.HooksConfig{},
	}

	opts := MigrationOptions{
		CreateComponents: true,
		PlaybookName:     "empty-playbook",
		PlaybookVersion:  "v1.0.0",
	}

	result, err := svc.MigrateProjectConfig(ctx, project.ID, cfg, opts)
	if err != nil {
		t.Fatalf("MigrateProjectConfig() error = %v", err)
	}

	// Verify no components created
	if len(result.Components) != 0 {
		t.Errorf("Components count = %d, want 0", len(result.Components))
	}
}

func TestMigrationService_MigrateProjectConfig_WithActivation(t *testing.T) {
	db := setupTestDB(t)
	svc := NewMigrationService(db)
	ctx := context.Background()

	// Create test project
	project := &storage.Project{
		Name: "test-project",
		Type: "node",
	}
	if err := db.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	cfg := &config.ProjectConfig{
		Name: "test-project",
		Hooks: config.HooksConfig{
			PostDeploy: []string{"npm run build"},
		},
	}

	opts := MigrationOptions{
		CreateComponents: true,
		PlaybookName:     "node-playbook",
		PlaybookVersion:  "v1.0.0",
		ActivatePlaybook: true,
	}

	result, err := svc.MigrateProjectConfig(ctx, project.ID, cfg, opts)
	if err != nil {
		t.Fatalf("MigrateProjectConfig() error = %v", err)
	}

	// Verify activation was created
	if result.Activation == nil {
		t.Fatal("Activation should not be nil when ActivatePlaybook is true")
	}
	if result.Activation.ProjectID != project.ID {
		t.Errorf("Activation.ProjectID = %d, want %d", result.Activation.ProjectID, project.ID)
	}
}

func TestMigrationService_CreateComponentFromHook(t *testing.T) {
	db := setupTestDB(t)
	svc := NewMigrationService(db)
	ctx := context.Background()

	// Test creating component from a hook command
	hook := "composer install --no-dev --optimize-autoloader"
	comp, err := svc.createComponentFromHook(ctx, "test-project", "pre_deploy", 0, hook)
	if err != nil {
		t.Fatalf("createComponentFromHook() error = %v", err)
	}

	// Verify component properties
	if comp.Namespace != storage.NamespaceUser {
		t.Errorf("Namespace = %v, want %v", comp.Namespace, storage.NamespaceUser)
	}
	// Hook components use "hook" type
	if comp.ComponentType != storage.ComponentTypeHook {
		t.Errorf("ComponentType = %v, want %v", comp.ComponentType, storage.ComponentTypeHook)
	}
	if len(comp.Content.Commands) == 0 {
		t.Error("Commands should not be empty")
	}
	if comp.Content.Commands[0] != hook {
		t.Errorf("Command = %v, want %v", comp.Content.Commands[0], hook)
	}
}

func TestMigrationService_Defaults(t *testing.T) {
	db := setupTestDB(t)
	svc := NewMigrationService(db)
	ctx := context.Background()

	// Create test project
	project := &storage.Project{
		Name: "default-test",
		Type: "generic",
	}
	if err := db.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	cfg := &config.ProjectConfig{
		Name: "default-test",
	}

	// Empty options - should use defaults
	opts := MigrationOptions{}

	result, err := svc.MigrateProjectConfig(ctx, project.ID, cfg, opts)
	if err != nil {
		t.Fatalf("MigrateProjectConfig() error = %v", err)
	}

	// Verify defaults were applied
	if result.Playbook == nil {
		t.Fatal("Playbook should not be nil")
	}
	if result.Playbook.Version != "v1.0.0" {
		t.Errorf("Playbook.Version = %v, want v1.0.0", result.Playbook.Version)
	}
}

func TestMigrationPreview(t *testing.T) {
	// MigrationPreview is a data struct, test its usage
	preview := &MigrationPreview{
		ProjectName:     "test",
		ProjectType:     "laravel",
		PreDeployHooks:  2,
		PostDeployHooks: 3,
		ReloadActions:   1,
		RollbackHooks:   0,
		TotalComponents: 6,
		Warnings:        []string{"Test warning"},
	}

	if preview.ProjectName != "test" {
		t.Errorf("ProjectName = %s, want test", preview.ProjectName)
	}
	if preview.ProjectType != "laravel" {
		t.Errorf("ProjectType = %s, want laravel", preview.ProjectType)
	}
	if preview.PreDeployHooks != 2 {
		t.Errorf("PreDeployHooks = %d, want 2", preview.PreDeployHooks)
	}
	if preview.PostDeployHooks != 3 {
		t.Errorf("PostDeployHooks = %d, want 3", preview.PostDeployHooks)
	}
	if preview.ReloadActions != 1 {
		t.Errorf("ReloadActions = %d, want 1", preview.ReloadActions)
	}
	if preview.RollbackHooks != 0 {
		t.Errorf("RollbackHooks = %d, want 0", preview.RollbackHooks)
	}
	if preview.TotalComponents != 6 {
		t.Errorf("TotalComponents = %d, want 6", preview.TotalComponents)
	}
	if len(preview.Warnings) != 1 {
		t.Errorf("Warnings count = %d, want 1", len(preview.Warnings))
	}
}
