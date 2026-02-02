package security

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"path/filepath"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
)

func setupTestCADB(t *testing.T) (storage.Store, *KMS) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	// Create KMS
	kms, err := NewKMS(context.Background(), store, nil)
	if err != nil {
		t.Fatalf("NewKMS: %v", err)
	}

	ctx := context.Background()
	if err := kms.Initialize(ctx); err != nil {
		t.Fatalf("KMS.Initialize: %v", err)
	}

	return store, kms
}

func TestNewCAManager(t *testing.T) {
	db, kms := setupTestCADB(t)
	defer db.Close()

	mgr, err := NewCAManager(db, kms, nil)
	if err != nil {
		t.Fatalf("NewCAManager() error: %v", err)
	}

	if mgr == nil {
		t.Fatal("NewCAManager() returned nil")
	}
}

func TestCAManagerInitialize(t *testing.T) {
	db, kms := setupTestCADB(t)
	defer db.Close()

	mgr, err := NewCAManager(db, kms, nil)
	if err != nil {
		t.Fatalf("NewCAManager() error: %v", err)
	}

	ctx := context.Background()
	config := DefaultCAConfig()

	if err := mgr.Initialize(ctx, config); err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}

	// Should have a current CA
	ca := mgr.GetCurrentCA()
	if ca == nil {
		t.Fatal("GetCurrentCA() returned nil after Initialize")
	}

	if ca.Status != CAStatusActive {
		t.Errorf("ca.Status = %v, want %v", ca.Status, CAStatusActive)
	}

	if ca.Version != 1 {
		t.Errorf("ca.Version = %d, want 1", ca.Version)
	}

	if ca.Certificate == nil {
		t.Error("ca.Certificate should not be nil")
	}

	if !ca.Certificate.IsCA {
		t.Error("certificate should be a CA")
	}
}

func TestCAManagerDoubleInitialize(t *testing.T) {
	db, kms := setupTestCADB(t)
	defer db.Close()

	mgr, _ := NewCAManager(db, kms, nil)
	ctx := context.Background()
	config := DefaultCAConfig()

	_ = mgr.Initialize(ctx, config)
	caBefore := mgr.GetCurrentCA()

	// Second initialize should be no-op
	_ = mgr.Initialize(ctx, config)
	caAfter := mgr.GetCurrentCA()

	if caBefore.ID != caAfter.ID {
		t.Error("second Initialize() should not create new CA")
	}
}

func TestCAManagerGetTrustPool(t *testing.T) {
	db, kms := setupTestCADB(t)
	defer db.Close()

	mgr, _ := NewCAManager(db, kms, nil)
	ctx := context.Background()
	config := DefaultCAConfig()
	_ = mgr.Initialize(ctx, config)

	pool := mgr.GetTrustPool()
	if pool == nil {
		t.Fatal("GetTrustPool() returned nil")
	}

	// Pool should contain the CA
	ca := mgr.GetCurrentCA()
	if ca.Certificate == nil {
		t.Fatal("CA certificate is nil")
	}

	// Verify the pool can verify a certificate signed by this CA
	// (We'll do this implicitly through IssueAgentCertificate test)
}

