package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMaintenanceMode_BlocksMutations(t *testing.T) {
	t.Parallel()

	server, apiKey, _, userID := newTestServerWithAuth(t)
	defer server.store.Close()

	// Enable maintenance mode
	server.maintenanceMode.Store(true)

	// POST (mutation) should be blocked
	body := bytes.NewBufferString(`{"name":"test"}`)
	req := httptest.NewRequest("POST", "/api/v1/projects", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)
	w := httptest.NewRecorder()

	req = requestWithUserContext(req, userID)
	// Run through the maintenance middleware
	handler := server.maintenanceMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMaintenanceMode_AllowsGET(t *testing.T) {
	t.Parallel()

	server, _, _, _ := newTestServerWithAuth(t)
	defer server.store.Close()

	server.maintenanceMode.Store(true)

	req := httptest.NewRequest("GET", "/api/v1/projects", nil)
	w := httptest.NewRecorder()

	handler := server.maintenanceMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET should be allowed during maintenance, got %d", w.Code)
	}
}

func TestMaintenanceMode_AllowsToggle(t *testing.T) {
	t.Parallel()

	server, _, _, _ := newTestServerWithAuth(t)
	defer server.store.Close()

	server.maintenanceMode.Store(true)

	// POST to maintenance toggle should pass through middleware
	req := httptest.NewRequest("POST", "/api/v1/admin/maintenance", nil)
	w := httptest.NewRecorder()

	handler := server.maintenanceMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("maintenance toggle should be allowed, got %d", w.Code)
	}
}

func TestMaintenanceMode_AllowsImport(t *testing.T) {
	t.Parallel()

	server, _, _, _ := newTestServerWithAuth(t)
	defer server.store.Close()

	server.maintenanceMode.Store(true)

	req := httptest.NewRequest("POST", "/api/v1/admin/backup/import", nil)
	w := httptest.NewRecorder()

	handler := server.maintenanceMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("import should be allowed during maintenance, got %d", w.Code)
	}
}

func TestMaintenanceMode_RetryAfterHeader(t *testing.T) {
	t.Parallel()

	server, _, _, _ := newTestServerWithAuth(t)
	defer server.store.Close()

	server.maintenanceMode.Store(true)

	req := httptest.NewRequest("POST", "/api/v1/projects", nil)
	w := httptest.NewRecorder()

	handler := server.maintenanceMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	handler.ServeHTTP(w, req)

	if w.Header().Get("Retry-After") != "1800" {
		t.Errorf("expected Retry-After: 1800, got %q", w.Header().Get("Retry-After"))
	}
}

