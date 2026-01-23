package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/config"
	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

func newTestServer(t *testing.T) *MasterServer {
	t.Helper()

	logger := zap.NewNop()
	db, err := storage.New(":memory:")
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

func TestNewMasterServer(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	db, _ := storage.New(":memory:")

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

func TestHandleProjects_GET(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	rec := httptest.NewRecorder()

	server.handleProjects(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", contentType)
	}
}

func TestHandleProjects_POST(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	body := bytes.NewBufferString(`{"name": "test-project"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.handleProjects(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != "created" {
		t.Errorf("status = %s, want created", resp["status"])
	}
}

func TestHandleProjects_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/projects", nil)
	rec := httptest.NewRecorder()

	server.handleProjects(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDeployments_GET(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments", nil)
	rec := httptest.NewRecorder()

	server.handleDeployments(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandleDeployments_POST(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	body := bytes.NewBufferString(`{"project": "test", "branch": "main"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/deployments", body)
	rec := httptest.NewRecorder()

	server.handleDeployments(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != "queued" {
		t.Errorf("status = %s, want queued", resp["status"])
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
	json.NewDecoder(rec.Body).Decode(&agents)

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

	server := newTestServer(t)
	called := false
	handler := server.withAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req.Header.Set("Authorization", "Bearer valid-api-key")
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

	server := newTestServer(t)
	called := false
	handler := server.withUIAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "valid-session-token"})
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
		w.Write([]byte("ok"))
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
	json.NewDecoder(rec.Body).Decode(&result)

	if result["key"] != "value" {
		t.Errorf("key = %v, want value", result["key"])
	}
}

func BenchmarkHandleHealth(b *testing.B) {
	logger := zap.NewNop()
	db, _ := storage.New(":memory:")
	cfg := &config.MasterConfig{}
	server, _ := NewMasterServer(cfg, db, logger)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		server.handleHealth(rec, req)
	}
}
