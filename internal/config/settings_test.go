// Package config defines configuration structures for vcdeploy.
package config

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// setupTestSettingsDB creates a temporary database for testing
func setupTestSettingsDB(t *testing.T) (*storage.DB, *security.KMS, func()) {
	t.Helper()

	// Create temp directory
	tmpDir := t.TempDir()

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	// Run migrations
	if err := db.MigrateUp(context.Background()); err != nil {
		db.Close()
		t.Fatalf("migrate database: %v", err)
	}

	// Initialize KMS
	kms, err := security.NewKMS(db.Conn(), nil)
	if err != nil {
		db.Close()
		t.Fatalf("init KMS: %v", err)
	}

	// Initialize KMS (create first key)
	if err := kms.Initialize(context.Background()); err != nil {
		db.Close()
		t.Fatalf("initialize KMS: %v", err)
	}

	cleanup := func() {
		db.Close()
	}

	return db, kms, cleanup
}

func TestNewSettingsService(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if svc.store != db {
		t.Error("db not set correctly")
	}
	if svc.kms != kms {
		t.Error("kms not set correctly")
	}
}

func TestSettingsService_SetAndGet(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Test basic set and get
	if err := svc.Set(ctx, "test", "key1", "value1", false); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	val, err := svc.Get(ctx, "test", "key1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "value1" {
		t.Errorf("expected 'value1', got '%s'", val)
	}
}

func TestSettingsService_SetAndGetEncrypted(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Test encrypted set and get
	secret := "super-secret-value"
	if err := svc.Set(ctx, "secrets", "password", secret, true); err != nil {
		t.Fatalf("Set encrypted failed: %v", err)
	}

	val, err := svc.Get(ctx, "secrets", "password")
	if err != nil {
		t.Fatalf("Get encrypted failed: %v", err)
	}
	if val != secret {
		t.Errorf("expected '%s', got '%s'", secret, val)
	}
}

func TestSettingsService_GetNonExistent(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	val, err := svc.Get(ctx, "nonexistent", "key")
	if err != nil {
		t.Fatalf("Get should not error for non-existent: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string for non-existent, got '%s'", val)
	}
}

func TestSettingsService_GetRequired(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Set a value
	if err := svc.Set(ctx, "test", "required", "required-value", false); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Test GetRequired with existing value
	val, err := svc.GetRequired(ctx, "test", "required")
	if err != nil {
		t.Fatalf("GetRequired failed: %v", err)
	}
	if val != "required-value" {
		t.Errorf("expected 'required-value', got '%s'", val)
	}

	// Test GetRequired with non-existent value
	_, err = svc.GetRequired(ctx, "test", "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent required setting")
	}
	if !errors.Is(err, ErrSettingRequired) {
		t.Errorf("expected ErrSettingRequired, got %v", err)
	}
}

func TestSettingsService_GetRequiredInt(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Set an integer
	if err := svc.SetInt(ctx, "test", "port", 8080); err != nil {
		t.Fatalf("SetInt failed: %v", err)
	}

	// Test GetRequiredInt
	val, err := svc.GetRequiredInt(ctx, "test", "port")
	if err != nil {
		t.Fatalf("GetRequiredInt failed: %v", err)
	}
	if val != 8080 {
		t.Errorf("expected 8080, got %d", val)
	}

	// Test with non-existent
	_, err = svc.GetRequiredInt(ctx, "test", "nonexistent")
	if !errors.Is(err, ErrSettingRequired) {
		t.Errorf("expected ErrSettingRequired, got %v", err)
	}

	// Test with invalid integer
	if err := svc.Set(ctx, "test", "invalid_int", "not-a-number", false); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	_, err = svc.GetRequiredInt(ctx, "test", "invalid_int")
	if err == nil {
		t.Fatal("expected error for invalid integer")
	}
}

