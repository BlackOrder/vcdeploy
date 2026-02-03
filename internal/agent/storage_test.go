package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
)

func TestAgentStoreInit(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	store, err := NewAgentStore(tmpDir)
	if err != nil {
		t.Fatalf("NewAgentStore: %v", err)
	}
	defer store.Close()

	if err := store.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	// Verify database file exists
	dbPath := filepath.Join(tmpDir, "agent.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("database file not created")
	}
}

func TestAgentStoreState(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewAgentStore(tmpDir)
	if err != nil {
		t.Fatalf("NewAgentStore: %v", err)
	}
	defer store.Close()

	if err := store.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	ctx := context.Background()

	// Test SetState/GetState
	key := "test-key"
	value := []byte("test-value-with-sensitive-data")

	if err := store.SetState(ctx, key, value); err != nil {
		t.Fatalf("SetState: %v", err)
	}

	retrieved, err := store.GetState(ctx, key)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}

	if string(retrieved) != string(value) {
		t.Errorf("GetState returned %q, want %q", retrieved, value)
	}

	// Test GetState for non-existent key
	_, err = store.GetState(ctx, "non-existent")
	if err != storage.ErrNotFound {
		t.Errorf("GetState for non-existent key: got %v, want ErrNotFound", err)
	}

	// Test DeleteState
	if err := store.DeleteState(ctx, key); err != nil {
		t.Fatalf("DeleteState: %v", err)
	}

	_, err = store.GetState(ctx, key)
	if err != storage.ErrNotFound {
		t.Errorf("GetState after delete: got %v, want ErrNotFound", err)
	}
}

func TestAgentStoreStateJSON(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewAgentStore(tmpDir)
	if err != nil {
		t.Fatalf("NewAgentStore: %v", err)
	}
	defer store.Close()

	if err := store.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	ctx := context.Background()

	type TestData struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	original := TestData{Name: "test", Count: 42}

	if err := store.SetStateJSON(ctx, "json-test", original); err != nil {
		t.Fatalf("SetStateJSON: %v", err)
	}

	var retrieved TestData
	if err := store.GetStateJSON(ctx, "json-test", &retrieved); err != nil {
		t.Fatalf("GetStateJSON: %v", err)
	}

	if retrieved.Name != original.Name || retrieved.Count != original.Count {
		t.Errorf("GetStateJSON returned %+v, want %+v", retrieved, original)
	}
}

func TestAgentStoreCertificate(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewAgentStore(tmpDir)
	if err != nil {
		t.Fatalf("NewAgentStore: %v", err)
	}
	defer store.Close()

	if err := store.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	ctx := context.Background()

	// Test SaveCertificate/GetCertificate
	certType := "agent"
	cert := []byte("-----BEGIN CERTIFICATE-----\ntest-cert-data\n-----END CERTIFICATE-----")
	key := []byte("-----BEGIN PRIVATE KEY-----\ntest-key-data\n-----END PRIVATE KEY-----")
	notAfter := time.Now().Add(365 * 24 * time.Hour).Truncate(time.Second)

	if err := store.SaveCertificate(ctx, certType, cert, key, notAfter); err != nil {
		t.Fatalf("SaveCertificate: %v", err)
	}

	record, err := store.GetCertificate(ctx, certType)
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}

	if string(record.Certificate) != string(cert) {
		t.Errorf("Certificate mismatch: got %q, want %q", record.Certificate, cert)
	}
	if string(record.PrivateKey) != string(key) {
		t.Errorf("PrivateKey mismatch: got %q, want %q", record.PrivateKey, key)
	}
	if record.Type != certType {
		t.Errorf("Type mismatch: got %q, want %q", record.Type, certType)
	}

	// Test CA cert without private key
	caCert := []byte("-----BEGIN CERTIFICATE-----\nca-cert-data\n-----END CERTIFICATE-----")
	if err := store.SaveCertificate(ctx, "ca", caCert, nil, notAfter); err != nil {
		t.Fatalf("SaveCertificate (CA): %v", err)
	}

	caRecord, err := store.GetCertificate(ctx, "ca")
	if err != nil {
		t.Fatalf("GetCertificate (CA): %v", err)
	}
	if caRecord.PrivateKey != nil {
		t.Errorf("CA PrivateKey should be nil, got %v", caRecord.PrivateKey)
	}

	// Test ListCertificates
	certs, err := store.ListCertificates(ctx)
	if err != nil {
		t.Fatalf("ListCertificates: %v", err)
	}
	if len(certs) != 2 {
		t.Errorf("ListCertificates returned %d certs, want 2", len(certs))
	}

	// Test DeleteCertificate
	if err := store.DeleteCertificate(ctx, certType); err != nil {
		t.Fatalf("DeleteCertificate: %v", err)
	}

	_, err = store.GetCertificate(ctx, certType)
	if err != storage.ErrNotFound {
		t.Errorf("GetCertificate after delete: got %v, want ErrNotFound", err)
	}
}

