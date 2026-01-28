// Package config provides tests for type configuration.
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTypeConfig(t *testing.T) {
	cfg := TypeConfig{
		Name:         "laravel",
		SharedDirs:   []string{"storage"},
		SharedFiles:  []string{".env"},
		WritableDirs: []string{"bootstrap/cache"},
		KeepReleases: 5,
	}

	if cfg.Name != "laravel" {
		t.Errorf("TypeConfig.Name = %v, want laravel", cfg.Name)
	}

	if cfg.KeepReleases != 5 {
		t.Errorf("TypeConfig.KeepReleases = %d, want 5", cfg.KeepReleases)
	}

	if len(cfg.SharedDirs) != 1 {
		t.Errorf("TypeConfig.SharedDirs count = %d, want 1", len(cfg.SharedDirs))
	}
}

func TestTypeConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *TypeConfig
		wantErr bool
	}{
		{
			name: "valid",
			cfg: &TypeConfig{
				Name:         "test",
				KeepReleases: 5,
			},
			wantErr: false,
		},
		{
			name: "missing name",
			cfg: &TypeConfig{
				KeepReleases: 5,
			},
			wantErr: true,
		},
		{
			name: "invalid keep_releases",
			cfg: &TypeConfig{
				Name:         "test",
				KeepReleases: 0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			hasErr := err != nil
			if hasErr != tt.wantErr {
				t.Errorf("Validate() hasErr = %v, wantErr = %v, err = %v", hasErr, tt.wantErr, err)
			}
		})
	}
}

func TestLoadTypeConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test config file
	configPath := filepath.Join(tmpDir, "test-type.yaml")
	configContent := `name: test-type
shared_dirs:
  - storage
  - logs
shared_files:
  - .env
keep_releases: 10
hooks:
  pre_deploy:
    - composer install
  post_deploy:
    - php artisan migrate
`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadTypeConfig(configPath)
	if err != nil {
		t.Fatalf("LoadTypeConfig() error = %v", err)
	}

	if cfg.Name != "test-type" {
		t.Errorf("Name = %v, want test-type", cfg.Name)
	}

	if cfg.KeepReleases != 10 {
		t.Errorf("KeepReleases = %d, want 10", cfg.KeepReleases)
	}

	if len(cfg.SharedDirs) != 2 {
		t.Errorf("SharedDirs count = %d, want 2", len(cfg.SharedDirs))
	}

	if len(cfg.Hooks.PreDeploy) != 1 {
		t.Errorf("PreDeploy hooks count = %d, want 1", len(cfg.Hooks.PreDeploy))
	}
}

func TestLoadTypeConfigDefaults(t *testing.T) {
	tmpDir := t.TempDir()

	// Config without keep_releases
	configPath := filepath.Join(tmpDir, "minimal.yaml")
	configContent := `name: minimal`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := LoadTypeConfig(configPath)
	if err != nil {
		t.Fatalf("LoadTypeConfig() error = %v", err)
	}

	// Should default to 5
	if cfg.KeepReleases != 5 {
		t.Errorf("Default KeepReleases = %d, want 5", cfg.KeepReleases)
	}
}

func TestLoadTypeConfigNotFound(t *testing.T) {
	_, err := LoadTypeConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("LoadTypeConfig() should return error for nonexistent file")
	}
}