func TestSettingsService_GetRequiredBool(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Set a boolean
	if err := svc.SetBool(ctx, "test", "enabled", true); err != nil {
		t.Fatalf("SetBool failed: %v", err)
	}

	// Test GetRequiredBool
	val, err := svc.GetRequiredBool(ctx, "test", "enabled")
	if err != nil {
		t.Fatalf("GetRequiredBool failed: %v", err)
	}
	if !val {
		t.Errorf("expected true, got false")
	}

	// Test with non-existent
	_, err = svc.GetRequiredBool(ctx, "test", "nonexistent")
	if !errors.Is(err, ErrSettingRequired) {
		t.Errorf("expected ErrSettingRequired, got %v", err)
	}

	// Test with invalid boolean
	if err := svc.Set(ctx, "test", "invalid_bool", "not-a-bool", false); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	_, err = svc.GetRequiredBool(ctx, "test", "invalid_bool")
	if err == nil {
		t.Fatal("expected error for invalid boolean")
	}
}

func TestSettingsService_GetRequiredDuration(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Set a duration
	if err := svc.SetDuration(ctx, "test", "timeout", 5*time.Minute); err != nil {
		t.Fatalf("SetDuration failed: %v", err)
	}

	// Test GetRequiredDuration
	val, err := svc.GetRequiredDuration(ctx, "test", "timeout")
	if err != nil {
		t.Fatalf("GetRequiredDuration failed: %v", err)
	}
	if val != 5*time.Minute {
		t.Errorf("expected 5m, got %s", val)
	}

	// Test with non-existent
	_, err = svc.GetRequiredDuration(ctx, "test", "nonexistent")
	if !errors.Is(err, ErrSettingRequired) {
		t.Errorf("expected ErrSettingRequired, got %v", err)
	}

	// Test with invalid duration
	if err := svc.Set(ctx, "test", "invalid_duration", "not-a-duration", false); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	_, err = svc.GetRequiredDuration(ctx, "test", "invalid_duration")
	if err == nil {
		t.Fatal("expected error for invalid duration")
	}
}

func TestSettingsService_GetWithDefaults(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Test GetString with default
	val := svc.GetString(ctx, "test", "nonexistent", "default")
	if val != "default" {
		t.Errorf("expected 'default', got '%s'", val)
	}

	// Test GetInt with default
	intVal := svc.GetInt(ctx, "test", "nonexistent", 42)
	if intVal != 42 {
		t.Errorf("expected 42, got %d", intVal)
	}

	// Test GetBool with default
	boolVal := svc.GetBool(ctx, "test", "nonexistent", true)
	if !boolVal {
		t.Error("expected true, got false")
	}

	// Test GetDuration with default
	durVal := svc.GetDuration(ctx, "test", "nonexistent", 10*time.Second)
	if durVal != 10*time.Second {
		t.Errorf("expected 10s, got %s", durVal)
	}
}

func TestSettingsService_SetTypes(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Test SetInt
	if err := svc.SetInt(ctx, "test", "int", 123); err != nil {
		t.Fatalf("SetInt failed: %v", err)
	}
	if val := svc.GetInt(ctx, "test", "int", 0); val != 123 {
		t.Errorf("expected 123, got %d", val)
	}

	// Test SetBool
	if err := svc.SetBool(ctx, "test", "bool", false); err != nil {
		t.Fatalf("SetBool failed: %v", err)
	}
	if val := svc.GetBool(ctx, "test", "bool", true); val {
		t.Error("expected false, got true")
	}

	// Test SetDuration
	if err := svc.SetDuration(ctx, "test", "dur", 1*time.Hour); err != nil {
		t.Fatalf("SetDuration failed: %v", err)
	}
	if val := svc.GetDuration(ctx, "test", "dur", 0); val != 1*time.Hour {
		t.Errorf("expected 1h, got %s", val)
	}
}

