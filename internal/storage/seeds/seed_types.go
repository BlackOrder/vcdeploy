// Package seeds provides seed data infrastructure for built-in recipes, playbooks, and project types.
package seeds

// SeedProjectType defines a built-in project type to seed into the database.
type SeedProjectType struct {
	Name        string
	Description string
	BuildCmd    string
}

// SeedProjectTypes contains the built-in project types seeded on first boot.
// These are the framework archetypes that vcdeploy supports out of the box.
var SeedProjectTypes = []SeedProjectType{
	{
		Name:        "generic",
		Description: "Generic project type",
	},
	{
		Name:        "laravel",
		Description: "PHP Laravel application with Composer, Artisan, and shared storage directory",
		BuildCmd:    "composer install --no-dev --optimize-autoloader",
	},
	{
		Name:        "symfony",
		Description: "PHP Symfony application with Composer, Doctrine migrations, and cache warmup",
		BuildCmd:    "composer install --no-dev --optimize-autoloader",
	},
	{
		Name:        "nextjs",
		Description: "Next.js application with npm build step and .next/cache sharing",
		BuildCmd:    "npm ci && npm run build",
	},
	{
		Name:        "static",
		Description: "Static site — no build step, serves index.html directly",
		BuildCmd:    "",
	},
	{
		Name:        "nodejs",
		Description: "Node.js application with npm production install and PM2 reload",
		BuildCmd:    "npm ci --production",
	},
}
