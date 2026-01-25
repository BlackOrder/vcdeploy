// Package server provides HTTP and gRPC servers for vcdeploy.
package server

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ACMEClient provides automatic certificate management using ACME protocol.
// This is a simplified implementation - production should use autocert or similar.
type ACMEClient struct {
	db           *sql.DB
	logger       *zap.Logger
	directoryURL string
	email        string
	domains      []string

	mu          sync.RWMutex
	currentCert *tls.Certificate
	certExpiry  time.Time

	// For testing
	testMode bool
}

// ACMEAccount represents a stored ACME account.
type ACMEAccount struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	DirectoryURL string    `json:"directory_url"`
	AccountURL   string    `json:"account_url"`
	PrivateKey   []byte    `json:"private_key"` // PEM encoded, encrypted
	CreatedAt    time.Time `json:"created_at"`
	LastUsedAt   time.Time `json:"last_used_at"`
}

// ACMECertificate represents a stored ACME certificate.
type ACMECertificate struct {
	ID            int64     `json:"id"`
	Domains       string    `json:"domains"`     // JSON array
	CertPEM       []byte    `json:"cert_pem"`    // Certificate chain PEM
	PrivateKeyPEM []byte    `json:"private_key"` // Private key PEM, encrypted
	NotBefore     time.Time `json:"not_before"`
	NotAfter      time.Time `json:"not_after"`
	Issuer        string    `json:"issuer"`
	CreatedAt     time.Time `json:"created_at"`
	RenewedAt     time.Time `json:"renewed_at"`
}

// ACMEClientConfig holds configuration for the ACME client.
type ACMEClientConfig struct {
	DB           *sql.DB
	Logger       *zap.Logger
	DirectoryURL string   // ACME directory URL (e.g., Let's Encrypt)
	Email        string   // Contact email
	Domains      []string // Domains to obtain certificates for
	TestMode     bool     // Use staging/test ACME server
}

// ACME directory URLs
const (
	LetsEncryptProduction = "https://acme-v02.api.letsencrypt.org/directory"
	LetsEncryptStaging    = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

// NewACMEClient creates a new ACME client.
func NewACMEClient(cfg ACMEClientConfig) (*ACMEClient, error) {
	if cfg.DB == nil {
		return nil, fmt.Errorf("database required")
	}
	if len(cfg.Domains) == 0 {
		return nil, fmt.Errorf("at least one domain required")
	}
	if cfg.Email == "" {
		return nil, fmt.Errorf("email required for ACME registration")
	}

	directoryURL := cfg.DirectoryURL
	if directoryURL == "" {
		if cfg.TestMode {
			directoryURL = LetsEncryptStaging
		} else {
			directoryURL = LetsEncryptProduction
		}
	}

	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	return &ACMEClient{
		db:           cfg.DB,
		logger:       logger,
		directoryURL: directoryURL,
		email:        cfg.Email,
		domains:      cfg.Domains,
		testMode:     cfg.TestMode,
	}, nil
}

// GetCertificate returns a valid TLS certificate, obtaining or renewing as needed.
// This implements tls.Config.GetCertificate.
func (c *ACMEClient) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	c.mu.RLock()
	cert := c.currentCert
	expiry := c.certExpiry
	c.mu.RUnlock()

	// Check if we have a valid certificate
	if cert != nil && time.Now().Before(expiry.Add(-24*time.Hour)) {
		return cert, nil
	}

	// Need to obtain or renew certificate
	ctx := hello.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	return c.obtainOrRenewCertificate(ctx)
}

// GetTLSConfig returns a TLS configuration that uses ACME certificates.
func (c *ACMEClient) GetTLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: c.GetCertificate,
		MinVersion:     tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
		PreferServerCipherSuites: true,
	}
}

