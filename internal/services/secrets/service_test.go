package secrets

import (
	"context"
	"testing"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/services/testutil"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

func newTestService(t *testing.T) (*Service, *storage.DB) {
	t.Helper()

	db, cleanup := testutil.NewTestDB(t)
	t.Cleanup(cleanup)

	kms, err := security.NewKMS(db.Conn(), nil)
	if err != nil {
		t.Fatalf("Failed to create KMS: %v", err)
	}

	ctx := context.Background()
	if err := kms.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize KMS: %v", err)
	}

	return New(db, kms), db
}

func TestService_Set_And_Get(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	err := svc.Set(ctx, "myproject", "production", "DATABASE_URL", "postgres://localhost/mydb")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	value, err := svc.Get(ctx, "myproject", "production", "DATABASE_URL")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if value != "postgres://localhost/mydb" {
		t.Errorf("Get() = %v, want %v", value, "postgres://localhost/mydb")
	}
}

func TestService_Get_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	value, err := svc.Get(ctx, "myproject", "production", "NONEXISTENT")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if value != "" {
		t.Errorf("Get() = %v, want empty string", value)
	}
}

func TestService_Set_UpdateExisting(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	err := svc.Set(ctx, "myproject", "production", "API_KEY", "old-key")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	err = svc.Set(ctx, "myproject", "production", "API_KEY", "new-key")
	if err != nil {
		t.Fatalf("Set() update error = %v", err)
	}

	value, err := svc.Get(ctx, "myproject", "production", "API_KEY")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if value != "new-key" {
		t.Errorf("Get() after update = %v, want %v", value, "new-key")
	}
}

func TestService_Set_InvalidKey(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	invalidKeys := []string{
		"",
		"with space",
		"1STARTS_NUM",
	}

	for _, key := range invalidKeys {
		err := svc.Set(ctx, "myproject", "production", key, "value")
		if err == nil {
			t.Errorf("Set() expected error for invalid key %q", key)
		}
	}
}

func TestService_Set_MissingProject(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	err := svc.Set(ctx, "", "production", "KEY", "value")
	if err == nil {
		t.Error("Set() expected error for empty project")
	}
}

func TestService_Set_MissingScope(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	err := svc.Set(ctx, "myproject", "", "KEY", "value")
	if err == nil {
		t.Error("Set() expected error for empty scope")
	}
}

func TestService_GetEntry(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	err := svc.Set(ctx, "myproject", "staging", "SECRET_KEY", "secret-value")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	entry, err := svc.GetEntry(ctx, "myproject", "staging", "SECRET_KEY")
	if err != nil {
		t.Fatalf("GetEntry() error = %v", err)
	}
	if entry == nil {
		t.Fatal("GetEntry() returned nil")
	}

	if entry.Project != "myproject" {
		t.Errorf("GetEntry() project = %v, want %v", entry.Project, "myproject")
	}
	if entry.Scope != "staging" {
		t.Errorf("GetEntry() scope = %v, want %v", entry.Scope, "staging")
	}
	if entry.Key != "SECRET_KEY" {
		t.Errorf("GetEntry() key = %v, want %v", entry.Key, "SECRET_KEY")
	}
	if entry.Value != "secret-value" {
		t.Errorf("GetEntry() value = %v, want %v", entry.Value, "secret-value")
	}
	if entry.ID == 0 {
		t.Error("GetEntry() ID should be set")
	}
	if entry.CreatedAt.IsZero() {
		t.Error("GetEntry() CreatedAt should be set")
	}
}

func TestService_GetEntry_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	entry, err := svc.GetEntry(ctx, "myproject", "production", "NONEXISTENT")
	if err == nil {
		t.Fatal("GetEntry() expected error for not found")
	}
	if !services.IsNotFound(err) {
		t.Errorf("GetEntry() expected ErrNotFound, got: %v", err)
	}
	if entry != nil {
		t.Error("GetEntry() expected nil entry for not found")
	}
}

func TestService_Delete(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	err := svc.Set(ctx, "myproject", "production", "TO_DELETE", "value")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	err = svc.Delete(ctx, "myproject", "production", "TO_DELETE")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	value, err := svc.Get(ctx, "myproject", "production", "TO_DELETE")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if value != "" {
		t.Errorf("Get() after delete = %v, want empty string", value)
	}
}

