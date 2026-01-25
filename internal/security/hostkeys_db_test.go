package security

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"os"
	"path/filepath"
	"testing"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"golang.org/x/crypto/ssh"
)

// setupTestDB creates a test database and returns a cleanup function.
func setupTestDB(t *testing.T) (*storage.DB, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "vcdeploy-hostkeys-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := storage.New(dbPath, nil)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create database: %v", err)
	}

	return db, func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}
}

// generateTestSSHKey generates an RSA key pair for testing.
func generateTestSSHKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}
	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("Failed to create SSH public key: %v", err)
	}
	return publicKey
}

func TestDBHostKeyStore_StoreAndRetrieve(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewDBHostKeyStore(db)
	ctx := context.Background()

	pubKey := generateTestSSHKey(t)

	// Store a key
	key := &StoredHostKey{
		Hostname:    "test.example.com",
		Port:        22,
		KeyType:     pubKey.Type(),
		PublicKey:   pubKey.Marshal(),
		Fingerprint: FingerprintSHA256(pubKey),
		Trusted:     false,
		AddedBy:     "test",
	}

	err := store.StoreHostKey(ctx, key)
	if err != nil {
		t.Fatalf("StoreHostKey() error = %v", err)
	}

	// Retrieve the key
	retrieved, err := store.GetHostKey(ctx, "test.example.com", 22, pubKey.Type())
	if err != nil {
		t.Fatalf("GetHostKey() error = %v", err)
	}

	if retrieved.Hostname != key.Hostname {
		t.Errorf("Hostname = %v, want %v", retrieved.Hostname, key.Hostname)
	}
	if retrieved.Port != key.Port {
		t.Errorf("Port = %v, want %v", retrieved.Port, key.Port)
	}
	if retrieved.Fingerprint != key.Fingerprint {
		t.Errorf("Fingerprint = %v, want %v", retrieved.Fingerprint, key.Fingerprint)
	}
	if retrieved.Trusted {
		t.Error("Key should not be trusted initially")
	}
}

func TestDBHostKeyStore_GetHostKey_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewDBHostKeyStore(db)
	ctx := context.Background()

	_, err := store.GetHostKey(ctx, "nonexistent.example.com", 22, "ssh-rsa")
	if err == nil {
		t.Error("GetHostKey() should return error for nonexistent key")
	}
	if err != ErrHostKeyUnknown {
		t.Errorf("GetHostKey() error = %v, want ErrHostKeyUnknown", err)
	}
}

func TestDBHostKeyStore_TrustHostKey(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewDBHostKeyStore(db)
	ctx := context.Background()

	pubKey := generateTestSSHKey(t)

	// Store an untrusted key
	key := &StoredHostKey{
		Hostname:    "trust.example.com",
		Port:        22,
		KeyType:     pubKey.Type(),
		PublicKey:   pubKey.Marshal(),
		Fingerprint: FingerprintSHA256(pubKey),
		Trusted:     false,
		AddedBy:     "auto",
	}

	if err := store.StoreHostKey(ctx, key); err != nil {
		t.Fatalf("StoreHostKey() error = %v", err)
	}

	// Trust the key
	if err := store.TrustHostKey(ctx, "trust.example.com", 22, pubKey.Type(), "admin"); err != nil {
		t.Fatalf("TrustHostKey() error = %v", err)
	}

	// Verify it's now trusted
	retrieved, err := store.GetHostKey(ctx, "trust.example.com", 22, pubKey.Type())
	if err != nil {
		t.Fatalf("GetHostKey() error = %v", err)
	}

	if !retrieved.Trusted {
		t.Error("Key should be trusted after TrustHostKey()")
	}
}

func TestDBHostKeyStore_TrustHostKey_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewDBHostKeyStore(db)
	ctx := context.Background()

	err := store.TrustHostKey(ctx, "nonexistent.example.com", 22, "ssh-rsa", "admin")
	if err == nil {
		t.Error("TrustHostKey() should return error for nonexistent key")
	}
	if err != ErrHostKeyUnknown {
		t.Errorf("TrustHostKey() error = %v, want ErrHostKeyUnknown", err)
	}
}

func TestDBHostKeyStore_DeleteHostKey(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewDBHostKeyStore(db)
	ctx := context.Background()

	pubKey := generateTestSSHKey(t)

	// Store a key
	key := &StoredHostKey{
		Hostname:    "delete.example.com",
		Port:        22,
		KeyType:     pubKey.Type(),
		PublicKey:   pubKey.Marshal(),
		Fingerprint: FingerprintSHA256(pubKey),
		Trusted:     true,
		AddedBy:     "test",
	}

	if err := store.StoreHostKey(ctx, key); err != nil {
		t.Fatalf("StoreHostKey() error = %v", err)
	}

	// Delete the key
	if err := store.DeleteHostKey(ctx, "delete.example.com", 22, pubKey.Type()); err != nil {
		t.Fatalf("DeleteHostKey() error = %v", err)
	}

	// Verify it's gone
	_, err := store.GetHostKey(ctx, "delete.example.com", 22, pubKey.Type())
	if err != ErrHostKeyUnknown {
		t.Errorf("GetHostKey() after delete error = %v, want ErrHostKeyUnknown", err)
	}
}

