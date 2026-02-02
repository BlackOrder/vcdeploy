// Package server provides HTTP and gRPC servers for vcdeploy.
package server

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"golang.org/x/crypto/acme/autocert"
)

// DBCertCache implements autocert.Cache using the storage.ACMEStore interface.
// This provides database-backed certificate persistence with memory layer caching.
type DBCertCache struct {
	store storage.ACMEStore
}

// NewDBCertCache creates a new database-backed certificate cache.
func NewDBCertCache(store storage.ACMEStore) *DBCertCache {
	return &DBCertCache{store: store}
}

// Get retrieves a certificate from the database cache.
// The key format from autocert is typically the domain name or a special key like "acme_account+key".
func (c *DBCertCache) Get(ctx context.Context, key string) ([]byte, error) {
	// autocert uses special keys for account data
	if strings.HasPrefix(key, "acme_account") {
		return c.getAccount(ctx, key)
	}

	// Otherwise it's a certificate for a domain
	cert, err := c.store.GetACMECertificate(ctx, key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, autocert.ErrCacheMiss
		}
		return nil, err
	}

	// Return the stored PEM data (certificate + private key concatenated)
	// autocert expects the full PEM bundle
	return buildCertBundle(cert), nil
}

// Put stores a certificate in the database cache.
func (c *DBCertCache) Put(ctx context.Context, key string, data []byte) error {
	// Handle account data separately
	if strings.HasPrefix(key, "acme_account") {
		return c.putAccount(ctx, key, data)
	}

	// Parse the certificate to extract metadata
	cert, err := parseCertBundle(key, data)
	if err != nil {
		return err
	}

	return c.store.SaveACMECertificate(ctx, cert)
}

// Delete removes a certificate from the database cache.
func (c *DBCertCache) Delete(ctx context.Context, key string) error {
	if strings.HasPrefix(key, "acme_account") {
		return c.deleteAccount(ctx, key)
	}

	err := c.store.DeleteACMECertificate(ctx, key)
	if errors.Is(err, storage.ErrNotFound) {
		return nil // autocert expects Delete to be idempotent
	}
	return err
}

// getAccount retrieves ACME account data from the database.
func (c *DBCertCache) getAccount(ctx context.Context, key string) ([]byte, error) {
	// Extract email from key (format: "acme_account+key" or similar)
	// autocert uses the email as part of the key
	email := extractEmailFromKey(key)
	if email == "" {
		// Fallback: try to get any account
		email = "default"
	}

	account, err := c.store.GetACMEAccount(ctx, email)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, autocert.ErrCacheMiss
		}
		return nil, err
	}

	return account.PrivateKeyEncrypted, nil
}

// putAccount stores ACME account data in the database.
func (c *DBCertCache) putAccount(ctx context.Context, key string, data []byte) error {
	email := extractEmailFromKey(key)
	if email == "" {
		email = "default"
	}

	account := &storage.ACMEAccount{
		Email:               email,
		PrivateKeyEncrypted: data,
		DirectoryURL:        LetsEncryptProduction, // Will be overwritten on actual use
		CreatedAt:           time.Now(),
	}

	return c.store.SaveACMEAccount(ctx, account)
}

// deleteAccount removes ACME account data from the database.
func (c *DBCertCache) deleteAccount(ctx context.Context, key string) error {
	email := extractEmailFromKey(key)
	if email == "" {
		email = "default"
	}

	err := c.store.DeleteACMEAccount(ctx, email)
	if errors.Is(err, storage.ErrNotFound) {
		return nil // idempotent
	}
	return err
}

// buildCertBundle reconstructs the PEM bundle from stored certificate data.
func buildCertBundle(cert *storage.ACMECertificate) []byte {
	// autocert stores and expects: certificate PEM + private key PEM concatenated
	var bundle []byte
	bundle = append(bundle, []byte(cert.CertificatePEM)...)
	if len(cert.PrivateKeyEncrypted) > 0 {
		bundle = append(bundle, cert.PrivateKeyEncrypted...)
	}
	return bundle
}

// parseCertBundle parses a PEM bundle into certificate metadata.
func parseCertBundle(domain string, data []byte) (*storage.ACMECertificate, error) {
	cert := &storage.ACMECertificate{
		Domain:    domain,
		AutoRenew: true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	remaining := data
	var certPEM, keyPEM []byte

	for len(remaining) > 0 {
		block, rest := pem.Decode(remaining)
		if block == nil {
			break
		}
		remaining = rest

		blockBytes := pem.EncodeToMemory(block)
		switch block.Type {
		case "CERTIFICATE":
			certPEM = append(certPEM, blockBytes...)
			// Parse the first certificate to extract metadata
			if cert.Issuer == "" {
				if x509Cert, err := x509.ParseCertificate(block.Bytes); err == nil {
					cert.Issuer = x509Cert.Issuer.CommonName
					cert.NotBefore = x509Cert.NotBefore
					cert.NotAfter = x509Cert.NotAfter
				}
			}
		case "RSA PRIVATE KEY", "EC PRIVATE KEY", "PRIVATE KEY":
			keyPEM = append(keyPEM, blockBytes...)
		}
	}

	cert.CertificatePEM = string(certPEM)
	cert.PrivateKeyEncrypted = keyPEM // Note: Should be encrypted before storage in production

	now := time.Now()
	cert.LastRenewal = &now

	return cert, nil
}

// extractEmailFromKey attempts to extract an email from the autocert cache key.
// autocert may use formats like "acme_account+key" where we need to map to our email-keyed storage.
func extractEmailFromKey(key string) string {
	// autocert doesn't include email in the key, so we use a sentinel
	// The actual email is set when configuring the manager
	return ""
}

// Verify DBCertCache implements autocert.Cache
var _ autocert.Cache = (*DBCertCache)(nil)