func TestCAManagerIssueAgentCertificate(t *testing.T) {
	db, kms := setupTestCADB(t)
	defer db.Close()

	mgr, _ := NewCAManager(db, kms, nil)
	ctx := context.Background()
	config := DefaultCAConfig()
	_ = mgr.Initialize(ctx, config)

	agentID := "agent-001"
	hostname := "agent1.example.com"

	cert, err := mgr.IssueAgentCertificate(ctx, agentID, hostname)
	if err != nil {
		t.Fatalf("IssueAgentCertificate() error: %v", err)
	}

	if cert == nil {
		t.Fatal("IssueAgentCertificate() returned nil")
	}

	// Check certificate properties
	if cert.AgentID != agentID {
		t.Errorf("cert.AgentID = %s, want %s", cert.AgentID, agentID)
	}

	if cert.Status != CertStatusActive {
		t.Errorf("cert.Status = %v, want %v", cert.Status, CertStatusActive)
	}

	if cert.Certificate == nil {
		t.Error("cert.Certificate should not be nil")
	}

	if cert.CertificatePEM == "" {
		t.Error("cert.CertificatePEM should not be empty")
	}

	if cert.PrivateKeyPEM == "" {
		t.Error("cert.PrivateKeyPEM should not be empty")
	}

	// Verify certificate is signed by CA
	ca := mgr.GetCurrentCA()
	roots := x509.NewCertPool()
	roots.AddCert(ca.Certificate)

	_, err = cert.Certificate.Verify(x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	if err != nil {
		t.Errorf("certificate verification failed: %v", err)
	}
}

func TestCAManagerRenewAgentCertificate(t *testing.T) {
	db, kms := setupTestCADB(t)
	defer db.Close()

	mgr, _ := NewCAManager(db, kms, nil)
	ctx := context.Background()
	config := DefaultCAConfig()
	_ = mgr.Initialize(ctx, config)

	agentID := "agent-001"
	hostname := "agent1.example.com"

	// Issue first certificate
	cert1, _ := mgr.IssueAgentCertificate(ctx, agentID, hostname)

	// Renew
	cert2, err := mgr.RenewAgentCertificate(ctx, agentID, hostname)
	if err != nil {
		t.Fatalf("RenewAgentCertificate() error: %v", err)
	}

	if cert2.SerialNumber == cert1.SerialNumber {
		t.Error("renewed certificate should have different serial number")
	}

	// Old certificate should be revoked
	oldCert, _ := mgr.GetAgentCertificate(ctx, agentID)
	// GetAgentCertificate returns only active certs, so if it returns the new one, old is revoked
	if oldCert.SerialNumber != cert2.SerialNumber {
		t.Error("old certificate should be revoked after renewal")
	}
}

func TestCAManagerRotateCA(t *testing.T) {
	db, kms := setupTestCADB(t)
	defer db.Close()

	mgr, _ := NewCAManager(db, kms, nil)
	ctx := context.Background()
	config := DefaultCAConfig()
	_ = mgr.Initialize(ctx, config)

	// Issue certificate with old CA
	cert1, _ := mgr.IssueAgentCertificate(ctx, "agent-001", "agent1.example.com")

	oldCA := mgr.GetCurrentCA()
	oldCAID := oldCA.ID

	// Rotate CA
	newCA, err := mgr.RotateCA(ctx, config)
	if err != nil {
		t.Fatalf("RotateCA() error: %v", err)
	}

	if newCA.ID == oldCAID {
		t.Error("new CA should have different ID")
	}

	if newCA.Version != 2 {
		t.Errorf("newCA.Version = %d, want 2", newCA.Version)
	}

	// Old certificate should still be verifiable (old CA in trust pool)
	pool := mgr.GetTrustPool()
	_, err = cert1.Certificate.Verify(x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	if err != nil {
		t.Errorf("old certificate should still verify after CA rotation: %v", err)
	}

	// New certificates should be signed by new CA
	cert2, _ := mgr.IssueAgentCertificate(ctx, "agent-002", "agent2.example.com")
	if cert2.CAID != newCA.ID {
		t.Error("new certificate should be signed by new CA")
	}
}

func TestCAManagerListCAs(t *testing.T) {
	db, kms := setupTestCADB(t)
	defer db.Close()

	mgr, _ := NewCAManager(db, kms, nil)
	ctx := context.Background()
	config := DefaultCAConfig()
	_ = mgr.Initialize(ctx, config)

	// Rotate a few times
	_, _ = mgr.RotateCA(ctx, config)
	_, _ = mgr.RotateCA(ctx, config)

	cas, err := mgr.ListCAs(ctx)
	if err != nil {
		t.Fatalf("ListCAs() error: %v", err)
	}

	if len(cas) != 3 {
		t.Errorf("len(cas) = %d, want 3", len(cas))
	}

	// Should be sorted by version descending
	if cas[0].Version != 3 {
		t.Errorf("cas[0].Version = %d, want 3", cas[0].Version)
	}
}

func TestCAManagerRevokeCertificate(t *testing.T) {
	db, kms := setupTestCADB(t)
	defer db.Close()

	mgr, _ := NewCAManager(db, kms, nil)
	ctx := context.Background()
	config := DefaultCAConfig()
	_ = mgr.Initialize(ctx, config)

	cert, _ := mgr.IssueAgentCertificate(ctx, "agent-001", "agent1.example.com")

	if err := mgr.RevokeCertificate(ctx, cert.SerialNumber, "test revocation"); err != nil {
		t.Fatalf("RevokeCertificate() error: %v", err)
	}

	// Should not be returned by GetAgentCertificate (active only)
	activeCert, _ := mgr.GetAgentCertificate(ctx, "agent-001")
	if activeCert != nil {
		t.Error("revoked certificate should not be returned as active")
	}
}

func TestCAManagerShouldRenew(t *testing.T) {
	db, kms := setupTestCADB(t)
	defer db.Close()

	mgr, _ := NewCAManager(db, kms, nil)
	ctx := context.Background()
	config := DefaultCAConfig()
	_ = mgr.Initialize(ctx, config)

	cert, _ := mgr.IssueAgentCertificate(ctx, "agent-001", "agent1.example.com")

	// Fresh certificate should not need renewal
	threshold := 6 * 30 * 24 * time.Hour // 6 months
	if mgr.ShouldRenew(cert, threshold) {
		t.Error("fresh certificate should not need renewal")
	}

	// Nil certificate should need renewal
	if !mgr.ShouldRenew(nil, threshold) {
		t.Error("nil certificate should need renewal")
	}
}

func TestCAManagerProcessExpiredCertificates(t *testing.T) {
	store, kms := setupTestCADB(t)
	defer store.Close()

	mgr, _ := NewCAManager(store, kms, nil)
	ctx := context.Background()
	config := DefaultCAConfig()
	_ = mgr.Initialize(ctx, config)

	// Issue certificate
	_, _ = mgr.IssueAgentCertificate(ctx, "agent-001", "agent1.example.com")

	// Manually set certificate as expired using storage's Conn() for backdoor testing
	conn := store.Conn()
	if conn != nil {
		_, _ = conn.Exec(`UPDATE agent_certificates SET not_after = datetime('now', '-1 day')`)
	}

	count, err := mgr.ProcessExpiredCertificates(ctx)
	if err != nil {
		t.Fatalf("ProcessExpiredCertificates() error: %v", err)
	}

	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestCAManagerGetAgentCertificate(t *testing.T) {
	db, kms := setupTestCADB(t)
	defer db.Close()

	mgr, _ := NewCAManager(db, kms, nil)
	ctx := context.Background()
	config := DefaultCAConfig()
	_ = mgr.Initialize(ctx, config)

	// No certificate yet
	cert, err := mgr.GetAgentCertificate(ctx, "agent-001")
	if err != nil {
		t.Fatalf("GetAgentCertificate() error: %v", err)
	}
	if cert != nil {
		t.Error("should return nil for non-existent agent")
	}

	// Issue certificate
	issued, _ := mgr.IssueAgentCertificate(ctx, "agent-001", "agent1.example.com")

	// Now should return it
	cert, err = mgr.GetAgentCertificate(ctx, "agent-001")
	if err != nil {
		t.Fatalf("GetAgentCertificate() error: %v", err)
	}
	if cert == nil {
		t.Fatal("should return certificate after issuance")
	}

	if cert.SerialNumber != issued.SerialNumber {
		t.Error("should return the issued certificate")
	}
}

func TestCAManagerPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create initial CA using storage package
	store1, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	ctx := context.Background()
	kms1, _ := NewKMS(ctx, store1, nil)
	_ = kms1.Initialize(ctx)

	mgr1, _ := NewCAManager(store1, kms1, nil)
	config := DefaultCAConfig()
	_ = mgr1.Initialize(ctx, config)

	caID := mgr1.GetCurrentCA().ID

	// Issue a certificate
	cert1, _ := mgr1.IssueAgentCertificate(ctx, "agent-001", "agent1.example.com")
	store1.Close()

	// Reopen
	store2, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer store2.Close()

	kms2, _ := NewKMS(ctx, store2, nil)
	mgr2, err := NewCAManager(store2, kms2, nil)
	if err != nil {
		t.Fatalf("NewCAManager() after reopen error: %v", err)
	}

	// Should load existing CA
	if mgr2.GetCurrentCA() == nil {
		t.Fatal("should have loaded existing CA")
	}

	if mgr2.GetCurrentCA().ID != caID {
		t.Error("should have loaded same CA")
	}

	// Certificate should still be valid
	pool := mgr2.GetTrustPool()
	_, err = cert1.Certificate.Verify(x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	if err != nil {
		t.Errorf("certificate should still verify after reopen: %v", err)
	}
}

func TestDefaultCAConfig(t *testing.T) {
	config := DefaultCAConfig()

	if config.CommonName == "" {
		t.Error("CommonName should not be empty")
	}

	if config.Organization == "" {
		t.Error("Organization should not be empty")
	}

	if config.CAValidity <= 0 {
		t.Error("CAValidity should be positive")
	}

	if config.CertValidity <= 0 {
		t.Error("CertValidity should be positive")
	}

	if config.RenewalThreshold <= 0 {
		t.Error("RenewalThreshold should be positive")
	}
}

func TestCAManager_GetTLSConfig(t *testing.T) {
	db, kms := setupTestCADB(t)
	defer db.Close()

	manager, err := NewCAManager(db, kms, nil)
	if err != nil {
		t.Fatalf("NewCAManager() error = %v", err)
	}

	ctx := context.Background()
	config := DefaultCAConfig()
	if err := manager.Initialize(ctx, config); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	// Issue an agent certificate to use as server cert
	agentCert, err := manager.IssueAgentCertificate(ctx, "server", "localhost")
	if err != nil {
		t.Fatalf("IssueAgentCertificate() error = %v", err)
	}

	// Parse the cert for tls.Certificate
	tlsCert, err := tls.X509KeyPair([]byte(agentCert.CertificatePEM), []byte(agentCert.PrivateKeyPEM))
	if err != nil {
		t.Fatalf("tls.X509KeyPair() error = %v", err)
	}

	// Get TLS config
	tlsConfig := manager.GetTLSConfig(tlsCert)

	if tlsConfig == nil {
		t.Fatal("GetTLSConfig() returned nil")
	}

	if len(tlsConfig.Certificates) != 1 {
		t.Errorf("GetTLSConfig() Certificates count = %d, want 1", len(tlsConfig.Certificates))
	}

	if tlsConfig.ClientCAs == nil {
		t.Error("GetTLSConfig() ClientCAs should not be nil")
	}

	if tlsConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("GetTLSConfig() ClientAuth = %v, want RequireAndVerifyClientCert", tlsConfig.ClientAuth)
	}

	if tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("GetTLSConfig() MinVersion = %v, want TLS 1.2", tlsConfig.MinVersion)
	}
}
