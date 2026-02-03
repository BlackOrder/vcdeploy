// Package seeds provides seed data infrastructure for built-in recipes and playbooks.
package seeds

// SeedComponents contains all built-in recipe components.
// TODO: Populate in Phase 15 after implementation research.
// See plan-securityAndRecipes.prompt.md Stage 15 for requirements.
var SeedComponents = []SeedComponent{
	// Example structure (commented out until Phase 15):
	// {
	// 	Slug:        "laravel:artisan-migrate",
	// 	Version:     "v1.0.0",
	// 	Name:        "Laravel Artisan Migrate",
	// 	Description: "Run Laravel database migrations",
	// 	Type:        "command",
	// 	Content: storage.ComponentContent{
	// 		Commands: []string{"{{php_binary}} artisan migrate --force"},
	// 		WorkDir:  "{{release_path}}",
	// 	},
	// 	Variables: []storage.VariableDefinition{
	// 		{Name: "php_binary", Type: "path", Required: false, Default: "php"},
	// 	},
	// },
}
