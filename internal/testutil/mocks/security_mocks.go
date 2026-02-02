package testutil

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// ErrNotFound is returned when a resource is not found in mock stores.
var ErrNotFound = errors.New("not found")

// MockKMS implements a mock Key Management Service for testing.
type MockKMS struct {
	mu sync.Mutex

	// Track calls
	Calls []string

	// Mock behavior
	EncryptFunc func(ctx context.Context, plaintext []byte) (string, error)
	DecryptFunc func(ctx context.Context, ciphertext string) ([]byte, error)

	// Store for simulating encryption/decryption
	encryptedData map[string][]byte
}

// NewMockKMS creates a new mock KMS with default behavior.
// By default, it stores encrypted data in memory and returns it on decrypt.
func NewMockKMS() *MockKMS {
	m := &MockKMS{
		encryptedData: make(map[string][]byte),
	}

	// Default encrypt: store data with a fake key ID
	m.EncryptFunc = func(_ context.Context, plaintext []byte) (string, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		key := "v1:test-key:" + randomID()
		m.encryptedData[key] = append([]byte(nil), plaintext...) // copy
		return key, nil
	}

	// Default decrypt: retrieve data
	m.DecryptFunc = func(_ context.Context, ciphertext string) ([]byte, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if data, ok := m.encryptedData[ciphertext]; ok {
			return append([]byte(nil), data...), nil // copy
		}
		return nil, ErrNotFound
	}

	return m
}

// Encrypt calls the mock encrypt function.
func (m *MockKMS) Encrypt(ctx context.Context, plaintext []byte) (string, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, "Encrypt")
	m.mu.Unlock()
	return m.EncryptFunc(ctx, plaintext)
}

// Decrypt calls the mock decrypt function.
func (m *MockKMS) Decrypt(ctx context.Context, ciphertext string) ([]byte, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, "Decrypt")
	m.mu.Unlock()
	return m.DecryptFunc(ctx, ciphertext)
}

// MockCAManager implements a mock Certificate Authority Manager for testing.
type MockCAManager struct {
	mu sync.Mutex

	// Track calls
	Calls []string

	// Mock returns
	InitializeFunc              func(ctx context.Context) error
	IssueAgentCertificateFunc   func(ctx context.Context, agentID string) (*tls.Certificate, error)
	RevokeCertificateFunc       func(ctx context.Context, agentID, reason string) error
	GetTrustPoolFunc            func(ctx context.Context) (*x509.CertPool, error)
	VerifyCertificateFunc       func(cert *x509.Certificate) error
	GetOrIssueServerCertFunc    func(ctx context.Context, hosts []string) (*tls.Certificate, error)
	GetAgentCertificateInfoFunc func(ctx context.Context, agentID string) (*CertificateInfo, error)

	// State
	IssuedCerts  map[string]*tls.Certificate
	RevokedCerts map[string]bool
	TrustPool    *x509.CertPool
	ServerCert   *tls.Certificate
}

// CertificateInfo contains certificate details for testing.
type CertificateInfo struct {
	AgentID      string
	SerialNumber string
	Fingerprint  string
	IssuedAt     time.Time
	ExpiresAt    time.Time
	Revoked      bool
	RevokedAt    time.Time
	RevokeReason string
}

// NewMockCAManager creates a new mock CA manager.
func NewMockCAManager() *MockCAManager {
	m := &MockCAManager{
		IssuedCerts:  make(map[string]*tls.Certificate),
		RevokedCerts: make(map[string]bool),
		TrustPool:    x509.NewCertPool(),
	}

	// Set default implementations
	m.InitializeFunc = func(_ context.Context) error {
		return nil
	}

	m.IssueAgentCertificateFunc = func(_ context.Context, agentID string) (*tls.Certificate, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		cert := &tls.Certificate{} // Minimal cert for testing
		m.IssuedCerts[agentID] = cert
		return cert, nil
	}

	m.RevokeCertificateFunc = func(_ context.Context, agentID, _ string) error {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.RevokedCerts[agentID] = true
		return nil
	}

	m.GetTrustPoolFunc = func(_ context.Context) (*x509.CertPool, error) {
		return m.TrustPool, nil
	}

	m.VerifyCertificateFunc = func(_ *x509.Certificate) error {
		return nil
	}

	return m
}

