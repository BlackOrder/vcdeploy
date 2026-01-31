package hostkeys

import (
	"context"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services/testutil"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

func newTestService(t *testing.T) (*Service, storage.Store) {
	t.Helper()

	db, cleanup := testutil.NewTestStore(t)
	t.Cleanup(cleanup)

	return New(db), db
}

func createTestHostKey(t *testing.T, hostname string, port int, keyType string) *storage.SSHHostKey {
	t.Helper()
	return &storage.SSHHostKey{
		Hostname:    hostname,
		Port:        port,
		KeyType:     keyType,
		PublicKey:   "AAAAB3NzaC1yc2EAAAADAQABAAABgQC7...",
		Fingerprint: "SHA256:abc123def456",
		Trusted:     false,
		AddedBy:     "test",
	}
}

// --- New() Tests ---

func TestNew(t *testing.T) {
	svc, db := newTestService(t)

	if svc == nil {
		t.Fatal("New() returned nil")
	}
	if svc.store != db {
		t.Error("New() did not set db correctly")
	}
}

// --- Create() Tests ---

func TestService_Create(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	key := createTestHostKey(t, "example.com", 22, "ssh-rsa")

	err := svc.Create(ctx, key)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if key.ID == 0 {
		t.Error("Create() did not set key ID")
	}
	if key.CreatedAt.IsZero() {
		t.Error("Create() did not set CreatedAt")
	}
}

func TestService_Create_SetsCreatedAtIfZero(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	key := createTestHostKey(t, "example.com", 22, "ssh-rsa")
	key.CreatedAt = time.Time{} // Zero value

	err := svc.Create(ctx, key)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if key.CreatedAt.IsZero() {
		t.Error("Create() should set CreatedAt when zero")
	}
}

func TestService_Create_PreservesCreatedAtIfSet(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	key := createTestHostKey(t, "example.com", 22, "ssh-rsa")
	customTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	key.CreatedAt = customTime

	err := svc.Create(ctx, key)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if !key.CreatedAt.Equal(customTime) {
		t.Errorf("Create() CreatedAt = %v, want %v", key.CreatedAt, customTime)
	}
}

func TestService_Create_DuplicateKey(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	key1 := createTestHostKey(t, "example.com", 22, "ssh-rsa")
	if err := svc.Create(ctx, key1); err != nil {
		t.Fatalf("Create() first key error = %v", err)
	}

	// Try to create a duplicate
	key2 := createTestHostKey(t, "example.com", 22, "ssh-rsa")
	err := svc.Create(ctx, key2)
	if err == nil {
		t.Error("Create() expected error for duplicate key")
	}
}

// --- Get() Tests ---

func TestService_Get(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create a host key first
	key := createTestHostKey(t, "example.com", 22, "ssh-rsa")
	if err := svc.Create(ctx, key); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Retrieve the key
	retrieved, err := svc.Get(ctx, "example.com", 22, "ssh-rsa")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if retrieved == nil {
		t.Fatal("Get() returned nil")
	}
	if retrieved.ID != key.ID {
		t.Errorf("Get() ID = %v, want %v", retrieved.ID, key.ID)
	}
	if retrieved.Hostname != "example.com" {
		t.Errorf("Get() Hostname = %v, want %v", retrieved.Hostname, "example.com")
	}
	if retrieved.Port != 22 {
		t.Errorf("Get() Port = %v, want %v", retrieved.Port, 22)
	}
	if retrieved.KeyType != "ssh-rsa" {
		t.Errorf("Get() KeyType = %v, want %v", retrieved.KeyType, "ssh-rsa")
	}
}

func TestService_Get_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.Get(ctx, "nonexistent.com", 22, "ssh-rsa")
	if err == nil {
		t.Error("Get() expected error for nonexistent key")
	}
	if err != storage.ErrNotFound {
		t.Errorf("Get() error = %v, want %v", err, storage.ErrNotFound)
	}
}

