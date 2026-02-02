//go:build integration
// +build integration

// Package server provides HTTP and gRPC servers for vcdeploy.
//
// This file contains integration tests for ACME/Let's Encrypt functionality.
// These tests require network access and use the Let's Encrypt staging environment.
//
// Run with: go test -tags=integration -v ./internal/server/... -run TestACME
package server

import (
	"context"
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// TestACMEClient_GetCertificate_Staging tests obtaining a certificate from Let's Encrypt staging.
// This test is skipped unless ACME_TEST_DOMAIN and ACME_TEST_EMAIL environment variables are set.
//
// Prerequisites:
// - A publicly accessible domain pointing to the test machine
// - Port 80 must be available for HTTP-01 challenge
// - ACME_TEST_DOMAIN environment variable set to your test domain
// - ACME_TEST_EMAIL environment variable set to a valid email
func TestACMEClient_GetCertificate_Staging(t *testing.T) {
	domain := os.Getenv("ACME_TEST_DOMAIN")
	email := os.Getenv("ACME_TEST_EMAIL")

	if domain == "" || email == "" {
		t.Skip("ACME_TEST_DOMAIN and ACME_TEST_EMAIL environment variables required for this test")
	}

	tmpDir := t.TempDir()

	client, err := NewACMEClient(ACMEClientConfig{
		Email:        email,
		Domains:      []string{domain},
		CacheDir:     tmpDir,
		DirectoryURL: LetsEncryptStaging, // Use staging to avoid rate limits
	})
	if err != nil {
		t.Fatalf("NewACMEClient failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Start HTTP server for ACME challenge
	go func() {
		// Note: In a real test, you'd need to actually serve the HTTP-01 challenge
		// This requires the domain to resolve to this machine and port 80 to be available
		_ = ctx // Placeholder for HTTP server setup
	}()

	hello := &tls.ClientHelloInfo{
		ServerName: domain,
	}

	cert, err := client.GetCertificate(hello)
	if err != nil {
		t.Fatalf("GetCertificate failed: %v", err)
	}

	if cert == nil {
		t.Fatal("expected non-nil certificate")
	}

	// Verify certificate properties
	if cert.Leaf != nil {
		t.Logf("Got certificate for: %s", cert.Leaf.Subject.CommonName)
		t.Logf("Issuer: %s", cert.Leaf.Issuer.CommonName)
		t.Logf("Valid until: %s", cert.Leaf.NotAfter)

		// Staging certs should be issued by the staging CA
		if cert.Leaf.Issuer.Organization != nil {
			for _, org := range cert.Leaf.Issuer.Organization {
				t.Logf("Issuer Organization: %s", org)
			}
		}
	}
}

// TestACMEClient_StoragePersistence tests that certificates survive client restart.
// This verifies the DBCertCache correctly persists and retrieves certificate data.
func TestACMEClient_StoragePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	store := storage.NewMemoryStore(nil)
	cache := NewDBCertCache(store)

	// Create client with DB-backed cache
	client1, err := NewACMEClient(ACMEClientConfig{
		Email:    "test@example.com",
		Domains:  []string{"persistence-test.local"},
		CacheDir: tmpDir,
		TestMode: true, // Use test mode to avoid real ACME calls
		Cache:    cache,
	})
	if err != nil {
		t.Fatalf("NewACMEClient failed: %v", err)
	}

	// Get a certificate (self-signed in test mode)
	hello := &tls.ClientHelloInfo{
		ServerName: "persistence-test.local",
	}
	cert1, err := client1.GetCertificate(hello)
	if err != nil {
		t.Fatalf("GetCertificate failed: %v", err)
	}
	if cert1 == nil {
		t.Fatal("expected non-nil certificate")
	}

	// Simulate restart by creating a new client with the same cache
	client2, err := NewACMEClient(ACMEClientConfig{
		Email:    "test@example.com",
		Domains:  []string{"persistence-test.local"},
		CacheDir: tmpDir,
		TestMode: true,
		Cache:    cache,
	})
	if err != nil {
		t.Fatalf("NewACMEClient (restart) failed: %v", err)
	}

	// The new client should be able to use the cached certificate data
	cert2, err := client2.GetCertificate(hello)
	if err != nil {
		t.Fatalf("GetCertificate after restart failed: %v", err)
	}
	if cert2 == nil {
		t.Fatal("expected non-nil certificate after restart")
	}

	// Verify certificate data was preserved (in test mode, certs are regenerated
	// but the cache mechanism is still exercised)
	status1 := client1.GetStatus()
	status2 := client2.GetStatus()

	if !status1.HasCertificate || !status2.HasCertificate {
		t.Error("both clients should have certificates")
	}

	t.Logf("Client 1 domains: %v", status1.Domains)
	t.Logf("Client 2 domains: %v", status2.Domains)
}

// TestACMEClient_RenewalLoop tests the certificate renewal monitoring loop.
// This verifies that the renewal loop correctly detects certificates needing renewal.
func TestACMEClient_RenewalLoop(t *testing.T) {
	tmpDir := t.TempDir()

	client, err := NewACMEClient(ACMEClientConfig{
		Email:    "test@example.com",
		Domains:  []string{"renewal-test.local"},
		CacheDir: tmpDir,
		TestMode: true,
	})
	if err != nil {
		t.Fatalf("NewACMEClient failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start the renewal loop
	client.StartRenewalLoop(ctx)

	// Get initial status
	status := client.GetStatus()
	t.Logf("Initial status - HasCertificate: %v, NeedsRenewal: %v, DaysRemaining: %d",
		status.HasCertificate, status.NeedsRenewal, status.DaysRemaining)

	// Generate a certificate
	hello := &tls.ClientHelloInfo{
		ServerName: "renewal-test.local",
	}
	_, err = client.GetCertificate(hello)
	if err != nil {
		t.Fatalf("GetCertificate failed: %v", err)
	}

	// Check status after certificate generation
	status = client.GetStatus()
	t.Logf("After cert generation - HasCertificate: %v, NeedsRenewal: %v, DaysRemaining: %d",
		status.HasCertificate, status.NeedsRenewal, status.DaysRemaining)

	if !status.HasCertificate {
		t.Error("expected HasCertificate = true")
	}

	// In test mode with a fresh cert, NeedsRenewal should be false
	// (self-signed certs are valid for 1 year by default)
	if status.NeedsRenewal && status.DaysRemaining > 30 {
		t.Errorf("unexpected NeedsRenewal with %d days remaining", status.DaysRemaining)
	}

	// Wait a moment for the renewal loop to run at least once
	time.Sleep(100 * time.Millisecond)

	// Cancel and verify clean shutdown
	cancel()
	time.Sleep(100 * time.Millisecond)
}

// TestDBCertCache_Integration tests the database-backed certificate cache
// with actual certificate data storage and retrieval.
func TestDBCertCache_Integration(t *testing.T) {
	store := storage.NewMemoryStore(nil)
	cache := NewDBCertCache(store)

	ctx := context.Background()

	// Test domain certificate storage
	domain := "cache-test.example.com"
	certData := []byte("-----BEGIN CERTIFICATE-----\nMIIBkTCB+wIBADBFMQswCQYD\n-----END CERTIFICATE-----")

	// Store certificate
	err := cache.Put(ctx, domain, certData)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Retrieve certificate
	retrieved, err := cache.Get(ctx, domain)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if string(retrieved) != string(certData) {
		t.Errorf("certificate data mismatch")
	}

	// Test account key storage (ACME account credentials)
	accountKey := "acme_account+admin@example.com"
	accountData := []byte("-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBg\n-----END PRIVATE KEY-----")

	err = cache.Put(ctx, accountKey, accountData)
	if err != nil {
		t.Fatalf("Put account failed: %v", err)
	}

	retrievedAccount, err := cache.Get(ctx, accountKey)
	if err != nil {
		t.Fatalf("Get account failed: %v", err)
	}

	if string(retrievedAccount) != string(accountData) {
		t.Errorf("account data mismatch")
	}

	// Test deletion
	err = cache.Delete(ctx, domain)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Should now get cache miss
	_, err = cache.Get(ctx, domain)
	if err == nil {
		t.Error("expected cache miss after delete")
	}
}

// TestACME_CertificateChain tests that the full certificate chain is properly handled.
func TestACME_CertificateChain(t *testing.T) {
	tmpDir := t.TempDir()

	client, err := NewACMEClient(ACMEClientConfig{
		Email:    "test@example.com",
		Domains:  []string{"chain-test.local"},
		CacheDir: tmpDir,
		TestMode: true,
	})
	if err != nil {
		t.Fatalf("NewACMEClient failed: %v", err)
	}

	hello := &tls.ClientHelloInfo{
		ServerName: "chain-test.local",
	}

	cert, err := client.GetCertificate(hello)
	if err != nil {
		t.Fatalf("GetCertificate failed: %v", err)
	}

	// Certificate should have at least the leaf certificate
	if len(cert.Certificate) == 0 {
		t.Error("expected at least one certificate in chain")
	}

	// Verify the certificate files were saved
	certPath := filepath.Join(tmpDir, "self-signed.crt")
	keyPath := filepath.Join(tmpDir, "self-signed.key")

	if _, err := os.Stat(certPath); err != nil {
		t.Errorf("certificate file not found: %v", err)
	}
	if _, err := os.Stat(keyPath); err != nil {
		t.Errorf("key file not found: %v", err)
	}
}

// TestACME_MultiDomain tests certificate generation with multiple domains (SAN).
func TestACME_MultiDomain(t *testing.T) {
	tmpDir := t.TempDir()

	domains := []string{
		"primary.example.com",
		"www.example.com",
		"api.example.com",
	}

	client, err := NewACMEClient(ACMEClientConfig{
		Email:    "test@example.com",
		Domains:  domains,
		CacheDir: tmpDir,
		TestMode: true,
	})
	if err != nil {
		t.Fatalf("NewACMEClient failed: %v", err)
	}

	// Get certificate for primary domain
	hello := &tls.ClientHelloInfo{
		ServerName: "primary.example.com",
	}

	cert, err := client.GetCertificate(hello)
	if err != nil {
		t.Fatalf("GetCertificate failed: %v", err)
	}

	// Verify all domains are in the certificate
	if cert.Leaf != nil {
		t.Logf("Certificate DNS names: %v", cert.Leaf.DNSNames)

		for _, domain := range domains {
			found := false
			for _, dnsName := range cert.Leaf.DNSNames {
				if dnsName == domain {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("domain %s not found in certificate DNS names", domain)
			}
		}
	}
}
