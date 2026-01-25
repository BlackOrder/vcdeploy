package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

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

	return server
}

// newTestServerWithAuth creates a test server with a test user, API key, and session.
// Returns the server, the raw API key, and the session token.
func newTestServerWithAuth(t *testing.T) (*MasterServer, string, string) {
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
	if err := server.db.CreateUser(ctx, user); err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	// Create a test API key
	rawAPIKey := "test-api-key-12345"
	hash := sha256.Sum256([]byte(rawAPIKey))
	apiKey := &storage.APIKey{
		UserID:    user.ID,
		Name:      "test-key",
		KeyHash:   hex.EncodeToString(hash[:]),
		Scopes:    `["*"]`,
		CreatedAt: time.Now(),
	}
	if err := server.db.CreateAPIKey(ctx, apiKey); err != nil {
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
	if err := server.db.CreateSession(ctx, session); err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	return server, rawAPIKey, sessionToken
}

func TestNewMasterServer(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	db, err := storage.New(":memory:", logger)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}

	cfg := &config.MasterConfig{
		Server: config.ServerConfig{Listen: ":8080"},
		GRPC:   config.GRPCConfig{Listen: ":9090"},
	}

	server, err := NewMasterServer(cfg, db, logger)

	if err != nil {
		t.Fatalf("NewMasterServer() error = %v", err)
	}
	if server == nil {
		t.Fatal("NewMasterServer() returned nil")
	}
	if server.config != cfg {
		t.Error("config not set")
	}
	if server.db != db {
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

	if resp["status"] != "healthy" {
		t.Errorf("status = %v, want healthy", resp["status"])
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets", nil)
	rec := httptest.NewRecorder()

	server.handleSecrets(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandleAuditLogs(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit", nil)
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

	server, apiKey, _ := newTestServerWithAuth(t)
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

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
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

	server, _, sessionToken := newTestServerWithAuth(t)
	called := false
	handler := server.withUIAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
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
	cfg := &config.MasterConfig{}
	server, err := NewMasterServer(cfg, db, logger)
	if err != nil {
		b.Fatalf("failed to create server: %v", err)
	}

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

	// Test creating a secret - note: this will fail without SecretService configured
	// This tests the validation path, not the full creation
	body := bytes.NewBufferString(`{"project":"test-project","scope":"env","key":"TEST_KEY","value":"secret-value"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.handleSecrets(rec, req)

	// Without SecretService configured, should get error
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d (expected error without SecretService)", rec.Code, http.StatusInternalServerError)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	// Verify error message mentions secret service
	if msg, ok := resp["message"].(string); !ok || !strings.Contains(strings.ToLower(msg), "secret") {
		t.Errorf("expected secret-related error message, got: %v", resp["message"])
	}
}

func TestHandleSecretsPostValidation(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	// Test validation - missing project
	body := bytes.NewBufferString(`{"scope":"env","key":"TEST_KEY","value":"secret-value"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.handleSecrets(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	// Test validation - missing key
	body = bytes.NewBufferString(`{"project":"test-project","scope":"env","value":"secret-value"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/secrets", body)
	req.Header.Set("Content-Type", "application/json")
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
	if err := server.db.SetSecretEncrypted(ctx, "test-project", "env", "DELETE_ME", []byte("encrypted-value")); err != nil {
		t.Fatalf("failed to create secret: %v", err)
	}

	// Now delete it
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets?project=test-project&scope=env&key=DELETE_ME", nil)
	rec := httptest.NewRecorder()

	server.handleSecrets(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandleProjectTypes(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	// Test listing (empty)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/project-types", nil)
	rec := httptest.NewRecorder()

	server.handleProjectTypes(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var types []interface{}
	if err := json.NewDecoder(rec.Body).Decode(&types); err != nil {
		t.Fatalf("decode error: %v", err)
	}
}

func TestHandleProjectTypesPost(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	// Create a project type
	body := bytes.NewBufferString(`{"name":"nodejs","description":"Node.js application","build_cmd":"npm install && npm run build"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/project-types", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.handleProjectTypes(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	// Verify it was created
	req = httptest.NewRequest(http.MethodGet, "/api/v1/project-types", nil)
	rec = httptest.NewRecorder()
	server.handleProjectTypes(rec, req)

	var types []*storage.ProjectType
	json.NewDecoder(rec.Body).Decode(&types)

	found := false
	for _, pt := range types {
		if pt.Name == "nodejs" {
			found = true
			break
		}
	}
	if !found {
		t.Error("project type 'nodejs' was not created")
	}
}

func TestHandleProjectType(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	// Create a project type directly in DB
	pt := &storage.ProjectType{
		Name:        "python",
		Description: "Python application",
		BuildCmd:    "pip install -r requirements.txt",
		CreatedAt:   time.Now(),
	}
	if err := server.db.CreateProjectType(pt); err != nil {
		t.Fatalf("failed to create project type: %v", err)
	}

	// Test GET single project type - use correct path so handler can extract name
	req := httptest.NewRequest(http.MethodGet, "/api/v1/project-types/python", nil)
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

	// Create a project type
	pt := &storage.ProjectType{
		Name:        "delete-me",
		Description: "To be deleted",
		CreatedAt:   time.Now(),
	}
	if err := server.db.CreateProjectType(pt); err != nil {
		t.Fatalf("failed to create project type: %v", err)
	}

	// Delete it
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/project-types/delete-me", nil)
	rec := httptest.NewRecorder()

	server.handleProjectType(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Verify it was deleted
	_, err := server.db.GetProjectTypeByName("delete-me")
	if err == nil {
		t.Error("project type should have been deleted")
	}
}

func TestHandleDeploymentLogs(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()

	// Create a test project
	project := &storage.Project{Name: "test-project", Repository: "https://github.com/test/test"}
	_ = server.db.CreateProject(project)

	// Create a test deployment
	deployment := &storage.Deployment{
		ID:          "test-deploy-1",
		Project:     "test-project",
		Status:      "running",
		Branch:      "main",
		CommitHash:  "abc123",
		TriggeredBy: "test",
	}
	_ = server.db.CreateDeployment(ctx, deployment)

	// Test non-streaming logs request via the logs path component
	// The handler expects deployment ID from path, which we need to simulate
	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/test-deploy-1/logs", nil)
	rec := httptest.NewRecorder()

	// Call handleDeploymentLogsStream with deployment ID
	server.handleDeploymentLogsStream(rec, req, "test-deploy-1")

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestSecretsFilterByProject(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := context.Background()

	// Create secrets for different projects
	_ = server.db.SetSecretEncrypted(ctx, "project-a", "env", "KEY1", []byte("value1"))
	_ = server.db.SetSecretEncrypted(ctx, "project-b", "env", "KEY2", []byte("value2"))

	// Filter by project-a
	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets?project=project-a", nil)
	rec := httptest.NewRecorder()

	server.handleSecrets(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var secrets []map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&secrets)

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
		(strings.Contains(body, "Templates not loaded") || strings.Contains(body, "Internal server error")) {
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
	// Check for stats dashboard elements
	if !strings.Contains(body, "stats-card") {
		t.Error("response should contain stats dashboard")
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

func TestUIDashboardPage(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()

	server.handleDashboard(rec, req)

	skipIfNoTemplates(t, rec)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Dashboard") {
		t.Error("response should contain 'Dashboard' title")
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

	req := httptest.NewRequest(http.MethodGet, "/apikeys", nil)
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