func TestService_Get_DifferentPort(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create key on port 22
	key := createTestHostKey(t, "example.com", 22, "ssh-rsa")
	if err := svc.Create(ctx, key); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Try to get key on different port
	_, err := svc.Get(ctx, "example.com", 2222, "ssh-rsa")
	if err == nil {
		t.Error("Get() expected error for different port")
	}
}

func TestService_Get_DifferentKeyType(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create RSA key
	key := createTestHostKey(t, "example.com", 22, "ssh-rsa")
	if err := svc.Create(ctx, key); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Try to get ED25519 key
	_, err := svc.Get(ctx, "example.com", 22, "ssh-ed25519")
	if err == nil {
		t.Error("Get() expected error for different key type")
	}
}

// --- GetByHost() Tests ---

func TestService_GetByHost(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create multiple keys for the same host
	key1 := createTestHostKey(t, "example.com", 22, "ssh-rsa")
	if err := svc.Create(ctx, key1); err != nil {
		t.Fatalf("Create() key1 error = %v", err)
	}

	key2 := createTestHostKey(t, "example.com", 22, "ssh-ed25519")
	key2.Fingerprint = "SHA256:xyz789" // Different fingerprint
	if err := svc.Create(ctx, key2); err != nil {
		t.Fatalf("Create() key2 error = %v", err)
	}

	// Create key for different host
	key3 := createTestHostKey(t, "other.com", 22, "ssh-rsa")
	key3.Fingerprint = "SHA256:other123"
	if err := svc.Create(ctx, key3); err != nil {
		t.Fatalf("Create() key3 error = %v", err)
	}

	// Get keys for example.com
	keys, err := svc.GetByHost(ctx, "example.com", 22)
	if err != nil {
		t.Fatalf("GetByHost() error = %v", err)
	}

	if len(keys) != 2 {
		t.Errorf("GetByHost() returned %d keys, want 2", len(keys))
	}
}

func TestService_GetByHost_NoKeys(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	keys, err := svc.GetByHost(ctx, "nonexistent.com", 22)
	if err != nil {
		t.Fatalf("GetByHost() error = %v", err)
	}

	if len(keys) != 0 {
		t.Errorf("GetByHost() returned %d keys, want 0", len(keys))
	}
}

func TestService_GetByHost_DifferentPorts(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create keys on different ports
	key1 := createTestHostKey(t, "example.com", 22, "ssh-rsa")
	if err := svc.Create(ctx, key1); err != nil {
		t.Fatalf("Create() key1 error = %v", err)
	}

	key2 := createTestHostKey(t, "example.com", 2222, "ssh-rsa")
	key2.Fingerprint = "SHA256:port2222"
	if err := svc.Create(ctx, key2); err != nil {
		t.Fatalf("Create() key2 error = %v", err)
	}

	// Get keys for port 22 only
	keys, err := svc.GetByHost(ctx, "example.com", 22)
	if err != nil {
		t.Fatalf("GetByHost() error = %v", err)
	}

	if len(keys) != 1 {
		t.Errorf("GetByHost() returned %d keys, want 1", len(keys))
	}
}

// --- List() Tests ---

