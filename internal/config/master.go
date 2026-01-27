// Package config defines configuration structures for vcdeploy.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// MasterConfig is the main configuration for the vcdeploy master.
type MasterConfig struct {
	Server        ServerConfig        `yaml:"server"`
	GRPC          GRPCConfig          `yaml:"grpc"`
	SSH           SSHConfig           `yaml:"ssh"`
	Security      SecurityConfig      `yaml:"security"`
	Backup        BackupConfig        `yaml:"backup"`
	Logs          LogsConfig          `yaml:"logs"`
	Webhooks      WebhooksConfig      `yaml:"webhooks"`
	Notifications NotificationsConfig `yaml:"notifications"`
	API           APIConfig           `yaml:"api"`
	Appearance    AppearanceConfig    `yaml:"appearance"`
}

// ServerConfig defines HTTP server settings.
type ServerConfig struct {
	Listen string    `yaml:"listen"`
	TLS    TLSConfig `yaml:"tls"`
}

// TLSConfig defines TLS settings.
type TLSConfig struct {
	Enabled bool   `yaml:"enabled"`
	Cert    string `yaml:"cert"`
	Key     string `yaml:"key"`
}

// GRPCConfig defines gRPC server settings for agent connections.
type GRPCConfig struct {
	Listen string `yaml:"listen"`
}

// SSHConfig defines SSH connection settings.
type SSHConfig struct {
	DefaultUser       string             `yaml:"default_user"`
	DefaultKey        string             `yaml:"default_key"`
	KnownHosts        string             `yaml:"known_hosts"`
	ConnectionTimeout time.Duration      `yaml:"connection_timeout"`
	KeepaliveInterval time.Duration      `yaml:"keepalive_interval"`
	IdleTimeout       time.Duration      `yaml:"idle_timeout"`
	JumpServers       []JumpServerConfig `yaml:"jump_servers"`
}

// JumpServerConfig defines a jump/bastion server.
type JumpServerConfig struct {
	Name string `yaml:"name"`
	Host string `yaml:"host"`
	User string `yaml:"user"`
	Key  string `yaml:"key"`
}

// SecurityConfig defines security settings.
type SecurityConfig struct {
	KeyRotation     KeyRotationConfig `yaml:"key_rotation"`
	SessionTimeout  time.Duration     `yaml:"session_timeout"`
	Require2FAAdmin bool              `yaml:"require_2fa_admin"`
}

// KeyRotationConfig defines master key rotation settings.
type KeyRotationConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Interval time.Duration `yaml:"interval"`
}

// BackupConfig defines backup settings.
type BackupConfig struct {
	Database DatabaseBackupConfig `yaml:"database"`
	Config   ConfigBackupConfig   `yaml:"config"`
}

// DatabaseBackupConfig defines database backup settings.
type DatabaseBackupConfig struct {
	Enabled   bool          `yaml:"enabled"`
	Interval  time.Duration `yaml:"interval"`
	Retention time.Duration `yaml:"retention"`
	Path      string        `yaml:"path"`
}

// ConfigBackupConfig defines config file backup settings.
type ConfigBackupConfig struct {
	Versions int `yaml:"versions"`
}

// LogsConfig defines logging settings.
type LogsConfig struct {
	Deployment  DeploymentLogsConfig  `yaml:"deployment"`
	Audit       AuditLogsConfig       `yaml:"audit"`
	Application ApplicationLogsConfig `yaml:"application"`
	Rotation    LogRotationConfig     `yaml:"rotation"`
}

// DeploymentLogsConfig defines deployment log settings.
type DeploymentLogsConfig struct {
	Retention time.Duration `yaml:"retention"`
	MaxSizeMB int           `yaml:"max_size_mb"`
}

// AuditLogsConfig defines audit log settings.
type AuditLogsConfig struct {
	Retention time.Duration     `yaml:"retention"`
	Export    AuditExportConfig `yaml:"export"`
}

// AuditExportConfig defines audit log export settings.
type AuditExportConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Destination string `yaml:"destination"`
	Schedule    string `yaml:"schedule"`
}

// ApplicationLogsConfig defines application log settings.
type ApplicationLogsConfig struct {
	Level     string        `yaml:"level"`
	Retention time.Duration `yaml:"retention"`
}

// LogRotationConfig defines log rotation settings.
type LogRotationConfig struct {
	Schedule string `yaml:"schedule"`
}

// WebhooksConfig defines webhook settings.
type WebhooksConfig struct {
	GitHub    WebhookProviderConfig `yaml:"github"`
	GitLab    WebhookProviderConfig `yaml:"gitlab"`
	Bitbucket WebhookProviderConfig `yaml:"bitbucket"`
}

// WebhookProviderConfig defines settings for a webhook provider.
type WebhookProviderConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

// NotificationsConfig defines notification settings.
type NotificationsConfig struct {
	Providers NotificationProvidersConfig `yaml:"providers"`
}

// NotificationProvidersConfig defines notification provider settings.
type NotificationProvidersConfig struct {
	Slack   SlackNotificationConfig   `yaml:"slack"`
	Email   EmailNotificationConfig   `yaml:"email"`
	Webhook WebhookNotificationConfig `yaml:"webhook"`
	Discord DiscordNotificationConfig `yaml:"discord"`
}

