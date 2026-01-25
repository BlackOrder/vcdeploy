package security

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net"
	"os"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

// mockHostKeyStore is a simple in-memory implementation for testing.
type mockHostKeyStore struct {
	keys map[string]*StoredHostKey
}

func newMockHostKeyStore() *mockHostKeyStore {
	return &mockHostKeyStore{
		keys: make(map[string]*StoredHostKey),
	}
}

func (m *mockHostKeyStore) keyID(hostname string, port int, keyType string) string {
	return hostname + ":" + string(rune(port)) + ":" + keyType
}

func (m *mockHostKeyStore) GetHostKey(ctx context.Context, hostname string, port int, keyType string) (*StoredHostKey, error) {
	key, ok := m.keys[m.keyID(hostname, port, keyType)]
	if !ok {
		return nil, ErrHostKeyUnknown
	}
	return key, nil
}

func (m *mockHostKeyStore) GetHostKeys(ctx context.Context, hostname string, port int) ([]*StoredHostKey, error) {
	var result []*StoredHostKey
	prefix := hostname + ":" + string(rune(port)) + ":"
	for id, key := range m.keys {
		if len(id) >= len(prefix) && id[:len(prefix)] == prefix {
			result = append(result, key)
		}
	}
	return result, nil
}

func (m *mockHostKeyStore) StoreHostKey(ctx context.Context, key *StoredHostKey) error {
	m.keys[m.keyID(key.Hostname, key.Port, key.KeyType)] = key
	return nil
}

func (m *mockHostKeyStore) TrustHostKey(ctx context.Context, hostname string, port int, keyType string, trustedBy string) error {
	id := m.keyID(hostname, port, keyType)
	key, ok := m.keys[id]
	if !ok {
		return ErrHostKeyUnknown
	}
	key.Trusted = true
	key.AddedBy = trustedBy
	return nil
}

func (m *mockHostKeyStore) DeleteHostKey(ctx context.Context, hostname string, port int, keyType string) error {
	id := m.keyID(hostname, port, keyType)
	if _, ok := m.keys[id]; !ok {
		return ErrHostKeyUnknown
	}
	delete(m.keys, id)
	return nil
}

func (m *mockHostKeyStore) ListAllKeys(ctx context.Context) ([]*StoredHostKey, error) {
	result := make([]*StoredHostKey, 0, len(m.keys))
	for _, key := range m.keys {
		result = append(result, key)
	}
	return result, nil
}

func generateTestKey(t *testing.T) (ssh.PublicKey, *rsa.PrivateKey) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}

	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("Failed to create SSH public key: %v", err)
	}

	return publicKey, privateKey
}

func TestHostKeyVerifier_UnknownHost_StrictMode(t *testing.T) {
	store := newMockHostKeyStore()
	verifier := NewHostKeyVerifier(store)
	verifier.StrictMode = true

	pubKey, _ := generateTestKey(t)
	remote := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 22}

	ctx := context.Background()
	err := verifier.VerifyHostKey(ctx, "example.com:22", remote, pubKey)

	if err == nil {
		t.Error("Expected error for unknown host in strict mode")
	}
}

func TestHostKeyVerifier_UnknownHost_StoresKey(t *testing.T) {
	store := newMockHostKeyStore()
	verifier := NewHostKeyVerifier(store)
	verifier.StrictMode = false
	verifier.RequireTrust = false // Don't require trust for this test

	pubKey, _ := generateTestKey(t)
	remote := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 22}

	ctx := context.Background()
	err := verifier.VerifyHostKey(ctx, "example.com:22", remote, pubKey)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Check key was stored
	stored, err := store.GetHostKey(ctx, "example.com", 22, pubKey.Type())
	if err != nil {
		t.Errorf("Key was not stored: %v", err)
	}
	if stored.Trusted {
		t.Error("Key should not be trusted initially")
	}
}

