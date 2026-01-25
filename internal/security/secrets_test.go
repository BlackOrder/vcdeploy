package security

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// setupTestSecretService creates a SecretService with test DB and KMS.
func setupTestSecretService(t *testing.T) (*SecretService, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create storage DB
	db, err := storage.New(dbPath, nil)
	if err != nil {
		t.Fatalf("storage.New() error: %v", err)
	}

	// Create KMS with its own connection for encryption keys table
	kmsDB, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL")
	if err != nil {
		db.Close()
		t.Fatalf("sql.Open() error: %v", err)
	}

	// Create encryption_keys table (KMS requirement)
	_, err = kmsDB.Exec(`
		CREATE TABLE IF NOT EXISTS encryption_keys (
			id TEXT PRIMARY KEY,
			version INTEGER NOT NULL,
			key_material_encrypted BLOB NOT NULL,
			algorithm TEXT NOT NULL DEFAULT 'AES-256-GCM',
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			activated_at DATETIME,
			deactivated_at DATETIME,
			scheduled_deletion_at DATETIME,
			deletion_cancelled_at DATETIME,
			UNIQUE(version)
		);
		CREATE TABLE IF NOT EXISTS encryption_key_usage (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key_id TEXT NOT NULL,
			operation TEXT NOT NULL,
			resource_type TEXT,
			resource_id TEXT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		kmsDB.Close()
		db.Close()
		t.Fatalf("create tables: %v", err)
	}

	kms, err := NewKMS(kmsDB, nil)
	if err != nil {
		kmsDB.Close()
		db.Close()
		t.Fatalf("NewKMS() error: %v", err)
	}

	ctx := context.Background()
	if err := kms.Initialize(ctx); err != nil {
		kmsDB.Close()
		db.Close()
		t.Fatalf("kms.Initialize() error: %v", err)
	}

	svc := NewSecretService(db, kms)

	cleanup := func() {
		kmsDB.Close()
		db.Close()
	}

	return svc, cleanup
}

func TestNewSecretService(t *testing.T) {
	svc, cleanup := setupTestSecretService(t)
	defer cleanup()

	if svc == nil {
		t.Fatal("NewSecretService() returned nil")
	}
}

func TestSecretServiceSetAndGet(t *testing.T) {
	svc, cleanup := setupTestSecretService(t)
	defer cleanup()

	ctx := context.Background()

	// Set a secret
	err := svc.Set(ctx, "myproject", "production", "DATABASE_URL", "postgres://localhost/mydb")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Get the secret
	secret, err := svc.Get(ctx, "myproject", "production", "DATABASE_URL")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if secret == nil {
		t.Fatal("Get() returned nil")
	}

	if secret.Key != "DATABASE_URL" {
		t.Errorf("secret.Key = %v, want DATABASE_URL", secret.Key)
	}

	if secret.Value != "postgres://localhost/mydb" {
		t.Errorf("secret.Value = %v, want postgres://localhost/mydb", secret.Value)
	}

	if secret.Project != "myproject" {
		t.Errorf("secret.Project = %v, want myproject", secret.Project)
	}

	if secret.Scope != "production" {
		t.Errorf("secret.Scope = %v, want production", secret.Scope)
	}
}

func TestSecretServiceGetNonExistent(t *testing.T) {
	svc, cleanup := setupTestSecretService(t)
	defer cleanup()

	ctx := context.Background()

	// Get non-existent secret
	secret, err := svc.Get(ctx, "myproject", "production", "NONEXISTENT")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if secret != nil {
		t.Errorf("Get() returned %v, want nil for non-existent secret", secret)
	}
}

func TestSecretServiceUpdate(t *testing.T) {
	svc, cleanup := setupTestSecretService(t)
	defer cleanup()

	ctx := context.Background()

	// Set initial secret
	err := svc.Set(ctx, "myproject", "production", "API_KEY", "old-key")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Update the secret
	err = svc.Set(ctx, "myproject", "production", "API_KEY", "new-key")
	if err != nil {
		t.Fatalf("Set() update error = %v", err)
	}

	// Verify update
	secret, err := svc.Get(ctx, "myproject", "production", "API_KEY")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if secret.Value != "new-key" {
		t.Errorf("secret.Value = %v, want new-key", secret.Value)
	}
}

func TestSecretServiceDelete(t *testing.T) {
	svc, cleanup := setupTestSecretService(t)
	defer cleanup()

	ctx := context.Background()

	// Set a secret
	err := svc.Set(ctx, "myproject", "staging", "SECRET_TOKEN", "token123")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Verify it exists
	secret, err := svc.Get(ctx, "myproject", "staging", "SECRET_TOKEN")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if secret == nil {
		t.Fatal("secret should exist before delete")
	}

	// Delete the secret
	err = svc.Delete(ctx, "myproject", "staging", "SECRET_TOKEN")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify it's gone
	secret, err = svc.Get(ctx, "myproject", "staging", "SECRET_TOKEN")
	if err != nil {
		t.Fatalf("Get() after delete error = %v", err)
	}
	if secret != nil {
		t.Error("secret should be nil after delete")
	}
}

func TestSecretServiceList(t *testing.T) {
	svc, cleanup := setupTestSecretService(t)
	defer cleanup()

	ctx := context.Background()

	// Set multiple secrets
	secrets := map[string]string{
		"DATABASE_URL": "postgres://localhost/db",
		"REDIS_URL":    "redis://localhost:6379",
		"API_KEY":      "secret-api-key",
	}

	for key, value := range secrets {
		if err := svc.Set(ctx, "testproject", "prod", key, value); err != nil {
			t.Fatalf("Set(%s) error = %v", key, err)
		}
	}

	// List secrets
	list, err := svc.List(ctx, "testproject", "prod")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(list) != 3 {
		t.Errorf("List() returned %d secrets, want 3", len(list))
	}

	// Verify metadata (no values exposed)
	for _, meta := range list {
		if meta.Project != "testproject" {
			t.Errorf("meta.Project = %v, want testproject", meta.Project)
		}
		if meta.Scope != "prod" {
			t.Errorf("meta.Scope = %v, want prod", meta.Scope)
		}
		if _, exists := secrets[meta.Key]; !exists {
			t.Errorf("unexpected key in list: %s", meta.Key)
		}
	}
}

func TestSecretServiceExport(t *testing.T) {
	svc, cleanup := setupTestSecretService(t)
	defer cleanup()

	ctx := context.Background()

	// Set secrets
	expected := map[string]string{
		"DB_HOST":     "localhost",
		"DB_PORT":     "5432",
		"DB_PASSWORD": "secret123",
	}

	for key, value := range expected {
		if err := svc.Set(ctx, "exporttest", "dev", key, value); err != nil {
			t.Fatalf("Set(%s) error = %v", key, err)
		}
	}

	// Export
	exported, err := svc.Export(ctx, "exporttest", "dev")
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	if len(exported) != len(expected) {
		t.Errorf("Export() returned %d secrets, want %d", len(exported), len(expected))
	}

	for key, expectedValue := range expected {
		if exported[key] != expectedValue {
			t.Errorf("exported[%s] = %v, want %v", key, exported[key], expectedValue)
		}
	}
}

func TestSecretServiceImport(t *testing.T) {
	svc, cleanup := setupTestSecretService(t)
	defer cleanup()

	ctx := context.Background()

	// Import secrets
	toImport := map[string]string{
		"IMPORTED_KEY1": "value1",
		"IMPORTED_KEY2": "value2",
	}

	err := svc.Import(ctx, "importtest", "staging", toImport)
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}

	// Verify import
	for key, expectedValue := range toImport {
		secret, err := svc.Get(ctx, "importtest", "staging", key)
		if err != nil {
			t.Fatalf("Get(%s) error = %v", key, err)
		}
		if secret == nil {
			t.Errorf("secret %s not found after import", key)
			continue
		}
		if secret.Value != expectedValue {
			t.Errorf("secret.Value = %v, want %v", secret.Value, expectedValue)
		}
	}
}

func TestSecretServiceSetValidation(t *testing.T) {
	svc, cleanup := setupTestSecretService(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name    string
		project string
		scope   string
		key     string
		wantErr bool
	}{
		{"valid", "myproject", "prod", "VALID_KEY", false},
		{"empty project", "", "prod", "KEY", true},
		{"empty scope", "project", "", "KEY", true},
		{"invalid key lowercase", "project", "prod", "lowercase", true},
		{"invalid key starts with number", "project", "prod", "123KEY", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.Set(ctx, tt.project, tt.scope, tt.key, "value")
			if (err != nil) != tt.wantErr {
				t.Errorf("Set() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSecretServiceExportEnvFile(t *testing.T) {
	svc, cleanup := setupTestSecretService(t)
	defer cleanup()

	ctx := context.Background()

	// Set secrets with special characters
	if err := svc.Set(ctx, "envtest", "test", "SIMPLE_KEY", "simplevalue"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := svc.Set(ctx, "envtest", "test", "QUOTED_VALUE", "value with spaces"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Export as env file
	envFile, err := svc.ExportEnvFile(ctx, "envtest", "test")
	if err != nil {
		t.Fatalf("ExportEnvFile() error = %v", err)
	}

	if envFile == "" {
		t.Error("ExportEnvFile() returned empty string")
	}

	// Should contain proper formatting
	if len(envFile) == 0 {
		t.Error("env file should not be empty")
	}
}

func TestValidateSecretKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"valid uppercase", "DATABASE_URL", false},
		{"valid single char", "A", false},
		{"valid with numbers", "API_KEY_123", false},
		{"empty", "", true},
		{"starts with number", "123_KEY", true},
		{"lowercase", "database_url", true},
		{"has spaces", "MY KEY", true},
		{"has hyphen", "MY-KEY", true},
		{"too long", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSecretKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSecretKey(%q) error = %v, wantErr %v", tt.key, err, tt.wantErr)
			}
		})
	}
}

func TestValidateProjectName(t *testing.T) {
	tests := []struct {
		name    string
		project string
		wantErr bool
	}{
		{"valid lowercase", "myproject", false},
		{"valid with hyphen", "my-project", false},
		{"valid with underscore", "my_project", false},
		{"valid with numbers", "project123", false},
		{"valid mixed", "My-Project_123", false},
		{"empty", "", true},
		{"starts with number", "123project", true},
		{"starts with hyphen", "-project", true},
		{"has spaces", "my project", true},
		{"too long", string(make([]byte, 65)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProjectName(tt.project)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateProjectName(%q) error = %v, wantErr %v", tt.project, err, tt.wantErr)
			}
		})
	}
}

func TestSecretServiceReEncryptAll(t *testing.T) {
	svc, cleanup := setupTestSecretService(t)
	defer cleanup()

	ctx := context.Background()

	// Set some secrets
	err := svc.Set(ctx, "project1", "production", "API_KEY", "secret-value-1")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	err = svc.Set(ctx, "project2", "staging", "DB_PASSWORD", "secret-value-2")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Re-encrypt all secrets
	err = svc.ReEncryptAll(ctx)
	if err != nil {
		t.Fatalf("ReEncryptAll() error = %v", err)
	}

	// Verify secrets are still accessible
	entry, err := svc.Get(ctx, "project1", "production", "API_KEY")
	if err != nil {
		t.Fatalf("Get() after ReEncryptAll error = %v", err)
	}
	if entry.Value != "secret-value-1" {
		t.Errorf("Get() after ReEncryptAll = %v, want %v", entry.Value, "secret-value-1")
	}
}
