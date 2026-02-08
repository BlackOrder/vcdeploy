package security

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"github.com/rs/xid"
)

func setupTestKMSDB(t *testing.T) storage.Store {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	return store
}

func testMasterKey(t *testing.T) *MasterKey {
	t.Helper()
	mk, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey: %v", err)
	}
	return mk
}

func TestNewKMS(t *testing.T) {
	db := setupTestKMSDB(t)
	defer db.Close()
	ctx := context.Background()

	kms, err := NewKMS(ctx, db, nil, testMasterKey(t))
	if err != nil {
		t.Fatalf("NewKMS() error: %v", err)
	}

	if kms == nil {
		t.Fatal("NewKMS() returned nil")
	}
}

func TestKMSInitialize(t *testing.T) {
	db := setupTestKMSDB(t)
	defer db.Close()
	ctx := context.Background()

	kms, err := NewKMS(ctx, db, nil, testMasterKey(t))
	if err != nil {
		t.Fatalf("NewKMS() error: %v", err)
	}

	// Initialize should create a key
	if err := kms.Initialize(ctx); err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}

	// Should have a current key
	key := kms.GetCurrentKey()
	if key == nil {
		t.Fatal("GetCurrentKey() returned nil after Initialize")
	}

	if key.Status != KeyStatusActive {
		t.Errorf("key.Status = %v, want %v", key.Status, KeyStatusActive)
	}

	if key.Version != 1 {
		t.Errorf("key.Version = %d, want 1", key.Version)
	}
}

func TestKMSEncryptDecrypt(t *testing.T) {
	db := setupTestKMSDB(t)
	defer db.Close()

	kms, err := NewKMS(context.Background(), db, nil, testMasterKey(t))
	if err != nil {
		t.Fatalf("NewKMS() error: %v", err)
	}

	ctx := context.Background()
	if err := kms.Initialize(ctx); err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}

	plaintext := []byte("secret message")

	// Encrypt
	ciphertext, err := kms.Encrypt(ctx, plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	// Verify format
	if !IsVersionedCiphertext(ciphertext) {
		t.Errorf("ciphertext should start with 'v1:', got %s", ciphertext[:10])
	}

	// Decrypt
	decrypted, err := kms.Decrypt(ctx, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() error: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("decrypted = %s, want %s", decrypted, plaintext)
	}
}

func TestKMSEncryptDecryptString(t *testing.T) {
	db := setupTestKMSDB(t)
	defer db.Close()

	kms, err := NewKMS(context.Background(), db, nil, testMasterKey(t))
	if err != nil {
		t.Fatalf("NewKMS() error: %v", err)
	}

	ctx := context.Background()
	if err := kms.Initialize(ctx); err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}

	original := "hello world"

	encrypted, err := kms.EncryptString(ctx, original)
	if err != nil {
		t.Fatalf("EncryptString() error: %v", err)
	}

	decrypted, err := kms.DecryptString(ctx, encrypted)
	if err != nil {
		t.Fatalf("DecryptString() error: %v", err)
	}

	if decrypted != original {
		t.Errorf("decrypted = %s, want %s", decrypted, original)
	}
}

