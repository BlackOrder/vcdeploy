// Package server provides HTTP and gRPC servers for vcdeploy.
package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDefaultSecurityConfig(t *testing.T) {
	cfg := DefaultSecurityConfig()

	if !cfg.EnableHSTS {
		t.Error("expected HSTS to be enabled by default")
	}
	if cfg.HSTSMaxAge != 365*24*time.Hour {
		t.Errorf("expected HSTS max-age 1 year, got %s", cfg.HSTSMaxAge)
	}
	if !cfg.HSTSIncludeSubdomains {
		t.Error("expected HSTS includeSubdomains by default")
	}
	if !cfg.EnableCSP {
		t.Error("expected CSP to be enabled by default")
	}
	if !cfg.EnableXFrameOptions {
		t.Error("expected X-Frame-Options to be enabled by default")
	}
	if cfg.XFrameOptions != "SAMEORIGIN" {
		t.Errorf("expected X-Frame-Options SAMEORIGIN, got %s", cfg.XFrameOptions)
	}
	if !cfg.EnableXContentTypeOptions {
		t.Error("expected X-Content-Type-Options to be enabled by default")
	}
	if !cfg.EnableXXSSProtection {
		t.Error("expected X-XSS-Protection to be enabled by default")
	}
	if !cfg.EnableReferrerPolicy {
		t.Error("expected Referrer-Policy to be enabled by default")
	}
	if cfg.ReferrerPolicy != "strict-origin-when-cross-origin" {
		t.Errorf("expected strict-origin-when-cross-origin, got %s", cfg.ReferrerPolicy)
	}
	if !cfg.EnableCSRF {
		t.Error("expected CSRF to be enabled by default")
	}
	if cfg.CSRFTokenLength != 32 {
		t.Errorf("expected CSRF token length 32, got %d", cfg.CSRFTokenLength)
	}
	if cfg.CSRFTokenExpiry != 1*time.Hour {
		t.Errorf("expected CSRF token expiry 1h, got %s", cfg.CSRFTokenExpiry)
	}
}

func TestDefaultCSPDirectives(t *testing.T) {
	directives := defaultCSPDirectives()

	if directives["default-src"] != "'self'" {
		t.Errorf("expected default-src 'self', got %s", directives["default-src"])
	}
	if !strings.Contains(directives["script-src"], "'self'") {
		t.Errorf("expected script-src to contain 'self', got %s", directives["script-src"])
	}
	if directives["frame-src"] != "'none'" {
		t.Errorf("expected frame-src 'none', got %s", directives["frame-src"])
	}
	if directives["object-src"] != "'none'" {
		t.Errorf("expected object-src 'none', got %s", directives["object-src"])
	}
}

func TestNewSecurityMiddleware(t *testing.T) {
	cfg := DefaultSecurityConfig()
	sm := NewSecurityMiddleware(cfg)

	if sm == nil {
		t.Fatal("expected non-nil middleware")
	}
	if sm.csrfTokens == nil {
		t.Error("expected csrf tokens map to be initialized")
	}
}

func TestNewSecurityMiddleware_Defaults(t *testing.T) {
	// Test with zero values - should set defaults
	sm := NewSecurityMiddleware(SecurityConfig{})

	if sm.config.CSRFTokenLength != 32 {
		t.Errorf("expected default CSRF token length 32, got %d", sm.config.CSRFTokenLength)
	}
	if sm.config.CSRFTokenExpiry != 1*time.Hour {
		t.Errorf("expected default CSRF token expiry 1h, got %s", sm.config.CSRFTokenExpiry)
	}
	if len(sm.config.CSRFSafeMethods) != 3 {
		t.Errorf("expected 3 safe methods, got %d", len(sm.config.CSRFSafeMethods))
	}
}

