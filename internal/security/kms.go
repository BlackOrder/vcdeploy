// Package security provides encryption and authentication utilities.
package security

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// KMS provides AWS KMS-style key management with versioning and rotation.
// Keys are never deleted, only deactivated for decryption backward compatibility.
// Ciphertext format: v1:{key_id}:{base64_nonce}:{base64_ciphertext}
type KMS struct {
	store      storage.Store
	logger     *zap.Logger
	cache      map[string]*EncryptionKey
	cacheMu    sync.RWMutex
	currentKey *EncryptionKey
	currentMu  sync.RWMutex
}

// EncryptionKey represents a versioned encryption key.
type EncryptionKey struct {
	ID                  string
	Version             int
	KeyMaterial         []byte // Raw 32-byte AES-256 key
	Algorithm           string
	Status              KeyStatus
	CreatedAt           time.Time
	ActivatedAt         *time.Time
	DeactivatedAt       *time.Time
	ScheduledDeletionAt *time.Time
	DeletionCancelledAt *time.Time
}

// KeyStatus represents the lifecycle status of an encryption key.
type KeyStatus string

const (
	KeyStatusPending   KeyStatus = "pending"   // Created but not yet active
	KeyStatusActive    KeyStatus = "active"    // Current key for encryption
	KeyStatusInactive  KeyStatus = "inactive"  // Can decrypt but not encrypt
	KeyStatusScheduled KeyStatus = "scheduled" // Scheduled for deletion (grace period)
	KeyStatusDeleted   KeyStatus = "deleted"   // Logically deleted (still retained)
)

// KMSConfig holds configuration for the KMS service.
type KMSConfig struct {
	// DeletionGracePeriod is the time before a scheduled key is deleted (default 30 days)
	DeletionGracePeriod time.Duration
	// AutoRotationPeriod triggers automatic key rotation (0 = disabled)
	AutoRotationPeriod time.Duration
}

// DefaultKMSConfig returns default KMS configuration.
func DefaultKMSConfig() KMSConfig {
	return KMSConfig{
		DeletionGracePeriod: 30 * 24 * time.Hour, // 30 days
		AutoRotationPeriod:  0,                   // Disabled by default
	}
}

// NewKMS creates a new KMS service backed by the store.
func NewKMS(ctx context.Context, store storage.Store, logger *zap.Logger) (*KMS, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	kms := &KMS{
		store:  store,
		logger: logger,
		cache:  make(map[string]*EncryptionKey),
	}

	// Load current active key
	if err := kms.loadCurrentKey(ctx); err != nil {
		return nil, err
	}

	return kms, nil
}

// Initialize sets up the KMS with an initial key if none exists.
// This should be called during system initialization.
func (k *KMS) Initialize(ctx context.Context) error {
	k.currentMu.Lock()
	defer k.currentMu.Unlock()

	// Check if we already have a key
	if k.currentKey != nil {
		return nil
	}

	// Generate initial key
	key, err := k.generateKey(ctx)
	if err != nil {
		return fmt.Errorf("generate initial key: %w", err)
	}

	// Save to database
	key.Status = KeyStatusActive
	now := time.Now()
	key.ActivatedAt = &now

	if err := k.saveKey(ctx, key); err != nil {
		return fmt.Errorf("save initial key: %w", err)
	}

	k.currentKey = key
	k.cacheKey(key)

	return nil
}

// Encrypt encrypts plaintext using the current active key.
// Returns versioned ciphertext format: v1:{key_id}:{nonce}:{ciphertext}
func (k *KMS) Encrypt(ctx context.Context, plaintext []byte) (string, error) {
	k.currentMu.RLock()
	key := k.currentKey
	k.currentMu.RUnlock()

	if key == nil {
		return "", fmt.Errorf("no active encryption key")
	}

	if key.Status != KeyStatusActive {
		return "", fmt.Errorf("current key is not active")
	}

	// Create cipher
	block, err := aes.NewCipher(key.KeyMaterial)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	// Encrypt
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	// Log key usage
	if err := k.logKeyUsage(ctx, key.ID, "encrypt", "", ""); err != nil {
		// Log error but don't fail encryption
		k.logger.Warn("failed to log key usage for encrypt", zap.String("keyID", key.ID), zap.Error(err))
	}

	// Format: v1:{key_id}:{base64_nonce}:{base64_ciphertext}
	return fmt.Sprintf("v1:%s:%s:%s",
		key.ID,
		base64.StdEncoding.EncodeToString(nonce),
		base64.StdEncoding.EncodeToString(ciphertext),
	), nil
}

