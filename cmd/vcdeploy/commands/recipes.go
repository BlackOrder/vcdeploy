// Package commands implements the CLI commands for vcdeploy.
package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/services/recipes"
	"github.com/spf13/cobra"
)

// RecipesCmd represents the recipes command group.
var RecipesCmd = &cobra.Command{
	Use:   "recipes",
	Short: "Recipe and playbook management commands",
	Long:  `Commands for managing deployment recipes, playbooks, and migrations.`,
}

// ImportYAMLCmd migrates a YAML project configuration to a playbook.
var ImportYAMLCmd = &cobra.Command{
	Use:   "import-yaml [yaml-file]",
	Short: "Import a project YAML configuration to create a playbook",
	Long: `Migrates a project YAML configuration file to a recipe playbook.

This command reads a project YAML file (e.g., project.yaml) and creates:
- Components from hooks (pre-deploy, post-deploy, rollback)
- Components from service reload actions
- A playbook containing all the components

Example:
  vcdeploy recipes import-yaml configs/myproject.yaml --project-id 123
  vcdeploy recipes import-yaml configs/myproject.yaml --preview
`,
	Args: cobra.ExactArgs(1),
	RunE: runImportYAML,
}

var (
	importProjectID        int64
	importPlaybookName     string
	importPlaybookVersion  string
	importCreateComponents bool
	importActivate         bool
	importPreview          bool
	importOutputJSON       bool
	importDatabasePath     string
)

func init() {
	RecipesCmd.AddCommand(ImportYAMLCmd)

	ImportYAMLCmd.Flags().Int64Var(&importProjectID, "project-id", 0, "Project ID to associate the playbook with")
	ImportYAMLCmd.Flags().StringVar(&importPlaybookName, "name", "", "Name for the generated playbook (default: <project>-playbook)")
	ImportYAMLCmd.Flags().StringVar(&importPlaybookVersion, "version", "v1.0.0", "Version for the generated playbook")
	ImportYAMLCmd.Flags().BoolVar(&importCreateComponents, "create-components", true, "Create individual components from hooks")
	ImportYAMLCmd.Flags().BoolVar(&importActivate, "activate", false, "Activate the playbook for the project after creation")
	ImportYAMLCmd.Flags().BoolVar(&importPreview, "preview", false, "Preview the migration without making changes")
	ImportYAMLCmd.Flags().BoolVar(&importOutputJSON, "json", false, "Output results as JSON")
	ImportYAMLCmd.Flags().StringVar(&importDatabasePath, "database", "", "Path to database file (default: auto-detect from config)")
}

func runImportYAML(cmd *cobra.Command, args []string) error {
	yamlPath := args[0]

	// Load the YAML config
	cfg, err := config.LoadProjectConfig(yamlPath)
	if err != nil {
		return fmt.Errorf("failed to load YAML: %w", err)
	}

	// Preview mode
	if importPreview {
		return runMigrationPreview(cfg)
	}

	// Full migration requires database
	dbPath := importDatabasePath
	if dbPath == "" {
		dbPath = getDatabasePath()
	}

	cliServices, cleanup, err := InitCLIServices(dbPath)
	if err != nil {
		return fmt.Errorf("failed to initialize services: %w", err)
	}
	defer cleanup()

	migrationSvc := recipes.NewMigrationService(cliServices.Store())

	opts := recipes.MigrationOptions{
		CreateComponents: importCreateComponents,
		PlaybookName:     importPlaybookName,
		PlaybookVersion:  importPlaybookVersion,
		ActivatePlaybook: importActivate && importProjectID > 0,
	}

	ctx := context.Background()
	result, err := migrationSvc.MigrateProjectConfig(ctx, importProjectID, cfg, opts)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	// Output results
	if importOutputJSON {
		return outputMigrationResultJSON(result)
	}

	return outputMigrationResult(result)
}

