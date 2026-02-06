// Package security provides encryption and authentication utilities.
package security

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// CAManager manages certificate authorities with multi-CA trust support.
// Old CAs are NEVER deleted, retained forever for backward compatibility.
type CAManager struct {
	store     storage.Store
	kms       *KMS
	logger    *zap.Logger
	currentCA *CertificateAuthority
	trustPool *x509.CertPool
	mu        sync.RWMutex
}

// CertificateAuthority represents a CA in the database.
type CertificateAuthority struct {
	ID             string
	Version        int
	CommonName     string
	Certificate    *x509.Certificate
	CertificatePEM string
	PrivateKey     *ecdsa.PrivateKey
	PrivateKeyEnc  []byte // KMS-encrypted
	NotBefore      time.Time
	NotAfter       time.Time
	Status         CAStatus
	IsCurrent      bool
	CreatedAt      time.Time
	RotatedAt      *time.Time
}

// AgentCertificate represents a certificate issued to an agent.
type AgentCertificate struct {
	ID               int64
	AgentID          string
	CAID             string
	SerialNumber     string
	Certificate      *x509.Certificate
	CertificatePEM   string
	PrivateKeyPEM    string // Only set during issuance, not stored in DB
	NotBefore        time.Time
	NotAfter         time.Time
	Status           CertStatus
	IssuedAt         time.Time
	RenewedAt        *time.Time
	RevokedAt        *time.Time
	RevocationReason string
}

// CAStatus represents the lifecycle status of a CA.
type CAStatus string

const (
	CAStatusActive   CAStatus = "active"   // Current CA for signing
	CAStatusInactive CAStatus = "inactive" // Can verify but not sign
)

// CertStatus represents the lifecycle status of a certificate.
type CertStatus string

const (
	CertStatusActive  CertStatus = "active"
	CertStatusExpired CertStatus = "expired"
	CertStatusRevoked CertStatus = "revoked"
)

// CAConfig holds configuration for CA operations.
type CAConfig struct {
	// CommonName for the CA certificate
	CommonName string
	// Organization for the CA certificate
	Organization string
	// CAValidity is how long the CA certificate is valid
	CAValidity time.Duration
	// CertValidity is how long agent certificates are valid
	CertValidity time.Duration
	// RenewalThreshold is when to auto-renew (e.g., 6 months before expiry)
	RenewalThreshold time.Duration
}

// DefaultCAConfig returns default CA configuration.
func DefaultCAConfig() CAConfig {
	return CAConfig{
		CommonName:       "vcdeploy Internal CA",
		Organization:     "vcdeploy",
		CAValidity:       10 * 365 * 24 * time.Hour, // 10 years
		CertValidity:     365 * 24 * time.Hour,      // 1 year
		RenewalThreshold: 6 * 30 * 24 * time.Hour,   // 6 months
	}
}

// NewCAManager creates a new CA manager.
func NewCAManager(store storage.Store, kms *KMS, logger *zap.Logger) (*CAManager, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	mgr := &CAManager{
		store:     store,
		kms:       kms,
		logger:    logger,
		trustPool: x509.NewCertPool(),
	}

	// Load all CAs into trust pool
	if err := mgr.loadTrustPool(context.Background()); err != nil {
		return nil, fmt.Errorf("load trust pool: %w", err)
	}

	// Load current CA
	if err := mgr.loadCurrentCA(context.Background()); err != nil {
		return nil, fmt.Errorf("load current CA: %w", err)
	}

	return mgr, nil
}

// requireKMS returns an error if KMS is not configured.
func (m *CAManager) requireKMS() error {
	if m.kms == nil {
		return ErrKMSNotConfigured
	}
	return nil
}

// Initialize creates the initial CA if none exists.
func (m *CAManager) Initialize(ctx context.Context, config CAConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.currentCA != nil {
		return nil // Already have a CA
	}

	ca, err := m.generateCA(ctx, config)
	if err != nil {
		return fmt.Errorf("generate CA: %w", err)
	}

	if err := m.saveCA(ctx, ca); err != nil {
		return fmt.Errorf("save CA: %w", err)
	}

	m.currentCA = ca
	m.trustPool.AddCert(ca.Certificate)

	return nil
}

// GetTrustPool returns the certificate pool containing all trusted CAs.
func (m *CAManager) GetTrustPool() *x509.CertPool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.trustPool
}

