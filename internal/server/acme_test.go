package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewACMEClient(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		cfg     ACMEClientConfig
		wantErr bool
	}{
		{
			name: "valid config with test mode",
			cfg: ACMEClientConfig{
				Email:    "admin@example.com",
				Domains:  []string{"example.com", "www.example.com"},
				CacheDir: tmpDir,
				TestMode: true,
			},
			wantErr: false,
		},
		{
			name: "valid config production",
			cfg: ACMEClientConfig{
				Email:    "admin@example.com",
				Domains:  []string{"example.com"},
				CacheDir: tmpDir,
			},
			wantErr: false,
		},
		{
			name: "missing domains",
			cfg: ACMEClientConfig{
				Email:    "admin@example.com",
				CacheDir: tmpDir,
			},
			wantErr: true,
		},
		{
			name: "missing email in production mode",
			cfg: ACMEClientConfig{
				Domains:  []string{"example.com"},
				CacheDir: tmpDir,
			},
			wantErr: true,
		},
		{
			name: "test mode allows missing email",
			cfg: ACMEClientConfig{
				Domains:  []string{"example.com"},
				CacheDir: tmpDir,
				TestMode: true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewACMEClient(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewACMEClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && client == nil {
				t.Error("NewACMEClient() returned nil client")
			}
		})
	}
}

