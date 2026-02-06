// Package commands implements the CLI commands for vcdeploy.
package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/services/recipes"
	"github.com/spf13/cobra"
)

// recipeCmd represents the recipe command group.
var recipeCmd = &cobra.Command{
	Use:     "recipe",
	Aliases: []string{"recipes"},
	Short:   "Recipe and playbook management commands",
	Long:    `Commands for managing deployment recipes, playbooks, and migrations.`,
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
	recipeCmd.AddCommand(ImportYAMLCmd)

	ImportYAMLCmd.Flags().Int64Var(&importProjectID, "project-id", 0, "Project ID to associate the playbook with")
	ImportYAMLCmd.Flags().StringVar(&importPlaybookName, "name", "", "Name for the generated playbook (default: <project>-playbook)")
	ImportYAMLCmd.Flags().StringVar(&importPlaybookVersion, "version", "v1.0.0", "Version for the generated playbook")
	ImportYAMLCmd.Flags().BoolVar(&importCreateComponents, "create-components", true, "Create individual components from hooks")
	ImportYAMLCmd.Flags().BoolVar(&importActivate, "activate", false, "Activate the playbook for the project after creation")
	ImportYAMLCmd.Flags().BoolVar(&importPreview, "preview", false, "Preview the migration without making changes")
	ImportYAMLCmd.Flags().BoolVar(&importOutputJSON, "json", false, "Output results as JSON")
	ImportYAMLCmd.Flags().StringVar(&importDatabasePath, "database", "", "Path to database file (default: auto-detect from config)")

	// API-based recipe commands
	initRecipeAPICommands()
}

func initRecipeAPICommands() {
	// List recipes (playbooks)
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all recipes/playbooks",
		Long: `List all available recipes and playbooks.

Example:
  vcdeploy recipe list --master localhost:9000 --token <token>`,
		RunE: runRecipeList,
	}
	listCmd.Flags().Bool("json", false, "Output as JSON")
	recipeCmd.AddCommand(listCmd)

	// Get recipe details
	getCmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get recipe/playbook details",
		Long: `Get detailed information about a specific recipe or playbook.

Example:
  vcdeploy recipe get my-playbook --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(1),
		RunE: runRecipeGet,
	}
	getCmd.Flags().Bool("json", false, "Output as JSON")
	recipeCmd.AddCommand(getCmd)

	// Create recipe
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new recipe/playbook",
		Long: `Create a new recipe or playbook.

Example:
  vcdeploy recipe create --name my-playbook --version v1.0.0 \
    --master localhost:9000 --token <token>`,
		RunE: runRecipeCreate,
	}
	createCmd.Flags().StringP("name", "n", "", "Recipe name (required)")
	createCmd.Flags().StringP("version", "v", "v1.0.0", "Recipe version")
	createCmd.Flags().String("description", "", "Recipe description")
	createCmd.Flags().String("file", "", "Load recipe from YAML file")
	_ = createCmd.MarkFlagRequired("name")
	recipeCmd.AddCommand(createCmd)

	// Update recipe
	updateCmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update an existing recipe/playbook",
		Long: `Update an existing recipe or playbook.

Example:
  vcdeploy recipe update my-playbook --version v1.1.0 \
    --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(1),
		RunE: runRecipeUpdate,
	}
	updateCmd.Flags().StringP("version", "v", "", "New version")
	updateCmd.Flags().String("description", "", "New description")
	updateCmd.Flags().String("file", "", "Update from YAML file")
	recipeCmd.AddCommand(updateCmd)

	// Delete recipe
	deleteCmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a recipe/playbook",
		Long: `Delete a recipe or playbook.

Example:
  vcdeploy recipe delete my-playbook --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(1),
		RunE: runRecipeDelete,
	}
	deleteCmd.Flags().BoolP("force", "f", false, "Skip confirmation")
	recipeCmd.AddCommand(deleteCmd)
}

// --- Recipe Types ---

type recipeListResponse struct {
	Recipes []recipeInfo `json:"recipes"`
}

type recipeInfo struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	StepCount   int    `json:"step_count"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func runRecipeList(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/recipes")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var result recipeListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	outputJSON, _ := cmd.Flags().GetBool("json")
	if outputJSON {
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if len(result.Recipes) == 0 {
		fmt.Println("No recipes found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tSTEPS\tDESCRIPTION\tUPDATED")
	for _, r := range result.Recipes {
		desc := r.Description
		if len(desc) > 30 {
			desc = desc[:27] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n",
			r.Name, r.Version, r.StepCount, desc, r.UpdatedAt)
	}
	w.Flush()
	return nil
}

func runRecipeGet(cmd *cobra.Command, args []string) error {
	name := args[0]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/recipes/" + name)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var r recipeInfo
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	outputJSON, _ := cmd.Flags().GetBool("json")
	if outputJSON {
		data, _ := json.MarshalIndent(r, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("Name:        %s\n", r.Name)
	fmt.Printf("Version:     %s\n", r.Version)
	fmt.Printf("Slug:        %s\n", r.Slug)
	fmt.Printf("Steps:       %d\n", r.StepCount)
	fmt.Printf("Created:     %s\n", r.CreatedAt)
	fmt.Printf("Updated:     %s\n", r.UpdatedAt)
	if r.Description != "" {
		fmt.Printf("Description: %s\n", r.Description)
	}
	return nil
}

func runRecipeCreate(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	version, _ := cmd.Flags().GetString("version")
	description, _ := cmd.Flags().GetString("description")
	filePath, _ := cmd.Flags().GetString("file")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	data := map[string]interface{}{
		"name":    name,
		"version": version,
	}
	if description != "" {
		data["description"] = description
	}

	// If file provided, load and include content
	if filePath != "" {
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
		data["content"] = string(content)
	}

	body, _ := json.Marshal(data)
	resp, err := client.post("/api/v1/recipes", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	fmt.Printf("Recipe '%s' created successfully.\n", name)
	return nil
}

func runRecipeUpdate(cmd *cobra.Command, args []string) error {
	name := args[0]
	version, _ := cmd.Flags().GetString("version")
	description, _ := cmd.Flags().GetString("description")
	filePath, _ := cmd.Flags().GetString("file")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	data := make(map[string]interface{})
	if version != "" {
		data["version"] = version
	}
	if description != "" {
		data["description"] = description
	}

	// If file provided, load and include content
	if filePath != "" {
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
		data["content"] = string(content)
	}

	if len(data) == 0 {
		return fmt.Errorf("no updates specified")
	}

	body, _ := json.Marshal(data)
	resp, err := client.put("/api/v1/recipes/"+name, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	fmt.Printf("Recipe '%s' updated successfully.\n", name)
	return nil
}

func runRecipeDelete(cmd *cobra.Command, args []string) error {
	name := args[0]
	force, _ := cmd.Flags().GetBool("force")

	if !force {
		fmt.Printf("Are you sure you want to delete recipe '%s'? [y/N]: ", name)
		var confirm string
		fmt.Scanln(&confirm)
		if confirm != "y" && confirm != "Y" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.delete("/api/v1/recipes/" + name)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	fmt.Printf("Recipe '%s' deleted successfully.\n", name)
	return nil
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
