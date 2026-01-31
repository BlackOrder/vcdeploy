// Package secrets provides secret management with encryption via KMS.
package secrets

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// Ensure Service implements the interface.
var _ services.SecretServicer = (*Service)(nil)

// Service handles secret management with encryption via KMS.
type Service struct {
	store storage.Store
	kms   *security.KMS
}

// Entry represents a decrypted secret.
type Entry struct {
	ID        int64
	Project   string
	Scope     string
	Key       string
	Value     string // Decrypted value
	CreatedAt time.Time
	UpdatedAt time.Time
}

// New creates a new secrets Service.
func New(store storage.Store, kms *security.KMS) *Service {
	return &Service{
		store: store,
		kms:   kms,
	}
}

// Set creates or updates a secret with encryption.
func (s *Service) Set(ctx context.Context, project, scope, key, value string) error {
	if err := ValidateKey(key); err != nil {
		return fmt.Errorf("invalid secret key: %w", err)
	}
	if project == "" {
		return fmt.Errorf("project is required")
	}
	if scope == "" {
		return fmt.Errorf("scope is required")
	}

	// Encrypt the value using KMS
	encrypted, err := s.kms.EncryptString(ctx, value)
	if err != nil {
		return fmt.Errorf("encrypting secret: %w", err)
	}

	// Store the encrypted value
	if err := s.store.SetSecretEncrypted(ctx, project, scope, key, []byte(encrypted)); err != nil {
		return fmt.Errorf("storing secret: %w", err)
	}

	return nil
}

// Get retrieves and decrypts a secret. Returns the decrypted value or empty string if not found.
func (s *Service) Get(ctx context.Context, project, scope, key string) (string, error) {
	secret, err := s.store.GetSecret(ctx, project, scope, key)
	if errors.Is(err, storage.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("retrieving secret: %w", err)
	}

	// Decrypt the value using KMS
	decrypted, err := s.kms.DecryptString(ctx, string(secret.ValueEncrypted))
	if err != nil {
		return "", fmt.Errorf("decrypting secret: %w", err)
	}

	return decrypted, nil
}

// GetEntry retrieves and decrypts a secret with full metadata.
func (s *Service) GetEntry(ctx context.Context, project, scope, key string) (*Entry, error) {
	secret, err := s.store.GetSecret(ctx, project, scope, key)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, services.NotFound("secrets.GetEntry", "secret", project+"/"+scope+"/"+key)
	}
	if err != nil {
		return nil, fmt.Errorf("retrieving secret: %w", err)
	}

	// Decrypt the value using KMS
	decrypted, err := s.kms.DecryptString(ctx, string(secret.ValueEncrypted))
	if err != nil {
		return nil, fmt.Errorf("decrypting secret: %w", err)
	}

	return &Entry{
		ID:        secret.ID,
		Project:   secret.Project,
		Scope:     secret.Scope,
		Key:       secret.Key,
		Value:     decrypted,
		CreatedAt: secret.CreatedAt,
		UpdatedAt: secret.UpdatedAt,
	}, nil
}

// Delete removes a secret.
func (s *Service) Delete(ctx context.Context, project, scope, key string) error {
	return s.store.DeleteSecretCtx(ctx, project, scope, key)
}

// List returns metadata for all secrets in a scope (values not decrypted).
func (s *Service) List(ctx context.Context, project, scope string) ([]services.SecretMetadata, error) {
	secrets, err := s.store.ListSecretsWithScope(ctx, project, scope)
	if err != nil {
		return nil, fmt.Errorf("listing secrets: %w", err)
	}

	result := make([]services.SecretMetadata, len(secrets))
	for i, sec := range secrets {
		result[i] = services.SecretMetadata{
			ID:        sec.ID,
			Project:   sec.Project,
			Scope:     sec.Scope,
			Key:       sec.Key,
			CreatedAt: sec.CreatedAt,
			UpdatedAt: sec.UpdatedAt,
		}
	}
	return result, nil
}

