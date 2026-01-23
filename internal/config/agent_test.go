package config

import (
	"os"
	"path/filepath"
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
	if config.Master.Cert != "/etc/vcdeploy/agent/cert.pem" {
		t.Errorf("unexpected default cert path: %s", config.Master.Cert)
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
  address: "localhost:9090"
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
				if c.Master.Address != "localhost:9090" {
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
  address: "localhost:9090"
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
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create temp file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "agent.yaml")
			if err := os.WriteFile(configPath, []byte(tt.content), 0644); err != nil {
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
	config.Master.Address = "localhost:9090"

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
					Address: "localhost:9090",
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
					Address: "localhost:9090",
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
					Address: "localhost:9090",
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
					Address: "localhost:9090",
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
					Address: "localhost:9090",
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
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
