// Package hostkeys provides SSH host key management functionality.
package hostkeys

import (
	"context"
	"fmt"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// Ensure Service implements the interface.
var _ services.HostKeyServicer = (*Service)(nil)

// Service handles SSH host key management.
type Service struct {
	store storage.Store
}

// New creates a new host keys Service.
func New(store storage.Store) *Service {
	return &Service{store: store}
}

// Create creates a new SSH host key record.
func (s *Service) Create(ctx context.Context, key *storage.SSHHostKey) error {
	if key.CreatedAt.IsZero() {
		key.CreatedAt = time.Now()
	}
	if err := s.store.CreateSSHHostKey(ctx, key); err != nil {
		return fmt.Errorf("creating SSH host key: %w", err)
	}
	return nil
}

// Get retrieves an SSH host key by hostname, port, and key type.
func (s *Service) Get(ctx context.Context, hostname string, port int, keyType string) (*storage.SSHHostKey, error) {
	key, err := s.store.GetSSHHostKey(ctx, hostname, port, keyType)
	if err != nil {
		return nil, err // Returns ErrNotFound if not found
	}
	return key, nil
}

// GetByHost retrieves all SSH host keys for a hostname and port.
func (s *Service) GetByHost(ctx context.Context, hostname string, port int) ([]*storage.SSHHostKey, error) {
	keys, err := s.store.GetSSHHostKeysByHost(ctx, hostname, port)
	if err != nil {
		return nil, fmt.Errorf("getting host keys: %w", err)
	}
	return keys, nil
}

// List retrieves all SSH host keys.
func (s *Service) List(ctx context.Context) ([]*storage.SSHHostKey, error) {
	keys, err := s.store.ListSSHHostKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing host keys: %w", err)
	}
	return keys, nil
}

// UpdateTrust updates the trust status of an SSH host key.
func (s *Service) UpdateTrust(ctx context.Context, id int64, trusted bool, verifiedBy string) error {
	if err := s.store.UpdateSSHHostKeyTrust(ctx, id, trusted, verifiedBy); err != nil {
		return fmt.Errorf("updating host key trust: %w", err)
	}
	return nil
}

// Delete removes an SSH host key by ID.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if err := s.store.DeleteSSHHostKey(ctx, id); err != nil {
		return fmt.Errorf("deleting host key: %w", err)
	}
	return nil
}

// DeleteByHost removes all SSH host keys for a hostname and port.
func (s *Service) DeleteByHost(ctx context.Context, hostname string, port int) (int64, error) {
	count, err := s.store.DeleteSSHHostKeysByHost(ctx, hostname, port)
	if err != nil {
		return 0, fmt.Errorf("deleting host keys: %w", err)
	}
	return count, nil
}

// IsTrusted checks if a host key is trusted.
func (s *Service) IsTrusted(ctx context.Context, hostname string, port int, keyType, fingerprint string) (bool, error) {
	key, err := s.store.GetSSHHostKey(ctx, hostname, port, keyType)
	if err != nil {
		if services.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("getting host key: %w", err)
	}

	return key.Trusted && key.Fingerprint == fingerprint, nil
}