// SlackNotificationConfig defines Slack notification settings.
type SlackNotificationConfig struct {
	Enabled bool `yaml:"enabled"`
}

// EmailNotificationConfig defines email notification settings.
type EmailNotificationConfig struct {
	Enabled bool       `yaml:"enabled"`
	SMTP    SMTPConfig `yaml:"smtp"`
}

// SMTPConfig defines SMTP settings.
type SMTPConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	User string `yaml:"user"`
}

// WebhookNotificationConfig defines generic webhook notification settings.
type WebhookNotificationConfig struct {
	Enabled bool `yaml:"enabled"`
}

// DiscordNotificationConfig defines Discord notification settings.
type DiscordNotificationConfig struct {
	Enabled    bool   `yaml:"enabled"`
	WebhookURL string `yaml:"webhook_url"`
	Username   string `yaml:"username"`
	AvatarURL  string `yaml:"avatar_url"`
}

// APIConfig defines API settings.
type APIConfig struct {
	Enabled bool `yaml:"enabled"`
}

// AppearanceConfig defines UI appearance settings.
type AppearanceConfig struct {
	Theme string `yaml:"theme"`
}

// DefaultMasterConfig returns a MasterConfig with default values.
func DefaultMasterConfig() *MasterConfig {
	return &MasterConfig{
		Server: ServerConfig{
			Listen: ":9000",
			TLS: TLSConfig{
				Enabled: true,
				Cert:    "/etc/vcdeploy/tls/cert.pem",
				Key:     "/etc/vcdeploy/tls/key.pem",
			},
		},
		GRPC: GRPCConfig{
			Listen: ":9001",
		},
		SSH: SSHConfig{
			DefaultUser:       "deploy",
			DefaultKey:        "/etc/vcdeploy/keys/default.pem",
			KnownHosts:        "/etc/vcdeploy/known_hosts",
			ConnectionTimeout: 30 * time.Second,
			KeepaliveInterval: 15 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
		Security: SecurityConfig{
			KeyRotation: KeyRotationConfig{
				Enabled:  true,
				Interval: 720 * time.Hour, // 30 days
			},
			SessionTimeout:  24 * time.Hour,
			Require2FAAdmin: true,
		},
		Backup: BackupConfig{
			Database: DatabaseBackupConfig{
				Enabled:   true,
				Interval:  720 * time.Hour,  // 30 days
				Retention: 8760 * time.Hour, // 365 days
				Path:      "/var/lib/vcdeploy/backups/",
			},
			Config: ConfigBackupConfig{
				Versions: 5,
			},
		},
		Logs: LogsConfig{
			Deployment: DeploymentLogsConfig{
				Retention: 2160 * time.Hour, // 90 days
				MaxSizeMB: 100,
			},
			Audit: AuditLogsConfig{
				Retention: 8760 * time.Hour, // 365 days
				Export: AuditExportConfig{
					Enabled:  false,
					Schedule: "0 0 1 * *",
				},
			},
			Application: ApplicationLogsConfig{
				Level:     "info",
				Retention: 720 * time.Hour, // 30 days
			},
			Rotation: LogRotationConfig{
				Schedule: "0 3 * * *",
			},
		},
		Webhooks: WebhooksConfig{
			GitHub:    WebhookProviderConfig{Enabled: true, Path: "/webhook/github"},
			GitLab:    WebhookProviderConfig{Enabled: true, Path: "/webhook/gitlab"},
			Bitbucket: WebhookProviderConfig{Enabled: true, Path: "/webhook/bitbucket"},
		},
		Notifications: NotificationsConfig{
			Providers: NotificationProvidersConfig{
				Slack:   SlackNotificationConfig{Enabled: true},
				Email:   EmailNotificationConfig{Enabled: true},
				Webhook: WebhookNotificationConfig{Enabled: true},
			},
		},
		API: APIConfig{
			Enabled: true,
		},
		Appearance: AppearanceConfig{
			Theme: "dark",
		},
	}
}

// LoadMasterConfig loads the master configuration from a file.
func LoadMasterConfig(path string) (*MasterConfig, error) {
	config := DefaultMasterConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return config, nil // Use defaults if no config file
		}
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	return config, nil
}

// LoadMaster is an alias for LoadMasterConfig
func LoadMaster(path string) (*MasterConfig, error) {
	return LoadMasterConfig(path)
}

// SaveMasterConfig saves the master configuration to a file.
func SaveMasterConfig(config *MasterConfig, path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}

// Validate validates the master configuration.
func (c *MasterConfig) Validate() error {
	if c.Server.Listen == "" {
		return fmt.Errorf("server.listen is required")
	}
	if c.GRPC.Listen == "" {
		return fmt.Errorf("grpc.listen is required")
	}
	if c.Security.KeyRotation.Enabled && c.Security.KeyRotation.Interval < time.Hour {
		return fmt.Errorf("key_rotation.interval must be at least 1 hour")
	}
	if c.Backup.Config.Versions < 1 {
		return fmt.Errorf("backup.config.versions must be at least 1")
	}
	return nil
}