func TestService_List(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	keys := []string{"DB_HOST", "DB_PORT", "DB_NAME"}
	for _, key := range keys {
		err := svc.Set(ctx, "myproject", "production", key, "value-"+key)
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}
	}

	metadata, err := svc.List(ctx, "myproject", "production")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(metadata) != 3 {
		t.Errorf("List() count = %v, want 3", len(metadata))
	}

	for _, m := range metadata {
		if m.Project != "myproject" {
			t.Errorf("List() metadata project = %v, want %v", m.Project, "myproject")
		}
		if m.Scope != "production" {
			t.Errorf("List() metadata scope = %v, want %v", m.Scope, "production")
		}
	}
}

func TestService_List_DifferentScopes(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	err := svc.Set(ctx, "myproject", "production", "KEY1", "value1")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	err = svc.Set(ctx, "myproject", "staging", "KEY2", "value2")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	metadata, err := svc.List(ctx, "myproject", "production")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(metadata) != 1 {
		t.Errorf("List() production count = %v, want 1", len(metadata))
	}
}

func TestService_ListByProject(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	err := svc.Set(ctx, "myproject", "production", "KEY1", "value1")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	err = svc.Set(ctx, "myproject", "staging", "KEY2", "value2")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	err = svc.Set(ctx, "myproject", "development", "KEY3", "value3")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	metadata, err := svc.ListByProject(ctx, "myproject")
	if err != nil {
		t.Fatalf("ListByProject() error = %v", err)
	}
	if len(metadata) != 3 {
		t.Errorf("ListByProject() count = %v, want 3", len(metadata))
	}
}

func TestService_ListAll(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	err := svc.Set(ctx, "project1", "production", "KEY1", "value1")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	err = svc.Set(ctx, "project2", "production", "KEY2", "value2")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	metadata, err := svc.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	if len(metadata) != 2 {
		t.Errorf("ListAll() count = %v, want 2", len(metadata))
	}
}

func TestService_Export(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	secrets := map[string]string{
		"DB_HOST":     "localhost",
		"DB_PORT":     "5432",
		"DB_PASSWORD": "secret123",
	}
	for key, value := range secrets {
		err := svc.Set(ctx, "myproject", "production", key, value)
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}
	}

	exported, err := svc.Export(ctx, "myproject", "production")
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if len(exported) != 3 {
		t.Errorf("Export() count = %v, want 3", len(exported))
	}

	for key, expectedValue := range secrets {
		if exported[key] != expectedValue {
			t.Errorf("Export() %s = %v, want %v", key, exported[key], expectedValue)
		}
	}
}

func TestValidateKey(t *testing.T) {
	validKeys := []string{
		"KEY",
		"MY_KEY",
		"DATABASE_URL",
		"KEY123",
		"PRIVATE_KEY",
	}

	for _, key := range validKeys {
		if err := ValidateKey(key); err != nil {
			t.Errorf("ValidateKey(%q) unexpected error = %v", key, err)
		}
	}
}

func TestValidateKey_Invalid(t *testing.T) {
	invalidKeys := []string{
		"",
		"123KEY",
		"my-key",
		"my key",
		"key.name",
	}

	for _, key := range invalidKeys {
		if err := ValidateKey(key); err == nil {
			t.Errorf("ValidateKey(%q) expected error", key)
		}
	}
}

func TestValidateKey_TooLong(t *testing.T) {
	// Create a key that exceeds 64 characters
	longKey := "A" + string(make([]byte, 64)) // 65 chars starting with A
	for i := 1; i < 65; i++ {
		longKey = "A" + longKey[:i]
	}
	longKey = ""
	for i := 0; i < 65; i++ {
		longKey += "A"
	}

	err := ValidateKey(longKey)
	if err == nil {
		t.Error("ValidateKey() expected error for key exceeding 64 characters")
	}
}

func TestService_ExportEnvFile(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set up test secrets
	err := svc.Set(ctx, "myproject", "production", "DB_HOST", "localhost")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	err = svc.Set(ctx, "myproject", "production", "DB_PORT", "5432")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	envFile, err := svc.ExportEnvFile(ctx, "myproject", "production")
	if err != nil {
		t.Fatalf("ExportEnvFile() error = %v", err)
	}

	if envFile == "" {
		t.Error("ExportEnvFile() returned empty string")
	}

	// Check that it contains expected format
	if !containsSubstring(envFile, "DB_HOST=") {
		t.Error("ExportEnvFile() should contain DB_HOST=")
	}
	if !containsSubstring(envFile, "DB_PORT=") {
		t.Error("ExportEnvFile() should contain DB_PORT=")
	}
}

