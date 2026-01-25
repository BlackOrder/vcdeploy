// Package security provides security-related functionality for vcdeploy.
package security

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// SecretService handles secret management with encryption via KMS.
type SecretService struct {
	db  *storage.DB
	kms *KMS
}

// SecretEntry represents a decrypted secret.
type SecretEntry struct {
	ID        int64
	Project   string
	Scope     string
	Key       string
	Value     string // Decrypted value
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SecretMetadata represents secret info without the value.
type SecretMetadata struct {
	ID        int64
	Project   string
	Scope     string
	Key       string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewSecretService creates a new SecretService.
func NewSecretService(db *storage.DB, kms *KMS) *SecretService {
	return &SecretService{
		db:  db,
		kms: kms,
	}
}

// Set creates or updates a secret with encryption.
func (s *SecretService) Set(ctx context.Context, project, scope, key, value string) error {
	if err := ValidateSecretKey(key); err != nil {
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
	if err := s.db.SetSecretEncrypted(ctx, project, scope, key, []byte(encrypted)); err != nil {
		return fmt.Errorf("storing secret: %w", err)
	}

	return nil
}

// Get retrieves and decrypts a secret.
func (s *SecretService) Get(ctx context.Context, project, scope, key string) (*SecretEntry, error) {
	secret, err := s.db.GetSecret(ctx, project, scope, key)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("retrieving secret: %w", err)
	}

	// Decrypt the value using KMS
	decrypted, err := s.kms.DecryptString(ctx, string(secret.ValueEncrypted))
	if err != nil {
		return nil, fmt.Errorf("decrypting secret: %w", err)
	}

	return &SecretEntry{
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
func (s *SecretService) Delete(ctx context.Context, project, scope, key string) error {
	return s.db.DeleteSecretCtx(ctx, project, scope, key)
}

// List returns metadata for all secrets in a scope (values not decrypted).
func (s *SecretService) List(ctx context.Context, project, scope string) ([]*SecretMetadata, error) {
	secrets, err := s.db.ListSecretsWithScope(ctx, project, scope)
	if err != nil {
		return nil, fmt.Errorf("listing secrets: %w", err)
	}

	result := make([]*SecretMetadata, len(secrets))
	for i, sec := range secrets {
		result[i] = &SecretMetadata{
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
func (s *SecretService) Export(ctx context.Context, project, scope string) (map[string]string, error) {
	secrets, err := s.db.ListSecretsWithScope(ctx, project, scope)
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
func (s *SecretService) ExportEnvFile(ctx context.Context, project, scope string) (string, error) {
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
		lines = append(lines, fmt.Sprintf("%s=\"%s\"", key, escaped))
	}
	return strings.Join(lines, "\n"), nil
}

// Import sets multiple secrets from a map.
func (s *SecretService) Import(ctx context.Context, project, scope string, secrets map[string]string) error {
	for key, value := range secrets {
		if err := s.Set(ctx, project, scope, key, value); err != nil {
			return fmt.Errorf("setting secret %s: %w", key, err)
		}
	}
	return nil
}

// ReEncryptAll re-encrypts all secrets with the current KMS key.
// Useful after key rotation.
func (s *SecretService) ReEncryptAll(ctx context.Context) error {
	// Get all secrets from all projects/scopes
	secrets, err := s.db.ListAllSecretsCtx(ctx)
	if err != nil {
		return fmt.Errorf("listing all secrets: %w", err)
	}

	for _, sec := range secrets {
		// Re-encrypt using KMS (it handles key versioning)
		reencrypted, err := s.kms.ReEncrypt(ctx, string(sec.ValueEncrypted))
		if err != nil {
			return fmt.Errorf("re-encrypting secret %s/%s/%s: %w", sec.Project, sec.Scope, sec.Key, err)
		}

		if err := s.db.SetSecretEncrypted(ctx, sec.Project, sec.Scope, sec.Key, []byte(reencrypted)); err != nil {
			return fmt.Errorf("storing re-encrypted secret: %w", err)
		}
	}
	return nil
}

// Validation patterns
var (
	secretKeyPattern   = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
	projectNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,63}$`)
)

// ValidateSecretKey validates a secret key name.
func ValidateSecretKey(key string) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}
	if len(key) > 64 {
		return fmt.Errorf("key cannot exceed 64 characters")
	}
	if !secretKeyPattern.MatchString(key) {
		return fmt.Errorf("key must start with uppercase letter, contain only A-Z, 0-9, underscore")
	}
	return nil
}

// ValidateProjectName validates a project name.
func ValidateProjectName(name string) error {
	if name == "" {
		return fmt.Errorf("project name cannot be empty")
	}
	if len(name) > 64 {
		return fmt.Errorf("project name cannot exceed 64 characters")
	}
	if !projectNamePattern.MatchString(name) {
		return fmt.Errorf("project name must start with letter, contain only letters, numbers, hyphens, underscores")
	}
	return nil
}