func TestSettingsService_Delete(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Set a value
	if err := svc.Set(ctx, "test", "delete_me", "value", false); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Verify it exists
	val, err := svc.Get(ctx, "test", "delete_me")
	if err != nil || val != "value" {
		t.Fatal("value should exist before delete")
	}

	// Delete it
	if err := svc.Delete(ctx, "test", "delete_me"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify it's gone
	val, err = svc.Get(ctx, "test", "delete_me")
	if err != nil {
		t.Fatalf("Get after delete failed: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string after delete, got '%s'", val)
	}
}

func TestSettingsService_GetCategory(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Set multiple values in a category
	if err := svc.Set(ctx, "category", "key1", "value1", false); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := svc.Set(ctx, "category", "key2", "value2", false); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := svc.Set(ctx, "other", "key3", "value3", false); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get category
	result, err := svc.GetCategory(ctx, "category")
	if err != nil {
		t.Fatalf("GetCategory failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 items, got %d", len(result))
	}
	if result["key1"] != "value1" {
		t.Errorf("expected 'value1', got '%s'", result["key1"])
	}
	if result["key2"] != "value2" {
		t.Errorf("expected 'value2', got '%s'", result["key2"])
	}
}

func TestSettingsService_SetDefaults(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Set defaults
	if err := svc.SetDefaults(ctx); err != nil {
		t.Fatalf("SetDefaults failed: %v", err)
	}

	// Verify some defaults are set
	listen, err := svc.Get(ctx, "server", "listen")
	if err != nil {
		t.Fatalf("Get server.listen failed: %v", err)
	}
	if listen == "" {
		t.Error("expected server.listen to be set")
	}

	// Default TLS is enabled (from DefaultMasterConfig)
	tlsEnabled := svc.GetBool(ctx, "server", "tls_enabled", false)
	if !tlsEnabled {
		t.Error("expected tls_enabled to be true by default")
	}
}

func TestSettingsService_GetMasterConfig(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Set some custom values
	if err := svc.Set(ctx, "server", "listen", ":9999", false); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := svc.SetBool(ctx, "server", "tls_enabled", true); err != nil {
		t.Fatalf("SetBool failed: %v", err)
	}

	// Get config
	cfg, err := svc.GetMasterConfig(ctx)
	if err != nil {
		t.Fatalf("GetMasterConfig failed: %v", err)
	}

	if cfg.Server.Listen != ":9999" {
		t.Errorf("expected ':9999', got '%s'", cfg.Server.Listen)
	}
	if !cfg.Server.TLS.Enabled {
		t.Error("expected TLS enabled")
	}
}

func TestSettingsService_IsInitialized(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Initially not initialized (no settings)
	initialized, err := svc.IsInitialized(ctx)
	if err != nil {
		t.Fatalf("IsInitialized failed: %v", err)
	}
	if initialized {
		t.Error("expected not initialized initially")
	}

	// Set defaults
	if err := svc.SetDefaults(ctx); err != nil {
		t.Fatalf("SetDefaults failed: %v", err)
	}

	// Now should be initialized
	initialized, err = svc.IsInitialized(ctx)
	if err != nil {
		t.Fatalf("IsInitialized failed: %v", err)
	}
	if !initialized {
		t.Error("expected initialized after SetDefaults")
	}
}

func TestSettingsService_GetDefaultsWithInvalidValues(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Set invalid values
	if err := svc.Set(ctx, "test", "bad_int", "not_an_int", false); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := svc.Set(ctx, "test", "bad_bool", "not_a_bool", false); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := svc.Set(ctx, "test", "bad_duration", "not_a_duration", false); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// GetInt with invalid value should return default
	intVal := svc.GetInt(ctx, "test", "bad_int", 99)
	if intVal != 99 {
		t.Errorf("expected default 99, got %d", intVal)
	}

	// GetBool with invalid value should return default
	boolVal := svc.GetBool(ctx, "test", "bad_bool", true)
	if !boolVal {
		t.Error("expected default true")
	}

	// GetDuration with invalid value should return default
	durVal := svc.GetDuration(ctx, "test", "bad_duration", 30*time.Second)
	if durVal != 30*time.Second {
		t.Errorf("expected default 30s, got %s", durVal)
	}
}

func TestSettingsService_NilKMS(t *testing.T) {
	db, _, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	// Create service without KMS
	svc := NewSettingsService(db, nil)
	ctx := context.Background()

	// Should still work for non-encrypted values
	if err := svc.Set(ctx, "test", "key", "value", false); err != nil {
		t.Fatalf("Set without KMS failed: %v", err)
	}

	val, err := svc.Get(ctx, "test", "key")
	if err != nil {
		t.Fatalf("Get without KMS failed: %v", err)
	}
	if val != "value" {
		t.Errorf("expected 'value', got '%s'", val)
	}
}

func TestSettingsService_ExportImport(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Set some values
	if err := svc.Set(ctx, "server", "listen", ":8080", false); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := svc.SetBool(ctx, "server", "tls_enabled", true); err != nil {
		t.Fatalf("SetBool failed: %v", err)
	}

	// Export as YAML
	yamlData, err := svc.Export(ctx)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	if len(yamlData) == 0 {
		t.Error("expected non-empty YAML export")
	}

	// Export as JSON
	jsonData, err := svc.ExportJSON(ctx)
	if err != nil {
		t.Fatalf("ExportJSON failed: %v", err)
	}
	if len(jsonData) == 0 {
		t.Error("expected non-empty JSON export")
	}
}

func TestSettingsService_Import(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Import settings from YAML
	yamlData := []byte(`
server:
  listen: ":9000"
  tls_enabled: true
database:
  max_connections: 100
`)

	if err := svc.Import(ctx, yamlData); err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Verify imported values
	listen, err := svc.Get(ctx, "server", "listen")
	if err != nil {
		t.Fatalf("Get server/listen failed: %v", err)
	}
	if listen != ":9000" {
		t.Errorf("expected ':9000', got '%s'", listen)
	}

	maxConn := svc.GetInt(ctx, "database", "max_connections", 0)
	if maxConn != 100 {
		t.Errorf("expected 100, got %d", maxConn)
	}
}

func TestSettingsService_ImportInvalidYAML(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Import invalid YAML
	invalidYAML := []byte(`{invalid yaml`)

	err := svc.Import(ctx, invalidYAML)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestSettingsService_ImportBoolAndString(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Import settings with different types
	yamlData := []byte(`
features:
  enabled: true
  name: "test-feature"
  count: 42
`)

	if err := svc.Import(ctx, yamlData); err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Verify bool was imported
	enabled := svc.GetBool(ctx, "features", "enabled", false)
	if !enabled {
		t.Error("expected enabled to be true")
	}

	// Verify string was imported
	name, err := svc.Get(ctx, "features", "name")
	if err != nil {
		t.Fatalf("Get features/name failed: %v", err)
	}
	if name != "test-feature" {
		t.Errorf("expected 'test-feature', got '%s'", name)
	}
}

func TestSettingsService_UpdateOverwrite(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Set initial value
	if err := svc.Set(ctx, "test", "key", "initial", false); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Overwrite with new value
	if err := svc.Set(ctx, "test", "key", "updated", false); err != nil {
		t.Fatalf("Set (update) failed: %v", err)
	}

	// Verify updated value
	val, err := svc.Get(ctx, "test", "key")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "updated" {
		t.Errorf("expected 'updated', got '%s'", val)
	}
}

func TestSettingsService_SetRaw(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	tests := []struct {
		name      string
		category  string
		key       string
		value     string
		valueType string
		encrypted bool
	}{
		{
			name:      "raw string",
			category:  "test",
			key:       "string_val",
			value:     "test-value",
			valueType: "string",
			encrypted: false,
		},
		{
			name:      "raw int",
			category:  "test",
			key:       "int_val",
			value:     "42",
			valueType: "int",
			encrypted: false,
		},
		{
			name:      "raw bool",
			category:  "test",
			key:       "bool_val",
			value:     "true",
			valueType: "bool",
			encrypted: false,
		},
		{
			name:      "raw duration",
			category:  "test",
			key:       "duration_val",
			value:     "5m",
			valueType: "duration",
			encrypted: false,
		},
		{
			name:      "encrypted value",
			category:  "secrets",
			key:       "api_key",
			value:     "secret-key",
			valueType: "string",
			encrypted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := svc.SetRaw(ctx, tt.category, tt.key, tt.value, tt.valueType, tt.encrypted); err != nil {
				t.Fatalf("SetRaw failed: %v", err)
			}

			val, err := svc.Get(ctx, tt.category, tt.key)
			if err != nil {
				t.Fatalf("Get failed: %v", err)
			}
			if val != tt.value {
				t.Errorf("expected '%s', got '%s'", tt.value, val)
			}
		})
	}
}

func TestSettingsService_ListByCategory(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Set up test data in multiple categories
	if err := svc.Set(ctx, "category_a", "key1", "value1", false); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := svc.Set(ctx, "category_a", "key2", "value2", false); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := svc.Set(ctx, "category_b", "key3", "value3", false); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// List by category
	settings, err := svc.ListByCategory(ctx, "category_a")
	if err != nil {
		t.Fatalf("ListByCategory failed: %v", err)
	}

	if len(settings) != 2 {
		t.Errorf("expected 2 settings in category_a, got %d", len(settings))
	}

	// Verify metadata fields are populated
	for _, s := range settings {
		if s.Category != "category_a" {
			t.Errorf("expected category 'category_a', got '%s'", s.Category)
		}
		if s.Key == "" {
			t.Error("expected non-empty key")
		}
	}
}

func TestSettingsService_ListAll(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Set up test data
	if err := svc.Set(ctx, "cat1", "key1", "val1", false); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := svc.SetInt(ctx, "cat2", "key2", 123); err != nil {
		t.Fatalf("SetInt failed: %v", err)
	}
	if err := svc.SetBool(ctx, "cat3", "key3", true); err != nil {
		t.Fatalf("SetBool failed: %v", err)
	}

	// List all settings
	settings, err := svc.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll failed: %v", err)
	}

	if len(settings) < 3 {
		t.Errorf("expected at least 3 settings, got %d", len(settings))
	}

	// Verify metadata
	for _, s := range settings {
		if s.Category == "" {
			t.Error("expected non-empty category")
		}
		if s.Key == "" {
			t.Error("expected non-empty key")
		}
	}
}