func TestDBHostKeyStore_DeleteHostKey_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewDBHostKeyStore(db)
	ctx := context.Background()

	err := store.DeleteHostKey(ctx, "nonexistent.example.com", 22, "ssh-rsa")
	if err == nil {
		t.Error("DeleteHostKey() should return error for nonexistent key")
	}
	if err != ErrHostKeyUnknown {
		t.Errorf("DeleteHostKey() error = %v, want ErrHostKeyUnknown", err)
	}
}

func TestDBHostKeyStore_GetHostKeys(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewDBHostKeyStore(db)
	ctx := context.Background()

	pubKey1 := generateTestSSHKey(t)
	pubKey2 := generateTestSSHKey(t)

	// Store multiple keys for the same host (different key types simulated)
	key1 := &StoredHostKey{
		Hostname:    "multi.example.com",
		Port:        22,
		KeyType:     "ssh-rsa",
		PublicKey:   pubKey1.Marshal(),
		Fingerprint: FingerprintSHA256(pubKey1),
		AddedBy:     "test",
	}

	key2 := &StoredHostKey{
		Hostname:    "multi.example.com",
		Port:        22,
		KeyType:     "ssh-ed25519",
		PublicKey:   pubKey2.Marshal(),
		Fingerprint: FingerprintSHA256(pubKey2),
		AddedBy:     "test",
	}

	if err := store.StoreHostKey(ctx, key1); err != nil {
		t.Fatalf("StoreHostKey(key1) error = %v", err)
	}
	if err := store.StoreHostKey(ctx, key2); err != nil {
		t.Fatalf("StoreHostKey(key2) error = %v", err)
	}

	// Get all keys for the host
	keys, err := store.GetHostKeys(ctx, "multi.example.com", 22)
	if err != nil {
		t.Fatalf("GetHostKeys() error = %v", err)
	}

	if len(keys) != 2 {
		t.Errorf("GetHostKeys() returned %d keys, want 2", len(keys))
	}
}

func TestDBHostKeyStore_ListAllKeys(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewDBHostKeyStore(db)
	ctx := context.Background()

	pubKey1 := generateTestSSHKey(t)
	pubKey2 := generateTestSSHKey(t)

	// Store keys for different hosts
	key1 := &StoredHostKey{
		Hostname:    "host1.example.com",
		Port:        22,
		KeyType:     pubKey1.Type(),
		PublicKey:   pubKey1.Marshal(),
		Fingerprint: FingerprintSHA256(pubKey1),
		AddedBy:     "test",
	}

	key2 := &StoredHostKey{
		Hostname:    "host2.example.com",
		Port:        2222,
		KeyType:     pubKey2.Type(),
		PublicKey:   pubKey2.Marshal(),
		Fingerprint: FingerprintSHA256(pubKey2),
		AddedBy:     "test",
	}

	if err := store.StoreHostKey(ctx, key1); err != nil {
		t.Fatalf("StoreHostKey(key1) error = %v", err)
	}
	if err := store.StoreHostKey(ctx, key2); err != nil {
		t.Fatalf("StoreHostKey(key2) error = %v", err)
	}

	// List all keys
	keys, err := store.ListAllKeys(ctx)
	if err != nil {
		t.Fatalf("ListAllKeys() error = %v", err)
	}

	if len(keys) != 2 {
		t.Errorf("ListAllKeys() returned %d keys, want 2", len(keys))
	}

	// Verify hosts are different
	hosts := make(map[string]bool)
	for _, k := range keys {
		hosts[k.Hostname] = true
	}
	if !hosts["host1.example.com"] || !hosts["host2.example.com"] {
		t.Error("ListAllKeys() should return keys for both hosts")
	}
}

func TestDBHostKeyStore_VerifierIntegration(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewDBHostKeyStore(db)
	verifier := NewHostKeyVerifier(store)
	verifier.RequireTrust = false // For this test, don't require trust

	ctx := context.Background()

	pubKey := generateTestSSHKey(t)

	// Store a trusted key
	key := &StoredHostKey{
		Hostname:    "verified.example.com",
		Port:        22,
		KeyType:     pubKey.Type(),
		PublicKey:   pubKey.Marshal(),
		Fingerprint: FingerprintSHA256(pubKey),
		Trusted:     true,
		AddedBy:     "admin",
	}

	if err := store.StoreHostKey(ctx, key); err != nil {
		t.Fatalf("StoreHostKey() error = %v", err)
	}

	// Verify the key via the verifier
	err := verifier.VerifyHostKey(ctx, "verified.example.com:22", nil, pubKey)
	if err != nil {
		t.Errorf("VerifyHostKey() error = %v, want nil", err)
	}
}

func TestDBHostKeyStore_VerifierIntegration_Mismatch(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := NewDBHostKeyStore(db)
	verifier := NewHostKeyVerifier(store)

	ctx := context.Background()

	pubKey1 := generateTestSSHKey(t)
	pubKey2 := generateTestSSHKey(t)

	// Store key1
	key := &StoredHostKey{
		Hostname:    "mismatch.example.com",
		Port:        22,
		KeyType:     pubKey1.Type(),
		PublicKey:   pubKey1.Marshal(),
		Fingerprint: FingerprintSHA256(pubKey1),
		Trusted:     true,
		AddedBy:     "admin",
	}

	if err := store.StoreHostKey(ctx, key); err != nil {
		t.Fatalf("StoreHostKey() error = %v", err)
	}

	// Try to verify with a different key - should fail
	err := verifier.VerifyHostKey(ctx, "mismatch.example.com:22", nil, pubKey2)
	if err == nil {
		t.Error("VerifyHostKey() should fail for mismatched key")
	}
}
