// Package security provides a database adapter for SSH host key storage.
package security

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// DBHostKeyStore adapts storage.DB to the HostKeyStore interface.
type DBHostKeyStore struct {
	db *storage.DB
}

// NewDBHostKeyStore creates a new database-backed host key store.
func NewDBHostKeyStore(db *storage.DB) *DBHostKeyStore {
	return &DBHostKeyStore{db: db}
}

// GetHostKey retrieves a stored host key for the given host, port, and key type.
func (s *DBHostKeyStore) GetHostKey(ctx context.Context, hostname string, port int, keyType string) (*StoredHostKey, error) {
	key, err := s.db.GetSSHHostKey(ctx, hostname, port, keyType)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrHostKeyUnknown
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	pubKey, err := base64.StdEncoding.DecodeString(key.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("decoding stored public key: %w", err)
	}

	return &StoredHostKey{
		Hostname:    key.Hostname,
		Port:        key.Port,
		KeyType:     key.KeyType,
		PublicKey:   pubKey,
		Fingerprint: key.Fingerprint,
		Trusted:     key.Trusted,
		AddedBy:     key.AddedBy,
	}, nil
}

// GetHostKeys retrieves all stored host keys for a given host and port.
func (s *DBHostKeyStore) GetHostKeys(ctx context.Context, hostname string, port int) ([]*StoredHostKey, error) {
	keys, err := s.db.GetSSHHostKeysByHost(ctx, hostname, port)
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}

	result := make([]*StoredHostKey, 0, len(keys))
	for _, key := range keys {
		pubKey, err := base64.StdEncoding.DecodeString(key.PublicKey)
		if err != nil {
			continue // Skip keys with invalid encoding
		}

		result = append(result, &StoredHostKey{
			Hostname:    key.Hostname,
			Port:        key.Port,
			KeyType:     key.KeyType,
			PublicKey:   pubKey,
			Fingerprint: key.Fingerprint,
			Trusted:     key.Trusted,
			AddedBy:     key.AddedBy,
		})
	}

	return result, nil
}

// ListAllKeys retrieves all stored host keys across all hosts.
func (s *DBHostKeyStore) ListAllKeys(ctx context.Context) ([]*StoredHostKey, error) {
	keys, err := s.db.ListSSHHostKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}

	result := make([]*StoredHostKey, 0, len(keys))
	for _, key := range keys {
		pubKey, err := base64.StdEncoding.DecodeString(key.PublicKey)
		if err != nil {
			continue // Skip keys with invalid encoding
		}

		result = append(result, &StoredHostKey{
			Hostname:    key.Hostname,
			Port:        key.Port,
			KeyType:     key.KeyType,
			PublicKey:   pubKey,
			Fingerprint: key.Fingerprint,
			Trusted:     key.Trusted,
			AddedBy:     key.AddedBy,
		})
	}

	return result, nil
}

// StoreHostKey stores a new host key (untrusted by default).
func (s *DBHostKeyStore) StoreHostKey(ctx context.Context, key *StoredHostKey) error {
	dbKey := &storage.SSHHostKey{
		Hostname:    key.Hostname,
		Port:        key.Port,
		KeyType:     key.KeyType,
		PublicKey:   base64.StdEncoding.EncodeToString(key.PublicKey),
		Fingerprint: key.Fingerprint,
		Trusted:     key.Trusted,
		AddedBy:     key.AddedBy,
	}

	return s.db.CreateSSHHostKey(ctx, dbKey)
}

// TrustHostKey marks a host key as trusted.
func (s *DBHostKeyStore) TrustHostKey(ctx context.Context, hostname string, port int, keyType string, trustedBy string) error {
	// First get the key to find its ID
	key, err := s.db.GetSSHHostKey(ctx, hostname, port, keyType)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return ErrHostKeyUnknown
		}
		return fmt.Errorf("database error: %w", err)
	}

	return s.db.UpdateSSHHostKeyTrust(ctx, key.ID, true, trustedBy)
}

// DeleteHostKey removes a host key.
func (s *DBHostKeyStore) DeleteHostKey(ctx context.Context, hostname string, port int, keyType string) error {
	// First get the key to find its ID
	key, err := s.db.GetSSHHostKey(ctx, hostname, port, keyType)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return ErrHostKeyUnknown
		}
		return fmt.Errorf("database error: %w", err)
	}

	return s.db.DeleteSSHHostKey(ctx, key.ID)
}

// Verify DBHostKeyStore implements HostKeyStore
var _ HostKeyStore = (*DBHostKeyStore)(nil)