func TestKMSKeyRotation(t *testing.T) {
	db := setupTestKMSDB(t)
	defer db.Close()

	kms, err := NewKMS(context.Background(), db, nil, testMasterKey(t))
	if err != nil {
		t.Fatalf("NewKMS() error: %v", err)
	}

	ctx := context.Background()
	if err := kms.Initialize(ctx); err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}

	// Encrypt with initial key
	plaintext := []byte("data encrypted with key v1")
	ciphertext1, err := kms.Encrypt(ctx, plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	oldKey := kms.GetCurrentKey()
	oldKeyID := oldKey.ID

	// Rotate key
	newKey, err := kms.RotateKey(ctx)
	if err != nil {
		t.Fatalf("RotateKey() error: %v", err)
	}

	if newKey.Version != 2 {
		t.Errorf("newKey.Version = %d, want 2", newKey.Version)
	}

	if newKey.ID == oldKeyID {
		t.Error("new key should have different ID")
	}

	// Old ciphertext should still decrypt
	decrypted, err := kms.Decrypt(ctx, ciphertext1)
	if err != nil {
		t.Fatalf("Decrypt(old ciphertext) error: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("decrypted = %s, want %s", decrypted, plaintext)
	}

	// New encryption should use new key
	ciphertext2, err := kms.Encrypt(ctx, plaintext)
	if err != nil {
		t.Fatalf("Encrypt(after rotation) error: %v", err)
	}

	if ciphertext1 == ciphertext2 {
		t.Error("ciphertexts should be different after rotation")
	}
}

func TestKMSReEncrypt(t *testing.T) {
	db := setupTestKMSDB(t)
	defer db.Close()

	kms, err := NewKMS(context.Background(), db, nil, testMasterKey(t))
	if err != nil {
		t.Fatalf("NewKMS() error: %v", err)
	}

	ctx := context.Background()
	if err := kms.Initialize(ctx); err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}

	// Encrypt with initial key
	plaintext := []byte("my secret")
	ciphertext1, err := kms.Encrypt(ctx, plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	// Rotate key
	if _, err := kms.RotateKey(ctx); err != nil {
		t.Fatalf("RotateKey() error: %v", err)
	}

	// Re-encrypt with new key
	ciphertext2, err := kms.ReEncrypt(ctx, ciphertext1)
	if err != nil {
		t.Fatalf("ReEncrypt() error: %v", err)
	}

	// Verify it's encrypted with new key
	if ciphertext1 == ciphertext2 {
		t.Error("re-encrypted ciphertext should be different")
	}

	// Both should decrypt to same plaintext
	decrypted1, err := kms.Decrypt(ctx, ciphertext1)
	if err != nil {
		t.Fatalf("Decrypt(ciphertext1): %v", err)
	}
	decrypted2, err := kms.Decrypt(ctx, ciphertext2)
	if err != nil {
		t.Fatalf("Decrypt(ciphertext2): %v", err)
	}

	if !bytes.Equal(decrypted1, decrypted2) {
		t.Error("both ciphertexts should decrypt to same plaintext")
	}
}

func TestKMSKeyDeletionScheduling(t *testing.T) {
	db := setupTestKMSDB(t)
	defer db.Close()

	kms, err := NewKMS(context.Background(), db, nil, testMasterKey(t))
	if err != nil {
		t.Fatalf("NewKMS() error: %v", err)
	}

	ctx := context.Background()
	if err := kms.Initialize(ctx); err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}

	oldKeyID := kms.GetCurrentKey().ID

	// Rotate to make old key inactive
	if _, err := kms.RotateKey(ctx); err != nil {
		t.Fatalf("RotateKey() error: %v", err)
	}

	// Cannot schedule active key deletion
	if err := kms.ScheduleKeyDeletion(ctx, kms.GetCurrentKey().ID, time.Hour); err == nil {
		t.Error("ScheduleKeyDeletion(active key) should fail")
	}

	// Can schedule inactive key deletion
	if err := kms.ScheduleKeyDeletion(ctx, oldKeyID, time.Hour); err != nil {
		t.Fatalf("ScheduleKeyDeletion() error: %v", err)
	}

	// Verify status changed
	key, err := kms.getKey(ctx, oldKeyID)
	if err != nil {
		t.Fatalf("getKey(): %v", err)
	}
	if key.Status != KeyStatusScheduled {
		t.Errorf("key.Status = %v, want %v", key.Status, KeyStatusScheduled)
	}
}