func TestAgentStoreHMACSecret(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewAgentStore(tmpDir)
	if err != nil {
		t.Fatalf("NewAgentStore: %v", err)
	}
	defer store.Close()

	if err := store.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	ctx := context.Background()

	masterHost := "master.example.com:8443"
	secret := []byte("super-secret-hmac-key")

	if err := store.SaveHMACSecret(ctx, masterHost, secret); err != nil {
		t.Fatalf("SaveHMACSecret: %v", err)
	}

	record, err := store.GetHMACSecret(ctx, masterHost)
	if err != nil {
		t.Fatalf("GetHMACSecret: %v", err)
	}

	if string(record.Secret) != string(secret) {
		t.Errorf("Secret mismatch: got %q, want %q", record.Secret, secret)
	}
	if record.MasterHost != masterHost {
		t.Errorf("MasterHost mismatch: got %q, want %q", record.MasterHost, masterHost)
	}

	// Test GetHMACSecret for non-existent host
	_, err = store.GetHMACSecret(ctx, "unknown.host")
	if err != storage.ErrNotFound {
		t.Errorf("GetHMACSecret for non-existent: got %v, want ErrNotFound", err)
	}

	// Test DeleteHMACSecret
	if err := store.DeleteHMACSecret(ctx, masterHost); err != nil {
		t.Fatalf("DeleteHMACSecret: %v", err)
	}

	_, err = store.GetHMACSecret(ctx, masterHost)
	if err != storage.ErrNotFound {
		t.Errorf("GetHMACSecret after delete: got %v, want ErrNotFound", err)
	}
}

func TestAgentStoreRevokedCerts(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewAgentStore(tmpDir)
	if err != nil {
		t.Fatalf("NewAgentStore: %v", err)
	}
	defer store.Close()

	if err := store.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	ctx := context.Background()

	serial := "ABC123"
	revokedAt := time.Now().Truncate(time.Second)

	if err := store.SaveRevokedCert(ctx, serial, revokedAt); err != nil {
		t.Fatalf("SaveRevokedCert: %v", err)
	}

	// Test IsRevoked
	revoked, err := store.IsRevoked(ctx, serial)
	if err != nil {
		t.Fatalf("IsRevoked: %v", err)
	}
	if !revoked {
		t.Errorf("IsRevoked(%q): got false, want true", serial)
	}

	revoked, err = store.IsRevoked(ctx, "unknown-serial")
	if err != nil {
		t.Fatalf("IsRevoked (unknown): %v", err)
	}
	if revoked {
		t.Errorf("IsRevoked(unknown): got true, want false")
	}

	// Test ListRevokedCerts
	certs, err := store.ListRevokedCerts(ctx)
	if err != nil {
		t.Fatalf("ListRevokedCerts: %v", err)
	}
	if len(certs) != 1 {
		t.Errorf("ListRevokedCerts returned %d certs, want 1", len(certs))
	}

	// Test SyncRevokedCerts
	newCerts := []*RevokedCertRecord{
		{SerialNumber: "NEW1", RevokedAt: time.Now()},
		{SerialNumber: "NEW2", RevokedAt: time.Now()},
	}
	if err := store.SyncRevokedCerts(ctx, newCerts); err != nil {
		t.Fatalf("SyncRevokedCerts: %v", err)
	}

	certs, err = store.ListRevokedCerts(ctx)
	if err != nil {
		t.Fatalf("ListRevokedCerts after sync: %v", err)
	}
	if len(certs) != 2 {
		t.Errorf("ListRevokedCerts after sync returned %d certs, want 2", len(certs))
	}

	// Old serial should no longer be revoked
	revoked, _ = store.IsRevoked(ctx, serial)
	if revoked {
		t.Errorf("Old serial should no longer be revoked after sync")
	}
}

