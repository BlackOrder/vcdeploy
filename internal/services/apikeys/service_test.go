package apikeys

import (
	"context"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services/testutil"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

func newTestService(t *testing.T) (*Service, *storage.DB) {
	t.Helper()

	db, cleanup := testutil.NewTestDB(t)
	t.Cleanup(cleanup)

	return New(db), db
}

func createTestUser(t *testing.T, db *storage.DB) int64 {
	t.Helper()
	ctx := context.Background()

	user := &storage.User{
		Username:     "testuser",
		PasswordHash: "testhash",
		Email:        "test@example.com",
		Role:         "user",
	}
	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	return user.ID
}

func TestService_Create(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	userID := createTestUser(t, db)
	scopes := []string{"read", "write"}

	rawKey, key, err := svc.Create(ctx, userID, "Test Key", scopes, nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if rawKey == "" {
		t.Error("Create() did not return raw key")
	}
	if !hasPrefix(rawKey, "vcd_") {
		t.Errorf("Create() raw key should start with 'vcd_', got %v", rawKey[:10])
	}
	if key.ID == 0 {
		t.Error("Create() did not set key ID")
	}
	if key.Name != "Test Key" {
		t.Errorf("Create() name = %v, want %v", key.Name, "Test Key")
	}
	if key.UserID != userID {
		t.Errorf("Create() userID = %v, want %v", key.UserID, userID)
	}
	if key.KeyPrefix != rawKey[:8] {
		t.Errorf("Create() keyPrefix = %v, want %v", key.KeyPrefix, rawKey[:8])
	}
}

func TestService_Create_WithExpiration(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	userID := createTestUser(t, db)
	expires := time.Now().Add(24 * time.Hour)

	_, key, err := svc.Create(ctx, userID, "Expiring Key", nil, &expires)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if key.ExpiresAt == nil {
		t.Error("Create() did not set expiration")
	}
}

func TestService_GetByRawKey(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	userID := createTestUser(t, db)

	rawKey, created, err := svc.Create(ctx, userID, "Test Key", nil, nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Retrieve by raw key
	key, err := svc.GetByRawKey(ctx, rawKey)
	if err != nil {
		t.Fatalf("GetByRawKey() error = %v", err)
	}
	if key == nil {
		t.Fatal("GetByRawKey() returned nil")
	}
	if key.ID != created.ID {
		t.Errorf("GetByRawKey() ID = %v, want %v", key.ID, created.ID)
	}
}

func TestService_GetByRawKey_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.GetByRawKey(ctx, "vcd_nonexistent_key")
	if err == nil {
		t.Error("GetByRawKey() expected error for nonexistent key")
	}
}

func TestService_GetByRawKey_Expired(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	userID := createTestUser(t, db)

	// Create an expired key
	expires := time.Now().Add(-1 * time.Hour) // Already expired
	rawKey, _, err := svc.Create(ctx, userID, "Expired Key", nil, &expires)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Should not find expired key
	_, err = svc.GetByRawKey(ctx, rawKey)
	if err == nil {
		t.Error("GetByRawKey() expected error for expired key")
	}
}

func TestService_Delete(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	userID := createTestUser(t, db)

	rawKey, key, err := svc.Create(ctx, userID, "Test Key", nil, nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Delete the key
	if err := svc.Delete(ctx, key.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify it's gone
	_, err = svc.GetByRawKey(ctx, rawKey)
	if err == nil {
		t.Error("GetByRawKey() expected error after delete")
	}
}

func TestService_List(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	userID := createTestUser(t, db)

	// Create multiple keys
	for i := 0; i < 3; i++ {
		_, _, err := svc.Create(ctx, userID, "Key", nil, nil)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	// List keys
	keys, err := svc.List(ctx, userID)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("List() count = %v, want 3", len(keys))
	}

	// All keys should belong to the user
	for _, k := range keys {
		if k.UserID != userID {
			t.Errorf("List() key userID = %v, want %v", k.UserID, userID)
		}
	}
}

func TestService_List_Empty(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	userID := createTestUser(t, db)

	keys, err := svc.List(ctx, userID)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("List() count = %v, want 0", len(keys))
	}
}

func TestService_UpdateUsage(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	userID := createTestUser(t, db)

	rawKey, key, err := svc.Create(ctx, userID, "Test Key", nil, nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Initially LastUsedAt should be nil
	if key.LastUsedAt != nil {
		t.Error("Create() LastUsedAt should be nil initially")
	}

	// Update usage
	if err := svc.UpdateUsage(ctx, key.ID); err != nil {
		t.Fatalf("UpdateUsage() error = %v", err)
	}

	// Get key and verify LastUsedAt is set
	updated, err := svc.GetByRawKey(ctx, rawKey)
	if err != nil {
		t.Fatalf("GetByRawKey() error = %v", err)
	}
	if updated.LastUsedAt == nil {
		t.Error("UpdateUsage() should have set LastUsedAt")
	}
}

func TestService_CleanupExpired(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	userID := createTestUser(t, db)

	// Create an expired key
	expired := time.Now().Add(-1 * time.Hour)
	_, _, err := svc.Create(ctx, userID, "Expired Key", nil, &expired)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Create a valid key
	_, _, err = svc.Create(ctx, userID, "Valid Key", nil, nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Cleanup expired keys
	count, err := svc.CleanupExpired(ctx)
	if err != nil {
		t.Fatalf("CleanupExpired() error = %v", err)
	}
	if count != 1 {
		t.Errorf("CleanupExpired() count = %v, want 1", count)
	}

	// Verify only one key remains
	keys, err := svc.List(ctx, userID)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("List() count = %v, want 1", len(keys))
	}
}

func TestGetScopes(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	userID := createTestUser(t, db)
	scopes := []string{"read", "write", "admin"}

	rawKey, _, err := svc.Create(ctx, userID, "Test Key", scopes, nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	key, err := svc.GetByRawKey(ctx, rawKey)
	if err != nil {
		t.Fatalf("GetByRawKey() error = %v", err)
	}

	parsed, err := GetScopes(key)
	if err != nil {
		t.Fatalf("GetScopes() error = %v", err)
	}
	if len(parsed) != 3 {
		t.Errorf("GetScopes() count = %v, want 3", len(parsed))
	}
}

func TestGetScopes_Empty(t *testing.T) {
	key := &storage.APIKey{Scopes: ""}

	scopes, err := GetScopes(key)
	if err != nil {
		t.Fatalf("GetScopes() error = %v", err)
	}
	if scopes != nil {
		t.Errorf("GetScopes() = %v, want nil", scopes)
	}
}

// hasPrefix checks if a string starts with a given prefix.
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