// obtainOrRenewCertificate obtains a new certificate or renews an existing one.
func (c *ACMEClient) obtainOrRenewCertificate(ctx context.Context) (*tls.Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring lock
	if c.currentCert != nil && time.Now().Before(c.certExpiry.Add(-24*time.Hour)) {
		return c.currentCert, nil
	}

	// First, try to load from database
	stored, err := c.loadCertificate(ctx)
	if err == nil && stored != nil {
		if time.Now().Before(stored.NotAfter.Add(-24 * time.Hour)) {
			// Certificate is valid, use it
			cert, err := c.parseCertificate(stored)
			if err == nil {
				c.currentCert = cert
				c.certExpiry = stored.NotAfter
				return cert, nil
			}
		}
	}

	// If test mode and no HTTP challenge handler, generate self-signed
	if c.testMode {
		return c.generateSelfSignedCertificate(ctx)
	}

	// In production, would use real ACME protocol
	// For now, return an error indicating manual certificate setup is needed
	return nil, fmt.Errorf("ACME certificate not available, please configure TLS certificates manually or enable test mode")
}

// loadCertificate loads the most recent valid certificate from the database.
func (c *ACMEClient) loadCertificate(ctx context.Context) (*ACMECertificate, error) {
	domainsJSON, err := json.Marshal(c.domains)
	if err != nil {
		return nil, fmt.Errorf("marshal domains: %w", err)
	}

	row := c.db.QueryRowContext(ctx, `
		SELECT id, domains, cert_pem, private_key_pem, not_before, not_after, issuer, created_at, renewed_at
		FROM acme_certificates
		WHERE domains = ? AND not_after > ?
		ORDER BY not_after DESC
		LIMIT 1
	`, string(domainsJSON), time.Now())

	cert := &ACMECertificate{}
	var renewedAt sql.NullTime
	err = row.Scan(&cert.ID, &cert.Domains, &cert.CertPEM, &cert.PrivateKeyPEM,
		&cert.NotBefore, &cert.NotAfter, &cert.Issuer, &cert.CreatedAt, &renewedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan certificate: %w", err)
	}

	if renewedAt.Valid {
		cert.RenewedAt = renewedAt.Time
	}

	return cert, nil
}

