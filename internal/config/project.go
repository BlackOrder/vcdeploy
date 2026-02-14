// Package config defines configuration structures for vcdeploy.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ProjectConfig defines a deployment project.
type ProjectConfig struct {
	Name          string                  `yaml:"name"`
	Type          string                  `yaml:"type"`
	Repository    string                  `yaml:"repository"`
	Archived      bool                    `yaml:"archived"`
	Watch         WatchConfig             `yaml:"watch"`
	Deployment    DeploymentConfig        `yaml:"deployment"`
	Env           EnvConfig               `yaml:"env"`
	Targets       map[string]TargetConfig `yaml:"targets"`
	Hooks         HooksConfig             `yaml:"hooks"`
	Health        HealthConfig            `yaml:"health"`
	Notifications ProjectNotifications    `yaml:"notifications"`
}

// WatchConfig defines branch watching settings.
type WatchConfig struct {
	Branches []string     `yaml:"branches"`
	Actions  []string     `yaml:"actions"`
	Guards   GuardsConfig `yaml:"guards"`
}

// GuardsConfig defines deployment guards.
type GuardsConfig struct {
	RejectForcePush *bool `yaml:"reject_force_push"`
	RequireCIPass   bool  `yaml:"require_ci_pass"`
}

// DeploymentConfig defines deployment behavior.
type DeploymentConfig struct {
	OnBusy       string `yaml:"on_busy"`  // cancel | queue | skip
	Strategy     string `yaml:"strategy"` // symlink | inplace
	KeepReleases int    `yaml:"keep_releases"`
}

// EnvConfig defines environment file settings.
type EnvConfig struct {
	Template           string   `yaml:"template"`
	PlaceholderPattern string   `yaml:"placeholder_pattern"`
	RequiredKeys       []string `yaml:"required_keys"`
}

// TargetConfig defines a deployment target.
type TargetConfig struct {
	// Agent-based deployment (mutually exclusive with SSH)
	Agent  string   `yaml:"agent,omitempty"`
	Agents []string `yaml:"agents,omitempty"`

	// SSH-based deployment (mutually exclusive with Agent)
	SSH *SSHTargetConfig `yaml:"ssh,omitempty"`

	// Common settings
	Branch         string `yaml:"branch"`
	Path           string `yaml:"path"`
	DeployStrategy string `yaml:"deploy_strategy,omitempty"` // parallel | rolling
}

// SSHTargetConfig defines SSH connection settings for a target.
type SSHTargetConfig struct {
	Host     string `yaml:"host"`
	User     string `yaml:"user,omitempty"`
	Key      string `yaml:"key,omitempty"`
	Jump     string `yaml:"jump,omitempty"`      // Reference to jump_servers in master config
	JumpHost string `yaml:"jump_host,omitempty"` // Inline jump host
	JumpUser string `yaml:"jump_user,omitempty"`
	JumpKey  string `yaml:"jump_key,omitempty"`
}

// HooksConfig defines deployment hooks.
type HooksConfig struct {
	PreDeploy  []string        `yaml:"pre_deploy,omitempty"`
	PostDeploy []string        `yaml:"post_deploy,omitempty"`
	Reload     []ServiceAction `yaml:"reload,omitempty"`
	Rollback   []string        `yaml:"rollback,omitempty"`
}

// ServiceAction defines a service management action.
type ServiceAction struct {
	Service string `yaml:"service"`
	Action  string `yaml:"action"` // reload | restart | start | stop
}

// HealthConfig defines health check settings.
type HealthConfig struct {
	URL            string `yaml:"url"`
	Timeout        string `yaml:"timeout"`
	Retries        int    `yaml:"retries"`
	RollbackOnFail bool   `yaml:"rollback_on_fail"`
}

// ProjectNotifications defines per-project notification settings.
type ProjectNotifications struct {
	OnSuccess []NotificationTarget `yaml:"on_success,omitempty"`
	OnFailure []NotificationTarget `yaml:"on_failure,omitempty"`
}

// NotificationTarget defines a notification destination.
type NotificationTarget struct {
	Slack   string `yaml:"slack,omitempty"`
	Email   string `yaml:"email,omitempty"`
	Webhook string `yaml:"webhook,omitempty"`
}

// LoadProjectConfig loads a project configuration from a file.
func LoadProjectConfig(path string) (*ProjectConfig, error) {
	data, err := os.ReadFile(path) // #nosec G304 - path is admin-controlled config file location
	if err != nil {
		return nil, fmt.Errorf("reading project file: %w", err)
	}

	var config ProjectConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing project file: %w", err)
	}

	// Apply defaults
	if config.Deployment.OnBusy == "" {
		config.Deployment.OnBusy = "cancel"
	}
	if config.Deployment.Strategy == "" {
		config.Deployment.Strategy = "symlink"
	}
	if config.Deployment.KeepReleases == 0 {
		config.Deployment.KeepReleases = DefaultKeepReleases
	}
	if config.Env.PlaceholderPattern == "" {
		config.Env.PlaceholderPattern = "${SECRET_NAME}"
	}
	if config.Watch.Guards.RejectForcePush == nil {
		defaultTrue := true
		config.Watch.Guards.RejectForcePush = &defaultTrue // Default to true for security
	}

	return &config, nil
}

// SaveProjectConfig saves a project configuration to a file.
func SaveProjectConfig(config *ProjectConfig, path string) error {
	dir := filepath.Dir(path)
	// #nosec G301 - Project directory needs group access for agents
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating project directory: %w", err)
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshaling project: %w", err)
	}

	// #nosec G306 - Project config needs to be readable by agents
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing project file: %w", err)
	}

	return nil
}

// Validate validates the project configuration.
func (c *ProjectConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("name is required")
	}
	if c.Repository == "" {
		return fmt.Errorf("repository is required")
	}
	if len(c.Targets) == 0 {
		return fmt.Errorf("at least one target is required")
	}

	for name, target := range c.Targets {
		if err := validateTarget(name, &target); err != nil {
			return err
		}
	}

	switch c.Deployment.OnBusy {
	case "cancel", "queue", "skip":
		// Valid
	default:
		return fmt.Errorf("deployment.on_busy must be cancel, queue, or skip")
	}

	switch c.Deployment.Strategy {
	case "symlink", "inplace":
		// Valid
	default:
		return fmt.Errorf("deployment.strategy must be symlink or inplace")
	}

	return nil
}

func validateTarget(name string, target *TargetConfig) error {
	hasAgent := target.Agent != "" || len(target.Agents) > 0
	hasSSH := target.SSH != nil

	if !hasAgent && !hasSSH {
		return fmt.Errorf("target %s: must specify agent, agents, or ssh", name)
	}
	if hasAgent && hasSSH {
		return fmt.Errorf("target %s: cannot specify both agent and ssh", name)
	}

	if target.Branch == "" {
		return fmt.Errorf("target %s: branch is required", name)
	}
	if target.Path == "" {
		return fmt.Errorf("target %s: path is required", name)
	}

	if hasSSH {
		if target.SSH.Host == "" {
			return fmt.Errorf("target %s: ssh.host is required", name)
		}
	}

	return nil
}

// GetAgents returns the list of agents for a target.
// Returns a single-element slice if Agent is set, or the Agents slice.
func (t *TargetConfig) GetAgents() []string {
	if t.Agent != "" {
		return []string{t.Agent}
	}
	return t.Agents
}

// IsSSH returns true if this target uses SSH instead of an agent.
func (t *TargetConfig) IsSSH() bool {
	return t.SSH != nil
}
