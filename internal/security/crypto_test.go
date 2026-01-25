package security

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateMasterKey(t *testing.T) {
	t.Parallel()

	key, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	if key == nil {
		t.Fatal("GenerateMasterKey() returned nil")
	}
	if key.KeyID() == "" {
		t.Error("KeyID() returned empty string")
	}
}

func TestGenerateMasterKeyUniqueness(t *testing.T) {
	t.Parallel()

	key1, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() for key1: %v", err)
	}
	key2, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() for key2: %v", err)
	}
	if key1.KeyID() == key2.KeyID() {
		t.Error("Generated keys should have different IDs")
	}
}

func TestMasterKeyEncryptDecrypt(t *testing.T) {
	t.Parallel()

	key, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey(): %v", err)
	}

	tests := []struct {
		name      string
		plaintext []byte
	}{
		{"empty", []byte{}},
		{"short", []byte("hello")},
		{"medium", []byte("This is a medium length test message for encryption testing.")},
		{"special_chars", []byte("Special: !@#$%^&*()_+-=[]{}|;':,./<>?\n\t")},
		{"binary", []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD}},
		{"unicode", []byte("Hello world")},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			encrypted, err := key.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt() error = %v", err)
			}

			decrypted, err := key.Decrypt(encrypted)
			if err != nil {
				t.Fatalf("Decrypt() error = %v", err)
			}

			if !bytes.Equal(decrypted, tt.plaintext) {
				t.Errorf("Decrypt() = %v, want %v", decrypted, tt.plaintext)
			}
		})
	}
}

func TestMasterKeyEncryptDifferentCiphertext(t *testing.T) {
	t.Parallel()

	key, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey(): %v", err)
	}
	plaintext := []byte("same message")

	enc1, err := key.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() for enc1: %v", err)
	}
	enc2, err := key.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() for enc2: %v", err)
	}

	if bytes.Equal(enc1, enc2) {
		t.Error("Same plaintext should produce different ciphertext due to random nonce")
	}
}

func TestMasterKeyDecryptWrongKey(t *testing.T) {
	t.Parallel()

	key1, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() for key1: %v", err)
	}
	key2, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() for key2: %v", err)
	}

	encrypted, err := key1.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt(): %v", err)
	}
	_, err = key2.Decrypt(encrypted)
	if err == nil {
		t.Error("Decrypt with wrong key should fail")
	}
}

func TestMasterKeyDecryptTamperedData(t *testing.T) {
	t.Parallel()

	key, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey(): %v", err)
	}
	encrypted, err := key.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt(): %v", err)
	}

	// Tamper with the ciphertext
	encrypted[len(encrypted)-1] ^= 0xFF

	_, err = key.Decrypt(encrypted)
	if err == nil {
		t.Error("Decrypt of tampered data should fail")
	}
}

func TestMasterKeyDecryptTooShort(t *testing.T) {
	t.Parallel()

	key, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey(): %v", err)
	}
	_, err = key.Decrypt([]byte("short"))
	if err == nil {
		t.Error("Decrypt of too-short data should fail")
	}
}

func TestMasterKeySaveAndLoad(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	keyPath := filepath.Join(tmpDir, "master.key")

	// Generate and save
	key1, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey(): %v", err)
	}
	if err := key1.SaveToFile(keyPath); err != nil {
		t.Fatalf("SaveToFile() error = %v", err)
	}

	// Check permissions
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("os.Stat(): %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("Key file permissions = %o, want 0600", info.Mode().Perm())
	}

	// Load and verify
	key2, err := LoadMasterKey(keyPath)
	if err != nil {
		t.Fatalf("LoadMasterKey() error = %v", err)
	}

	if key1.KeyID() != key2.KeyID() {
		t.Error("Loaded key should have same ID")
	}

	// Verify encryption/decryption works across save/load
	plaintext := []byte("test message")
	encrypted, err := key1.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt(): %v", err)
	}
	decrypted, err := key2.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt(): %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Error("Loaded key should decrypt data encrypted by original key")
	}
}

func TestLoadMasterKeyFromEnv(t *testing.T) {
	// Note: Cannot use t.Parallel() - modifies environment

	// Generate a valid key and encode it
	key, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey(): %v", err)
	}
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "master.key")
	if err := key.SaveToFile(keyPath); err != nil {
		t.Fatalf("SaveToFile(): %v", err)
	}

	// Read the encoded key
	data, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("os.ReadFile(): %v", err)
	}
	os.Setenv("VCDEPLOY_MASTER_KEY", string(data))
	defer os.Unsetenv("VCDEPLOY_MASTER_KEY")

	loaded, err := LoadMasterKey("/nonexistent/path")
	if err != nil {
		t.Fatalf("LoadMasterKey from env error = %v", err)
	}

	if loaded.KeyID() != key.KeyID() {
		t.Error("Key loaded from env should match original")
	}
}