func TestAgentStoreAuditLog(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewAgentStore(tmpDir)
	if err != nil {
		t.Fatalf("NewAgentStore: %v", err)
	}
	defer store.Close()

	if err := store.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	ctx := context.Background()

	// Log some events
	events := []struct {
		eventType string
		details   string
		success   bool
	}{
		{AuditEventConnect, "connected to master", true},
		{AuditEventDeployStart, "deploying project-x", true},
		{AuditEventDeployFailed, "build failed", false},
		{AuditEventCertIssued, "new certificate issued", true},
	}

	for _, ev := range events {
		if err := store.LogAuditEvent(ctx, ev.eventType, ev.details, ev.success); err != nil {
			t.Fatalf("LogAuditEvent(%q): %v", ev.eventType, err)
		}
	}

	// Get all events
	allEvents, err := store.GetAuditEvents(ctx, "", 100)
	if err != nil {
		t.Fatalf("GetAuditEvents: %v", err)
	}
	if len(allEvents) != len(events) {
		t.Errorf("GetAuditEvents returned %d events, want %d", len(allEvents), len(events))
	}

	// Get events by type
	deployEvents, err := store.GetAuditEvents(ctx, AuditEventDeployStart, 100)
	if err != nil {
		t.Fatalf("GetAuditEvents (deploy): %v", err)
	}
	if len(deployEvents) != 1 {
		t.Errorf("GetAuditEvents (deploy) returned %d events, want 1", len(deployEvents))
	}

	// Check that failed event is recorded correctly
	failedEvents, err := store.GetAuditEvents(ctx, AuditEventDeployFailed, 100)
	if err != nil {
		t.Fatalf("GetAuditEvents (failed): %v", err)
	}
	if len(failedEvents) != 1 || failedEvents[0].Success {
		t.Errorf("Failed event not recorded correctly")
	}
}

func TestAgentStoreRepoCache(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewAgentStore(tmpDir)
	if err != nil {
		t.Fatalf("NewAgentStore: %v", err)
	}
	defer store.Close()

	if err := store.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	ctx := context.Background()

	repoURL := "https://github.com/example/repo.git"
	archivePath := "/var/cache/repos/example-repo.tar.gz"
	checksum := "sha256:abc123"

	if err := store.SaveRepoCache(ctx, repoURL, archivePath, checksum); err != nil {
		t.Fatalf("SaveRepoCache: %v", err)
	}

	record, err := store.GetRepoCache(ctx, repoURL)
	if err != nil {
		t.Fatalf("GetRepoCache: %v", err)
	}

	if record.ArchivePath != archivePath {
		t.Errorf("ArchivePath mismatch: got %q, want %q", record.ArchivePath, archivePath)
	}
	if record.Checksum != checksum {
		t.Errorf("Checksum mismatch: got %q, want %q", record.Checksum, checksum)
	}

	// Test TouchRepoCache
	if err := store.TouchRepoCache(ctx, repoURL); err != nil {
		t.Fatalf("TouchRepoCache: %v", err)
	}

	// Test ListRepoCache
	repos, err := store.ListRepoCache(ctx)
	if err != nil {
		t.Fatalf("ListRepoCache: %v", err)
	}
	if len(repos) != 1 {
		t.Errorf("ListRepoCache returned %d repos, want 1", len(repos))
	}

	// Test DeleteRepoCache
	if err := store.DeleteRepoCache(ctx, repoURL); err != nil {
		t.Fatalf("DeleteRepoCache: %v", err)
	}

	_, err = store.GetRepoCache(ctx, repoURL)
	if err != storage.ErrNotFound {
		t.Errorf("GetRepoCache after delete: got %v, want ErrNotFound", err)
	}
}

func TestEncryptionRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewAgentStore(tmpDir)
	if err != nil {
		t.Fatalf("NewAgentStore: %v", err)
	}
	defer store.Close()

	// Test that encryption/decryption works correctly
	testData := []byte("This is sensitive data that should be encrypted!")

	encrypted, err := store.encrypt(testData)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// Encrypted data should be different from original
	if string(encrypted) == string(testData) {
		t.Error("encrypted data should not equal original")
	}

	// Encrypted data should be longer (nonce + ciphertext + tag)
	if len(encrypted) <= len(testData) {
		t.Error("encrypted data should be longer than original")
	}

	decrypted, err := store.decrypt(encrypted)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if string(decrypted) != string(testData) {
		t.Errorf("decrypted data mismatch: got %q, want %q", decrypted, testData)
	}
}