// GetTrustPoolVersion returns a version string for the trust pool.
// The version changes when CAs are added or rotated.
func (m *CAManager) GetTrustPoolVersion(ctx context.Context) string {
	cas, err := m.store.ListCAs(ctx)
	if err != nil {
		return "unknown"
	}

	// Version is hash of all CA IDs sorted
	ids := make([]string, 0, len(cas))
	for _, ca := range cas {
		ids = append(ids, ca.ID)
	}
	sort.Strings(ids)

	h := sha256.New()
	h.Write([]byte(strings.Join(ids, ":")))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// GetAllCACertificates returns the PEM-encoded certificates for all trusted CAs.
func (m *CAManager) GetAllCACertificates(ctx context.Context) ([][]byte, error) {
	cas, err := m.store.ListCAs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list CAs: %w", err)
	}

	certs := make([][]byte, 0, len(cas))
	for _, ca := range cas {
		if ca.CertificatePEM != "" {
			certs = append(certs, []byte(ca.CertificatePEM))
		}
	}
	return certs, nil
}

// GetCurrentCA returns the current active CA.
func (m *CAManager) GetCurrentCA() *CertificateAuthority {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentCA
}

// GetTLSConfig returns a TLS config for the server with client cert verification.
func (m *CAManager) GetTLSConfig(serverCert tls.Certificate) *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    m.GetTrustPool(),
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
	}
}

