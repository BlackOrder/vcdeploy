// Package config defines configuration structures for vcdeploy.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// TypeConfig defines a project type (template).
type TypeConfig struct {
	Name         string         `yaml:"name"`
	SharedDirs   []string       `yaml:"shared_dirs,omitempty"`
	SharedFiles  []string       `yaml:"shared_files,omitempty"`
	WritableDirs []string       `yaml:"writable_dirs,omitempty"`
	KeepReleases int            `yaml:"keep_releases,omitempty"`
	Validation   TypeValidation `yaml:"validation,omitempty"`
	Hooks        TypeHooks      `yaml:"hooks,omitempty"`
}

// TypeValidation defines validation rules for a project type.
type TypeValidation struct {
	RequiredFiles []string      `yaml:"required_files,omitempty"`
	Env           EnvValidation `yaml:"env,omitempty"`
}

// EnvValidation defines environment variable validation.
type EnvValidation struct {
	RequiredKeys []string          `yaml:"required_keys,omitempty"`
	Patterns     map[string]string `yaml:"patterns,omitempty"` // key -> regex pattern
}

// TypeHooks defines default hooks for a project type.
type TypeHooks struct {
	PreDeploy  []string        `yaml:"pre_deploy,omitempty"`
	PostDeploy []string        `yaml:"post_deploy,omitempty"`
	Reload     []ServiceAction `yaml:"reload,omitempty"`
	Rollback   []string        `yaml:"rollback,omitempty"`
}

// LoadTypeConfig loads a type configuration from a file.
func LoadTypeConfig(path string) (*TypeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading type file: %w", err)
	}

	var config TypeConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing type file: %w", err)
	}

	// Apply defaults
	if config.KeepReleases == 0 {
		config.KeepReleases = 5
	}

	return &config, nil
}

// SaveTypeConfig saves a type configuration to a file.
func SaveTypeConfig(config *TypeConfig, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating type directory: %w", err)
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshaling type: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing type file: %w", err)
	}

	return nil
}

// Validate validates the type configuration.
func (c *TypeConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("name is required")
	}
	if c.KeepReleases < 1 {
		return fmt.Errorf("keep_releases must be at least 1")
	}
	return nil
}

// BuiltinTypes returns the built-in project types.
func BuiltinTypes() map[string]*TypeConfig {
	return map[string]*TypeConfig{
		"laravel": {
			Name:         "laravel",
			SharedDirs:   []string{"storage"},
			SharedFiles:  []string{".env"},
			WritableDirs: []string{"bootstrap/cache", "storage"},
			KeepReleases: 5,
			Validation: TypeValidation{
				RequiredFiles: []string{"artisan", "composer.json"},
				Env: EnvValidation{
					RequiredKeys: []string{"APP_KEY", "APP_ENV", "DB_CONNECTION"},
				},
			},
			Hooks: TypeHooks{
				PostDeploy: []string{
					"composer install --no-dev --optimize-autoloader",
					"php artisan migrate --force",
					"php artisan config:cache",
					"php artisan route:cache",
					"php artisan view:cache",
				},
			},
		},
		"symfony": {
			Name:         "symfony",
			SharedDirs:   []string{"var/log", "var/sessions"},
			SharedFiles:  []string{".env.local"},
			WritableDirs: []string{"var"},
			KeepReleases: 5,
			Validation: TypeValidation{
				RequiredFiles: []string{"bin/console", "composer.json"},
			},
			Hooks: TypeHooks{
				PostDeploy: []string{
					"composer install --no-dev --optimize-autoloader",
					"bin/console doctrine:migrations:migrate --no-interaction",
					"bin/console cache:clear --env=prod",
					"bin/console cache:warmup --env=prod",
				},
			},
		},
		"nextjs": {
			Name:         "nextjs",
			SharedDirs:   []string{".next/cache"},
			SharedFiles:  []string{".env.local"},
			KeepReleases: 5,
			Validation: TypeValidation{
				RequiredFiles: []string{"package.json", "next.config.js"},
			},
			Hooks: TypeHooks{
				PostDeploy: []string{
					"npm ci",
					"npm run build",
				},
			},
		},
		"static": {
			Name:         "static",
			KeepReleases: 5,
			Validation: TypeValidation{
				RequiredFiles: []string{"index.html"},
			},
		},
		"nodejs": {
			Name:         "nodejs",
			SharedFiles:  []string{".env"},
			KeepReleases: 5,
			Validation: TypeValidation{
				RequiredFiles: []string{"package.json"},
			},
			Hooks: TypeHooks{
				PostDeploy: []string{
					"npm ci --production",
				},
				Reload: []ServiceAction{
					{Service: "pm2", Action: "reload"},
				},
			},
		},
	}
}