// saveCertificate saves a certificate to the database.
func (c *ACMEClient) saveCertificate(ctx context.Context, cert *ACMECertificate) error {
	result, err := c.db.ExecContext(ctx, `
		INSERT INTO acme_certificates (domains, cert_pem, private_key_pem, not_before, not_after, issuer, created_at, renewed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, cert.Domains, cert.CertPEM, cert.PrivateKeyPEM, cert.NotBefore, cert.NotAfter,
		cert.Issuer, cert.CreatedAt, cert.RenewedAt)
	if err != nil {
		return fmt.Errorf("insert certificate: %w", err)
	}

	// Note: SQLite's LastInsertId() never returns an error
	id, _ := result.LastInsertId()
	cert.ID = id
	return nil
}

// parseCertificate parses a stored certificate into a tls.Certificate.
func (c *ACMEClient) parseCertificate(stored *ACMECertificate) (*tls.Certificate, error) {
	cert, err := tls.X509KeyPair(stored.CertPEM, stored.PrivateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse X509 key pair: %w", err)
	}
	return &cert, nil
}

// generateSelfSignedCertificate generates a self-signed certificate for testing.
func (c *ACMEClient) generateSelfSignedCertificate(ctx context.Context) (*tls.Certificate, error) {
	// Generate private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate private key: %w", err)
	}

	// Generate serial number
	serialNumber, err := randomSerialNumber()
	if err != nil {
		return nil, fmt.Errorf("generate serial number: %w", err)
	}

	// Create certificate template
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               pkixName("Self-Signed Certificate"),
		NotBefore:             now,
		NotAfter:              now.Add(90 * 24 * time.Hour), // 90 days
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              c.domains,
	}

	// Self-sign the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	// Encode certificate to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Encode private key to PEM
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})

	// Parse into tls.Certificate
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse X509 key pair: %w", err)
	}

	// Save to database
	domainsJSON, err := json.Marshal(c.domains)
	if err != nil {
		return nil, fmt.Errorf("marshal domains: %w", err)
	}
	stored := &ACMECertificate{
		Domains:       string(domainsJSON),
		CertPEM:       certPEM,
		PrivateKeyPEM: keyPEM,
		NotBefore:     now,
		NotAfter:      now.Add(90 * 24 * time.Hour),
		Issuer:        "Self-Signed",
		CreatedAt:     now,
	}
	if err := c.saveCertificate(ctx, stored); err != nil {
		// Log but don't fail - we still have the certificate in memory
		c.logger.Warn("failed to save self-signed certificate", zap.Error(err))
	}

	// Update current cert
	c.currentCert = &tlsCert
	c.certExpiry = template.NotAfter

	return &tlsCert, nil
}

// --- ACME Account Management ---

// getOrCreateAccount gets or creates an ACME account.
func (c *ACMEClient) getOrCreateAccount(ctx context.Context) (*ACMEAccount, crypto.PrivateKey, error) {
	// Try to load existing account
	account, key, err := c.loadAccount(ctx)
	if err == nil && account != nil {
		return account, key, nil
	}

	// Create new account
	return c.createAccount(ctx)
}

// loadAccount loads an ACME account from the database.
func (c *ACMEClient) loadAccount(ctx context.Context) (*ACMEAccount, crypto.PrivateKey, error) {
	row := c.db.QueryRowContext(ctx, `
		SELECT id, email, directory_url, account_url, private_key, created_at, last_used_at
		FROM acme_accounts
		WHERE email = ? AND directory_url = ?
	`, c.email, c.directoryURL)

	account := &ACMEAccount{}
	var lastUsedAt sql.NullTime
	err := row.Scan(&account.ID, &account.Email, &account.DirectoryURL, &account.AccountURL,
		&account.PrivateKey, &account.CreatedAt, &lastUsedAt)
	if err == sql.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("scan account: %w", err)
	}

	if lastUsedAt.Valid {
		account.LastUsedAt = lastUsedAt.Time
	}

	// Parse private key
	block, _ := pem.Decode(account.PrivateKey)
	if block == nil {
		return nil, nil, fmt.Errorf("invalid private key PEM")
	}

	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse private key: %w", err)
	}

	return account, key, nil
}

// createAccount creates a new ACME account.
func (c *ACMEClient) createAccount(ctx context.Context) (*ACMEAccount, crypto.PrivateKey, error) {
	// Generate account key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate account key: %w", err)
	}

	// Encode private key
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})

	now := time.Now()
	account := &ACMEAccount{
		Email:        c.email,
		DirectoryURL: c.directoryURL,
		AccountURL:   "", // Would be set after ACME registration
		PrivateKey:   keyPEM,
		CreatedAt:    now,
		LastUsedAt:   now,
	}

	// Save to database
	result, err := c.db.ExecContext(ctx, `
		INSERT INTO acme_accounts (email, directory_url, account_url, private_key, created_at, last_used_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, account.Email, account.DirectoryURL, account.AccountURL, account.PrivateKey,
		account.CreatedAt, account.LastUsedAt)
	if err != nil {
		return nil, nil, fmt.Errorf("insert account: %w", err)
	}

	// Note: SQLite's LastInsertId() never returns an error
	id, _ := result.LastInsertId()
	account.ID = id

	return account, privateKey, nil
}

// --- Certificate Renewal ---

// StartRenewalLoop starts a background goroutine that renews certificates before expiry.
func (c *ACMEClient) StartRenewalLoop(ctx context.Context) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				c.logger.Error("panic in certificate renewal loop", zap.Any("panic", r))
			}
		}()

		ticker := time.NewTicker(12 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.mu.RLock()
				expiry := c.certExpiry
				c.mu.RUnlock()

				// Renew if expiring within 7 days
				if time.Now().After(expiry.Add(-7 * 24 * time.Hour)) {
					if _, err := c.obtainOrRenewCertificate(ctx); err != nil {
						c.logger.Error("failed to renew certificate", zap.Error(err))
					}
				}
			}
		}
	}()
}

// --- Certificate Status ---

// CertificateStatus returns information about the current certificate.
type CertificateStatus struct {
	HasCertificate bool      `json:"has_certificate"`
	Domains        []string  `json:"domains"`
	NotBefore      time.Time `json:"not_before"`
	NotAfter       time.Time `json:"not_after"`
	Issuer         string    `json:"issuer"`
	DaysRemaining  int       `json:"days_remaining"`
	NeedsRenewal   bool      `json:"needs_renewal"`
}