func TestSecurityMiddleware_AddSecurityHeaders(t *testing.T) {
	cfg := DefaultSecurityConfig()
	sm := NewSecurityMiddleware(cfg)

	handler := sm.HeadersOnlyMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Check X-Frame-Options
	if rec.Header().Get("X-Frame-Options") != "SAMEORIGIN" {
		t.Errorf("expected X-Frame-Options SAMEORIGIN, got %s", rec.Header().Get("X-Frame-Options"))
	}

	// Check X-Content-Type-Options
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("expected X-Content-Type-Options nosniff, got %s", rec.Header().Get("X-Content-Type-Options"))
	}

	// Check X-XSS-Protection
	if rec.Header().Get("X-XSS-Protection") != "1; mode=block" {
		t.Errorf("expected X-XSS-Protection 1; mode=block, got %s", rec.Header().Get("X-XSS-Protection"))
	}

	// Check Referrer-Policy
	if rec.Header().Get("Referrer-Policy") != "strict-origin-when-cross-origin" {
		t.Errorf("expected Referrer-Policy strict-origin-when-cross-origin, got %s", rec.Header().Get("Referrer-Policy"))
	}

	// Check Permissions-Policy
	if rec.Header().Get("Permissions-Policy") == "" {
		t.Error("expected Permissions-Policy header")
	}

	// Check CSP
	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Error("expected Content-Security-Policy header")
	}
}

func TestSecurityMiddleware_HSTS_OnlyWithTLS(t *testing.T) {
	cfg := DefaultSecurityConfig()
	sm := NewSecurityMiddleware(cfg)

	handler := sm.HeadersOnlyMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Request without TLS
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// HSTS should not be set without TLS
	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Error("HSTS should not be set without TLS")
	}
}

func TestSecurityMiddleware_DisabledFeatures(t *testing.T) {
	cfg := SecurityConfig{
		EnableHSTS:                false,
		EnableCSP:                 false,
		EnableXFrameOptions:       false,
		EnableXContentTypeOptions: false,
		EnableXXSSProtection:      false,
		EnableReferrerPolicy:      false,
		EnableCSRF:                false,
	}
	sm := NewSecurityMiddleware(cfg)

	handler := sm.HeadersOnlyMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// These headers should not be set when disabled
	if rec.Header().Get("X-Frame-Options") != "" {
		t.Error("X-Frame-Options should not be set when disabled")
	}
	if rec.Header().Get("X-Content-Type-Options") != "" {
		t.Error("X-Content-Type-Options should not be set when disabled")
	}
	if rec.Header().Get("X-XSS-Protection") != "" {
		t.Error("X-XSS-Protection should not be set when disabled")
	}
	if rec.Header().Get("Referrer-Policy") != "" {
		t.Error("Referrer-Policy should not be set when disabled")
	}
	if rec.Header().Get("Content-Security-Policy") != "" {
		t.Error("CSP should not be set when disabled")
	}
}

func TestSecurityMiddleware_IsSafeMethod(t *testing.T) {
	cfg := DefaultSecurityConfig()
	sm := NewSecurityMiddleware(cfg)

	tests := []struct {
		method   string
		expected bool
	}{
		{"GET", true},
		{"HEAD", true},
		{"OPTIONS", true},
		{"POST", false},
		{"PUT", false},
		{"DELETE", false},
		{"PATCH", false},
	}

	for _, tc := range tests {
		t.Run(tc.method, func(t *testing.T) {
			result := sm.isSafeMethod(tc.method)
			if result != tc.expected {
				t.Errorf("isSafeMethod(%s) = %v, expected %v", tc.method, result, tc.expected)
			}
		})
	}
}