// ListByProject returns metadata for all secrets in a project (all scopes).
func (s *Service) ListByProject(ctx context.Context, project string) ([]services.SecretMetadata, error) {
	secrets, err := s.store.ListSecretsCtx(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("listing secrets: %w", err)
	}

	result := make([]services.SecretMetadata, len(secrets))
	for i, sec := range secrets {
		result[i] = services.SecretMetadata{
			ID:        sec.ID,
			Project:   sec.Project,
			Scope:     sec.Scope,
			Key:       sec.Key,
			CreatedAt: sec.CreatedAt,
			UpdatedAt: sec.UpdatedAt,
		}
	}
	return result, nil
}

// ListAll returns metadata for all secrets across all projects (admin only).
func (s *Service) ListAll(ctx context.Context) ([]services.SecretMetadata, error) {
	secrets, err := s.store.ListAllSecretsCtx(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing secrets: %w", err)
	}

	result := make([]services.SecretMetadata, len(secrets))
	for i, sec := range secrets {
		result[i] = services.SecretMetadata{
			ID:        sec.ID,
			Project:   sec.Project,
			Scope:     sec.Scope,
			Key:       sec.Key,
			CreatedAt: sec.CreatedAt,
			UpdatedAt: sec.UpdatedAt,
		}
	}
	return result, nil
}

// Export returns all decrypted secrets for a scope as a map.
func (s *Service) Export(ctx context.Context, project, scope string) (map[string]string, error) {
	secrets, err := s.store.ListSecretsWithScope(ctx, project, scope)
	if err != nil {
		return nil, fmt.Errorf("listing secrets: %w", err)
	}

	result := make(map[string]string, len(secrets))
	for _, sec := range secrets {
		decrypted, err := s.kms.DecryptString(ctx, string(sec.ValueEncrypted))
		if err != nil {
			return nil, fmt.Errorf("decrypting secret %s: %w", sec.Key, err)
		}
		result[sec.Key] = decrypted
	}
	return result, nil
}

// ExportEnvFile returns secrets formatted as an env file.
func (s *Service) ExportEnvFile(ctx context.Context, project, scope string) (string, error) {
	secrets, err := s.Export(ctx, project, scope)
	if err != nil {
		return "", err
	}

	var lines []string
	for key, value := range secrets {
		// Escape special characters in value
		escaped := strings.ReplaceAll(value, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
		escaped = strings.ReplaceAll(escaped, "\n", "\\n")
		//nolint:gocritic // Using explicit quotes for env file format, not %q
		lines = append(lines, fmt.Sprintf("%s=\"%s\"", key, escaped))
	}
	return strings.Join(lines, "\n"), nil
}

// Import sets multiple secrets from a map.
func (s *Service) Import(ctx context.Context, project, scope string, secrets map[string]string) error {
	for key, value := range secrets {
		if err := s.Set(ctx, project, scope, key, value); err != nil {
			return fmt.Errorf("setting secret %s: %w", key, err)
		}
	}
	return nil
}

// ReEncryptAll re-encrypts all secrets with the current KMS key.
// Useful after key rotation.
func (s *Service) ReEncryptAll(ctx context.Context) error {
	// Get all secrets from all projects/scopes
	secrets, err := s.store.ListAllSecretsCtx(ctx)
	if err != nil {
		return fmt.Errorf("listing all secrets: %w", err)
	}

	for _, sec := range secrets {
		// Re-encrypt using KMS (it handles key versioning)
		reencrypted, err := s.kms.ReEncrypt(ctx, string(sec.ValueEncrypted))
		if err != nil {
			return fmt.Errorf("re-encrypting secret %s/%s/%s: %w", sec.Project, sec.Scope, sec.Key, err)
		}

		if err := s.store.SetSecretEncrypted(ctx, sec.Project, sec.Scope, sec.Key, []byte(reencrypted)); err != nil {
			return fmt.Errorf("storing re-encrypted secret: %w", err)
		}
	}
	return nil
}

// Validation patterns
var (
	keyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
)

// ValidateKey validates a secret key name.
func ValidateKey(key string) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}
	if len(key) > 64 {
		return fmt.Errorf("key cannot exceed 64 characters")
	}
	if !keyPattern.MatchString(key) {
		return fmt.Errorf("key must start with uppercase letter, contain only A-Z, 0-9, underscore")
	}
	return nil
}
