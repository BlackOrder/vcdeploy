// Package security provides encryption and authentication utilities.
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// MasterKey holds the encryption key and related operations.
type MasterKey struct {
	key   []byte
	keyID string
}

// LoadMasterKey loads the master key from file or environment.
func LoadMasterKey(keyPath string) (*MasterKey, error) {
	// First check environment variable
	if envKey := os.Getenv("VCDEPLOY_MASTER_KEY"); envKey != "" {
		key, err := base64.StdEncoding.DecodeString(envKey)
		if err != nil {
			return nil, fmt.Errorf("invalid VCDEPLOY_MASTER_KEY: %w", err)
		}
		if len(key) != 32 {
			return nil, fmt.Errorf("master key must be 32 bytes")
		}
		return &MasterKey{key: key, keyID: hashKeyID(key)}, nil
	}

	// Load from file
	data, err := os.ReadFile(keyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("master key not found at %s (run 'vcdeploy init' to generate)", keyPath)
		}
		return nil, fmt.Errorf("reading master key: %w", err)
	}

	key, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return nil, fmt.Errorf("invalid master key format: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("master key must be 32 bytes")
	}

	return &MasterKey{key: key, keyID: hashKeyID(key)}, nil
}

// GenerateMasterKey creates a new random master key.
func GenerateMasterKey() (*MasterKey, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generating random key: %w", err)
	}
	return &MasterKey{key: key, keyID: hashKeyID(key)}, nil
}

// SaveToFile saves the master key to a file with restricted permissions.
func (k *MasterKey) SaveToFile(path string) error {
	encoded := base64.StdEncoding.EncodeToString(k.key)
	if err := os.WriteFile(path, []byte(encoded), 0600); err != nil {
		return fmt.Errorf("writing master key: %w", err)
	}
	return nil
}

// KeyID returns the key identifier (hash of key).
func (k *MasterKey) KeyID() string {
	return k.keyID
}

// Encrypt encrypts plaintext using AES-256-GCM.
func (k *MasterKey) Encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(k.key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}

	// Prepend nonce to ciphertext
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext using AES-256-GCM.
func (k *MasterKey) Decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(k.key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	if len(ciphertext) < gcm.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypting: %w", err)
	}

	return plaintext, nil
}

func hashKeyID(key []byte) string {
	h := sha256.Sum256(key)
	return base64.StdEncoding.EncodeToString(h[:8])
}

// --- Password hashing ---

// HashPassword creates a bcrypt hash of the password.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword checks if password matches the hash.
func VerifyPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// --- Random string generation ---

// GenerateRandomPassword generates a secure random password.
func GenerateRandomPassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating random bytes: %w", err)
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b), nil
}

// GenerateToken generates a secure random token.
func GenerateToken(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating random bytes: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// GenerateSessionID generates a session ID.
func GenerateSessionID() (string, error) {
	return GenerateToken(32)
}

// --- Passphrase-based encryption ---

// EncryptWithPassphrase encrypts data using a passphrase with Argon2 key derivation.
func EncryptWithPassphrase(plaintext, passphrase []byte) ([]byte, error) {
	// Generate a random salt
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generating salt: %w", err)
	}

	// Derive key from passphrase using Argon2id
	key := argon2.IDKey(passphrase, salt, 1, 64*1024, 4, 32)

	// Encrypt with AES-256-GCM
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}

	// Format: salt (16 bytes) + nonce (12 bytes) + ciphertext
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	result := make([]byte, len(salt)+len(nonce)+len(ciphertext))
	copy(result, salt)
	copy(result[len(salt):], nonce)
	copy(result[len(salt)+len(nonce):], ciphertext)

	return result, nil
}

// DecryptWithPassphrase decrypts data using a passphrase.
func DecryptWithPassphrase(ciphertext, passphrase []byte) ([]byte, error) {
	if len(ciphertext) < 28 { // 16 (salt) + 12 (nonce)
		return nil, fmt.Errorf("ciphertext too short")
	}

	// Extract salt, nonce, and encrypted data
	salt := ciphertext[:16]
	nonce := ciphertext[16:28]
	encryptedData := ciphertext[28:]

	// Derive key from passphrase using same parameters
	key := argon2.IDKey(passphrase, salt, 1, 64*1024, 4, 32)

	// Decrypt with AES-256-GCM
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, encryptedData, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypting (wrong passphrase?): %w", err)
	}

	return plaintext, nil
}

// GenerateSecureToken generates a cryptographically secure random token.
// The returned token is hex-encoded, so it will be 2*length characters long.
func GenerateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generating secure token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}