func TestSettingsService_GetCategoryWithEncrypted(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Set normal and encrypted values in same category
	if err := svc.Set(ctx, "mixed", "normal", "plain-value", false); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := svc.Set(ctx, "mixed", "secret", "encrypted-value", true); err != nil {
		t.Fatalf("Set encrypted failed: %v", err)
	}

	// Get entire category
	result, err := svc.GetCategory(ctx, "mixed")
	if err != nil {
		t.Fatalf("GetCategory failed: %v", err)
	}

	if result["normal"] != "plain-value" {
		t.Errorf("expected 'plain-value', got '%s'", result["normal"])
	}
	if result["secret"] != "encrypted-value" {
		t.Errorf("expected 'encrypted-value', got '%s'", result["secret"])
	}
}

func TestSettingsService_SetDefaultsAllCategories(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Set all defaults
	if err := svc.SetDefaults(ctx); err != nil {
		t.Fatalf("SetDefaults failed: %v", err)
	}

	// Verify various categories are set
	categories := []struct {
		category string
		key      string
	}{
		{"server", "listen"},
		{"server", "tls_enabled"},
		{"grpc", "listen"},
		{"ssh", "default_user"},
		{"security", "session_timeout"},
		{"backup", "db_enabled"},
		{"logs", "app_level"},
		{"webhooks", "github_enabled"},
		{"notifications", "slack_enabled"},
		{"api", "enabled"},
		{"appearance", "theme"},
	}

	for _, c := range categories {
		val, err := svc.Get(ctx, c.category, c.key)
		if err != nil {
			t.Errorf("Get %s/%s failed: %v", c.category, c.key, err)
		}
		if val == "" {
			t.Errorf("expected %s/%s to be set", c.category, c.key)
		}
	}
}