func TestKMSCancelKeyDeletion(t *testing.T) {
	db := setupTestKMSDB(t)
	defer db.Close()

	kms, err := NewKMS(context.Background(), db, nil, testMasterKey(t))
	if err != nil {
		t.Fatalf("NewKMS() error: %v", err)
	}

	ctx := context.Background()
	if err := kms.Initialize(ctx); err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}

	oldKeyID := kms.GetCurrentKey().ID

	// Rotate and schedule deletion
	if _, err := kms.RotateKey(ctx); err != nil {
		t.Fatalf("RotateKey() error: %v", err)
	}

	if err := kms.ScheduleKeyDeletion(ctx, oldKeyID, time.Hour); err != nil {
		t.Fatalf("ScheduleKeyDeletion() error: %v", err)
	}

	// Cancel deletion
	if err := kms.CancelKeyDeletion(ctx, oldKeyID); err != nil {
		t.Fatalf("CancelKeyDeletion() error: %v", err)
	}

	// Verify status changed back
	key, err := kms.getKey(ctx, oldKeyID)
	if err != nil {
		t.Fatalf("getKey(): %v", err)
	}
	if key.Status != KeyStatusInactive {
		t.Errorf("key.Status = %v, want %v", key.Status, KeyStatusInactive)
	}
}

func TestKMSDeleteKeyNow(t *testing.T) {
	db := setupTestKMSDB(t)
	defer db.Close()

	kms, err := NewKMS(context.Background(), db, nil, testMasterKey(t))
	if err != nil {
		t.Fatalf("NewKMS() error: %v", err)
	}

	ctx := context.Background()
	if err := kms.Initialize(ctx); err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}

	// Encrypt some data
	plaintext := []byte("will be inaccessible")
	ciphertext, err := kms.Encrypt(ctx, plaintext)
	if err != nil {
		t.Fatalf("Encrypt(): %v", err)
	}

	oldKeyID := kms.GetCurrentKey().ID

	// Rotate
	if _, err := kms.RotateKey(ctx); err != nil {
		t.Fatalf("RotateKey() error: %v", err)
	}

	// Cannot delete active key
	if err := kms.DeleteKeyNow(ctx, kms.GetCurrentKey().ID); err == nil {
		t.Error("DeleteKeyNow(active key) should fail")
	}

	// Delete old key immediately
	if err := kms.DeleteKeyNow(ctx, oldKeyID); err != nil {
		t.Fatalf("DeleteKeyNow() error: %v", err)
	}

	// Decryption should fail
	_, err = kms.Decrypt(ctx, ciphertext)
	if err == nil {
		t.Error("Decrypt() should fail after key deletion")
	}
}

func TestKMSListKeys(t *testing.T) {
	db := setupTestKMSDB(t)
	defer db.Close()

	kms, err := NewKMS(context.Background(), db, nil, testMasterKey(t))
	if err != nil {
		t.Fatalf("NewKMS() error: %v", err)
	}

	ctx := context.Background()
	if err := kms.Initialize(ctx); err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}

	// Rotate a few times
	if _, err := kms.RotateKey(ctx); err != nil {
		t.Fatalf("RotateKey() first: %v", err)
	}
	if _, err := kms.RotateKey(ctx); err != nil {
		t.Fatalf("RotateKey() second: %v", err)
	}

	keys, err := kms.ListKeys(ctx)
	if err != nil {
		t.Fatalf("ListKeys() error: %v", err)
	}

	if len(keys) != 3 {
		t.Errorf("len(keys) = %d, want 3", len(keys))
	}

	// Should be sorted by version descending
	if keys[0].Version != 3 {
		t.Errorf("keys[0].Version = %d, want 3", keys[0].Version)
	}
}