// Decrypt decrypts versioned ciphertext using the appropriate key.
func (k *KMS) Decrypt(ctx context.Context, versioned string) ([]byte, error) {
	// Parse format: v1:{key_id}:{nonce}:{ciphertext}
	parts := strings.SplitN(versioned, ":", 4)
	if len(parts) != 4 {
		return nil, fmt.Errorf("invalid ciphertext format")
	}

	version := parts[0]
	if version != "v1" {
		return nil, fmt.Errorf("unsupported ciphertext version: %s", version)
	}

	keyID := parts[1]
	nonceB64 := parts[2]
	ciphertextB64 := parts[3]

	// Decode base64
	nonce, err := base64.StdEncoding.DecodeString(nonceB64)
	if err != nil {
		return nil, fmt.Errorf("decode nonce: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}

	// Get key
	key, err := k.getKey(ctx, keyID)
	if err != nil {
		return nil, fmt.Errorf("get key %s: %w", keyID, err)
	}

	if key.Status == KeyStatusDeleted {
		return nil, fmt.Errorf("key %s has been deleted", keyID)
	}

	// Create cipher
	block, err := aes.NewCipher(key.KeyMaterial)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	// Log key usage
	if err := k.logKeyUsage(ctx, key.ID, "decrypt", "", ""); err != nil {
		k.logger.Warn("failed to log key usage for decrypt", zap.String("keyID", key.ID), zap.Error(err))
	}

	return plaintext, nil
}

// RotateKey creates a new active key and deactivates the current one.
// The old key is retained for decryption of existing ciphertext.
func (k *KMS) RotateKey(ctx context.Context) (*EncryptionKey, error) {
	k.currentMu.Lock()
	defer k.currentMu.Unlock()

	// Generate new key
	newKey, err := k.generateKey(ctx)
	if err != nil {
		return nil, fmt.Errorf("generate new key: %w", err)
	}

	now := time.Now()
	newKey.Status = KeyStatusActive
	newKey.ActivatedAt = &now

	// Deactivate current key
	if k.currentKey != nil {
		if err := k.store.UpdateEncryptionKeyStatus(ctx, k.currentKey.ID, string(KeyStatusInactive), nil); err != nil {
			return nil, fmt.Errorf("deactivate current key: %w", err)
		}
		k.currentKey.Status = KeyStatusInactive
		k.currentKey.DeactivatedAt = &now
	}

	// Save new key
	storageKey := k.toStorageKey(newKey)
	if err := k.store.SaveEncryptionKey(ctx, storageKey); err != nil {
		return nil, fmt.Errorf("save new key: %w", err)
	}

	k.currentKey = newKey
	k.cacheKey(newKey)

	return newKey, nil
}

// ScheduleKeyDeletion schedules a key for deletion after the grace period.
// Keys can only be scheduled if they are inactive.
func (k *KMS) ScheduleKeyDeletion(ctx context.Context, keyID string, gracePeriod time.Duration) error {
	key, err := k.getKey(ctx, keyID)
	if err != nil {
		return err
	}

	if key.Status == KeyStatusActive {
		return fmt.Errorf("cannot schedule deletion of active key")
	}

	if key.Status == KeyStatusScheduled {
		return fmt.Errorf("key is already scheduled for deletion")
	}

	if key.Status == KeyStatusDeleted {
		return fmt.Errorf("key is already deleted")
	}

	scheduledTime := time.Now().Add(gracePeriod)
	if err := k.store.UpdateEncryptionKeyStatus(ctx, keyID, string(KeyStatusScheduled), &scheduledTime); err != nil {
		return fmt.Errorf("schedule deletion: %w", err)
	}

	// Update cache
	key.Status = KeyStatusScheduled
	key.ScheduledDeletionAt = &scheduledTime
	key.DeletionCancelledAt = nil
	k.cacheKey(key)

	return nil
}

// CancelKeyDeletion cancels a scheduled key deletion.
func (k *KMS) CancelKeyDeletion(ctx context.Context, keyID string) error {
	key, err := k.getKey(ctx, keyID)
	if err != nil {
		return err
	}

	if key.Status != KeyStatusScheduled {
		return fmt.Errorf("key is not scheduled for deletion")
	}

	if err := k.store.UpdateEncryptionKeyStatus(ctx, keyID, string(KeyStatusInactive), nil); err != nil {
		return fmt.Errorf("cancel deletion: %w", err)
	}

	// Update cache
	now := time.Now()
	key.Status = KeyStatusInactive
	key.ScheduledDeletionAt = nil
	key.DeletionCancelledAt = &now
	k.cacheKey(key)

	return nil
}

// DeleteKeyNow immediately marks a key as deleted (with --confirm-destroy).
// The key material is retained but marked as deleted.
func (k *KMS) DeleteKeyNow(ctx context.Context, keyID string) error {
	key, err := k.getKey(ctx, keyID)
	if err != nil {
		return err
	}

	if key.Status == KeyStatusActive {
		return fmt.Errorf("cannot delete active key")
	}

	if key.Status == KeyStatusDeleted {
		return nil // Already deleted
	}

	if err := k.store.UpdateEncryptionKeyStatus(ctx, keyID, string(KeyStatusDeleted), nil); err != nil {
		return fmt.Errorf("delete key: %w", err)
	}

	// Update cache
	key.Status = KeyStatusDeleted
	k.cacheKey(key)

	return nil
}

// ProcessScheduledDeletions processes keys that have passed their deletion grace period.
// This should be called periodically by a background job.
func (k *KMS) ProcessScheduledDeletions(ctx context.Context) (int, error) {
	keys, err := k.store.ListEncryptionKeys(ctx)
	if err != nil {
		return 0, fmt.Errorf("list keys: %w", err)
	}

	now := time.Now()
	count := 0

	for _, storageKey := range keys {
		if storageKey.Status == string(KeyStatusScheduled) && storageKey.ScheduledDeletionAt != nil && storageKey.ScheduledDeletionAt.Before(now) {
			if err := k.store.UpdateEncryptionKeyStatus(ctx, storageKey.ID, string(KeyStatusDeleted), nil); err != nil {
				k.logger.Warn("failed to process scheduled deletion", zap.String("keyID", storageKey.ID), zap.Error(err))
				continue
			}
			count++
		}
	}

	// Clear cache if any keys were deleted
	if count > 0 {
		k.invalidateCache()
	}

	return count, nil
}

// ListKeys returns all encryption keys.
func (k *KMS) ListKeys(ctx context.Context) ([]*EncryptionKey, error) {
	storageKeys, err := k.store.ListEncryptionKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("list keys: %w", err)
	}

	keys := make([]*EncryptionKey, 0, len(storageKeys))
	for _, sk := range storageKeys {
		keys = append(keys, k.fromStorageKey(sk))
	}

	return keys, nil
}

// GetCurrentKey returns the current active encryption key.
func (k *KMS) GetCurrentKey() *EncryptionKey {
	k.currentMu.RLock()
	defer k.currentMu.RUnlock()
	return k.currentKey
}

// ReEncrypt re-encrypts ciphertext with the current key.
// Useful for rotating encrypted data after key rotation.
func (k *KMS) ReEncrypt(ctx context.Context, versioned string) (string, error) {
	// Decrypt with old key
	plaintext, err := k.Decrypt(ctx, versioned)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	// Encrypt with current key
	return k.Encrypt(ctx, plaintext)
}

// --- Internal methods ---

func (k *KMS) loadCurrentKey(ctx context.Context) error {
	storageKey, err := k.store.GetCurrentEncryptionKey(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			// No active key yet
			return nil
		}
		return fmt.Errorf("load current key: %w", err)
	}

	key := k.fromStorageKey(storageKey)
	k.currentKey = key
	k.cacheKey(key)
	return nil
}