// Initialize calls the mock initialize function.
func (m *MockCAManager) Initialize(ctx context.Context) error {
	m.mu.Lock()
	m.Calls = append(m.Calls, "Initialize")
	m.mu.Unlock()
	return m.InitializeFunc(ctx)
}

// IssueAgentCertificate calls the mock issue function.
func (m *MockCAManager) IssueAgentCertificate(ctx context.Context, agentID string) (*tls.Certificate, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, "IssueAgentCertificate:"+agentID)
	m.mu.Unlock()
	return m.IssueAgentCertificateFunc(ctx, agentID)
}

// RevokeCertificate calls the mock revoke function.
func (m *MockCAManager) RevokeCertificate(ctx context.Context, agentID, reason string) error {
	m.mu.Lock()
	m.Calls = append(m.Calls, "RevokeCertificate:"+agentID)
	m.mu.Unlock()
	return m.RevokeCertificateFunc(ctx, agentID, reason)
}

// GetTrustPool calls the mock trust pool function.
func (m *MockCAManager) GetTrustPool(ctx context.Context) (*x509.CertPool, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, "GetTrustPool")
	m.mu.Unlock()
	return m.GetTrustPoolFunc(ctx)
}

// VerifyCertificate calls the mock verify function.
func (m *MockCAManager) VerifyCertificate(cert *x509.Certificate) error {
	m.mu.Lock()
	m.Calls = append(m.Calls, "VerifyCertificate")
	m.mu.Unlock()
	return m.VerifyCertificateFunc(cert)
}

// MockSSHKeyStore implements a mock SSH key store for testing.
type MockSSHKeyStore struct {
	mu sync.Mutex

	// Track calls
	Calls []string

	// Mock data
	Keys map[int64]*SSHKeyInfo

	// Mock functions
	GetKeyFunc    func(id int64) (*SSHKeyInfo, error)
	ListKeysFunc  func() ([]*SSHKeyInfo, error)
	CreateKeyFunc func(key *SSHKeyInfo) error
	DeleteKeyFunc func(id int64) error
	GetSignerFunc func(id int64) (ssh.Signer, error)
}

// SSHKeyInfo contains SSH key details for testing.
type SSHKeyInfo struct {
	ID          int64
	Name        string
	Type        string
	Fingerprint string
	PublicKey   string
	PrivateKey  []byte // encrypted
	Comment     string
	CreatedAt   time.Time
	LastUsedAt  time.Time
}

// NewMockSSHKeyStore creates a new mock SSH key store.
func NewMockSSHKeyStore() *MockSSHKeyStore {
	m := &MockSSHKeyStore{
		Keys: make(map[int64]*SSHKeyInfo),
	}

	m.GetKeyFunc = func(id int64) (*SSHKeyInfo, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if key, ok := m.Keys[id]; ok {
			return key, nil
		}
		return nil, ErrNotFound
	}

	m.ListKeysFunc = func() ([]*SSHKeyInfo, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		result := make([]*SSHKeyInfo, 0, len(m.Keys))
		for _, key := range m.Keys {
			result = append(result, key)
		}
		return result, nil
	}

	m.CreateKeyFunc = func(key *SSHKeyInfo) error {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.Keys[key.ID] = key
		return nil
	}

	m.DeleteKeyFunc = func(id int64) error {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.Keys, id)
		return nil
	}

	return m
}

// GetKey calls the mock get function.
func (m *MockSSHKeyStore) GetKey(id int64) (*SSHKeyInfo, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, "GetKey")
	m.mu.Unlock()
	return m.GetKeyFunc(id)
}

// ListKeys calls the mock list function.
func (m *MockSSHKeyStore) ListKeys() ([]*SSHKeyInfo, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, "ListKeys")
	m.mu.Unlock()
	return m.ListKeysFunc()
}