func TestKMSProcessScheduledDeletions(t *testing.T) {
	db := setupTestKMSDB(t)
	defer db.Close()

	kms, err := NewKMS(context.Background(), db, nil, testMasterKey(t))
	if err != nil {
		t.Fatalf("NewKMS() error: %v", err)
	}

	ctx := context.Background()
	if err := kms.Initialize(ctx); err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}

	oldKeyID := kms.GetCurrentKey().ID

	// Rotate and schedule deletion with 0 grace period
	if _, err := kms.RotateKey(ctx); err != nil {
		t.Fatalf("RotateKey() error: %v", err)
	}

	if err := kms.ScheduleKeyDeletion(ctx, oldKeyID, 0); err != nil {
		t.Fatalf("ScheduleKeyDeletion() error: %v", err)
	}

	// Process deletions
	count, err := kms.ProcessScheduledDeletions(ctx)
	if err != nil {
		t.Fatalf("ProcessScheduledDeletions() error: %v", err)
	}

	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	// Key should be deleted
	key, err := kms.getKey(ctx, oldKeyID)
	if err != nil {
		t.Fatalf("getKey(): %v", err)
	}
	if key.Status != KeyStatusDeleted {
		t.Errorf("key.Status = %v, want %v", key.Status, KeyStatusDeleted)
	}
}

func TestKMSDecryptInvalidFormat(t *testing.T) {
	db := setupTestKMSDB(t)
	defer db.Close()

	kms, err := NewKMS(context.Background(), db, nil, testMasterKey(t))
	if err != nil {
		t.Fatalf("NewKMS() error: %v", err)
	}

	ctx := context.Background()
	if err := kms.Initialize(ctx); err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}

	// Invalid format
	_, err = kms.Decrypt(ctx, "invalid")
	if err == nil {
		t.Error("Decrypt(invalid) should fail")
	}

	// Wrong version
	_, err = kms.Decrypt(ctx, "v2:key:nonce:ciphertext")
	if err == nil {
		t.Error("Decrypt(v2) should fail")
	}
}

func TestKMSEncryptWithoutKey(t *testing.T) {
	db := setupTestKMSDB(t)
	defer db.Close()

	kms, err := NewKMS(context.Background(), db, nil, testMasterKey(t))
	if err != nil {
		t.Fatalf("NewKMS() error: %v", err)
	}

	ctx := context.Background()
	// Don't initialize - no key

	_, err = kms.Encrypt(ctx, []byte("test"))
	if err == nil {
		t.Error("Encrypt() without key should fail")
	}
}

func TestIsVersionedCiphertext(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"v1:keyid:nonce:ciphertext", true},
		{"v1:", true},
		{"v2:keyid:nonce:ciphertext", false},
		{"plaintext", false},
		{"", false},
	}

	for _, tt := range tests {
		got := IsVersionedCiphertext(tt.input)
		if got != tt.want {
			t.Errorf("IsVersionedCiphertext(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestKMSConcurrentEncryption(t *testing.T) {
	db := setupTestKMSDB(t)
	defer db.Close()

	kms, err := NewKMS(context.Background(), db, nil, testMasterKey(t))
	if err != nil {
		t.Fatalf("NewKMS() error: %v", err)
	}

	ctx := context.Background()
	if err := kms.Initialize(ctx); err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}

	// Concurrent encryption should work
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			plaintext := []byte("concurrent test")
			ciphertext, err := kms.Encrypt(ctx, plaintext)
			if err != nil {
				t.Errorf("concurrent Encrypt() error: %v", err)
			}
			_, err = kms.Decrypt(ctx, ciphertext)
			if err != nil {
				t.Errorf("concurrent Decrypt() error: %v", err)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestKMSDoubleInitialize(t *testing.T) {
	db := setupTestKMSDB(t)
	defer db.Close()

	kms, err := NewKMS(context.Background(), db, nil, testMasterKey(t))
	if err != nil {
		t.Fatalf("NewKMS() error: %v", err)
	}

	ctx := context.Background()

	// First initialize
	if err := kms.Initialize(ctx); err != nil {
		t.Fatalf("first Initialize() error: %v", err)
	}

	keyBefore := kms.GetCurrentKey()

	// Second initialize should be no-op
	if err := kms.Initialize(ctx); err != nil {
		t.Fatalf("second Initialize() error: %v", err)
	}

	keyAfter := kms.GetCurrentKey()

	if keyBefore.ID != keyAfter.ID {
		t.Error("second Initialize() should not create new key")
	}
}

func TestKMSPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Use the SAME master key for both instances so encrypted key material can be decrypted after reopen
	mk := testMasterKey(t)

	// Create and initialize using storage package
	store1, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("storage.Open() for store1: %v", err)
	}

	kms1, err := NewKMS(context.Background(), store1, nil, mk)
	if err != nil {
		t.Fatalf("NewKMS() for kms1: %v", err)
	}
	ctx := context.Background()
	if err := kms1.Initialize(ctx); err != nil {
		t.Fatalf("kms1.Initialize(): %v", err)
	}

	plaintext := []byte("persistent data")
	ciphertext, err := kms1.Encrypt(ctx, plaintext)
	if err != nil {
		t.Fatalf("kms1.Encrypt(): %v", err)
	}

	keyID := kms1.GetCurrentKey().ID
	store1.Close()

	// Reopen with SAME master key
	store2, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("storage.Open() for store2: %v", err)
	}
	defer store2.Close()

	kms2, err := NewKMS(context.Background(), store2, nil, mk)
	if err != nil {
		t.Fatalf("NewKMS() after reopen error: %v", err)
	}

	// Should load existing key
	if kms2.GetCurrentKey() == nil {
		t.Fatal("should have loaded existing key")
	}

	if kms2.GetCurrentKey().ID != keyID {
		t.Error("should have loaded same key")
	}

	// Should decrypt
	decrypted, err := kms2.Decrypt(ctx, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() after reopen error: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Error("decrypted data should match")
	}
}

