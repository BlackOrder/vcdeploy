package security

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
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

func TestNewKMS(t *testing.T) {
	db := setupTestKMSDB(t)
	defer db.Close()
	ctx := context.Background()

	kms, err := NewKMS(ctx, db, nil)
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

	kms, err := NewKMS(ctx, db, nil)
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

	kms, err := NewKMS(context.Background(), db, nil)
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

	kms, err := NewKMS(context.Background(), db, nil)
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

	kms, err := NewKMS(context.Background(), db, nil)
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

	kms, err := NewKMS(context.Background(), db, nil)
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

	kms, err := NewKMS(context.Background(), db, nil)
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

	kms, err := NewKMS(context.Background(), db, nil)
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

	kms, err := NewKMS(context.Background(), db, nil)
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

	kms, err := NewKMS(context.Background(), db, nil)
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

	kms, err := NewKMS(context.Background(), db, nil)
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

	kms, err := NewKMS(context.Background(), db, nil)
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

	kms, err := NewKMS(context.Background(), db, nil)
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

	kms, err := NewKMS(context.Background(), db, nil)
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

	kms, err := NewKMS(context.Background(), db, nil)
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

	// Create and initialize using storage package
	store1, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("storage.Open() for store1: %v", err)
	}

	kms1, err := NewKMS(context.Background(), store1, nil)
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

	// Reopen
	store2, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("storage.Open() for store2: %v", err)
	}
	defer store2.Close()

	kms2, err := NewKMS(context.Background(), store2, nil)
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
