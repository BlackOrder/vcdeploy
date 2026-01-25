package security

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	_ "modernc.org/sqlite"
)

func setupTestSSHDB(t *testing.T) (*sql.DB, *KMS) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	// Create required tables
	_, err = db.Exec(`
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
		CREATE TABLE IF NOT EXISTS ssh_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			public_key TEXT NOT NULL,
			private_key_encrypted BLOB NOT NULL,
			key_type TEXT NOT NULL DEFAULT 'ed25519',
			fingerprint TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_used_at DATETIME
		);
		CREATE TABLE IF NOT EXISTS known_hosts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			hostname TEXT NOT NULL,
			port INTEGER NOT NULL DEFAULT 22,
			key_type TEXT NOT NULL,
			public_key TEXT NOT NULL,
			fingerprint TEXT NOT NULL,
			added_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_verified_at DATETIME,
			UNIQUE(hostname, port, key_type)
		);
	`)
	if err != nil {
		t.Fatalf("create tables: %v", err)
	}

	// Create KMS
	kms, err := NewKMS(db, nil)
	if err != nil {
		t.Fatalf("NewKMS: %v", err)
	}

	ctx := context.Background()
	if err := kms.Initialize(ctx); err != nil {
		t.Fatalf("KMS.Initialize: %v", err)
	}

	return db, kms
}

func TestNewSSHKeyManager(t *testing.T) {
	db, kms := setupTestSSHDB(t)
	defer db.Close()

	mgr := NewSSHKeyManager(db, kms)
	if mgr == nil {
		t.Fatal("NewSSHKeyManager() returned nil")
	}
}

func TestSSHKeyManagerGenerateKey(t *testing.T) {
	db, kms := setupTestSSHDB(t)
	defer db.Close()

	mgr := NewSSHKeyManager(db, kms)
	ctx := context.Background()

	key, err := mgr.GenerateKey(ctx, "test-key")
	if err != nil {
		t.Fatalf("GenerateKey() error: %v", err)
	}

	if key == nil {
		t.Fatal("GenerateKey() returned nil")
	}

	if key.Name != "test-key" {
		t.Errorf("key.Name = %s, want test-key", key.Name)
	}

	if key.KeyType != "ed25519" {
		t.Errorf("key.KeyType = %s, want ed25519", key.KeyType)
	}

	if key.PublicKey == "" {
		t.Error("key.PublicKey should not be empty")
	}

	if !strings.HasPrefix(key.PublicKey, "ssh-ed25519 ") {
		t.Errorf("key.PublicKey should start with 'ssh-ed25519 ', got %s", key.PublicKey[:20])
	}

	if key.Fingerprint == "" {
		t.Error("key.Fingerprint should not be empty")
	}

	if !strings.HasPrefix(key.Fingerprint, "SHA256:") {
		t.Errorf("key.Fingerprint should start with 'SHA256:', got %s", key.Fingerprint)
	}
}

func TestSSHKeyManagerGenerateKeyDuplicate(t *testing.T) {
	db, kms := setupTestSSHDB(t)
	defer db.Close()

	mgr := NewSSHKeyManager(db, kms)
	ctx := context.Background()

	if _, err := mgr.GenerateKey(ctx, "test-key"); err != nil {
		t.Fatalf("GenerateKey() first: %v", err)
	}

	// Second key with same name should fail
	_, err := mgr.GenerateKey(ctx, "test-key")
	if err == nil {
		t.Error("GenerateKey() with duplicate name should fail")
	}
}

func TestSSHKeyManagerGetKey(t *testing.T) {
	db, kms := setupTestSSHDB(t)
	defer db.Close()

	mgr := NewSSHKeyManager(db, kms)
	ctx := context.Background()

	// Non-existent key
	key, err := mgr.GetKey(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetKey() error: %v", err)
	}
	if key != nil {
		t.Error("GetKey() should return nil for non-existent key")
	}

	// Create and retrieve
	created, err := mgr.GenerateKey(ctx, "test-key")
	if err != nil {
		t.Fatalf("GenerateKey(): %v", err)
	}

	key, err = mgr.GetKey(ctx, "test-key")
	if err != nil {
		t.Fatalf("GetKey() error: %v", err)
	}
	if key == nil {
		t.Fatal("GetKey() returned nil for existing key")
	}

	if key.ID != created.ID {
		t.Error("GetKey() should return the same key")
	}
}

