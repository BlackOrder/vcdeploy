// Package seeds provides seed data infrastructure for built-in recipes and playbooks.
package seeds

import "github.com/BlackOrder/vcdeploy/internal/storage"

// SeedComponents contains all built-in recipe components.
var SeedComponents = []SeedComponent{
	// Common Components
	{
		Slug:        "symlink-shared",
		Version:     "v1.0.0",
		Name:        "Symlink Shared Directories",
		Description: "Creates symlinks for shared directories (uploads, logs, etc.) that persist across deployments",
		Type:        "command",
		Content: storage.ComponentContent{
			Commands: []string{
				"for dir in {{shared_dirs}}; do",
				"  if [ -d \"{{shared_path}}/$dir\" ]; then",
				"    rm -rf \"{{release_path}}/$dir\"",
				"    ln -sf \"{{shared_path}}/$dir\" \"{{release_path}}/$dir\"",
				"  fi",
				"done",
			},
		},
		Variables: []storage.VariableDefinition{
			{Name: "shared_dirs", Type: "string", Required: true, Description: "Space-separated list of directories to symlink"},
			{Name: "shared_path", Type: "path", Required: true, Description: "Path to shared storage directory"},
			{Name: "release_path", Type: "path", Required: true, Description: "Path to current release directory"},
		},
	},
	{
		Slug:        "symlink-shared-files",
		Version:     "v1.0.0",
		Name:        "Symlink Shared Files",
		Description: "Creates symlinks for shared files (.env, config files) that persist across deployments",
		Type:        "command",
		Content: storage.ComponentContent{
			Commands: []string{
				"for file in {{shared_files}}; do",
				"  if [ -f \"{{shared_path}}/$file\" ]; then",
				"    rm -f \"{{release_path}}/$file\"",
				"    ln -sf \"{{shared_path}}/$file\" \"{{release_path}}/$file\"",
				"  fi",
				"done",
			},
		},
		Variables: []storage.VariableDefinition{
			{Name: "shared_files", Type: "string", Required: true, Description: "Space-separated list of files to symlink"},
			{Name: "shared_path", Type: "path", Required: true, Description: "Path to shared storage directory"},
			{Name: "release_path", Type: "path", Required: true, Description: "Path to current release directory"},
		},
	},
	{
		Slug:        "set-permissions",
		Version:     "v1.0.0",
		Name:        "Set Directory Permissions",
		Description: "Sets write permissions on specified directories (writable_dirs)",
		Type:        "command",
		Content: storage.ComponentContent{
			Commands: []string{
				"for dir in {{writable_dirs}}; do",
				"  chmod -R 775 \"{{release_path}}/$dir\"",
				"done",
			},
		},
		Variables: []storage.VariableDefinition{
			{Name: "writable_dirs", Type: "string", Required: true, Description: "Space-separated list of writable directories"},
			{Name: "release_path", Type: "path", Required: true, Description: "Path to current release directory"},
		},
	},

	// Laravel Components
	{
		Slug:        "laravel-artisan-migrate",
		Version:     "v1.0.0",
		Name:        "Laravel Database Migrations",
		Description: "Run Laravel database migrations with --force flag for production",
		Type:        "command",
		Content: storage.ComponentContent{
			Commands: []string{"{{php_binary}} artisan migrate --force"},
			WorkDir:  "{{release_path}}",
		},
		Variables: []storage.VariableDefinition{
			{Name: "php_binary", Type: "path", Required: false, Default: "php", Description: "Path to PHP binary"},
			{Name: "release_path", Type: "path", Required: true, Description: "Path to current release directory"},
		},
	},
	{
		Slug:        "laravel-cache-clear",
		Version:     "v1.0.0",
		Name:        "Laravel Clear All Caches",
		Description: "Clears Laravel application, config, route, and view caches",
		Type:        "command",
		Content: storage.ComponentContent{
			Commands: []string{
				"{{php_binary}} artisan cache:clear",
				"{{php_binary}} artisan config:clear",
				"{{php_binary}} artisan route:clear",
				"{{php_binary}} artisan view:clear",
			},
			WorkDir: "{{release_path}}",
		},
		Variables: []storage.VariableDefinition{
			{Name: "php_binary", Type: "path", Required: false, Default: "php", Description: "Path to PHP binary"},
			{Name: "release_path", Type: "path", Required: true, Description: "Path to current release directory"},
		},
	},
	{
		Slug:        "laravel-optimize",
		Version:     "v1.0.0",
		Name:        "Laravel Optimize",
		Description: "Optimizes Laravel for production (caches config, routes, and views)",
		Type:        "command",
		Content: storage.ComponentContent{
			Commands: []string{
				"{{php_binary}} artisan config:cache",
				"{{php_binary}} artisan route:cache",
				"{{php_binary}} artisan view:cache",
			},
			WorkDir: "{{release_path}}",
		},
		Variables: []storage.VariableDefinition{
			{Name: "php_binary", Type: "path", Required: false, Default: "php", Description: "Path to PHP binary"},
			{Name: "release_path", Type: "path", Required: true, Description: "Path to current release directory"},
		},
	},
	{
		Slug:        "laravel-queue-restart",
		Version:     "v1.0.0",
		Name:        "Laravel Queue Restart",
		Description: "Gracefully restart Laravel queue workers to pick up new code",
		Type:        "command",
		Content: storage.ComponentContent{
			Commands: []string{"{{php_binary}} artisan queue:restart"},
			WorkDir:  "{{release_path}}",
		},
		Variables: []storage.VariableDefinition{
			{Name: "php_binary", Type: "path", Required: false, Default: "php", Description: "Path to PHP binary"},
			{Name: "release_path", Type: "path", Required: true, Description: "Path to current release directory"},
		},
	},

	// Node.js Components
	{
		Slug:        "node-npm-install",
		Version:     "v1.0.0",
		Name:        "NPM Install (Production)",
		Description: "Installs Node.js dependencies using npm ci for reproducible builds",
		Type:        "command",
		Content: storage.ComponentContent{
			Commands: []string{"npm ci --production"},
			WorkDir:  "{{release_path}}",
		},
		Variables: []storage.VariableDefinition{
			{Name: "release_path", Type: "path", Required: true, Description: "Path to current release directory"},
		},
	},
	{
		Slug:        "node-npm-build",
		Version:     "v1.0.0",
		Name:        "NPM Build",
		Description: "Runs npm build script for production assets",
		Type:        "command",
		Content: storage.ComponentContent{
			Commands: []string{"npm run build"},
			WorkDir:  "{{release_path}}",
		},
		Variables: []storage.VariableDefinition{
			{Name: "release_path", Type: "path", Required: true, Description: "Path to current release directory"},
		},
	},
	{
		Slug:        "node-pm2-reload",
		Version:     "v1.0.0",
		Name:        "PM2 Reload Application",
		Description: "Gracefully reloads Node.js application using PM2",
		Type:        "command",
		Content: storage.ComponentContent{
			Commands: []string{"pm2 reload {{app_name}} --update-env"},
		},
		Variables: []storage.VariableDefinition{
			{Name: "app_name", Type: "string", Required: true, Description: "PM2 application name"},
		},
	},

	// Python Components
	{
		Slug:        "python-pip-install",
		Version:     "v1.0.0",
		Name:        "Python Pip Install",
		Description: "Installs Python dependencies from requirements.txt",
		Type:        "command",
		Content: storage.ComponentContent{
			Commands: []string{"{{pip_binary}} install -r requirements.txt --no-cache-dir"},
			WorkDir:  "{{release_path}}",
		},
		Variables: []storage.VariableDefinition{
			{Name: "pip_binary", Type: "path", Required: false, Default: "pip3", Description: "Path to pip binary"},
			{Name: "release_path", Type: "path", Required: true, Description: "Path to current release directory"},
		},
	},
	{
		Slug:        "python-django-migrate",
		Version:     "v1.0.0",
		Name:        "Django Database Migrations",
		Description: "Runs Django database migrations",
		Type:        "command",
		Content: storage.ComponentContent{
			Commands: []string{"{{python_binary}} manage.py migrate --noinput"},
			WorkDir:  "{{release_path}}",
		},
		Variables: []storage.VariableDefinition{
			{Name: "python_binary", Type: "path", Required: false, Default: "python3", Description: "Path to Python binary"},
			{Name: "release_path", Type: "path", Required: true, Description: "Path to current release directory"},
		},
	},
	{
		Slug:        "python-django-collectstatic",
		Version:     "v1.0.0",
		Name:        "Django Collect Static Files",
		Description: "Collects static files for Django applications",
		Type:        "command",
		Content: storage.ComponentContent{
			Commands: []string{"{{python_binary}} manage.py collectstatic --noinput"},
			WorkDir:  "{{release_path}}",
		},
		Variables: []storage.VariableDefinition{
			{Name: "python_binary", Type: "path", Required: false, Default: "python3", Description: "Path to Python binary"},
			{Name: "release_path", Type: "path", Required: true, Description: "Path to current release directory"},
		},
	},

	// Service Management Components
	{
		Slug:        "systemd-reload",
		Version:     "v1.0.0",
		Name:        "Systemd Service Reload",
		Description: "Reloads a systemd service",
		Type:        "service_reload",
		Content: storage.ComponentContent{
			Commands: []string{"systemctl reload {{service_name}}"},
		},
		Variables: []storage.VariableDefinition{
			{Name: "service_name", Type: "string", Required: true, Description: "Name of the systemd service"},
		},
	},
	{
		Slug:        "systemd-restart",
		Version:     "v1.0.0",
		Name:        "Systemd Service Restart",
		Description: "Restarts a systemd service",
		Type:        "service_reload",
		Content: storage.ComponentContent{
			Commands: []string{"systemctl restart {{service_name}}"},
		},
		Variables: []storage.VariableDefinition{
			{Name: "service_name", Type: "string", Required: true, Description: "Name of the systemd service"},
		},
	},
	{
		Slug:        "nginx-reload",
		Version:     "v1.0.0",
		Name:        "Nginx Reload Configuration",
		Description: "Reloads Nginx configuration without downtime",
		Type:        "service_reload",
		Content: storage.ComponentContent{
			Commands: []string{"nginx -t && systemctl reload nginx"},
		},
		Variables: []storage.VariableDefinition{},
	},
	{
		Slug:        "php-fpm-reload",
		Version:     "v1.0.0",
		Name:        "PHP-FPM Reload",
		Description: "Reloads PHP-FPM to pick up OPcache changes",
		Type:        "service_reload",
		Content: storage.ComponentContent{
			Commands: []string{"systemctl reload {{php_fpm_service}}"},
		},
		Variables: []storage.VariableDefinition{
			{Name: "php_fpm_service", Type: "string", Required: false, Default: "php-fpm", Description: "Name of PHP-FPM service (e.g., php8.2-fpm)"},
		},
	},
}
