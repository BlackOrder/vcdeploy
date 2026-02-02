// Package security provides config file encryption for sensitive values.
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/hkdf"
)

const (
	// EncryptedValuePrefix marks encrypted values in config files.
	EncryptedValuePrefix = "ENC:"

	// ConfigEncryptionSalt is the HKDF salt for config encryption.
	ConfigEncryptionSalt = "vcdeploy-config-encryption-v1"
)

// ConfigEncryptor handles encryption/decryption of config values.
type ConfigEncryptor struct {
	key []byte // 32-byte AES-256 key
}

// NewConfigEncryptor creates an encryptor from a master password.
func NewConfigEncryptor(masterPassword string) (*ConfigEncryptor, error) {
	if len(masterPassword) < MinPasswordLength {
		return nil, fmt.Errorf("master password must be at least %d characters", MinPasswordLength)
	}

	// Derive 32-byte key using HKDF
	hkdfReader := hkdf.New(sha256.New, []byte(masterPassword),
		[]byte(ConfigEncryptionSalt), []byte("config-key"))

	key := make([]byte, 32)
	if _, err := hkdfReader.Read(key); err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}

	return &ConfigEncryptor{key: key}, nil
}

// Encrypt encrypts a plaintext value, returning "ENC:<base64>".
func (e *ConfigEncryptor) Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return EncryptedValuePrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts an "ENC:<base64>" value back to plaintext.
// If the value is not encrypted (no prefix), returns it as-is.
func (e *ConfigEncryptor) Decrypt(encrypted string) (string, error) {
	if !strings.HasPrefix(encrypted, EncryptedValuePrefix) {
		return encrypted, nil // Not encrypted, return as-is
	}

	ciphertext, err := base64.StdEncoding.DecodeString(
		strings.TrimPrefix(encrypted, EncryptedValuePrefix))
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}

// IsEncrypted checks if a value is encrypted.
func IsEncrypted(value string) bool {
	return strings.HasPrefix(value, EncryptedValuePrefix)
}

// MustDecrypt decrypts a value and panics on error.
// Useful for configuration loading where errors should be fatal.
func (e *ConfigEncryptor) MustDecrypt(encrypted string) string {
	result, err := e.Decrypt(encrypted)
	if err != nil {
		panic(fmt.Sprintf("failed to decrypt config value: %v", err))
	}
	return result
}