func TestService_ExportEnvFile_SpecialCharacters(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set up a secret with special characters that need escaping
	err := svc.Set(ctx, "myproject", "production", "PASSWORD", "pass\"word\\with\nnewline")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	envFile, err := svc.ExportEnvFile(ctx, "myproject", "production")
	if err != nil {
		t.Fatalf("ExportEnvFile() error = %v", err)
	}

	// The value should be escaped
	if !containsSubstring(envFile, "PASSWORD=") {
		t.Error("ExportEnvFile() should contain PASSWORD=")
	}
	// Should have escaped the quote
	if !containsSubstring(envFile, "\\\"") {
		t.Error("ExportEnvFile() should escape double quotes")
	}
}

func TestService_ExportEnvFile_Empty(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	envFile, err := svc.ExportEnvFile(ctx, "myproject", "production")
	if err != nil {
		t.Fatalf("ExportEnvFile() error = %v", err)
	}

	if envFile != "" {
		t.Errorf("ExportEnvFile() for empty scope = %v, want empty string", envFile)
	}
}

func TestService_Import(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	secrets := map[string]string{
		"API_KEY":     "key123",
		"API_SECRET":  "secret456",
		"DB_PASSWORD": "dbpass",
	}

	err := svc.Import(ctx, "myproject", "production", secrets)
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}

	// Verify all secrets were imported
	for key, expectedValue := range secrets {
		value, err := svc.Get(ctx, "myproject", "production", key)
		if err != nil {
			t.Fatalf("Get(%s) error = %v", key, err)
		}
		if value != expectedValue {
			t.Errorf("Get(%s) = %v, want %v", key, value, expectedValue)
		}
	}
}

func TestService_Import_InvalidKey(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	secrets := map[string]string{
		"VALID_KEY":   "value",
		"invalid-key": "value", // invalid key format
	}

	err := svc.Import(ctx, "myproject", "production", secrets)
	if err == nil {
		t.Error("Import() expected error for invalid key")
	}
}

func TestService_Import_Empty(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	secrets := map[string]string{}

	err := svc.Import(ctx, "myproject", "production", secrets)
	if err != nil {
		t.Fatalf("Import() error = %v for empty map", err)
	}
}

func TestService_ReEncryptAll(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set up some secrets
	testSecrets := []struct {
		project, scope, key, value string
	}{
		{"project1", "production", "KEY1", "value1"},
		{"project1", "staging", "KEY2", "value2"},
		{"project2", "production", "KEY3", "value3"},
	}

	for _, s := range testSecrets {
		err := svc.Set(ctx, s.project, s.scope, s.key, s.value)
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}
	}

	// Re-encrypt all secrets
	err := svc.ReEncryptAll(ctx)
	if err != nil {
		t.Fatalf("ReEncryptAll() error = %v", err)
	}

	// Verify all secrets can still be decrypted correctly
	for _, s := range testSecrets {
		value, err := svc.Get(ctx, s.project, s.scope, s.key)
		if err != nil {
			t.Fatalf("Get() after ReEncryptAll error = %v", err)
		}
		if value != s.value {
			t.Errorf("Get(%s/%s/%s) after ReEncryptAll = %v, want %v", s.project, s.scope, s.key, value, s.value)
		}
	}
}

func TestService_ReEncryptAll_Empty(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Re-encrypt with no secrets should not error
	err := svc.ReEncryptAll(ctx)
	if err != nil {
		t.Fatalf("ReEncryptAll() error = %v for empty database", err)
	}
}

func TestService_Export_Empty(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	exported, err := svc.Export(ctx, "myproject", "production")
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if len(exported) != 0 {
		t.Errorf("Export() for empty scope = %v, want empty map", exported)
	}
}

func TestService_List_Empty(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	metadata, err := svc.List(ctx, "myproject", "production")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(metadata) != 0 {
		t.Errorf("List() for empty scope = %v, want empty slice", metadata)
	}
}

func TestService_ListByProject_Empty(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	metadata, err := svc.ListByProject(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("ListByProject() error = %v", err)
	}
	if len(metadata) != 0 {
		t.Errorf("ListByProject() for nonexistent project = %v, want empty slice", metadata)
	}
}

func TestService_ListAll_Empty(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	metadata, err := svc.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	if len(metadata) != 0 {
		t.Errorf("ListAll() for empty database = %v, want empty slice", metadata)
	}
}

func TestService_Delete_NonExistent(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Deleting a non-existent secret should not error
	err := svc.Delete(ctx, "myproject", "production", "NONEXISTENT")
	if err != nil {
		t.Fatalf("Delete() error = %v for non-existent secret", err)
	}
}

