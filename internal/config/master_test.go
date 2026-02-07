package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultMasterConfig(t *testing.T) {
	t.Parallel()

	config := DefaultMasterConfig()

	if config == nil {
		t.Fatal("DefaultMasterConfig returned nil")
	}

	// Check default values
	if config.Server.Listen != ":9000" {
		t.Errorf("unexpected default server listen: %s", config.Server.Listen)
	}
	if config.GRPC.Listen != ":9001" {
		t.Errorf("unexpected default gRPC listen: %s", config.GRPC.Listen)
	}
	if !config.Server.TLS.Enabled {
		t.Error("expected TLS to be enabled by default")
	}
	if config.SSH.DefaultUser != "deploy" {
		t.Errorf("unexpected default SSH user: %s", config.SSH.DefaultUser)
	}
	if config.Security.SessionTimeout != 24*time.Hour {
		t.Errorf("unexpected default session timeout: %v", config.Security.SessionTimeout)
	}
	if !config.Security.Require2FAAdmin {
		t.Error("expected 2FA to be required for admin by default")
	}
	if !config.Webhooks.GitHub.Enabled {
		t.Error("expected GitHub webhooks to be enabled by default")
	}
}

func TestLoadMasterConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		content   string
		wantErr   bool
		checkFunc func(*testing.T, *MasterConfig)
	}{
		{
			name: "valid config",
			content: `
server:
  listen: ":8080"
  tls:
    enabled: false
grpc:
  listen: ":9001"
ssh:
  default_user: testuser
  connection_timeout: 60s
security:
  session_timeout: 12h
  require_2fa_admin: false
`,
			wantErr: false,
			checkFunc: func(t *testing.T, c *MasterConfig) {
				if c.Server.Listen != ":8080" {
					t.Errorf("unexpected server listen: %s", c.Server.Listen)
				}
				if c.GRPC.Listen != ":9001" {
					t.Errorf("unexpected gRPC listen: %s", c.GRPC.Listen)
				}
				if c.Server.TLS.Enabled {
					t.Error("expected TLS to be disabled")
				}
				if c.SSH.DefaultUser != "testuser" {
					t.Errorf("unexpected SSH user: %s", c.SSH.DefaultUser)
				}
				if c.Security.SessionTimeout != 12*time.Hour {
					t.Errorf("unexpected session timeout: %v", c.Security.SessionTimeout)
				}
			},
		},
		{
			name: "minimal config with defaults",
			content: `
server:
  listen: ":8000"
grpc:
  listen: ":9000"
`,
			wantErr: false,
			checkFunc: func(t *testing.T, c *MasterConfig) {
				if c.Server.Listen != ":8000" {
					t.Errorf("unexpected server listen: %s", c.Server.Listen)
				}
				// Defaults should be preserved
				if c.SSH.DefaultUser != "deploy" {
					t.Errorf("default SSH user not preserved: %s", c.SSH.DefaultUser)
				}
				if !c.Webhooks.GitHub.Enabled {
					t.Error("default GitHub webhook setting not preserved")
				}
			},
		},
		{
			name:    "invalid yaml",
			content: `invalid: [yaml: broken`,
			wantErr: true,
		},
		{
			name: "webhooks config",
			content: `
server:
  listen: ":8080"
grpc:
  listen: ":9001"
webhooks:
  github:
    enabled: true
    path: /hooks/gh
  gitlab:
    enabled: false
  bitbucket:
    enabled: true
    path: /hooks/bb
`,
			wantErr: false,
			checkFunc: func(t *testing.T, c *MasterConfig) {
				if !c.Webhooks.GitHub.Enabled {
					t.Error("expected GitHub webhooks to be enabled")
				}
				if c.Webhooks.GitHub.Path != "/hooks/gh" {
					t.Errorf("unexpected GitHub webhook path: %s", c.Webhooks.GitHub.Path)
				}
				if c.Webhooks.GitLab.Enabled {
					t.Error("expected GitLab webhooks to be disabled")
				}
				if !c.Webhooks.Bitbucket.Enabled {
					t.Error("expected Bitbucket webhooks to be enabled")
				}
			},
		},
		{
			name: "backup config",
			content: `
server:
  listen: ":8080"
grpc:
  listen: ":9001"
backup:
  database:
    enabled: true
    interval: 168h
    retention: 720h
    path: /backups
  config:
    versions: 10
`,
			wantErr: false,
			checkFunc: func(t *testing.T, c *MasterConfig) {
				if !c.Backup.Database.Enabled {
					t.Error("expected database backup to be enabled")
				}
				if c.Backup.Database.Interval != 168*time.Hour {
					t.Errorf("unexpected backup interval: %v", c.Backup.Database.Interval)
				}
				if c.Backup.Config.Versions != 10 {
					t.Errorf("unexpected config versions: %d", c.Backup.Config.Versions)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "master.yaml")
			if err := os.WriteFile(configPath, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("failed to write config file: %v", err)
			}

			config, err := LoadMasterConfig(configPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadMasterConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.checkFunc != nil {
				tt.checkFunc(t, config)
			}
		})
	}
}