func runMigrationPreview(cfg *config.ProjectConfig) error {
	preview := &recipes.MigrationPreview{
		ProjectName:     cfg.Name,
		ProjectType:     cfg.Type,
		PreDeployHooks:  len(cfg.Hooks.PreDeploy),
		PostDeployHooks: len(cfg.Hooks.PostDeploy),
		ReloadActions:   len(cfg.Hooks.Reload),
		RollbackHooks:   len(cfg.Hooks.Rollback),
		Warnings:        []string{},
	}
	preview.TotalComponents = preview.PreDeployHooks + preview.PostDeployHooks + preview.ReloadActions + preview.RollbackHooks

	// Check for warnings
	if cfg.Watch.Guards.RequireCIPass {
		preview.Warnings = append(preview.Warnings, "CI guard cannot be migrated")
	}
	if cfg.Health.URL != "" {
		preview.Warnings = append(preview.Warnings, "Health checks require separate configuration")
	}
	if len(cfg.Env.RequiredKeys) > 0 {
		preview.Warnings = append(preview.Warnings, fmt.Sprintf("%d environment variables should be defined as playbook variables", len(cfg.Env.RequiredKeys)))
	}

	if importOutputJSON {
		data, _ := json.MarshalIndent(preview, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("Migration Preview for: %s\n", preview.ProjectName)
	fmt.Printf("=====================================\n")
	fmt.Printf("Project Type:      %s\n", preview.ProjectType)
	fmt.Printf("Pre-deploy hooks:  %d\n", preview.PreDeployHooks)
	fmt.Printf("Post-deploy hooks: %d\n", preview.PostDeployHooks)
	fmt.Printf("Reload actions:    %d\n", preview.ReloadActions)
	fmt.Printf("Rollback hooks:    %d\n", preview.RollbackHooks)
	fmt.Printf("Total components:  %d\n", preview.TotalComponents)

	if len(preview.Warnings) > 0 {
		fmt.Printf("\nWarnings:\n")
		for _, w := range preview.Warnings {
			fmt.Printf("  ⚠ %s\n", w)
		}
	}

	return nil
}

func outputMigrationResult(result *recipes.MigrationResult) error {
	fmt.Printf("Migration Complete\n")
	fmt.Printf("==================\n")

	if result.Playbook != nil {
		fmt.Printf("Playbook created: %s (v%s)\n", result.Playbook.Name, result.Playbook.Version)
		fmt.Printf("  ID: %d\n", result.Playbook.ID)
		fmt.Printf("  Steps: %d\n", len(result.Playbook.Steps))
	}

	if len(result.Components) > 0 {
		fmt.Printf("\nComponents created: %d\n", len(result.Components))
		for _, c := range result.Components {
			fmt.Printf("  - %s (%s)\n", c.Name, c.Slug)
		}
	}

	if result.Activation != nil {
		fmt.Printf("\nPlaybook activated for project ID: %d\n", result.Activation.ProjectID)
	}

	if len(result.Warnings) > 0 {
		fmt.Printf("\nWarnings:\n")
		for _, w := range result.Warnings {
			fmt.Printf("  ⚠ %s\n", w)
		}
	}

	return nil
}

func outputMigrationResultJSON(result *recipes.MigrationResult) error {
	output := map[string]interface{}{
		"success": true,
	}

	if result.Playbook != nil {
		output["playbook"] = map[string]interface{}{
			"id":      result.Playbook.ID,
			"name":    result.Playbook.Name,
			"version": result.Playbook.Version,
			"steps":   len(result.Playbook.Steps),
		}
	}

	if len(result.Components) > 0 {
		components := []map[string]interface{}{}
		for _, c := range result.Components {
			components = append(components, map[string]interface{}{
				"id":   c.ID,
				"name": c.Name,
				"slug": c.Slug,
			})
		}
		output["components"] = components
	}

	if result.Activation != nil {
		output["activation"] = map[string]interface{}{
			"id":         result.Activation.ID,
			"project_id": result.Activation.ProjectID,
		}
	}

	if len(result.Warnings) > 0 {
		output["warnings"] = result.Warnings
	}

	data, _ := json.MarshalIndent(output, "", "  ")
	fmt.Println(string(data))
	return nil
}

// getDatabasePath returns the database path from environment or default.
func getDatabasePath() string {
	if path := os.Getenv("VCDEPLOY_DB_PATH"); path != "" {
		return path
	}
	return "/var/lib/vcdeploy/vcdeploy.db"
}
