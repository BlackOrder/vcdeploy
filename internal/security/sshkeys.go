// Package security provides encryption and authentication utilities.
//
// # SSH Host Key Verification Modes
//
// This package supports three host key verification modes for SSH connections:
//
// ## StrictHostKey (Recommended for Production)
//
// Uses a pre-populated known_hosts database. Connections to unknown hosts are
// rejected. This is the most secure option and should be used in production
// environments where target hosts are known in advance.
//
// Usage:
//
//	callback := sshKeyMgr.StrictHostKeyCallback(ctx)
//	config := &ssh.ClientConfig{HostKeyCallback: callback}
//
// Security: Prevents MITM attacks by rejecting connections to hosts with
// unknown or changed keys.
//
// ## TrustOnFirstUse (TOFU) - Suitable for Dynamic Environments
//
// Trusts the first key presented by a host and stores it. Subsequent connections
// verify the key matches. This provides a balance between security and usability
// for environments where hosts are created dynamically (e.g., cloud provisioning).
//
// Usage:
//
//	callback := sshKeyMgr.TrustOnFirstUse(ctx)
//	config := &ssh.ClientConfig{HostKeyCallback: callback}
//
// Security: Vulnerable to MITM on first connection only. Subsequent connections
// are verified. Consider manual key verification for high-security environments.
//
// ## InsecureIgnoreHostKey (Testing Only - NEVER USE IN PRODUCTION)
//
// Accepts any host key without verification. This completely disables host key
// checking and should ONLY be used in isolated test environments with no network
// exposure.
//
// Usage:
//
//	config := &ssh.ClientConfig{HostKeyCallback: ssh.InsecureIgnoreHostKey()}
//
// Security: NO SECURITY. Vulnerable to MITM attacks. Using this in production
// could allow attackers to intercept credentials and deployment data.
//
// # Recommendations
//
//   - Production: Use StrictHostKey with pre-provisioned known_hosts
//   - Staging/Dev: Use TrustOnFirstUse with regular key audits
//   - Testing: Use TrustOnFirstUse (preferred) or InsecureIgnoreHostKey (isolated only)
package security

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"
	"golang.org/x/crypto/ssh"
)

// SSHKeyManager manages SSH keys stored in the database.
type SSHKeyManager struct {
	db    *sql.DB
	kms   *KMS
	cache map[string]*SSHKey
	mu    sync.RWMutex
}

// SSHKey represents an Ed25519 SSH key pair.
type SSHKey struct {
	ID               string
	Name             string
	PublicKey        string // OpenSSH format
	PrivateKeyEnc    []byte // KMS-encrypted PEM
	KeyType          string
	Fingerprint      string
	CreatedAt        time.Time
	LastUsedAt       *time.Time
	privateKeySigner ssh.Signer // Cached decrypted signer
}

// KnownHost represents a host key in the known_hosts table.
type KnownHost struct {
	ID             string
	Hostname       string
	Port           int
	KeyType        string
	PublicKey      string
	Fingerprint    string
	AddedAt        time.Time
	LastVerifiedAt *time.Time
}

// NewSSHKeyManager creates a new SSH key manager.
func NewSSHKeyManager(db *sql.DB, kms *KMS) *SSHKeyManager {
	return &SSHKeyManager{
		db:    db,
		kms:   kms,
		cache: make(map[string]*SSHKey),
	}
}

// requireKMS returns an error if KMS is not configured.
func (m *SSHKeyManager) requireKMS() error {
	if m.kms == nil {
		return ErrKMSNotConfigured
	}
	return nil
}