// IssueAgentCertificate issues a new certificate for an agent.
func (m *CAManager) IssueAgentCertificate(ctx context.Context, agentID, hostname string) (*AgentCertificate, error) {
	m.mu.RLock()
	ca := m.currentCA
	m.mu.RUnlock()

	if ca == nil {
		return nil, fmt.Errorf("no active CA")
	}

	// Decrypt CA private key
	privateKey, err := m.decryptCAKey(ctx, ca)
	if err != nil {
		return nil, fmt.Errorf("decrypt CA key: %w", err)
	}

	// Generate agent key pair
	agentKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate agent key: %w", err)
	}

	// Generate serial number
	serialNumber, err := generateSerialNumber()
	if err != nil {
		return nil, fmt.Errorf("generate serial: %w", err)
	}

	now := time.Now()
	config := DefaultCAConfig()

	// Create certificate template
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   fmt.Sprintf("agent-%s", agentID),
			Organization: []string{config.Organization},
		},
		DNSNames:              []string{hostname, fmt.Sprintf("agent-%s", agentID)},
		NotBefore:             now,
		NotAfter:              now.Add(config.CertValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	// Sign with CA
	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.Certificate, &agentKey.PublicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	// Encode to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	keyDER, err := x509.MarshalECPrivateKey(agentKey)
	if err != nil {
		return nil, fmt.Errorf("marshal key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})

	// Save to database
	agentCert := &AgentCertificate{
		AgentID:        agentID,
		CAID:           ca.ID,
		SerialNumber:   fmt.Sprintf("%x", serialNumber),
		Certificate:    cert,
		CertificatePEM: string(certPEM),
		NotBefore:      cert.NotBefore,
		NotAfter:       cert.NotAfter,
		Status:         CertStatusActive,
		IssuedAt:       now,
	}

	if err := m.saveAgentCert(ctx, agentCert); err != nil {
		return nil, fmt.Errorf("save agent cert: %w", err)
	}

	// Attach key to return (not stored in DB - returned once to agent)
	agentCert.PrivateKeyPEM = string(keyPEM)

	return agentCert, nil
}

// RenewAgentCertificate renews an agent's certificate.
// Can be called for expired certificates.
func (m *CAManager) RenewAgentCertificate(ctx context.Context, agentID, hostname string) (*AgentCertificate, error) {
	// Revoke old certificates
	if err := m.revokeAgentCertificates(ctx, agentID, "renewed"); err != nil {
		// Log but don't fail
		m.logger.Warn("failed to revoke old certificates during renewal", zap.String("agentID", agentID), zap.Error(err))
	}

	// Issue new certificate
	return m.IssueAgentCertificate(ctx, agentID, hostname)
}

// RotateCA creates a new CA and marks the current one as inactive.
// All existing agent certificates remain valid (old CA stays in trust pool).
func (m *CAManager) RotateCA(ctx context.Context, config CAConfig) (*CertificateAuthority, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Generate new CA
	newCA, err := m.generateCA(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("generate new CA: %w", err)
	}

	// Save new CA first
	storageCA := m.toStorageCA(newCA)
	if err := m.store.SaveCA(ctx, storageCA); err != nil {
		return nil, fmt.Errorf("save new CA: %w", err)
	}

	// Set new CA as current (this deactivates the old one)
	if err := m.store.SetCurrentCA(ctx, newCA.ID); err != nil {
		return nil, fmt.Errorf("set current CA: %w", err)
	}

	// Update local state
	if m.currentCA != nil {
		m.currentCA.Status = CAStatusInactive
		m.currentCA.IsCurrent = false
		now := time.Now()
		m.currentCA.RotatedAt = &now
	}

	m.currentCA = newCA
	m.trustPool.AddCert(newCA.Certificate)

	return newCA, nil
}

// ListCAs returns all certificate authorities.
func (m *CAManager) ListCAs(ctx context.Context) ([]*CertificateAuthority, error) {
	storageCAs, err := m.store.ListCAs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list CAs: %w", err)
	}

	cas := make([]*CertificateAuthority, 0, len(storageCAs))
	for _, sca := range storageCAs {
		ca := m.fromStorageCA(sca)
		cas = append(cas, ca)
	}

	return cas, nil
}

// GetAgentCertificate returns an agent's current certificate.
func (m *CAManager) GetAgentCertificate(ctx context.Context, agentID string) (*AgentCertificate, error) {
	storageCert, err := m.store.GetAgentCert(ctx, agentID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get agent cert: %w", err)
	}

	return m.fromStorageAgentCert(storageCert), nil
}

// ShouldRenew checks if a certificate should be renewed.
func (m *CAManager) ShouldRenew(cert *AgentCertificate, threshold time.Duration) bool {
	if cert == nil || cert.Certificate == nil {
		return true
	}

	// Check if expired
	if time.Now().After(cert.NotAfter) {
		return true
	}

	// Check if within renewal threshold
	renewalTime := cert.NotAfter.Add(-threshold)
	return time.Now().After(renewalTime)
}

// RevokeCertificate revokes a specific certificate.
func (m *CAManager) RevokeCertificate(ctx context.Context, serialNumber, reason string) error {
	return m.store.RevokeAgentCertBySerial(ctx, serialNumber, reason, "system")
}

// ProcessExpiredCertificates marks expired certificates.
func (m *CAManager) ProcessExpiredCertificates(ctx context.Context) (int, error) {
	certs, err := m.store.ListAgentCerts(ctx)
	if err != nil {
		return 0, err
	}

	now := time.Now()
	count := 0

	for _, cert := range certs {
		if cert.Status == storage.CertStatusActive && cert.NotAfter.Before(now) {
			if err := m.store.UpdateAgentCertStatus(ctx, cert.SerialNumber, storage.CertStatusExpired); err != nil {
				m.logger.Warn("failed to mark cert expired", zap.String("serial", cert.SerialNumber), zap.Error(err))
				continue
			}
			count++
		}
	}

	return count, nil
}

// --- Internal methods ---

func (m *CAManager) generateCA(ctx context.Context, config CAConfig) (*CertificateAuthority, error) {
	if err := m.requireKMS(); err != nil {
		return nil, err
	}

	// Generate key pair
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	// Generate serial number
	serialNumber, err := generateSerialNumber()
	if err != nil {
		return nil, fmt.Errorf("generate serial: %w", err)
	}

	now := time.Now()

	// Create CA certificate template
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   config.CommonName,
			Organization: []string{config.Organization},
		},
		NotBefore:             now,
		NotAfter:              now.Add(config.CAValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	// Self-sign
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	// Encode to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Encrypt private key with KMS
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("marshal key: %w", err)
	}

	encryptedKey, err := m.kms.Encrypt(ctx, keyDER)
	if err != nil {
		return nil, fmt.Errorf("encrypt key: %w", err)
	}

	// Generate CA ID
	caID := generateCAID(cert)

	// Get next version by listing existing CAs
	cas, _ := m.store.ListCAs(ctx)
	version := 1
	for _, existingCA := range cas {
		if existingCA.Version >= version {
			version = existingCA.Version + 1
		}
	}

	return &CertificateAuthority{
		ID:             caID,
		Version:        version,
		CommonName:     config.CommonName,
		Certificate:    cert,
		CertificatePEM: string(certPEM),
		PrivateKey:     privateKey,
		PrivateKeyEnc:  []byte(encryptedKey),
		NotBefore:      cert.NotBefore,
		NotAfter:       cert.NotAfter,
		Status:         CAStatusActive,
		IsCurrent:      true,
		CreatedAt:      now,
	}, nil
}

func (m *CAManager) loadCurrentCA(ctx context.Context) error {
	storageCA, err := m.store.GetCurrentCA(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil // No CA yet
		}
		return fmt.Errorf("get current CA: %w", err)
	}

	ca := m.fromStorageCA(storageCA)
	m.currentCA = ca
	return nil
}

func (m *CAManager) loadTrustPool(ctx context.Context) error {
	cas, err := m.store.ListCAs(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil // No CAs yet
		}
		return err
	}

	for _, storageCA := range cas {
		block, _ := pem.Decode([]byte(storageCA.CertificatePEM))
		if block != nil {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err == nil {
				m.trustPool.AddCert(cert)
			}
		}
	}

	return nil
}

func (m *CAManager) saveCA(ctx context.Context, ca *CertificateAuthority) error {
	storageCA := m.toStorageCA(ca)
	return m.store.SaveCA(ctx, storageCA)
}

