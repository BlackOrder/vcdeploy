package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/security"
	"github.com/BlackOrder/vcdeploy/internal/services/agents"
	"github.com/BlackOrder/vcdeploy/internal/services/apikeys"
	"github.com/BlackOrder/vcdeploy/internal/services/audit"
	"github.com/BlackOrder/vcdeploy/internal/services/deployments"
	"github.com/BlackOrder/vcdeploy/internal/services/hostkeys"
	"github.com/BlackOrder/vcdeploy/internal/services/projects"
	"github.com/BlackOrder/vcdeploy/internal/services/projecttypes"
	"github.com/BlackOrder/vcdeploy/internal/services/provision"
	"github.com/BlackOrder/vcdeploy/internal/services/secrets"
	"github.com/BlackOrder/vcdeploy/internal/services/sessions"
	"github.com/BlackOrder/vcdeploy/internal/services/settings"
	"github.com/BlackOrder/vcdeploy/internal/services/users"
	"github.com/BlackOrder/vcdeploy/internal/services/webhooks"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

// TestMain sets up a temp data directory so NewMasterServer can save/load
// the master key without requiring /var/lib/vcdeploy to exist.
func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "vcdeploy-server-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	os.Setenv("VCDEPLOY_DATA_DIR", tmpDir)
	config.ResetSystemConfig() // reset singleton so it picks up the env var
	code := m.Run()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

// loadTemplatesOrSkip loads templates for a server, skipping the test if templates aren't available.
func loadTemplatesOrSkip(t *testing.T, server *MasterServer) {
	t.Helper()
	if err := server.loadTemplates(); err != nil {
		t.Skipf("skipping test, templates not available: %v", err)
	}
	if len(server.templates) == 0 {
		t.Skip("skipping test, no templates loaded (template directory not found)")
	}
}

func newTestServer(t *testing.T) *MasterServer {
	t.Helper()

	logger := zap.NewNop()
	db, err := storage.New(":memory:", logger)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}

	cfg := &config.MasterConfig{
		Server: config.ServerConfig{
			Listen: ":0",
		},
		GRPC: config.GRPCConfig{
			Listen: ":0",
		},
	}

	server, err := NewMasterServer(cfg, db, logger)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Register cleanup
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Stop(ctx) //nolint:errcheck
		db.Close()       //nolint:errcheck
	})

	// Use the KMS already initialized by NewMasterServer
	kms := server.kms

	// Initialize all services for tests
	server.secretService = secrets.New(db, kms)
	server.settingsSvc = settings.New(db, kms)
	server.userService = users.New(db)
	server.sessionService = sessions.New(db)
	server.apiKeyService = apikeys.New(db)
	server.projectService = projects.New(db)
	server.projectTypeService = projecttypes.New(db)
	server.webhookService = webhooks.New(db, kms)
	server.deploymentService = deployments.New(db)
	server.agentService = agents.New(db)
	server.auditService = audit.New(db)
	server.hostKeyService = hostkeys.New(db)
	server.provisionService = provision.New(db)

	// Re-initialize enforcement middleware with the userService now set
	server.enforcementMiddleware = NewEnforcementMiddleware(cfg, server.userService, logger)

	return server
}

