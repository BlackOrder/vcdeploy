// Package settings provides settings management functionality.
// For backwards compatibility, the full SettingsService implementation
// remains in the config package. This package provides an interface-compatible
// wrapper and will eventually contain the full implementation.
package settings

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// Ensure Service implements the interface.
var _ services.SettingsServicer = (*Service)(nil)

// ErrSettingRequired is returned when a required setting is missing or empty.
var ErrSettingRequired = errors.New("required setting is missing or empty")

// Service handles settings management.
type Service struct {
	store storage.Store
	kms   *security.KMS
}

// New creates a new settings Service.
func New(store storage.Store, kms *security.KMS) *Service {
	return &Service{store: store, kms: kms}
}

// IsInitialized checks if the system has been initialized.
func (s *Service) IsInitialized(ctx context.Context) (bool, error) {
	return s.store.HasSettings(ctx)
}

// Get retrieves a setting value as a string.
func (s *Service) Get(ctx context.Context, category, key string) (string, error) {
	setting, err := s.store.GetSetting(ctx, category, key)
	if services.IsNotFound(err) {
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

// GetRequired retrieves a required setting value.
func (s *Service) GetRequired(ctx context.Context, category, key string) (string, error) {
	val, err := s.Get(ctx, category, key)
	if err != nil {
		return "", err
	}
	if val == "" {
		return "", fmt.Errorf("%w: %s.%s", ErrSettingRequired, category, key)
	}
	return val, nil
}

// GetRequiredInt retrieves a required integer setting.
func (s *Service) GetRequiredInt(ctx context.Context, category, key string) (int, error) {
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

// GetRequiredBool retrieves a required boolean setting.
func (s *Service) GetRequiredBool(ctx context.Context, category, key string) (bool, error) {
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

// GetRequiredDuration retrieves a required duration setting.
func (s *Service) GetRequiredDuration(ctx context.Context, category, key string) (time.Duration, error) {
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
func (s *Service) GetString(ctx context.Context, category, key, defaultVal string) (string, error) {
	val, err := s.Get(ctx, category, key)
	if err != nil {
		return defaultVal, err
	}
	if val == "" {
		return defaultVal, nil
	}
	return val, nil
}

// GetInt retrieves an integer setting with a default.
func (s *Service) GetInt(ctx context.Context, category, key string, defaultVal int) (int, error) {
	val, err := s.Get(ctx, category, key)
	if err != nil || val == "" {
		return defaultVal, err
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal, nil
	}
	return i, nil
}

// GetBool retrieves a boolean setting with a default.
func (s *Service) GetBool(ctx context.Context, category, key string, defaultVal bool) (bool, error) {
	val, err := s.Get(ctx, category, key)
	if err != nil || val == "" {
		return defaultVal, err
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return defaultVal, nil
	}
	return b, nil
}

// GetDuration retrieves a duration setting with a default.
func (s *Service) GetDuration(ctx context.Context, category, key string, defaultVal time.Duration) (time.Duration, error) {
	val, err := s.Get(ctx, category, key)
	if err != nil || val == "" {
		return defaultVal, err
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return defaultVal, nil
	}
	return d, nil
}

// Set stores a setting value.
func (s *Service) Set(ctx context.Context, category, key, value string, encrypted bool) error {
	storeValue := value

	// Encrypt if needed
	if encrypted && s.kms != nil {
		encryptedVal, err := s.kms.EncryptString(ctx, value)
		if err != nil {
			return fmt.Errorf("encrypting setting: %w", err)
		}
		storeValue = encryptedVal
	}

	return s.store.SetSetting(ctx, category, key, storeValue, "string", encrypted)
}

// SetInt stores an integer setting.
func (s *Service) SetInt(ctx context.Context, category, key string, value int) error {
	return s.store.SetSetting(ctx, category, key, strconv.Itoa(value), "int", false)
}

// SetBool stores a boolean setting.
func (s *Service) SetBool(ctx context.Context, category, key string, value bool) error {
	return s.store.SetSetting(ctx, category, key, strconv.FormatBool(value), "bool", false)
}

// SetDuration stores a duration setting.
func (s *Service) SetDuration(ctx context.Context, category, key string, value time.Duration) error {
	return s.store.SetSetting(ctx, category, key, value.String(), "duration", false)
}

// SetRaw stores a setting with explicit value type.
func (s *Service) SetRaw(ctx context.Context, category, key, value, valueType string, encrypted bool) error {
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
func (s *Service) Delete(ctx context.Context, category, key string) error {
	return s.store.DeleteSetting(ctx, category, key)
}

// ListByCategory returns all settings in a category.
func (s *Service) ListByCategory(ctx context.Context, category string) ([]services.SettingMetadata, error) {
	settings, err := s.store.ListSettingsByCategory(ctx, category)
	if err != nil {
		return nil, fmt.Errorf("listing settings: %w", err)
	}

	result := make([]services.SettingMetadata, len(settings))
	for i, setting := range settings {
		result[i] = services.SettingMetadata{
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
func (s *Service) ListAll(ctx context.Context) ([]services.SettingMetadata, error) {
	settings, err := s.store.ListAllSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing settings: %w", err)
	}

	result := make([]services.SettingMetadata, len(settings))
	for i, setting := range settings {
		result[i] = services.SettingMetadata{
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
