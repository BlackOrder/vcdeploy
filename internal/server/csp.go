// Package server provides HTTP server components for vcdeploy.
package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
)

// CSP nonce context key type
type cspNonceKeyType struct{}

var cspNonceKey = cspNonceKeyType{}

// CSPConfig defines Content Security Policy configuration with nonce support.
type CSPConfig struct {
	// EnableNonces enables per-request nonce generation for scripts and styles
	EnableNonces bool
	// NonceLength is the length of the random nonce in bytes (default 16)
	NonceLength int
	// Directives are the CSP directives to apply
	Directives map[string]string
	// ReportURI is the endpoint to report CSP violations (optional)
	ReportURI string
	// ReportOnly sends CSP as report-only header (for testing)
	ReportOnly bool
}

// DefaultCSPConfig returns a secure default CSP configuration.
func DefaultCSPConfig() CSPConfig {
	return CSPConfig{
		EnableNonces: false, // Disabled by default for backward compatibility
		NonceLength:  16,
		Directives: map[string]string{
			"default-src": "'self'",
			"script-src":  "'self'",
			"style-src":   "'self'",
			"img-src":     "'self' data:",
			"font-src":    "'self'",
			"connect-src": "'self'",
			"frame-src":   "'none'",
			"object-src":  "'none'",
			"base-uri":    "'self'",
			"form-action": "'self'",
		},
	}
}

// DefaultCSPConfigWithUnsafeInline returns CSP config allowing inline scripts/styles.
// This is less secure but required when templates use inline handlers.
func DefaultCSPConfigWithUnsafeInline() CSPConfig {
	cfg := DefaultCSPConfig()
	cfg.Directives["script-src"] = "'self' 'unsafe-inline' 'unsafe-eval' https://unpkg.com https://cdn.tailwindcss.com"
	cfg.Directives["style-src"] = "'self' 'unsafe-inline'"
	return cfg
}

// CSPMiddleware applies Content Security Policy headers with optional nonce support.
type CSPMiddleware struct {
	config CSPConfig
}

// NewCSPMiddleware creates a new CSP middleware.
func NewCSPMiddleware(config CSPConfig) *CSPMiddleware {
	if config.NonceLength == 0 {
		config.NonceLength = 16
	}
	return &CSPMiddleware{config: config}
}

// Handler wraps an http.Handler with CSP headers.
func (m *CSPMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var nonce string

		if m.config.EnableNonces {
			var err error
			nonce, err = GenerateCSPNonce(m.config.NonceLength)
			if err != nil {
				WriteJSONError(w, http.StatusInternalServerError, "internal server error")
				return
			}
			// Store nonce in context for template use
			r = r.WithContext(context.WithValue(r.Context(), cspNonceKey, nonce))
		}

		// Build CSP header
		csp := m.buildCSPHeader(nonce)

		// Set appropriate header
		headerName := "Content-Security-Policy"
		if m.config.ReportOnly {
			headerName = "Content-Security-Policy-Report-Only"
		}
		w.Header().Set(headerName, csp)

		next.ServeHTTP(w, r)
	})
}

// buildCSPHeader constructs the CSP header value.
func (m *CSPMiddleware) buildCSPHeader(nonce string) string {
	directives := make([]string, 0, len(m.config.Directives)+1)

	for directive, value := range m.config.Directives {
		finalValue := value

		// Inject nonce into script-src and style-src if enabled
		if nonce != "" && (directive == "script-src" || directive == "style-src") {
			nonceValue := fmt.Sprintf("'nonce-%s'", nonce)
			// Add nonce alongside existing values, removing unsafe-inline if present
			finalValue = strings.Replace(finalValue, "'unsafe-inline'", nonceValue, 1)
			if !strings.Contains(finalValue, nonceValue) {
				finalValue = finalValue + " " + nonceValue
			}
		}

		directives = append(directives, fmt.Sprintf("%s %s", directive, finalValue))
	}

	// Add report-uri if configured
	if m.config.ReportURI != "" {
		directives = append(directives, fmt.Sprintf("report-uri %s", m.config.ReportURI))
	}

	return strings.Join(directives, "; ")
}

// GenerateCSPNonce generates a cryptographically secure random nonce.
func GenerateCSPNonce(length int) (string, error) {
	if length <= 0 {
		length = 16
	}

	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}

	return base64.StdEncoding.EncodeToString(bytes), nil
}

// GetCSPNonce retrieves the CSP nonce from the request context.
// Returns empty string if no nonce was generated for this request.
func GetCSPNonce(r *http.Request) string {
	nonce, _ := r.Context().Value(cspNonceKey).(string)
	return nonce
}

// GetCSPNonceFromContext retrieves the CSP nonce from a context.
// Returns empty string if no nonce was generated.
func GetCSPNonceFromContext(ctx context.Context) string {
	nonce, _ := ctx.Value(cspNonceKey).(string)
	return nonce
}

// NonceScriptTag returns an HTML script tag with the nonce attribute.
// Use this in templates to inject scripts with the proper nonce.
func NonceScriptTag(nonce string) string {
	if nonce == "" {
		return "<script>"
	}
	//nolint:gocritic // Using explicit quotes for HTML attribute, not %q
	return fmt.Sprintf(`<script nonce="%s">`, nonce)
}

// NonceStyleTag returns an HTML style tag with the nonce attribute.
// Use this in templates to inject styles with the proper nonce.
func NonceStyleTag(nonce string) string {
	if nonce == "" {
		return "<style>"
	}
	//nolint:gocritic // Using explicit quotes for HTML attribute, not %q
	return fmt.Sprintf(`<style nonce="%s">`, nonce)
}

// TemplateData represents common template data including CSP nonce.
type TemplateData struct {
	CSPNonce string
	Data     interface{}
}

// WrapTemplateData wraps arbitrary data with CSP nonce for template rendering.
func WrapTemplateData(r *http.Request, data interface{}) TemplateData {
	return TemplateData{
		CSPNonce: GetCSPNonce(r),
		Data:     data,
	}
}
