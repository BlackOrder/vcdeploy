// Package server provides HTTP and gRPC servers for vcdeploy.
package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/acme/autocert"
)

// ACMEClient provides automatic certificate management using ACME protocol.
// Uses golang.org/x/crypto/acme/autocert for production-ready ACME support.
type ACMEClient struct {
	manager      *autocert.Manager
	logger       *zap.Logger
	domains      []string
	cacheDir     string
	email        string
	directoryURL string

	mu          sync.RWMutex
	currentCert *tls.Certificate
	certExpiry  time.Time

	// For testing - use self-signed certificates instead of ACME
	testMode bool
}

// ACMEClientConfig holds configuration for the ACME client.
type ACMEClientConfig struct {
	Logger       *zap.Logger
	DirectoryURL string   // ACME directory URL (e.g., Let's Encrypt), empty for default
	Email        string   // Contact email for ACME registration
	Domains      []string // Domains to obtain certificates for
	CacheDir     string   // Directory to cache certificates (defaults to ~/.vcdeploy/certs)
	TestMode     bool     // Use self-signed certificates for testing
}

// ACME directory URLs
const (
	LetsEncryptProduction = "https://acme-v02.api.letsencrypt.org/directory"
	LetsEncryptStaging    = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

// NewACMEClient creates a new ACME client.
func NewACMEClient(cfg ACMEClientConfig) (*ACMEClient, error) {
	if len(cfg.Domains) == 0 {
		return nil, fmt.Errorf("at least one domain required")
	}
	if cfg.Email == "" && !cfg.TestMode {
		return nil, fmt.Errorf("email required for ACME registration")
	}

	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	cacheDir := cfg.CacheDir
	if cacheDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			homeDir = "/tmp"
		}
		cacheDir = filepath.Join(homeDir, ".vcdeploy", "certs")
	}

	// Ensure cache directory exists
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return nil, fmt.Errorf("create certificate cache directory: %w", err)
	}

	directoryURL := cfg.DirectoryURL
	if directoryURL == "" {
		directoryURL = LetsEncryptProduction
	}

	client := &ACMEClient{
		logger:       logger,
		domains:      cfg.Domains,
		cacheDir:     cacheDir,
		email:        cfg.Email,
		directoryURL: directoryURL,
		testMode:     cfg.TestMode,
	}

	// Only create autocert.Manager if not in test mode
	if !cfg.TestMode {
		client.manager = &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			Cache:      autocert.DirCache(cacheDir),
			HostPolicy: autocert.HostWhitelist(cfg.Domains...),
			Email:      cfg.Email,
		}
	}

	return client, nil
}

// GetCertificate returns a valid TLS certificate, obtaining or renewing as needed.
// This implements tls.Config.GetCertificate.
func (c *ACMEClient) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if c.testMode {
		return c.getSelfSignedCertificate(hello)
	}

	// Use autocert manager for production
	return c.manager.GetCertificate(hello)
}

// getSelfSignedCertificate returns a self-signed certificate for testing.
func (c *ACMEClient) getSelfSignedCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	c.mu.RLock()
	cert := c.currentCert
	expiry := c.certExpiry
	c.mu.RUnlock()

	// Check if we have a valid certificate (with 24h buffer)
	if cert != nil && time.Now().Before(expiry.Add(-24*time.Hour)) {
		return cert, nil
	}

	// Generate new self-signed certificate
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring lock
	if c.currentCert != nil && time.Now().Before(c.certExpiry.Add(-24*time.Hour)) {
		return c.currentCert, nil
	}

	ctx := hello.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	return c.generateSelfSignedCertificate(ctx)
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
	}
}

// HTTPHandler returns an http.Handler that responds to ACME HTTP-01 challenges.
// This should be mounted at /.well-known/acme-challenge/ on port 80.
func (c *ACMEClient) HTTPHandler(fallback http.Handler) http.Handler {
	if c.testMode || c.manager == nil {
		return fallback
	}
	return c.manager.HTTPHandler(fallback)
}

