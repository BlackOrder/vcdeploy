package security

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net"
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
