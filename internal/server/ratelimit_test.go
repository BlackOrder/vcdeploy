package server

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupRateLimitTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}

	// Create required tables
	schema := `
		CREATE TABLE IF NOT EXISTS blocked_ips (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ip_address TEXT NOT NULL UNIQUE,
			blocked_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME NOT NULL,
			reason TEXT DEFAULT '',
			manual_block BOOLEAN DEFAULT FALSE
		);
		CREATE INDEX IF NOT EXISTS idx_blocked_ips_expires ON blocked_ips(expires_at);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}

	return db
}

func TestNewRateLimiter(t *testing.T) {
	db := setupRateLimitTestDB(t)
	defer db.Close()

	tests := []struct {
		name   string
		config RateLimitConfig
	}{
		{
			name:   "default config",
			config: DefaultRateLimitConfig(),
		},
		{
			name: "custom config",
			config: RateLimitConfig{
				RequestsPerSecond: 100,
				BurstSize:         200,
				BlockDuration:     time.Hour,
				BlockThreshold:    5,
			},
		},
		{
			name:   "zero values use defaults",
			config: RateLimitConfig{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl, err := NewRateLimiter(db, tt.config)
			if err != nil {
				t.Fatalf("NewRateLimiter() error: %v", err)
			}
			defer rl.Stop()

			if rl == nil {
				t.Error("NewRateLimiter() returned nil")
			}
		})
	}
}

func TestRateLimiterBasicAllow(t *testing.T) {
	rl, err := NewRateLimiter(nil, RateLimitConfig{
		RequestsPerSecond: 10,
		BurstSize:         5,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rl.Stop()

	ip := "192.168.1.1"
	endpoint := "/api/test"

	// First 5 requests should be allowed (burst)
	for i := 0; i < 5; i++ {
		allowed, status := rl.Allow(ip, endpoint)
		if !allowed {
			t.Errorf("Request %d: expected allowed, got denied", i+1)
		}
		if status.TokensRemaining < 0 {
			t.Errorf("Request %d: negative tokens remaining: %f", i+1, status.TokensRemaining)
		}
	}

	// Next request should be denied (burst exhausted)
	allowed, status := rl.Allow(ip, endpoint)
	if allowed {
		t.Error("Expected request to be denied after burst exhausted")
	}
	if status.TokensRemaining >= 1 {
		t.Errorf("Expected less than 1 token, got %f", status.TokensRemaining)
	}
}

func TestRateLimiterTokenRefill(t *testing.T) {
	rl, err := NewRateLimiter(nil, RateLimitConfig{
		RequestsPerSecond: 100, // Fast refill for testing
		BurstSize:         2,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rl.Stop()

	ip := "192.168.1.1"
	endpoint := "/api/test"

	// Exhaust burst
	rl.Allow(ip, endpoint)
	rl.Allow(ip, endpoint)

	// Should be denied
	allowed, _ := rl.Allow(ip, endpoint)
	if allowed {
		t.Error("Expected denied after burst exhausted")
	}

	// Wait for refill (10ms for 1 token at 100 rps)
	time.Sleep(20 * time.Millisecond)

	// Should be allowed now
	allowed, _ = rl.Allow(ip, endpoint)
	if !allowed {
		t.Error("Expected allowed after token refill")
	}
}

func TestRateLimiterBlocking(t *testing.T) {
	rl, err := NewRateLimiter(nil, RateLimitConfig{
		RequestsPerSecond: 1000, // High rate to minimize timing issues
		BurstSize:         1,
		BlockDuration:     100 * time.Millisecond,
		BlockThreshold:    3,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rl.Stop()

	ip := "192.168.1.1"
	endpoint := "/api/test"

	// Use burst token
	rl.Allow(ip, endpoint)

	// Trigger violations until blocked
	for i := 0; i < 3; i++ {
		allowed, status := rl.Allow(ip, endpoint)
		if allowed {
			t.Errorf("Violation %d: expected denied", i+1)
		}
		if i == 2 && !status.IsBlocked {
			t.Error("Expected IP to be blocked after threshold")
		}
	}

	// Verify blocked
	allowed, status := rl.Allow(ip, endpoint)
	if allowed {
		t.Error("Expected request denied when blocked")
	}
	if !status.IsBlocked {
		t.Error("Expected IsBlocked = true")
	}

	// Wait for block to expire
	time.Sleep(150 * time.Millisecond)

	// Should be allowed again
	allowed, status = rl.Allow(ip, endpoint)
	if !allowed {
		t.Error("Expected allowed after block expired")
	}
	if status.IsBlocked {
		t.Error("Expected IsBlocked = false after expiry")
	}
}

func TestRateLimiterWhitelist(t *testing.T) {
	rl, err := NewRateLimiter(nil, RateLimitConfig{
		RequestsPerSecond: 1,
		BurstSize:         1,
		WhitelistedIPs:    []string{"192.168.1.100", "10.0.0.0/8"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rl.Stop()

	endpoint := "/api/test"

	// Exact match whitelist
	for i := 0; i < 100; i++ {
		allowed, status := rl.Allow("192.168.1.100", endpoint)
		if !allowed || !status.IsWhitelisted {
			t.Error("Whitelisted IP should always be allowed")
		}
	}

	// CIDR match whitelist
	for i := 0; i < 100; i++ {
		allowed, status := rl.Allow("10.5.6.7", endpoint)
		if !allowed || !status.IsWhitelisted {
			t.Error("IP in whitelisted CIDR should always be allowed")
		}
	}

	// Non-whitelisted should be limited
	rl.Allow("192.168.1.1", endpoint) // Use burst
	allowed, _ := rl.Allow("192.168.1.1", endpoint)
	if allowed {
		t.Error("Non-whitelisted IP should be rate limited")
	}
}

func TestRateLimiterBlacklist(t *testing.T) {
	rl, err := NewRateLimiter(nil, RateLimitConfig{
		RequestsPerSecond: 100,
		BurstSize:         100,
		BlacklistedIPs:    []string{"192.168.1.200", "172.16.0.0/12"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rl.Stop()

	endpoint := "/api/test"

	// Exact match blacklist
	allowed, status := rl.Allow("192.168.1.200", endpoint)
	if allowed {
		t.Error("Blacklisted IP should be denied")
	}
	if !status.IsBlacklisted || !status.IsBlocked {
		t.Error("Expected IsBlacklisted and IsBlocked = true")
	}

	// CIDR match blacklist
	allowed, status = rl.Allow("172.20.5.5", endpoint)
	if allowed {
		t.Error("IP in blacklisted CIDR should be denied")
	}
	if !status.IsBlacklisted {
		t.Error("Expected IsBlacklisted = true for CIDR match")
	}
}

func TestRateLimiterPerEndpointLimits(t *testing.T) {
	rl, err := NewRateLimiter(nil, RateLimitConfig{
		RequestsPerSecond: 100,
		BurstSize:         100,
		PerEndpointLimits: map[string]EndpointLimit{
			"/api/login": {
				RequestsPerSecond: 1,
				BurstSize:         2,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rl.Stop()

	ip := "192.168.1.1"

	// Normal endpoint uses default limits
	for i := 0; i < 50; i++ {
		allowed, _ := rl.Allow(ip, "/api/normal")
		if !allowed {
			t.Errorf("Normal endpoint request %d denied", i+1)
		}
	}

	// Login endpoint uses strict limits
	rl.Allow(ip, "/api/login")                    // 1
	rl.Allow(ip, "/api/login")                    // 2
	allowed, status := rl.Allow(ip, "/api/login") // 3 - should be denied
	if allowed {
		t.Error("Login endpoint should be rate limited after burst")
	}
	if status.BurstSize != 2 {
		t.Errorf("Expected burst size 2 for login, got %d", status.BurstSize)
	}
}

func TestRateLimiterManualBlock(t *testing.T) {
	db := setupRateLimitTestDB(t)
	defer db.Close()

	rl, err := NewRateLimiter(db, RateLimitConfig{
		UseDatabase: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rl.Stop()

	ctx := context.Background()
	ip := "192.168.1.50"

	// Block IP manually
	err = rl.BlockIP(ctx, ip, "suspicious_activity", time.Hour)
	if err != nil {
		t.Fatalf("BlockIP() error: %v", err)
	}

	// Verify blocked
	allowed, status := rl.Allow(ip, "/api/test")
	if allowed {
		t.Error("Manually blocked IP should be denied")
	}
	if !status.IsBlocked {
		t.Error("Expected IsBlocked = true")
	}

	// Verify in database
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM blocked_ips WHERE ip_address = ?", ip).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("Expected 1 blocked IP in database, got %d", count)
	}

	// Unblock
	err = rl.UnblockIP(ctx, ip)
	if err != nil {
		t.Fatalf("UnblockIP() error: %v", err)
	}

	// Verify unblocked
	allowed, status = rl.Allow(ip, "/api/test")
	if !allowed {
		t.Error("Unblocked IP should be allowed")
	}
	if status.IsBlocked {
		t.Error("Expected IsBlocked = false after unblock")
	}
}

func TestRateLimiterGetStatus(t *testing.T) {
	rl, err := NewRateLimiter(nil, RateLimitConfig{
		RequestsPerSecond: 10,
		BurstSize:         20,
		WhitelistedIPs:    []string{"10.0.0.1"},
		BlacklistedIPs:    []string{"10.0.0.2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rl.Stop()

	// New IP should have full burst
	status := rl.GetStatus("192.168.1.1")
	if status.TokensRemaining != 20 {
		t.Errorf("Expected 20 tokens for new IP, got %f", status.TokensRemaining)
	}

	// Use some tokens
	rl.Allow("192.168.1.1", "/api/test")
	rl.Allow("192.168.1.1", "/api/test")

	status = rl.GetStatus("192.168.1.1")
	if status.TokensRemaining >= 20 {
		t.Errorf("Expected less than 20 tokens after use, got %f", status.TokensRemaining)
	}

	// Whitelisted IP
	status = rl.GetStatus("10.0.0.1")
	if !status.IsWhitelisted {
		t.Error("Expected IsWhitelisted = true")
	}

	// Blacklisted IP
	status = rl.GetStatus("10.0.0.2")
	if !status.IsBlacklisted {
		t.Error("Expected IsBlacklisted = true")
	}
}

func TestRateLimiterMiddleware(t *testing.T) {
	rl, err := NewRateLimiter(nil, RateLimitConfig{
		RequestsPerSecond: 1,
		BurstSize:         2,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rl.Stop()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))

	// First two requests should succeed (burst)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Request %d: expected 200, got %d", i+1, rec.Code)
		}
		if rec.Header().Get("X-RateLimit-Limit") == "" {
			t.Error("Expected X-RateLimit-Limit header")
		}
	}

	// Third request should be rate limited
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("Expected 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("Expected Retry-After header")
	}
}

func TestRateLimiterMiddlewareBlocked(t *testing.T) {
	rl, err := NewRateLimiter(nil, RateLimitConfig{
		BlacklistedIPs: []string{"192.168.1.100"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rl.Stop()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Expected 403 for blocked IP, got %d", rec.Code)
	}
}

func TestRateLimiterExtractIP(t *testing.T) {
	rl, err := NewRateLimiter(nil, DefaultRateLimitConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer rl.Stop()

	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		expected   string
	}{
		{
			name:       "remote addr only",
			remoteAddr: "192.168.1.1:12345",
			expected:   "192.168.1.1",
		},
		{
			name:       "X-Forwarded-For single",
			remoteAddr: "127.0.0.1:12345",
			headers:    map[string]string{"X-Forwarded-For": "10.0.0.1"},
			expected:   "10.0.0.1",
		},
		{
			name:       "X-Forwarded-For multiple",
			remoteAddr: "127.0.0.1:12345",
			headers:    map[string]string{"X-Forwarded-For": "10.0.0.1, 172.16.0.1"},
			expected:   "10.0.0.1",
		},
		{
			name:       "X-Real-IP",
			remoteAddr: "127.0.0.1:12345",
			headers:    map[string]string{"X-Real-IP": "10.0.0.2"},
			expected:   "10.0.0.2",
		},
		{
			name:       "X-Forwarded-For takes precedence",
			remoteAddr: "127.0.0.1:12345",
			headers: map[string]string{
				"X-Forwarded-For": "10.0.0.1",
				"X-Real-IP":       "10.0.0.2",
			},
			expected: "10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			ip := rl.extractIP(req)
			if ip != tt.expected {
				t.Errorf("Expected IP %s, got %s", tt.expected, ip)
			}
		})
	}
}

func TestRateLimiterListBlockedIPs(t *testing.T) {
	db := setupRateLimitTestDB(t)
	defer db.Close()

	rl, err := NewRateLimiter(db, RateLimitConfig{
		UseDatabase:   true,
		BlockDuration: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rl.Stop()

	ctx := context.Background()

	// Block some IPs
	_ = rl.BlockIP(ctx, "192.168.1.1", "reason1", time.Hour)
	_ = rl.BlockIP(ctx, "192.168.1.2", "reason2", time.Hour)

	// List blocked
	blocked, err := rl.ListBlockedIPs(ctx)
	if err != nil {
		t.Fatalf("ListBlockedIPs() error: %v", err)
	}

	if len(blocked) != 2 {
		t.Errorf("Expected 2 blocked IPs, got %d", len(blocked))
	}
}

func TestRateLimiterListBlockedIPs_InMemory(t *testing.T) {
	// Test in-memory mode (UseDatabase=false)
	rl, err := NewRateLimiter(nil, RateLimitConfig{
		UseDatabase:   false,
		BlockDuration: time.Hour,
		BurstSize:     2,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rl.Stop()

	ctx := context.Background()

	// Manually block some IPs using in-memory storage
	rl.mu.Lock()
	rl.blocked["10.0.0.1"] = time.Now().Add(time.Hour)
	rl.blocked["10.0.0.2"] = time.Now().Add(time.Hour)
	rl.blocked["10.0.0.3"] = time.Now().Add(-time.Hour) // expired, should not be returned
	rl.mu.Unlock()

	// List blocked
	blocked, err := rl.ListBlockedIPs(ctx)
	if err != nil {
		t.Fatalf("ListBlockedIPs() error: %v", err)
	}

	if len(blocked) != 2 {
		t.Errorf("Expected 2 blocked IPs (excluding expired), got %d", len(blocked))
	}

	// Verify returned IPs have correct fields
	for _, b := range blocked {
		if b.IPAddress != "10.0.0.1" && b.IPAddress != "10.0.0.2" {
			t.Errorf("Unexpected IP address: %s", b.IPAddress)
		}
		if b.Reason != "rate_limit_exceeded" {
			t.Errorf("Expected reason 'rate_limit_exceeded', got '%s'", b.Reason)
		}
		if b.ExpiresAt.Before(time.Now()) {
			t.Error("Expected ExpiresAt to be in the future")
		}
	}
}

func TestUserRateLimiter(t *testing.T) {
	rl, err := NewRateLimiter(nil, RateLimitConfig{
		RequestsPerSecond: 10,
		BurstSize:         2,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rl.Stop()

	// Create user-specific limiters
	user1 := NewUserRateLimiter(rl, "user1")
	user2 := NewUserRateLimiter(rl, "user2")

	// Each user should have independent limits
	user1.Allow("/api/test")
	user1.Allow("/api/test")
	allowed, _ := user1.Allow("/api/test")
	if allowed {
		t.Error("User1 should be rate limited")
	}

	// User2 should still have tokens
	allowed, _ = user2.Allow("/api/test")
	if !allowed {
		t.Error("User2 should not be affected by User1's limit")
	}
}

func TestAPIKeyRateLimiter(t *testing.T) {
	rl, err := NewRateLimiter(nil, RateLimitConfig{
		RequestsPerSecond: 10,
		BurstSize:         2,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rl.Stop()

	// Create API key-specific limiters
	key1 := NewAPIKeyRateLimiter(rl, "abcd1234efgh5678")
	key2 := NewAPIKeyRateLimiter(rl, "ijkl9012mnop3456")

	// Each API key should have independent limits
	key1.Allow("/api/test")
	key1.Allow("/api/test")
	allowed, _ := key1.Allow("/api/test")
	if allowed {
		t.Error("Key1 should be rate limited")
	}

	// Key2 should still have tokens
	allowed, _ = key2.Allow("/api/test")
	if !allowed {
		t.Error("Key2 should not be affected by Key1's limit")
	}
}

func TestDefaultRateLimitConfig(t *testing.T) {
	config := DefaultRateLimitConfig()

	if config.RequestsPerSecond <= 0 {
		t.Error("RequestsPerSecond should be positive")
	}
	if config.BurstSize <= 0 {
		t.Error("BurstSize should be positive")
	}
	if config.BlockDuration <= 0 {
		t.Error("BlockDuration should be positive")
	}
	if config.BlockThreshold <= 0 {
		t.Error("BlockThreshold should be positive")
	}
	if config.CleanupInterval <= 0 {
		t.Error("CleanupInterval should be positive")
	}

	// Check login endpoint has stricter limits
	loginLimit, ok := config.PerEndpointLimits["/api/v1/login"]
	if !ok {
		t.Error("Expected per-endpoint limit for /api/v1/login")
	}
	if loginLimit.RequestsPerSecond >= config.RequestsPerSecond {
		t.Error("Login endpoint should have stricter rate limit")
	}
}
