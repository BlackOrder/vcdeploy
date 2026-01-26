package settings

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

func newTestService(t *testing.T) (*Service, *storage.DB) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	logger := zap.NewNop()
	db, err := storage.New(dbPath, logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Create KMS using the underlying sql.DB connection
	kms, err := security.NewKMS(db.Conn(), nil)
	if err != nil {
		t.Fatalf("Failed to create KMS: %v", err)
	}

	// Initialize KMS (creates encryption key)
	ctx := context.Background()
	if err := kms.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize KMS: %v", err)
	}

	return New(db, kms), db
}

func TestService_Set_And_Get(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set a string setting
	err := svc.Set(ctx, "general", "site_name", "My Site", false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Get the setting
	value, err := svc.Get(ctx, "general", "site_name")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if value != "My Site" {
		t.Errorf("Get() = %v, want %v", value, "My Site")
	}
}

func TestService_Get_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Get non-existent setting (returns empty string, not error)
	value, err := svc.Get(ctx, "general", "nonexistent")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if value != "" {
		t.Errorf("Get() = %v, want empty string", value)
	}
}

func TestService_Set_Encrypted(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set an encrypted setting
	err := svc.Set(ctx, "security", "api_secret", "super-secret-value", true)
	if err != nil {
		t.Fatalf("Set() encrypted error = %v", err)
	}

	// Get the setting (should be decrypted)
	value, err := svc.Get(ctx, "security", "api_secret")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if value != "super-secret-value" {
		t.Errorf("Get() encrypted = %v, want %v", value, "super-secret-value")
	}
}

