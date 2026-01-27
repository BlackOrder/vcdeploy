package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGenerateCSPNonce(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{"default length", 16},
		{"custom length 8", 8},
		{"custom length 32", 32},
		{"zero defaults to 16", 0},
		{"negative defaults to 16", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nonce, err := GenerateCSPNonce(tt.length)
			if err != nil {
				t.Fatalf("GenerateCSPNonce() error = %v", err)
			}
			if nonce == "" {
				t.Error("GenerateCSPNonce() returned empty nonce")
			}

			// Verify base64 encoding (should be decodable)
			if !strings.ContainsAny(nonce, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/=") {
				t.Error("GenerateCSPNonce() nonce doesn't appear to be base64 encoded")
			}
		})
	}
}

func TestGenerateCSPNonceUniqueness(t *testing.T) {
	// Generate multiple nonces and verify they're unique
	nonces := make(map[string]bool)
	for i := 0; i < 100; i++ {
		nonce, err := GenerateCSPNonce(16)
		if err != nil {
			t.Fatalf("GenerateCSPNonce() error = %v", err)
		}
		if nonces[nonce] {
			t.Errorf("GenerateCSPNonce() generated duplicate nonce: %s", nonce)
		}
		nonces[nonce] = true
	}
}

func TestDefaultCSPConfig(t *testing.T) {
	cfg := DefaultCSPConfig()

	if cfg.EnableNonces {
		t.Error("DefaultCSPConfig() should have EnableNonces = false")
	}
	if cfg.NonceLength != 16 {
		t.Errorf("DefaultCSPConfig() NonceLength = %d, want 16", cfg.NonceLength)
	}
	if len(cfg.Directives) == 0 {
		t.Error("DefaultCSPConfig() should have directives")
	}

	// Check key directives
	if _, ok := cfg.Directives["default-src"]; !ok {
		t.Error("DefaultCSPConfig() missing default-src directive")
	}
	if _, ok := cfg.Directives["script-src"]; !ok {
		t.Error("DefaultCSPConfig() missing script-src directive")
	}
}

func TestDefaultCSPConfigWithUnsafeInline(t *testing.T) {
	cfg := DefaultCSPConfigWithUnsafeInline()

	scriptSrc := cfg.Directives["script-src"]
	if !strings.Contains(scriptSrc, "'unsafe-inline'") {
		t.Errorf("DefaultCSPConfigWithUnsafeInline() script-src = %s, want 'unsafe-inline'", scriptSrc)
	}

	styleSrc := cfg.Directives["style-src"]
	if !strings.Contains(styleSrc, "'unsafe-inline'") {
		t.Errorf("DefaultCSPConfigWithUnsafeInline() style-src = %s, want 'unsafe-inline'", styleSrc)
	}
}

func TestCSPMiddlewareWithoutNonces(t *testing.T) {
	cfg := DefaultCSPConfig()
	cfg.EnableNonces = false

	middleware := NewCSPMiddleware(cfg)

	handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify no nonce in context
		nonce := GetCSPNonce(r)
		if nonce != "" {
			t.Errorf("GetCSPNonce() = %s, want empty", nonce)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Error("CSP header not set")
	}
	if strings.Contains(csp, "'nonce-") {
		t.Error("CSP header should not contain nonce when disabled")
	}
}

func TestCSPMiddlewareWithNonces(t *testing.T) {
	cfg := DefaultCSPConfig()
	cfg.EnableNonces = true

	middleware := NewCSPMiddleware(cfg)

	var capturedNonce string
	handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedNonce = GetCSPNonce(r)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Verify nonce was generated and stored in context
	if capturedNonce == "" {
		t.Error("GetCSPNonce() returned empty nonce when nonces enabled")
	}

	// Verify CSP header contains the nonce
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "'nonce-"+capturedNonce+"'") {
		t.Errorf("CSP header doesn't contain nonce: %s", csp)
	}
}