func TestService_List(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create multiple keys
	for i := 0; i < 5; i++ {
		key := createTestHostKey(t, "host"+string(rune('a'+i))+".com", 22, "ssh-rsa")
		key.Fingerprint = "SHA256:host" + string(rune('a'+i))
		if err := svc.Create(ctx, key); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	keys, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(keys) != 5 {
		t.Errorf("List() returned %d keys, want 5", len(keys))
	}
}

func TestService_List_Empty(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	keys, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(keys) != 0 {
		t.Errorf("List() returned %d keys, want 0", len(keys))
	}
}

// --- UpdateTrust() Tests ---

func TestService_UpdateTrust(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create a key
	key := createTestHostKey(t, "example.com", 22, "ssh-rsa")
	key.Trusted = false
	if err := svc.Create(ctx, key); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Update trust to true
	err := svc.UpdateTrust(ctx, key.ID, true, "admin")
	if err != nil {
		t.Fatalf("UpdateTrust() error = %v", err)
	}

	// Verify the update
	retrieved, err := svc.Get(ctx, "example.com", 22, "ssh-rsa")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if !retrieved.Trusted {
		t.Error("UpdateTrust() did not set Trusted to true")
	}
}

func TestService_UpdateTrust_ToUntrusted(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create a trusted key
	key := createTestHostKey(t, "example.com", 22, "ssh-rsa")
	key.Trusted = true
	if err := svc.Create(ctx, key); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Update trust to false
	err := svc.UpdateTrust(ctx, key.ID, false, "admin")
	if err != nil {
		t.Fatalf("UpdateTrust() error = %v", err)
	}

	// Verify the update
	retrieved, err := svc.Get(ctx, "example.com", 22, "ssh-rsa")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if retrieved.Trusted {
		t.Error("UpdateTrust() did not set Trusted to false")
	}
}

func TestService_UpdateTrust_NonexistentKey(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// This should not return an error (SQLite update affects 0 rows)
	err := svc.UpdateTrust(ctx, 99999, true, "admin")
	if err != nil {
		t.Fatalf("UpdateTrust() error = %v (update on non-existent ID should not error)", err)
	}
}

// --- Delete() Tests ---

func TestService_Delete(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create a key
	key := createTestHostKey(t, "example.com", 22, "ssh-rsa")
	if err := svc.Create(ctx, key); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Delete the key
	err := svc.Delete(ctx, key.ID)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deletion
	_, err = svc.Get(ctx, "example.com", 22, "ssh-rsa")
	if err == nil {
		t.Error("Get() expected error after delete")
	}
	if err != storage.ErrNotFound {
		t.Errorf("Get() error = %v, want %v", err, storage.ErrNotFound)
	}
}

func TestService_Delete_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	err := svc.Delete(ctx, 99999)
	if err == nil {
		t.Error("Delete() expected error for nonexistent key")
	}
}

// --- DeleteByHost() Tests ---

