// Package config defines configuration structures for vcdeploy.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// Default system paths
const (
	DefaultSystemConfigPath = "/etc/vcdeploy/vcdeploy.yaml"
	DefaultDataDir          = "/var/lib/vcdeploy"
	DefaultRunDir           = "/var/run/vcdeploy"
	DefaultLogDir           = "/var/log/vcdeploy"
	DefaultConfigDir        = "/etc/vcdeploy"
)

// SystemPaths holds configurable system paths.
type SystemPaths struct {
	ConfigDir string `yaml:"config_dir"`
	DataDir   string `yaml:"data_dir"`
	RunDir    string `yaml:"run_dir"`
	LogDir    string `yaml:"log_dir"`
}

// SystemConfig holds system-wide configuration including paths.
type SystemConfig struct {
	Paths SystemPaths `yaml:"paths"`
}

var (
	systemConfig     *SystemConfig
	systemConfigOnce sync.Once
	systemConfigErr  error
)

// GetSystemConfig returns the singleton system configuration.
// It loads from /etc/vcdeploy/vcdeploy.yaml if it exists, otherwise uses defaults.
func GetSystemConfig() (*SystemConfig, error) {
	systemConfigOnce.Do(func() {
		systemConfig = &SystemConfig{
			Paths: SystemPaths{
				ConfigDir: DefaultConfigDir,
				DataDir:   DefaultDataDir,
				RunDir:    DefaultRunDir,
				LogDir:    DefaultLogDir,
			},
		}

		// Try to load from config file
		if _, err := os.Stat(DefaultSystemConfigPath); err == nil {
			data, err := os.ReadFile(DefaultSystemConfigPath)
			if err != nil {
				systemConfigErr = err
				return
			}

			var loaded SystemConfig
			if err := yaml.Unmarshal(data, &loaded); err != nil {
				systemConfigErr = err
				return
			}

			// Merge loaded values (only override if set)
			if loaded.Paths.ConfigDir != "" {
				systemConfig.Paths.ConfigDir = loaded.Paths.ConfigDir
			}
			if loaded.Paths.DataDir != "" {
				systemConfig.Paths.DataDir = loaded.Paths.DataDir
			}
			if loaded.Paths.RunDir != "" {
				systemConfig.Paths.RunDir = loaded.Paths.RunDir
			}
			if loaded.Paths.LogDir != "" {
				systemConfig.Paths.LogDir = loaded.Paths.LogDir
			}
		}
		// If file doesn't exist, silently use defaults (backward compatibility)
	})

	return systemConfig, systemConfigErr
}

// MustGetSystemConfig returns the system config or panics on error.
// This should only be used during application startup where recovery is not possible.
// Prefer GetSystemConfig() with proper error handling in most cases.
func MustGetSystemConfig() *SystemConfig {
	cfg, err := GetSystemConfig()
	if err != nil {
		panic(fmt.Sprintf("failed to load system config: %v", err))
	}
	return cfg
}

// Convenience methods for common paths

// DatabasePath returns the path to the SQLite database.
func (c *SystemConfig) DatabasePath() string {
	return filepath.Join(c.Paths.DataDir, "vcdeploy.db")
}

// MasterConfigPath returns the path to the master config file.
func (c *SystemConfig) MasterConfigPath() string {
	return filepath.Join(c.Paths.ConfigDir, "master.yaml")
}

// AgentConfigPath returns the path to the agent config file.
func (c *SystemConfig) AgentConfigPath() string {
	return filepath.Join(c.Paths.ConfigDir, "agent.yaml")
}

// MasterPIDPath returns the path to the master PID file.
func (c *SystemConfig) MasterPIDPath() string {
	return filepath.Join(c.Paths.RunDir, "vcdeploy.pid")
}

// AgentPIDPath returns the path to the agent PID file.
func (c *SystemConfig) AgentPIDPath() string {
	return filepath.Join(c.Paths.RunDir, "vcdeploy-agent.pid")
}

// BackupsDir returns the path to the backups directory.
func (c *SystemConfig) BackupsDir() string {
	return filepath.Join(c.Paths.DataDir, "backups")
}

// SecretsBackupsDir returns the path to the secrets backups directory.
func (c *SystemConfig) SecretsBackupsDir() string {
	return filepath.Join(c.Paths.DataDir, "backups", "secrets")
}

// TemplatesDir returns the path to the templates directory.
func (c *SystemConfig) TemplatesDir() string {
	return filepath.Join(c.Paths.DataDir, "templates")
}

// StaticDir returns the path to the static files directory.
func (c *SystemConfig) StaticDir() string {
	return filepath.Join(c.Paths.DataDir, "static")
}

// MasterLogPath returns the path to the master log file.
func (c *SystemConfig) MasterLogPath() string {
	return filepath.Join(c.Paths.LogDir, "master.log")
}

// AgentLogPath returns the path to the agent log file.
func (c *SystemConfig) AgentLogPath() string {
	return filepath.Join(c.Paths.LogDir, "agent.log")
}

// SSHKeysDir returns the path to the SSH keys directory.
func (c *SystemConfig) SSHKeysDir() string {
	return filepath.Join(c.Paths.DataDir, "ssh_keys")
}

// CertsDir returns the path to the certificates directory.
func (c *SystemConfig) CertsDir() string {
	return filepath.Join(c.Paths.DataDir, "certs")
}

// EnsureDirectories creates all required directories with appropriate permissions.
func (c *SystemConfig) EnsureDirectories() error {
	dirs := []string{
		c.Paths.ConfigDir,
		c.Paths.DataDir,
		c.Paths.RunDir,
		c.Paths.LogDir,
		c.BackupsDir(),
		c.SecretsBackupsDir(),
		c.TemplatesDir(),
		c.StaticDir(),
		c.SSHKeysDir(),
		c.CertsDir(),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	return nil
}

// ResetSystemConfig resets the singleton for testing purposes.
func ResetSystemConfig() {
	systemConfigOnce = sync.Once{}
	systemConfig = nil
	systemConfigErr = nil
}