func TestLoadMasterConfig_FileNotFound(t *testing.T) {
	t.Parallel()

	// When file doesn't exist, should return defaults (not error)
	config, err := LoadMasterConfig("/nonexistent/path/config.yaml")
	if err != nil {
		t.Errorf("expected no error for nonexistent file, got: %v", err)
	}
	if config == nil {
		t.Fatal("expected default config, got nil")
	}
	if config.Server.Listen != ":9000" {
		t.Errorf("expected default server listen, got: %s", config.Server.Listen)
	}
}

func TestLoadMaster(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "master.yaml")
	content := `
server:
  listen: ":7000"
grpc:
  listen: ":7001"
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	config, err := LoadMaster(configPath)
	if err != nil {
		t.Fatalf("LoadMaster failed: %v", err)
	}
	if config.Server.Listen != ":7000" {
		t.Errorf("unexpected server listen: %s", config.Server.Listen)
	}
}

func TestSaveMasterConfig(t *testing.T) {
	t.Parallel()

	config := DefaultMasterConfig()
	config.Server.Listen = ":6000"
	config.Security.SessionTimeout = 48 * time.Hour

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "subdir", "master.yaml")

	// Save config
	if err := SaveMasterConfig(config, configPath); err != nil {
		t.Fatalf("SaveMasterConfig failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created")
	}

	// Load and verify
	loaded, err := LoadMasterConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load saved config: %v", err)
	}

	if loaded.Server.Listen != config.Server.Listen {
		t.Errorf("server listen mismatch: got %s, want %s", loaded.Server.Listen, config.Server.Listen)
	}
	if loaded.Security.SessionTimeout != config.Security.SessionTimeout {
		t.Errorf("session timeout mismatch: got %v, want %v", loaded.Security.SessionTimeout, config.Security.SessionTimeout)
	}
}

func TestMasterConfigValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  *MasterConfig
		wantErr bool
	}{
		{
			name:    "valid default config",
			config:  DefaultMasterConfig(),
			wantErr: false,
		},
		{
			name: "missing server listen",
			config: func() *MasterConfig {
				c := DefaultMasterConfig()
				c.Server.Listen = ""
				return c
			}(),
			wantErr: true,
		},
		{
			name: "missing grpc listen",
			config: func() *MasterConfig {
				c := DefaultMasterConfig()
				c.GRPC.Listen = ""
				return c
			}(),
			wantErr: true,
		},
		{
			name: "key rotation interval too short",
			config: func() *MasterConfig {
				c := DefaultMasterConfig()
				c.Security.KeyRotation.Enabled = true
				c.Security.KeyRotation.Interval = 30 * time.Minute
				return c
			}(),
			wantErr: true,
		},
		{
			name: "key rotation disabled with short interval is ok",
			config: func() *MasterConfig {
				c := DefaultMasterConfig()
				c.Security.KeyRotation.Enabled = false
				c.Security.KeyRotation.Interval = 30 * time.Minute
				return c
			}(),
			wantErr: false,
		},
		{
			name: "backup versions zero",
			config: func() *MasterConfig {
				c := DefaultMasterConfig()
				c.Backup.Config.Versions = 0
				return c
			}(),
			wantErr: true,
		},
		{
			name: "minimal valid config",
			config: &MasterConfig{
				Server: ServerConfig{
					Listen: ":8080",
				},
				GRPC: GRPCConfig{
					Listen: ":9001",
				},
				Backup: BackupConfig{
					Config: ConfigBackupConfig{
						Versions: 1,
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