func (k *KMS) generateKey(ctx context.Context) (*EncryptionKey, error) {
	// Generate random key ID
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return nil, fmt.Errorf("generate id: %w", err)
	}
	keyID := base64.URLEncoding.EncodeToString(idBytes)

	// Generate key material
	keyMaterial := make([]byte, 32) // AES-256
	if _, err := rand.Read(keyMaterial); err != nil {
		return nil, fmt.Errorf("generate key material: %w", err)
	}

	// Get next version by listing existing keys
	keys, _ := k.store.ListEncryptionKeys(ctx)
	version := 1
	for _, existingKey := range keys {
		if existingKey.Version >= version {
			version = existingKey.Version + 1
		}
	}

	return &EncryptionKey{
		ID:          keyID,
		Version:     version,
		KeyMaterial: keyMaterial,
		Algorithm:   "AES-256-GCM",
		Status:      KeyStatusPending,
		CreatedAt:   time.Now(),
	}, nil
}

func (k *KMS) saveKey(ctx context.Context, key *EncryptionKey) error {
	storageKey := k.toStorageKey(key)
	return k.store.SaveEncryptionKey(ctx, storageKey)
}

func (k *KMS) getKey(ctx context.Context, keyID string) (*EncryptionKey, error) {
	// Check cache first
	k.cacheMu.RLock()
	if key, ok := k.cache[keyID]; ok {
		k.cacheMu.RUnlock()
		return key, nil
	}
	k.cacheMu.RUnlock()

	// Load from store
	storageKey, err := k.store.GetEncryptionKey(ctx, keyID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, fmt.Errorf("key not found: %s", keyID)
		}
		return nil, fmt.Errorf("load key: %w", err)
	}

	key := k.fromStorageKey(storageKey)
	k.cacheKey(key)
	return key, nil
}

