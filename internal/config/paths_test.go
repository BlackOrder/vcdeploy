package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetSystemConfig(t *testing.T) {
	// Reset before test
	ResetSystemConfig()
	defer ResetSystemConfig()

	cfg, err := GetSystemConfig()
	if err != nil {
		t.Fatalf("GetSystemConfig() error = %v", err)
	}

	if cfg == nil {
		t.Fatal("GetSystemConfig() returned nil")
	}

	// Check defaults
	if cfg.Paths.ConfigDir != DefaultConfigDir {
		t.Errorf("ConfigDir = %v, want %v", cfg.Paths.ConfigDir, DefaultConfigDir)
	}
	if cfg.Paths.DataDir != DefaultDataDir {
		t.Errorf("DataDir = %v, want %v", cfg.Paths.DataDir, DefaultDataDir)
	}
	if cfg.Paths.RunDir != DefaultRunDir {
		t.Errorf("RunDir = %v, want %v", cfg.Paths.RunDir, DefaultRunDir)
	}
	if cfg.Paths.LogDir != DefaultLogDir {
		t.Errorf("LogDir = %v, want %v", cfg.Paths.LogDir, DefaultLogDir)
	}
}

func TestSystemConfigPaths(t *testing.T) {
	cfg := &SystemConfig{
		Paths: SystemPaths{
			ConfigDir: "/etc/vcdeploy",
			DataDir:   "/var/lib/vcdeploy",
			RunDir:    "/var/run/vcdeploy",
			LogDir:    "/var/log/vcdeploy",
		},
	}

	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"DatabasePath", cfg.DatabasePath(), "/var/lib/vcdeploy/vcdeploy.db"},
		{"MasterConfigPath", cfg.MasterConfigPath(), "/etc/vcdeploy/master.yaml"},
		{"AgentConfigPath", cfg.AgentConfigPath(), "/etc/vcdeploy/agent.yaml"},
		{"MasterPIDPath", cfg.MasterPIDPath(), "/var/run/vcdeploy/vcdeploy.pid"},
		{"AgentPIDPath", cfg.AgentPIDPath(), "/var/run/vcdeploy/vcdeploy-agent.pid"},
		{"BackupsDir", cfg.BackupsDir(), "/var/lib/vcdeploy/backups"},
		{"SecretsBackupsDir", cfg.SecretsBackupsDir(), "/var/lib/vcdeploy/backups/secrets"},
		{"TemplatesDir", cfg.TemplatesDir(), "/var/lib/vcdeploy/templates"},
		{"StaticDir", cfg.StaticDir(), "/var/lib/vcdeploy/static"},
		{"MasterLogPath", cfg.MasterLogPath(), "/var/log/vcdeploy/master.log"},
		{"AgentLogPath", cfg.AgentLogPath(), "/var/log/vcdeploy/agent.log"},
		{"SSHKeysDir", cfg.SSHKeysDir(), "/var/lib/vcdeploy/ssh_keys"},
		{"CertsDir", cfg.CertsDir(), "/var/lib/vcdeploy/certs"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s() = %v, want %v", tt.name, tt.got, tt.expected)
			}
		})
	}
}

func TestEnsureDirectories(t *testing.T) {
	// Use temp directory for test
	tmpDir := t.TempDir()

	cfg := &SystemConfig{
		Paths: SystemPaths{
			ConfigDir: filepath.Join(tmpDir, "etc"),
			DataDir:   filepath.Join(tmpDir, "data"),
			RunDir:    filepath.Join(tmpDir, "run"),
			LogDir:    filepath.Join(tmpDir, "log"),
		},
	}

	if err := cfg.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories() error = %v", err)
	}

	// Check directories exist
	expectedDirs := []string{
		cfg.Paths.ConfigDir,
		cfg.Paths.DataDir,
		cfg.Paths.RunDir,
		cfg.Paths.LogDir,
		cfg.BackupsDir(),
		cfg.SecretsBackupsDir(),
		cfg.TemplatesDir(),
		cfg.StaticDir(),
		cfg.SSHKeysDir(),
		cfg.CertsDir(),
	}

	for _, dir := range expectedDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("Directory %s was not created", dir)
		}
	}
}

func TestGetSystemConfigOrDefaults(t *testing.T) {
	// Reset before test
	ResetSystemConfig()
	defer ResetSystemConfig()

	// Should always return valid config, never nil
	cfg := GetSystemConfigOrDefaults()
	if cfg == nil {
		t.Error("GetSystemConfigOrDefaults() returned nil")
	}
}