func TestLoadMasterKeyInvalidEnv(t *testing.T) {
	// Note: Cannot use t.Parallel() - modifies environment

	os.Setenv("VCDEPLOY_MASTER_KEY", "invalid-base64!")
	defer os.Unsetenv("VCDEPLOY_MASTER_KEY")

	_, err := LoadMasterKey("/nonexistent")
	if err == nil {
		t.Error("LoadMasterKey with invalid env should fail")
	}
}

func TestLoadMasterKeyWrongSize(t *testing.T) {
	// Note: Cannot use t.Parallel() - modifies environment

	os.Setenv("VCDEPLOY_MASTER_KEY", "dG9vLXNob3J0") // "too-short" in base64
	defer os.Unsetenv("VCDEPLOY_MASTER_KEY")

	_, err := LoadMasterKey("/nonexistent")
	if err == nil {
		t.Error("LoadMasterKey with wrong size should fail")
	}
}

func TestLoadMasterKeyNotFound(t *testing.T) {
	t.Parallel()

	_, err := LoadMasterKey("/nonexistent/path/key")
	if err == nil {
		t.Error("LoadMasterKey with nonexistent file should fail")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Error should mention 'not found', got: %v", err)
	}
}

func TestHashPassword(t *testing.T) {
	t.Parallel()

	password := "MySecureP@ssw0rd!"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	if hash == password {
		t.Error("Hash should not equal plaintext password")
	}

	if !VerifyPassword(password, hash) {
		t.Error("VerifyPassword() should return true for correct password")
	}

	if VerifyPassword("wrong-password", hash) {
		t.Error("VerifyPassword() should return false for wrong password")
	}
}

func TestHashPasswordUnique(t *testing.T) {
	t.Parallel()

	password := "same-password"
	hash1, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() for hash1: %v", err)
	}
	hash2, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() for hash2: %v", err)
	}

	if hash1 == hash2 {
		t.Error("Same password should produce different hashes (salt)")
	}

	// Both should still verify
	if !VerifyPassword(password, hash1) || !VerifyPassword(password, hash2) {
		t.Error("Both hashes should verify correctly")
	}
}

func TestGenerateRandomPassword(t *testing.T) {
	t.Parallel()

	lengths := []int{8, 16, 32, 64}
	for _, length := range lengths {
		pass, err := GenerateRandomPassword(length)
		if err != nil {
			t.Fatalf("GenerateRandomPassword(%d) error = %v", length, err)
		}
		if len(pass) != length {
			t.Errorf("Password length = %d, want %d", len(pass), length)
		}
	}
}

func TestGenerateRandomPasswordUnique(t *testing.T) {
	t.Parallel()

	pass1, err := GenerateRandomPassword(16)
	if err != nil {
		t.Fatalf("GenerateRandomPassword() for pass1: %v", err)
	}
	pass2, err := GenerateRandomPassword(16)
	if err != nil {
		t.Fatalf("GenerateRandomPassword() for pass2: %v", err)
	}
	if pass1 == pass2 {
		t.Error("Generated passwords should be unique")
	}
}

func TestGenerateToken(t *testing.T) {
	t.Parallel()

	token, err := GenerateToken(32)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	if token == "" {
		t.Error("Token should not be empty")
	}
}

func TestGenerateTokenUnique(t *testing.T) {
	t.Parallel()

	token1, err := GenerateToken(32)
	if err != nil {
		t.Fatalf("GenerateToken() for token1: %v", err)
	}
	token2, err := GenerateToken(32)
	if err != nil {
		t.Fatalf("GenerateToken() for token2: %v", err)
	}
	if token1 == token2 {
		t.Error("Generated tokens should be unique")
	}
}

func TestGenerateSessionID(t *testing.T) {
	t.Parallel()

	sid1, err := GenerateSessionID()
	if err != nil {
		t.Fatalf("GenerateSessionID() error = %v", err)
	}
	sid2, err := GenerateSessionID()
	if err != nil {
		t.Fatalf("GenerateSessionID() for sid2: %v", err)
	}

	if sid1 == "" {
		t.Error("Session ID should not be empty")
	}
	if sid1 == sid2 {
		t.Error("Session IDs should be unique")
	}
}