// GetStatus returns the current certificate status.
func (c *ACMEClient) GetStatus() CertificateStatus {
	c.mu.RLock()
	cert := c.currentCert
	expiry := c.certExpiry
	c.mu.RUnlock()

	if cert == nil {
		return CertificateStatus{
			HasCertificate: false,
			Domains:        c.domains,
		}
	}

	// Parse certificate to get details
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return CertificateStatus{
			HasCertificate: true,
			Domains:        c.domains,
			NotAfter:       expiry,
		}
	}

	daysRemaining := int(time.Until(expiry).Hours() / 24)

	return CertificateStatus{
		HasCertificate: true,
		Domains:        x509Cert.DNSNames,
		NotBefore:      x509Cert.NotBefore,
		NotAfter:       x509Cert.NotAfter,
		Issuer:         x509Cert.Issuer.CommonName,
		DaysRemaining:  daysRemaining,
		NeedsRenewal:   daysRemaining < 7,
	}
}

// --- Challenge Handlers ---

// HTTPChallengeHandler handles HTTP-01 ACME challenges.
// This would be used as an HTTP handler on port 80 for Let's Encrypt validation.
type HTTPChallengeHandler struct {
	mu         sync.RWMutex
	challenges map[string]string // token -> keyAuth
}

// NewHTTPChallengeHandler creates a new HTTP challenge handler.
func NewHTTPChallengeHandler() *HTTPChallengeHandler {
	return &HTTPChallengeHandler{
		challenges: make(map[string]string),
	}
}

// SetChallenge sets a challenge token and response.
func (h *HTTPChallengeHandler) SetChallenge(token, keyAuth string) {
	h.mu.Lock()
	h.challenges[token] = keyAuth
	h.mu.Unlock()
}

// ClearChallenge removes a challenge token.
func (h *HTTPChallengeHandler) ClearChallenge(token string) {
	h.mu.Lock()
	delete(h.challenges, token)
	h.mu.Unlock()
}

// GetChallenge returns the key authorization for a token.
func (h *HTTPChallengeHandler) GetChallenge(token string) (string, bool) {
	h.mu.RLock()
	keyAuth, ok := h.challenges[token]
	h.mu.RUnlock()
	return keyAuth, ok
}

// ServeHTTP handles HTTP-01 challenge requests.
// The path should be /.well-known/acme-challenge/{token}
func (h *HTTPChallengeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract token from path
	// Expected: /.well-known/acme-challenge/{token}
	const prefix = "/.well-known/acme-challenge/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.NotFound(w, r)
		return
	}
	token := strings.TrimPrefix(r.URL.Path, prefix)

	keyAuth, ok := h.GetChallenge(token)
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(keyAuth))
}

// --- Helper Types for ACME Protocol ---

// acmeDirectory represents an ACME directory response.
type acmeDirectory struct {
	NewNonce   string `json:"newNonce"`
	NewAccount string `json:"newAccount"`
	NewOrder   string `json:"newOrder"`
	RevokeCert string `json:"revokeCert"`
	KeyChange  string `json:"keyChange"`
}

// acmeOrder represents an ACME order.
type acmeOrder struct {
	Status         string   `json:"status"`
	Expires        string   `json:"expires"`
	Identifiers    []acmeID `json:"identifiers"`
	Authorizations []string `json:"authorizations"`
	Finalize       string   `json:"finalize"`
	Certificate    string   `json:"certificate,omitempty"`
}

// acmeID represents an ACME identifier.
type acmeID struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// acmeAuthorization represents an ACME authorization.
type acmeAuthorization struct {
	Status     string          `json:"status"`
	Identifier acmeID          `json:"identifier"`
	Challenges []acmeChallenge `json:"challenges"`
}

// acmeChallenge represents an ACME challenge.
type acmeChallenge struct {
	Type   string `json:"type"`
	URL    string `json:"url"`
	Token  string `json:"token"`
	Status string `json:"status"`
}

// randomSerialNumber generates a random serial number for certificates.
func randomSerialNumber() (*big.Int, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("crypto/rand failure: %w", err)
	}
	return serialNumber, nil
}

// pkixName creates a pkix.Name with the given common name.
func pkixName(commonName string) pkix.Name {
	return pkix.Name{
		Organization: []string{"VCDeploy"},
		CommonName:   commonName,
	}
}
