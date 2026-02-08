// Package config defines configuration structures for vcdeploy.
package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"gopkg.in/yaml.v3"
)

// preBootCategories lists settings categories whose values come from YAML
// and are always overwritten on server restart. They cannot be changed via
// the API/UI — the user must edit master.yaml and restart.
var preBootCategories = map[string]bool{
	"server": true,
	"grpc":   true,
}

// IsPreBootCategory returns true if the entire category is pre-boot (read-only).
func IsPreBootCategory(category string) bool {
	return preBootCategories[category]
}

// SettingsService manages configuration stored in the database.
type SettingsService struct {
	store storage.Store
	kms   *security.KMS
}

// NewSettingsService creates a new SettingsService.
func NewSettingsService(store storage.Store, kms *security.KMS) *SettingsService {
	return &SettingsService{
		store: store,
		kms:   kms,
	}
}

// IsInitialized checks if the system has been initialized.
func (s *SettingsService) IsInitialized(ctx context.Context) (bool, error) {
	return s.store.HasSettings(ctx)
}

// Get retrieves a setting value as a string.
func (s *SettingsService) Get(ctx context.Context, category, key string) (string, error) {
	setting, err := s.store.GetSetting(ctx, category, key)
	if errors.Is(err, storage.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	// Decrypt if needed
	if setting.Encrypted && s.kms != nil {
		decrypted, err := s.kms.DecryptString(ctx, setting.Value)
		if err != nil {
			return "", fmt.Errorf("decrypting setting: %w", err)
		}
		return decrypted, nil
	}

	return setting.Value, nil
}

// ErrSettingRequired is returned when a required setting is missing or empty.
var ErrSettingRequired = fmt.Errorf("required setting is missing or empty")

// GetRequired retrieves a required setting value. Returns ErrSettingRequired if missing or empty.
func (s *SettingsService) GetRequired(ctx context.Context, category, key string) (string, error) {
	val, err := s.Get(ctx, category, key)
	if err != nil {
		return "", err
	}
	if val == "" {
		return "", fmt.Errorf("%w: %s.%s", ErrSettingRequired, category, key)
	}
	return val, nil
}

// GetRequiredInt retrieves a required integer setting. Returns ErrSettingRequired if missing or empty.
func (s *SettingsService) GetRequiredInt(ctx context.Context, category, key string) (int, error) {
	val, err := s.GetRequired(ctx, category, key)
	if err != nil {
		return 0, err
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("invalid integer value for %s.%s: %w", category, key, err)
	}
	return i, nil
}

// GetRequiredBool retrieves a required boolean setting. Returns ErrSettingRequired if missing or empty.
func (s *SettingsService) GetRequiredBool(ctx context.Context, category, key string) (bool, error) {
	val, err := s.GetRequired(ctx, category, key)
	if err != nil {
		return false, err
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return false, fmt.Errorf("invalid boolean value for %s.%s: %w", category, key, err)
	}
	return b, nil
}

// GetRequiredDuration retrieves a required duration setting. Returns ErrSettingRequired if missing or empty.
func (s *SettingsService) GetRequiredDuration(ctx context.Context, category, key string) (time.Duration, error) {
	val, err := s.GetRequired(ctx, category, key)
	if err != nil {
		return 0, err
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return 0, fmt.Errorf("invalid duration value for %s.%s: %w", category, key, err)
	}
	return d, nil
}

// GetString retrieves a string setting with a default.
func (s *SettingsService) GetString(ctx context.Context, category, key, defaultVal string) string {
	val, err := s.Get(ctx, category, key)
	if err != nil || val == "" {
		return defaultVal
	}
	return val
}

// GetInt retrieves an integer setting with a default.
func (s *SettingsService) GetInt(ctx context.Context, category, key string, defaultVal int) int {
	val, err := s.Get(ctx, category, key)
	if err != nil || val == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return i
}

// GetBool retrieves a boolean setting with a default.
func (s *SettingsService) GetBool(ctx context.Context, category, key string, defaultVal bool) bool {
	val, err := s.Get(ctx, category, key)
	if err != nil || val == "" {
		return defaultVal
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return defaultVal
	}
	return b
}

// GetDuration retrieves a duration setting with a default.
func (s *SettingsService) GetDuration(ctx context.Context, category, key string, defaultVal time.Duration) time.Duration {
	val, err := s.Get(ctx, category, key)
	if err != nil || val == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return defaultVal
	}
	return d
}