// CreateKey calls the mock create function.
func (m *MockSSHKeyStore) CreateKey(key *SSHKeyInfo) error {
	m.mu.Lock()
	m.Calls = append(m.Calls, "CreateKey")
	m.mu.Unlock()
	return m.CreateKeyFunc(key)
}

// DeleteKey calls the mock delete function.
func (m *MockSSHKeyStore) DeleteKey(id int64) error {
	m.mu.Lock()
	m.Calls = append(m.Calls, "DeleteKey")
	m.mu.Unlock()
	return m.DeleteKeyFunc(id)
}

// MockCredentialStore implements a mock credential store for testing.
type MockCredentialStore struct {
	mu sync.Mutex

	// Track calls
	Calls []string

	// Mock data
	Credentials map[int64]*CredentialInfo

	// Mock functions
	GetCredentialFunc    func(id int64) (*CredentialInfo, error)
	ListCredentialsFunc  func() ([]*CredentialInfo, error)
	CreateCredentialFunc func(cred *CredentialInfo) error
	UpdateCredentialFunc func(cred *CredentialInfo) error
	DeleteCredentialFunc func(id int64) error
	MatchURLFunc         func(url string) (*CredentialInfo, error)
}

// CredentialInfo contains credential details for testing.
type CredentialInfo struct {
	ID            int64
	Name          string
	Type          string
	URLPattern    string
	Username      string
	CredentialEnc []byte // encrypted
	SSHKeyID      int64
	Description   string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	LastUsedAt    time.Time
	UsageCount    int
}

// NewMockCredentialStore creates a new mock credential store.
func NewMockCredentialStore() *MockCredentialStore {
	m := &MockCredentialStore{
		Credentials: make(map[int64]*CredentialInfo),
	}

	m.GetCredentialFunc = func(id int64) (*CredentialInfo, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if cred, ok := m.Credentials[id]; ok {
			return cred, nil
		}
		return nil, ErrNotFound
	}

	m.ListCredentialsFunc = func() ([]*CredentialInfo, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		result := make([]*CredentialInfo, 0, len(m.Credentials))
		for _, cred := range m.Credentials {
			result = append(result, cred)
		}
		return result, nil
	}

	m.CreateCredentialFunc = func(cred *CredentialInfo) error {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.Credentials[cred.ID] = cred
		return nil
	}

	m.UpdateCredentialFunc = func(cred *CredentialInfo) error {
		m.mu.Lock()
		defer m.mu.Unlock()
		if _, ok := m.Credentials[cred.ID]; !ok {
			return ErrNotFound
		}
		m.Credentials[cred.ID] = cred
		return nil
	}

	m.DeleteCredentialFunc = func(id int64) error {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.Credentials, id)
		return nil
	}

	return m
}

// GetCredential calls the mock get function.
func (m *MockCredentialStore) GetCredential(id int64) (*CredentialInfo, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, "GetCredential")
	m.mu.Unlock()
	return m.GetCredentialFunc(id)
}

// ListCredentials calls the mock list function.
func (m *MockCredentialStore) ListCredentials() ([]*CredentialInfo, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, "ListCredentials")
	m.mu.Unlock()
	return m.ListCredentialsFunc()
}

// CreateCredential calls the mock create function.
func (m *MockCredentialStore) CreateCredential(cred *CredentialInfo) error {
	m.mu.Lock()
	m.Calls = append(m.Calls, "CreateCredential")
	m.mu.Unlock()
	return m.CreateCredentialFunc(cred)
}

// UpdateCredential calls the mock update function.
func (m *MockCredentialStore) UpdateCredential(cred *CredentialInfo) error {
	m.mu.Lock()
	m.Calls = append(m.Calls, "UpdateCredential")
	m.mu.Unlock()
	return m.UpdateCredentialFunc(cred)
}

// DeleteCredential calls the mock delete function.
func (m *MockCredentialStore) DeleteCredential(id int64) error {
	m.mu.Lock()
	m.Calls = append(m.Calls, "DeleteCredential")
	m.mu.Unlock()
	return m.DeleteCredentialFunc(id)
}

// Helper for generating random IDs
func randomID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, 8)
	for i := range result {
		result[i] = chars[i%len(chars)]
	}
	return string(result)
}