func init() {
	// Suppress unused import error
	_ = os.TempDir
}

func TestDefaultKMSConfig(t *testing.T) {
	config := DefaultKMSConfig()

	// Check default values
	expectedGracePeriod := 30 * 24 * time.Hour
	if config.DeletionGracePeriod != expectedGracePeriod {
		t.Errorf("DefaultKMSConfig().DeletionGracePeriod = %v, want %v", config.DeletionGracePeriod, expectedGracePeriod)
	}

	if config.AutoRotationPeriod != 0 {
		t.Errorf("DefaultKMSConfig().AutoRotationPeriod = %v, want 0 (disabled)", config.AutoRotationPeriod)
	}
}

// --- Stage 11 tests: KMS security invariants ---

// TestMasterKeyMetaPopulated verifies that after KMS init, the encryption_keys
// table has at least one row with a valid XID-format key_id.
func TestMasterKeyMetaPopulated(t *testing.T) {
	db := setupTestKMSDB(t)
	defer db.Close()
	ctx := context.Background()

	mk := testMasterKey(t)
	kms, err := NewKMS(ctx, db, nil, mk)
	if err != nil {
		t.Fatalf("NewKMS: %v", err)
	}
	if err := kms.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// Query raw DB for encryption_keys rows
	rows, err := db.Conn().QueryContext(ctx, "SELECT id FROM encryption_keys")
	if err != nil {
		t.Fatalf("query encryption_keys: %v", err)
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var keyID string
		if err := rows.Scan(&keyID); err != nil {
			t.Fatalf("scan key_id: %v", err)
		}
		count++
		// Verify XID format: 20 chars, parseable
		if len(keyID) != 20 {
			t.Errorf("key ID %q length = %d, want 20", keyID, len(keyID))
		}
		if _, err := xid.FromString(keyID); err != nil {
			t.Errorf("key ID %q is not valid XID: %v", keyID, err)
		}
	}
	if count == 0 {
		t.Error("encryption_keys table is empty after Initialize")
	}
}

