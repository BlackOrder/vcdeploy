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
	Storage       StorageConfig       `yaml:"storage"`
	Tracing       TracingConfig       `yaml:"tracing"`
	Alerting      AlertingConfig      `yaml:"alerting"`
	Webhooks      WebhooksConfig      `yaml:"webhooks"`
	Notifications NotificationsConfig `yaml:"notifications"`
	API           APIConfig           `yaml:"api"`
	Appearance    AppearanceConfig    `yaml:"appearance"`
}

// TLSMode defines the TLS operation mode.
type TLSMode string

const (
	// TLSModeDisabled serves HTTP only (development, behind proxy).
	TLSModeDisabled TLSMode = "disabled"
	// TLSModeStatic uses certificate files provided by user.
	TLSModeStatic TLSMode = "static"
	// TLSModeACME uses automatic certificate management (Let's Encrypt).
	TLSModeACME TLSMode = "acme"
)

// ServerConfig defines HTTP server settings.
type ServerConfig struct {
	Listen       string    `yaml:"listen"`        // HTTP address (default: ":80")
	HTTPSAddress string    `yaml:"https_address"` // HTTPS address (default: ":443")
	SocketPath   string    `yaml:"socket_path"`   // Unix socket path for local CLI access (default: /var/run/vcdeploy/vcdeploy.sock)
	TLS          TLSConfig `yaml:"tls"`
}

// TLSConfig defines TLS settings.
type TLSConfig struct {
	// Mode determines TLS behavior: disabled, static, or acme
	Mode TLSMode `yaml:"mode"`

	// Static mode: paths to certificate and key files
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`

	// ACME mode configuration
	ACME ACMEConfig `yaml:"acme"`

	// ForceHTTPS redirects HTTP to HTTPS when TLS enabled
	ForceHTTPS bool `yaml:"force_https"`

	// MinVersion is minimum TLS version ("1.2" or "1.3")
	MinVersion string `yaml:"min_version"`

	// Legacy fields for backward compatibility during migration
	Enabled bool   `yaml:"enabled"` // Deprecated: use Mode instead
	Cert    string `yaml:"cert"`    // Deprecated: use CertFile instead
	Key     string `yaml:"key"`     // Deprecated: use KeyFile instead
}

// ACMEConfig defines ACME (Let's Encrypt) configuration.
type ACMEConfig struct {
	Email    string   `yaml:"email"`     // Contact email for Let's Encrypt
	Domains  []string `yaml:"domains"`   // Domains to obtain certs for
	Staging  bool     `yaml:"staging"`   // Use Let's Encrypt staging (for testing)
	CacheDir string   `yaml:"cache_dir"` // Directory to cache certificates
}

// GRPCConfig defines gRPC server settings for agent connections.
type GRPCConfig struct {
	Listen        string `yaml:"listen"`
	ReauthAddress string `yaml:"reauth_address"` // Dedicated port for unauthenticated re-authentication
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
//
//nolint:revive // Keeping explicit naming for clarity
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

// TracingConfig defines OpenTelemetry tracing settings.
type TracingConfig struct {
	Enabled     bool    `yaml:"enabled"`
	Endpoint    string  `yaml:"endpoint"`
	ServiceName string  `yaml:"service_name"`
	SampleRate  float64 `yaml:"sample_rate"`
	Insecure    bool    `yaml:"insecure"`
}

// AlertingConfig defines system alerting thresholds.
type AlertingConfig struct {
	Enabled              bool          `yaml:"enabled"`
	DiskWarningPercent   float64       `yaml:"disk_warning_percent"`
	DiskCriticalPercent  float64       `yaml:"disk_critical_percent"`
	MemoryWarningPercent float64       `yaml:"memory_warning_percent"`
	CPUWarningPercent    float64       `yaml:"cpu_warning_percent"`
	DeploymentTimeout    time.Duration `yaml:"deployment_timeout"`
	AlertCooldown        time.Duration `yaml:"alert_cooldown"`
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
	Enabled    bool   `yaml:"enabled"`
	WebhookURL string `yaml:"webhook_url"`
	Channel    string `yaml:"channel"`
	Username   string `yaml:"username"`
	IconEmoji  string `yaml:"icon_emoji"`
}

// EmailNotificationConfig defines email notification settings.
type EmailNotificationConfig struct {
	Enabled bool       `yaml:"enabled"`
	SMTP    SMTPConfig `yaml:"smtp"`
}

// SMTPConfig defines SMTP settings.
type SMTPConfig struct {
	Host        string   `yaml:"host"`
	Port        int      `yaml:"port"`
	User        string   `yaml:"user"`
	Password    string   `yaml:"password"`
	TLS         bool     `yaml:"tls"`
	FromAddress string   `yaml:"from_address"`
	FromName    string   `yaml:"from_name"`
	ToAddresses []string `yaml:"to_addresses"`
}

// WebhookNotificationConfig defines generic webhook notification settings.
type WebhookNotificationConfig struct {
	Enabled bool              `yaml:"enabled"`
	URL     string            `yaml:"url"`
	Method  string            `yaml:"method"`
	Headers map[string]string `yaml:"headers"`
	Secret  string            `yaml:"secret"`
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
	Enabled   bool            `yaml:"enabled"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
}

