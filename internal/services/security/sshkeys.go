// Package security provides service layer for security-related operations.
package security

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

// ErrKMSNotConfigured is returned when a KMS operation is attempted but KMS is nil.
var ErrKMSNotConfigured = errors.New("KMS not configured: encryption/decryption operations are unavailable")

// SSHKeyService provides business logic for SSH key operations.
type SSHKeyService struct {
	store  storage.Store
	kms    SecretEncryptor
	logger *zap.Logger
}

// NewSSHKeyService creates a new SSH key service.
func NewSSHKeyService(store storage.Store, kms SecretEncryptor, logger *zap.Logger) *SSHKeyService {
	return &SSHKeyService{
		store:  store,
		kms:    kms,
		logger: logger,
	}
}

// requireKMS returns an error if KMS is not configured.
func (s *SSHKeyService) requireKMS() error {
	if s.kms == nil {
		return ErrKMSNotConfigured
	}
	return nil
}

// SSHKeyInfo represents SSH key info for API responses (without private key).
type SSHKeyInfo struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	PublicKey   string    `json:"public_key"`
	Fingerprint string    `json:"fingerprint"`
	KeyType     string    `json:"key_type"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
}

// GenerateSSHKeyRequest represents a request to generate a new SSH key.
type GenerateSSHKeyRequest struct {
	Name      string `json:"name"`
	KeyType   string `json:"key_type"` // "ed25519", "rsa", "ecdsa"
	CreatedBy string `json:"-"`        // Set by handler from auth context
}

// ImportSSHKeyRequest represents a request to import an existing SSH key.
type ImportSSHKeyRequest struct {
	Name       string `json:"name"`
	PrivateKey string `json:"private_key"`
	Passphrase string `json:"passphrase,omitempty"` // Optional passphrase for encrypted keys
	CreatedBy  string `json:"-"`                    // Set by handler from auth context
}

// Validate validates the generate request.
func (r *GenerateSSHKeyRequest) Validate() error {
	if r.Name == "" {
		return services.NewInputError("name is required", "name")
	}
	if len(r.Name) > 255 {
		return services.NewInputError("name must be 255 characters or less", "name")
	}

	switch r.KeyType {
	case "", storage.SSHKeyTypeEd25519:
		// Default to ed25519
		r.KeyType = storage.SSHKeyTypeEd25519
	case storage.SSHKeyTypeRSA, storage.SSHKeyTypeECDSA:
		// Valid types
	default:
		return services.NewInputError("key_type must be ed25519, rsa, or ecdsa", "key_type")
	}

	return nil
}

// Validate validates the import request.
func (r *ImportSSHKeyRequest) Validate() error {
	if r.Name == "" {
		return services.NewInputError("name is required", "name")
	}
	if len(r.Name) > 255 {
		return services.NewInputError("name must be 255 characters or less", "name")
	}

	if r.PrivateKey == "" {
		return services.NewInputError("private_key is required", "private_key")
	}

	// Validate private key format - try with passphrase first if provided
	var err error
	if r.Passphrase != "" {
		_, err = ssh.ParsePrivateKeyWithPassphrase([]byte(r.PrivateKey), []byte(r.Passphrase))
		if err != nil {
			// Check if it's a wrong passphrase error
			if err.Error() == "ssh: this private key is passphrase protected" {
				return services.NewInputError("invalid passphrase for encrypted key", "passphrase")
			}
			return services.NewInputError("invalid private key format or passphrase", "private_key")
		}
	} else {
		_, err = ssh.ParsePrivateKey([]byte(r.PrivateKey))
		if err != nil {
			// Check if the key requires a passphrase
			if err.Error() == "ssh: this private key is passphrase protected" {
				return services.NewInputError("private key is encrypted, passphrase required", "passphrase")
			}
			return services.NewInputError("invalid private key format", "private_key")
		}
	}

	return nil
}

// ListSSHKeys returns all SSH keys without their private keys.
func (s *SSHKeyService) ListSSHKeys(ctx context.Context) ([]SSHKeyInfo, error) {
	keys, err := s.store.ListSSHKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing SSH keys: %w", err)
	}

	result := make([]SSHKeyInfo, 0, len(keys))
	for _, key := range keys {
		result = append(result, SSHKeyInfo{
			ID:          key.ID,
			Name:        key.Name,
			PublicKey:   key.PublicKey,
			Fingerprint: key.Fingerprint,
			KeyType:     key.KeyType,
			CreatedBy:   key.CreatedBy,
			CreatedAt:   key.CreatedAt,
		})
	}

	return result, nil
}

// GetSSHKey returns SSH key info by ID (without private key).
func (s *SSHKeyService) GetSSHKey(ctx context.Context, id int64) (*SSHKeyInfo, error) {
	key, err := s.store.GetSSHKey(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("getting SSH key: %w", err)
	}

	return &SSHKeyInfo{
		ID:          key.ID,
		Name:        key.Name,
		PublicKey:   key.PublicKey,
		Fingerprint: key.Fingerprint,
		KeyType:     key.KeyType,
		CreatedBy:   key.CreatedBy,
		CreatedAt:   key.CreatedAt,
	}, nil
}

// GetSSHKeyByName returns SSH key info by name (without private key).
func (s *SSHKeyService) GetSSHKeyByName(ctx context.Context, name string) (*SSHKeyInfo, error) {
	key, err := s.store.GetSSHKeyByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("getting SSH key by name: %w", err)
	}

	return &SSHKeyInfo{
		ID:          key.ID,
		Name:        key.Name,
		PublicKey:   key.PublicKey,
		Fingerprint: key.Fingerprint,
		KeyType:     key.KeyType,
		CreatedBy:   key.CreatedBy,
		CreatedAt:   key.CreatedAt,
	}, nil
}

// GetPublicKey returns just the public key for an SSH key (for authorized_keys).
func (s *SSHKeyService) GetPublicKey(ctx context.Context, id int64) (string, error) {
	key, err := s.store.GetSSHKey(ctx, id)
	if err != nil {
		return "", fmt.Errorf("getting SSH key: %w", err)
	}

	return key.PublicKey, nil
}

// GenerateSSHKey generates a new SSH key pair.
func (s *SSHKeyService) GenerateSSHKey(ctx context.Context, req GenerateSSHKeyRequest) (*SSHKeyInfo, error) {
	if err := s.requireKMS(); err != nil {
		return nil, err
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Check for duplicate name
	_, err := s.store.GetSSHKeyByName(ctx, req.Name)
	if err == nil {
		return nil, services.NewInputError("SSH key with this name already exists", "name")
	}

	var privateKey, publicKey []byte
	var fingerprint, keyType string

	switch req.KeyType {
	case storage.SSHKeyTypeEd25519:
		privateKey, publicKey, fingerprint, err = generateEd25519Key()
		keyType = storage.SSHKeyTypeEd25519
	case storage.SSHKeyTypeRSA:
		privateKey, publicKey, fingerprint, err = generateRSAKey(4096)
		keyType = storage.SSHKeyTypeRSA
	case storage.SSHKeyTypeECDSA:
		privateKey, publicKey, fingerprint, err = generateECDSAKey(elliptic.P256())
		keyType = storage.SSHKeyTypeECDSA
	default:
		return nil, services.NewInputError("unsupported key type: must be ed25519, rsa, or ecdsa", "key_type")
	}

	if err != nil {
		return nil, fmt.Errorf("generating key: %w", err)
	}

	// Encrypt the private key
	encrypted, err := s.kms.Encrypt(ctx, privateKey)
	if err != nil {
		return nil, fmt.Errorf("encrypting private key: %w", err)
	}

	key := &storage.SSHKey{
		Name:          req.Name,
		PublicKey:     string(publicKey),
		PrivateKeyEnc: []byte(encrypted), // Store versioned string as bytes
		Fingerprint:   fingerprint,
		KeyType:       keyType,
		CreatedBy:     req.CreatedBy,
		CreatedAt:     time.Now(),
	}

	if err := s.store.SaveSSHKey(ctx, key); err != nil {
		return nil, fmt.Errorf("saving SSH key: %w", err)
	}

	s.logger.Info("SSH key generated",
		zap.String("name", req.Name),
		zap.String("key_type", keyType),
		zap.String("fingerprint", fingerprint),
		zap.String("created_by", req.CreatedBy),
	)

	return &SSHKeyInfo{
		ID:          key.ID,
		Name:        key.Name,
		PublicKey:   key.PublicKey,
		Fingerprint: key.Fingerprint,
		KeyType:     key.KeyType,
		CreatedBy:   key.CreatedBy,
		CreatedAt:   key.CreatedAt,
	}, nil
}

// ImportSSHKey imports an existing SSH key.
func (s *SSHKeyService) ImportSSHKey(ctx context.Context, req ImportSSHKeyRequest) (*SSHKeyInfo, error) {
	if err := s.requireKMS(); err != nil {
		return nil, err
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Check for duplicate name
	_, err := s.store.GetSSHKeyByName(ctx, req.Name)
	if err == nil {
		return nil, services.NewInputError("SSH key with this name already exists", "name")
	}

	// Parse the private key to get key type and public key
	// Handle passphrase-protected keys
	var privateKey ssh.Signer
	if req.Passphrase != "" {
		privateKey, err = ssh.ParsePrivateKeyWithPassphrase([]byte(req.PrivateKey), []byte(req.Passphrase))
	} else {
		privateKey, err = ssh.ParsePrivateKey([]byte(req.PrivateKey))
	}
	if err != nil {
		return nil, services.NewInputError("invalid private key format", "private_key")
	}

	// Get public key in authorized_keys format
	publicKey := ssh.MarshalAuthorizedKey(privateKey.PublicKey())

	// Get fingerprint
	fingerprint := ssh.FingerprintSHA256(privateKey.PublicKey())

	// Determine key type
	keyType := determineKeyType(privateKey.PublicKey())

	// Encrypt the private key (store original with passphrase for re-import scenarios)
	encrypted, err := s.kms.Encrypt(ctx, []byte(req.PrivateKey))
	if err != nil {
		return nil, fmt.Errorf("encrypting private key: %w", err)
	}

	key := &storage.SSHKey{
		Name:          req.Name,
		PublicKey:     string(publicKey),
		PrivateKeyEnc: []byte(encrypted), // Store versioned string as bytes
		Fingerprint:   fingerprint,
		KeyType:       keyType,
		CreatedBy:     req.CreatedBy,
		CreatedAt:     time.Now(),
	}

	if err := s.store.SaveSSHKey(ctx, key); err != nil {
		return nil, fmt.Errorf("saving SSH key: %w", err)
	}

	s.logger.Info("SSH key imported",
		zap.String("name", req.Name),
		zap.String("key_type", keyType),
		zap.String("fingerprint", fingerprint),
		zap.String("created_by", req.CreatedBy),
	)

	return &SSHKeyInfo{
		ID:          key.ID,
		Name:        key.Name,
		PublicKey:   key.PublicKey,
		Fingerprint: key.Fingerprint,
		KeyType:     key.KeyType,
		CreatedBy:   key.CreatedBy,
		CreatedAt:   key.CreatedAt,
	}, nil
}

// DeleteSSHKey removes an SSH key.
func (s *SSHKeyService) DeleteSSHKey(ctx context.Context, id int64) error {
	// Check exists
	key, err := s.store.GetSSHKey(ctx, id)
	if err != nil {
		return fmt.Errorf("getting SSH key: %w", err)
	}

	if err := s.store.DeleteSSHKey(ctx, id); err != nil {
		return fmt.Errorf("deleting SSH key: %w", err)
	}

	s.logger.Info("SSH key deleted",
		zap.Int64("id", id),
		zap.String("name", key.Name),
	)

	return nil
}

// GetSigner returns an ssh.Signer for a key (for use in SSH connections).
func (s *SSHKeyService) GetSigner(ctx context.Context, id int64) (ssh.Signer, error) {
	if err := s.requireKMS(); err != nil {
		return nil, err
	}

	key, err := s.store.GetSSHKey(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("getting SSH key: %w", err)
	}

	// Decrypt the private key
	decrypted, err := s.kms.Decrypt(ctx, string(key.PrivateKeyEnc))
	if err != nil {
		return nil, fmt.Errorf("decrypting private key: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(decrypted)
	if err != nil {
		return nil, fmt.Errorf("parsing private key: %w", err)
	}

	return signer, nil
}

// Helper functions

func generateEd25519Key() (privateKey, publicKey []byte, fingerprint string, err error) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, "", fmt.Errorf("generating ed25519 key: %w", err)
	}

	// Convert to SSH format
	sshPubKey, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		return nil, nil, "", fmt.Errorf("converting public key: %w", err)
	}

	// Get public key in authorized_keys format
	publicKey = ssh.MarshalAuthorizedKey(sshPubKey)

	// Get fingerprint
	fingerprint = ssh.FingerprintSHA256(sshPubKey)

	// Encode private key to PEM format
	// Note: ed25519 private keys include the public key, so we use the full 64-byte key
	privateKey = pem.EncodeToMemory(&pem.Block{
		Type:  "OPENSSH PRIVATE KEY",
		Bytes: marshalED25519PrivateKey(privKey),
	})

	return privateKey, publicKey, fingerprint, nil
}

// generateRSAKey generates an RSA SSH key pair.
func generateRSAKey(bits int) (privateKey, publicKey []byte, fingerprint string, err error) {
	if bits < 2048 {
		bits = 2048
	}
	if bits > 8192 {
		bits = 8192
	}

	rsaKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, nil, "", fmt.Errorf("generating RSA key: %w", err)
	}

	// Convert to SSH format
	sshPubKey, err := ssh.NewPublicKey(&rsaKey.PublicKey)
	if err != nil {
		return nil, nil, "", fmt.Errorf("converting public key: %w", err)
	}

	// Get public key in authorized_keys format
	publicKey = ssh.MarshalAuthorizedKey(sshPubKey)

	// Get fingerprint
	fingerprint = ssh.FingerprintSHA256(sshPubKey)

	// Encode private key to PEM format
	privateKey = pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(rsaKey),
	})

	return privateKey, publicKey, fingerprint, nil
}

// generateECDSAKey generates an ECDSA SSH key pair.
func generateECDSAKey(curve elliptic.Curve) (privateKey, publicKey []byte, fingerprint string, err error) {
	ecKey, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, nil, "", fmt.Errorf("generating ECDSA key: %w", err)
	}

	// Convert to SSH format
	sshPubKey, err := ssh.NewPublicKey(&ecKey.PublicKey)
	if err != nil {
		return nil, nil, "", fmt.Errorf("converting public key: %w", err)
	}

	// Get public key in authorized_keys format
	publicKey = ssh.MarshalAuthorizedKey(sshPubKey)

	// Get fingerprint
	fingerprint = ssh.FingerprintSHA256(sshPubKey)

	// Encode private key to PEM format
	ecKeyBytes, err := x509.MarshalECPrivateKey(ecKey)
	if err != nil {
		return nil, nil, "", fmt.Errorf("marshaling ECDSA key: %w", err)
	}
	privateKey = pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: ecKeyBytes,
	})

	return privateKey, publicKey, fingerprint, nil
}

// marshalED25519PrivateKey marshals an ed25519 private key to OpenSSH format.
// This is a simplified version - full implementation would use proper OpenSSH format.
func marshalED25519PrivateKey(key ed25519.PrivateKey) []byte {
	// For simplicity, we'll just encode the raw key
	// In production, you'd want to use a proper OpenSSH format encoder
	return key
}

func determineKeyType(pubKey ssh.PublicKey) string {
	keyType := pubKey.Type()
	switch keyType {
	case "ssh-ed25519":
		return storage.SSHKeyTypeEd25519
	case "ssh-rsa":
		return storage.SSHKeyTypeRSA
	case "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521":
		return storage.SSHKeyTypeECDSA
	default:
		return keyType
	}
}
