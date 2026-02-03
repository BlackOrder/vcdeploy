package security

import (
	"strings"
	"testing"
)

func TestNewConfigEncryptor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		password    string
		shouldError bool
	}{
		{"valid password", "validpassword123", false},
		{"password too short", "short", true},
		{"exactly min length", "123456789012", false},
		{"one char short", "12345678901", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			enc, err := NewConfigEncryptor(tt.password)
			if tt.shouldError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if enc == nil {
					t.Error("expected encryptor, got nil")
				}
			}
		})
	}
}

func TestConfigEncryptor_RoundTrip(t *testing.T) {
	t.Parallel()

	enc, err := NewConfigEncryptor("test-password-123")
	if err != nil {
		t.Fatalf("NewConfigEncryptor() error = %v", err)
	}

	tests := []string{
		"simple string",
		"with special chars: !@#$%^&*()",
		"unicode: 你好世界 🌍",
		"",
		"a",
		strings.Repeat("long", 1000),
	}

	for _, plaintext := range tests {
		encrypted, err := enc.Encrypt(plaintext)
		if err != nil {
			t.Errorf("Encrypt(%q) error = %v", plaintext, err)
			continue
		}

		// Verify it has the prefix
		if !strings.HasPrefix(encrypted, EncryptedValuePrefix) {
			t.Errorf("Encrypt(%q) = %q, missing prefix", plaintext, encrypted)
		}

		decrypted, err := enc.Decrypt(encrypted)
		if err != nil {
			t.Errorf("Decrypt(%q) error = %v", encrypted, err)
			continue
		}

		if decrypted != plaintext {
			t.Errorf("RoundTrip: got %q, want %q", decrypted, plaintext)
		}
	}
}

func TestConfigEncryptor_DifferentPasswords(t *testing.T) {
	t.Parallel()

	enc1, _ := NewConfigEncryptor("password-one-123")
	enc2, _ := NewConfigEncryptor("password-two-456")

	plaintext := "secret value"

	encrypted, err := enc1.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	// Decrypting with different password should fail
	_, err = enc2.Decrypt(encrypted)
	if err == nil {
		t.Error("expected decryption to fail with different password")
	}
}

func TestConfigEncryptor_Decrypt_PlaintextPassthrough(t *testing.T) {
	t.Parallel()

	enc, _ := NewConfigEncryptor("test-password-123")

	// Non-encrypted value should pass through unchanged
	plaintext := "not encrypted value"
	result, err := enc.Decrypt(plaintext)
	if err != nil {
		t.Errorf("Decrypt(plaintext) error = %v", err)
	}
	if result != plaintext {
		t.Errorf("Decrypt(plaintext) = %q, want %q", result, plaintext)
	}
}

func TestIsEncrypted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value    string
		expected bool
	}{
		{"ENC:somebase64data", true},
		{"ENC:", true},
		{"plaintext", false},
		{"", false},
		{"enc:lowercase", false},
		{" ENC:with space", false},
	}

	for _, tt := range tests {
		got := IsEncrypted(tt.value)
		if got != tt.expected {
			t.Errorf("IsEncrypted(%q) = %v, want %v", tt.value, got, tt.expected)
		}
	}
}

func TestConfigEncryptor_UniqueEncryption(t *testing.T) {
	t.Parallel()

	enc, _ := NewConfigEncryptor("test-password-123")

	plaintext := "same value"

	// Encrypting the same value twice should produce different ciphertext (due to random nonce)
	encrypted1, _ := enc.Encrypt(plaintext)
	encrypted2, _ := enc.Encrypt(plaintext)

	if encrypted1 == encrypted2 {
		t.Error("encrypting same value twice produced identical ciphertext (nonce should differ)")
	}

	// But both should decrypt to the same value
	decrypted1, _ := enc.Decrypt(encrypted1)
	decrypted2, _ := enc.Decrypt(encrypted2)

	if decrypted1 != plaintext || decrypted2 != plaintext {
		t.Error("decryption failed for one or both encrypted values")
	}
}

func TestConfigEncryptor_Decrypt_InvalidBase64(t *testing.T) {
	t.Parallel()

	enc, _ := NewConfigEncryptor("test-password-123")

	_, err := enc.Decrypt("ENC:not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestConfigEncryptor_Decrypt_TruncatedCiphertext(t *testing.T) {
	t.Parallel()

	enc, _ := NewConfigEncryptor("test-password-123")

	// Very short ciphertext (less than nonce size)
	_, err := enc.Decrypt("ENC:YWJj") // "abc" in base64
	if err == nil {
		t.Error("expected error for truncated ciphertext")
	}
}

func TestConfigEncryptor_MustDecrypt(t *testing.T) {
	t.Parallel()

	enc, _ := NewConfigEncryptor("test-password-123")

	// Should work for valid encrypted value
	encrypted, _ := enc.Encrypt("test value")
	result := enc.MustDecrypt(encrypted)
	if result != "test value" {
		t.Errorf("MustDecrypt() = %q, want %q", result, "test value")
	}

	// Should pass through plaintext
	plain := enc.MustDecrypt("plaintext")
	if plain != "plaintext" {
		t.Errorf("MustDecrypt(plaintext) = %q, want %q", plain, "plaintext")
	}
}

func TestConfigEncryptor_MustDecrypt_Panics(t *testing.T) {
	t.Parallel()

	enc, _ := NewConfigEncryptor("test-password-123")

	defer func() {
		if r := recover(); r == nil {
			t.Error("MustDecrypt should panic on invalid ciphertext")
		}
	}()

	// This should panic
	enc.MustDecrypt("ENC:invalid-ciphertext-data==")
}