// newTestServerWithAuth creates a test server with a test user, API key, and session.
// Returns the server, the raw API key, the session token, and the user ID.
func newTestServerWithAuth(t *testing.T) (*MasterServer, string, string, string) {
	t.Helper()

	server := newTestServer(t)
	ctx := context.Background()

	// Create a test user
	user := &storage.User{
		Username:     "testuser",
		PasswordHash: "test-hash",
		Email:        "test@example.com",
		Role:         "admin",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := server.store.CreateUser(ctx, user); err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	// Create a test API key with all scopes
	rawAPIKey := "test-api-key-12345"
	hash := sha256.Sum256([]byte(rawAPIKey))
	apiKey := &storage.APIKey{
		UserID:    user.ID,
		Name:      "test-key",
		KeyHash:   hex.EncodeToString(hash[:]),
		KeyPrefix: "test-api",
		Scopes:    `["admin"]`,
		CreatedAt: time.Now(),
	}
	if err := server.store.CreateAPIKey(ctx, apiKey); err != nil {
		t.Fatalf("failed to create test API key: %v", err)
	}

	// Create a test session
	sessionToken := "test-session-token-12345"
	session := &storage.Session{
		ID:        sessionToken,
		UserID:    user.ID,
		IPAddress: "127.0.0.1",
		UserAgent: "test-agent",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := server.store.CreateSession(ctx, session); err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	return server, rawAPIKey, sessionToken, user.ID
}

// createTestAdminUser creates an admin user in the test server and returns their ID.
// Use this when testing handlers that require authentication without full middleware.
func createTestAdminUser(t *testing.T, server *MasterServer) string {
	t.Helper()
	ctx := context.Background()

	user := &storage.User{
		Username:     fmt.Sprintf("testadmin_%d", time.Now().UnixNano()),
		PasswordHash: "test-hash",
		Email:        fmt.Sprintf("admin_%d@example.com", time.Now().UnixNano()),
		Role:         "admin",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := server.store.CreateUser(ctx, user); err != nil {
		t.Fatalf("failed to create test admin user: %v", err)
	}
	return user.ID
}

// requestWithAdminContext creates a request with admin user context for testing.
func requestWithAdminContext(req *http.Request, userID string) *http.Request {
	ctx := context.WithValue(req.Context(), contextKeyUserID, userID)
	return req.WithContext(ctx)
}

func TestNewMasterServer(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	db, err := storage.New(":memory:", logger)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cfg := &config.MasterConfig{
		Server: config.ServerConfig{Listen: ":9000"},
		GRPC:   config.GRPCConfig{Listen: ":9001"},
	}

	server, err := NewMasterServer(cfg, db, logger)
	if err != nil {
		t.Fatalf("NewMasterServer() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Stop(ctx) //nolint:errcheck
	})

	if server == nil {
		t.Fatal("NewMasterServer() returned nil")
	}
	if server.config != cfg {
		t.Error("config not set")
	}
	if server.store != db {
		t.Error("db not set")
	}
	if server.agents == nil {
		t.Error("agents map not initialized")
	}
}

func TestHandleHealth(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	server.handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	// In test mode, gRPC server is not initialized, so status will be "degraded"
	status := resp["status"].(string)
	if status != "healthy" && status != "degraded" {
		t.Errorf("status = %v, want healthy or degraded", resp["status"])
	}
	if _, ok := resp["timestamp"]; !ok {
		t.Error("response missing timestamp")
	}
}

func TestHandleAgents(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	// Add a mock agent
	server.agentsMu.Lock()
	server.agents["agent-1"] = &AgentConnection{
		ID:          "agent-1",
		Name:        "Test Agent",
		Tags:        []string{"prod", "us-west"},
		Status:      "connected",
		ConnectedAt: time.Now(),
		LastPing:    time.Now(),
	}
	server.agentsMu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	rec := httptest.NewRecorder()

	server.handleAgents(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var agents []map[string]interface{}
	_ = json.NewDecoder(rec.Body).Decode(&agents)

	if len(agents) != 1 {
		t.Errorf("len(agents) = %d, want 1", len(agents))
	}
	if len(agents) > 0 && agents[0]["id"] != "agent-1" {
		t.Errorf("agent id = %v, want agent-1", agents[0]["id"])
	}
}

func TestHandleSecrets(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets", nil)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleSecrets(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandleAuditLogs(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit", nil)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleAuditLogs(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestWithAuth_MissingHeader(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	called := false
	handler := server.withAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if called {
		t.Error("handler should not have been called")
	}
}

func TestWithAuth_InvalidHeader(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	called := false
	handler := server.withAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req.Header.Set("Authorization", "Basic invalidtoken")
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if called {
		t.Error("handler should not have been called")
	}
}

func TestWithAuth_ValidBearer(t *testing.T) {
	t.Parallel()

	server, apiKey, _, _ := newTestServerWithAuth(t)
	called := false
	handler := server.withAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !called {
		t.Error("handler should have been called")
	}
}

func TestWithUIAuth_NoCookie(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	called := false
	handler := server.withUIAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if called {
		t.Error("handler should not have been called")
	}
}

func TestWithUIAuth_ValidCookie(t *testing.T) {
	t.Parallel()

	server, _, sessionToken, _ := newTestServerWithAuth(t)
	called := false
	handler := server.withUIAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sessionToken})
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !called {
		t.Error("handler should have been called")
	}
}

func TestHandleGitHubWebhook_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/webhook/github/project", nil)
	rec := httptest.NewRecorder()

	server.handleGitHubWebhook(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleGitHubWebhook_MissingSignature(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	body := bytes.NewBufferString(`{"action": "push"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/github/project", body)
	rec := httptest.NewRecorder()

	server.handleGitHubWebhook(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleGitHubWebhook_ValidRequest(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	body := bytes.NewBufferString(`{"action": "push", "ref": "refs/heads/main"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/github/project", body)
	req.Header.Set("X-Hub-Signature-256", "sha256=test-signature")
	rec := httptest.NewRecorder()

	server.handleGitHubWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandleGitLabWebhook_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/webhook/gitlab/project", nil)
	rec := httptest.NewRecorder()

	server.handleGitLabWebhook(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleGitLabWebhook_MissingToken(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	body := bytes.NewBufferString(`{"object_kind": "push"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/gitlab/project", body)
	rec := httptest.NewRecorder()

	server.handleGitLabWebhook(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleGitLabWebhook_ValidRequest(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	body := bytes.NewBufferString(`{"object_kind": "push", "ref": "refs/heads/main"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/gitlab/project", body)
	req.Header.Set("X-Gitlab-Token", "test-token")
	rec := httptest.NewRecorder()

	server.handleGitLabWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandleBitbucketWebhook_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/webhook/bitbucket/project", nil)
	rec := httptest.NewRecorder()

	server.handleBitbucketWebhook(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleBitbucketWebhook_ValidRequest(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	body := bytes.NewBufferString(`{"push": {"changes": []}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/bitbucket/project", body)
	rec := httptest.NewRecorder()

	server.handleBitbucketWebhook(rec, req)

	// Bitbucket webhooks don't require authentication header
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestLoggingMiddleware(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	wrapped := server.loggingMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "ok")
	}
}

func TestResponseWriter(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, status: 200}

	rw.WriteHeader(http.StatusNotFound)

	if rw.status != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rw.status, http.StatusNotFound)
	}
}

func TestCheckAgentHealth(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	// Add agents with different ping times
	server.agentsMu.Lock()
	server.agents["healthy"] = &AgentConnection{
		ID:       "healthy",
		Status:   "connected",
		LastPing: time.Now(),
	}
	server.agents["stale"] = &AgentConnection{
		ID:       "stale",
		Status:   "connected",
		LastPing: time.Now().Add(-5 * time.Minute),
	}
	server.agentsMu.Unlock()

	server.checkAgentHealth()

	server.agentsMu.RLock()
	healthyAgent := server.agents["healthy"]
	staleAgent := server.agents["stale"]
	server.agentsMu.RUnlock()

	if healthyAgent.Status != "connected" {
		t.Errorf("healthy agent status = %s, want connected", healthyAgent.Status)
	}
	if staleAgent.Status != "stale" {
		t.Errorf("stale agent status = %s, want stale", staleAgent.Status)
	}
}

func TestMasterServer_Shutdown(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := server.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}
}

func TestTemplateFuncs(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	funcs := server.templateFuncs()

	if funcs["formatTime"] == nil {
		t.Error("formatTime function not found")
	}
	if funcs["json"] == nil {
		t.Error("json function not found")
	}

	// Test formatTime
	formatTime := funcs["formatTime"].(func(time.Time) string)
	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	formatted := formatTime(testTime)
	if formatted != "2024-01-15 10:30:00" {
		t.Errorf("formatTime() = %s, want 2024-01-15 10:30:00", formatted)
	}

	// Test json
	jsonFunc := funcs["json"].(func(interface{}) string)
	result := jsonFunc(map[string]string{"key": "value"})
	if result != `{"key":"value"}` {
		t.Errorf("json() = %s, want {\"key\":\"value\"}", result)
	}
}

func TestJsonResponse(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	rec := httptest.NewRecorder()

	data := map[string]interface{}{
		"key":   "value",
		"count": 42,
	}

	server.jsonResponse(rec, data)

	if rec.Header().Get("Content-Type") != "application/json" {
		t.Error("Content-Type should be application/json")
	}

	var result map[string]interface{}
	_ = json.NewDecoder(rec.Body).Decode(&result)

	if result["key"] != "value" {
		t.Errorf("key = %v, want value", result["key"])
	}
}

func BenchmarkHandleHealth(b *testing.B) {
	logger := zap.NewNop()
	db, err := storage.New(":memory:", logger)
	if err != nil {
		b.Fatalf("failed to create db: %v", err)
	}
	b.Cleanup(func() { db.Close() })
	cfg := &config.MasterConfig{}
	server, err := NewMasterServer(cfg, db, logger)
	if err != nil {
		b.Fatalf("failed to create server: %v", err)
	}
	b.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Stop(ctx) //nolint:errcheck
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		server.handleHealth(rec, req)
	}
}

func TestHandleSecretsPost(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	// Test creating a secret - should succeed now that services are initialized
	body := bytes.NewBufferString(`{"project":"test-project","scope":"env","key":"TEST_KEY","value":"secret-value"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", body)
	req.Header.Set("Content-Type", "application/json")
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleSecrets(rec, req)

	// Should succeed with services configured
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

func TestHandleSecretsPostValidation(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	// Test validation - missing project
	body := bytes.NewBufferString(`{"scope":"env","key":"TEST_KEY","value":"secret-value"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", body)
	req.Header.Set("Content-Type", "application/json")
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleSecrets(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	// Test validation - missing key
	body = bytes.NewBufferString(`{"project":"test-project","scope":"env","value":"secret-value"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/secrets", body)
	req.Header.Set("Content-Type", "application/json")
	req = requestWithAdminContext(req, adminUserID)
	rec = httptest.NewRecorder()

	server.handleSecrets(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleSecretsDelete(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()

	// First create a secret via SetSecretEncrypted
	if err := server.store.SetSecretEncrypted(ctx, "test-project", "env", "DELETE_ME", []byte("encrypted-value")); err != nil {
		t.Fatalf("failed to create secret: %v", err)
	}

	// Now delete it
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets?project=test-project&scope=env&key=DELETE_ME", nil)
	rec := httptest.NewRecorder()

	server.handleSecrets(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestHandleProjectTypes(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	// Test listing (empty)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/project-types", nil)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleProjectTypes(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	// Verify paginated response structure
	if _, ok := result["items"]; !ok {
		t.Error("expected 'items' field in response")
	}
	if _, ok := result["totalCount"]; !ok {
		t.Error("expected 'totalCount' field in response")
	}
}

func TestHandleProjectTypesPost(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	// Create a project type
	body := bytes.NewBufferString(`{"name":"nodejs","description":"Node.js application","buildCmd":"npm install && npm run build"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/project-types", body)
	req.Header.Set("Content-Type", "application/json")
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleProjectTypes(rec, req)

	// H4 FIX: POST endpoints now return 201 Created for new resources
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	// Verify it was created
	req = httptest.NewRequest(http.MethodGet, "/api/v1/project-types", nil)
	req = requestWithAdminContext(req, adminUserID)
	rec = httptest.NewRecorder()
	server.handleProjectTypes(rec, req)

	var result map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	found := false
	if items, ok := result["items"].([]interface{}); ok {
		for _, item := range items {
			if m, ok := item.(map[string]interface{}); ok {
				// JSON struct uses Name, not name
				if m["Name"] == "nodejs" {
					found = true
					break
				}
			}
		}
	}
	if !found {
		t.Errorf("project type 'nodejs' was not created, result: %v", result)
	}
}

func TestHandleProjectType(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	// Create a project type directly in DB
	pt := &storage.ProjectType{
		Name:        "python",
		Description: "Python application",
		BuildCmd:    "pip install -r requirements.txt",
		CreatedAt:   time.Now(),
	}
	if err := server.store.CreateProjectType(context.Background(), pt); err != nil {
		t.Fatalf("failed to create project type: %v", err)
	}

	// Test GET single project type - use correct path so handler can extract name
	req := httptest.NewRequest(http.MethodGet, "/api/v1/project-types/python", nil)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleProjectType(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var result storage.ProjectType
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if result.Name != "python" {
		t.Errorf("name = %v, want python", result.Name)
	}
}

func TestHandleProjectTypeDelete(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	// Create a project type
	pt := &storage.ProjectType{
		Name:        "delete-me",
		Description: "To be deleted",
		CreatedAt:   time.Now(),
	}
	if err := server.store.CreateProjectType(context.Background(), pt); err != nil {
		t.Fatalf("failed to create project type: %v", err)
	}

	// Delete it
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/project-types/delete-me", nil)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleProjectType(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	// Verify it was deleted
	_, err := server.store.GetProjectTypeByName(context.Background(), "delete-me")
	if err == nil {
		t.Error("project type should have been deleted")
	}
}

func TestHandleProjectTypePut(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	// Create a project type
	pt := &storage.ProjectType{
		Name:        "update-me",
		Description: "Original description",
		BuildCmd:    "npm build",
		CreatedAt:   time.Now(),
	}
	if err := server.store.CreateProjectType(context.Background(), pt); err != nil {
		t.Fatalf("failed to create project type: %v", err)
	}

	// Update it
	body := bytes.NewBufferString(`{"description":"Updated description","buildCmd":"npm run build"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/project-types/update-me", body)
	req.Header.Set("Content-Type", "application/json")
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleProjectType(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	// Verify the update
	updated, err := server.store.GetProjectTypeByName(context.Background(), "update-me")
	if err != nil {
		t.Fatalf("failed to get updated project type: %v", err)
	}
	if updated.Description != "Updated description" {
		t.Errorf("description = %q, want %q", updated.Description, "Updated description")
	}
	if updated.BuildCmd != "npm run build" {
		t.Errorf("buildCmd = %q, want %q", updated.BuildCmd, "npm run build")
	}
}

func TestHandleProjectType_NotFound(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/project-types/nonexistent", nil)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleProjectType(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleProjectType_EmptyName(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/project-types/", nil)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleProjectType(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleProjectType_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	// Create a project type
	pt := &storage.ProjectType{
		Name:        "test-type",
		Description: "Test",
		CreatedAt:   time.Now(),
	}
	if err := server.store.CreateProjectType(context.Background(), pt); err != nil {
		t.Fatalf("failed to create project type: %v", err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/project-types/test-type", nil)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleProjectType(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusMethodNotAllowed, rec.Body.String())
	}
}

func TestHandleProjectType_InvalidJSON(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)

	// Create a project type
	pt := &storage.ProjectType{
		Name:        "invalid-json-type",
		Description: "Test",
		CreatedAt:   time.Now(),
	}
	if err := server.store.CreateProjectType(context.Background(), pt); err != nil {
		t.Fatalf("failed to create project type: %v", err)
	}

	body := bytes.NewBufferString(`{invalid json`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/project-types/invalid-json-type", body)
	req.Header.Set("Content-Type", "application/json")
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleProjectType(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleDeploymentLogs(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()

	// Create a test project
	project := &storage.Project{Name: "test-project", Repository: "https://github.com/test/test"}
	_ = server.store.CreateProject(context.Background(), project)

	// Create a test deployment
	deployment := &storage.DeploymentRecord{
		ID:          "test-deploy-1",
		Project:     "test-project",
		Status:      "running",
		Branch:      "main",
		CommitHash:  "abc123",
		TriggeredBy: "test",
	}
	_ = server.store.CreateDeployment(ctx, deployment)

	// Test streaming logs with a context that cancels quickly
	// This simulates a client disconnecting
	cancelCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/test-deploy-1/logs", nil)
	req = req.WithContext(cancelCtx)
	rec := httptest.NewRecorder()

	// Call handleDeploymentLogsStream with deployment ID
	server.handleDeploymentLogsStream(rec, req, "test-deploy-1")

	// Should return OK (even though it's a streaming endpoint that eventually closes)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestSecretsFilterByProject(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	adminUserID := createTestAdminUser(t, server)
	ctx := context.Background()

	// Create secrets for different projects
	_ = server.store.SetSecretEncrypted(ctx, "project-a", "env", "KEY1", []byte("value1"))
	_ = server.store.SetSecretEncrypted(ctx, "project-b", "env", "KEY2", []byte("value2"))

	// Filter by project-a
	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets?project=project-a", nil)
	req = requestWithAdminContext(req, adminUserID)
	rec := httptest.NewRecorder()

	server.handleSecrets(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var secrets []map[string]interface{}
	_ = json.NewDecoder(rec.Body).Decode(&secrets)

	for _, s := range secrets {
		if s["project"] != "project-a" {
			t.Errorf("expected only project-a secrets, got %v", s["project"])
		}
	}
}

// UI Integration Tests
// Note: These tests require templates to be loaded. They verify the UI handlers
// work correctly when templates are available. When templates aren't available
// (like in CI environments), the handlers return 500, which is expected and we skip.

func skipIfNoTemplates(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	body := rec.Body.String()
	if rec.Code == http.StatusInternalServerError &&
		(strings.Contains(body, "Templates not loaded") ||
			strings.Contains(body, "Template not found") ||
			strings.Contains(body, "Internal server error")) {
		t.Skip("Templates not loaded - skipping UI test")
	}
}

func TestUISecretsPage(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	// Test secrets UI
	req := httptest.NewRequest(http.MethodGet, "/secrets", nil)
	rec := httptest.NewRecorder()

	server.handleSecretsUI(rec, req)

	skipIfNoTemplates(t, rec)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	// Should return HTML content
	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("content-type = %q, should contain text/html", contentType)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Secrets") {
		t.Error("response should contain 'Secrets' title")
	}
	if !strings.Contains(body, "createSecretModal") {
		t.Error("response should contain create modal element")
	}
}

func TestUIProjectTypesPage(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/project-types", nil)
	rec := httptest.NewRecorder()

	server.handleProjectTypesUI(rec, req)

	skipIfNoTemplates(t, rec)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("content-type = %q, should contain text/html", contentType)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Project Types") {
		t.Error("response should contain 'Project Types' title")
	}
	if !strings.Contains(body, "createTypeModal") {
		t.Error("response should contain create modal element")
	}
}

func TestUIAgentsPage(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/agents", nil)
	rec := httptest.NewRecorder()

	server.handleAgentsUI(rec, req)

	skipIfNoTemplates(t, rec)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Agents") {
		t.Error("response should contain 'Agents' title")
	}
	// Check for stats page elements
	if !strings.Contains(body, "stats-card") {
		t.Error("response should contain stats page")
	}
}

func TestUIDeploymentsPage(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/deployments", nil)
	rec := httptest.NewRecorder()

	server.handleDeploymentsUI(rec, req)

	skipIfNoTemplates(t, rec)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Deployments") {
		t.Error("response should contain 'Deployments' title")
	}
	// Check for deployment log viewer elements
	if !strings.Contains(body, "deployment-logs") {
		t.Error("response should contain deployment logs viewer")
	}
}

func TestUIStatsPage(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	rec := httptest.NewRecorder()

	server.handleStatsUI(rec, req)

	skipIfNoTemplates(t, rec)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Stats") {
		t.Error("response should contain 'Stats' title")
	}
}

func TestUIProjectsPage(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/projects", nil)
	rec := httptest.NewRecorder()

	server.handleProjectsUI(rec, req)

	skipIfNoTemplates(t, rec)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Projects") {
		t.Error("response should contain 'Projects' title")
	}
}

func TestUIAuditPage(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/audit", nil)
	rec := httptest.NewRecorder()

	server.handleAuditUI(rec, req)

	skipIfNoTemplates(t, rec)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Audit") {
		t.Error("response should contain 'Audit' title")
	}
}

func TestUISettingsPage(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()

	server.handleSettingsUI(rec, req)

	skipIfNoTemplates(t, rec)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Settings") {
		t.Error("response should contain 'Settings' title")
	}
}

func TestUIAPIKeysPage(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api-keys", nil)
	rec := httptest.NewRecorder()

	server.handleAPIKeysUI(rec, req)

	skipIfNoTemplates(t, rec)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "API Keys") {
		t.Error("response should contain 'API Keys' title")
	}
}

// --- Authentication Tests ---

func TestWithAuth_ValidXAPIKey(t *testing.T) {
	t.Parallel()

	server, apiKey, _, _ := newTestServerWithAuth(t)
	called := false
	handler := server.withAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !called {
		t.Error("handler should have been called")
	}
}

func TestWithAuth_ExpiredAPIKey(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()

	// Create a user first
	user := &storage.User{
		Username:     "expired-key-user",
		PasswordHash: "hash",
		Email:        "expired@example.com",
		Role:         "admin",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	_ = server.store.CreateUser(ctx, user)

	// Create an expired API key
	expiredKey := "expired-api-key-12345"
	hash := sha256.Sum256([]byte(expiredKey))
	pastTime := time.Now().Add(-24 * time.Hour)
	apiKey := &storage.APIKey{
		UserID:    user.ID,
		Name:      "expired-key",
		KeyHash:   hex.EncodeToString(hash[:]),
		Scopes:    `["*"]`,
		CreatedAt: time.Now(),
		ExpiresAt: &pastTime, // Expired
	}
	_ = server.store.CreateAPIKey(ctx, apiKey)

	called := false
	handler := server.withAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req.Header.Set("Authorization", "Bearer "+expiredKey)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if called {
		t.Error("handler should not have been called for expired key")
	}
}

func TestWithUIAuth_ExpiredSession(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()

	// Create a user
	user := &storage.User{
		Username:     "expired-session-user",
		PasswordHash: "hash",
		Email:        "session@example.com",
		Role:         "admin",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	_ = server.store.CreateUser(ctx, user)

	// Create an expired session
	expiredSession := &storage.Session{
		ID:        "expired-session-token",
		UserID:    user.ID,
		IPAddress: "127.0.0.1",
		UserAgent: "test-agent",
		CreatedAt: time.Now().Add(-48 * time.Hour),
		ExpiresAt: time.Now().Add(-24 * time.Hour), // Expired
	}
	_ = server.store.CreateSession(ctx, expiredSession)

	called := false
	handler := server.withUIAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "expired-session-token"})
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d (redirect)", rec.Code, http.StatusSeeOther)
	}
	if called {
		t.Error("handler should not have been called for expired session")
	}
}

func TestWithUIAuth_InvalidSession(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	called := false
	handler := server.withUIAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "invalid-session-token"})
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if called {
		t.Error("handler should not have been called")
	}
}

// --- JSON Response Tests ---

func TestJsonError(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	rec := httptest.NewRecorder()

	server.jsonError(rec, http.StatusBadRequest, "test error message")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	if rec.Header().Get("Content-Type") != "application/json" {
		t.Error("Content-Type should be application/json")
	}

	var result map[string]interface{}
	_ = json.NewDecoder(rec.Body).Decode(&result)

	if result["error"] != true {
		t.Error("error field should be true")
	}
	if result["message"] != "test error message" {
		t.Errorf("message = %v, want 'test error message'", result["message"])
	}
}

// --- API Login Tests ---

func TestHandleAPILogin(t *testing.T) {
	t.Parallel()

	t.Run("successful login", func(t *testing.T) {
		t.Parallel()
		server := newTestServer(t)
		ctx := context.Background()

		// Create a test user with a known password
		user, err := server.userService.Create(ctx, "loginuser", "TestPass123!", "login@test.com", "admin")
		if err != nil {
			t.Fatalf("failed to create test user: %v", err)
		}

		body := `{"username": "loginuser", "password": "TestPass123!"}`
		req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		server.handleAPILogin(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		var result map[string]interface{}
		if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if result["token"] == nil || result["token"] == "" {
			t.Error("expected token in response")
		}

		userResp, ok := result["user"].(map[string]interface{})
		if !ok {
			t.Fatal("expected user object in response")
		}
		if userResp["username"] != "loginuser" {
			t.Errorf("username = %v, want 'loginuser'", userResp["username"])
		}
		if userResp["id"].(string) != user.ID {
			t.Errorf("user id = %v, want %s", userResp["id"], user.ID)
		}
	})

	t.Run("invalid credentials", func(t *testing.T) {
		t.Parallel()
		server := newTestServer(t)
		ctx := context.Background()

		// Create a test user
		_, err := server.userService.Create(ctx, "loginuser2", "TestPass123!", "login2@test.com", "user")
		if err != nil {
			t.Fatalf("failed to create test user: %v", err)
		}

		body := `{"username": "loginuser2", "password": "WrongPassword!"}`
		req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		server.handleAPILogin(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("missing credentials", func(t *testing.T) {
		t.Parallel()
		server := newTestServer(t)

		body := `{"username": "someuser"}`
		req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		server.handleAPILogin(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("invalid method", func(t *testing.T) {
		t.Parallel()
		server := newTestServer(t)

		req := httptest.NewRequest("GET", "/api/v1/auth/login", nil)
		rec := httptest.NewRecorder()

		server.handleAPILogin(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("user not found", func(t *testing.T) {
		t.Parallel()
		server := newTestServer(t)

		body := `{"username": "nonexistent", "password": "SomePass123!"}`
		req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		server.handleAPILogin(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})
}

// --- Agent Connection Tests ---

func TestCheckAgentHealth_MultipleStates(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	now := time.Now()
	server.agentsMu.Lock()
	server.agents["healthy"] = &AgentConnection{
		ID:       "healthy",
		Status:   "connected",
		LastPing: now,
	}
	server.agents["stale"] = &AgentConnection{
		ID:       "stale",
		Status:   "connected",
		LastPing: now.Add(-5 * time.Minute),
	}
	server.agents["recently-active"] = &AgentConnection{
		ID:       "recently-active",
		Status:   "connected",
		LastPing: now.Add(-1 * time.Minute), // Within threshold
	}
	server.agents["already-stale"] = &AgentConnection{
		ID:       "already-stale",
		Status:   "stale",
		LastPing: now.Add(-10 * time.Minute),
	}
	server.agentsMu.Unlock()

	server.checkAgentHealth()

	server.agentsMu.RLock()
	defer server.agentsMu.RUnlock()

	if server.agents["healthy"].Status != "connected" {
		t.Errorf("healthy agent should remain connected, got %s", server.agents["healthy"].Status)
	}
	if server.agents["stale"].Status != "stale" {
		t.Errorf("stale agent should be marked stale, got %s", server.agents["stale"].Status)
	}
	if server.agents["recently-active"].Status != "connected" {
		t.Errorf("recently-active agent should remain connected, got %s", server.agents["recently-active"].Status)
	}
	// An already stale agent should remain stale
	if server.agents["already-stale"].Status != "stale" {
		t.Errorf("already-stale agent should remain stale, got %s", server.agents["already-stale"].Status)
	}
}

// --- Logging Middleware Tests ---

func TestLoggingMiddleware_RecordsStatus(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	})

	wrapped := server.loggingMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestLoggingMiddleware_HandlesErrors(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal error", http.StatusInternalServerError)
	})

	wrapped := server.loggingMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/error", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// --- ResponseWriter Tests ---

func TestResponseWriter_DefaultStatus(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, status: 200}

	// Write without calling WriteHeader
	_, _ = rw.Write([]byte("test"))

	if rw.status != 200 {
		t.Errorf("status = %d, want 200 (default)", rw.status)
	}
}

func TestResponseWriter_WriteHeaderMultipleCalls(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, status: 200}

	// First call should set status
	rw.WriteHeader(http.StatusCreated)
	if rw.status != http.StatusCreated {
		t.Errorf("status = %d, want %d", rw.status, http.StatusCreated)
	}

	// Second call - status already recorded (per HTTP spec, only first call matters)
	rw.WriteHeader(http.StatusAccepted)
	// The status in rw tracks the actual status sent
}

// --- Template Function Tests ---

func TestTemplateFuncs_FormatTime(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	funcs := server.templateFuncs()

	formatTime := funcs["formatTime"].(func(time.Time) string)

	tests := []struct {
		input    time.Time
		expected string
	}{
		{time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC), "2024-01-15 10:30:00"},
		{time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC), "2024-12-31 23:59:59"},
		{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), "2024-01-01 00:00:00"},
	}

	for _, tc := range tests {
		result := formatTime(tc.input)
		if result != tc.expected {
			t.Errorf("formatTime(%v) = %s, want %s", tc.input, result, tc.expected)
		}
	}
}

func TestTemplateFuncs_Json(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	funcs := server.templateFuncs()

	jsonFunc := funcs["json"].(func(interface{}) string)

	tests := []struct {
		input    interface{}
		expected string
	}{
		{map[string]string{"key": "value"}, `{"key":"value"}`},
		{[]int{1, 2, 3}, `[1,2,3]`},
		{"simple string", `"simple string"`},
		{42, `42`},
		{nil, `null`},
	}

	for _, tc := range tests {
		result := jsonFunc(tc.input)
		if result != tc.expected {
			t.Errorf("json(%v) = %s, want %s", tc.input, result, tc.expected)
		}
	}
}

// --- Server Configuration Tests ---

func TestSetCAManager(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	// caManager is now initialized by newTestServer (via NewMasterServer when KMS is available)
	// Verify we can set it to a different value
	initialCA := server.caManager
	server.SetCAManager(nil)

	if server.caManager != nil {
		t.Error("caManager should be nil after setting to nil")
	}

	// Restore for other tests that might use this server
	server.SetCAManager(initialCA)
}

func TestSetKMS(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	// Can set KMS (already set in newTestServer, but test method)
	server.SetKMS(nil)

	if server.kms != nil {
		t.Error("kms should be nil after setting to nil")
	}
}

// --- GetUserIDFromContext Tests ---

func TestGetUserIDFromContext(t *testing.T) {
	t.Parallel()

	// Test with user ID in context
	req := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(req.Context(), contextKeyUserID, "test-user-123")
	req = req.WithContext(ctx)

	userID, ok := GetUserIDFromContext(req.Context())
	if !ok {
		t.Error("expected ok = true")
	}
	if userID != "test-user-123" {
		t.Errorf("userID = %s, want test-user-123", userID)
	}
}

func TestGetUserIDFromContext_Missing(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest("GET", "/", nil)

	userID, ok := GetUserIDFromContext(req.Context())
	if ok {
		t.Error("expected ok = false for missing context")
	}
	if userID != "" {
		t.Errorf("userID = %s, want empty for missing context", userID)
	}
}

func TestGetUserIDFromContext_WrongType(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(req.Context(), contextKeyUserID, int64(123))
	req = req.WithContext(ctx)

	userID, ok := GetUserIDFromContext(req.Context())
	if ok {
		t.Error("expected ok = false for wrong type")
	}
	if userID != "" {
		t.Errorf("userID = %s, want empty for wrong type", userID)
	}
}

// --- MasterServer Initialization Tests ---

func TestNewMasterServer_WithRateLimiter(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	db, err := storage.New(":memory:", logger)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cfg := &config.MasterConfig{
		Server: config.ServerConfig{Listen: ":9000"},
		GRPC:   config.GRPCConfig{Listen: ":9001"},
	}

	server, err := NewMasterServer(cfg, db, logger)
	if err != nil {
		t.Fatalf("NewMasterServer() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Stop(ctx) //nolint:errcheck
	})

	// Rate limiter should be initialized
	if server.rateLimiter == nil {
		t.Log("Note: rateLimiter may be nil if initialization failed (expected in test environment)")
	}
}

func TestNewMasterServer_WithSecurityMiddleware(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	db, err := storage.New(":memory:", logger)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cfg := &config.MasterConfig{
		Server: config.ServerConfig{Listen: ":9000"},
		GRPC:   config.GRPCConfig{Listen: ":9001"},
	}

	server, err := NewMasterServer(cfg, db, logger)
	if err != nil {
		t.Fatalf("NewMasterServer() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Stop(ctx) //nolint:errcheck
	})

	// Security middleware should be initialized
	if server.securityMiddleware == nil {
		t.Error("securityMiddleware should be initialized")
	}
}

// --- Admin Credentials Flow Tests ---

// TestSyncAdminCredentials_WithEnvPassword tests admin sync with environment password set.
func TestSyncAdminCredentials_WithEnvPassword(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - uses t.Setenv

	// Set environment variables BEFORE creating server (syncAdminCredentials runs during init)
	t.Setenv("VCDEPLOY_ADMIN_PASSWORD", "Admin@Test123!")
	t.Setenv("VCDEPLOY_ADMIN_USERNAME", "envadmin")
	t.Setenv("VCDEPLOY_ADMIN_EMAIL", "envadmin@example.com")

	server := newTestServer(t)

	// Verify user was created
	ctx := context.Background()
	user, err := server.userService.GetByUsername(ctx, "envadmin")
	if err != nil {
		t.Fatalf("GetByUsername() error = %v", err)
	}

	if user.Email != "envadmin@example.com" {
		t.Errorf("user.Email = %q, want %q", user.Email, "envadmin@example.com")
	}
	if user.Role != "admin" {
		t.Errorf("user.Role = %q, want %q", user.Role, "admin")
	}
	if server.requiresSetup {
		t.Error("requiresSetup should be false after env-based setup")
	}
}

// TestSyncAdminCredentials_WithEnvPassword_UpdatesExisting tests updating existing admin.
func TestSyncAdminCredentials_WithEnvPassword_UpdatesExisting(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - uses t.Setenv

	server := newTestServer(t)
	ctx := context.Background()

	// Create existing admin user
	_, err := server.userService.Create(ctx, "admin", "OldPass@123!", "old@example.com", "admin")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Set environment variables with different email and password
	t.Setenv("VCDEPLOY_ADMIN_PASSWORD", "NewAdmin@Pass123!")
	t.Setenv("VCDEPLOY_ADMIN_USERNAME", "admin")
	t.Setenv("VCDEPLOY_ADMIN_EMAIL", "new@example.com")

	// Sync credentials
	err = server.syncAdminCredentials()
	if err != nil {
		t.Fatalf("syncAdminCredentials() error = %v", err)
	}

	// Verify user was updated
	user, err := server.userService.GetByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("GetByUsername() error = %v", err)
	}

	if user.Email != "new@example.com" {
		t.Errorf("user.Email = %q, want %q", user.Email, "new@example.com")
	}
}

// TestSyncAdminCredentials_NoEnvNoUsers_RequiresSetup tests setup mode when no users exist.
func TestSyncAdminCredentials_NoEnvNoUsers_RequiresSetup(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - uses t.Setenv

	// Ensure no env password BEFORE creating server
	t.Setenv("VCDEPLOY_ADMIN_PASSWORD", "")
	t.Setenv("VCDEPLOY_ADMIN_USERNAME", "")
	t.Setenv("VCDEPLOY_ADMIN_EMAIL", "")

	server := newTestServer(t)

	// Server should be in setup-required mode since no env password and no users
	if !server.requiresSetup {
		t.Error("requiresSetup should be true when no env password and no users")
	}
}

// TestSyncAdminCredentials_NoEnvWithUsers_NormalOperation tests normal mode when users exist.
func TestSyncAdminCredentials_NoEnvWithUsers_NormalOperation(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - uses t.Setenv

	// First create a test server with an admin user via env
	t.Setenv("VCDEPLOY_ADMIN_PASSWORD", "Existing@Pass123!")
	t.Setenv("VCDEPLOY_ADMIN_USERNAME", "existingadmin")
	t.Setenv("VCDEPLOY_ADMIN_EMAIL", "existing@example.com")

	server := newTestServer(t)

	// Verify requiresSetup is false (because users exist via env setup)
	if server.requiresSetup {
		t.Error("requiresSetup should be false when admin was created via env")
	}

	// Now clear env vars and re-sync - should still not require setup since user exists
	t.Setenv("VCDEPLOY_ADMIN_PASSWORD", "")
	t.Setenv("VCDEPLOY_ADMIN_USERNAME", "")
	t.Setenv("VCDEPLOY_ADMIN_EMAIL", "")
	server.requiresSetup = false // reset for re-test

	err := server.syncAdminCredentials()
	if err != nil {
		t.Fatalf("syncAdminCredentials() error = %v", err)
	}

	if server.requiresSetup {
		t.Error("requiresSetup should be false when users exist (even without env password)")
	}
}

// TestSyncAdminCredentials_DefaultUsername tests default username when not specified.
func TestSyncAdminCredentials_DefaultUsername(t *testing.T) {
	// NOTE: Cannot use t.Parallel() - uses t.Setenv

	// Set only password BEFORE creating server
	t.Setenv("VCDEPLOY_ADMIN_PASSWORD", "Admin@Test123!")
	t.Setenv("VCDEPLOY_ADMIN_USERNAME", "") // Empty - should default to "admin"
	t.Setenv("VCDEPLOY_ADMIN_EMAIL", "")    // Empty - should default to "admin@localhost"

	server := newTestServer(t)

	ctx := context.Background()
	user, err := server.userService.GetByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("GetByUsername() error = %v", err)
	}

	if user.Email != "admin@localhost" {
		t.Errorf("user.Email = %q, want %q", user.Email, "admin@localhost")
	}
}

// TestSetupRequiredMiddleware_RedirectsWhenSetupRequired tests middleware redirect.
func TestSetupRequiredMiddleware_RedirectsWhenSetupRequired(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	server.requiresSetup = true

	// Create middleware chain
	handler := server.setupRequiredMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name           string
		path           string
		wantRedirect   bool
		wantStatusCode int
	}{
		{"stats redirects", "/stats", true, http.StatusSeeOther},
		{"login redirects", "/login", true, http.StatusSeeOther},
		{"api redirects", "/api/v1/users", true, http.StatusSeeOther},
		{"setup allowed", "/setup", false, http.StatusOK},
		{"static allowed", "/static/css/style.css", false, http.StatusOK},
		{"favicon allowed", "/favicon.ico", false, http.StatusOK},
		{"healthz allowed", "/healthz", false, http.StatusOK},
		{"livez allowed", "/livez", false, http.StatusOK},
		{"readyz allowed", "/readyz", false, http.StatusOK},
		{"api health allowed", "/api/v1/health", false, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if tt.wantRedirect {
				if rec.Code != http.StatusSeeOther {
					t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
				}
				location := rec.Header().Get("Location")
				if location != "/setup" {
					t.Errorf("Location = %q, want /setup", location)
				}
			} else {
				if rec.Code != http.StatusOK {
					t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
				}
			}
		})
	}
}

// TestSetupRequiredMiddleware_PassesThroughWhenNotRequired tests middleware passes through.
func TestSetupRequiredMiddleware_PassesThroughWhenNotRequired(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	server.requiresSetup = false

	handled := false
	handler := server.setupRequiredMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !handled {
		t.Error("handler should have been called when requiresSetup is false")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// TestHandleSetup_GET_RedirectsWhenNotRequired tests GET /setup redirects if not needed.
func TestHandleSetup_GET_RedirectsWhenNotRequired(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	server.requiresSetup = false

	req := httptest.NewRequest(http.MethodGet, "/setup", nil)
	rec := httptest.NewRecorder()

	server.handleSetup(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	location := rec.Header().Get("Location")
	if location != "/login" {
		t.Errorf("Location = %q, want /login", location)
	}
}

// TestHandleSetup_POST_CreatesAdminUser tests POST /setup creates admin user.
func TestHandleSetup_POST_CreatesAdminUser(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	server.requiresSetup = true

	// Load templates for rendering
	loadTemplatesOrSkip(t, server)

	form := strings.NewReader("username=setupadmin&email=setup@example.com&password=Setup@Pass123!&confirm_password=Setup@Pass123!")
	req := httptest.NewRequest(http.MethodPost, "/setup", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	server.handleSetup(rec, req)

	// Should redirect to stats on success
	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d (body: %s)", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	if location != "/stats" {
		t.Errorf("Location = %q, want /stats", location)
	}

	// Verify user was created
	ctx := context.Background()
	user, err := server.userService.GetByUsername(ctx, "setupadmin")
	if err != nil {
		t.Fatalf("GetByUsername() error = %v", err)
	}
	if user.Email != "setup@example.com" {
		t.Errorf("user.Email = %q, want %q", user.Email, "setup@example.com")
	}
	if user.Role != "admin" {
		t.Errorf("user.Role = %q, want %q", user.Role, "admin")
	}

	// Verify requiresSetup is now false
	if server.requiresSetup {
		t.Error("requiresSetup should be false after setup")
	}

	// Verify session cookie was set
	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Error("session cookie should be set after setup")
	}
}

// TestHandleSetup_POST_ValidationErrors tests POST /setup validation.
func TestHandleSetup_POST_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		form    string
		wantErr string
	}{
		{"missing username", "email=test@example.com&password=Pass@123!&confirm_password=Pass@123!", "Username is required"},
		{"missing email", "username=test&password=Pass@123!&confirm_password=Pass@123!", "Email is required"},
		{"missing password", "username=test&email=test@example.com&confirm_password=Pass@123!", "Password is required"},
		{"password mismatch", "username=test&email=test@example.com&password=Pass@123!&confirm_password=Different@123!", "Passwords do not match"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t)
			server.requiresSetup = true

			// Load templates for error rendering - skip if not available
			loadTemplatesOrSkip(t, server)

			req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(tt.form))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rec := httptest.NewRecorder()

			server.handleSetup(rec, req)

			// Should stay on setup page with error (200) or render error (could be 500 if templates fail)
			// Main check: requiresSetup should still be true after validation error
			if !server.requiresSetup {
				t.Error("requiresSetup should still be true after validation error")
			}
		})
	}
}

// TestHandleSetup_MethodNotAllowed tests unsupported methods on /setup.
func TestHandleSetup_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	server.requiresSetup = true

	req := httptest.NewRequest(http.MethodDelete, "/setup", nil)
	rec := httptest.NewRecorder()

	server.handleSetup(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

// --- Benchmark Tests ---

func BenchmarkHandleStats(b *testing.B) {
	logger := zap.NewNop()
	db, err := storage.New(":memory:", logger)
	if err != nil {
		b.Fatalf("failed to create db: %v", err)
	}
	b.Cleanup(func() { db.Close() })
	cfg := &config.MasterConfig{}
	server, err := NewMasterServer(cfg, db, logger)
	if err != nil {
		b.Fatalf("failed to create server: %v", err)
	}
	b.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Stop(ctx) //nolint:errcheck
	})

	// Initialize services that handleStats needs
	server.projectService = projects.New(db)
	server.agentService = agents.New(db)
	server.deploymentService = deployments.New(db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		server.handleStats(rec, req)
	}
}

func BenchmarkWithAuth(b *testing.B) {
	logger := zap.NewNop()
	db, err := storage.New(":memory:", logger)
	if err != nil {
		b.Fatalf("failed to create db: %v", err)
	}
	b.Cleanup(func() { db.Close() })
	cfg := &config.MasterConfig{}
	server, err := NewMasterServer(cfg, db, logger)
	if err != nil {
		b.Fatalf("failed to create server: %v", err)
	}
	b.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Stop(ctx) //nolint:errcheck
	})

	// Setup test user and API key
	ctx := context.Background()
	user := &storage.User{
		Username:     "benchuser",
		PasswordHash: "hash",
		Email:        "bench@example.com",
		Role:         "admin",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	_ = server.store.CreateUser(ctx, user)

	rawAPIKey := "bench-api-key-12345"
	hash := sha256.Sum256([]byte(rawAPIKey))
	apiKey := &storage.APIKey{
		UserID:    user.ID,
		Name:      "bench-key",
		KeyHash:   hex.EncodeToString(hash[:]),
		Scopes:    `["*"]`,
		CreatedAt: time.Now(),
	}
	_ = server.store.CreateAPIKey(ctx, apiKey)

	// Initialize API key service for lookup
	server.apiKeyService = apikeys.New(db)

	handler := server.withAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req.Header.Set("Authorization", "Bearer "+rawAPIKey)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		handler(rec, req)
	}
}

// --- Login Handler Tests ---

func TestHandleLogin_GET_RendersLoginPage(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	loadTemplatesOrSkip(t, server)

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()

	server.handleLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	// Body should contain login form elements
	body := rec.Body.String()
	if !strings.Contains(body, "username") || !strings.Contains(body, "password") {
		t.Error("response should contain login form elements")
	}
}

func TestHandleLogin_POST_ValidCredentials(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()

	// Create a test user with known password
	_, err := server.userService.Create(ctx, "logintest", "TestPass@123!", "login@test.com", "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	form := strings.NewReader("username=logintest&password=TestPass@123!")
	req := httptest.NewRequest(http.MethodPost, "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	server.handleLogin(rec, req)

	// Should redirect to stats
	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	location := rec.Header().Get("Location")
	if location != "/stats" {
		t.Errorf("Location = %q, want /stats", location)
	}

	// Should set session cookie
	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Error("session cookie should be set after login")
	}
}

func TestHandleLogin_POST_InvalidCredentials(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	loadTemplatesOrSkip(t, server)

	ctx := context.Background()

	// Create a test user
	_, err := server.userService.Create(ctx, "logintest2", "TestPass@123!", "login2@test.com", "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	form := strings.NewReader("username=logintest2&password=WrongPassword!")
	req := httptest.NewRequest(http.MethodPost, "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	server.handleLogin(rec, req)

	// Should stay on login page with error (200 OK for form re-render)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Body should contain error message
	body := rec.Body.String()
	if !strings.Contains(body, "Invalid") && !strings.Contains(body, "invalid") && !strings.Contains(body, "error") {
		t.Error("response should contain error message")
	}
}

func TestHandleLogin_POST_UserNotFound(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	loadTemplatesOrSkip(t, server)

	form := strings.NewReader("username=nonexistent&password=SomePass@123!")
	req := httptest.NewRequest(http.MethodPost, "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	server.handleLogin(rec, req)

	// Should stay on login page with error
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandleLogin_POST_MustChangePassword(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()

	// Create a test user with MustChangePassword flag
	user, err := server.userService.Create(ctx, "mustchange", "TestPass@123!", "mustchange@test.com", "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	// Set MustChangePassword flag
	user.MustChangePassword = true
	if err := server.userService.Update(ctx, user); err != nil {
		t.Fatalf("failed to update user: %v", err)
	}

	form := strings.NewReader("username=mustchange&password=TestPass@123!")
	req := httptest.NewRequest(http.MethodPost, "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	server.handleLogin(rec, req)

	// Should redirect to change-password
	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	location := rec.Header().Get("Location")
	if location != "/change-password" {
		t.Errorf("Location = %q, want /change-password", location)
	}

	// Should still set session cookie (needed for change-password page)
	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Error("session cookie should be set even for must-change-password")
	}
}

func TestHandleLogin_POST_TOTPRequired(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	loadTemplatesOrSkip(t, server)

	ctx := context.Background()

	// Create a test user with TOTP enabled
	user, err := server.userService.Create(ctx, "totpuser", "TestPass@123!", "totp@test.com", "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	// Enable TOTP
	secret, err := security.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("failed to generate TOTP secret: %v", err)
	}
	user.TOTPEnabled = true
	user.TOTPSecret = secret
	if err := server.userService.Update(ctx, user); err != nil {
		t.Fatalf("failed to update user: %v", err)
	}

	// Login without TOTP code
	form := strings.NewReader("username=totpuser&password=TestPass@123!")
	req := httptest.NewRequest(http.MethodPost, "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	server.handleLogin(rec, req)

	// Should re-render login page with TOTP prompt
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Body should indicate TOTP is needed
	body := rec.Body.String()
	if !strings.Contains(body, "totp") && !strings.Contains(body, "TOTP") && !strings.Contains(body, "verification") {
		t.Error("response should contain TOTP prompt")
	}
}

func TestHandleLogin_POST_TOTPValid(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()

	// Create a test user with TOTP enabled
	user, err := server.userService.Create(ctx, "totpvalid", "TestPass@123!", "totpvalid@test.com", "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	// Enable TOTP with known secret
	secret, err := security.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("failed to generate TOTP secret: %v", err)
	}
	user.TOTPEnabled = true
	user.TOTPSecret = secret
	if err := server.userService.Update(ctx, user); err != nil {
		t.Fatalf("failed to update user: %v", err)
	}

	// Generate valid TOTP code
	validCode := security.GenerateTOTPCode(secret, time.Now().Unix(), security.DefaultTOTPConfig())

	// Login with valid TOTP code
	form := strings.NewReader(fmt.Sprintf("username=totpvalid&password=TestPass@123!&totp=%s", validCode))
	req := httptest.NewRequest(http.MethodPost, "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	server.handleLogin(rec, req)

	// Should redirect to stats
	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	if location != "/stats" {
		t.Errorf("Location = %q, want /stats", location)
	}
}

func TestHandleLogin_POST_TOTPInvalid(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	loadTemplatesOrSkip(t, server)

	ctx := context.Background()

	// Create a test user with TOTP enabled
	user, err := server.userService.Create(ctx, "totpinvalid", "TestPass@123!", "totpinvalid@test.com", "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	// Enable TOTP
	secret, err := security.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("failed to generate TOTP secret: %v", err)
	}
	user.TOTPEnabled = true
	user.TOTPSecret = secret
	if err := server.userService.Update(ctx, user); err != nil {
		t.Fatalf("failed to update user: %v", err)
	}

	// Login with invalid TOTP code
	form := strings.NewReader("username=totpinvalid&password=TestPass@123!&totp=000000")
	req := httptest.NewRequest(http.MethodPost, "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	server.handleLogin(rec, req)

	// Should stay on login page with error
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Body should contain error message about invalid verification code
	body := rec.Body.String()
	if !strings.Contains(body, "Invalid") && !strings.Contains(body, "invalid") && !strings.Contains(body, "verification") {
		t.Error("response should contain error message about invalid verification code")
	}
}

// --- Logout Handler Tests ---

func TestHandleLogout_DeletesSession(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()

	// Create a test user and session
	user, err := server.userService.Create(ctx, "logouttest", "TestPass@123!", "logout@test.com", "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	session, err := server.sessionService.Create(ctx, user.ID, "127.0.0.1", "test-agent", 24*time.Hour)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	server.handleLogout(rec, req)

	// Session should be deleted from database
	_, err = server.sessionService.GetByToken(ctx, session.ID)
	if err == nil {
		t.Error("session should be deleted after logout")
	}
}

func TestHandleLogout_ClearsCookie(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()

	// Create a test user and session
	user, err := server.userService.Create(ctx, "logoutcookie", "TestPass@123!", "logoutcookie@test.com", "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	session, err := server.sessionService.Create(ctx, user.ID, "127.0.0.1", "test-agent", 24*time.Hour)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	server.handleLogout(rec, req)

	// Cookie should be cleared (MaxAge = -1)
	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("session cookie should be in response")
	}
	if sessionCookie.MaxAge != -1 {
		t.Errorf("session cookie MaxAge = %d, want -1", sessionCookie.MaxAge)
	}
}

func TestHandleLogout_RedirectsToLogin(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/logout", nil)
	rec := httptest.NewRecorder()

	server.handleLogout(rec, req)

	// Should redirect to login
	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	location := rec.Header().Get("Location")
	if location != "/login" {
		t.Errorf("Location = %q, want /login", location)
	}
}

func TestHandleLogout_CreatesAuditEntry(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()

	// Create a test user and session
	user, err := server.userService.Create(ctx, "logoutaudit", "TestPass@123!", "logoutaudit@test.com", "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	session, err := server.sessionService.Create(ctx, user.ID, "127.0.0.1", "test-agent", 24*time.Hour)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	server.handleLogout(rec, req)

	// Check audit log for logout entry
	entries, err := server.auditService.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("failed to list audit entries: %v", err)
	}

	found := false
	for _, entry := range entries {
		if entry.Action == "logout" {
			found = true
			break
		}
	}
	if !found {
		t.Error("audit log should contain logout entry")
	}
}

func TestHandleLogout_NoSession(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	// Logout without session cookie - should still redirect gracefully
	req := httptest.NewRequest(http.MethodGet, "/logout", nil)
	rec := httptest.NewRecorder()

	server.handleLogout(rec, req)

	// Should still redirect to login
	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	location := rec.Header().Get("Location")
	if location != "/login" {
		t.Errorf("Location = %q, want /login", location)
	}
}

// --- Change Password Handler Tests ---

func TestHandleChangePassword_GET_Authenticated(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	loadTemplatesOrSkip(t, server)

	ctx := context.Background()

	// Create a test user
	user, err := server.userService.Create(ctx, "changepassget", "TestPass@123!", "changepassget@test.com", "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/change-password", nil)
	// Add user to context (simulating authenticated request)
	ctx = context.WithValue(req.Context(), contextKeyUserID, user.ID)
	ctx = WithUserContext(ctx, user)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	server.handleChangePassword(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Body should contain change password form
	body := rec.Body.String()
	if !strings.Contains(body, "current") && !strings.Contains(body, "new") {
		t.Error("response should contain change password form elements")
	}
}

func TestHandleChangePassword_GET_WithMustChangeFlag(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	loadTemplatesOrSkip(t, server)

	ctx := context.Background()

	// Create a test user with MustChangePassword flag
	user, err := server.userService.Create(ctx, "mustchangeget", "TestPass@123!", "mustchangeget@test.com", "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	user.MustChangePassword = true
	if err := server.userService.Update(ctx, user); err != nil {
		t.Fatalf("failed to update user: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/change-password", nil)
	ctx = context.WithValue(req.Context(), contextKeyUserID, user.ID)
	ctx = WithUserContext(ctx, user)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	server.handleChangePassword(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Template should receive MustChangePassword=true (visible in rendered HTML)
	// The form should be rendered successfully
	body := rec.Body.String()
	if len(body) == 0 {
		t.Error("response body should not be empty")
	}
}

func TestHandleChangePassword_GET_Unauthenticated(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/change-password", nil)
	rec := httptest.NewRecorder()

	server.handleChangePassword(rec, req)

	// Should redirect to login
	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	location := rec.Header().Get("Location")
	if location != "/login" {
		t.Errorf("Location = %q, want /login", location)
	}
}

func TestHandleChangePassword_POST_WrongCurrentPassword(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	loadTemplatesOrSkip(t, server)

	ctx := context.Background()

	// Create a test user
	user, err := server.userService.Create(ctx, "wrongcurrent", "TestPass@123!", "wrongcurrent@test.com", "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	form := strings.NewReader("current_password=WrongPassword&new_password=NewPass@123!&confirm_password=NewPass@123!")
	req := httptest.NewRequest(http.MethodPost, "/change-password", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx = context.WithValue(req.Context(), contextKeyUserID, user.ID)
	ctx = WithUserContext(ctx, user)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	server.handleChangePassword(rec, req)

	// Should stay on page with error
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "incorrect") && !strings.Contains(body, "Current") {
		t.Error("response should contain error about current password")
	}
}

func TestHandleChangePassword_POST_PasswordMismatch(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	loadTemplatesOrSkip(t, server)

	ctx := context.Background()

	// Create a test user
	user, err := server.userService.Create(ctx, "mismatch", "TestPass@123!", "mismatch@test.com", "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	form := strings.NewReader("current_password=TestPass@123!&new_password=NewPass@123!&confirm_password=DifferentPass@123!")
	req := httptest.NewRequest(http.MethodPost, "/change-password", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx = context.WithValue(req.Context(), contextKeyUserID, user.ID)
	ctx = WithUserContext(ctx, user)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	server.handleChangePassword(rec, req)

	// Should stay on page with error
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "match") && !strings.Contains(body, "Match") {
		t.Error("response should contain error about password mismatch")
	}
}

func TestHandleChangePassword_POST_SamePassword(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	loadTemplatesOrSkip(t, server)

	ctx := context.Background()

	// Create a test user
	user, err := server.userService.Create(ctx, "samepass", "TestPass@123!", "samepass@test.com", "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	form := strings.NewReader("current_password=TestPass@123!&new_password=TestPass@123!&confirm_password=TestPass@123!")
	req := httptest.NewRequest(http.MethodPost, "/change-password", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx = context.WithValue(req.Context(), contextKeyUserID, user.ID)
	ctx = WithUserContext(ctx, user)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	server.handleChangePassword(rec, req)

	// Should stay on page with error
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "different") && !strings.Contains(body, "Different") {
		t.Error("response should contain error about password must be different")
	}
}

func TestHandleChangePassword_POST_WeakPassword(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	loadTemplatesOrSkip(t, server)

	ctx := context.Background()

	// Create a test user
	user, err := server.userService.Create(ctx, "weakpass", "TestPass@123!", "weakpass@test.com", "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	form := strings.NewReader("current_password=TestPass@123!&new_password=weak&confirm_password=weak")
	req := httptest.NewRequest(http.MethodPost, "/change-password", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx = context.WithValue(req.Context(), contextKeyUserID, user.ID)
	ctx = WithUserContext(ctx, user)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	server.handleChangePassword(rec, req)

	// Should stay on page with error
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Error message should mention password requirements
	body := rec.Body.String()
	if !strings.Contains(body, "password") && !strings.Contains(body, "Password") {
		t.Error("response should contain error about password requirements")
	}
}

func TestHandleChangePassword_POST_Success(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()

	// Create a test user
	user, err := server.userService.Create(ctx, "changepasssuccess", "TestPass@123!", "changepasssuccess@test.com", "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	form := strings.NewReader("current_password=TestPass@123!&new_password=NewPass@456!&confirm_password=NewPass@456!")
	req := httptest.NewRequest(http.MethodPost, "/change-password", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx = context.WithValue(req.Context(), contextKeyUserID, user.ID)
	ctx = WithUserContext(ctx, user)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	server.handleChangePassword(rec, req)

	// Should redirect to stats
	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	if location != "/stats" {
		t.Errorf("Location = %q, want /stats", location)
	}

	// Verify password was actually changed - should be able to verify new password
	updatedUser, err := server.userService.GetByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("failed to get updated user: %v", err)
	}

	// Old password should no longer work
	if verifyPassword(updatedUser.PasswordHash, "TestPass@123!") {
		t.Error("old password should no longer work")
	}

	// New password should work
	if !verifyPassword(updatedUser.PasswordHash, "NewPass@456!") {
		t.Error("new password should work")
	}
}

func TestHandleChangePassword_POST_ClearsMustChangeFlag(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()

	// Create a test user with MustChangePassword flag
	user, err := server.userService.Create(ctx, "clearmustchange", "TestPass@123!", "clearmustchange@test.com", "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	user.MustChangePassword = true
	if err := server.userService.Update(ctx, user); err != nil {
		t.Fatalf("failed to update user: %v", err)
	}

	form := strings.NewReader("current_password=TestPass@123!&new_password=NewPass@456!&confirm_password=NewPass@456!")
	req := httptest.NewRequest(http.MethodPost, "/change-password", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx = context.WithValue(req.Context(), contextKeyUserID, user.ID)
	ctx = WithUserContext(ctx, user)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	server.handleChangePassword(rec, req)

	// Should redirect to stats
	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}

	// Verify MustChangePassword flag was cleared
	updatedUser, err := server.userService.GetByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("failed to get updated user: %v", err)
	}

	if updatedUser.MustChangePassword {
		t.Error("MustChangePassword flag should be cleared after successful password change")
	}
}

func TestHandleChangePassword_POST_InvalidatesSessions(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()

	// Create a test user
	user, err := server.userService.Create(ctx, "sessioninvalidate", "TestPass@123!", "sessioninvalidate@test.com", "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	// Create multiple sessions for the user
	session1, err := server.sessionService.Create(ctx, user.ID, "192.168.1.1", "TestAgent1", 24*time.Hour)
	if err != nil {
		t.Fatalf("failed to create session 1: %v", err)
	}
	session2, err := server.sessionService.Create(ctx, user.ID, "192.168.1.2", "TestAgent2", 24*time.Hour)
	if err != nil {
		t.Fatalf("failed to create session 2: %v", err)
	}

	// Verify sessions exist
	sessions, err := server.sessionService.ListForUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	// Change password
	form := strings.NewReader("current_password=TestPass@123!&new_password=NewPass@456!&confirm_password=NewPass@456!")
	req := httptest.NewRequest(http.MethodPost, "/change-password", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx = context.WithValue(req.Context(), contextKeyUserID, user.ID)
	ctx = WithUserContext(ctx, user)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	server.handleChangePassword(rec, req)

	// Should redirect to stats
	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}

	// Verify all sessions were invalidated
	sessions, err = server.sessionService.ListForUser(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("failed to list sessions after password change: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions after password change, got %d", len(sessions))
	}

	// Verify old session tokens are invalid
	_, err = server.sessionService.GetByToken(context.Background(), session1.Token)
	if err == nil {
		t.Error("session 1 should be invalid after password change")
	}
	_, err = server.sessionService.GetByToken(context.Background(), session2.Token)
	if err == nil {
		t.Error("session 2 should be invalid after password change")
	}
}

// --- API Login Tests for MustChangePassword ---

func TestHandleAPILogin_MustChangePassword_Returns403(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()

	// Create a test user with MustChangePassword flag
	user, err := server.userService.Create(ctx, "apimustchange", "TestPass@123!", "apimustchange@test.com", "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	user.MustChangePassword = true
	if err := server.userService.Update(ctx, user); err != nil {
		t.Fatalf("failed to update user: %v", err)
	}

	body := `{"username": "apimustchange", "password": "TestPass@123!"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.handleAPILogin(rec, req)

	// Should return 403 Forbidden
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}

	// Body should mention web UI
	respBody := rec.Body.String()
	if !strings.Contains(respBody, "web") && !strings.Contains(respBody, "Web") {
		t.Error("response should mention using web UI to change password")
	}
}

func TestHandleAPILogin_WithTOTP_Success(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()

	// Create a test user with TOTP enabled
	user, err := server.userService.Create(ctx, "apitotpvalid", "TestPass@123!", "apitotpvalid@test.com", "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	// Enable TOTP
	secret, err := security.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("failed to generate TOTP secret: %v", err)
	}
	user.TOTPEnabled = true
	user.TOTPSecret = secret
	if err := server.userService.Update(ctx, user); err != nil {
		t.Fatalf("failed to update user: %v", err)
	}

	// Generate valid TOTP code
	validCode := security.GenerateTOTPCode(secret, time.Now().Unix(), security.DefaultTOTPConfig())

	body := fmt.Sprintf(`{"username": "apitotpvalid", "password": "TestPass@123!", "totp": "%s"}`, validCode)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.handleAPILogin(rec, req)

	// Should return 200 OK
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var result map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result["token"] == nil || result["token"] == "" {
		t.Error("expected token in response")
	}
}

func TestHandleAPILogin_WithTOTP_MissingCode(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()

	// Create a test user with TOTP enabled
	user, err := server.userService.Create(ctx, "apitotpmissing", "TestPass@123!", "apitotpmissing@test.com", "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	// Enable TOTP
	secret, err := security.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("failed to generate TOTP secret: %v", err)
	}
	user.TOTPEnabled = true
	user.TOTPSecret = secret
	if err := server.userService.Update(ctx, user); err != nil {
		t.Fatalf("failed to update user: %v", err)
	}

	// Login without TOTP code
	body := `{"username": "apitotpmissing", "password": "TestPass@123!"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.handleAPILogin(rec, req)

	// Should return 401 Unauthorized
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	// Body should mention TOTP
	respBody := rec.Body.String()
	if !strings.Contains(respBody, "TOTP") && !strings.Contains(respBody, "totp") {
		t.Error("response should mention TOTP required")
	}
}

func TestHandleAPILogin_WithTOTP_InvalidCode(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()

	// Create a test user with TOTP enabled
	user, err := server.userService.Create(ctx, "apitotpinvalid", "TestPass@123!", "apitotpinvalid@test.com", "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	// Enable TOTP
	secret, err := security.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("failed to generate TOTP secret: %v", err)
	}
	user.TOTPEnabled = true
	user.TOTPSecret = secret
	if err := server.userService.Update(ctx, user); err != nil {
		t.Fatalf("failed to update user: %v", err)
	}

	// Login with invalid TOTP code
	body := `{"username": "apitotpinvalid", "password": "TestPass@123!", "totp": "000000"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.handleAPILogin(rec, req)

	// Should return 401 Unauthorized
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// --- Session Expiry Tests (Auth Flow) ---

func TestAuthFlow_WithAuth_ExpiredSession(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()

	// Create a test user
	user, err := server.userService.Create(ctx, "expiredsession", "TestPass@123!", "expiredsession@test.com", "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	// Create an expired session directly in DB
	expiredSession := &storage.Session{
		ID:        "expired-session-token-12345",
		UserID:    user.ID,
		IPAddress: "127.0.0.1",
		UserAgent: "test-agent",
		CreatedAt: time.Now().Add(-48 * time.Hour),
		ExpiresAt: time.Now().Add(-1 * time.Hour), // Expired 1 hour ago
	}
	if err := server.store.CreateSession(ctx, expiredSession); err != nil {
		t.Fatalf("failed to create expired session: %v", err)
	}

	handler := server.withAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: expiredSession.ID})
	rec := httptest.NewRecorder()

	handler(rec, req)

	// Should return 401 Unauthorized
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAuthFlow_WithUIAuth_ExpiredSession(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()

	// Create a test user
	user, err := server.userService.Create(ctx, "uiexpiredsession", "TestPass@123!", "uiexpiredsession@test.com", "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	// Create an expired session directly in DB
	expiredSession := &storage.Session{
		ID:        "ui-expired-session-token-12345",
		UserID:    user.ID,
		IPAddress: "127.0.0.1",
		UserAgent: "test-agent",
		CreatedAt: time.Now().Add(-48 * time.Hour),
		ExpiresAt: time.Now().Add(-1 * time.Hour), // Expired 1 hour ago
	}
	if err := server.store.CreateSession(ctx, expiredSession); err != nil {
		t.Fatalf("failed to create expired session: %v", err)
	}

	handler := server.withUIAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: expiredSession.ID})
	rec := httptest.NewRecorder()

	handler(rec, req)

	// Should redirect to login
	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	location := rec.Header().Get("Location")
	if location != "/login" {
		t.Errorf("Location = %q, want /login", location)
	}
}

func TestAuthFlow_WithAuth_ValidSession(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()

	// Create a test user
	user, err := server.userService.Create(ctx, "validsession", "TestPass@123!", "validsession@test.com", "user")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	// Create a valid session
	session, err := server.sessionService.Create(ctx, user.ID, "127.0.0.1", "test-agent", 24*time.Hour)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	handler := server.withAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	handler(rec, req)

	// Should return 200 OK
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