// GenerateKey creates a new Ed25519 SSH key pair and stores it in the database.
func (m *SSHKeyManager) GenerateKey(ctx context.Context, name string) (*SSHKey, error) {
	if err := m.requireKMS(); err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if name already exists
	var count int
	if err := m.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ssh_keys WHERE name = ?`, name).Scan(&count); err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, fmt.Errorf("key with name %q already exists", name)
	}

	// Generate Ed25519 key pair
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	// Convert to SSH format
	sshPubKey, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		return nil, fmt.Errorf("create SSH public key: %w", err)
	}

	// Format public key in authorized_keys format
	pubKeyStr := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPubKey)))

	// Encode private key as OpenSSH PEM using the proper ssh library function
	privKeyBlock, err := ssh.MarshalPrivateKey(privKey, "")
	if err != nil {
		return nil, fmt.Errorf("marshal private key: %w", err)
	}
	privKeyPEM := pem.EncodeToMemory(privKeyBlock)

	// Encrypt private key with KMS
	encryptedKey, err := m.kms.Encrypt(ctx, privKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("encrypt private key: %w", err)
	}

	// Calculate fingerprint
	fingerprint := ssh.FingerprintSHA256(sshPubKey)

	now := time.Now()
	key := &SSHKey{
		Name:          name,
		PublicKey:     pubKeyStr,
		PrivateKeyEnc: []byte(encryptedKey),
		KeyType:       "ed25519",
		Fingerprint:   fingerprint,
		CreatedAt:     now,
	}

	// Save to database
	key.ID = xid.New().String()
	_, err = m.db.ExecContext(ctx, `
		INSERT INTO ssh_keys (id, name, public_key, private_key_encrypted, key_type, fingerprint, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, key.ID, key.Name, key.PublicKey, key.PrivateKeyEnc, key.KeyType, key.Fingerprint, key.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("save key: %w", err)
	}

	m.cache[name] = key

	return key, nil
}

// GetKey retrieves a key by name.
func (m *SSHKeyManager) GetKey(ctx context.Context, name string) (*SSHKey, error) {
	m.mu.RLock()
	if key, ok := m.cache[name]; ok {
		m.mu.RUnlock()
		return key, nil
	}
	m.mu.RUnlock()

	row := m.db.QueryRowContext(ctx, `
		SELECT id, name, public_key, private_key_encrypted, key_type, fingerprint, created_at, last_used_at
		FROM ssh_keys
		WHERE name = ?
	`, name)

	key := &SSHKey{}
	var lastUsedAt sql.NullTime
	err := row.Scan(&key.ID, &key.Name, &key.PublicKey, &key.PrivateKeyEnc, &key.KeyType, &key.Fingerprint, &key.CreatedAt, &lastUsedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan key: %w", err)
	}

	if lastUsedAt.Valid {
		key.LastUsedAt = &lastUsedAt.Time
	}

	// Cache the found key
	m.mu.Lock()
	m.cache[name] = key
	m.mu.Unlock()

	return key, nil
}

// GetKeyByID retrieves a key by ID.
func (m *SSHKeyManager) GetKeyByID(ctx context.Context, id string) (*SSHKey, error) {
	row := m.db.QueryRowContext(ctx, `
		SELECT id, name, public_key, private_key_encrypted, key_type, fingerprint, created_at, last_used_at
		FROM ssh_keys
		WHERE id = ?
	`, id)

	key := &SSHKey{}
	var lastUsedAt sql.NullTime
	err := row.Scan(&key.ID, &key.Name, &key.PublicKey, &key.PrivateKeyEnc, &key.KeyType, &key.Fingerprint, &key.CreatedAt, &lastUsedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan key: %w", err)
	}

	if lastUsedAt.Valid {
		key.LastUsedAt = &lastUsedAt.Time
	}

	return key, nil
}

// ListKeys returns all SSH keys.
func (m *SSHKeyManager) ListKeys(ctx context.Context) ([]*SSHKey, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT id, name, public_key, key_type, fingerprint, created_at, last_used_at
		FROM ssh_keys
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("query keys: %w", err)
	}
	defer rows.Close()

	var keys []*SSHKey
	for rows.Next() {
		key := &SSHKey{}
		var lastUsedAt sql.NullTime
		err := rows.Scan(&key.ID, &key.Name, &key.PublicKey, &key.KeyType, &key.Fingerprint, &key.CreatedAt, &lastUsedAt)
		if err != nil {
			return nil, fmt.Errorf("scan key: %w", err)
		}
		if lastUsedAt.Valid {
			key.LastUsedAt = &lastUsedAt.Time
		}
		keys = append(keys, key)
	}

	return keys, rows.Err()
}

// DeleteKey deletes an SSH key by name.
func (m *SSHKeyManager) DeleteKey(ctx context.Context, name string) error {
	m.mu.Lock()
	delete(m.cache, name)
	m.mu.Unlock()

	_, err := m.db.ExecContext(ctx, `DELETE FROM ssh_keys WHERE name = ?`, name)
	return err
}

