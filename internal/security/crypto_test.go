package security

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateMasterKey(t *testing.T) {
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
	key1, _ := GenerateMasterKey()
	key2, _ := GenerateMasterKey()
	if key1.KeyID() == key2.KeyID() {
		t.Error("Generated keys should have different IDs")
	}
}

func TestMasterKeyEncryptDecrypt(t *testing.T) {
	key, _ := GenerateMasterKey()

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
		t.Run(tt.name, func(t *testing.T) {
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
	key, _ := GenerateMasterKey()
	plaintext := []byte("same message")

	enc1, _ := key.Encrypt(plaintext)
	enc2, _ := key.Encrypt(plaintext)

	if bytes.Equal(enc1, enc2) {
		t.Error("Same plaintext should produce different ciphertext due to random nonce")
	}
}

func TestMasterKeyDecryptWrongKey(t *testing.T) {
	key1, _ := GenerateMasterKey()
	key2, _ := GenerateMasterKey()

	encrypted, _ := key1.Encrypt([]byte("secret"))
	_, err := key2.Decrypt(encrypted)
	if err == nil {
		t.Error("Decrypt with wrong key should fail")
	}
}

func TestMasterKeyDecryptTamperedData(t *testing.T) {
	key, _ := GenerateMasterKey()
	encrypted, _ := key.Encrypt([]byte("secret"))

	// Tamper with the ciphertext
	encrypted[len(encrypted)-1] ^= 0xFF

	_, err := key.Decrypt(encrypted)
	if err == nil {
		t.Error("Decrypt of tampered data should fail")
	}
}

func TestMasterKeyDecryptTooShort(t *testing.T) {
	key, _ := GenerateMasterKey()
	_, err := key.Decrypt([]byte("short"))
	if err == nil {
		t.Error("Decrypt of too-short data should fail")
	}
}

func TestMasterKeySaveAndLoad(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "vcdeploy-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	keyPath := filepath.Join(tmpDir, "master.key")

	// Generate and save
	key1, _ := GenerateMasterKey()
	if err := key1.SaveToFile(keyPath); err != nil {
		t.Fatalf("SaveToFile() error = %v", err)
	}

	// Check permissions
	info, _ := os.Stat(keyPath)
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
	encrypted, _ := key1.Encrypt(plaintext)
	decrypted, _ := key2.Decrypt(encrypted)

	if !bytes.Equal(decrypted, plaintext) {
		t.Error("Loaded key should decrypt data encrypted by original key")
	}
}

func TestLoadMasterKeyFromEnv(t *testing.T) {
	// Generate a valid key and encode it
	key, _ := GenerateMasterKey()
	tmpDir, _ := os.MkdirTemp("", "vcdeploy-test-*")
	defer os.RemoveAll(tmpDir)
	keyPath := filepath.Join(tmpDir, "master.key")
	key.SaveToFile(keyPath)

	// Read the encoded key
	data, _ := os.ReadFile(keyPath)
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
	os.Setenv("VCDEPLOY_MASTER_KEY", "invalid-base64!")
	defer os.Unsetenv("VCDEPLOY_MASTER_KEY")

	_, err := LoadMasterKey("/nonexistent")
	if err == nil {
		t.Error("LoadMasterKey with invalid env should fail")
	}
}

func TestLoadMasterKeyWrongSize(t *testing.T) {
	os.Setenv("VCDEPLOY_MASTER_KEY", "dG9vLXNob3J0") // "too-short" in base64
	defer os.Unsetenv("VCDEPLOY_MASTER_KEY")

	_, err := LoadMasterKey("/nonexistent")
	if err == nil {
		t.Error("LoadMasterKey with wrong size should fail")
	}
}

func TestLoadMasterKeyNotFound(t *testing.T) {
	_, err := LoadMasterKey("/nonexistent/path/key")
	if err == nil {
		t.Error("LoadMasterKey with nonexistent file should fail")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Error should mention 'not found', got: %v", err)
	}
}

func TestHashPassword(t *testing.T) {
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
	password := "same-password"
	hash1, _ := HashPassword(password)
	hash2, _ := HashPassword(password)

	if hash1 == hash2 {
		t.Error("Same password should produce different hashes (salt)")
	}

	// Both should still verify
	if !VerifyPassword(password, hash1) || !VerifyPassword(password, hash2) {
		t.Error("Both hashes should verify correctly")
	}
}

func TestGenerateRandomPassword(t *testing.T) {
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
	pass1, _ := GenerateRandomPassword(16)
	pass2, _ := GenerateRandomPassword(16)
	if pass1 == pass2 {
		t.Error("Generated passwords should be unique")
	}
}

func TestGenerateToken(t *testing.T) {
	token, err := GenerateToken(32)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	if token == "" {
		t.Error("Token should not be empty")
	}
}

func TestGenerateTokenUnique(t *testing.T) {
	token1, _ := GenerateToken(32)
	token2, _ := GenerateToken(32)
	if token1 == token2 {
		t.Error("Generated tokens should be unique")
	}
}

func TestGenerateSessionID(t *testing.T) {
	sid1, err := GenerateSessionID()
	if err != nil {
		t.Fatalf("GenerateSessionID() error = %v", err)
	}
	sid2, _ := GenerateSessionID()

	if sid1 == "" {
		t.Error("Session ID should not be empty")
	}
	if sid1 == sid2 {
		t.Error("Session IDs should be unique")
	}
}

// Benchmarks
func BenchmarkGenerateMasterKey(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GenerateMasterKey()
	}
}

func BenchmarkEncrypt(b *testing.B) {
	key, _ := GenerateMasterKey()
	plaintext := []byte("benchmark test message for encryption")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key.Encrypt(plaintext)
	}
}

func BenchmarkDecrypt(b *testing.B) {
	key, _ := GenerateMasterKey()
	plaintext := []byte("benchmark test message for decryption")
	encrypted, _ := key.Encrypt(plaintext)
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
	hash, _ := HashPassword("benchmark-password")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		VerifyPassword("benchmark-password", hash)
	}
}