// Set stores a setting value.
func (s *SettingsService) Set(ctx context.Context, category, key, value string, encrypted bool) error {
	valueType := "string"
	storeValue := value

	// Encrypt if needed
	if encrypted && s.kms != nil {
		encryptedVal, err := s.kms.EncryptString(ctx, value)
		if err != nil {
			return fmt.Errorf("encrypting setting: %w", err)
		}
		storeValue = encryptedVal
	}

	return s.store.SetSetting(ctx, category, key, storeValue, valueType, encrypted)
}

// SetInt stores an integer setting.
func (s *SettingsService) SetInt(ctx context.Context, category, key string, value int) error {
	return s.store.SetSetting(ctx, category, key, strconv.Itoa(value), "int", false)
}

// SetBool stores a boolean setting.
func (s *SettingsService) SetBool(ctx context.Context, category, key string, value bool) error {
	return s.store.SetSetting(ctx, category, key, strconv.FormatBool(value), "bool", false)
}

// SetDuration stores a duration setting.
func (s *SettingsService) SetDuration(ctx context.Context, category, key string, value time.Duration) error {
	return s.store.SetSetting(ctx, category, key, value.String(), "duration", false)
}

// initString seeds a string setting only if it does not already exist.
// Used for runtime defaults where user edits should survive restarts.
func (s *SettingsService) initString(ctx context.Context, category, key, value string) error {
	return s.store.InitSetting(ctx, category, key, value, "string", false)
}

// initBool seeds a boolean setting only if it does not already exist.
func (s *SettingsService) initBool(ctx context.Context, category, key string, value bool) error {
	return s.store.InitSetting(ctx, category, key, strconv.FormatBool(value), "bool", false)
}

// initInt seeds an integer setting only if it does not already exist.
func (s *SettingsService) initInt(ctx context.Context, category, key string, value int) error {
	return s.store.InitSetting(ctx, category, key, strconv.Itoa(value), "int", false)
}

// initDuration seeds a duration setting only if it does not already exist.
func (s *SettingsService) initDuration(ctx context.Context, category, key string, value time.Duration) error {
	return s.store.InitSetting(ctx, category, key, value.String(), "duration", false)
}

// SetRaw stores a setting with explicit value type (for import scenarios).
func (s *SettingsService) SetRaw(ctx context.Context, category, key, value, valueType string, encrypted bool) error {
	storeValue := value

	// Encrypt if needed
	if encrypted && s.kms != nil {
		encryptedVal, err := s.kms.EncryptString(ctx, value)
		if err != nil {
			return fmt.Errorf("encrypting setting: %w", err)
		}
		storeValue = encryptedVal
	}

	return s.store.SetSetting(ctx, category, key, storeValue, valueType, encrypted)
}

// Delete removes a setting.
func (s *SettingsService) Delete(ctx context.Context, category, key string) error {
	return s.store.DeleteSetting(ctx, category, key)
}

// SettingMetadata represents setting info returned by list operations.
type SettingMetadata struct {
	ID          string
	Category    string
	Key         string
	Value       string
	ValueType   string
	Encrypted   bool
	Description string
}

// ListByCategory returns all settings in a category.
func (s *SettingsService) ListByCategory(ctx context.Context, category string) ([]*SettingMetadata, error) {
	settings, err := s.store.ListSettingsByCategory(ctx, category)
	if err != nil {
		return nil, fmt.Errorf("listing settings: %w", err)
	}

	result := make([]*SettingMetadata, len(settings))
	for i, setting := range settings {
		result[i] = &SettingMetadata{
			ID:          setting.ID,
			Category:    setting.Category,
			Key:         setting.Key,
			Value:       setting.Value,
			ValueType:   setting.ValueType,
			Encrypted:   setting.Encrypted,
			Description: setting.Description,
		}
	}
	return result, nil
}

// ListAll returns all settings across all categories.
func (s *SettingsService) ListAll(ctx context.Context) ([]*SettingMetadata, error) {
	settings, err := s.store.ListAllSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing settings: %w", err)
	}

	result := make([]*SettingMetadata, len(settings))
	for i, setting := range settings {
		result[i] = &SettingMetadata{
			ID:          setting.ID,
			Category:    setting.Category,
			Key:         setting.Key,
			Value:       setting.Value,
			ValueType:   setting.ValueType,
			Encrypted:   setting.Encrypted,
			Description: setting.Description,
		}
	}
	return result, nil
}

