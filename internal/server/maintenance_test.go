package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
