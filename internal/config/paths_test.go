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

func TestMustGetSystemConfig(t *testing.T) {
	// Reset before test
	ResetSystemConfig()
	defer ResetSystemConfig()

	// Should not panic with defaults
	cfg := MustGetSystemConfig()
	if cfg == nil {
		t.Error("MustGetSystemConfig() returned nil")
	}
}

func TestGetSystemConfigSingleton(t *testing.T) {
	// Reset before test
	ResetSystemConfig()
	defer ResetSystemConfig()

	cfg1, _ := GetSystemConfig()
	cfg2, _ := GetSystemConfig()

	if cfg1 != cfg2 {
		t.Error("GetSystemConfig() should return same instance")
	}
}