func TestSettingsService_ExportWithTypes(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Set values of different types
	if err := svc.Set(ctx, "test", "str", "string-value", false); err != nil {
		t.Fatalf("Set string failed: %v", err)
	}
	if err := svc.SetInt(ctx, "test", "num", 42); err != nil {
		t.Fatalf("SetInt failed: %v", err)
	}
	if err := svc.SetBool(ctx, "test", "flag", true); err != nil {
		t.Fatalf("SetBool failed: %v", err)
	}
	if err := svc.SetDuration(ctx, "test", "dur", 5*time.Minute); err != nil {
		t.Fatalf("SetDuration failed: %v", err)
	}

	// Export YAML
	yamlData, err := svc.Export(ctx)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	yamlStr := string(yamlData)
	if yamlStr == "" {
		t.Error("expected non-empty YAML export")
	}

	// Export JSON
	jsonData, err := svc.ExportJSON(ctx)
	if err != nil {
		t.Fatalf("ExportJSON failed: %v", err)
	}

	jsonStr := string(jsonData)
	if jsonStr == "" {
		t.Error("expected non-empty JSON export")
	}

	// Verify JSON contains expected structure
	if len(jsonStr) < 10 {
		t.Error("JSON export too short")
	}
}

func TestSettingsService_ExportWithEncryptedValues(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Set encrypted value
	if err := svc.Set(ctx, "secrets", "password", "my-secret-password", true); err != nil {
		t.Fatalf("Set encrypted failed: %v", err)
	}

	// Export should decrypt values
	yamlData, err := svc.Export(ctx)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	yamlStr := string(yamlData)
	if yamlStr == "" {
		t.Error("expected non-empty YAML export")
	}
}