func TestSecurityMiddleware_IsExemptPath(t *testing.T) {
	cfg := DefaultSecurityConfig()
	sm := NewSecurityMiddleware(cfg)

	tests := []struct {
		path     string
		expected bool
	}{
		{"/webhook/github/", true},
		{"/webhook/github/hook123", true},
		{"/webhook/gitlab/", true},
		{"/webhook/bitbucket/", true},
		{"/api/v1/agents/", true},
		{"/api/v1/agents/abc123", true},
		{"/api/v1/users", false},
		{"/dashboard", false},
		{"/", false},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			result := sm.isExemptPath(tc.path)
			if result != tc.expected {
				t.Errorf("isExemptPath(%s) = %v, expected %v", tc.path, result, tc.expected)
			}
		})
	}
}

func TestSecurityMiddleware_GenerateCSRFToken(t *testing.T) {
	cfg := DefaultSecurityConfig()
	sm := NewSecurityMiddleware(cfg)

	sessionID := "test-session-123"
	token1, err := sm.GenerateCSRFToken(sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if token1 == "" {
		t.Fatal("expected non-empty CSRF token")
	}

	// Token should be stored
	sm.csrfMu.RLock()
	storedToken, exists := sm.csrfTokens[sessionID]
	sm.csrfMu.RUnlock()

	if !exists {
		t.Fatal("expected token to be stored")
	}
	if storedToken.Token != token1 {
		t.Error("stored token doesn't match generated token")
	}
	if time.Now().After(storedToken.ExpiresAt) {
		t.Error("token should not be expired immediately")
	}

	// Generate new token for same session (should overwrite)
	token2, err := sm.GenerateCSRFToken(sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token1 == token2 {
		t.Error("expected different token on regeneration")
	}
}

func TestSecurityMiddleware_GetCSRFToken(t *testing.T) {
	cfg := DefaultSecurityConfig()
	sm := NewSecurityMiddleware(cfg)

	sessionID := "test-session-456"

	// First call should generate a token
	token1, err := sm.GetCSRFToken(sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token1 == "" {
		t.Fatal("expected non-empty token")
	}

	// Second call should return the same token
	token2, err := sm.GetCSRFToken(sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token1 != token2 {
		t.Error("expected same token on second call")
	}
}

func TestSecurityMiddleware_GetCSRFToken_ExpiredToken(t *testing.T) {
	cfg := SecurityConfig{
		EnableCSRF:      true,
		CSRFTokenLength: 32,
		CSRFTokenExpiry: 1 * time.Millisecond, // Very short expiry
	}
	sm := NewSecurityMiddleware(cfg)

	sessionID := "test-session-expire"
	token1, err := sm.GenerateCSRFToken(sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for token to expire
	time.Sleep(5 * time.Millisecond)

	// GetCSRFToken should generate a new token
	token2, err := sm.GetCSRFToken(sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token1 == token2 {
		t.Error("expected new token after expiration")
	}
}

func TestSecurityMiddleware_ValidateCSRFToken(t *testing.T) {
	cfg := DefaultSecurityConfig()
	sm := NewSecurityMiddleware(cfg)

	sessionID := "test-session-validate"
	token, err := sm.GenerateCSRFToken(sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Create request with valid token
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("X-CSRF-Token", token)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})

	if !sm.validateCSRFToken(req) {
		t.Error("expected valid CSRF token to pass validation")
	}
}

func TestSecurityMiddleware_ValidateCSRFToken_InvalidToken(t *testing.T) {
	cfg := DefaultSecurityConfig()
	sm := NewSecurityMiddleware(cfg)

	sessionID := "test-session-invalid"
	_, err := sm.GenerateCSRFToken(sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Create request with invalid token
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("X-CSRF-Token", "invalid-token")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})

	if sm.validateCSRFToken(req) {
		t.Error("expected invalid CSRF token to fail validation")
	}
}

func TestSecurityMiddleware_ValidateCSRFToken_MissingToken(t *testing.T) {
	cfg := DefaultSecurityConfig()
	sm := NewSecurityMiddleware(cfg)

	sessionID := "test-session-missing"
	_, _ = sm.GenerateCSRFToken(sessionID)

	// Create request without token
	req := httptest.NewRequest("POST", "/", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})

	if sm.validateCSRFToken(req) {
		t.Error("expected missing CSRF token to fail validation")
	}
}

func TestSecurityMiddleware_ValidateCSRFToken_MissingSession(t *testing.T) {
	cfg := DefaultSecurityConfig()
	sm := NewSecurityMiddleware(cfg)

	// Create request without session cookie
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("X-CSRF-Token", "some-token")

	if sm.validateCSRFToken(req) {
		t.Error("expected missing session to fail validation")
	}
}

func TestSecurityMiddleware_ValidateCSRFToken_FormValue(t *testing.T) {
	cfg := DefaultSecurityConfig()
	sm := NewSecurityMiddleware(cfg)

	sessionID := "test-session-form"
	token, err := sm.GenerateCSRFToken(sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Create request with token in form
	req := httptest.NewRequest("POST", "/", strings.NewReader("_csrf_token="+token))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})

	if !sm.validateCSRFToken(req) {
		t.Error("expected valid CSRF token from form to pass validation")
	}
}

func TestSecurityMiddleware_CSRFProtection(t *testing.T) {
	cfg := DefaultSecurityConfig()
	sm := NewSecurityMiddleware(cfg)

	sessionID := "csrf-middleware-test"
	token, err := sm.GenerateCSRFToken(sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	handler := sm.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// POST without CSRF token should fail
	req := httptest.NewRequest("POST", "/protected", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden, got %d", rec.Code)
	}

	// POST with valid CSRF token should succeed
	req = httptest.NewRequest("POST", "/protected", nil)
	req.Header.Set("X-CSRF-Token", token)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})
	rec = httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}
}

