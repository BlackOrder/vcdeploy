// Package config defines configuration structures for vcdeploy.
package config

import (
	"context"
	"errors"
	"os"
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
	tmpDir, err := os.MkdirTemp("", "settings_test")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("open database: %v", err)
	}

	// Run migrations
	if err := db.MigrateUp(context.Background()); err != nil {
		db.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("migrate database: %v", err)
	}

	// Initialize KMS
	kms, err := security.NewKMS(db.Conn(), nil)
	if err != nil {
		db.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("init KMS: %v", err)
	}

	// Initialize KMS (create first key)
	if err := kms.Initialize(context.Background()); err != nil {
		db.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("initialize KMS: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
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
	if svc.db != db {
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