// GetSigner returns an SSH signer for the key, decrypting if necessary.
func (m *SSHKeyManager) GetSigner(ctx context.Context, name string) (ssh.Signer, error) {
	if err := m.requireKMS(); err != nil {
		return nil, err
	}

	key, err := m.GetKey(ctx, name)
	if err != nil {
		return nil, err
	}
	if key == nil {
		return nil, fmt.Errorf("key not found: %s", name)
	}

	// Check if signer is cached
	m.mu.RLock()
	if key.privateKeySigner != nil {
		m.mu.RUnlock()
		return key.privateKeySigner, nil
	}
	m.mu.RUnlock()

	// Decrypt private key
	privKeyPEM, err := m.kms.Decrypt(ctx, string(key.PrivateKeyEnc))
	if err != nil {
		return nil, fmt.Errorf("decrypt private key: %w", err)
	}

	// Parse private key
	signer, err := ssh.ParsePrivateKey(privKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	// Cache the signer
	m.mu.Lock()
	key.privateKeySigner = signer
	m.mu.Unlock()

	// Update last used
	_, _ = m.db.ExecContext(ctx, `UPDATE ssh_keys SET last_used_at = ? WHERE name = ?`, time.Now(), name)

	return signer, nil
}

// --- Known Hosts Management ---

// AddKnownHost adds or updates a known host entry.
func (m *SSHKeyManager) AddKnownHost(ctx context.Context, hostname string, port int, hostKey ssh.PublicKey) error {
	keyType := hostKey.Type()
	pubKeyStr := base64.StdEncoding.EncodeToString(hostKey.Marshal())
	fingerprint := ssh.FingerprintSHA256(hostKey)
	now := time.Now()

	_, err := m.db.ExecContext(ctx, `
		INSERT INTO known_hosts (id, hostname, port, key_type, public_key, fingerprint, added_at, last_verified_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(hostname, port, key_type) DO UPDATE SET
			public_key = excluded.public_key,
			fingerprint = excluded.fingerprint,
			last_verified_at = excluded.last_verified_at
	`, xid.New().String(), hostname, port, keyType, pubKeyStr, fingerprint, now, now)
	return err
}

// GetKnownHost retrieves a known host entry.
func (m *SSHKeyManager) GetKnownHost(ctx context.Context, hostname string, port int) ([]*KnownHost, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT id, hostname, port, key_type, public_key, fingerprint, added_at, last_verified_at
		FROM known_hosts
		WHERE hostname = ? AND port = ?
	`, hostname, port)
	if err != nil {
		return nil, fmt.Errorf("query known hosts: %w", err)
	}
	defer rows.Close()

	var hosts []*KnownHost
	for rows.Next() {
		h := &KnownHost{}
		var lastVerified sql.NullTime
		err := rows.Scan(&h.ID, &h.Hostname, &h.Port, &h.KeyType, &h.PublicKey, &h.Fingerprint, &h.AddedAt, &lastVerified)
		if err != nil {
			return nil, fmt.Errorf("scan known host: %w", err)
		}
		if lastVerified.Valid {
			h.LastVerifiedAt = &lastVerified.Time
		}
		hosts = append(hosts, h)
	}

	return hosts, rows.Err()
}

// DeleteKnownHost removes a known host entry.
func (m *SSHKeyManager) DeleteKnownHost(ctx context.Context, hostname string, port int) error {
	_, err := m.db.ExecContext(ctx, `DELETE FROM known_hosts WHERE hostname = ? AND port = ?`, hostname, port)
	return err
}

// ListKnownHosts returns all known hosts.
func (m *SSHKeyManager) ListKnownHosts(ctx context.Context) ([]*KnownHost, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT id, hostname, port, key_type, public_key, fingerprint, added_at, last_verified_at
		FROM known_hosts
		ORDER BY hostname, port
	`)
	if err != nil {
		return nil, fmt.Errorf("query known hosts: %w", err)
	}
	defer rows.Close()

	var hosts []*KnownHost
	for rows.Next() {
		h := &KnownHost{}
		var lastVerified sql.NullTime
		err := rows.Scan(&h.ID, &h.Hostname, &h.Port, &h.KeyType, &h.PublicKey, &h.Fingerprint, &h.AddedAt, &lastVerified)
		if err != nil {
			return nil, fmt.Errorf("scan known host: %w", err)
		}
		if lastVerified.Valid {
			h.LastVerifiedAt = &lastVerified.Time
		}
		hosts = append(hosts, h)
	}

	return hosts, rows.Err()
}

