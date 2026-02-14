// Package seeds provides seed data infrastructure for built-in recipes and playbooks.
package seeds

import "github.com/BlackOrder/vcdeploy/internal/storage"

// SeedPlaybooks contains all built-in playbooks.
var SeedPlaybooks = []SeedPlaybook{
	// Laravel Playbooks
	{
		Slug:          "laravel-standard",
		Version:       "v1.0.0",
		Name:          "Laravel Standard Deployment",
		Description:   "Standard Laravel deployment with migrations, cache clearing, and optimization",
		FrameworkType: "laravel",
		Steps: []storage.PlaybookStep{
			{Order: 1, ComponentRef: "seed:symlink-shared:v1.0.0", Phase: storage.PhaseDeploy},
			{Order: 2, ComponentRef: "seed:symlink-shared-files:v1.0.0", Phase: storage.PhaseDeploy},
			{Order: 3, ComponentRef: "seed:set-permissions:v1.0.0", Phase: storage.PhaseDeploy},
			{Order: 4, ComponentRef: "seed:laravel-cache-clear:v1.0.0", Phase: storage.PhaseDeploy},
			{Order: 5, ComponentRef: "seed:laravel-artisan-migrate:v1.0.0", Phase: storage.PhaseDeploy},
			{Order: 6, ComponentRef: "seed:laravel-optimize:v1.0.0", Phase: storage.PhasePostDeploy},
			{Order: 7, ComponentRef: "seed:php-fpm-reload:v1.0.0", Phase: storage.PhasePostDeploy},
			{Order: 8, ComponentRef: "seed:laravel-queue-restart:v1.0.0", Phase: storage.PhasePostDeploy},
		},
		SharedDirs:   []string{"storage"},
		SharedFiles:  []string{".env"},
		WritableDirs: []string{"storage", "bootstrap/cache"},
		KeepReleases: 5,
	},
	{
		Slug:          "laravel-minimal",
		Version:       "v1.0.0",
		Name:          "Laravel Minimal Deployment",
		Description:   "Minimal Laravel deployment without migrations (for static content updates)",
		FrameworkType: "laravel",
		Steps: []storage.PlaybookStep{
			{Order: 1, ComponentRef: "seed:symlink-shared:v1.0.0", Phase: storage.PhaseDeploy},
			{Order: 2, ComponentRef: "seed:symlink-shared-files:v1.0.0", Phase: storage.PhaseDeploy},
			{Order: 3, ComponentRef: "seed:laravel-cache-clear:v1.0.0", Phase: storage.PhasePostDeploy},
			{Order: 4, ComponentRef: "seed:php-fpm-reload:v1.0.0", Phase: storage.PhasePostDeploy},
		},
		SharedDirs:   []string{"storage"},
		SharedFiles:  []string{".env"},
		WritableDirs: []string{"storage", "bootstrap/cache"},
		KeepReleases: 5,
	},

	// Node.js Playbooks
	{
		Slug:          "nodejs-pm2",
		Version:       "v1.0.0",
		Name:          "Node.js with PM2",
		Description:   "Node.js application deployment using PM2 process manager",
		FrameworkType: "nodejs",
		Steps: []storage.PlaybookStep{
			{Order: 1, ComponentRef: "seed:symlink-shared:v1.0.0", Phase: storage.PhaseDeploy},
			{Order: 2, ComponentRef: "seed:symlink-shared-files:v1.0.0", Phase: storage.PhaseDeploy},
			{Order: 3, ComponentRef: "seed:node-npm-install:v1.0.0", Phase: storage.PhaseDeploy},
			{Order: 4, ComponentRef: "seed:node-npm-build:v1.0.0", Phase: storage.PhaseDeploy},
			{Order: 5, ComponentRef: "seed:node-pm2-reload:v1.0.0", Phase: storage.PhasePostDeploy},
		},
		SharedDirs:   []string{"logs", "uploads"},
		SharedFiles:  []string{".env"},
		WritableDirs: []string{"logs"},
		KeepReleases: 5,
	},
	{
		Slug:          "nodejs-static",
		Version:       "v1.0.0",
		Name:          "Node.js Static Build",
		Description:   "Build and deploy static assets from a Node.js project (React, Vue, etc.)",
		FrameworkType: "nodejs",
		Steps: []storage.PlaybookStep{
			{Order: 1, ComponentRef: "seed:node-npm-install:v1.0.0", Phase: storage.PhaseDeploy},
			{Order: 2, ComponentRef: "seed:node-npm-build:v1.0.0", Phase: storage.PhaseDeploy},
			{Order: 3, ComponentRef: "seed:nginx-reload:v1.0.0", Phase: storage.PhasePostDeploy},
		},
		SharedDirs:   []string{},
		SharedFiles:  []string{".env"},
		WritableDirs: []string{},
		KeepReleases: 3,
	},

	// Python/Django Playbooks
	{
		Slug:          "django-standard",
		Version:       "v1.0.0",
		Name:          "Django Standard Deployment",
		Description:   "Django deployment with migrations and static file collection",
		FrameworkType: "django",
		Steps: []storage.PlaybookStep{
			{Order: 1, ComponentRef: "seed:symlink-shared:v1.0.0", Phase: storage.PhaseDeploy},
			{Order: 2, ComponentRef: "seed:symlink-shared-files:v1.0.0", Phase: storage.PhaseDeploy},
			{Order: 3, ComponentRef: "seed:python-pip-install:v1.0.0", Phase: storage.PhaseDeploy},
			{Order: 4, ComponentRef: "seed:python-django-migrate:v1.0.0", Phase: storage.PhaseDeploy},
			{Order: 5, ComponentRef: "seed:python-django-collectstatic:v1.0.0", Phase: storage.PhaseDeploy},
			{Order: 6, ComponentRef: "seed:systemd-reload:v1.0.0", Phase: storage.PhasePostDeploy},
		},
		SharedDirs:   []string{"media", "logs"},
		SharedFiles:  []string{".env", "settings_local.py"},
		WritableDirs: []string{"media", "logs", "static"},
		KeepReleases: 5,
	},

	// Generic Playbooks
	{
		Slug:          "generic-static",
		Version:       "v1.0.0",
		Name:          "Generic Static Site",
		Description:   "Simple static file deployment with optional nginx reload",
		FrameworkType: "generic",
		Steps: []storage.PlaybookStep{
			{Order: 1, ComponentRef: "seed:nginx-reload:v1.0.0", Phase: storage.PhasePostDeploy},
		},
		SharedDirs:   []string{},
		SharedFiles:  []string{},
		WritableDirs: []string{},
		KeepReleases: 3,
	},
	{
		Slug:          "generic-systemd",
		Version:       "v1.0.0",
		Name:          "Generic Systemd Service",
		Description:   "Deploy an application managed by systemd",
		FrameworkType: "generic",
		Steps: []storage.PlaybookStep{
			{Order: 1, ComponentRef: "seed:symlink-shared:v1.0.0", Phase: storage.PhaseDeploy},
			{Order: 2, ComponentRef: "seed:symlink-shared-files:v1.0.0", Phase: storage.PhaseDeploy},
			{Order: 3, ComponentRef: "seed:set-permissions:v1.0.0", Phase: storage.PhaseDeploy},
			{Order: 4, ComponentRef: "seed:systemd-restart:v1.0.0", Phase: storage.PhasePostDeploy},
		},
		SharedDirs:   []string{"logs", "data"},
		SharedFiles:  []string{".env"},
		WritableDirs: []string{"logs", "data"},
		KeepReleases: 5,
	},
}