func TestService_ListByProject_DifferentProjects(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Set up secrets for different projects
	err := svc.Set(ctx, "project1", "production", "KEY1", "value1")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	err = svc.Set(ctx, "project2", "production", "KEY2", "value2")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// List for project1 should only return project1 secrets
	metadata, err := svc.ListByProject(ctx, "project1")
	if err != nil {
		t.Fatalf("ListByProject() error = %v", err)
	}
	if len(metadata) != 1 {
		t.Errorf("ListByProject(project1) count = %v, want 1", len(metadata))
	}
	if len(metadata) > 0 && metadata[0].Project != "project1" {
		t.Errorf("ListByProject() returned wrong project = %v", metadata[0].Project)
	}
}

func TestService_GetEntry_Timestamps(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	err := svc.Set(ctx, "myproject", "production", "TIMESTAMPED", "value")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	entry, err := svc.GetEntry(ctx, "myproject", "production", "TIMESTAMPED")
	if err != nil {
		t.Fatalf("GetEntry() error = %v", err)
	}
	if entry == nil {
		t.Fatal("GetEntry() returned nil")
	}

	if entry.UpdatedAt.IsZero() {
		t.Error("GetEntry() UpdatedAt should be set")
	}

	// UpdatedAt should be >= CreatedAt
	if entry.UpdatedAt.Before(entry.CreatedAt) {
		t.Error("GetEntry() UpdatedAt should be >= CreatedAt")
	}
}

func TestService_Set_EmptyValue(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Setting an empty value should be allowed
	err := svc.Set(ctx, "myproject", "production", "EMPTY_VAL", "")
	if err != nil {
		t.Fatalf("Set() error = %v for empty value", err)
	}

	value, err := svc.Get(ctx, "myproject", "production", "EMPTY_VAL")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if value != "" {
		t.Errorf("Get() = %v, want empty string", value)
	}
}

func TestService_Set_LargeValue(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create a large value (10KB)
	largeValue := make([]byte, 10*1024)
	for i := range largeValue {
		largeValue[i] = byte('a' + (i % 26))
	}

	err := svc.Set(ctx, "myproject", "production", "LARGE_VAL", string(largeValue))
	if err != nil {
		t.Fatalf("Set() error = %v for large value", err)
	}

	value, err := svc.Get(ctx, "myproject", "production", "LARGE_VAL")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if value != string(largeValue) {
		t.Error("Get() returned different value for large secret")
	}
}

func TestService_MultipleScopes(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	scopes := []string{"development", "staging", "production"}

	// Set the same key in different scopes with different values
	for _, scope := range scopes {
		err := svc.Set(ctx, "myproject", scope, "CONFIG", "value-"+scope)
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}
	}

	// Verify each scope has its own value
	for _, scope := range scopes {
		value, err := svc.Get(ctx, "myproject", scope, "CONFIG")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		expected := "value-" + scope
		if value != expected {
			t.Errorf("Get(%s) = %v, want %v", scope, value, expected)
		}
	}

	// List for project should return all 3
	metadata, err := svc.ListByProject(ctx, "myproject")
	if err != nil {
		t.Fatalf("ListByProject() error = %v", err)
	}
	if len(metadata) != 3 {
		t.Errorf("ListByProject() count = %v, want 3", len(metadata))
	}
}

func TestService_List_MetadataFields(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	err := svc.Set(ctx, "myproject", "production", "TEST_KEY", "test-value")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	metadata, err := svc.List(ctx, "myproject", "production")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(metadata) != 1 {
		t.Fatalf("List() count = %v, want 1", len(metadata))
	}

	m := metadata[0]
	if m.ID == 0 {
		t.Error("List() metadata ID should be set")
	}
	if m.Project != "myproject" {
		t.Errorf("List() metadata Project = %v, want myproject", m.Project)
	}
	if m.Scope != "production" {
		t.Errorf("List() metadata Scope = %v, want production", m.Scope)
	}
	if m.Key != "TEST_KEY" {
		t.Errorf("List() metadata Key = %v, want TEST_KEY", m.Key)
	}
	if m.CreatedAt.IsZero() {
		t.Error("List() metadata CreatedAt should be set")
	}
	if m.UpdatedAt.IsZero() {
		t.Error("List() metadata UpdatedAt should be set")
	}
}

func TestNew(t *testing.T) {
	db, cleanup := testutil.NewTestDB(t)
	defer cleanup()

	kms, err := security.NewKMS(db.Conn(), nil)
	if err != nil {
		t.Fatalf("Failed to create KMS: %v", err)
	}

	ctx := context.Background()
	if err := kms.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize KMS: %v", err)
	}

	svc := New(db, kms)
	if svc == nil {
		t.Fatal("New() returned nil")
	}
	if svc.store != db {
		t.Error("New() did not set db correctly")
	}
	if svc.kms != kms {
		t.Error("New() did not set kms correctly")
	}
}

// Helper function for string contains check
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (substr == "" || findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
