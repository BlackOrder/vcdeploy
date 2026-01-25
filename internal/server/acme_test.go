package server

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupACMETestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}

	// Create required tables
	schema := `
		CREATE TABLE IF NOT EXISTS acme_accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT NOT NULL,
			directory_url TEXT NOT NULL,
			account_url TEXT DEFAULT '',
			private_key BLOB NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_used_at DATETIME,
			UNIQUE(email, directory_url)
		);
		
		CREATE TABLE IF NOT EXISTS acme_certificates (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			domains TEXT NOT NULL,
			cert_pem BLOB NOT NULL,
			private_key_pem BLOB NOT NULL,
			not_before DATETIME NOT NULL,
			not_after DATETIME NOT NULL,
			issuer TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			renewed_at DATETIME
		);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}

	return db
}

func TestNewACMEClient(t *testing.T) {
	db := setupACMETestDB(t)
	defer db.Close()

	tests := []struct {
		name    string
		cfg     ACMEClientConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: ACMEClientConfig{
				DB:      db,
				Email:   "admin@example.com",
				Domains: []string{"example.com"},
			},
			wantErr: false,
		},
		{
			name: "valid config with test mode",
			cfg: ACMEClientConfig{
				DB:       db,
				Email:    "admin@example.com",
				Domains:  []string{"example.com", "www.example.com"},
				TestMode: true,
			},
			wantErr: false,
		},
		{
			name: "missing database",
			cfg: ACMEClientConfig{
				Email:   "admin@example.com",
				Domains: []string{"example.com"},
			},
			wantErr: true,
		},
		{
			name: "missing domains",
			cfg: ACMEClientConfig{
				DB:    db,
				Email: "admin@example.com",
			},
			wantErr: true,
		},
		{
			name: "missing email",
			cfg: ACMEClientConfig{
				DB:      db,
				Domains: []string{"example.com"},
			},
			wantErr: true,
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
	db := setupACMETestDB(t)
	defer db.Close()

	// Test production URL
	client, err := NewACMEClient(ACMEClientConfig{
		DB:      db,
		Email:   "admin@example.com",
		Domains: []string{"example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if client.directoryURL != LetsEncryptProduction {
		t.Errorf("Expected production URL, got %s", client.directoryURL)
	}

	// Test staging URL
	client, err = NewACMEClient(ACMEClientConfig{
		DB:       db,
		Email:    "admin@example.com",
		Domains:  []string{"example.com"},
		TestMode: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if client.directoryURL != LetsEncryptStaging {
		t.Errorf("Expected staging URL, got %s", client.directoryURL)
	}

	// Test custom URL
	customURL := "https://custom-acme.example.com/directory"
	client, err = NewACMEClient(ACMEClientConfig{
		DB:           db,
		Email:        "admin@example.com",
		Domains:      []string{"example.com"},
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
	db := setupACMETestDB(t)
	defer db.Close()

	client, err := NewACMEClient(ACMEClientConfig{
		DB:       db,
		Email:    "admin@example.com",
		Domains:  []string{"example.com"},
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
	db := setupACMETestDB(t)
	defer db.Close()

	client, err := NewACMEClient(ACMEClientConfig{
		DB:       db,
		Email:    "admin@example.com",
		Domains:  []string{"test.example.com"},
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

	// Verify certificate was saved to database
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM acme_certificates").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("Expected 1 certificate in database, got %d", count)
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
	db := setupACMETestDB(t)
	defer db.Close()

	client, err := NewACMEClient(ACMEClientConfig{
		DB:       db,
		Email:    "admin@example.com",
		Domains:  []string{"test.example.com"},
		TestMode: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Generate cert directly first (GetCertificate uses hello.Context() which may be nil)
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
	db := setupACMETestDB(t)
	defer db.Close()

	client, err := NewACMEClient(ACMEClientConfig{
		DB:       db,
		Email:    "admin@example.com",
		Domains:  []string{"test.example.com"},
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

func TestACMEClientAccountCreation(t *testing.T) {
	db := setupACMETestDB(t)
	defer db.Close()

	client, err := NewACMEClient(ACMEClientConfig{
		DB:       db,
		Email:    "admin@example.com",
		Domains:  []string{"example.com"},
		TestMode: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	account, key, err := client.getOrCreateAccount(ctx)
	if err != nil {
		t.Fatalf("getOrCreateAccount() error: %v", err)
	}
	if account == nil {
		t.Fatal("Account is nil")
	}
	if key == nil {
		t.Fatal("Key is nil")
	}
	if account.Email != "admin@example.com" {
		t.Errorf("Expected email admin@example.com, got %s", account.Email)
	}

	// Verify saved to database
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM acme_accounts").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("Expected 1 account in database, got %d", count)
	}

	// Second call should return same account
	account2, _, err := client.getOrCreateAccount(ctx)
	if err != nil {
		t.Fatalf("getOrCreateAccount() second call error: %v", err)
	}
	if account2.ID != account.ID {
		t.Error("Expected same account to be returned")
	}
}

func TestACMEClientLoadCertificateFromDB(t *testing.T) {
	db := setupACMETestDB(t)
	defer db.Close()

	client, err := NewACMEClient(ACMEClientConfig{
		DB:       db,
		Email:    "admin@example.com",
		Domains:  []string{"test.example.com"},
		TestMode: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Generate and save certificate
	_, err = client.generateSelfSignedCertificate(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Clear in-memory state
	client.currentCert = nil
	client.certExpiry = time.Time{}

	// Load from database
	stored, err := client.loadCertificate(ctx)
	if err != nil {
		t.Fatalf("loadCertificate() error: %v", err)
	}
	if stored == nil {
		t.Fatal("Stored certificate is nil")
	}

	// Parse the stored certificate
	cert, err := client.parseCertificate(stored)
	if err != nil {
		t.Fatalf("parseCertificate() error: %v", err)
	}
	if cert == nil {
		t.Fatal("Parsed certificate is nil")
	}
}

func TestHTTPChallengeHandler(t *testing.T) {
	handler := NewHTTPChallengeHandler()

	// Set a challenge
	handler.SetChallenge("test-token", "test-key-auth")

	// Verify challenge is set
	keyAuth, ok := handler.GetChallenge("test-token")
	if !ok {
		t.Error("Expected challenge to be found")
	}
	if keyAuth != "test-key-auth" {
		t.Errorf("Expected keyAuth 'test-key-auth', got %s", keyAuth)
	}

	// Test HTTP handler
	req := httptest.NewRequest("GET", "/.well-known/acme-challenge/test-token", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
	if rec.Body.String() != "test-key-auth" {
		t.Errorf("Expected body 'test-key-auth', got %s", rec.Body.String())
	}

	// Test missing token
	req = httptest.NewRequest("GET", "/.well-known/acme-challenge/missing-token", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", rec.Code)
	}

	// Test wrong method
	req = httptest.NewRequest("POST", "/.well-known/acme-challenge/test-token", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", rec.Code)
	}

	// Clear challenge
	handler.ClearChallenge("test-token")
	_, ok = handler.GetChallenge("test-token")
	if ok {
		t.Error("Expected challenge to be cleared")
	}
}

func TestHTTPChallengeHandlerPathValidation(t *testing.T) {
	handler := NewHTTPChallengeHandler()
	handler.SetChallenge("valid-token", "valid-auth")

	tests := []struct {
		path       string
		wantStatus int
	}{
		{"/.well-known/acme-challenge/valid-token", http.StatusOK},
		{"/.well-known/acme-challenge/", http.StatusNotFound},
		{"/.well-known/acme-challenge", http.StatusNotFound},
		{"/other/path", http.StatusNotFound},
		{"/", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("Path %s: expected status %d, got %d", tt.path, tt.wantStatus, rec.Code)
			}
		})
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
}

func TestACMEConstants(t *testing.T) {
	if LetsEncryptProduction != "https://acme-v02.api.letsencrypt.org/directory" {
		t.Errorf("Unexpected production URL: %s", LetsEncryptProduction)
	}
	if LetsEncryptStaging != "https://acme-staging-v02.api.letsencrypt.org/directory" {
		t.Errorf("Unexpected staging URL: %s", LetsEncryptStaging)
	}
}