func TestService_DeleteByHost(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create multiple keys for the same host
	key1 := createTestHostKey(t, "example.com", 22, "ssh-rsa")
	if err := svc.Create(ctx, key1); err != nil {
		t.Fatalf("Create() key1 error = %v", err)
	}

	key2 := createTestHostKey(t, "example.com", 22, "ssh-ed25519")
	key2.Fingerprint = "SHA256:ed25519fp"
	if err := svc.Create(ctx, key2); err != nil {
		t.Fatalf("Create() key2 error = %v", err)
	}

	// Create key for different host (should not be deleted)
	key3 := createTestHostKey(t, "other.com", 22, "ssh-rsa")
	key3.Fingerprint = "SHA256:otherfp"
	if err := svc.Create(ctx, key3); err != nil {
		t.Fatalf("Create() key3 error = %v", err)
	}

	// Delete all keys for example.com
	count, err := svc.DeleteByHost(ctx, "example.com", 22)
	if err != nil {
		t.Fatalf("DeleteByHost() error = %v", err)
	}

	if count != 2 {
		t.Errorf("DeleteByHost() deleted %d keys, want 2", count)
	}

	// Verify deletion
	keys, err := svc.GetByHost(ctx, "example.com", 22)
	if err != nil {
		t.Fatalf("GetByHost() error = %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("GetByHost() returned %d keys after delete, want 0", len(keys))
	}

	// Verify other.com key still exists
	otherKeys, err := svc.GetByHost(ctx, "other.com", 22)
	if err != nil {
		t.Fatalf("GetByHost() error = %v", err)
	}
	if len(otherKeys) != 1 {
		t.Errorf("GetByHost() returned %d keys for other.com, want 1", len(otherKeys))
	}
}

func TestService_DeleteByHost_NoKeys(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	count, err := svc.DeleteByHost(ctx, "nonexistent.com", 22)
	if err != nil {
		t.Fatalf("DeleteByHost() error = %v", err)
	}

	if count != 0 {
		t.Errorf("DeleteByHost() deleted %d keys, want 0", count)
	}
}

func TestService_DeleteByHost_SpecificPort(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create keys on different ports
	key1 := createTestHostKey(t, "example.com", 22, "ssh-rsa")
	if err := svc.Create(ctx, key1); err != nil {
		t.Fatalf("Create() key1 error = %v", err)
	}

	key2 := createTestHostKey(t, "example.com", 2222, "ssh-rsa")
	key2.Fingerprint = "SHA256:port2222fp"
	if err := svc.Create(ctx, key2); err != nil {
		t.Fatalf("Create() key2 error = %v", err)
	}

	// Delete keys only for port 22
	count, err := svc.DeleteByHost(ctx, "example.com", 22)
	if err != nil {
		t.Fatalf("DeleteByHost() error = %v", err)
	}

	if count != 1 {
		t.Errorf("DeleteByHost() deleted %d keys, want 1", count)
	}

	// Verify port 2222 key still exists
	keys, err := svc.GetByHost(ctx, "example.com", 2222)
	if err != nil {
		t.Fatalf("GetByHost() error = %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("GetByHost() returned %d keys for port 2222, want 1", len(keys))
	}
}

// --- IsTrusted() Tests ---

func TestService_IsTrusted_TrustedWithMatchingFingerprint(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create a trusted key
	key := createTestHostKey(t, "example.com", 22, "ssh-rsa")
	key.Trusted = true
	key.Fingerprint = "SHA256:trusted123"
	if err := svc.Create(ctx, key); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	trusted, err := svc.IsTrusted(ctx, "example.com", 22, "ssh-rsa", "SHA256:trusted123")
	if err != nil {
		t.Fatalf("IsTrusted() error = %v", err)
	}

	if !trusted {
		t.Error("IsTrusted() = false, want true for trusted key with matching fingerprint")
	}
}

func TestService_IsTrusted_TrustedWithDifferentFingerprint(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create a trusted key
	key := createTestHostKey(t, "example.com", 22, "ssh-rsa")
	key.Trusted = true
	key.Fingerprint = "SHA256:trusted123"
	if err := svc.Create(ctx, key); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Check with different fingerprint (possible MITM)
	trusted, err := svc.IsTrusted(ctx, "example.com", 22, "ssh-rsa", "SHA256:different")
	if err != nil {
		t.Fatalf("IsTrusted() error = %v", err)
	}

	if trusted {
		t.Error("IsTrusted() = true, want false for mismatched fingerprint")
	}
}

func TestService_IsTrusted_UntrustedKey(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create an untrusted key
	key := createTestHostKey(t, "example.com", 22, "ssh-rsa")
	key.Trusted = false
	key.Fingerprint = "SHA256:untrusted123"
	if err := svc.Create(ctx, key); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	trusted, err := svc.IsTrusted(ctx, "example.com", 22, "ssh-rsa", "SHA256:untrusted123")
	if err != nil {
		t.Fatalf("IsTrusted() error = %v", err)
	}

	if trusted {
		t.Error("IsTrusted() = true, want false for untrusted key")
	}
}

func TestService_IsTrusted_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	trusted, err := svc.IsTrusted(ctx, "nonexistent.com", 22, "ssh-rsa", "SHA256:any")
	if err != nil {
		t.Fatalf("IsTrusted() error = %v", err)
	}

	if trusted {
		t.Error("IsTrusted() = true, want false for nonexistent key")
	}
}

func TestService_IsTrusted_DifferentKeyType(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create a trusted RSA key
	key := createTestHostKey(t, "example.com", 22, "ssh-rsa")
	key.Trusted = true
	key.Fingerprint = "SHA256:rsa123"
	if err := svc.Create(ctx, key); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Check ED25519 key type (should not be found)
	trusted, err := svc.IsTrusted(ctx, "example.com", 22, "ssh-ed25519", "SHA256:ed25519")
	if err != nil {
		t.Fatalf("IsTrusted() error = %v", err)
	}

	if trusted {
		t.Error("IsTrusted() = true, want false for different key type")
	}
}

// --- Integration Tests ---

func TestService_Integration_FullWorkflow(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// 1. Create a new host key
	key := createTestHostKey(t, "server.example.com", 22, "ssh-rsa")
	key.Fingerprint = "SHA256:workflow123"
	if err := svc.Create(ctx, key); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// 2. Verify key is not trusted initially
	trusted, err := svc.IsTrusted(ctx, "server.example.com", 22, "ssh-rsa", "SHA256:workflow123")
	if err != nil {
		t.Fatalf("IsTrusted() error = %v", err)
	}
	if trusted {
		t.Error("New key should not be trusted initially")
	}

	// 3. Update trust status
	if err := svc.UpdateTrust(ctx, key.ID, true, "admin"); err != nil {
		t.Fatalf("UpdateTrust() error = %v", err)
	}

	// 4. Verify key is now trusted
	trusted, err = svc.IsTrusted(ctx, "server.example.com", 22, "ssh-rsa", "SHA256:workflow123")
	if err != nil {
		t.Fatalf("IsTrusted() error = %v", err)
	}
	if !trusted {
		t.Error("Key should be trusted after UpdateTrust")
	}

	// 5. Verify key appears in list
	keys, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("List() returned %d keys, want 1", len(keys))
	}

	// 6. Delete the key
	if err := svc.Delete(ctx, key.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// 7. Verify key is gone
	_, err = svc.Get(ctx, "server.example.com", 22, "ssh-rsa")
	if err != storage.ErrNotFound {
		t.Errorf("Get() after delete error = %v, want %v", err, storage.ErrNotFound)
	}
}

func TestService_Integration_MultipleHosts(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	hosts := []struct {
		hostname string
		port     int
		keyType  string
	}{
		{"server1.example.com", 22, "ssh-rsa"},
		{"server1.example.com", 22, "ssh-ed25519"},
		{"server2.example.com", 22, "ssh-rsa"},
		{"server3.example.com", 2222, "ssh-rsa"},
	}

	// Create all keys
	for i, h := range hosts {
		key := createTestHostKey(t, h.hostname, h.port, h.keyType)
		key.Fingerprint = "SHA256:fp" + string(rune('a'+i))
		if err := svc.Create(ctx, key); err != nil {
			t.Fatalf("Create() error = %v for host %s", err, h.hostname)
		}
	}

	// Verify total count
	allKeys, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(allKeys) != 4 {
		t.Errorf("List() returned %d keys, want 4", len(allKeys))
	}

	// Verify server1 has 2 keys
	server1Keys, err := svc.GetByHost(ctx, "server1.example.com", 22)
	if err != nil {
		t.Fatalf("GetByHost() error = %v", err)
	}
	if len(server1Keys) != 2 {
		t.Errorf("GetByHost() for server1 returned %d keys, want 2", len(server1Keys))
	}

	// Delete all keys for server1
	deleted, err := svc.DeleteByHost(ctx, "server1.example.com", 22)
	if err != nil {
		t.Fatalf("DeleteByHost() error = %v", err)
	}
	if deleted != 2 {
		t.Errorf("DeleteByHost() deleted %d keys, want 2", deleted)
	}

	// Verify remaining keys
	remainingKeys, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(remainingKeys) != 2 {
		t.Errorf("List() after delete returned %d keys, want 2", len(remainingKeys))
	}
}