// generateSelfSignedCertificate generates a self-signed certificate for testing.
func (c *ACMEClient) generateSelfSignedCertificate(ctx context.Context) (*tls.Certificate, error) {
	_ = ctx // Reserved for future use

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
		Subject:               pkixName("VCDeploy Self-Signed Certificate"),
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

	// Optionally save to cache directory for persistence
	if c.cacheDir != "" {
		certPath := filepath.Join(c.cacheDir, "self-signed.crt")
		keyPath := filepath.Join(c.cacheDir, "self-signed.key")
		if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
			c.logger.Warn("failed to save self-signed certificate", zap.Error(err))
		}
		if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
			c.logger.Warn("failed to save self-signed key", zap.Error(err))
		}
	}

	// Update current cert
	c.currentCert = &tlsCert
	c.certExpiry = template.NotAfter

	c.logger.Info("generated self-signed certificate",
		zap.Strings("domains", c.domains),
		zap.Time("expires", template.NotAfter),
	)

	return &tlsCert, nil
}

// StartRenewalLoop starts a background goroutine that monitors certificate expiry.
// For autocert, this is mostly informational since autocert handles renewal automatically.
func (c *ACMEClient) StartRenewalLoop(ctx context.Context) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				c.logger.Error("panic in certificate monitoring loop", zap.Any("panic", r))
			}
		}()

		ticker := time.NewTicker(12 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				status := c.GetStatus()
				if status.NeedsRenewal {
					c.logger.Info("certificate needs renewal",
						zap.Int("days_remaining", status.DaysRemaining),
						zap.Time("expires", status.NotAfter),
					)
				}
			}
		}
	}()
}

// CertificateStatus returns information about the current certificate.
type CertificateStatus struct {
	HasCertificate bool      `json:"has_certificate"`
	Domains        []string  `json:"domains"`
	NotBefore      time.Time `json:"not_before"`
	NotAfter       time.Time `json:"not_after"`
	Issuer         string    `json:"issuer"`
	DaysRemaining  int       `json:"days_remaining"`
	NeedsRenewal   bool      `json:"needs_renewal"`
	TestMode       bool      `json:"test_mode"`
}

// GetStatus returns the current certificate status.
func (c *ACMEClient) GetStatus() CertificateStatus {
	status := CertificateStatus{
		Domains:  c.domains,
		TestMode: c.testMode,
	}

	if c.testMode {
		c.mu.RLock()
		cert := c.currentCert
		expiry := c.certExpiry
		c.mu.RUnlock()

		if cert == nil {
			return status
		}

		status.HasCertificate = true
		status.NotAfter = expiry

		if len(cert.Certificate) > 0 {
			x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
			if err == nil {
				status.NotBefore = x509Cert.NotBefore
				status.NotAfter = x509Cert.NotAfter
				status.Issuer = x509Cert.Issuer.CommonName
			}
		}

		status.DaysRemaining = int(time.Until(status.NotAfter).Hours() / 24)
		status.NeedsRenewal = status.DaysRemaining < 7
		return status
	}

	// For production, try to get certificate info from cache
	if c.manager != nil && len(c.domains) > 0 {
		// Try to load cached certificate to check status
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		certData, err := c.manager.Cache.Get(ctx, c.domains[0])
		if err == nil && len(certData) > 0 {
			cert, err := tls.X509KeyPair(certData, certData)
			if err == nil && len(cert.Certificate) > 0 {
				x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
				if err == nil {
					status.HasCertificate = true
					status.NotBefore = x509Cert.NotBefore
					status.NotAfter = x509Cert.NotAfter
					status.Issuer = x509Cert.Issuer.CommonName
					status.DaysRemaining = int(time.Until(status.NotAfter).Hours() / 24)
					status.NeedsRenewal = status.DaysRemaining < 30 // autocert renews at 30 days
				}
			}
		}
	}

	return status
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