func TestLoadTypeConfigInvalid(t *testing.T) {
	tmpDir := t.TempDir()

	configPath := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(configPath, []byte("invalid: [yaml: content"), 0o644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	_, err := LoadTypeConfig(configPath)
	if err == nil {
		t.Error("LoadTypeConfig() should return error for invalid YAML")
	}
}

func TestSaveTypeConfig(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &TypeConfig{
		Name:         "saved-type",
		SharedDirs:   []string{"storage"},
		KeepReleases: 7,
	}

	savePath := filepath.Join(tmpDir, "nested", "saved.yaml")

	err := SaveTypeConfig(cfg, savePath)
	if err != nil {
		t.Fatalf("SaveTypeConfig() error = %v", err)
	}

	// Load it back and verify
	loaded, err := LoadTypeConfig(savePath)
	if err != nil {
		t.Fatalf("LoadTypeConfig() error = %v", err)
	}

	if loaded.Name != cfg.Name {
		t.Errorf("Loaded Name = %v, want %v", loaded.Name, cfg.Name)
	}

	if loaded.KeepReleases != cfg.KeepReleases {
		t.Errorf("Loaded KeepReleases = %d, want %d", loaded.KeepReleases, cfg.KeepReleases)
	}
}

func TestBuiltinTypes(t *testing.T) {
	types := BuiltinTypes()
	if types == nil {
		t.Fatal("BuiltinTypes() returned nil")
	}

	// Check for some expected types
	expectedTypes := []string{"laravel", "nodejs", "static"}
	for _, name := range expectedTypes {
		if _, ok := types[name]; !ok {
			t.Errorf("BuiltinTypes() missing type: %s", name)
		}
	}
}

func TestBuiltinTypeLaravel(t *testing.T) {
	types := BuiltinTypes()
	laravel := types["laravel"]

	if laravel == nil {
		t.Fatal("Laravel type not found")
	}

	if laravel.Name != "laravel" {
		t.Errorf("Laravel.Name = %v, want laravel", laravel.Name)
	}

	// Laravel should have storage as shared dir
	hasStorage := false
	for _, dir := range laravel.SharedDirs {
		if dir == "storage" {
			hasStorage = true
			break
		}
	}
	if !hasStorage {
		t.Error("Laravel type should have 'storage' in SharedDirs")
	}

	// Laravel should have .env as shared file
	hasEnv := false
	for _, file := range laravel.SharedFiles {
		if file == ".env" {
			hasEnv = true
			break
		}
	}
	if !hasEnv {
		t.Error("Laravel type should have '.env' in SharedFiles")
	}
}

func TestBuiltinTypeNodeJS(t *testing.T) {
	types := BuiltinTypes()
	nodejs := types["nodejs"]

	if nodejs == nil {
		t.Fatal("NodeJS type not found")
	}

	if nodejs.Name != "nodejs" {
		t.Errorf("NodeJS.Name = %v, want nodejs", nodejs.Name)
	}

	// NodeJS should have package.json in required files
	hasPackageJSON := false
	for _, file := range nodejs.Validation.RequiredFiles {
		if file == "package.json" {
			hasPackageJSON = true
			break
		}
	}
	if !hasPackageJSON {
		t.Error("NodeJS type should have 'package.json' in RequiredFiles")
	}
}

func TestTypeValidation(t *testing.T) {
	validation := TypeValidation{
		RequiredFiles: []string{"package.json", "package-lock.json"},
		Env: EnvValidation{
			RequiredKeys: []string{"APP_KEY", "DB_HOST"},
			Patterns: map[string]string{
				"APP_KEY": "^base64:.+",
			},
		},
	}

	if len(validation.RequiredFiles) != 2 {
		t.Errorf("RequiredFiles count = %d, want 2", len(validation.RequiredFiles))
	}

	if len(validation.Env.RequiredKeys) != 2 {
		t.Errorf("Env.RequiredKeys count = %d, want 2", len(validation.Env.RequiredKeys))
	}
}

func TestTypeHooks(t *testing.T) {
	hooks := TypeHooks{
		PreDeploy:  []string{"npm install", "npm run build"},
		PostDeploy: []string{"npm run migrate"},
		Reload: []ServiceAction{
			{Service: "nginx", Action: "reload"},
		},
		Rollback: []string{"npm run rollback"},
	}

	if len(hooks.PreDeploy) != 2 {
		t.Errorf("PreDeploy count = %d, want 2", len(hooks.PreDeploy))
	}

	if len(hooks.Reload) != 1 {
		t.Errorf("Reload count = %d, want 1", len(hooks.Reload))
	}
}

// Benchmark tests
func BenchmarkLoadTypeConfig(b *testing.B) {
	tmpDir := b.TempDir()

	configPath := filepath.Join(tmpDir, "bench.yaml")
	configContent := `name: benchmark
shared_dirs:
  - storage
  - logs
  - cache
keep_releases: 10
hooks:
  pre_deploy:
    - composer install
    - npm install
    - npm run build
`
	_ = os.WriteFile(configPath, []byte(configContent), 0o644)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = LoadTypeConfig(configPath)
	}
}

func BenchmarkSaveTypeConfig(b *testing.B) {
	tmpDir := b.TempDir()

	cfg := &TypeConfig{
		Name:         "benchmark",
		SharedDirs:   []string{"storage", "logs", "cache"},
		KeepReleases: 10,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		savePath := filepath.Join(tmpDir, "bench.yaml")
		_ = SaveTypeConfig(cfg, savePath)
	}
}