// RateLimitConfig defines rate limiting settings.
type RateLimitConfig struct {
	Enabled           bool    `yaml:"enabled"`
	RequestsPerSecond float64 `yaml:"requests_per_second"`
	BurstSize         int     `yaml:"burst_size"`
}

// AppearanceConfig defines UI appearance settings.
type AppearanceConfig struct {
	Theme string `yaml:"theme"`
}

// StorageConfig defines storage settings.
type StorageConfig struct {
	// UseMemoryCache enables the in-memory cache layer with batched SQLite persistence.
	// When enabled, all reads are served from memory and writes are batched,
	// eliminating SQLITE_BUSY errors from concurrent access.
	// Default: true
	UseMemoryCache bool `yaml:"use_memory_cache"`
}

// DefaultMasterConfig returns a MasterConfig with default values.
func DefaultMasterConfig() *MasterConfig {
	return &MasterConfig{
		Server: ServerConfig{
			Listen:     ":9000",
			SocketPath: "/var/run/vcdeploy/vcdeploy.sock",
			TLS: TLSConfig{
				Mode:       TLSModeDisabled,
				CertFile:   "/etc/vcdeploy/certs/server.crt",
				KeyFile:    "/etc/vcdeploy/certs/server.key",
				Enabled:    true, // Deprecated: for backward compatibility
				ForceHTTPS: true,
				MinVersion: "1.2",
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
		Storage: StorageConfig{
			UseMemoryCache: true,
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
			RateLimit: RateLimitConfig{
				Enabled:           true,
				RequestsPerSecond: 10,
				BurstSize:         20,
			},
		},
		Appearance: AppearanceConfig{
			Theme: "dark",
		},
	}
}

// LoadMasterConfig loads the master configuration from a file.
func LoadMasterConfig(path string) (*MasterConfig, error) {
	config := DefaultMasterConfig()

	data, err := os.ReadFile(path) // #nosec G304 - path is admin-controlled config file location
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
	// #nosec G301 - Config directory needs group access for service user
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	// #nosec G306 - Master config needs to be readable by service user
	if err := os.WriteFile(path, data, 0o644); err != nil {
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

	// TLS file validation for static mode
	if c.Server.TLS.Mode == TLSModeStatic {
		if c.Server.TLS.CertFile == "" {
			return fmt.Errorf("tls.cert_file is required when tls.mode is 'static'")
		}
		if c.Server.TLS.KeyFile == "" {
			return fmt.Errorf("tls.key_file is required when tls.mode is 'static'")
		}
		if _, err := os.Stat(c.Server.TLS.CertFile); err != nil {
			return fmt.Errorf("TLS cert file not found: %s", c.Server.TLS.CertFile)
		}
		if _, err := os.Stat(c.Server.TLS.KeyFile); err != nil {
			return fmt.Errorf("TLS key file not found: %s", c.Server.TLS.KeyFile)
		}
	}

	// ACME mode validation
	if c.Server.TLS.Mode == TLSModeACME {
		if c.Server.TLS.ACME.Email == "" {
			return fmt.Errorf("tls.acme.email is required when tls.mode is 'acme'")
		}
		if len(c.Server.TLS.ACME.Domains) == 0 {
			return fmt.Errorf("tls.acme.domains is required when tls.mode is 'acme'")
		}
	}

	return nil
}
