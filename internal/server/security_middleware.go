// Package server provides HTTP and gRPC servers for vcdeploy.
package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/config"
)

// SecurityMiddleware provides security-related HTTP middleware.
type SecurityMiddleware struct {
	config SecurityConfig

	// CSRF token store (in-memory, could be backed by session)
	csrfTokens map[string]csrfToken
	csrfMu     sync.RWMutex

	// stopCh signals the cleanup goroutine to stop
	stopCh   chan struct{}
	stopOnce sync.Once
}

// csrfToken represents a CSRF token with expiry.
type csrfToken struct {
	Token     string
	ExpiresAt time.Time
}

// SecurityConfig configures security middleware.
type SecurityConfig struct {
	// EnableHSTS enables HTTP Strict Transport Security
	EnableHSTS bool
	// HSTSMaxAge is the max-age for HSTS header (default 1 year)
	HSTSMaxAge time.Duration
	// HSTSIncludeSubdomains includes subdomains in HSTS
	HSTSIncludeSubdomains bool

	// EnableCSP enables Content Security Policy
	EnableCSP bool
	// CSPDirectives are custom CSP directives
	CSPDirectives map[string]string

	// EnableXFrameOptions enables X-Frame-Options header
	EnableXFrameOptions bool
	// XFrameOptions is the value (DENY, SAMEORIGIN, or ALLOW-FROM uri)
	XFrameOptions string

	// EnableXContentTypeOptions enables X-Content-Type-Options: nosniff
	EnableXContentTypeOptions bool

	// EnableXXSSProtection enables X-XSS-Protection header
	EnableXXSSProtection bool

	// EnableReferrerPolicy enables Referrer-Policy header
	EnableReferrerPolicy bool
	// ReferrerPolicy is the policy value
	ReferrerPolicy string

	// EnableCSRF enables CSRF protection
	EnableCSRF bool
	// CSRFTokenLength is the length of CSRF tokens (default 32)
	CSRFTokenLength int
	// CSRFTokenExpiry is how long tokens are valid (default 1 hour)
	CSRFTokenExpiry time.Duration
	// CSRFSafeMethods are methods exempt from CSRF checks
	CSRFSafeMethods []string
	// CSRFExemptPaths are paths exempt from CSRF checks
	CSRFExemptPaths []string
}

// DefaultSecurityConfig returns sensible security defaults.
func DefaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		EnableHSTS:                true,
		HSTSMaxAge:                365 * 24 * time.Hour,
		HSTSIncludeSubdomains:     true,
		EnableCSP:                 true,
		CSPDirectives:             defaultCSPDirectives(),
		EnableXFrameOptions:       true,
		XFrameOptions:             "SAMEORIGIN",
		EnableXContentTypeOptions: true,
		EnableXXSSProtection:      true,
		EnableReferrerPolicy:      true,
		ReferrerPolicy:            "strict-origin-when-cross-origin",
		EnableCSRF:                true,
		CSRFTokenLength:           config.DefaultCSRFTokenBytes,
		CSRFTokenExpiry:           1 * time.Hour,
		CSRFSafeMethods:           []string{"GET", "HEAD", "OPTIONS"},
		CSRFExemptPaths: []string{
			"/webhook/github/",
			"/webhook/gitlab/",
			"/webhook/bitbucket/",
			"/api/v1/agents/", // Agent API uses bearer tokens
		},
	}
}

// defaultCSPDirectives returns default Content Security Policy directives.
// Note: 'unsafe-inline' is required for script-src and style-src because the dashboard
// templates use inline event handlers and style attributes. A future enhancement would
// be to migrate to CSP nonces (script-src 'nonce-xxx') for stronger XSS protection,
// which requires generating unique nonces per request and injecting them into templates.
func defaultCSPDirectives() map[string]string {
	return map[string]string{
		"default-src": "'self'",
		"script-src":  "'self' 'unsafe-inline' 'unsafe-eval' https://unpkg.com https://cdn.tailwindcss.com", // CDN scripts for htmx and tailwind
		"style-src":   "'self' 'unsafe-inline'",                                                             // Required for template inline styles; consider nonce migration
		"img-src":     "'self' data:",                                                                       // Allow data URIs for images
		"font-src":    "'self'",
		"connect-src": "'self'",
		"frame-src":   "'none'",
		"object-src":  "'none'",
		"base-uri":    "'self'",
		"form-action": "'self'",
	}
}

// NewSecurityMiddleware creates a new security middleware.
func NewSecurityMiddleware(cfg SecurityConfig) *SecurityMiddleware {
	if cfg.CSRFTokenLength == 0 {
		cfg.CSRFTokenLength = config.DefaultCSRFTokenBytes
	}
	if cfg.CSRFTokenExpiry == 0 {
		cfg.CSRFTokenExpiry = 1 * time.Hour
	}
	if len(cfg.CSRFSafeMethods) == 0 {
		cfg.CSRFSafeMethods = []string{"GET", "HEAD", "OPTIONS"}
	}

	sm := &SecurityMiddleware{
		config:     cfg,
		csrfTokens: make(map[string]csrfToken),
		stopCh:     make(chan struct{}),
	}

	// Start cleanup goroutine
	go sm.cleanupExpiredTokens()

	return sm
}