// GetCategory retrieves all settings in a category as a map.
func (s *SettingsService) GetCategory(ctx context.Context, category string) (map[string]string, error) {
	settings, err := s.store.ListSettingsByCategory(ctx, category)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string, len(settings))
	for _, setting := range settings {
		val := setting.Value
		// Decrypt if needed
		if setting.Encrypted && s.kms != nil {
			decrypted, err := s.kms.DecryptString(ctx, val)
			if err != nil {
				return nil, fmt.Errorf("decrypting setting %s: %w", setting.Key, err)
			}
			val = decrypted
		}
		result[setting.Key] = val
	}
	return result, nil
}

// SetDefaults initializes the database with default settings.
// Pre-boot fields (server, gRPC) always overwrite — YAML is source of truth.
// Runtime fields (SSH, security, backup, logs, etc.) are seeded only on first boot —
// user edits via UI/CLI survive server restarts.
func (s *SettingsService) SetDefaults(ctx context.Context) error {
	defaults := DefaultMasterConfig()

	// === Pre-boot fields: YAML always overwrites DB on restart ===

	// Server settings (pre-boot)
	if err := s.Set(ctx, "server", "listen", defaults.Server.Listen, false); err != nil {
		return err
	}
	if err := s.SetBool(ctx, "server", "tls_enabled", defaults.Server.TLS.Enabled); err != nil {
		return err
	}
	if err := s.Set(ctx, "server", "tls_cert", defaults.Server.TLS.Cert, false); err != nil {
		return err
	}
	if err := s.Set(ctx, "server", "tls_key", defaults.Server.TLS.Key, false); err != nil {
		return err
	}

	// gRPC settings (pre-boot)
	if err := s.Set(ctx, "grpc", "listen", defaults.GRPC.Listen, false); err != nil {
		return err
	}

	// === Runtime fields: DB is source of truth, seed only on first boot ===

	// SSH settings (runtime)
	if err := s.initString(ctx, "ssh", "default_user", defaults.SSH.DefaultUser); err != nil {
		return err
	}
	if err := s.initString(ctx, "ssh", "default_key", defaults.SSH.DefaultKey); err != nil {
		return err
	}
	if err := s.initString(ctx, "ssh", "known_hosts", defaults.SSH.KnownHosts); err != nil {
		return err
	}
	if err := s.initDuration(ctx, "ssh", "connection_timeout", defaults.SSH.ConnectionTimeout); err != nil {
		return err
	}
	if err := s.initDuration(ctx, "ssh", "keepalive_interval", defaults.SSH.KeepaliveInterval); err != nil {
		return err
	}
	if err := s.initDuration(ctx, "ssh", "idle_timeout", defaults.SSH.IdleTimeout); err != nil {
		return err
	}

	// Security settings (runtime)
	if err := s.initBool(ctx, "security", "key_rotation_enabled", defaults.Security.KeyRotation.Enabled); err != nil {
		return err
	}
	if err := s.initDuration(ctx, "security", "key_rotation_interval", defaults.Security.KeyRotation.Interval); err != nil {
		return err
	}
	if err := s.initDuration(ctx, "security", "session_timeout", defaults.Security.SessionTimeout); err != nil {
		return err
	}
	if err := s.initBool(ctx, "security", "require_2fa_admin", defaults.Security.Require2FAAdmin); err != nil {
		return err
	}

	// Backup settings (runtime)
	if err := s.initBool(ctx, "backup", "db_enabled", defaults.Backup.Database.Enabled); err != nil {
		return err
	}
	if err := s.initDuration(ctx, "backup", "db_interval", defaults.Backup.Database.Interval); err != nil {
		return err
	}
	if err := s.initDuration(ctx, "backup", "db_retention", defaults.Backup.Database.Retention); err != nil {
		return err
	}
	if err := s.initString(ctx, "backup", "db_path", defaults.Backup.Database.Path); err != nil {
		return err
	}
	if err := s.initInt(ctx, "backup", "config_versions", defaults.Backup.Config.Versions); err != nil {
		return err
	}

	// Logs settings (runtime)
	if err := s.initDuration(ctx, "logs", "deploy_retention", defaults.Logs.Deployment.Retention); err != nil {
		return err
	}
	if err := s.initInt(ctx, "logs", "deploy_max_size_mb", defaults.Logs.Deployment.MaxSizeMB); err != nil {
		return err
	}
	if err := s.initDuration(ctx, "logs", "audit_retention", defaults.Logs.Audit.Retention); err != nil {
		return err
	}
	if err := s.initBool(ctx, "logs", "audit_export_enabled", defaults.Logs.Audit.Export.Enabled); err != nil {
		return err
	}
	if err := s.initString(ctx, "logs", "audit_export_destination", defaults.Logs.Audit.Export.Destination); err != nil {
		return err
	}
	if err := s.initString(ctx, "logs", "audit_export_schedule", defaults.Logs.Audit.Export.Schedule); err != nil {
		return err
	}
	if err := s.initString(ctx, "logs", "app_level", defaults.Logs.Application.Level); err != nil {
		return err
	}
	if err := s.initDuration(ctx, "logs", "app_retention", defaults.Logs.Application.Retention); err != nil {
		return err
	}
	if err := s.initString(ctx, "logs", "rotation_schedule", defaults.Logs.Rotation.Schedule); err != nil {
		return err
	}

	// Webhooks settings (runtime)
	if err := s.initBool(ctx, "webhooks", "github_enabled", defaults.Webhooks.GitHub.Enabled); err != nil {
		return err
	}
	if err := s.initString(ctx, "webhooks", "github_path", defaults.Webhooks.GitHub.Path); err != nil {
		return err
	}
	if err := s.initBool(ctx, "webhooks", "gitlab_enabled", defaults.Webhooks.GitLab.Enabled); err != nil {
		return err
	}
	if err := s.initString(ctx, "webhooks", "gitlab_path", defaults.Webhooks.GitLab.Path); err != nil {
		return err
	}
	if err := s.initBool(ctx, "webhooks", "bitbucket_enabled", defaults.Webhooks.Bitbucket.Enabled); err != nil {
		return err
	}
	if err := s.initString(ctx, "webhooks", "bitbucket_path", defaults.Webhooks.Bitbucket.Path); err != nil {
		return err
	}

	// Notifications settings (runtime)
	if err := s.initBool(ctx, "notifications", "slack_enabled", defaults.Notifications.Providers.Slack.Enabled); err != nil {
		return err
	}
	if err := s.initBool(ctx, "notifications", "email_enabled", defaults.Notifications.Providers.Email.Enabled); err != nil {
		return err
	}
	if err := s.initBool(ctx, "notifications", "webhook_enabled", defaults.Notifications.Providers.Webhook.Enabled); err != nil {
		return err
	}

	// API settings (runtime)
	if err := s.initBool(ctx, "api", "enabled", defaults.API.Enabled); err != nil {
		return err
	}

	// Appearance settings (runtime)
	if err := s.initString(ctx, "appearance", "theme", defaults.Appearance.Theme); err != nil {
		return err
	}

	return nil
}