func TestEncryptWithPassphrase(t *testing.T) {
	t.Parallel()

	plaintext := []byte("test secret data")
	passphrase := []byte("my-secure-passphrase")

	encrypted, err := EncryptWithPassphrase(plaintext, passphrase)
	if err != nil {
		t.Fatalf("EncryptWithPassphrase failed: %v", err)
	}

	if len(encrypted) == 0 {
		t.Error("encrypted data should not be empty")
	}

	// Encrypted should be longer than plaintext (salt + nonce + ciphertext + auth tag)
	if len(encrypted) <= len(plaintext) {
		t.Errorf("encrypted length %d should be greater than plaintext length %d", len(encrypted), len(plaintext))
	}
}

func TestDecryptWithPassphrase(t *testing.T) {
	t.Parallel()

	plaintext := []byte("test secret data for roundtrip")
	passphrase := []byte("my-secure-passphrase")

	encrypted, err := EncryptWithPassphrase(plaintext, passphrase)
	if err != nil {
		t.Fatalf("EncryptWithPassphrase failed: %v", err)
	}

	decrypted, err := DecryptWithPassphrase(encrypted, passphrase)
	if err != nil {
		t.Fatalf("DecryptWithPassphrase failed: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("decrypted = %q, want %q", string(decrypted), string(plaintext))
	}
}

func TestDecryptWithPassphraseWrongPassword(t *testing.T) {
	t.Parallel()

	plaintext := []byte("test secret data")
	passphrase := []byte("correct-passphrase")
	wrongPassphrase := []byte("wrong-passphrase")

	encrypted, err := EncryptWithPassphrase(plaintext, passphrase)
	if err != nil {
		t.Fatalf("EncryptWithPassphrase failed: %v", err)
	}

	_, err = DecryptWithPassphrase(encrypted, wrongPassphrase)
	if err == nil {
		t.Error("expected error for wrong passphrase")
	}
}

func TestDecryptWithPassphraseTooShort(t *testing.T) {
	t.Parallel()

	passphrase := []byte("passphrase")

	// Ciphertext is too short
	_, err := DecryptWithPassphrase([]byte("short"), passphrase)
	if err == nil {
		t.Error("expected error for too short ciphertext")
	}
}

func TestGenerateSecureToken(t *testing.T) {
	t.Parallel()

	token, err := GenerateSecureToken(32)
	if err != nil {
		t.Fatalf("GenerateSecureToken failed: %v", err)
	}

	// 32 bytes -> 64 hex chars
	if len(token) != 64 {
		t.Errorf("expected token length 64, got %d", len(token))
	}

	// Generate another to verify uniqueness
	token2, err := GenerateSecureToken(32)
	if err != nil {
		t.Fatalf("GenerateSecureToken failed: %v", err)
	}

	if token == token2 {
		t.Error("tokens should be unique")
	}
}

func TestGenerateSecureTokenLength(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		length int
		hexLen int
	}{
		{16, 32},
		{32, 64},
		{64, 128},
	}

	for _, tc := range testCases {
		token, err := GenerateSecureToken(tc.length)
		if err != nil {
			t.Errorf("GenerateSecureToken(%d) failed: %v", tc.length, err)
			continue
		}
		if len(token) != tc.hexLen {
			t.Errorf("GenerateSecureToken(%d) = %d chars, want %d", tc.length, len(token), tc.hexLen)
		}
	}
}

// Benchmarks
func BenchmarkGenerateMasterKey(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GenerateMasterKey()
	}
}

func BenchmarkEncrypt(b *testing.B) {
	key, err := GenerateMasterKey()
	if err != nil {
		b.Fatalf("GenerateMasterKey(): %v", err)
	}
	plaintext := []byte("benchmark test message for encryption")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key.Encrypt(plaintext)
	}
}

func BenchmarkDecrypt(b *testing.B) {
	key, err := GenerateMasterKey()
	if err != nil {
		b.Fatalf("GenerateMasterKey(): %v", err)
	}
	plaintext := []byte("benchmark test message for decryption")
	encrypted, err := key.Encrypt(plaintext)
	if err != nil {
		b.Fatalf("Encrypt(): %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key.Decrypt(encrypted)
	}
}

func BenchmarkHashPassword(b *testing.B) {
	for i := 0; i < b.N; i++ {
		HashPassword("benchmark-password")
	}
}

func BenchmarkVerifyPassword(b *testing.B) {
	hash, err := HashPassword("benchmark-password")
	if err != nil {
		b.Fatalf("HashPassword(): %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		VerifyPassword("benchmark-password", hash)
	}
}