func TestACMEClientDirectoryURLs(t *testing.T) {
	tmpDir := t.TempDir()

	// Test production URL (default)
	client, err := NewACMEClient(ACMEClientConfig{
		Email:    "admin@example.com",
		Domains:  []string{"example.com"},
		CacheDir: tmpDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if client.directoryURL != LetsEncryptProduction {
		t.Errorf("Expected production URL, got %s", client.directoryURL)
	}

	// Test custom URL
	customURL := "https://custom-acme.example.com/directory"
	client, err = NewACMEClient(ACMEClientConfig{
		Email:        "admin@example.com",
		Domains:      []string{"example.com"},
		CacheDir:     tmpDir,
		DirectoryURL: customURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if client.directoryURL != customURL {
		t.Errorf("Expected custom URL, got %s", client.directoryURL)
	}
}

func TestACMEClientGetTLSConfig(t *testing.T) {
	tmpDir := t.TempDir()

	client, err := NewACMEClient(ACMEClientConfig{
		Email:    "admin@example.com",
		Domains:  []string{"example.com"},
		CacheDir: tmpDir,
		TestMode: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	tlsConfig := client.GetTLSConfig()
	if tlsConfig == nil {
		t.Fatal("GetTLSConfig() returned nil")
	}

	if tlsConfig.GetCertificate == nil {
		t.Error("GetCertificate callback not set")
	}

	if tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("Expected MinVersion TLS 1.2, got %d", tlsConfig.MinVersion)
	}
}

func TestACMEClientSelfSignedCertificate(t *testing.T) {
	tmpDir := t.TempDir()

	client, err := NewACMEClient(ACMEClientConfig{
		Email:    "admin@example.com",
		Domains:  []string{"test.example.com"},
		CacheDir: tmpDir,
		TestMode: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	cert, err := client.generateSelfSignedCertificate(ctx)
	if err != nil {
		t.Fatalf("generateSelfSignedCertificate() error: %v", err)
	}

	if cert == nil {
		t.Fatal("Certificate is nil")
	}

	// Verify certificate was saved to cache directory
	certPath := filepath.Join(tmpDir, "self-signed.crt")
	keyPath := filepath.Join(tmpDir, "self-signed.key")

	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		t.Error("Certificate file not created")
	}
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Error("Key file not created")
	}

	// Verify client state was updated
	if client.currentCert == nil {
		t.Error("currentCert not set")
	}
	if client.certExpiry.IsZero() {
		t.Error("certExpiry not set")
	}
}

func TestACMEClientGetCertificate(t *testing.T) {
	tmpDir := t.TempDir()

	client, err := NewACMEClient(ACMEClientConfig{
		Email:    "admin@example.com",
		Domains:  []string{"test.example.com"},
		CacheDir: tmpDir,
		TestMode: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Generate cert directly first
	_, err = client.generateSelfSignedCertificate(ctx)
	if err != nil {
		t.Fatalf("generateSelfSignedCertificate() error: %v", err)
	}

	// Now GetCertificate should return cached cert
	hello := &tls.ClientHelloInfo{
		ServerName: "test.example.com",
	}
	cert, err := client.GetCertificate(hello)
	if err != nil {
		t.Fatalf("GetCertificate() error: %v", err)
	}
	if cert == nil {
		t.Fatal("Certificate is nil")
	}

	// Second call should return same cached cert
	cert2, err := client.GetCertificate(hello)
	if err != nil {
		t.Fatalf("GetCertificate() second call error: %v", err)
	}
	if cert2 != cert {
		t.Error("Expected cached certificate to be returned")
	}
}

func TestACMEClientGetStatus(t *testing.T) {
	tmpDir := t.TempDir()

	client, err := NewACMEClient(ACMEClientConfig{
		Email:    "admin@example.com",
		Domains:  []string{"test.example.com"},
		CacheDir: tmpDir,
		TestMode: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Status before certificate
	status := client.GetStatus()
	if status.HasCertificate {
		t.Error("Expected HasCertificate = false initially")
	}
	if !status.TestMode {
		t.Error("Expected TestMode = true")
	}

	// Generate certificate
	ctx := context.Background()
	_, err = client.generateSelfSignedCertificate(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Status after certificate
	status = client.GetStatus()
	if !status.HasCertificate {
		t.Error("Expected HasCertificate = true after generation")
	}
	if status.DaysRemaining <= 0 {
		t.Errorf("Expected positive DaysRemaining, got %d", status.DaysRemaining)
	}
	if len(status.Domains) == 0 {
		t.Error("Expected domains in status")
	}
}

func TestACMEClientHTTPHandler(t *testing.T) {
	tmpDir := t.TempDir()

	// Test mode - should return fallback
	testClient, err := NewACMEClient(ACMEClientConfig{
		Email:    "admin@example.com",
		Domains:  []string{"example.com"},
		CacheDir: tmpDir,
		TestMode: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	fallback := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	handler := testClient.HTTPHandler(fallback)
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Errorf("Expected fallback handler (418), got %d", rec.Code)
	}

	// Production mode - should return autocert handler
	prodClient, err := NewACMEClient(ACMEClientConfig{
		Email:    "admin@example.com",
		Domains:  []string{"example.com"},
		CacheDir: tmpDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	handler = prodClient.HTTPHandler(fallback)
	if handler == nil {
		t.Error("HTTPHandler returned nil")
	}
}

func TestCertificateStatus(t *testing.T) {
	// Test JSON serialization
	status := CertificateStatus{
		HasCertificate: true,
		Domains:        []string{"example.com", "www.example.com"},
		NotBefore:      time.Now(),
		NotAfter:       time.Now().Add(90 * 24 * time.Hour),
		Issuer:         "Let's Encrypt",
		DaysRemaining:  89,
		NeedsRenewal:   false,
		TestMode:       false,
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("JSON marshal error: %v", err)
	}

	var parsed CertificateStatus
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("JSON unmarshal error: %v", err)
	}

	if parsed.DaysRemaining != status.DaysRemaining {
		t.Errorf("DaysRemaining mismatch: %d != %d", parsed.DaysRemaining, status.DaysRemaining)
	}
	if len(parsed.Domains) != len(status.Domains) {
		t.Errorf("Domains count mismatch: %d != %d", len(parsed.Domains), len(status.Domains))
	}
	if parsed.TestMode != status.TestMode {
		t.Error("TestMode mismatch")
	}
}

func TestACMEConstants(t *testing.T) {
	if LetsEncryptProduction != "https://acme-v02.api.letsencrypt.org/directory" {
		t.Errorf("Unexpected production URL: %s", LetsEncryptProduction)
	}
	if LetsEncryptStaging != "https://acme-staging-v02.api.letsencrypt.org/directory" {
		t.Errorf("Unexpected staging URL: %s", LetsEncryptStaging)
	}
}

func TestRandomSerialNumber(t *testing.T) {
	serial1, err := randomSerialNumber()
	if err != nil {
		t.Fatalf("randomSerialNumber() error: %v", err)
	}
	if serial1 == nil {
		t.Fatal("Serial number is nil")
	}

	serial2, err := randomSerialNumber()
	if err != nil {
		t.Fatalf("randomSerialNumber() error: %v", err)
	}

	// Two random serial numbers should be different
	if serial1.Cmp(serial2) == 0 {
		t.Error("Two serial numbers should be different")
	}
}

func TestPkixName(t *testing.T) {
	name := pkixName("Test Certificate")
	if name.CommonName != "Test Certificate" {
		t.Errorf("Expected CommonName 'Test Certificate', got %s", name.CommonName)
	}
	if len(name.Organization) != 1 || name.Organization[0] != "VCDeploy" {
		t.Errorf("Expected Organization ['VCDeploy'], got %v", name.Organization)
	}
}

func TestACMEClientCacheDirCreation(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "nested", "cache", "dir")

	client, err := NewACMEClient(ACMEClientConfig{
		Email:    "admin@example.com",
		Domains:  []string{"example.com"},
		CacheDir: cacheDir,
		TestMode: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify directory was created
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		t.Error("Cache directory was not created")
	}

	if client.cacheDir != cacheDir {
		t.Errorf("Expected cacheDir %s, got %s", cacheDir, client.cacheDir)
	}
}

func TestACMEClientDefaultCacheDir(t *testing.T) {
	// Create client without specifying cache dir
	client, err := NewACMEClient(ACMEClientConfig{
		Email:    "admin@example.com",
		Domains:  []string{"example.com"},
		TestMode: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should have a default cache dir
	if client.cacheDir == "" {
		t.Error("Expected default cache directory to be set")
	}
}

func TestStartRenewalLoop(t *testing.T) {
	tmpDir := t.TempDir()

	client, err := NewACMEClient(ACMEClientConfig{
		Email:    "admin@example.com",
		Domains:  []string{"example.com"},
		CacheDir: tmpDir,
		TestMode: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start the loop
	client.StartRenewalLoop(ctx)

	// Give it a moment to start
	time.Sleep(10 * time.Millisecond)

	// Cancel to stop the loop
	cancel()

	// Give it a moment to stop
	time.Sleep(10 * time.Millisecond)

	// If we get here without deadlock, the test passes
}

func TestGetSelfSignedCertificateDoubleCheck(t *testing.T) {
	tmpDir := t.TempDir()

	client, err := NewACMEClient(ACMEClientConfig{
		Email:    "admin@example.com",
		Domains:  []string{"test.example.com"},
		CacheDir: tmpDir,
		TestMode: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	hello := &tls.ClientHelloInfo{
		ServerName: "test.example.com",
	}

	// First call generates cert
	cert1, err := client.getSelfSignedCertificate(hello)
	if err != nil {
		t.Fatal(err)
	}

	// Second call should return cached
	cert2, err := client.getSelfSignedCertificate(hello)
	if err != nil {
		t.Fatal(err)
	}

	if cert1 != cert2 {
		t.Error("Expected same cached certificate")
	}
}
