// Package testutil provides shared testing utilities for vcdeploy.
package testutil

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"path/filepath"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// TestCABundle contains all components needed for testing with a CA.
type TestCABundle struct {
	Store     storage.Store
	KMS       *security.KMS
	CAManager *security.CAManager
	TrustPool *x509.CertPool

	// CleanupFuncs should be called in deferred order
	CleanupFuncs []func()
}

// Close cleans up all test resources.
func (b *TestCABundle) Close() {
	// Call cleanup funcs in reverse order
	for i := len(b.CleanupFuncs) - 1; i >= 0; i-- {
		b.CleanupFuncs[i]()
	}
}

// NewTestCA creates a complete CA infrastructure for testing.
// Returns a TestCABundle with initialized CA, KMS, and trust pool.
// The bundle should be cleaned up with Close() or in a deferred call.
//
// Example:
//
//	bundle := testutil.NewTestCA(t)
//	defer bundle.Close()
//	// Use bundle.CAManager, bundle.TrustPool, etc.
func NewTestCA(t *testing.T) *TestCABundle {
	t.Helper()

	bundle := &TestCABundle{}

	// Create temp directory for test database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Open store
	store, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("testutil.NewTestCA: open store: %v", err)
	}
	bundle.Store = store
	bundle.CleanupFuncs = append(bundle.CleanupFuncs, func() {
		_ = store.Close() // #nosec G104 - best effort cleanup in test
	})

	// Create and initialize KMS
	ctx := context.Background()
	masterKey, err := security.GenerateMasterKey()
	if err != nil {
		bundle.Close()
		t.Fatalf("testutil.NewTestCA: GenerateMasterKey: %v", err)
	}
	kms, err := security.NewKMS(ctx, store, nil, masterKey)
	if err != nil {
		bundle.Close()
		t.Fatalf("testutil.NewTestCA: NewKMS: %v", err)
	}
	bundle.KMS = kms

	if err := kms.Initialize(ctx); err != nil {
		bundle.Close()
		t.Fatalf("testutil.NewTestCA: KMS.Initialize: %v", err)
	}

	// Create and initialize CA Manager
	mgr, err := security.NewCAManager(store, kms, nil)
	if err != nil {
		bundle.Close()
		t.Fatalf("testutil.NewTestCA: NewCAManager: %v", err)
	}
	bundle.CAManager = mgr

	// Initialize CA with test-appropriate config
	caConfig := security.DefaultCAConfig()
	caConfig.CAValidity = 24 * time.Hour // Short validity for tests
	caConfig.CertValidity = 1 * time.Hour
	caConfig.Organization = "Test Organization"
	caConfig.CommonName = "Test CA"

	if err := mgr.Initialize(ctx, caConfig); err != nil {
		bundle.Close()
		t.Fatalf("testutil.NewTestCA: CAManager.Initialize: %v", err)
	}

	// Get trust pool
	bundle.TrustPool = mgr.GetTrustPool()

	return bundle
}

// TestAgentCert contains an agent certificate and its key.
type TestAgentCert struct {
	Certificate *x509.Certificate
	AgentCert   *security.AgentCertificate
	AgentID     string
}

// NewTestAgentCert issues a test certificate for an agent.
// The certificate is signed by the CA in the bundle.
//
// Example:
//
//	bundle := testutil.NewTestCA(t)
//	defer bundle.Close()
//	agentCert := testutil.NewTestAgentCert(t, bundle, "agent-001")
func NewTestAgentCert(t *testing.T, bundle *TestCABundle, agentID string) *TestAgentCert {
	t.Helper()

	ctx := context.Background()

	agentCert, err := bundle.CAManager.IssueAgentCertificate(ctx, agentID, "test-host")
	if err != nil {
		t.Fatalf("testutil.NewTestAgentCert: IssueAgentCertificate: %v", err)
	}

	return &TestAgentCert{
		Certificate: agentCert.Certificate,
		AgentCert:   agentCert,
		AgentID:     agentID,
	}
}

// TLSCert returns a tls.Certificate from the agent certificate.
// This can be used for TLS client authentication.
func (c *TestAgentCert) TLSCert(t *testing.T) *tls.Certificate {
	t.Helper()

	if c.AgentCert.CertificatePEM == "" || c.AgentCert.PrivateKeyPEM == "" {
		t.Fatal("testutil.TestAgentCert.TLSCert: certificate or key PEM not available")
	}

	cert, err := tls.X509KeyPair([]byte(c.AgentCert.CertificatePEM), []byte(c.AgentCert.PrivateKeyPEM))
	if err != nil {
		t.Fatalf("testutil.TestAgentCert.TLSCert: X509KeyPair: %v", err)
	}

	return &cert
}

// SelfSignedCert generates a self-signed certificate for testing.
// This creates a minimal certificate without going through the CA infrastructure.
// Useful for testing certificate validation failure cases.
func SelfSignedCert(t *testing.T, commonName string, validFor time.Duration) *tls.Certificate {
	t.Helper()

	// Generate key
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("testutil.SelfSignedCert: GenerateKey: %v", err)
	}

	// Create certificate template
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(validFor),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Self-sign
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("testutil.SelfSignedCert: CreateCertificate: %v", err)
	}

	return &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  priv,
	}
}

// ExpiredCert generates an expired self-signed certificate for testing.
// Useful for testing certificate expiration handling.
func ExpiredCert(t *testing.T, commonName string) *tls.Certificate {
	t.Helper()

	// Generate key
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("testutil.ExpiredCert: GenerateKey: %v", err)
	}

	// Create certificate template with dates in the past
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:             time.Now().Add(-48 * time.Hour),
		NotAfter:              time.Now().Add(-24 * time.Hour), // Expired 24 hours ago
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	// Self-sign
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("testutil.ExpiredCert: CreateCertificate: %v", err)
	}

	return &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  priv,
	}
}