func TestCSPMiddlewareReplacesUnsafeInline(t *testing.T) {
	cfg := DefaultCSPConfigWithUnsafeInline()
	cfg.EnableNonces = true

	middleware := NewCSPMiddleware(cfg)

	var capturedNonce string
	handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedNonce = GetCSPNonce(r)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")

	// Should have nonce
	if !strings.Contains(csp, "'nonce-"+capturedNonce+"'") {
		t.Errorf("CSP header doesn't contain nonce: %s", csp)
	}

	// Should NOT have unsafe-inline (replaced by nonce)
	if strings.Contains(csp, "'unsafe-inline'") {
		t.Errorf("CSP header still contains 'unsafe-inline': %s", csp)
	}
}

func TestCSPMiddlewareReportOnly(t *testing.T) {
	cfg := DefaultCSPConfig()
	cfg.ReportOnly = true

	middleware := NewCSPMiddleware(cfg)

	handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Should use report-only header
	cspReportOnly := rec.Header().Get("Content-Security-Policy-Report-Only")
	csp := rec.Header().Get("Content-Security-Policy")

	if cspReportOnly == "" {
		t.Error("CSP-Report-Only header not set when ReportOnly=true")
	}
	if csp != "" {
		t.Error("CSP header should not be set when ReportOnly=true")
	}
}

func TestCSPMiddlewareWithReportURI(t *testing.T) {
	cfg := DefaultCSPConfig()
	cfg.ReportURI = "/api/csp-report"

	middleware := NewCSPMiddleware(cfg)

	handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "report-uri /api/csp-report") {
		t.Errorf("CSP header doesn't contain report-uri: %s", csp)
	}
}

func TestGetCSPNonceFromContext(t *testing.T) {
	// Test with nonce in context
	ctx := context.WithValue(context.Background(), cspNonceKey, "test-nonce-123")
	nonce := GetCSPNonceFromContext(ctx)
	if nonce != "test-nonce-123" {
		t.Errorf("GetCSPNonceFromContext() = %s, want test-nonce-123", nonce)
	}

	// Test without nonce in context
	nonce = GetCSPNonceFromContext(context.Background())
	if nonce != "" {
		t.Errorf("GetCSPNonceFromContext() = %s, want empty", nonce)
	}
}

func TestNonceScriptTag(t *testing.T) {
	tests := []struct {
		name     string
		nonce    string
		expected string
	}{
		{"with nonce", "abc123", `<script nonce="abc123">`},
		{"empty nonce", "", "<script>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NonceScriptTag(tt.nonce)
			if got != tt.expected {
				t.Errorf("NonceScriptTag() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestNonceStyleTag(t *testing.T) {
	tests := []struct {
		name     string
		nonce    string
		expected string
	}{
		{"with nonce", "xyz789", `<style nonce="xyz789">`},
		{"empty nonce", "", "<style>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NonceStyleTag(tt.nonce)
			if got != tt.expected {
				t.Errorf("NonceStyleTag() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestWrapTemplateData(t *testing.T) {
	// Create a request with nonce in context
	ctx := context.WithValue(context.Background(), cspNonceKey, "template-nonce")
	req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)

	data := map[string]string{"key": "value"}
	wrapped := WrapTemplateData(req, data)

	if wrapped.CSPNonce != "template-nonce" {
		t.Errorf("WrapTemplateData() CSPNonce = %s, want template-nonce", wrapped.CSPNonce)
	}
	if wrapped.Data == nil {
		t.Error("WrapTemplateData() Data = nil")
	}
}

func TestCSPMiddlewareUniqueNoncesPerRequest(t *testing.T) {
	cfg := DefaultCSPConfig()
	cfg.EnableNonces = true

	middleware := NewCSPMiddleware(cfg)

	nonces := make(map[string]bool)

	handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce := GetCSPNonce(r)
		if nonces[nonce] {
			t.Errorf("Duplicate nonce generated: %s", nonce)
		}
		nonces[nonce] = true
		w.WriteHeader(http.StatusOK)
	}))

	// Make multiple requests
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	if len(nonces) != 10 {
		t.Errorf("Expected 10 unique nonces, got %d", len(nonces))
	}
}