func TestSecurityMiddleware_CSRFExemptPaths(t *testing.T) {
	cfg := DefaultSecurityConfig()
	sm := NewSecurityMiddleware(cfg)

	handler := sm.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// POST to webhook path without CSRF token should succeed
	req := httptest.NewRequest("POST", "/webhook/github/hook123", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK for exempt path, got %d", rec.Code)
	}
}

func TestSecurityMiddleware_SafeMethods(t *testing.T) {
	cfg := DefaultSecurityConfig()
	sm := NewSecurityMiddleware(cfg)

	handler := sm.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// GET without CSRF token should succeed
	req := httptest.NewRequest("GET", "/protected", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK for GET, got %d", rec.Code)
	}

	// HEAD without CSRF token should succeed
	req = httptest.NewRequest("HEAD", "/protected", nil)
	rec = httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK for HEAD, got %d", rec.Code)
	}
}

func TestSecurityMiddleware_BuildCSP_Empty(t *testing.T) {
	cfg := SecurityConfig{
		EnableCSP:     true,
		CSPDirectives: map[string]string{},
	}
	sm := NewSecurityMiddleware(cfg)

	csp := sm.buildCSP()
	if csp != "default-src 'self'" {
		t.Errorf("expected default CSP, got %s", csp)
	}
}

func TestSecurityMiddleware_BuildCSP_Custom(t *testing.T) {
	cfg := SecurityConfig{
		EnableCSP: true,
		CSPDirectives: map[string]string{
			"default-src": "'self'",
			"script-src":  "'self' https://cdn.example.com",
		},
	}
	sm := NewSecurityMiddleware(cfg)

	csp := sm.buildCSP()
	if !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("expected default-src in CSP, got %s", csp)
	}
	if !strings.Contains(csp, "script-src") {
		t.Errorf("expected script-src in CSP, got %s", csp)
	}
}

func TestSecurityMiddleware_CSRFDisabled(t *testing.T) {
	cfg := DefaultSecurityConfig()
	cfg.EnableCSRF = false
	sm := NewSecurityMiddleware(cfg)

	handler := sm.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// POST without CSRF token should succeed when CSRF is disabled
	req := httptest.NewRequest("POST", "/protected", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK when CSRF disabled, got %d", rec.Code)
	}
}
