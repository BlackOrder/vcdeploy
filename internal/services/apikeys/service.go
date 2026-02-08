// Package apikeys provides API key management functionality.
package apikeys

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// Ensure Service implements the interface.
var _ services.APIKeyServicer = (*Service)(nil)

// Service handles API key management.
type Service struct {
	store storage.Store
}

// New creates a new API keys Service.
func New(store storage.Store) *Service {
	return &Service{store: store}
}

// Create creates a new API key and returns the raw key (only shown once).
func (s *Service) Create(ctx context.Context, userID string, name string, scopes []string, expiresAt *time.Time) (string, *storage.APIKey, error) {
	// Generate raw key
	rawKey, err := generateAPIKey()
	if err != nil {
		return "", nil, fmt.Errorf("generating API key: %w", err)
	}

	// Hash the key for storage
	hash := hashAPIKey(rawKey)

	// Serialize scopes
	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return "", nil, fmt.Errorf("serializing scopes: %w", err)
	}

	key := &storage.APIKey{
		UserID:    userID,
		Name:      name,
		KeyHash:   hash,
		KeyPrefix: rawKey[:8], // Store first 8 chars for identification
		Scopes:    string(scopesJSON),
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}

	if err := s.store.CreateAPIKey(ctx, key); err != nil {
		return "", nil, fmt.Errorf("creating API key: %w", err)
	}

	return rawKey, key, nil
}

// GetByRawKey retrieves an API key by its raw value.
func (s *Service) GetByRawKey(ctx context.Context, rawKey string) (*storage.APIKey, error) {
	hash := hashAPIKey(rawKey)
	key, err := s.store.GetAPIKeyByHash(ctx, hash)
	if err != nil {
		return nil, err // Returns ErrNotFound if not found
	}

	// Check if expired
	if !key.IsValid() {
		return nil, storage.ErrNotFound
	}

	return key, nil
}

// GetByID retrieves an API key by its ID.
func (s *Service) GetByID(ctx context.Context, keyID string) (*storage.APIKey, error) {
	key, err := s.store.GetAPIKeyByID(ctx, keyID)
	if err != nil {
		return nil, err // Returns ErrNotFound if not found
	}

	// Check if expired
	if !key.IsValid() {
		return nil, storage.ErrNotFound
	}

	return key, nil
}

// Delete removes an API key by ID.
func (s *Service) Delete(ctx context.Context, keyID string) error {
	return s.store.DeleteAPIKey(ctx, keyID)
}

// List returns all API keys for a user.
func (s *Service) List(ctx context.Context, userID string) ([]*storage.APIKey, error) {
	return s.store.ListAPIKeys(ctx, userID)
}

// UpdateUsage updates the last used timestamp for an API key.
func (s *Service) UpdateUsage(ctx context.Context, keyID string) error {
	return s.store.UpdateAPIKeyUsage(ctx, keyID)
}

// CleanupExpired removes all expired API keys.
func (s *Service) CleanupExpired(ctx context.Context) (int64, error) {
	return s.store.CleanupExpiredAPIKeys(ctx, time.Now())
}

// generateAPIKey creates a cryptographically secure API key.
func generateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "vcd_" + hex.EncodeToString(b), nil
}

// hashAPIKey creates a SHA-256 hash of an API key.
func hashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

// GetScopes parses and returns the scopes from an API key.
func GetScopes(key *storage.APIKey) ([]string, error) {
	if key.Scopes == "" {
		return nil, nil
	}
	var scopes []string
	if err := json.Unmarshal([]byte(key.Scopes), &scopes); err != nil {
		return nil, fmt.Errorf("parsing scopes: %w", err)
	}
	return scopes, nil
}
