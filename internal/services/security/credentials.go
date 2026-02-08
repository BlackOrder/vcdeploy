// Package security provides service layer for security-related operations.
package security

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// CredentialService provides business logic for source credential operations.
type CredentialService struct {
	store  storage.Store
	kms    SecretEncryptor
	logger *zap.Logger
}

// SecretEncryptor provides encryption/decryption for secrets.
type SecretEncryptor interface {
	Encrypt(ctx context.Context, data []byte) (string, error)
	Decrypt(ctx context.Context, versioned string) ([]byte, error)
}

// NewCredentialService creates a new credential service.
func NewCredentialService(store storage.Store, kms SecretEncryptor, logger *zap.Logger) *CredentialService {
	return &CredentialService{
		store:  store,
		kms:    kms,
		logger: logger,
	}
}

// requireKMS returns an error if KMS is not configured.
func (s *CredentialService) requireKMS() error {
	if s.kms == nil {
		return ErrKMSNotConfigured
	}
	return nil
}

// CredentialInfo represents credential info for API responses (without secrets).
type CredentialInfo struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	URLPattern string    `json:"url_pattern"`
	CreatedBy  string    `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CreateCredentialRequest represents a request to create a credential.
type CreateCredentialRequest struct {
	Name       string `json:"name"`
	Type       string `json:"type"`        // "ssh_key", "https_token", "https_basic"
	URLPattern string `json:"url_pattern"` // Regex to match repo URLs
	Credential string `json:"credential"`  // The actual credential value
	CreatedBy  string `json:"-"`           // Set by handler from auth context
}

// UpdateCredentialRequest represents a request to update a credential.
type UpdateCredentialRequest struct {
	Name       *string `json:"name,omitempty"`
	Type       *string `json:"type,omitempty"`
	URLPattern *string `json:"url_pattern,omitempty"`
	Credential *string `json:"credential,omitempty"`
}

// TestCredentialRequest represents a request to test a credential.
type TestCredentialRequest struct {
	RepoURL string `json:"repo_url"`
}

// TestCredentialResult represents the result of credential testing.
type TestCredentialResult struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	URLMatches bool   `json:"url_matches"`
}

// Validate validates the create request.
func (r *CreateCredentialRequest) Validate() error {
	if r.Name == "" {
		return services.NewInputError("name is required", "name")
	}
	if len(r.Name) > 255 {
		return services.NewInputError("name must be 255 characters or less", "name")
	}

	switch r.Type {
	case storage.SourceCredentialTypeSSHKey,
		storage.SourceCredentialTypeHTTPSToken,
		storage.SourceCredentialTypeHTTPSBasic:
		// Valid types
	default:
		return services.NewInputError("type must be ssh_key, https_token, or https_basic", "type")
	}

	if r.URLPattern == "" {
		return services.NewInputError("url_pattern is required", "url_pattern")
	}

	// Validate URL pattern is valid regex
	if _, err := regexp.Compile(r.URLPattern); err != nil {
		return services.NewInputError("url_pattern must be a valid regular expression", "url_pattern")
	}

	if r.Credential == "" {
		return services.NewInputError("credential is required", "credential")
	}

	// Validate credential format based on type
	switch r.Type {
	case storage.SourceCredentialTypeSSHKey:
		// SSH key should start with -----BEGIN
		if len(r.Credential) < 30 {
			return services.NewInputError("ssh_key credential appears invalid", "credential")
		}
	case storage.SourceCredentialTypeHTTPSBasic:
		// Should be in format username:password
		if len(r.Credential) < 3 {
			return services.NewInputError("https_basic credential must be in username:password format", "credential")
		}
	}

	return nil
}

// ListCredentials returns all credentials without their secret values.
func (s *CredentialService) ListCredentials(ctx context.Context) ([]CredentialInfo, error) {
	creds, err := s.store.ListSourceCredentials(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing credentials: %w", err)
	}

	result := make([]CredentialInfo, 0, len(creds))
	for _, cred := range creds {
		result = append(result, CredentialInfo{
			ID:         cred.ID,
			Name:       cred.Name,
			Type:       cred.Type,
			URLPattern: cred.URLPattern,
			CreatedBy:  cred.CreatedBy,
			CreatedAt:  cred.CreatedAt,
			UpdatedAt:  cred.UpdatedAt,
		})
	}

	return result, nil
}

// GetCredential returns credential info by ID (without secret).
func (s *CredentialService) GetCredential(ctx context.Context, id string) (*CredentialInfo, error) {
	cred, err := s.store.GetSourceCredential(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("getting credential: %w", err)
	}

	return &CredentialInfo{
		ID:         cred.ID,
		Name:       cred.Name,
		Type:       cred.Type,
		URLPattern: cred.URLPattern,
		CreatedBy:  cred.CreatedBy,
		CreatedAt:  cred.CreatedAt,
		UpdatedAt:  cred.UpdatedAt,
	}, nil
}

// GetCredentialByName returns credential info by name (without secret).
func (s *CredentialService) GetCredentialByName(ctx context.Context, name string) (*CredentialInfo, error) {
	cred, err := s.store.GetSourceCredentialByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("getting credential by name: %w", err)
	}

	return &CredentialInfo{
		ID:         cred.ID,
		Name:       cred.Name,
		Type:       cred.Type,
		URLPattern: cred.URLPattern,
		CreatedBy:  cred.CreatedBy,
		CreatedAt:  cred.CreatedAt,
		UpdatedAt:  cred.UpdatedAt,
	}, nil
}

// CreateCredential creates a new source credential.
func (s *CredentialService) CreateCredential(ctx context.Context, req CreateCredentialRequest) (*CredentialInfo, error) {
	if err := s.requireKMS(); err != nil {
		return nil, err
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Check for duplicate name
	_, err := s.store.GetSourceCredentialByName(ctx, req.Name)
	if err == nil {
		return nil, services.NewInputError("credential with this name already exists", "name")
	}

	// Encrypt the credential
	credData, err := json.Marshal(map[string]string{"value": req.Credential})
	if err != nil {
		return nil, fmt.Errorf("marshaling credential: %w", err)
	}

	encrypted, err := s.kms.Encrypt(ctx, credData)
	if err != nil {
		return nil, fmt.Errorf("encrypting credential: %w", err)
	}

	now := time.Now()
	cred := &storage.SourceCredential{
		Name:          req.Name,
		Type:          req.Type,
		URLPattern:    req.URLPattern,
		CredentialEnc: []byte(encrypted), // Store versioned string as bytes
		CreatedBy:     req.CreatedBy,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.store.SaveSourceCredential(ctx, cred); err != nil {
		return nil, fmt.Errorf("saving credential: %w", err)
	}

	s.logger.Info("Credential created",
		zap.String("name", req.Name),
		zap.String("type", req.Type),
		zap.String("created_by", req.CreatedBy),
	)

	return &CredentialInfo{
		ID:         cred.ID,
		Name:       cred.Name,
		Type:       cred.Type,
		URLPattern: cred.URLPattern,
		CreatedBy:  cred.CreatedBy,
		CreatedAt:  cred.CreatedAt,
		UpdatedAt:  cred.UpdatedAt,
	}, nil
}

// UpdateCredential updates an existing credential.
func (s *CredentialService) UpdateCredential(ctx context.Context, id string, req UpdateCredentialRequest) (*CredentialInfo, error) {
	// Only require KMS if credential value is being updated
	if req.Credential != nil {
		if err := s.requireKMS(); err != nil {
			return nil, err
		}
	}

	cred, err := s.store.GetSourceCredential(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("getting credential: %w", err)
	}

	// Apply updates
	if req.Name != nil {
		if *req.Name == "" {
			return nil, services.NewInputError("name cannot be empty", "name")
		}
		if len(*req.Name) > 255 {
			return nil, services.NewInputError("name must be 255 characters or less", "name")
		}
		// Check for duplicate name if changing
		if *req.Name != cred.Name {
			if _, err := s.store.GetSourceCredentialByName(ctx, *req.Name); err == nil {
				return nil, services.NewInputError("credential with this name already exists", "name")
			}
		}
		cred.Name = *req.Name
	}

	if req.Type != nil {
		switch *req.Type {
		case storage.SourceCredentialTypeSSHKey,
			storage.SourceCredentialTypeHTTPSToken,
			storage.SourceCredentialTypeHTTPSBasic:
			cred.Type = *req.Type
		default:
			return nil, services.NewInputError("type must be ssh_key, https_token, or https_basic", "type")
		}
	}

	if req.URLPattern != nil {
		if *req.URLPattern == "" {
			return nil, services.NewInputError("url_pattern cannot be empty", "url_pattern")
		}
		if _, err := regexp.Compile(*req.URLPattern); err != nil {
			return nil, services.NewInputError("url_pattern must be a valid regular expression", "url_pattern")
		}
		cred.URLPattern = *req.URLPattern
	}

	if req.Credential != nil {
		if *req.Credential == "" {
			return nil, services.NewInputError("credential cannot be empty", "credential")
		}

		// Encrypt new credential
		credData, err := json.Marshal(map[string]string{"value": *req.Credential})
		if err != nil {
			return nil, fmt.Errorf("marshaling credential: %w", err)
		}

		encrypted, err := s.kms.Encrypt(ctx, credData)
		if err != nil {
			return nil, fmt.Errorf("encrypting credential: %w", err)
		}
		cred.CredentialEnc = []byte(encrypted) // Store versioned string as bytes
	}

	cred.UpdatedAt = time.Now()

	if err := s.store.SaveSourceCredential(ctx, cred); err != nil {
		return nil, fmt.Errorf("saving credential: %w", err)
	}

	s.logger.Info("Credential updated",
		zap.String("id", id),
		zap.String("name", cred.Name),
	)

	return &CredentialInfo{
		ID:         cred.ID,
		Name:       cred.Name,
		Type:       cred.Type,
		URLPattern: cred.URLPattern,
		CreatedBy:  cred.CreatedBy,
		CreatedAt:  cred.CreatedAt,
		UpdatedAt:  cred.UpdatedAt,
	}, nil
}

// DeleteCredential removes a credential.
func (s *CredentialService) DeleteCredential(ctx context.Context, id string) error {
	// Check exists
	cred, err := s.store.GetSourceCredential(ctx, id)
	if err != nil {
		return fmt.Errorf("getting credential: %w", err)
	}

	if err := s.store.DeleteSourceCredential(ctx, id); err != nil {
		return fmt.Errorf("deleting credential: %w", err)
	}

	s.logger.Info("Credential deleted",
		zap.String("id", id),
		zap.String("name", cred.Name),
	)

	return nil
}

// TestCredential tests if a credential matches and works with a repo URL.
func (s *CredentialService) TestCredential(ctx context.Context, id string, repoURL string) (*TestCredentialResult, error) {
	cred, err := s.store.GetSourceCredential(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("getting credential: %w", err)
	}

	result := &TestCredentialResult{}

	// Test URL pattern match
	re, err := regexp.Compile(cred.URLPattern)
	if err != nil {
		result.Message = "Invalid URL pattern in credential"
		return result, nil //nolint:nilerr // Intentional: return result with failure message, not error
	}

	result.URLMatches = re.MatchString(repoURL)
	if !result.URLMatches {
		result.Message = "URL does not match credential's URL pattern"
		return result, nil
	}

	// For now, we just verify the URL pattern matches
	// Full testing would require attempting to connect to the repo
	// which is beyond the scope of this service
	result.Success = true
	result.Message = "URL matches credential pattern"

	return result, nil
}

// MatchCredentialForURL finds a credential that matches the given repo URL.
func (s *CredentialService) MatchCredentialForURL(ctx context.Context, repoURL string) (*storage.SourceCredential, error) {
	creds, err := s.store.ListSourceCredentials(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing credentials: %w", err)
	}

	for _, cred := range creds {
		re, err := regexp.Compile(cred.URLPattern)
		if err != nil {
			continue
		}
		if re.MatchString(repoURL) {
			return cred, nil
		}
	}

	return nil, services.ErrNotFound
}

// GetDecryptedCredential returns the decrypted credential value.
// This should only be used internally when actually needing the credential.
func (s *CredentialService) GetDecryptedCredential(ctx context.Context, id string) (string, error) {
	if err := s.requireKMS(); err != nil {
		return "", err
	}

	cred, err := s.store.GetSourceCredential(ctx, id)
	if err != nil {
		return "", fmt.Errorf("getting credential: %w", err)
	}

	decrypted, err := s.kms.Decrypt(ctx, string(cred.CredentialEnc))
	if err != nil {
		return "", fmt.Errorf("decrypting credential: %w", err)
	}

	var data map[string]string
	if err := json.Unmarshal(decrypted, &data); err != nil {
		return "", fmt.Errorf("unmarshaling credential: %w", err)
	}

	return data["value"], nil
}