func TestGetSystemConfigSingleton(t *testing.T) {
	// Reset before test
	ResetSystemConfig()
	defer ResetSystemConfig()

	cfg1, err := GetSystemConfig()
	if err != nil {
		t.Fatalf("GetSystemConfig() first call error = %v", err)
	}
	cfg2, err := GetSystemConfig()
	if err != nil {
		t.Fatalf("GetSystemConfig() second call error = %v", err)
	}

	if cfg1 != cfg2 {
		t.Error("GetSystemConfig() should return same instance")
	}
}

func TestGetSystemConfigWithCustomPaths(t *testing.T) {
	// Reset before test
	ResetSystemConfig()
	defer ResetSystemConfig()

	// Create a custom config file
	tmpDir := t.TempDir()
	customConfigPath := filepath.Join(tmpDir, "vcdeploy.yaml")
	customConfig := `paths:
  config_dir: /custom/etc
  data_dir: /custom/data
  run_dir: /custom/run
  log_dir: /custom/log
`
	if err := os.WriteFile(customConfigPath, []byte(customConfig), 0o644); err != nil {
		t.Fatalf("Failed to write custom config: %v", err)
	}

	// Note: We can't easily test loading from custom path since DefaultSystemConfigPath is a constant
	// But we can test the default behavior
	cfg, err := GetSystemConfig()
	if err != nil {
		t.Fatalf("GetSystemConfig() error = %v", err)
	}
	if cfg == nil {
		t.Fatal("GetSystemConfig() returned nil")
	}
}

func TestEnsureDirectoriesError(t *testing.T) {
	// Test with a path that cannot be created (invalid path)
	cfg := &SystemConfig{
		Paths: SystemPaths{
			ConfigDir: "/dev/null/invalid/path",
			DataDir:   "/dev/null/invalid/path2",
			RunDir:    "/dev/null/invalid/path3",
			LogDir:    "/dev/null/invalid/path4",
		},
	}

	err := cfg.EnsureDirectories()
	// Should fail since /dev/null is not a directory
	if err == nil {
		t.Error("EnsureDirectories should fail with invalid path")
	}
}

func TestSystemConfigCustomPaths(t *testing.T) {
	cfg := &SystemConfig{
		Paths: SystemPaths{
			ConfigDir: "/custom/config",
			DataDir:   "/custom/data",
			RunDir:    "/custom/run",
			LogDir:    "/custom/log",
		},
	}

	// Test all path methods with custom values
	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"DatabasePath", cfg.DatabasePath(), "/custom/data/vcdeploy.db"},
		{"MasterConfigPath", cfg.MasterConfigPath(), "/custom/config/master.yaml"},
		{"AgentConfigPath", cfg.AgentConfigPath(), "/custom/config/agent.yaml"},
		{"MasterPIDPath", cfg.MasterPIDPath(), "/custom/run/vcdeploy.pid"},
		{"AgentPIDPath", cfg.AgentPIDPath(), "/custom/run/vcdeploy-agent.pid"},
		{"BackupsDir", cfg.BackupsDir(), "/custom/data/backups"},
		{"SecretsBackupsDir", cfg.SecretsBackupsDir(), "/custom/data/backups/secrets"},
		{"TemplatesDir", cfg.TemplatesDir(), "/custom/data/templates"},
		{"StaticDir", cfg.StaticDir(), "/custom/data/static"},
		{"MasterLogPath", cfg.MasterLogPath(), "/custom/log/master.log"},
		{"AgentLogPath", cfg.AgentLogPath(), "/custom/log/agent.log"},
		{"SSHKeysDir", cfg.SSHKeysDir(), "/custom/data/ssh_keys"},
		{"CertsDir", cfg.CertsDir(), "/custom/data/certs"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s() = %v, want %v", tt.name, tt.got, tt.expected)
			}
		})
	}
}

func TestResetSystemConfigMultipleTimes(t *testing.T) {
	// Reset multiple times to ensure it's safe
	ResetSystemConfig()
	ResetSystemConfig()
	ResetSystemConfig()

	cfg, err := GetSystemConfig()
	if err != nil {
		t.Fatalf("GetSystemConfig() after multiple resets error = %v", err)
	}
	if cfg == nil {
		t.Fatal("GetSystemConfig() returned nil after multiple resets")
	}

	ResetSystemConfig()
}