func (k *KMS) cacheKey(key *EncryptionKey) {
	k.cacheMu.Lock()
	k.cache[key.ID] = key
	k.cacheMu.Unlock()
}

func (k *KMS) invalidateCache() {
	k.cacheMu.Lock()
	k.cache = make(map[string]*EncryptionKey)
	k.cacheMu.Unlock()
}

func (k *KMS) logKeyUsage(ctx context.Context, keyID, operation, resourceType, resourceID string) error {
	// Key usage logging is now optional - skipped if encryption_key_usage table doesn't exist
	// The main audit logging is handled through cert_audit_events
	k.logger.Debug("key usage",
		zap.String("keyID", keyID),
		zap.String("operation", operation),
		zap.String("resourceType", resourceType),
		zap.String("resourceID", resourceID))
	return nil
}

// --- Type conversion helpers ---

func (k *KMS) toStorageKey(key *EncryptionKey) *storage.EncryptionKey {
	return &storage.EncryptionKey{
		ID:                  key.ID,
		Version:             key.Version,
		KeyMaterialEnc:      key.KeyMaterial,
		Algorithm:           key.Algorithm,
		Status:              string(key.Status),
		CreatedAt:           key.CreatedAt,
		ActivatedAt:         key.ActivatedAt,
		DeactivatedAt:       key.DeactivatedAt,
		ScheduledDeletionAt: key.ScheduledDeletionAt,
		DeletionCancelledAt: key.DeletionCancelledAt,
	}
}

func (k *KMS) fromStorageKey(sk *storage.EncryptionKey) *EncryptionKey {
	return &EncryptionKey{
		ID:                  sk.ID,
		Version:             sk.Version,
		KeyMaterial:         sk.KeyMaterialEnc,
		Algorithm:           sk.Algorithm,
		Status:              KeyStatus(sk.Status),
		CreatedAt:           sk.CreatedAt,
		ActivatedAt:         sk.ActivatedAt,
		DeactivatedAt:       sk.DeactivatedAt,
		ScheduledDeletionAt: sk.ScheduledDeletionAt,
		DeletionCancelledAt: sk.DeletionCancelledAt,
	}
}

// --- Helper functions for using KMS with existing secrets ---

// EncryptString encrypts a string and returns versioned ciphertext.
func (k *KMS) EncryptString(ctx context.Context, plaintext string) (string, error) {
	return k.Encrypt(ctx, []byte(plaintext))
}

// DecryptString decrypts versioned ciphertext and returns a string.
func (k *KMS) DecryptString(ctx context.Context, versioned string) (string, error) {
	plaintext, err := k.Decrypt(ctx, versioned)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// IsVersionedCiphertext checks if a string is in versioned ciphertext format.
func IsVersionedCiphertext(s string) bool {
	return strings.HasPrefix(s, "v1:")
}
