// Package config defines configuration structures for vcdeploy.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// AgentConfig is the configuration for the vcdeploy agent.
type AgentConfig struct {
	Master           AgentMasterConfig      `yaml:"master"`
	Agent            AgentIdentityConfig    `yaml:"agent"`
	Paths            AgentPathsConfig       `yaml:"paths"`
	Execution        ExecutionConfig        `yaml:"execution"`
	Health           AgentHealthConfig      `yaml:"health"`
	GracefulShutdown GracefulShutdownConfig `yaml:"graceful_shutdown"`
}

// AgentMasterConfig defines master connection settings.
type AgentMasterConfig struct {
	Address       string          `yaml:"address"`
	Token         string          `yaml:"token"`
	Cert          string          `yaml:"cert"`
	AllowInsecure bool            `yaml:"allow_insecure"` // Allow unencrypted connection (NOT recommended)
	Reconnect     ReconnectConfig `yaml:"reconnect"`
}

// ReconnectConfig defines reconnection behavior.
type ReconnectConfig struct {
	InitialDelay      time.Duration `yaml:"initial_delay"`
	MaxDelay          time.Duration `yaml:"max_delay"`
	HeartbeatInterval time.Duration `yaml:"heartbeat_interval"`
}

// AgentIdentityConfig defines agent identity.
type AgentIdentityConfig struct {
	ID     string            `yaml:"id"`
	Labels map[string]string `yaml:"labels"`
}

// AgentPathsConfig defines local paths.
type AgentPathsConfig struct {
	Repos    string `yaml:"repos"`
	Releases string `yaml:"releases"`
}

// ExecutionConfig defines command execution settings.
type ExecutionConfig struct {
	User           string        `yaml:"user"`
	Group          string        `yaml:"group"`
	Timeout        time.Duration `yaml:"timeout"`
	UseNamespaces  bool          `yaml:"use_namespaces"`
	AllowedEnvVars []string      `yaml:"allowed_env_vars"`
}

// AgentHealthConfig defines health reporting settings.
type AgentHealthConfig struct {
	DiskWarningThreshold int           `yaml:"disk_warning_threshold"`
	ReportInterval       time.Duration `yaml:"report_interval"`
}

// GracefulShutdownConfig defines shutdown behavior.
type GracefulShutdownConfig struct {
	DrainTimeout time.Duration `yaml:"drain_timeout"`
}

// DefaultAgentConfig returns an AgentConfig with default values.
func DefaultAgentConfig() *AgentConfig {
	return &AgentConfig{
		Master: AgentMasterConfig{
			Cert: "/etc/vcdeploy/agent/cert.pem",
			Reconnect: ReconnectConfig{
				InitialDelay:      1 * time.Second,
				MaxDelay:          5 * time.Minute,
				HeartbeatInterval: 10 * time.Second,
			},
		},
		Agent: AgentIdentityConfig{
			Labels: make(map[string]string),
		},
		Paths: AgentPathsConfig{
			Repos:    "/var/lib/vcdeploy/repos/",
			Releases: "/var/www/",
		},
		Execution: ExecutionConfig{
			User:           "www-data",
			Group:          "www-data",
			Timeout:        10 * time.Minute,
			UseNamespaces:  true,
			AllowedEnvVars: []string{"PATH", "HOME", "USER", "LANG"},
		},
		Health: AgentHealthConfig{
			DiskWarningThreshold: 90,
			ReportInterval:       30 * time.Second,
		},
		GracefulShutdown: GracefulShutdownConfig{
			DrainTimeout: 10 * time.Minute,
		},
	}
}

// LoadAgentConfig loads the agent configuration from a file.
func LoadAgentConfig(path string) (*AgentConfig, error) {
	config := DefaultAgentConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found: %s", path)
		}
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	return config, nil
}

// SaveAgentConfig saves the agent configuration to a file.
func SaveAgentConfig(config *AgentConfig, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}

// Validate validates the agent configuration.
func (c *AgentConfig) Validate() error {
	if c.Agent.ID == "" {
		return fmt.Errorf("agent.id is required")
	}
	if c.Master.Address == "" && c.Master.Token == "" {
		return fmt.Errorf("master.address or master.token is required")
	}
	if c.Paths.Repos == "" {
		return fmt.Errorf("paths.repos is required")
	}
	if c.Paths.Releases == "" {
		return fmt.Errorf("paths.releases is required")
	}
	if c.Execution.Timeout < time.Second {
		return fmt.Errorf("execution.timeout must be at least 1 second")
	}
	return nil
}