// TestKMSEncryptsKeyMaterial reads raw key_material_encrypted from the DB and
// verifies it is NOT the plaintext 32-byte key material.
func TestKMSEncryptsKeyMaterial(t *testing.T) {
	db := setupTestKMSDB(t)
	defer db.Close()
	ctx := context.Background()

	mk := testMasterKey(t)
	kms, err := NewKMS(ctx, db, nil, mk)
	if err != nil {
		t.Fatalf("NewKMS: %v", err)
	}
	if err := kms.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// Get the in-memory key material
	currentKey := kms.GetCurrentKey()
	if currentKey == nil {
		t.Fatal("no current key after Initialize")
	}
	rawKeyMaterial := currentKey.KeyMaterial // 32 bytes

	// Read the stored value directly from SQLite
	var storedMaterial []byte
	err = db.Conn().QueryRowContext(ctx,
		"SELECT key_material_encrypted FROM encryption_keys WHERE id = ?",
		currentKey.ID,
	).Scan(&storedMaterial)
	if err != nil {
		t.Fatalf("read raw key_material_encrypted: %v", err)
	}

	// The stored value must NOT be the raw key material
	if bytes.Equal(storedMaterial, rawKeyMaterial) {
		t.Fatal("key_material_encrypted in DB is raw plaintext — encryption not working")
	}

	// The stored value should be longer (nonce + ciphertext + tag)
	if len(storedMaterial) <= 32 {
		t.Errorf("stored material length = %d, expected > 32 (nonce + ciphertext + tag)", len(storedMaterial))
	}

	// Verify the stored value IS decryptable with the MasterKey
	decrypted, err := mk.Decrypt(storedMaterial)
	if err != nil {
		t.Fatalf("MasterKey.Decrypt(stored material) failed: %v", err)
	}
	if !bytes.Equal(decrypted, rawKeyMaterial) {
		t.Error("MasterKey.Decrypt(stored) != original key material")
	}
}

// TestKMSWithDifferentMasterKey verifies that opening a KMS with the wrong
// MasterKey fails to decrypt stored key material.
func TestKMSWithDifferentMasterKey(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	ctx := context.Background()

	// Create KMS with MasterKey A
	mkA := testMasterKey(t)
	store1, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open db: %v", err)
	}
	kms1, err := NewKMS(ctx, store1, nil, mkA)
	if err != nil {
		t.Fatalf("NewKMS(A): %v", err)
	}
	if err := kms1.Initialize(ctx); err != nil {
		t.Fatalf("Initialize(A): %v", err)
	}
	store1.Close()

	// Reopen with MasterKey B — should fail
	mkB := testMasterKey(t)
	store2, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open db (2): %v", err)
	}
	defer store2.Close()

	_, err = NewKMS(ctx, store2, nil, mkB)
	if err == nil {
		t.Fatal("NewKMS with wrong MasterKey should have failed, but succeeded")
	}
	// Error should mention decryption failure
	if !strings.Contains(err.Error(), "decrypt") {
		t.Errorf("expected decryption error, got: %v", err)
	}
}

// TestKMSKeyIDIsXID verifies that generated key IDs are valid 20-char XIDs.
func TestKMSKeyIDIsXID(t *testing.T) {
	db := setupTestKMSDB(t)
	defer db.Close()
	ctx := context.Background()

	kms, err := NewKMS(ctx, db, nil, testMasterKey(t))
	if err != nil {
		t.Fatalf("NewKMS: %v", err)
	}
	if err := kms.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	key := kms.GetCurrentKey()
	if key == nil {
		t.Fatal("no current key")
	}

	// XID is exactly 20 characters
	if len(key.ID) != 20 {
		t.Errorf("key.ID length = %d, want 20", len(key.ID))
	}

	// Must be parseable as XID
	parsed, err := xid.FromString(key.ID)
	if err != nil {
		t.Fatalf("xid.FromString(%q) failed: %v", key.ID, err)
	}

	// Round-trip: parsed XID string should match
	if parsed.String() != key.ID {
		t.Errorf("XID round-trip mismatch: %q != %q", parsed.String(), key.ID)
	}

	// Rotate and verify the new key also has valid XID
	newKey, err := kms.RotateKey(ctx)
	if err != nil {
		t.Fatalf("RotateKey: %v", err)
	}
	if len(newKey.ID) != 20 {
		t.Errorf("rotated key.ID length = %d, want 20", len(newKey.ID))
	}
	if _, err := xid.FromString(newKey.ID); err != nil {
		t.Errorf("rotated key ID not valid XID: %v", err)
	}
}

