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
	"database/sql"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"sync"
	"time"

	"go.uber.org/zap"
)

// CAManager manages certificate authorities with multi-CA trust support.
// Old CAs are NEVER deleted, retained forever for backward compatibility.
type CAManager struct {
	db        *sql.DB
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
func NewCAManager(db *sql.DB, kms *KMS, logger *zap.Logger) (*CAManager, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	mgr := &CAManager{
		db:        db,
		kms:       kms,
		logger:    logger,
		trustPool: x509.NewCertPool(),
	}

	// Load all CAs into trust pool
	if err := mgr.loadTrustPool(); err != nil {
		return nil, fmt.Errorf("load trust pool: %w", err)
	}

	// Load current CA
	if err := mgr.loadCurrentCA(); err != nil {
		return nil, fmt.Errorf("load current CA: %w", err)
	}

	return mgr, nil
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

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now()

	// Deactivate current CA
	if m.currentCA != nil {
		_, err = tx.ExecContext(ctx, `
			UPDATE certificate_authorities 
			SET status = ?, is_current = 0, rotated_at = ?
			WHERE id = ?
		`, CAStatusInactive, now, m.currentCA.ID)
		if err != nil {
			return nil, fmt.Errorf("deactivate current CA: %w", err)
		}
		m.currentCA.Status = CAStatusInactive
		m.currentCA.IsCurrent = false
		m.currentCA.RotatedAt = &now
	}

	// Insert new CA
	_, err = tx.ExecContext(ctx, `
		INSERT INTO certificate_authorities 
		(id, version, common_name, certificate_pem, private_key_encrypted, not_before, not_after, status, is_current, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, newCA.ID, newCA.Version, newCA.CommonName, newCA.CertificatePEM, newCA.PrivateKeyEnc,
		newCA.NotBefore, newCA.NotAfter, newCA.Status, 1, newCA.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert new CA: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	m.currentCA = newCA
	m.trustPool.AddCert(newCA.Certificate)

	return newCA, nil
}

// ListCAs returns all certificate authorities.
func (m *CAManager) ListCAs(ctx context.Context) ([]*CertificateAuthority, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT id, version, common_name, certificate_pem, not_before, not_after, status, is_current, created_at, rotated_at
		FROM certificate_authorities
		ORDER BY version DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query CAs: %w", err)
	}
	defer rows.Close()

	var cas []*CertificateAuthority
	for rows.Next() {
		ca := &CertificateAuthority{}
		var rotatedAt sql.NullTime
		err := rows.Scan(
			&ca.ID, &ca.Version, &ca.CommonName, &ca.CertificatePEM,
			&ca.NotBefore, &ca.NotAfter, &ca.Status, &ca.IsCurrent, &ca.CreatedAt, &rotatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan CA: %w", err)
		}
		if rotatedAt.Valid {
			ca.RotatedAt = &rotatedAt.Time
		}

		// Parse certificate
		block, _ := pem.Decode([]byte(ca.CertificatePEM))
		if block != nil {
			parsedCert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				// Use global logger since this is in ListCAs before we have a manager
				zap.L().Warn("failed to parse CA certificate", zap.String("caID", ca.ID), zap.Error(err))
				ca.Certificate = nil
			} else {
				ca.Certificate = parsedCert
			}
		}

		cas = append(cas, ca)
	}

	return cas, rows.Err()
}

// GetAgentCertificate returns an agent's current certificate.
func (m *CAManager) GetAgentCertificate(ctx context.Context, agentID string) (*AgentCertificate, error) {
	row := m.db.QueryRowContext(ctx, `
		SELECT id, agent_id, ca_id, serial_number, certificate_pem, not_before, not_after, status, issued_at, renewed_at, revoked_at, revocation_reason
		FROM agent_certificates
		WHERE agent_id = ? AND status = ?
		ORDER BY issued_at DESC
		LIMIT 1
	`, agentID, CertStatusActive)

	cert := &AgentCertificate{}
	var renewedAt, revokedAt sql.NullTime
	var revocationReason sql.NullString
	err := row.Scan(
		&cert.ID, &cert.AgentID, &cert.CAID, &cert.SerialNumber, &cert.CertificatePEM,
		&cert.NotBefore, &cert.NotAfter, &cert.Status, &cert.IssuedAt,
		&renewedAt, &revokedAt, &revocationReason,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan certificate: %w", err)
	}

	if renewedAt.Valid {
		cert.RenewedAt = &renewedAt.Time
	}
	if revokedAt.Valid {
		cert.RevokedAt = &revokedAt.Time
	}
	if revocationReason.Valid {
		cert.RevocationReason = revocationReason.String
	}

	// Parse certificate
	block, _ := pem.Decode([]byte(cert.CertificatePEM))
	if block != nil {
		parsedCert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			// Use global logger since GetAgentCertificate is called before we have context
			zap.L().Warn("failed to parse agent certificate",
				zap.String("agentID", cert.AgentID),
				zap.String("serial", cert.SerialNumber),
				zap.Error(err))
			cert.Certificate = nil
		} else {
			cert.Certificate = parsedCert
		}
	}

	return cert, nil
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
	now := time.Now()
	_, err := m.db.ExecContext(ctx, `
		UPDATE agent_certificates 
		SET status = ?, revoked_at = ?, revocation_reason = ?
		WHERE serial_number = ?
	`, CertStatusRevoked, now, reason, serialNumber)
	return err
}

// ProcessExpiredCertificates marks expired certificates.
func (m *CAManager) ProcessExpiredCertificates(ctx context.Context) (int, error) {
	now := time.Now()
	result, err := m.db.ExecContext(ctx, `
		UPDATE agent_certificates 
		SET status = ?
		WHERE status = ? AND not_after < ?
	`, CertStatusExpired, CertStatusActive, now)
	if err != nil {
		return 0, err
	}
	affected, _ := result.RowsAffected()
	return int(affected), nil
}

// --- Internal methods ---

func (m *CAManager) generateCA(ctx context.Context, config CAConfig) (*CertificateAuthority, error) {
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

	// Get next version
	var maxVersion sql.NullInt64
	m.db.QueryRow(`SELECT MAX(version) FROM certificate_authorities`).Scan(&maxVersion)
	version := 1
	if maxVersion.Valid {
		version = int(maxVersion.Int64) + 1
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

func (m *CAManager) loadCurrentCA() error {
	row := m.db.QueryRow(`
		SELECT id, version, common_name, certificate_pem, private_key_encrypted, not_before, not_after, status, is_current, created_at, rotated_at
		FROM certificate_authorities
		WHERE is_current = 1
		LIMIT 1
	`)

	ca := &CertificateAuthority{}
	var rotatedAt sql.NullTime
	err := row.Scan(
		&ca.ID, &ca.Version, &ca.CommonName, &ca.CertificatePEM, &ca.PrivateKeyEnc,
		&ca.NotBefore, &ca.NotAfter, &ca.Status, &ca.IsCurrent, &ca.CreatedAt, &rotatedAt,
	)
	if err == sql.ErrNoRows {
		return nil // No CA yet
	}
	if err != nil {
		return fmt.Errorf("scan CA: %w", err)
	}

	if rotatedAt.Valid {
		ca.RotatedAt = &rotatedAt.Time
	}

	// Parse certificate
	block, _ := pem.Decode([]byte(ca.CertificatePEM))
	if block != nil {
		parsedCert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			m.logger.Warn("failed to parse current CA certificate", zap.String("caID", ca.ID), zap.Error(err))
			ca.Certificate = nil
		} else {
			ca.Certificate = parsedCert
		}
	}

	m.currentCA = ca
	return nil
}

func (m *CAManager) loadTrustPool() error {
	rows, err := m.db.Query(`
		SELECT certificate_pem FROM certificate_authorities
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var certPEM string
		if err := rows.Scan(&certPEM); err != nil {
			return err
		}

		block, _ := pem.Decode([]byte(certPEM))
		if block != nil {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err == nil {
				m.trustPool.AddCert(cert)
			}
		}
	}

	return rows.Err()
}

func (m *CAManager) saveCA(ctx context.Context, ca *CertificateAuthority) error {
	_, err := m.db.ExecContext(ctx, `
		INSERT INTO certificate_authorities 
		(id, version, common_name, certificate_pem, private_key_encrypted, not_before, not_after, status, is_current, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, ca.ID, ca.Version, ca.CommonName, ca.CertificatePEM, ca.PrivateKeyEnc,
		ca.NotBefore, ca.NotAfter, ca.Status, ca.IsCurrent, ca.CreatedAt)
	return err
}

func (m *CAManager) decryptCAKey(ctx context.Context, ca *CertificateAuthority) (*ecdsa.PrivateKey, error) {
	// If key is already loaded (from generation), use it
	if ca.PrivateKey != nil {
		return ca.PrivateKey, nil
	}

	// Decrypt from KMS
	keyDER, err := m.kms.Decrypt(ctx, string(ca.PrivateKeyEnc))
	if err != nil {
		return nil, err
	}

	return x509.ParseECPrivateKey(keyDER)
}

func (m *CAManager) saveAgentCert(ctx context.Context, cert *AgentCertificate) error {
	result, err := m.db.ExecContext(ctx, `
		INSERT INTO agent_certificates 
		(agent_id, ca_id, serial_number, certificate_pem, not_before, not_after, status, issued_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, cert.AgentID, cert.CAID, cert.SerialNumber, cert.CertificatePEM,
		cert.NotBefore, cert.NotAfter, cert.Status, cert.IssuedAt)
	if err != nil {
		return err
	}
	id, _ := result.LastInsertId()
	cert.ID = id
	return nil
}

func (m *CAManager) revokeAgentCertificates(ctx context.Context, agentID, reason string) error {
	now := time.Now()
	_, err := m.db.ExecContext(ctx, `
		UPDATE agent_certificates 
		SET status = ?, revoked_at = ?, revocation_reason = ?
		WHERE agent_id = ? AND status = ?
	`, CertStatusRevoked, now, reason, agentID, CertStatusActive)
	return err
}

func generateSerialNumber() (*big.Int, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, serialNumberLimit)
}

func generateCAID(cert *x509.Certificate) string {
	hash := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(hash[:8])
}
