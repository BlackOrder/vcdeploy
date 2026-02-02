package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// --- SSH Host Key API Tests ---

func TestHandleHostKeys_List(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)
	ctx := context.Background()

	// Create some host keys
	key1 := &storage.SSHHostKey{
		Hostname:    "host1.example.com",
		Port:        22,
		KeyType:     "ssh-rsa",
		PublicKey:   "AAAAB3...",
		Fingerprint: "SHA256:...",
		AddedBy:     "test",
		CreatedAt:   time.Now(),
	}
	_ = server.hostKeyService.Create(ctx, key1)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/host-keys", http.NoBody)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleHostKeys(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var result map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify paginated response structure
	if _, ok := result["items"]; !ok {
		t.Error("expected 'items' field in response")
	}
	if _, ok := result["totalCount"]; !ok {
		t.Error("expected 'totalCount' field in response")
	}
	items, ok := result["items"].([]interface{})
	if !ok || len(items) < 1 {
		t.Error("expected at least one host key")
	}
}

func TestHandleHostKeys_Create(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	body := bytes.NewBufferString(`{
		"hostname": "newhost.example.com",
		"port": 22,
		"keyType": "ssh-ed25519",
		"publicKey": "AAAAC3...",
		"fingerprint": "SHA256:abc123",
		"trusted": false
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/host-keys", body)
	req.Header.Set("Content-Type", "application/json")
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleHostKeys(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

func TestHandleHostKeys_CreateInvalidJSON(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	body := bytes.NewBufferString(`{invalid json`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/host-keys", body)
	req.Header.Set("Content-Type", "application/json")
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleHostKeys(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleHostKeys_CreateMissingHostname(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	body := bytes.NewBufferString(`{
		"port": 22,
		"keyType": "ssh-ed25519",
		"publicKey": "AAAAC3..."
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/host-keys", body)
	req.Header.Set("Content-Type", "application/json")
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleHostKeys(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleHostKeys_CreateMissingKeyType(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	body := bytes.NewBufferString(`{
		"hostname": "test.example.com",
		"port": 22,
		"publicKey": "AAAAC3..."
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/host-keys", body)
	req.Header.Set("Content-Type", "application/json")
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleHostKeys(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleHostKeys_CreateMissingPublicKey(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	body := bytes.NewBufferString(`{
		"hostname": "test.example.com",
		"port": 22,
		"keyType": "ssh-ed25519"
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/host-keys", body)
	req.Header.Set("Content-Type", "application/json")
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleHostKeys(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleHostKeys_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/host-keys", http.NoBody)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleHostKeys(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusMethodNotAllowed, rec.Body.String())
	}
}

func TestHandleHostKey_Get(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)
	ctx := context.Background()

	// Create a host key
	key := &storage.SSHHostKey{
		Hostname:    "gettest.example.com",
		Port:        22,
		KeyType:     "ssh-rsa",
		PublicKey:   "AAAAB3...",
		Fingerprint: "SHA256:...",
		Trusted:     true,
		AddedBy:     "test",
		CreatedAt:   time.Now(),
	}
	_ = server.hostKeyService.Create(ctx, key)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/host-keys/1", http.NoBody)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleHostKey(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var keyResponse storage.SSHHostKey
	if err := json.NewDecoder(rec.Body).Decode(&keyResponse); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if keyResponse.Hostname != "gettest.example.com" {
		t.Errorf("hostname = %v, want %v", keyResponse.Hostname, "gettest.example.com")
	}
}

func TestHandleHostKey_GetNotFound(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/host-keys/999", http.NoBody)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleHostKey(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleHostKey_Delete(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)
	ctx := context.Background()

	// Create a host key
	key := &storage.SSHHostKey{
		Hostname:    "deletetest.example.com",
		Port:        22,
		KeyType:     "ssh-rsa",
		PublicKey:   "AAAAB3...",
		Fingerprint: "SHA256:...",
		Trusted:     false,
		AddedBy:     "test",
		CreatedAt:   time.Now(),
	}
	_ = server.hostKeyService.Create(ctx, key)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/host-keys/1", http.NoBody)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleHostKey(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestHandleHostKey_InvalidID(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/host-keys/invalid", http.NoBody)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleHostKey(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleHostKey_UpdateTrust(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)
	ctx := context.Background()

	// Create a host key
	key := &storage.SSHHostKey{
		Hostname:    "trusttest.example.com",
		Port:        22,
		KeyType:     "ssh-rsa",
		PublicKey:   "AAAAB3...",
		Fingerprint: "SHA256:...",
		Trusted:     false,
		AddedBy:     "test",
		CreatedAt:   time.Now(),
	}
	_ = server.hostKeyService.Create(ctx, key)

	body := bytes.NewBufferString(`{"trusted": true}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/host-keys/1", body)
	req.Header.Set("Content-Type", "application/json")
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleHostKey(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// --- SSH Jump Server API Tests ---

func TestHandleJumpServers_List(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)
	ctx := context.Background()

	// Create a jump server
	js := &storage.SSHJumpServer{
		Name:      "bastion-test",
		Host:      "bastion.example.com",
		Port:      22,
		Username:  "jumpuser",
		CreatedAt: time.Now(),
	}
	_ = server.store.CreateJumpServer(ctx, js)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jump-servers", http.NoBody)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleJumpServers(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var result map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify paginated response structure
	if _, ok := result["items"]; !ok {
		t.Error("expected 'items' field in response")
	}
	if _, ok := result["totalCount"]; !ok {
		t.Error("expected 'totalCount' field in response")
	}
	items, ok := result["items"].([]interface{})
	if !ok || len(items) < 1 {
		t.Error("expected at least one jump server")
	}
}

func TestHandleJumpServers_Create(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	body := bytes.NewBufferString(`{
		"name": "new-bastion",
		"host": "newbastion.example.com",
		"port": 22,
		"username": "admin"
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/jump-servers", body)
	req.Header.Set("Content-Type", "application/json")
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleJumpServers(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

func TestHandleJumpServers_CreateMissingName(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	body := bytes.NewBufferString(`{
		"host": "newbastion.example.com",
		"port": 22,
		"username": "admin"
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/jump-servers", body)
	req.Header.Set("Content-Type", "application/json")
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleJumpServers(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleJumpServers_CreateMissingHost(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	body := bytes.NewBufferString(`{
		"name": "new-bastion",
		"port": 22,
		"username": "admin"
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/jump-servers", body)
	req.Header.Set("Content-Type", "application/json")
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleJumpServers(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleJumpServers_CreateMissingUsername(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	body := bytes.NewBufferString(`{
		"name": "new-bastion",
		"host": "newbastion.example.com",
		"port": 22
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/jump-servers", body)
	req.Header.Set("Content-Type", "application/json")
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleJumpServers(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleJumpServers_CreateInvalidJSON(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	body := bytes.NewBufferString(`{invalid json`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/jump-servers", body)
	req.Header.Set("Content-Type", "application/json")
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleJumpServers(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleJumpServers_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/jump-servers", http.NoBody)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleJumpServers(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusMethodNotAllowed, rec.Body.String())
	}
}

func TestHandleJumpServer_Get(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)
	ctx := context.Background()

	// Create a jump server
	js := &storage.SSHJumpServer{
		Name:      "get-test-bastion",
		Host:      "gettest.example.com",
		Port:      22,
		Username:  "testuser",
		CreatedAt: time.Now(),
	}
	_ = server.store.CreateJumpServer(ctx, js)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jump-servers/1", http.NoBody)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleJumpServer(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestHandleJumpServer_Delete(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)
	ctx := context.Background()

	// Create a jump server
	js := &storage.SSHJumpServer{
		Name:      "delete-test-bastion",
		Host:      "deltest.example.com",
		Port:      22,
		Username:  "deluser",
		CreatedAt: time.Now(),
	}
	_ = server.store.CreateJumpServer(ctx, js)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/jump-servers/1", http.NoBody)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleJumpServer(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestHandleJumpServer_GetNotFound(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jump-servers/999", http.NoBody)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleJumpServer(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleJumpServer_InvalidID(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jump-servers/invalid", http.NoBody)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleJumpServer(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleJumpServer_Update(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)
	ctx := context.Background()

	// Create a jump server
	js := &storage.SSHJumpServer{
		Name:      "update-test-bastion",
		Host:      "updatetest.example.com",
		Port:      22,
		Username:  "updateuser",
		CreatedAt: time.Now(),
	}
	_ = server.store.CreateJumpServer(ctx, js)

	body := bytes.NewBufferString(`{
		"name": "updated-bastion",
		"host": "updated.example.com",
		"port": 2222,
		"username": "newuser"
	}`)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/jump-servers/1", body)
	req.Header.Set("Content-Type", "application/json")
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleJumpServer(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// --- Blocked IPs API Tests ---

func TestHandleBlockedIPs_List(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)
	ctx := context.Background()

	// Block an IP
	block := &storage.BlockedIP{
		IPAddress: "192.168.1.1",
		Reason:    "test block",
		BlockedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		BlockedBy: "test",
	}
	_ = server.store.BlockIP(ctx, block)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/blocked", http.NoBody)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleBlockedIPs(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestHandleBlockedIPs_Block(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	body := bytes.NewBufferString(`{
		"ipAddress": "10.0.0.1",
		"reason": "automated test block",
		"duration": "1h"
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/blocked", body)
	req.Header.Set("Content-Type", "application/json")
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleBlockedIPs(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

func TestHandleBlockedIP_Unblock(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)
	ctx := context.Background()

	// Block an IP
	block := &storage.BlockedIP{
		IPAddress: "172.16.0.1",
		Reason:    "test for unblock",
		BlockedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		BlockedBy: "test",
	}
	_ = server.store.BlockIP(ctx, block)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/blocked/172.16.0.1", http.NoBody)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleBlockedIP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestHandleBlockedIP_Get(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)
	ctx := context.Background()

	// Block an IP
	block := &storage.BlockedIP{
		IPAddress: "10.0.0.1",
		Reason:    "test for get",
		BlockedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		BlockedBy: "test",
	}
	_ = server.store.BlockIP(ctx, block)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/blocked/10.0.0.1", http.NoBody)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleBlockedIP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestHandleBlockedIP_GetNotFound(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/blocked/192.168.99.99", http.NoBody)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleBlockedIP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleBlockedIP_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/blocked/10.0.0.1", http.NoBody)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleBlockedIP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

// --- Provision API Tests ---

func TestHandleProvisionJobs_List(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)
	ctx := context.Background()

	// Create a provision job
	job := &storage.ProvisionJob{
		TargetHost: "target.example.com",
		TargetPort: 22,
		TargetUser: "root",
		Status:     "pending",
	}
	_ = server.provisionService.CreateJob(ctx, job)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provision", http.NoBody)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleProvisionJobs(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestHandleProvisionJobs_Create(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	body := bytes.NewBufferString(`{
		"targetHost": "newtarget.example.com",
		"targetPort": 22,
		"targetUser": "admin"
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/provision", body)
	req.Header.Set("Content-Type", "application/json")
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleProvisionJobs(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

func TestHandleProvisionJob_Get(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)
	ctx := context.Background()

	// Create a provision job
	job := &storage.ProvisionJob{
		TargetHost: "gettarget.example.com",
		TargetPort: 22,
		TargetUser: "root",
		Status:     "pending",
	}
	_ = server.provisionService.CreateJob(ctx, job)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provision/"+job.ID, http.NoBody)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleProvisionJob(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestHandleProvisionJob_Cancel(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)
	ctx := context.Background()

	// Create a provision job
	job := &storage.ProvisionJob{
		TargetHost: "canceltarget.example.com",
		TargetPort: 22,
		TargetUser: "root",
		Status:     "pending",
	}
	_ = server.provisionService.CreateJob(ctx, job)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/provision/"+job.ID, http.NoBody)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleProvisionJob(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestHandleProvisionJob_GetNotFound(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provision/nonexistent-job-id", http.NoBody)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleProvisionJob(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleProvisionJob_EmptyID(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provision/", http.NoBody)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleProvisionJob(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleProvisionJob_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/provision/some-id", http.NoBody)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleProvisionJob(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusMethodNotAllowed, rec.Body.String())
	}
}

// --- Authorization Tests for New Endpoints ---

func TestNewEndpoints_RequireAdminAccess(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()

	// Create a viewer user (non-admin)
	viewer := &storage.User{
		Username:     "viewer_test",
		PasswordHash: "hash",
		Email:        "viewer@example.com",
		Role:         "viewer",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	_ = server.store.CreateUser(ctx, viewer)

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/blocked"},
		{http.MethodPost, "/api/v1/blocked"},
		{http.MethodGet, "/api/v1/provision"},
		{http.MethodPost, "/api/v1/provision"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			var body *bytes.Buffer
			if ep.method == http.MethodPost {
				body = bytes.NewBufferString(`{}`)
			}

			var req *http.Request
			if body != nil {
				req = httptest.NewRequest(ep.method, ep.path, body)
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(ep.method, ep.path, http.NoBody)
			}
			req = requestWithAdminContext(req, viewer.ID)
			rec := httptest.NewRecorder()

			switch ep.path {
			case "/api/v1/blocked":
				server.handleBlockedIPs(rec, req)
			case "/api/v1/provision":
				server.handleProvisionJobs(rec, req)
			}

			// Should be forbidden for viewer
			if rec.Code != http.StatusForbidden {
				t.Errorf("expected 403 Forbidden for %s %s, got %d", ep.method, ep.path, rec.Code)
			}
		})
	}
}
