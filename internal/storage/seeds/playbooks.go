// Package seeds provides seed data infrastructure for built-in recipes and playbooks.
package seeds

// SeedPlaybooks contains all built-in playbooks.
// TODO: Populate in Phase 15 after implementation research.
// See plan-securityAndRecipes.prompt.md Stage 15 for requirements.
var SeedPlaybooks = []SeedPlaybook{
	// Example structure (commented out until Phase 15):
	// {
	// 	Slug:          "laravel:standard",
	// 	Version:       "v1.0.0",
	// 	Name:          "Laravel Standard Deployment",
	// 	Description:   "Standard Laravel deployment with migrations and cache clearing",
	// 	FrameworkType: "laravel",
	// 	Steps: []storage.PlaybookStep{
	// 		{Order: 1, ComponentRef: "seed:common:symlink-shared:v1.0.0", Phase: "deploy"},
	// 		{Order: 2, ComponentRef: "seed:laravel:artisan-migrate:v1.0.0", Phase: "deploy"},
	// 		{Order: 3, ComponentRef: "seed:laravel:cache-clear:v1.0.0", Phase: "post_deploy"},
	// 	},
	// 	SharedDirs:   []string{"storage"},
	// 	SharedFiles:  []string{".env"},
	// 	WritableDirs: []string{"storage", "bootstrap/cache"},
	// 	KeepReleases: 5,
	// },
}