// GetMasterConfig builds a MasterConfig from database settings.
func (s *SettingsService) GetMasterConfig(ctx context.Context) (*MasterConfig, error) {
	cfg := DefaultMasterConfig()

	// Server settings
	cfg.Server.Listen = s.GetString(ctx, "server", "listen", cfg.Server.Listen)
	cfg.Server.TLS.Enabled = s.GetBool(ctx, "server", "tls_enabled", cfg.Server.TLS.Enabled)
	cfg.Server.TLS.Cert = s.GetString(ctx, "server", "tls_cert", cfg.Server.TLS.Cert)
	cfg.Server.TLS.Key = s.GetString(ctx, "server", "tls_key", cfg.Server.TLS.Key)

	// gRPC settings
	cfg.GRPC.Listen = s.GetString(ctx, "grpc", "listen", cfg.GRPC.Listen)

	// SSH settings
	cfg.SSH.DefaultUser = s.GetString(ctx, "ssh", "default_user", cfg.SSH.DefaultUser)
	cfg.SSH.DefaultKey = s.GetString(ctx, "ssh", "default_key", cfg.SSH.DefaultKey)
	cfg.SSH.KnownHosts = s.GetString(ctx, "ssh", "known_hosts", cfg.SSH.KnownHosts)
	cfg.SSH.ConnectionTimeout = s.GetDuration(ctx, "ssh", "connection_timeout", cfg.SSH.ConnectionTimeout)
	cfg.SSH.KeepaliveInterval = s.GetDuration(ctx, "ssh", "keepalive_interval", cfg.SSH.KeepaliveInterval)
	cfg.SSH.IdleTimeout = s.GetDuration(ctx, "ssh", "idle_timeout", cfg.SSH.IdleTimeout)

	// Security settings
	cfg.Security.KeyRotation.Enabled = s.GetBool(ctx, "security", "key_rotation_enabled", cfg.Security.KeyRotation.Enabled)
	cfg.Security.KeyRotation.Interval = s.GetDuration(ctx, "security", "key_rotation_interval", cfg.Security.KeyRotation.Interval)
	cfg.Security.SessionTimeout = s.GetDuration(ctx, "security", "session_timeout", cfg.Security.SessionTimeout)
	cfg.Security.Require2FAAdmin = s.GetBool(ctx, "security", "require_2fa_admin", cfg.Security.Require2FAAdmin)

	// Backup settings
	cfg.Backup.Database.Enabled = s.GetBool(ctx, "backup", "db_enabled", cfg.Backup.Database.Enabled)
	cfg.Backup.Database.Interval = s.GetDuration(ctx, "backup", "db_interval", cfg.Backup.Database.Interval)
	cfg.Backup.Database.Retention = s.GetDuration(ctx, "backup", "db_retention", cfg.Backup.Database.Retention)
	cfg.Backup.Database.Path = s.GetString(ctx, "backup", "db_path", cfg.Backup.Database.Path)
	cfg.Backup.Config.Versions = s.GetInt(ctx, "backup", "config_versions", cfg.Backup.Config.Versions)

	// Logs settings
	cfg.Logs.Deployment.Retention = s.GetDuration(ctx, "logs", "deploy_retention", cfg.Logs.Deployment.Retention)
	cfg.Logs.Deployment.MaxSizeMB = s.GetInt(ctx, "logs", "deploy_max_size_mb", cfg.Logs.Deployment.MaxSizeMB)
	cfg.Logs.Audit.Retention = s.GetDuration(ctx, "logs", "audit_retention", cfg.Logs.Audit.Retention)
	cfg.Logs.Audit.Export.Enabled = s.GetBool(ctx, "logs", "audit_export_enabled", cfg.Logs.Audit.Export.Enabled)
	cfg.Logs.Audit.Export.Destination = s.GetString(ctx, "logs", "audit_export_destination", cfg.Logs.Audit.Export.Destination)
	cfg.Logs.Audit.Export.Schedule = s.GetString(ctx, "logs", "audit_export_schedule", cfg.Logs.Audit.Export.Schedule)
	cfg.Logs.Application.Level = s.GetString(ctx, "logs", "app_level", cfg.Logs.Application.Level)
	cfg.Logs.Application.Retention = s.GetDuration(ctx, "logs", "app_retention", cfg.Logs.Application.Retention)
	cfg.Logs.Rotation.Schedule = s.GetString(ctx, "logs", "rotation_schedule", cfg.Logs.Rotation.Schedule)

	// Webhooks settings
	cfg.Webhooks.GitHub.Enabled = s.GetBool(ctx, "webhooks", "github_enabled", cfg.Webhooks.GitHub.Enabled)
	cfg.Webhooks.GitHub.Path = s.GetString(ctx, "webhooks", "github_path", cfg.Webhooks.GitHub.Path)
	cfg.Webhooks.GitLab.Enabled = s.GetBool(ctx, "webhooks", "gitlab_enabled", cfg.Webhooks.GitLab.Enabled)
	cfg.Webhooks.GitLab.Path = s.GetString(ctx, "webhooks", "gitlab_path", cfg.Webhooks.GitLab.Path)
	cfg.Webhooks.Bitbucket.Enabled = s.GetBool(ctx, "webhooks", "bitbucket_enabled", cfg.Webhooks.Bitbucket.Enabled)
	cfg.Webhooks.Bitbucket.Path = s.GetString(ctx, "webhooks", "bitbucket_path", cfg.Webhooks.Bitbucket.Path)

	// Notifications settings
	cfg.Notifications.Providers.Slack.Enabled = s.GetBool(ctx, "notifications", "slack_enabled", cfg.Notifications.Providers.Slack.Enabled)
	cfg.Notifications.Providers.Email.Enabled = s.GetBool(ctx, "notifications", "email_enabled", cfg.Notifications.Providers.Email.Enabled)
	cfg.Notifications.Providers.Webhook.Enabled = s.GetBool(ctx, "notifications", "webhook_enabled", cfg.Notifications.Providers.Webhook.Enabled)

	// API settings
	cfg.API.Enabled = s.GetBool(ctx, "api", "enabled", cfg.API.Enabled)

	// Appearance settings
	cfg.Appearance.Theme = s.GetString(ctx, "appearance", "theme", cfg.Appearance.Theme)

	return cfg, nil
}

