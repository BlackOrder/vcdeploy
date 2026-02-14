package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultAgentConfig(t *testing.T) {
	t.Parallel()

	config := DefaultAgentConfig()

	if config == nil {
		t.Fatal("DefaultAgentConfig returned nil")
	}

	// Check default values
	if config.Master.CACert != "/etc/vcdeploy/agent/ca.pem" {
		t.Errorf("unexpected default CA cert path: %s", config.Master.CACert)
	}
	if config.Master.Reconnect.InitialDelay != 1*time.Second {
		t.Errorf("unexpected default initial delay: %v", config.Master.Reconnect.InitialDelay)
	}
	if config.Master.Reconnect.MaxDelay != 5*time.Minute {
		t.Errorf("unexpected default max delay: %v", config.Master.Reconnect.MaxDelay)
	}
	if config.Paths.Repos != "/var/lib/vcdeploy/repos/" {
		t.Errorf("unexpected default repos path: %s", config.Paths.Repos)
	}
	if config.Execution.Timeout != 10*time.Minute {
		t.Errorf("unexpected default timeout: %v", config.Execution.Timeout)
	}
}

func TestLoadAgentConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		content   string
		wantErr   bool
		checkFunc func(*testing.T, *AgentConfig)
	}{
		{
			name: "valid config",
			content: `
master:
  address: "localhost:9001"
  token: "test-token"
  reconnect:
    initial_delay: 2s
    max_delay: 10m
    heartbeat_interval: 30s
agent:
  id: "test-agent"
  labels:
    env: test
paths:
  repos: /tmp/repos
  releases: /tmp/releases
execution:
  user: testuser
  timeout: 5m
`,
			wantErr: false,
			checkFunc: func(t *testing.T, c *AgentConfig) {
				if c.Master.Address != "localhost:9001" {
					t.Errorf("unexpected address: %s", c.Master.Address)
				}
				if c.Agent.ID != "test-agent" {
					t.Errorf("unexpected agent id: %s", c.Agent.ID)
				}
				if c.Master.Reconnect.InitialDelay != 2*time.Second {
					t.Errorf("unexpected initial delay: %v", c.Master.Reconnect.InitialDelay)
				}
				if c.Execution.Timeout != 5*time.Minute {
					t.Errorf("unexpected timeout: %v", c.Execution.Timeout)
				}
			},
		},
		{
			name: "minimal config with defaults",
			content: `
agent:
  id: "minimal-agent"
master:
  address: "localhost:9001"
`,
			wantErr: false,
			checkFunc: func(t *testing.T, c *AgentConfig) {
				if c.Agent.ID != "minimal-agent" {
					t.Errorf("unexpected agent id: %s", c.Agent.ID)
				}
				// Defaults should be preserved
				if c.Master.Reconnect.HeartbeatInterval != 10*time.Second {
					t.Errorf("default heartbeat interval not preserved: %v", c.Master.Reconnect.HeartbeatInterval)
				}
			},
		},
		{
			name:    "invalid yaml",
			content: `invalid: [yaml: broken`,
			wantErr: true,
		},
		{
			name:    "empty file",
			content: "",
			wantErr: false, // Empty file should use defaults
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create temp file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "agent.yaml")
			if err := os.WriteFile(configPath, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("failed to write config file: %v", err)
			}

			config, err := LoadAgentConfig(configPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadAgentConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.checkFunc != nil {
				tt.checkFunc(t, config)
			}
		})
	}
}

func TestLoadAgentConfig_FileNotFound(t *testing.T) {
	t.Parallel()

	_, err := LoadAgentConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestSaveAgentConfig(t *testing.T) {
	t.Parallel()

	config := DefaultAgentConfig()
	config.Agent.ID = "save-test-agent"
	config.Master.Address = "localhost:9001"

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "subdir", "agent.yaml")

	// Save config
	if err := SaveAgentConfig(config, configPath); err != nil {
		t.Fatalf("SaveAgentConfig failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created")
	}

	// Load and verify
	loaded, err := LoadAgentConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load saved config: %v", err)
	}

	if loaded.Agent.ID != config.Agent.ID {
		t.Errorf("agent id mismatch: got %s, want %s", loaded.Agent.ID, config.Agent.ID)
	}
	if loaded.Master.Address != config.Master.Address {
		t.Errorf("master address mismatch: got %s, want %s", loaded.Master.Address, config.Master.Address)
	}
}

func TestAgentConfigValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  *AgentConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &AgentConfig{
				Agent: AgentIdentityConfig{
					ID: "test-agent",
				},
				Master: AgentMasterConfig{
					Address: "localhost:9001",
				},
				Paths: AgentPathsConfig{
					Repos:    "/tmp/repos",
					Releases: "/tmp/releases",
				},
				Execution: ExecutionConfig{
					Timeout: 5 * time.Minute,
				},
			},
			wantErr: false,
		},
		{
			name: "missing agent id",
			config: &AgentConfig{
				Agent: AgentIdentityConfig{
					ID: "",
				},
				Master: AgentMasterConfig{
					Address: "localhost:9001",
				},
				Paths: AgentPathsConfig{
					Repos:    "/tmp/repos",
					Releases: "/tmp/releases",
				},
				Execution: ExecutionConfig{
					Timeout: 5 * time.Minute,
				},
			},
			wantErr: true,
		},
		{
			name: "missing master address and token",
			config: &AgentConfig{
				Agent: AgentIdentityConfig{
					ID: "test-agent",
				},
				Master: AgentMasterConfig{},
				Paths: AgentPathsConfig{
					Repos:    "/tmp/repos",
					Releases: "/tmp/releases",
				},
				Execution: ExecutionConfig{
					Timeout: 5 * time.Minute,
				},
			},
			wantErr: true,
		},
		{
			name: "valid with token only",
			config: &AgentConfig{
				Agent: AgentIdentityConfig{
					ID: "test-agent",
				},
				Master: AgentMasterConfig{
					Token: "some-token",
				},
				Paths: AgentPathsConfig{
					Repos:    "/tmp/repos",
					Releases: "/tmp/releases",
				},
				Execution: ExecutionConfig{
					Timeout: 5 * time.Minute,
				},
			},
			wantErr: false,
		},
		{
			name: "missing repos path",
			config: &AgentConfig{
				Agent: AgentIdentityConfig{
					ID: "test-agent",
				},
				Master: AgentMasterConfig{
					Address: "localhost:9001",
				},
				Paths: AgentPathsConfig{
					Repos:    "",
					Releases: "/tmp/releases",
				},
				Execution: ExecutionConfig{
					Timeout: 5 * time.Minute,
				},
			},
			wantErr: true,
		},
		{
			name: "missing releases path",
			config: &AgentConfig{
				Agent: AgentIdentityConfig{
					ID: "test-agent",
				},
				Master: AgentMasterConfig{
					Address: "localhost:9001",
				},
				Paths: AgentPathsConfig{
					Repos:    "/tmp/repos",
					Releases: "",
				},
				Execution: ExecutionConfig{
					Timeout: 5 * time.Minute,
				},
			},
			wantErr: true,
		},
		{
			name: "timeout too short",
			config: &AgentConfig{
				Agent: AgentIdentityConfig{
					ID: "test-agent",
				},
				Master: AgentMasterConfig{
					Address: "localhost:9001",
				},
				Paths: AgentPathsConfig{
					Repos:    "/tmp/repos",
					Releases: "/tmp/releases",
				},
				Execution: ExecutionConfig{
					Timeout: 500 * time.Millisecond,
				},
			},
			wantErr: true,
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

func TestLoadAgentConfig_ReadError(t *testing.T) {
	t.Parallel()

	// Try to read a directory as a file (should fail)
	tmpDir := t.TempDir()
	_, err := LoadAgentConfig(tmpDir)
	if err == nil {
		t.Error("expected error when reading directory as file")
	}
}

func TestSaveAgentConfig_MarshalAndWrite(t *testing.T) {
	t.Parallel()

	config := DefaultAgentConfig()
	config.Agent.ID = "marshal-test-agent"
	config.Master.Address = "localhost:9001"
	config.Agent.Labels = map[string]string{
		"env":    "test",
		"region": "us-east-1",
	}

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "deep", "nested", "dir", "agent.yaml")

	// Save should create directories
	if err := SaveAgentConfig(config, configPath); err != nil {
		t.Fatalf("SaveAgentConfig failed: %v", err)
	}

	// Verify file permissions
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("failed to stat config file: %v", err)
	}
	// File should be 0600 (owner read/write only)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("unexpected file permissions: %v, want 0600", info.Mode().Perm())
	}
}

func TestSaveAgentConfig_DirectoryCreationError(t *testing.T) {
	t.Parallel()

	config := DefaultAgentConfig()
	config.Agent.ID = "test"
	config.Master.Address = "localhost:9001"

	// Try to save to a path where we can't create directory
	// /dev/null is a device file, can't create subdirectories under it
	invalidPath := "/dev/null/cannot/create/agent.yaml"

	err := SaveAgentConfig(config, invalidPath)
	if err == nil {
		t.Error("expected error when creating directory under /dev/null")
	}
}

func TestAgentConfigValidateErrorMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		config         *AgentConfig
		expectedSubstr string
	}{
		{
			name: "missing agent.id message",
			config: &AgentConfig{
				Agent: AgentIdentityConfig{ID: ""},
			},
			expectedSubstr: "agent.id",
		},
		{
			name: "missing master address/token message",
			config: &AgentConfig{
				Agent:  AgentIdentityConfig{ID: "test"},
				Master: AgentMasterConfig{},
			},
			expectedSubstr: "master.address or master.token",
		},
		{
			name: "missing repos path message",
			config: &AgentConfig{
				Agent:  AgentIdentityConfig{ID: "test"},
				Master: AgentMasterConfig{Address: "localhost:9001"},
				Paths:  AgentPathsConfig{Repos: ""},
			},
			expectedSubstr: "paths.repos",
		},
		{
			name: "missing releases path message",
			config: &AgentConfig{
				Agent:  AgentIdentityConfig{ID: "test"},
				Master: AgentMasterConfig{Address: "localhost:9001"},
				Paths:  AgentPathsConfig{Repos: "/tmp/repos", Releases: ""},
			},
			expectedSubstr: "paths.releases",
		},
		{
			name: "timeout too short message",
			config: &AgentConfig{
				Agent:     AgentIdentityConfig{ID: "test"},
				Master:    AgentMasterConfig{Address: "localhost:9001"},
				Paths:     AgentPathsConfig{Repos: "/tmp/repos", Releases: "/tmp/releases"},
				Execution: ExecutionConfig{Timeout: 100 * time.Millisecond},
			},
			expectedSubstr: "execution.timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.config.Validate()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.expectedSubstr) {
				t.Errorf("error message should contain '%s', got: %s", tt.expectedSubstr, err.Error())
			}
		})
	}
}

func TestDefaultAgentConfigLabels(t *testing.T) {
	t.Parallel()

	config := DefaultAgentConfig()

	// Labels should be initialized as empty map, not nil
	if config.Agent.Labels == nil {
		t.Error("expected Labels to be initialized, got nil")
	}

	// Should be able to add labels
	config.Agent.Labels["key"] = "value"
	if config.Agent.Labels["key"] != "value" {
		t.Error("failed to set label")
	}
}

func TestLoadAgentConfigWithAllFields(t *testing.T) {
	t.Parallel()

	content := `
master:
  address: "master.example.com:9001"
  token: "secret-token"
  ca_cert: "/custom/ca.pem"
  reconnect:
    initial_delay: 5s
    max_delay: 30m
    heartbeat_interval: 1m
agent:
  id: "full-test-agent"
  labels:
    env: production
    team: platform
    region: eu-west-1
paths:
  repos: /data/repos
  releases: /data/releases
execution:
  user: appuser
  group: appgroup
  timeout: 30m
  use_namespaces: false
  allowed_env_vars:
    - PATH
    - HOME
    - CUSTOM_VAR
health:
  disk_warning_threshold: 85
  report_interval: 1m
graceful_shutdown:
  drain_timeout: 5m
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "agent.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	config, err := LoadAgentConfig(configPath)
	if err != nil {
		t.Fatalf("LoadAgentConfig() error = %v", err)
	}

	// Verify all fields
	if config.Master.Address != "master.example.com:9001" {
		t.Errorf("unexpected Master.Address: %s", config.Master.Address)
	}
	if config.Master.Token != "secret-token" {
		t.Errorf("unexpected Master.Token: %s", config.Master.Token)
	}
	if config.Master.CACert != "/custom/ca.pem" {
		t.Errorf("unexpected Master.CACert: %s", config.Master.CACert)
	}
	if config.Master.Reconnect.InitialDelay != 5*time.Second {
		t.Errorf("unexpected InitialDelay: %v", config.Master.Reconnect.InitialDelay)
	}
	if len(config.Agent.Labels) != 3 {
		t.Errorf("expected 3 labels, got %d", len(config.Agent.Labels))
	}
	if config.Execution.User != "appuser" {
		t.Errorf("unexpected Execution.User: %s", config.Execution.User)
	}
	if config.Execution.UseNamespaces {
		t.Error("expected UseNamespaces to be false")
	}
	if len(config.Execution.AllowedEnvVars) != 3 {
		t.Errorf("expected 3 allowed env vars, got %d", len(config.Execution.AllowedEnvVars))
	}
	if config.Health.DiskWarningThreshold != 85 {
		t.Errorf("unexpected DiskWarningThreshold: %d", config.Health.DiskWarningThreshold)
	}
	if config.GracefulShutdown.DrainTimeout != 5*time.Minute {
		t.Errorf("unexpected DrainTimeout: %v", config.GracefulShutdown.DrainTimeout)
	}
}