func TestSSHKeyManagerGetKeyByID(t *testing.T) {
	db, kms := setupTestSSHDB(t)
	defer db.Close()

	mgr := NewSSHKeyManager(db, kms)
	ctx := context.Background()

	created, err := mgr.GenerateKey(ctx, "test-key")
	if err != nil {
		t.Fatalf("GenerateKey(): %v", err)
	}

	key, err := mgr.GetKeyByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetKeyByID() error: %v", err)
	}
	if key == nil {
		t.Fatal("GetKeyByID() returned nil")
	}

	if key.Name != created.Name {
		t.Error("GetKeyByID() should return the same key")
	}
}

func TestSSHKeyManagerListKeys(t *testing.T) {
	db, kms := setupTestSSHDB(t)
	defer db.Close()

	mgr := NewSSHKeyManager(db, kms)
	ctx := context.Background()

	if _, err := mgr.GenerateKey(ctx, "key-a"); err != nil {
		t.Fatalf("GenerateKey(key-a): %v", err)
	}
	if _, err := mgr.GenerateKey(ctx, "key-b"); err != nil {
		t.Fatalf("GenerateKey(key-b): %v", err)
	}
	if _, err := mgr.GenerateKey(ctx, "key-c"); err != nil {
		t.Fatalf("GenerateKey(key-c): %v", err)
	}

	keys, err := mgr.ListKeys(ctx)
	if err != nil {
		t.Fatalf("ListKeys() error: %v", err)
	}

	if len(keys) != 3 {
		t.Errorf("len(keys) = %d, want 3", len(keys))
	}

	// Should be sorted by name
	if keys[0].Name != "key-a" {
		t.Errorf("keys[0].Name = %s, want key-a", keys[0].Name)
	}
}

func TestSSHKeyManagerDeleteKey(t *testing.T) {
	db, kms := setupTestSSHDB(t)
	defer db.Close()

	mgr := NewSSHKeyManager(db, kms)
	ctx := context.Background()

	if _, err := mgr.GenerateKey(ctx, "test-key"); err != nil {
		t.Fatalf("GenerateKey(): %v", err)
	}

	if err := mgr.DeleteKey(ctx, "test-key"); err != nil {
		t.Fatalf("DeleteKey() error: %v", err)
	}

	key, err := mgr.GetKey(ctx, "test-key")
	if err != nil {
		t.Fatalf("GetKey(): %v", err)
	}
	if key != nil {
		t.Error("key should be deleted")
	}
}

func TestSSHKeyManagerGetSigner(t *testing.T) {
	db, kms := setupTestSSHDB(t)
	defer db.Close()

	mgr := NewSSHKeyManager(db, kms)
	ctx := context.Background()

	if _, err := mgr.GenerateKey(ctx, "test-key"); err != nil {
		t.Fatalf("GenerateKey(): %v", err)
	}

	signer, err := mgr.GetSigner(ctx, "test-key")
	if err != nil {
		t.Fatalf("GetSigner() error: %v", err)
	}

	if signer == nil {
		t.Fatal("GetSigner() returned nil")
	}

	// Verify signer can sign data
	data := []byte("test data to sign")
	sig, err := signer.Sign(nil, data)
	if err != nil {
		t.Errorf("signer.Sign() error: %v", err)
	}

	if sig == nil {
		t.Error("signature should not be nil")
	}
}

func TestSSHKeyManagerGetSignerNotFound(t *testing.T) {
	db, kms := setupTestSSHDB(t)
	defer db.Close()

	mgr := NewSSHKeyManager(db, kms)
	ctx := context.Background()

	_, err := mgr.GetSigner(ctx, "nonexistent")
	if err == nil {
		t.Error("GetSigner() should fail for non-existent key")
	}
}