func TestHostKeyVerifier_UnknownHost_RequiresTrust(t *testing.T) {
	store := newMockHostKeyStore()
	verifier := NewHostKeyVerifier(store)
	verifier.StrictMode = false
	verifier.RequireTrust = true

	pubKey, _ := generateTestKey(t)
	remote := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 22}

	ctx := context.Background()
	err := verifier.VerifyHostKey(ctx, "example.com:22", remote, pubKey)

	if err == nil {
		t.Error("Expected error when trust is required")
	}

	// Key should still be stored
	stored, err := store.GetHostKey(ctx, "example.com", 22, pubKey.Type())
	if err != nil {
		t.Errorf("Key was not stored: %v", err)
	}
	if stored == nil {
		t.Error("Key should be stored even when trust error is returned")
	}
}

func TestHostKeyVerifier_TrustedHost_Succeeds(t *testing.T) {
	store := newMockHostKeyStore()
	verifier := NewHostKeyVerifier(store)
	verifier.RequireTrust = true

	pubKey, _ := generateTestKey(t)
	remote := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 22}

	ctx := context.Background()

	// Pre-store and trust the key
	storedKey := &StoredHostKey{
		Hostname:    "example.com",
		Port:        22,
		KeyType:     pubKey.Type(),
		PublicKey:   pubKey.Marshal(),
		Fingerprint: FingerprintSHA256(pubKey),
		Trusted:     true,
		AddedBy:     "admin",
	}
	if err := store.StoreHostKey(ctx, storedKey); err != nil {
		t.Fatalf("Failed to store key: %v", err)
	}

	// Verify should succeed
	err := verifier.VerifyHostKey(ctx, "example.com:22", remote, pubKey)
	if err != nil {
		t.Errorf("Unexpected error for trusted host: %v", err)
	}
}

func TestHostKeyVerifier_KeyMismatch(t *testing.T) {
	store := newMockHostKeyStore()
	verifier := NewHostKeyVerifier(store)

	pubKey1, _ := generateTestKey(t)
	pubKey2, _ := generateTestKey(t) // Different key
	remote := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 22}

	ctx := context.Background()

	// Store the first key
	storedKey := &StoredHostKey{
		Hostname:    "example.com",
		Port:        22,
		KeyType:     pubKey1.Type(),
		PublicKey:   pubKey1.Marshal(),
		Fingerprint: FingerprintSHA256(pubKey1),
		Trusted:     true,
		AddedBy:     "admin",
	}
	if err := store.StoreHostKey(ctx, storedKey); err != nil {
		t.Fatalf("Failed to store key: %v", err)
	}

	// Try to connect with a different key
	err := verifier.VerifyHostKey(ctx, "example.com:22", remote, pubKey2)
	if err == nil {
		t.Error("Expected error for key mismatch")
	}
}

func TestHostKeyVerifier_TrustKey(t *testing.T) {
	store := newMockHostKeyStore()
	verifier := NewHostKeyVerifier(store)
	verifier.RequireTrust = true

	pubKey, _ := generateTestKey(t)
	remote := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 22}

	ctx := context.Background()

	// Store untrusted key
	storedKey := &StoredHostKey{
		Hostname:    "example.com",
		Port:        22,
		KeyType:     pubKey.Type(),
		PublicKey:   pubKey.Marshal(),
		Fingerprint: FingerprintSHA256(pubKey),
		Trusted:     false,
		AddedBy:     "auto",
	}
	if err := store.StoreHostKey(ctx, storedKey); err != nil {
		t.Fatalf("Failed to store key: %v", err)
	}

	// Verify should fail (not trusted)
	err := verifier.VerifyHostKey(ctx, "example.com:22", remote, pubKey)
	if err == nil {
		t.Error("Expected error for untrusted key")
	}

	// Trust the key
	if err := store.TrustHostKey(ctx, "example.com", 22, pubKey.Type(), "admin"); err != nil {
		t.Fatalf("Failed to trust key: %v", err)
	}

	// Now verify should succeed
	err = verifier.VerifyHostKey(ctx, "example.com:22", remote, pubKey)
	if err != nil {
		t.Errorf("Unexpected error after trusting key: %v", err)
	}
}

func TestFingerprintSHA256(t *testing.T) {
	pubKey, _ := generateTestKey(t)
	fp := FingerprintSHA256(pubKey)

	if fp == "" {
		t.Error("Fingerprint should not be empty")
	}
	if len(fp) < 10 {
		t.Error("Fingerprint seems too short")
	}
	if fp[:7] != "SHA256:" {
		t.Errorf("Fingerprint should start with 'SHA256:', got %s", fp)
	}
}