// VerifyHostKey verifies a host key against the known_hosts database.
// Returns nil if the key is known and matches, error otherwise.
func (m *SSHKeyManager) VerifyHostKey(ctx context.Context, hostname string, port int, key ssh.PublicKey) error {
	hosts, err := m.GetKnownHost(ctx, hostname, port)
	if err != nil {
		return err
	}

	if len(hosts) == 0 {
		return &UnknownHostError{
			Hostname:    hostname,
			Port:        port,
			Fingerprint: ssh.FingerprintSHA256(key),
		}
	}

	keyType := key.Type()
	keyData := base64.StdEncoding.EncodeToString(key.Marshal())

	for _, h := range hosts {
		if h.KeyType == keyType && h.PublicKey == keyData {
			// Update last verified
			_, _ = m.db.ExecContext(ctx, `
				UPDATE known_hosts SET last_verified_at = ? 
				WHERE hostname = ? AND port = ? AND key_type = ?
			`, time.Now(), hostname, port, keyType)
			return nil
		}
	}

	// Key type exists but doesn't match - potential MITM
	return &HostKeyMismatchError{
		Hostname:    hostname,
		Port:        port,
		ExpectedKey: hosts[0].Fingerprint,
		ReceivedKey: ssh.FingerprintSHA256(key),
	}
}

// GetHostKeyCallback returns an SSH host key callback function that verifies against the database.
func (m *SSHKeyManager) GetHostKeyCallback(ctx context.Context) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		// Parse hostname and port
		host, portStr, err := net.SplitHostPort(hostname)
		if err != nil {
			host = hostname
			portStr = "22"
		}

		port := 22
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 && p <= 65535 {
			port = p
		}

		return m.VerifyHostKey(ctx, host, port, key)
	}
}

// TrustOnFirstUse returns a callback that adds unknown hosts to the database.
func (m *SSHKeyManager) TrustOnFirstUse(ctx context.Context) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		host, portStr, err := net.SplitHostPort(hostname)
		if err != nil {
			host = hostname
			portStr = "22"
		}

		port := 22
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 && p <= 65535 {
			port = p
		}

		err = m.VerifyHostKey(ctx, host, port, key)
		if err == nil {
			return nil
		}

		// If unknown, add it
		var unknownErr *UnknownHostError
		if isUnknownHostError(err, &unknownErr) {
			return m.AddKnownHost(ctx, host, port, key)
		}

		// Mismatch errors should not be auto-trusted
		return err
	}
}

// --- Error Types ---

// UnknownHostError indicates the host is not in known_hosts.
type UnknownHostError struct {
	Hostname    string
	Port        int
	Fingerprint string
}

func (e *UnknownHostError) Error() string {
	return fmt.Sprintf("host %s:%d is not known (fingerprint: %s)", e.Hostname, e.Port, e.Fingerprint)
}

// HostKeyMismatchError indicates the host key doesn't match known_hosts.
type HostKeyMismatchError struct {
	Hostname    string
	Port        int
	ExpectedKey string
	ReceivedKey string
}

func (e *HostKeyMismatchError) Error() string {
	return fmt.Sprintf("host key mismatch for %s:%d: expected %s, got %s (possible MITM attack)",
		e.Hostname, e.Port, e.ExpectedKey, e.ReceivedKey)
}

// --- Helper Functions ---

// isUnknownHostError checks if an error is an UnknownHostError.
func isUnknownHostError(err error, target **UnknownHostError) bool {
	var e *UnknownHostError
	if errors.As(err, &e) {
		*target = e
		return true
	}
	return false
}

// GetSSHPublicKeyFingerprint returns the SHA256 fingerprint of an SSH public key.
func GetSSHPublicKeyFingerprint(pubKeyStr string) (string, error) {
	// Parse the public key
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pubKeyStr))
	if err != nil {
		return "", fmt.Errorf("parse public key: %w", err)
	}
	return ssh.FingerprintSHA256(pubKey), nil
}

// HashPublicKey returns a short hash of a public key for identification.
func HashPublicKey(pubKey ssh.PublicKey) string {
	hash := sha256.Sum256(pubKey.Marshal())
	return base64.StdEncoding.EncodeToString(hash[:8])
}
