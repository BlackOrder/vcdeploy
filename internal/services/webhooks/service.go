// Package webhooks provides project webhook configuration management.
package webhooks

import (
	"context"
	"fmt"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// Ensure Service implements the interface.
var _ services.WebhookServicer = (*Service)(nil)

// Service handles project webhook configuration.
type Service struct {
	db  *storage.DB
	kms *security.KMS
}

// New creates a new webhooks Service.
func New(db *storage.DB, kms *security.KMS) *Service {
	return &Service{db: db, kms: kms}
}

// Get retrieves a webhook config for a project and provider.
func (s *Service) Get(ctx context.Context, projectID int64, provider string) (*storage.ProjectWebhook, error) {
	webhook, err := s.db.GetProjectWebhook(ctx, projectID, provider)
	if err != nil {
		return nil, err // Returns ErrNotFound if not found
	}
	return webhook, nil
}

// Set creates or updates a webhook config.
// The secret is encrypted before storage.
func (s *Service) Set(ctx context.Context, projectID int64, provider string, secret []byte, enabled, requireSecret bool) error {
	var encryptedSecret []byte
	var err error

	if len(secret) > 0 && s.kms != nil {
		encrypted, err := s.kms.EncryptString(ctx, string(secret))
		if err != nil {
			return fmt.Errorf("encrypting webhook secret: %w", err)
		}
		encryptedSecret = []byte(encrypted)
	} else {
		encryptedSecret = secret
	}

	if err = s.db.SetProjectWebhook(ctx, projectID, provider, encryptedSecret, enabled, requireSecret); err != nil {
		return fmt.Errorf("setting webhook config: %w", err)
	}

	return nil
}

// List returns all webhooks for a project.
func (s *Service) List(ctx context.Context, projectID int64) ([]*storage.ProjectWebhook, error) {
	webhooks, err := s.db.ListProjectWebhooks(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("listing webhooks: %w", err)
	}
	return webhooks, nil
}

// Delete removes a webhook config.
func (s *Service) Delete(ctx context.Context, projectID int64, provider string) error {
	if err := s.db.DeleteProjectWebhook(ctx, projectID, provider); err != nil {
		return fmt.Errorf("deleting webhook: %w", err)
	}
	return nil
}

// GetDecryptedSecret retrieves and decrypts a webhook secret.
func (s *Service) GetDecryptedSecret(ctx context.Context, projectID int64, provider string) ([]byte, error) {
	webhook, err := s.db.GetProjectWebhook(ctx, projectID, provider)
	if err != nil {
		return nil, err
	}

	if len(webhook.SecretEncrypted) == 0 {
		return nil, nil
	}

	if s.kms == nil {
		return webhook.SecretEncrypted, nil
	}

	decrypted, err := s.kms.DecryptString(ctx, string(webhook.SecretEncrypted))
	if err != nil {
		return nil, fmt.Errorf("decrypting webhook secret: %w", err)
	}

	return []byte(decrypted), nil
}