func TestKnownHostsExporter_ExportAll(t *testing.T) {
	store := newMockHostKeyStore()
	exporter := NewKnownHostsExporter(store)

	pubKey1, _ := generateTestKey(t)
	pubKey2, _ := generateTestKey(t)

	ctx := context.Background()

	// Store some keys (must be trusted for export)
	if err := store.StoreHostKey(ctx, &StoredHostKey{
		Hostname:    "host1.example.com",
		Port:        22,
		KeyType:     pubKey1.Type(),
		PublicKey:   pubKey1.Marshal(),
		Fingerprint: FingerprintSHA256(pubKey1),
		Trusted:     true,
	}); err != nil {
		t.Fatalf("Failed to store key 1: %v", err)
	}

	if err := store.StoreHostKey(ctx, &StoredHostKey{
		Hostname:    "host2.example.com",
		Port:        2222,
		KeyType:     pubKey2.Type(),
		PublicKey:   pubKey2.Marshal(),
		Fingerprint: FingerprintSHA256(pubKey2),
		Trusted:     true,
	}); err != nil {
		t.Fatalf("Failed to store key 2: %v", err)
	}

	// Export to file
	tmpDir := t.TempDir()
	exportPath := tmpDir + "/known_hosts"

	if err := exporter.ExportAll(ctx, exportPath); err != nil {
		t.Fatalf("ExportAll() error = %v", err)
	}

	// Read and verify file
	content, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("Failed to read exported file: %v", err)
	}

	// Should contain both hosts
	if !bytes.Contains(content, []byte("host1.example.com")) {
		t.Error("Exported file should contain host1.example.com")
	}
	if !bytes.Contains(content, []byte("host2.example.com")) {
		t.Error("Exported file should contain host2.example.com")
	}
	if !bytes.Contains(content, []byte("[host2.example.com]:2222")) {
		t.Error("Exported file should contain [host2.example.com]:2222 for non-standard port")
	}
}

func TestKnownHostsExporter_ExportHost(t *testing.T) {
	store := newMockHostKeyStore()
	exporter := NewKnownHostsExporter(store)

	pubKey, _ := generateTestKey(t)

	ctx := context.Background()

	// Store a trusted key
	if err := store.StoreHostKey(ctx, &StoredHostKey{
		Hostname:    "test.example.com",
		Port:        22,
		KeyType:     pubKey.Type(),
		PublicKey:   pubKey.Marshal(),
		Fingerprint: FingerprintSHA256(pubKey),
		Trusted:     true,
	}); err != nil {
		t.Fatalf("Failed to store key: %v", err)
	}

	// Export host
	lines, err := exporter.ExportHost(ctx, "test.example.com", 22)
	if err != nil {
		t.Fatalf("ExportHost() error = %v", err)
	}

	if len(lines) != 1 {
		t.Errorf("ExportHost() returned %d lines, want 1", len(lines))
	}

	if len(lines) > 0 && !bytes.Contains([]byte(lines[0]), []byte("test.example.com")) {
		t.Errorf("Exported line should contain hostname")
	}
}

func TestKnownHostsExporter_ExportAllEmpty(t *testing.T) {
	store := newMockHostKeyStore()
	exporter := NewKnownHostsExporter(store)

	ctx := context.Background()

	// Export to file with no keys
	tmpDir := t.TempDir()
	exportPath := tmpDir + "/known_hosts"

	// Should not error even with no keys
	if err := exporter.ExportAll(ctx, exportPath); err != nil {
		t.Fatalf("ExportAll() with no keys should not error: %v", err)
	}

	// File should exist with header but no key lines
	content, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("Failed to read exported file: %v", err)
	}

	// Should have header comments but no actual key lines
	if !bytes.Contains(content, []byte("# Auto-generated")) {
		t.Error("Exported file should contain header")
	}
}