// Middleware returns HTTP middleware that adds security headers.
func (sm *SecurityMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add security headers
		sm.addSecurityHeaders(w, r)

		// CSRF protection for non-safe methods
		if sm.config.EnableCSRF && !sm.isSafeMethod(r.Method) && !sm.isExemptPath(r.URL.Path) {
			if !sm.validateCSRFToken(r) {
				WriteJSONError(w, http.StatusForbidden, "csrf token validation failed")
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// HeadersOnlyMiddleware returns middleware that only adds headers, no CSRF.
func (sm *SecurityMiddleware) HeadersOnlyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sm.addSecurityHeaders(w, r)
		next.ServeHTTP(w, r)
	})
}

func (sm *SecurityMiddleware) addSecurityHeaders(w http.ResponseWriter, r *http.Request) {
	// HSTS
	if sm.config.EnableHSTS && r.TLS != nil {
		hstsValue := fmt.Sprintf("max-age=%d", int(sm.config.HSTSMaxAge.Seconds()))
		if sm.config.HSTSIncludeSubdomains {
			hstsValue += "; includeSubDomains"
		}
		w.Header().Set("Strict-Transport-Security", hstsValue)
	}

	// Content Security Policy
	if sm.config.EnableCSP {
		csp := sm.buildCSP()
		w.Header().Set("Content-Security-Policy", csp)
	}

	// X-Frame-Options
	if sm.config.EnableXFrameOptions {
		w.Header().Set("X-Frame-Options", sm.config.XFrameOptions)
	}

	// X-Content-Type-Options
	if sm.config.EnableXContentTypeOptions {
		w.Header().Set("X-Content-Type-Options", "nosniff")
	}

	// X-XSS-Protection (legacy, but still useful for older browsers)
	if sm.config.EnableXXSSProtection {
		w.Header().Set("X-XSS-Protection", "1; mode=block")
	}

	// Referrer-Policy
	if sm.config.EnableReferrerPolicy {
		w.Header().Set("Referrer-Policy", sm.config.ReferrerPolicy)
	}

	// Permissions-Policy (formerly Feature-Policy)
	w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
}

func (sm *SecurityMiddleware) buildCSP() string {
	if len(sm.config.CSPDirectives) == 0 {
		return "default-src 'self'"
	}

	var parts []string
	for directive, value := range sm.config.CSPDirectives {
		parts = append(parts, fmt.Sprintf("%s %s", directive, value))
	}
	return strings.Join(parts, "; ")
}

func (sm *SecurityMiddleware) isSafeMethod(method string) bool {
	for _, safe := range sm.config.CSRFSafeMethods {
		if method == safe {
			return true
		}
	}
	return false
}

func (sm *SecurityMiddleware) isExemptPath(path string) bool {
	for _, exempt := range sm.config.CSRFExemptPaths {
		if strings.HasPrefix(path, exempt) {
			return true
		}
	}
	return false
}

// GenerateCSRFToken generates a new CSRF token for a session.
// Returns an error if secure random generation fails.
func (sm *SecurityMiddleware) GenerateCSRFToken(sessionID string) (string, error) {
	token := make([]byte, sm.config.CSRFTokenLength)
	if _, err := rand.Read(token); err != nil {
		return "", fmt.Errorf("generating CSRF token: %w", err)
	}

	tokenStr := base64.URLEncoding.EncodeToString(token)

	sm.csrfMu.Lock()
	sm.csrfTokens[sessionID] = csrfToken{
		Token:     tokenStr,
		ExpiresAt: time.Now().Add(sm.config.CSRFTokenExpiry),
	}
	sm.csrfMu.Unlock()

	return tokenStr, nil
}

// validateCSRFToken validates the CSRF token from the request.
func (sm *SecurityMiddleware) validateCSRFToken(r *http.Request) bool {
	// Get token from header or form
	token := r.Header.Get("X-CSRF-Token")
	if token == "" {
		token = r.FormValue("_csrf_token")
	}
	if token == "" {
		return false
	}

	// Get session ID from cookie
	sessionCookie, err := r.Cookie("session_id")
	if err != nil {
		return false
	}
	sessionID := sessionCookie.Value

	// Validate token
	sm.csrfMu.RLock()
	storedToken, exists := sm.csrfTokens[sessionID]
	sm.csrfMu.RUnlock()

	if !exists {
		return false
	}

	if time.Now().After(storedToken.ExpiresAt) {
		// Token expired
		sm.csrfMu.Lock()
		delete(sm.csrfTokens, sessionID)
		sm.csrfMu.Unlock()
		return false
	}

	// Constant-time comparison
	return subtle.ConstantTimeCompare([]byte(token), []byte(storedToken.Token)) == 1
}

// cleanupExpiredTokens periodically removes expired CSRF tokens.
func (sm *SecurityMiddleware) cleanupExpiredTokens() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-sm.stopCh:
			return
		case <-ticker.C:
			now := time.Now()
			sm.csrfMu.Lock()
			for sessionID, token := range sm.csrfTokens {
				if now.After(token.ExpiresAt) {
					delete(sm.csrfTokens, sessionID)
				}
			}
			sm.csrfMu.Unlock()
		}
	}
}

// Stop stops the cleanup goroutine. Call this on server shutdown.
// Stop is idempotent and can be called multiple times safely.
func (sm *SecurityMiddleware) Stop() {
	sm.stopOnce.Do(func() {
		close(sm.stopCh)
	})
}

// GetCSRFToken returns the current CSRF token for a session.
// If no valid token exists, generates a new one.
func (sm *SecurityMiddleware) GetCSRFToken(sessionID string) (string, error) {
	sm.csrfMu.RLock()
	token, exists := sm.csrfTokens[sessionID]
	sm.csrfMu.RUnlock()

	if !exists || time.Now().After(token.ExpiresAt) {
		return sm.GenerateCSRFToken(sessionID)
	}
	return token.Token, nil
}