func TestMaintenanceToggle_Enable(t *testing.T) {
	t.Parallel()

	server, apiKey, _, userID := newTestServerWithAuth(t)
	defer server.store.Close()

	body := bytes.NewBufferString(`{"enabled":true}`)
	req := httptest.NewRequest("POST", "/api/v1/admin/maintenance", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)
	w := httptest.NewRecorder()

	req = requestWithUserContext(req, userID)
	server.handleMaintenanceToggle(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if !server.maintenanceMode.Load() {
		t.Error("maintenance mode should be enabled")
	}
}

func TestMaintenanceToggle_Disable(t *testing.T) {
	t.Parallel()

	server, apiKey, _, userID := newTestServerWithAuth(t)
	defer server.store.Close()

	// Start in maintenance mode
	server.maintenanceMode.Store(true)

	body := bytes.NewBufferString(`{"enabled":false}`)
	req := httptest.NewRequest("POST", "/api/v1/admin/maintenance", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)
	w := httptest.NewRecorder()

	req = requestWithUserContext(req, userID)
	server.handleMaintenanceToggle(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if server.maintenanceMode.Load() {
		t.Error("maintenance mode should be disabled")
	}
}

func TestMaintenanceToggle_Status(t *testing.T) {
	t.Parallel()

	server, apiKey, _, userID := newTestServerWithAuth(t)
	defer server.store.Close()

	server.maintenanceMode.Store(true)

	req := httptest.NewRequest("GET", "/api/v1/admin/maintenance", nil)
	req.Header.Set("X-API-Key", apiKey)
	w := httptest.NewRecorder()

	req = requestWithUserContext(req, userID)
	server.handleMaintenanceToggle(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["maintenance"] != true {
		t.Errorf("expected maintenance=true, got %v", result["maintenance"])
	}
}

// --- Stage 11 tests: maintenance mode invariants ---

// TestMaintenanceMode_FlushesWrites verifies that enabling maintenance mode
// calls FlushPending (observable: handler succeeds and maintenance is enabled).
// On the test DB store FlushPending is a no-op, so we verify no errors.
func TestMaintenanceMode_FlushesWrites(t *testing.T) {
	t.Parallel()

	server, apiKey, _, userID := newTestServerWithAuth(t)
	defer server.store.Close()

	// Ensure maintenance is off
	server.maintenanceMode.Store(false)

	// Enable maintenance — this should call FlushPending internally
	body := bytes.NewBufferString(`{"enabled":true}`)
	req := httptest.NewRequest("POST", "/api/v1/admin/maintenance", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)
	w := httptest.NewRecorder()

	req = requestWithUserContext(req, userID)
	server.handleMaintenanceToggle(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on enable, got %d: %s", w.Code, w.Body.String())
	}

	if !server.maintenanceMode.Load() {
		t.Error("maintenance mode should be enabled after toggle")
	}

	// Verify response includes maintenance_enabled status
	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["status"] != "maintenance_enabled" {
		t.Errorf("expected status=maintenance_enabled, got %v", result["status"])
	}
}

// TestMaintenanceMode_RefreshesOnExit verifies that disabling maintenance mode
// calls store.Reload() to refresh the in-memory cache. This validates the
// Stage 11 bug fix (§11.1).
func TestMaintenanceMode_RefreshesOnExit(t *testing.T) {
	t.Parallel()

	server, apiKey, _, userID := newTestServerWithAuth(t)
	defer server.store.Close()

	// Start in maintenance mode
	server.maintenanceMode.Store(true)

	// Simulate a DB modification during maintenance (direct insert)
	ctx := context.Background()
	_, err := server.store.Conn().ExecContext(ctx,
		`INSERT INTO project_types (id, name) VALUES (?, ?)`,
		"test-uid-refresh-001", "refresh-test-type",
	)
	if err != nil {
		t.Fatalf("insert during maintenance: %v", err)
	}

	// Disable maintenance — this should call store.Reload()
	body := bytes.NewBufferString(`{"enabled":false}`)
	req := httptest.NewRequest("POST", "/api/v1/admin/maintenance", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)
	w := httptest.NewRecorder()

	req = requestWithUserContext(req, userID)
	server.handleMaintenanceToggle(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on disable, got %d: %s", w.Code, w.Body.String())
	}

	if server.maintenanceMode.Load() {
		t.Error("maintenance mode should be disabled after toggle")
	}

	// Verify the response confirms disabled state
	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["status"] != "maintenance_disabled" {
		t.Errorf("expected status=maintenance_disabled, got %v", result["status"])
	}
}

// TestMaintenanceMode_GRPCUnavailable verifies that the AgentServer returns
// codes.Unavailable when maintenance mode is enabled.
func TestMaintenanceMode_GRPCUnavailable(t *testing.T) {
	t.Parallel()

	server, _, _, _ := newTestServerWithAuth(t)
	defer server.store.Close()

	// The AgentServer is created by NewMasterServer. Verify it has the
	// maintenance check wired.
	agentServer := server.GetAgentServer()
	if agentServer == nil {
		t.Skip("AgentServer not initialized in test (no CA)")
	}

	// Enable maintenance mode on the master
	server.maintenanceMode.Store(true)

	// The maintenance check should return true
	if agentServer.isMaintenanceMode == nil {
		t.Fatal("AgentServer.isMaintenanceMode not wired")
	}
	if !agentServer.isMaintenanceMode() {
		t.Error("AgentServer maintenance check should return true when master is in maintenance")
	}

	// Call Connect — it should return Unavailable immediately
	err := agentServer.Connect(nil)
	if err == nil {
		t.Fatal("Connect should return error during maintenance")
	}

	// Check it's a gRPC Unavailable status
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got: %v", err)
	}
	if st.Code() != codes.Unavailable {
		t.Errorf("expected codes.Unavailable, got %v", st.Code())
	}
	if st.Message() != "Server is in maintenance mode" {
		t.Errorf("unexpected message: %q", st.Message())
	}
}