// Export exports all settings as YAML.
func (s *SettingsService) Export(ctx context.Context) ([]byte, error) {
	settings, err := s.store.ListAllSettings(ctx)
	if err != nil {
		return nil, err
	}

	// Group by category
	grouped := make(map[string]map[string]interface{})
	for _, setting := range settings {
		if grouped[setting.Category] == nil {
			grouped[setting.Category] = make(map[string]interface{})
		}

		val := setting.Value
		// Decrypt if needed for export
		if setting.Encrypted && s.kms != nil {
			decrypted, err := s.kms.DecryptString(ctx, val)
			if err != nil {
				return nil, fmt.Errorf("decrypting setting %s/%s: %w", setting.Category, setting.Key, err)
			}
			val = decrypted
		}

		// Convert to appropriate type
		switch setting.ValueType {
		case "int":
			if i, err := strconv.Atoi(val); err == nil {
				grouped[setting.Category][setting.Key] = i
			} else {
				grouped[setting.Category][setting.Key] = val
			}
		case "bool":
			if b, err := strconv.ParseBool(val); err == nil {
				grouped[setting.Category][setting.Key] = b
			} else {
				grouped[setting.Category][setting.Key] = val
			}
		case "duration":
			grouped[setting.Category][setting.Key] = val
		default:
			grouped[setting.Category][setting.Key] = val
		}
	}

	return yaml.Marshal(grouped)
}