func TestSettingsService_ImportWithAllTypes(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Import settings with various types
	yamlData := []byte(`
settings:
  string_val: "hello"
  int_val: 123
  float_val: 45.6
  bool_true: true
  bool_false: false
  int64_val: 9223372036854775807
`)

	if err := svc.Import(ctx, yamlData); err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Verify string
	strVal, _ := svc.Get(ctx, "settings", "string_val")
	if strVal != "hello" {
		t.Errorf("expected 'hello', got '%s'", strVal)
	}

	// Verify int
	intVal := svc.GetInt(ctx, "settings", "int_val", 0)
	if intVal != 123 {
		t.Errorf("expected 123, got %d", intVal)
	}

	// Verify bool true
	boolTrue := svc.GetBool(ctx, "settings", "bool_true", false)
	if !boolTrue {
		t.Error("expected bool_true to be true")
	}

	// Verify bool false
	boolFalse := svc.GetBool(ctx, "settings", "bool_false", true)
	if boolFalse {
		t.Error("expected bool_false to be false")
	}
}

func TestSettingsService_ImportSecurityEncryption(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Import security settings that should be encrypted
	yamlData := []byte(`
security:
  master_key: "super-secret-key"
  smtp_password: "email-password"
  normal_setting: "not-encrypted"
`)

	if err := svc.Import(ctx, yamlData); err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Verify values are retrievable (decrypted)
	masterKey, _ := svc.Get(ctx, "security", "master_key")
	if masterKey != "super-secret-key" {
		t.Errorf("expected 'super-secret-key', got '%s'", masterKey)
	}

	smtpPass, _ := svc.Get(ctx, "security", "smtp_password")
	if smtpPass != "email-password" {
		t.Errorf("expected 'email-password', got '%s'", smtpPass)
	}
}

func TestSettingsService_SetRawWithoutKMS(t *testing.T) {
	db, _, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	// Create service without KMS
	svc := NewSettingsService(db, nil)
	ctx := context.Background()

	// SetRaw with encrypted=true but no KMS should still work
	// (value won't actually be encrypted)
	if err := svc.SetRaw(ctx, "test", "key", "value", "string", true); err != nil {
		t.Fatalf("SetRaw without KMS failed: %v", err)
	}

	val, err := svc.Get(ctx, "test", "key")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "value" {
		t.Errorf("expected 'value', got '%s'", val)
	}
}

func TestSettingsService_GetCategoryEmpty(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// Get category that doesn't exist
	result, err := svc.GetCategory(ctx, "nonexistent_category")
	if err != nil {
		t.Fatalf("GetCategory failed: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("expected empty map for nonexistent category, got %d items", len(result))
	}
}

func TestSettingsService_ListByCategoryEmpty(t *testing.T) {
	db, kms, cleanup := setupTestSettingsDB(t)
	defer cleanup()

	svc := NewSettingsService(db, kms)
	ctx := context.Background()

	// List category that doesn't exist
	settings, err := svc.ListByCategory(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("ListByCategory failed: %v", err)
	}

	if len(settings) != 0 {
		t.Errorf("expected empty list for nonexistent category, got %d", len(settings))
	}
}