func TestMockHostKeyStore_ListAllKeys(t *testing.T) {
	store := newMockHostKeyStore()

	pubKey1, _ := generateTestKey(t)
	pubKey2, _ := generateTestKey(t)

	ctx := context.Background()

	// Store keys for different hosts
	if err := store.StoreHostKey(ctx, &StoredHostKey{
		Hostname:    "host1.example.com",
		Port:        22,
		KeyType:     pubKey1.Type(),
		PublicKey:   pubKey1.Marshal(),
		Fingerprint: FingerprintSHA256(pubKey1),
	}); err != nil {
		t.Fatalf("Failed to store key 1: %v", err)
	}

	if err := store.StoreHostKey(ctx, &StoredHostKey{
		Hostname:    "host2.example.com",
		Port:        22,
		KeyType:     pubKey2.Type(),
		PublicKey:   pubKey2.Marshal(),
		Fingerprint: FingerprintSHA256(pubKey2),
	}); err != nil {
		t.Fatalf("Failed to store key 2: %v", err)
	}

	// List all keys
	keys, err := store.ListAllKeys(ctx)
	if err != nil {
		t.Fatalf("ListAllKeys() error = %v", err)
	}

	if len(keys) != 2 {
		t.Errorf("ListAllKeys() returned %d keys, want 2", len(keys))
	}
}

func TestHostKeyVerifier_HostKeyCallback(t *testing.T) {
	store := newMockHostKeyStore()
	verifier := NewHostKeyVerifier(store)
	verifier.StrictMode = true // Use strict mode to avoid storing keys

	ctx := context.Background()

	// Get the callback
	callback := verifier.HostKeyCallback(ctx)
	if callback == nil {
		t.Fatal("HostKeyCallback() returned nil")
	}

	// Try to verify an unknown host - should fail in strict mode
	pubKey, _ := generateTestKey(t)
	addr := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 22}
	err := callback("test.example.com:22", addr, pubKey)
	if err == nil {
		t.Error("HostKeyCallback() should fail for unknown host in strict mode")
	}
}

func TestParseKnownHostsLine(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantNil   bool
		wantHost  string
		wantPort  int
		wantError bool
	}{
		{
			name:    "empty line",
			line:    "",
			wantNil: true,
		},
		{
			name:    "comment line",
			line:    "# This is a comment",
			wantNil: true,
		},
		{
			name:      "invalid format",
			line:      "only-one-field",
			wantError: true,
		},
		{
			name:      "invalid base64",
			line:      "hostname ssh-ed25519 not-valid-base64!!!",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := ParseKnownHostsLine(tt.line)

			if tt.wantError {
				if err == nil {
					t.Error("ParseKnownHostsLine() expected error")
				}
				return
			}

			if err != nil {
				t.Errorf("ParseKnownHostsLine() error = %v", err)
				return
			}

			if tt.wantNil {
				if info != nil {
					t.Error("ParseKnownHostsLine() expected nil")
				}
				return
			}

			if info.Hostname != tt.wantHost {
				t.Errorf("ParseKnownHostsLine() hostname = %v, want %v", info.Hostname, tt.wantHost)
			}

			if info.Port != tt.wantPort {
				t.Errorf("ParseKnownHostsLine() port = %v, want %v", info.Port, tt.wantPort)
			}
		})
	}
}

func TestExportToKnownHostsFile(t *testing.T) {
	store := newMockHostKeyStore()
	verifier := NewHostKeyVerifier(store)

	pubKey, _ := generateTestKey(t)
	ctx := context.Background()

	// Store a trusted key
	if err := store.StoreHostKey(ctx, &StoredHostKey{
		Hostname:    "test.example.com",
		Port:        22,
		KeyType:     pubKey.Type(),
		PublicKey:   pubKey.Marshal(),
		Fingerprint: FingerprintSHA256(pubKey),
		Trusted:     true,
	}); err != nil {
		t.Fatalf("StoreHostKey() error = %v", err)
	}

	// Export to file
	tmpDir := t.TempDir()
	filePath := tmpDir + "/known_hosts"

	err := verifier.ExportToKnownHostsFile(ctx, filePath, true)
	if err != nil {
		t.Fatalf("ExportToKnownHostsFile() error = %v", err)
	}

	// Verify file exists and has content
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if len(content) == 0 {
		t.Error("ExportToKnownHostsFile() created empty file")
	}

	// Should contain the hostname
	if !strings.Contains(string(content), "test.example.com") {
		t.Error("ExportToKnownHostsFile() should contain hostname")
	}
}