func TestSSHKeyManagerKnownHosts(t *testing.T) {
	db, kms := setupTestSSHDB(t)
	defer db.Close()

	mgr := NewSSHKeyManager(db, kms)
	ctx := context.Background()

	// Generate a test host key
	key, err := mgr.GenerateKey(ctx, "host-key")
	if err != nil {
		t.Fatalf("GenerateKey(): %v", err)
	}
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(key.PublicKey))
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}

	// Add known host
	if err := mgr.AddKnownHost(ctx, "example.com", 22, pubKey); err != nil {
		t.Fatalf("AddKnownHost() error: %v", err)
	}

	// Retrieve
	hosts, err := mgr.GetKnownHost(ctx, "example.com", 22)
	if err != nil {
		t.Fatalf("GetKnownHost() error: %v", err)
	}

	if len(hosts) != 1 {
		t.Errorf("len(hosts) = %d, want 1", len(hosts))
	}

	if hosts[0].Hostname != "example.com" {
		t.Errorf("hostname = %s, want example.com", hosts[0].Hostname)
	}
}

func TestSSHKeyManagerVerifyHostKey(t *testing.T) {
	db, kms := setupTestSSHDB(t)
	defer db.Close()

	mgr := NewSSHKeyManager(db, kms)
	ctx := context.Background()

	// Generate test keys
	key1, err := mgr.GenerateKey(ctx, "host-key-1")
	if err != nil {
		t.Fatalf("GenerateKey(host-key-1): %v", err)
	}
	pubKey1, _, _, _, err := ssh.ParseAuthorizedKey([]byte(key1.PublicKey))
	if err != nil {
		t.Fatalf("ParseAuthorizedKey(key1): %v", err)
	}

	key2, err := mgr.GenerateKey(ctx, "host-key-2")
	if err != nil {
		t.Fatalf("GenerateKey(host-key-2): %v", err)
	}
	pubKey2, _, _, _, err := ssh.ParseAuthorizedKey([]byte(key2.PublicKey))
	if err != nil {
		t.Fatalf("ParseAuthorizedKey(key2): %v", err)
	}

	// Add host with key1
	if err := mgr.AddKnownHost(ctx, "example.com", 22, pubKey1); err != nil {
		t.Fatalf("AddKnownHost(): %v", err)
	}

	// Verify with correct key
	err = mgr.VerifyHostKey(ctx, "example.com", 22, pubKey1)
	if err != nil {
		t.Errorf("VerifyHostKey() with correct key should succeed: %v", err)
	}

	// Verify with wrong key
	err = mgr.VerifyHostKey(ctx, "example.com", 22, pubKey2)
	if err == nil {
		t.Error("VerifyHostKey() with wrong key should fail")
	}

	// Check it's a mismatch error
	if _, ok := err.(*HostKeyMismatchError); !ok {
		t.Errorf("expected HostKeyMismatchError, got %T", err)
	}

	// Verify unknown host
	err = mgr.VerifyHostKey(ctx, "unknown.com", 22, pubKey1)
	if err == nil {
		t.Error("VerifyHostKey() for unknown host should fail")
	}

	if _, ok := err.(*UnknownHostError); !ok {
		t.Errorf("expected UnknownHostError, got %T", err)
	}
}

func TestSSHKeyManagerListKnownHosts(t *testing.T) {
	db, kms := setupTestSSHDB(t)
	defer db.Close()

	mgr := NewSSHKeyManager(db, kms)
	ctx := context.Background()

	key, err := mgr.GenerateKey(ctx, "host-key")
	if err != nil {
		t.Fatalf("GenerateKey(): %v", err)
	}
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(key.PublicKey))
	if err != nil {
		t.Fatalf("ParseAuthorizedKey(): %v", err)
	}

	if err := mgr.AddKnownHost(ctx, "host1.example.com", 22, pubKey); err != nil {
		t.Fatalf("AddKnownHost(host1): %v", err)
	}
	if err := mgr.AddKnownHost(ctx, "host2.example.com", 22, pubKey); err != nil {
		t.Fatalf("AddKnownHost(host2): %v", err)
	}
	if err := mgr.AddKnownHost(ctx, "host3.example.com", 2222, pubKey); err != nil {
		t.Fatalf("AddKnownHost(host3): %v", err)
	}

	hosts, err := mgr.ListKnownHosts(ctx)
	if err != nil {
		t.Fatalf("ListKnownHosts() error: %v", err)
	}

	if len(hosts) != 3 {
		t.Errorf("len(hosts) = %d, want 3", len(hosts))
	}
}