// ExportJSON exports all settings as JSON.
func (s *SettingsService) ExportJSON(ctx context.Context) ([]byte, error) {
	settings, err := s.store.ListAllSettings(ctx)
	if err != nil {
		return nil, err
	}

	// Group by category
	grouped := make(map[string]map[string]interface{})
	for _, setting := range settings {
		if grouped[setting.Category] == nil {
			grouped[setting.Category] = make(map[string]interface{})
		}

		val := setting.Value
		if setting.Encrypted && s.kms != nil {
			decrypted, err := s.kms.DecryptString(ctx, val)
			if err != nil {
				return nil, fmt.Errorf("decrypting setting %s/%s: %w", setting.Category, setting.Key, err)
			}
			val = decrypted
		}

		switch setting.ValueType {
		case "int":
			if i, err := strconv.Atoi(val); err == nil {
				grouped[setting.Category][setting.Key] = i
			} else {
				grouped[setting.Category][setting.Key] = val
			}
		case "bool":
			if b, err := strconv.ParseBool(val); err == nil {
				grouped[setting.Category][setting.Key] = b
			} else {
				grouped[setting.Category][setting.Key] = val
			}
		default:
			grouped[setting.Category][setting.Key] = val
		}
	}

	return json.MarshalIndent(grouped, "", "  ")
}

// Import imports settings from YAML.
func (s *SettingsService) Import(ctx context.Context, data []byte) error {
	var settings map[string]map[string]interface{}
	if err := yaml.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("parsing YAML: %w", err)
	}

	for category, kvs := range settings {
		for key, value := range kvs {
			var strVal string
			var valType string
			var encrypted bool

			switch v := value.(type) {
			case string:
				strVal = v
				valType = "string"
			case int:
				strVal = strconv.Itoa(v)
				valType = "int"
			case int64:
				strVal = strconv.FormatInt(v, 10)
				valType = "int"
			case float64:
				strVal = strconv.FormatInt(int64(v), 10)
				valType = "int"
			case bool:
				strVal = strconv.FormatBool(v)
				valType = "bool"
			default:
				strVal = fmt.Sprintf("%v", v)
				valType = "string"
			}

			// Check if this setting should be encrypted
			if category == "security" && (key == "master_key" || key == "smtp_password") {
				encrypted = true
				if s.kms != nil {
					encVal, err := s.kms.EncryptString(ctx, strVal)
					if err != nil {
						return fmt.Errorf("encrypting %s/%s: %w", category, key, err)
					}
					strVal = encVal
				}
			}

			if err := s.store.SetSetting(ctx, category, key, strVal, valType, encrypted); err != nil {
				return fmt.Errorf("setting %s/%s: %w", category, key, err)
			}
		}
	}

	return nil
}