func TestEncryptionNilHandling(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewAgentStore(tmpDir)
	if err != nil {
		t.Fatalf("NewAgentStore: %v", err)
	}
	defer store.Close()

	// Test that nil input returns nil output
	encrypted, err := store.encrypt(nil)
	if err != nil {
		t.Fatalf("encrypt(nil): %v", err)
	}
	if encrypted != nil {
		t.Errorf("encrypt(nil) should return nil, got %v", encrypted)
	}

	decrypted, err := store.decrypt(nil)
	if err != nil {
		t.Fatalf("decrypt(nil): %v", err)
	}
	if decrypted != nil {
		t.Errorf("decrypt(nil) should return nil, got %v", decrypted)
	}
}

func TestAuditLogWithMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewAgentStore(tmpDir)
	if err != nil {
		t.Fatalf("NewAgentStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	// Test logging with metadata
	metadata := map[string]interface{}{
		"project":  "test-project",
		"revision": "abc123",
		"duration": 1.5,
	}
	if err := store.LogAuditEventWithMetadata(ctx, AuditEventDeployComplete, "Deploy succeeded", true, metadata); err != nil {
		t.Fatalf("LogAuditEventWithMetadata: %v", err)
	}

	// Test logging without metadata (should work with nil)
	if err := store.LogAuditEventWithMetadata(ctx, AuditEventConnect, "Connected to master", true, nil); err != nil {
		t.Fatalf("LogAuditEventWithMetadata(nil metadata): %v", err)
	}

	// Retrieve events and verify metadata
	events, err := store.GetAuditEvents(ctx, "", 10)
	if err != nil {
		t.Fatalf("GetAuditEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(events))
	}

	// Events are returned in descending order, so connect is first
	if events[0].EventType != AuditEventConnect {
		t.Errorf("Expected first event type %s, got %s", AuditEventConnect, events[0].EventType)
	}
	if events[1].EventType != AuditEventDeployComplete {
		t.Errorf("Expected second event type %s, got %s", AuditEventDeployComplete, events[1].EventType)
	}

	// Check metadata on deploy event
	if events[1].Metadata == nil {
		t.Error("Expected metadata on deploy event")
	} else {
		if events[1].Metadata["project"] != "test-project" {
			t.Errorf("Expected project=test-project, got %v", events[1].Metadata["project"])
		}
	}
}

func TestQueryAuditLogs(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewAgentStore(tmpDir)
	if err != nil {
		t.Fatalf("NewAgentStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	// Log some events
	_ = store.LogAuditEvent(ctx, AuditEventConnect, "Connected", true)
	_ = store.LogAuditEvent(ctx, AuditEventDeployStart, "Starting deploy", true)
	_ = store.LogAuditEvent(ctx, AuditEventDeployFailed, "Deploy failed", false)
	_ = store.LogAuditEvent(ctx, AuditEventConnect, "Reconnected", true)

	// Test filtering by event type
	filter := AuditLogFilter{EventTypes: []string{AuditEventConnect}}
	events, err := store.QueryAuditLogs(ctx, filter)
	if err != nil {
		t.Fatalf("QueryAuditLogs (by type): %v", err)
	}
	if len(events) != 2 {
		t.Errorf("Expected 2 connect events, got %d", len(events))
	}

	// Test filtering by success
	success := false
	filter = AuditLogFilter{Success: &success}
	events, err = store.QueryAuditLogs(ctx, filter)
	if err != nil {
		t.Fatalf("QueryAuditLogs (by success): %v", err)
	}
	if len(events) != 1 {
		t.Errorf("Expected 1 failed event, got %d", len(events))
	}
	if events[0].EventType != AuditEventDeployFailed {
		t.Errorf("Expected failed event type %s, got %s", AuditEventDeployFailed, events[0].EventType)
	}

	// Test limit and offset
	filter = AuditLogFilter{Limit: 2, Offset: 1}
	events, err = store.QueryAuditLogs(ctx, filter)
	if err != nil {
		t.Fatalf("QueryAuditLogs (limit/offset): %v", err)
	}
	if len(events) != 2 {
		t.Errorf("Expected 2 events with limit/offset, got %d", len(events))
	}
}