func TestService_GetRequired(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set a setting
	err := svc.Set(ctx, "general", "required_setting", "value", false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Get required setting
	value, err := svc.GetRequired(ctx, "general", "required_setting")
	if err != nil {
		t.Fatalf("GetRequired() error = %v", err)
	}
	if value != "value" {
		t.Errorf("GetRequired() = %v, want %v", value, "value")
	}
}

func TestService_GetRequired_Missing(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Get required setting that doesn't exist
	_, err := svc.GetRequired(ctx, "general", "missing")
	if err == nil {
		t.Error("GetRequired() expected error for missing setting")
	}
}

func TestService_SetInt_And_GetRequiredInt(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set an int setting
	err := svc.SetInt(ctx, "general", "max_items", 100)
	if err != nil {
		t.Fatalf("SetInt() error = %v", err)
	}

	// Get the int setting
	value, err := svc.GetRequiredInt(ctx, "general", "max_items")
	if err != nil {
		t.Fatalf("GetRequiredInt() error = %v", err)
	}
	if value != 100 {
		t.Errorf("GetRequiredInt() = %v, want %v", value, 100)
	}
}

func TestService_GetRequiredInt_Invalid(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set a non-integer value
	err := svc.Set(ctx, "general", "bad_int", "not-a-number", false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Try to get as int
	_, err = svc.GetRequiredInt(ctx, "general", "bad_int")
	if err == nil {
		t.Error("GetRequiredInt() expected error for non-integer value")
	}
}

func TestService_SetBool_And_GetRequiredBool(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set a bool setting
	err := svc.SetBool(ctx, "features", "enabled", true)
	if err != nil {
		t.Fatalf("SetBool() error = %v", err)
	}

	// Get the bool setting
	value, err := svc.GetRequiredBool(ctx, "features", "enabled")
	if err != nil {
		t.Fatalf("GetRequiredBool() error = %v", err)
	}
	if value != true {
		t.Errorf("GetRequiredBool() = %v, want %v", value, true)
	}
}

func TestService_SetDuration_And_GetRequiredDuration(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set a duration setting
	err := svc.SetDuration(ctx, "timeouts", "session", 24*time.Hour)
	if err != nil {
		t.Fatalf("SetDuration() error = %v", err)
	}

	// Get the duration setting
	value, err := svc.GetRequiredDuration(ctx, "timeouts", "session")
	if err != nil {
		t.Fatalf("GetRequiredDuration() error = %v", err)
	}
	if value != 24*time.Hour {
		t.Errorf("GetRequiredDuration() = %v, want %v", value, 24*time.Hour)
	}
}

func TestService_GetString_WithDefault(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Get non-existent setting with default
	value, err := svc.GetString(ctx, "general", "missing", "default-value")
	if err != nil {
		t.Fatalf("GetString() error = %v", err)
	}
	if value != "default-value" {
		t.Errorf("GetString() = %v, want %v", value, "default-value")
	}

	// Set a value and get it (should not use default)
	err = svc.Set(ctx, "general", "has_value", "actual-value", false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	value, err = svc.GetString(ctx, "general", "has_value", "default-value")
	if err != nil {
		t.Fatalf("GetString() error = %v", err)
	}
	if value != "actual-value" {
		t.Errorf("GetString() = %v, want %v", value, "actual-value")
	}
}

func TestService_GetInt_WithDefault(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Get non-existent setting with default
	value, err := svc.GetInt(ctx, "general", "missing", 42)
	if err != nil {
		t.Fatalf("GetInt() error = %v", err)
	}
	if value != 42 {
		t.Errorf("GetInt() = %v, want %v", value, 42)
	}
}

func TestService_GetBool_WithDefault(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Get non-existent setting with default
	value, err := svc.GetBool(ctx, "general", "missing", true)
	if err != nil {
		t.Fatalf("GetBool() error = %v", err)
	}
	if value != true {
		t.Errorf("GetBool() = %v, want %v", value, true)
	}
}

func TestService_GetDuration_WithDefault(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Get non-existent setting with default
	value, err := svc.GetDuration(ctx, "general", "missing", 5*time.Minute)
	if err != nil {
		t.Fatalf("GetDuration() error = %v", err)
	}
	if value != 5*time.Minute {
		t.Errorf("GetDuration() = %v, want %v", value, 5*time.Minute)
	}
}

func TestService_Delete(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set a setting
	err := svc.Set(ctx, "general", "to_delete", "value", false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Verify it exists
	value, err := svc.Get(ctx, "general", "to_delete")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if value == "" {
		t.Fatal("Setting should exist before delete")
	}

	// Delete it
	err = svc.Delete(ctx, "general", "to_delete")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify it's gone
	value, err = svc.Get(ctx, "general", "to_delete")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if value != "" {
		t.Errorf("Get() after delete = %v, want empty string", value)
	}
}

func TestService_ListByCategory(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set settings in different categories
	err := svc.Set(ctx, "general", "setting1", "value1", false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	err = svc.Set(ctx, "general", "setting2", "value2", false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	err = svc.Set(ctx, "other", "setting3", "value3", false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// List settings in 'general' category
	settings, err := svc.ListByCategory(ctx, "general")
	if err != nil {
		t.Fatalf("ListByCategory() error = %v", err)
	}
	if len(settings) != 2 {
		t.Errorf("ListByCategory() count = %v, want 2", len(settings))
	}

	// All should be in 'general' category
	for _, s := range settings {
		if s.Category != "general" {
			t.Errorf("ListByCategory() category = %v, want %v", s.Category, "general")
		}
	}
}

func TestService_ListAll(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set settings in different categories
	err := svc.Set(ctx, "general", "setting1", "value1", false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	err = svc.Set(ctx, "security", "setting2", "value2", false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	err = svc.Set(ctx, "features", "setting3", "value3", false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// List all settings
	settings, err := svc.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	if len(settings) != 3 {
		t.Errorf("ListAll() count = %v, want 3", len(settings))
	}
}

func TestService_IsInitialized(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Initially should not be initialized (no settings)
	initialized, err := svc.IsInitialized(ctx)
	if err != nil {
		t.Fatalf("IsInitialized() error = %v", err)
	}
	if initialized {
		t.Error("IsInitialized() should be false initially")
	}

	// Add a setting
	err = svc.Set(ctx, "general", "initialized", "true", false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Now should be initialized
	initialized, err = svc.IsInitialized(ctx)
	if err != nil {
		t.Fatalf("IsInitialized() error = %v", err)
	}
	if !initialized {
		t.Error("IsInitialized() should be true after adding settings")
	}
}

func TestService_SetRaw(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set with explicit value type
	err := svc.SetRaw(ctx, "general", "custom", "custom-value", "custom-type", false)
	if err != nil {
		t.Fatalf("SetRaw() error = %v", err)
	}

	// Verify by listing
	settings, err := svc.ListByCategory(ctx, "general")
	if err != nil {
		t.Fatalf("ListByCategory() error = %v", err)
	}
	if len(settings) != 1 {
		t.Fatalf("ListByCategory() count = %v, want 1", len(settings))
	}
	if settings[0].ValueType != "custom-type" {
		t.Errorf("SetRaw() valueType = %v, want %v", settings[0].ValueType, "custom-type")
	}
}

func TestService_Update_Existing(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set initial value
	err := svc.Set(ctx, "general", "key", "initial", false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Update value
	err = svc.Set(ctx, "general", "key", "updated", false)
	if err != nil {
		t.Fatalf("Set() update error = %v", err)
	}

	// Verify updated value
	value, err := svc.Get(ctx, "general", "key")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if value != "updated" {
		t.Errorf("Get() = %v, want %v", value, "updated")
	}
}

func TestService_GetRequiredInt_Missing(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Get required int that doesn't exist
	_, err := svc.GetRequiredInt(ctx, "general", "nonexistent")
	if err == nil {
		t.Error("GetRequiredInt() expected error for missing setting")
	}
}

func TestService_GetRequiredBool_Missing(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Get required bool that doesn't exist
	_, err := svc.GetRequiredBool(ctx, "features", "nonexistent")
	if err == nil {
		t.Error("GetRequiredBool() expected error for missing setting")
	}
}

func TestService_GetRequiredBool_Invalid(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set a non-boolean value
	err := svc.Set(ctx, "features", "bad_bool", "not-a-bool", false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Try to get as bool
	_, err = svc.GetRequiredBool(ctx, "features", "bad_bool")
	if err == nil {
		t.Error("GetRequiredBool() expected error for non-boolean value")
	}
}

func TestService_GetRequiredDuration_Missing(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Get required duration that doesn't exist
	_, err := svc.GetRequiredDuration(ctx, "timeouts", "nonexistent")
	if err == nil {
		t.Error("GetRequiredDuration() expected error for missing setting")
	}
}

func TestService_GetRequiredDuration_Invalid(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set a non-duration value
	err := svc.Set(ctx, "timeouts", "bad_duration", "not-a-duration", false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Try to get as duration
	_, err = svc.GetRequiredDuration(ctx, "timeouts", "bad_duration")
	if err == nil {
		t.Error("GetRequiredDuration() expected error for non-duration value")
	}
}

func TestService_GetInt_ExistingValue(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set an integer value
	err := svc.SetInt(ctx, "general", "port", 8080)
	if err != nil {
		t.Fatalf("SetInt() error = %v", err)
	}

	// Get it with default (should return actual value, not default)
	value, err := svc.GetInt(ctx, "general", "port", 9999)
	if err != nil {
		t.Fatalf("GetInt() error = %v", err)
	}
	if value != 8080 {
		t.Errorf("GetInt() = %v, want %v", value, 8080)
	}
}

func TestService_GetInt_InvalidValue(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set a non-integer value
	err := svc.Set(ctx, "general", "invalid_int", "not-a-number", false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Get with default (should return default due to parse error)
	value, err := svc.GetInt(ctx, "general", "invalid_int", 42)
	if err != nil {
		t.Fatalf("GetInt() error = %v", err)
	}
	if value != 42 {
		t.Errorf("GetInt() = %v, want default %v", value, 42)
	}
}

func TestService_GetBool_ExistingValue(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set a boolean value
	err := svc.SetBool(ctx, "features", "debug", true)
	if err != nil {
		t.Fatalf("SetBool() error = %v", err)
	}

	// Get it with default (should return actual value, not default)
	value, err := svc.GetBool(ctx, "features", "debug", false)
	if err != nil {
		t.Fatalf("GetBool() error = %v", err)
	}
	if value != true {
		t.Errorf("GetBool() = %v, want %v", value, true)
	}
}

func TestService_GetBool_InvalidValue(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set a non-boolean value
	err := svc.Set(ctx, "features", "invalid_bool", "maybe", false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Get with default (should return default due to parse error)
	value, err := svc.GetBool(ctx, "features", "invalid_bool", true)
	if err != nil {
		t.Fatalf("GetBool() error = %v", err)
	}
	if value != true {
		t.Errorf("GetBool() = %v, want default %v", value, true)
	}
}

func TestService_GetDuration_ExistingValue(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set a duration value
	err := svc.SetDuration(ctx, "timeouts", "request", 30*time.Second)
	if err != nil {
		t.Fatalf("SetDuration() error = %v", err)
	}

	// Get it with default (should return actual value, not default)
	value, err := svc.GetDuration(ctx, "timeouts", "request", 1*time.Hour)
	if err != nil {
		t.Fatalf("GetDuration() error = %v", err)
	}
	if value != 30*time.Second {
		t.Errorf("GetDuration() = %v, want %v", value, 30*time.Second)
	}
}

func TestService_GetDuration_InvalidValue(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set a non-duration value
	err := svc.Set(ctx, "timeouts", "invalid_duration", "five minutes", false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Get with default (should return default due to parse error)
	value, err := svc.GetDuration(ctx, "timeouts", "invalid_duration", 5*time.Minute)
	if err != nil {
		t.Fatalf("GetDuration() error = %v", err)
	}
	if value != 5*time.Minute {
		t.Errorf("GetDuration() = %v, want default %v", value, 5*time.Minute)
	}
}

func TestService_SetRaw_Encrypted(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set an encrypted raw setting
	err := svc.SetRaw(ctx, "secrets", "api_token", "secret-token-value", "token", true)
	if err != nil {
		t.Fatalf("SetRaw() encrypted error = %v", err)
	}

	// Get the setting (should be decrypted)
	value, err := svc.Get(ctx, "secrets", "api_token")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if value != "secret-token-value" {
		t.Errorf("Get() encrypted = %v, want %v", value, "secret-token-value")
	}

	// Verify the metadata via ListByCategory
	settings, err := svc.ListByCategory(ctx, "secrets")
	if err != nil {
		t.Fatalf("ListByCategory() error = %v", err)
	}
	if len(settings) != 1 {
		t.Fatalf("ListByCategory() count = %v, want 1", len(settings))
	}
	if settings[0].ValueType != "token" {
		t.Errorf("SetRaw() valueType = %v, want %v", settings[0].ValueType, "token")
	}
	if !settings[0].Encrypted {
		t.Error("SetRaw() encrypted should be true")
	}
}

func TestService_SetBool_False(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set a false bool setting
	err := svc.SetBool(ctx, "features", "disabled", false)
	if err != nil {
		t.Fatalf("SetBool() error = %v", err)
	}

	// Get the bool setting
	value, err := svc.GetRequiredBool(ctx, "features", "disabled")
	if err != nil {
		t.Fatalf("GetRequiredBool() error = %v", err)
	}
	if value != false {
		t.Errorf("GetRequiredBool() = %v, want %v", value, false)
	}
}

func TestService_GetBool_FalseDefault(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Get non-existent setting with false default
	value, err := svc.GetBool(ctx, "general", "missing_bool", false)
	if err != nil {
		t.Fatalf("GetBool() error = %v", err)
	}
	if value != false {
		t.Errorf("GetBool() = %v, want %v", value, false)
	}
}

func TestService_Delete_NonExistent(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Delete a non-existent setting (should not error)
	err := svc.Delete(ctx, "general", "nonexistent")
	if err != nil {
		t.Fatalf("Delete() error = %v (expected nil for non-existent)", err)
	}
}

func TestService_ListByCategory_Empty(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// List settings in an empty category
	settings, err := svc.ListByCategory(ctx, "empty_category")
	if err != nil {
		t.Fatalf("ListByCategory() error = %v", err)
	}
	if len(settings) != 0 {
		t.Errorf("ListByCategory() count = %v, want 0", len(settings))
	}
}

func TestService_ListAll_Empty(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// List all settings when there are none
	settings, err := svc.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	if len(settings) != 0 {
		t.Errorf("ListAll() count = %v, want 0", len(settings))
	}
}

func TestService_GetString_ExistingValue(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set a string value
	err := svc.Set(ctx, "general", "site_url", "https://example.com", false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Get it with default (should return actual value, not default)
	value, err := svc.GetString(ctx, "general", "site_url", "https://default.com")
	if err != nil {
		t.Fatalf("GetString() error = %v", err)
	}
	if value != "https://example.com" {
		t.Errorf("GetString() = %v, want %v", value, "https://example.com")
	}
}

func TestService_SetInt_Zero(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set zero value
	err := svc.SetInt(ctx, "general", "zero_setting", 0)
	if err != nil {
		t.Fatalf("SetInt() error = %v", err)
	}

	// Get the int setting
	value, err := svc.GetRequiredInt(ctx, "general", "zero_setting")
	if err != nil {
		t.Fatalf("GetRequiredInt() error = %v", err)
	}
	if value != 0 {
		t.Errorf("GetRequiredInt() = %v, want %v", value, 0)
	}
}

func TestService_SetInt_Negative(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set negative value
	err := svc.SetInt(ctx, "general", "negative_setting", -100)
	if err != nil {
		t.Fatalf("SetInt() error = %v", err)
	}

	// Get the int setting
	value, err := svc.GetRequiredInt(ctx, "general", "negative_setting")
	if err != nil {
		t.Fatalf("GetRequiredInt() error = %v", err)
	}
	if value != -100 {
		t.Errorf("GetRequiredInt() = %v, want %v", value, -100)
	}
}

func TestService_GetInt_ZeroDefault(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Get non-existent setting with zero default
	value, err := svc.GetInt(ctx, "general", "missing_int", 0)
	if err != nil {
		t.Fatalf("GetInt() error = %v", err)
	}
	if value != 0 {
		t.Errorf("GetInt() = %v, want %v", value, 0)
	}
}

func TestService_SetDuration_Zero(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set zero duration
	err := svc.SetDuration(ctx, "timeouts", "no_delay", 0)
	if err != nil {
		t.Fatalf("SetDuration() error = %v", err)
	}

	// Get the duration setting
	value, err := svc.GetRequiredDuration(ctx, "timeouts", "no_delay")
	if err != nil {
		t.Fatalf("GetRequiredDuration() error = %v", err)
	}
	if value != 0 {
		t.Errorf("GetRequiredDuration() = %v, want %v", value, 0)
	}
}

func TestService_GetDuration_ZeroDefault(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Get non-existent setting with zero default
	value, err := svc.GetDuration(ctx, "timeouts", "missing_duration", 0)
	if err != nil {
		t.Fatalf("GetDuration() error = %v", err)
	}
	if value != 0 {
		t.Errorf("GetDuration() = %v, want %v", value, 0)
	}
}

func TestService_Set_EmptyValue(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set an empty string value
	err := svc.Set(ctx, "general", "empty_setting", "", false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Get the setting (returns empty string)
	value, err := svc.Get(ctx, "general", "empty_setting")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if value != "" {
		t.Errorf("Get() = %v, want empty string", value)
	}

	// GetRequired should fail for empty value
	_, err = svc.GetRequired(ctx, "general", "empty_setting")
	if err == nil {
		t.Error("GetRequired() expected error for empty value")
	}
}

func TestService_Get_UnencryptedWithEncryptedFlag(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// First set a non-encrypted value
	err := svc.Set(ctx, "test", "plain", "plain-value", false)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Now set an encrypted value
	err = svc.Set(ctx, "test", "secret", "secret-value", true)
	if err != nil {
		t.Fatalf("Set() encrypted error = %v", err)
	}

	// Verify both can be retrieved correctly
	plainValue, err := svc.Get(ctx, "test", "plain")
	if err != nil {
		t.Fatalf("Get() plain error = %v", err)
	}
	if plainValue != "plain-value" {
		t.Errorf("Get() plain = %v, want %v", plainValue, "plain-value")
	}

	secretValue, err := svc.Get(ctx, "test", "secret")
	if err != nil {
		t.Fatalf("Get() secret error = %v", err)
	}
	if secretValue != "secret-value" {
		t.Errorf("Get() secret = %v, want %v", secretValue, "secret-value")
	}
}

func TestService_ListByCategory_VerifyMetadata(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set settings with different types
	err := svc.SetInt(ctx, "test_meta", "int_setting", 42)
	if err != nil {
		t.Fatalf("SetInt() error = %v", err)
	}
	err = svc.SetBool(ctx, "test_meta", "bool_setting", true)
	if err != nil {
		t.Fatalf("SetBool() error = %v", err)
	}
	err = svc.SetDuration(ctx, "test_meta", "duration_setting", 5*time.Minute)
	if err != nil {
		t.Fatalf("SetDuration() error = %v", err)
	}

	// List and verify metadata
	settings, err := svc.ListByCategory(ctx, "test_meta")
	if err != nil {
		t.Fatalf("ListByCategory() error = %v", err)
	}
	if len(settings) != 3 {
		t.Fatalf("ListByCategory() count = %v, want 3", len(settings))
	}

	// Create a map for easy lookup
	settingMap := make(map[string]string)
	for _, s := range settings {
		settingMap[s.Key] = s.ValueType
	}

	if settingMap["int_setting"] != "int" {
		t.Errorf("int_setting type = %v, want int", settingMap["int_setting"])
	}
	if settingMap["bool_setting"] != "bool" {
		t.Errorf("bool_setting type = %v, want bool", settingMap["bool_setting"])
	}
	if settingMap["duration_setting"] != "duration" {
		t.Errorf("duration_setting type = %v, want duration", settingMap["duration_setting"])
	}
}

func TestService_ListAll_MultipleCategories(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set settings in multiple categories
	categories := []string{"cat1", "cat2", "cat3"}
	for i, cat := range categories {
		err := svc.Set(ctx, cat, "key", "value", false)
		if err != nil {
			t.Fatalf("Set() error for category %d = %v", i, err)
		}
	}

	// List all and verify count
	settings, err := svc.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	if len(settings) != 3 {
		t.Errorf("ListAll() count = %v, want 3", len(settings))
	}

	// Verify all categories are present
	foundCats := make(map[string]bool)
	for _, s := range settings {
		foundCats[s.Category] = true
	}
	for _, cat := range categories {
		if !foundCats[cat] {
			t.Errorf("ListAll() missing category %s", cat)
		}
	}
}

func TestService_GetRequired_ErrorWrapping(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Get required setting that doesn't exist - verify error message format
	_, err := svc.GetRequired(ctx, "app", "config")
	if err == nil {
		t.Fatal("GetRequired() expected error for missing setting")
	}

	// Error should contain category.key format
	errMsg := err.Error()
	if !contains(errMsg, "app.config") {
		t.Errorf("GetRequired() error = %v, should contain app.config", errMsg)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