func (m *CAManager) decryptCAKey(ctx context.Context, ca *CertificateAuthority) (*ecdsa.PrivateKey, error) {
	// If key is already loaded (from generation), use it
	if ca.PrivateKey != nil {
		return ca.PrivateKey, nil
	}

	if err := m.requireKMS(); err != nil {
		return nil, err
	}

	// Decrypt from KMS
	keyDER, err := m.kms.Decrypt(ctx, string(ca.PrivateKeyEnc))
	if err != nil {
		return nil, err
	}

	return x509.ParseECPrivateKey(keyDER)
}

func (m *CAManager) saveAgentCert(ctx context.Context, cert *AgentCertificate) error {
	storageCert := m.toStorageAgentCert(cert)
	if err := m.store.SaveAgentCert(ctx, storageCert); err != nil {
		return err
	}
	cert.ID = storageCert.ID
	return nil
}

func (m *CAManager) revokeAgentCertificates(ctx context.Context, agentID, reason string) error {
	return m.store.RevokeAgentCert(ctx, agentID, reason, "system")
}

// --- Type conversion helpers ---

func (m *CAManager) toStorageCA(ca *CertificateAuthority) *storage.CertificateAuthority {
	return &storage.CertificateAuthority{
		ID:             ca.ID,
		Version:        ca.Version,
		CommonName:     ca.CommonName,
		CertificatePEM: ca.CertificatePEM,
		PrivateKeyEnc:  ca.PrivateKeyEnc,
		NotBefore:      ca.NotBefore,
		NotAfter:       ca.NotAfter,
		Status:         string(ca.Status),
		IsCurrent:      ca.IsCurrent,
		CreatedAt:      ca.CreatedAt,
		RotatedAt:      ca.RotatedAt,
	}
}

func (m *CAManager) fromStorageCA(sca *storage.CertificateAuthority) *CertificateAuthority {
	ca := &CertificateAuthority{
		ID:             sca.ID,
		Version:        sca.Version,
		CommonName:     sca.CommonName,
		CertificatePEM: sca.CertificatePEM,
		PrivateKeyEnc:  sca.PrivateKeyEnc,
		NotBefore:      sca.NotBefore,
		NotAfter:       sca.NotAfter,
		Status:         CAStatus(sca.Status),
		IsCurrent:      sca.IsCurrent,
		CreatedAt:      sca.CreatedAt,
		RotatedAt:      sca.RotatedAt,
	}

	// Parse certificate
	block, _ := pem.Decode([]byte(ca.CertificatePEM))
	if block != nil {
		parsedCert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			m.logger.Warn("failed to parse CA certificate", zap.String("caID", ca.ID), zap.Error(err))
		} else {
			ca.Certificate = parsedCert
		}
	}

	return ca
}

func (m *CAManager) toStorageAgentCert(cert *AgentCertificate) *storage.AgentCertificate {
	return &storage.AgentCertificate{
		ID:               cert.ID,
		AgentID:          cert.AgentID,
		CAID:             cert.CAID,
		SerialNumber:     cert.SerialNumber,
		CertificatePEM:   cert.CertificatePEM,
		NotBefore:        cert.NotBefore,
		NotAfter:         cert.NotAfter,
		Status:           string(cert.Status),
		IssuedAt:         cert.IssuedAt,
		RenewedAt:        cert.RenewedAt,
		RevokedAt:        cert.RevokedAt,
		RevocationReason: cert.RevocationReason,
	}
}

func (m *CAManager) fromStorageAgentCert(sc *storage.AgentCertificate) *AgentCertificate {
	cert := &AgentCertificate{
		ID:               sc.ID,
		AgentID:          sc.AgentID,
		CAID:             sc.CAID,
		SerialNumber:     sc.SerialNumber,
		CertificatePEM:   sc.CertificatePEM,
		NotBefore:        sc.NotBefore,
		NotAfter:         sc.NotAfter,
		Status:           CertStatus(sc.Status),
		IssuedAt:         sc.IssuedAt,
		RenewedAt:        sc.RenewedAt,
		RevokedAt:        sc.RevokedAt,
		RevocationReason: sc.RevocationReason,
	}

	// Parse certificate
	block, _ := pem.Decode([]byte(cert.CertificatePEM))
	if block != nil {
		parsedCert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			m.logger.Warn("failed to parse agent certificate",
				zap.String("agentID", cert.AgentID),
				zap.String("serial", cert.SerialNumber),
				zap.Error(err))
		} else {
			cert.Certificate = parsedCert
		}
	}

	return cert
}

func generateSerialNumber() (*big.Int, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, serialNumberLimit)
}

func generateCAID(cert *x509.Certificate) string {
	hash := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(hash[:8])
}