func TestSSHKeyManagerDeleteKnownHost(t *testing.T) {
	db, kms := setupTestSSHDB(t)
	defer db.Close()

	mgr := NewSSHKeyManager(db, kms)
	ctx := context.Background()

	key, err := mgr.GenerateKey(ctx, "host-key")
	if err != nil {
		t.Fatalf("GenerateKey(): %v", err)
	}
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(key.PublicKey))
	if err != nil {
		t.Fatalf("ParseAuthorizedKey(): %v", err)
	}

	if err := mgr.AddKnownHost(ctx, "example.com", 22, pubKey); err != nil {
		t.Fatalf("AddKnownHost(): %v", err)
	}

	if err := mgr.DeleteKnownHost(ctx, "example.com", 22); err != nil {
		t.Fatalf("DeleteKnownHost() error: %v", err)
	}

	hosts, err := mgr.GetKnownHost(ctx, "example.com", 22)
	if err != nil {
		t.Fatalf("GetKnownHost(): %v", err)
	}
	if len(hosts) != 0 {
		t.Error("known host should be deleted")
	}
}

func TestSSHKeyManagerTrustOnFirstUse(t *testing.T) {
	db, kms := setupTestSSHDB(t)
	defer db.Close()

	mgr := NewSSHKeyManager(db, kms)
	ctx := context.Background()

	key, err := mgr.GenerateKey(ctx, "host-key")
	if err != nil {
		t.Fatalf("GenerateKey(): %v", err)
	}
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(key.PublicKey))
	if err != nil {
		t.Fatalf("ParseAuthorizedKey(): %v", err)
	}

	callback := mgr.TrustOnFirstUse(ctx)

	// First connection - should add to known hosts
	err = callback("example.com:22", nil, pubKey)
	if err != nil {
		t.Errorf("TrustOnFirstUse first call should succeed: %v", err)
	}

	// Verify it was added
	hosts, err := mgr.GetKnownHost(ctx, "example.com", 22)
	if err != nil {
		t.Fatalf("GetKnownHost(): %v", err)
	}
	if len(hosts) != 1 {
		t.Error("host should have been added")
	}

	// Second connection with same key - should succeed
	err = callback("example.com:22", nil, pubKey)
	if err != nil {
		t.Errorf("TrustOnFirstUse second call should succeed: %v", err)
	}
}

func TestGetSSHPublicKeyFingerprint(t *testing.T) {
	db, kms := setupTestSSHDB(t)
	defer db.Close()

	mgr := NewSSHKeyManager(db, kms)
	ctx := context.Background()

	key, err := mgr.GenerateKey(ctx, "test-key")
	if err != nil {
		t.Fatalf("GenerateKey(): %v", err)
	}

	fingerprint, err := GetSSHPublicKeyFingerprint(key.PublicKey)
	if err != nil {
		t.Fatalf("GetSSHPublicKeyFingerprint() error: %v", err)
	}

	if fingerprint != key.Fingerprint {
		t.Errorf("fingerprint = %s, want %s", fingerprint, key.Fingerprint)
	}
}

func TestUnknownHostError(t *testing.T) {
	err := &UnknownHostError{
		Hostname:    "example.com",
		Port:        22,
		Fingerprint: "SHA256:abc123",
	}

	msg := err.Error()
	if !strings.Contains(msg, "example.com") {
		t.Error("error message should contain hostname")
	}
	if !strings.Contains(msg, "22") {
		t.Error("error message should contain port")
	}
	if !strings.Contains(msg, "SHA256:abc123") {
		t.Error("error message should contain fingerprint")
	}
}

func TestHostKeyMismatchError(t *testing.T) {
	err := &HostKeyMismatchError{
		Hostname:    "example.com",
		Port:        22,
		ExpectedKey: "SHA256:expected",
		ReceivedKey: "SHA256:received",
	}

	msg := err.Error()
	if !strings.Contains(msg, "example.com") {
		t.Error("error message should contain hostname")
	}
	if !strings.Contains(msg, "MITM") {
		t.Error("error message should warn about MITM")
	}
}