// TestMasterKeyRotation verifies that encrypting data with MasterKey A, then
// attempting to access with MasterKey B (without re-encryption) fails.
// True MasterKey rotation requires re-encrypting all KMS key material.
func TestMasterKeyRotation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	ctx := context.Background()

	// Init with MasterKey A
	mkA := testMasterKey(t)
	store1, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open db: %v", err)
	}
	kmsA, err := NewKMS(ctx, store1, nil, mkA)
	if err != nil {
		t.Fatalf("NewKMS(A): %v", err)
	}
	if err := kmsA.Initialize(ctx); err != nil {
		t.Fatalf("Initialize(A): %v", err)
	}

	// Encrypt some data
	plaintext := []byte("sensitive data for rotation test")
	ciphertext, err := kmsA.Encrypt(ctx, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Verify decryption works with A
	decrypted, err := kmsA.Decrypt(ctx, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt with A: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatal("Decrypt mismatch with original MasterKey")
	}
	store1.Close()

	// Attempt to open with MasterKey B — should fail (key material can't be decrypted)
	mkB := testMasterKey(t)
	store2, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open db (2): %v", err)
	}
	defer store2.Close()

	_, err = NewKMS(ctx, store2, nil, mkB)
	if err == nil {
		t.Fatal("NewKMS with rotated MasterKey B should fail without re-encryption")
	}

	// Reopen with original MasterKey A — should still work
	store3, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open db (3): %v", err)
	}
	defer store3.Close()

	kmsA2, err := NewKMS(ctx, store3, nil, mkA)
	if err != nil {
		t.Fatalf("NewKMS(A again): %v", err)
	}
	decrypted2, err := kmsA2.Decrypt(ctx, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt with A after failed B: %v", err)
	}
	if !bytes.Equal(decrypted2, plaintext) {
		t.Error("Decrypt mismatch after reopening with original MasterKey")
	}
}

// TestRawDBSecurityOpaque opens the SQLite file directly and verifies that
// the key_material_encrypted column contains opaque (encrypted) data, not
// the raw AES key bytes.
func TestRawDBSecurityOpaque(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	ctx := context.Background()

	mk := testMasterKey(t)
	store1, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open db: %v", err)
	}
	kms1, err := NewKMS(ctx, store1, nil, mk)
	if err != nil {
		t.Fatalf("NewKMS: %v", err)
	}
	if err := kms1.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	inMemoryKey := kms1.GetCurrentKey().KeyMaterial
	store1.Close()

	// Open the raw SQLite file directly (no KMS, no MasterKey — just raw SQL)
	rawDB, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	defer rawDB.Close()

	var rawMaterial []byte
	err = rawDB.Conn().QueryRowContext(ctx,
		"SELECT key_material_encrypted FROM encryption_keys LIMIT 1",
	).Scan(&rawMaterial)
	if err != nil {
		t.Fatalf("raw read key_material_encrypted: %v", err)
	}

	// Must NOT be the raw 32-byte key
	if bytes.Equal(rawMaterial, inMemoryKey) {
		t.Fatal("SECURITY VIOLATION: raw DB contains plaintext key material")
	}

	// Must be opaque (longer than 32 bytes due to nonce + tag)
	if len(rawMaterial) <= 32 {
		t.Errorf("stored material is only %d bytes — expected nonce+ciphertext+tag > 32", len(rawMaterial))
	}
}
