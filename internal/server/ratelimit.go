// Package server provides HTTP and gRPC servers for vcdeploy.
package server

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// RateLimiter provides rate limiting functionality with IP blocking support.
type RateLimiter struct {
	db     *sql.DB
	config RateLimitConfig

	mu      sync.RWMutex
	buckets map[string]*tokenBucket
	blocked map[string]time.Time

	// Cleanup ticker
	stopCh   chan struct{}
	stopOnce sync.Once
}

// RateLimitConfig configures the rate limiter.
type RateLimitConfig struct {
	// RequestsPerSecond is the sustained rate limit
	RequestsPerSecond float64

	// BurstSize is the maximum burst size
	BurstSize int

	// BlockDuration is how long to block IPs after exceeding limits
	BlockDuration time.Duration

	// BlockThreshold is the number of rate limit violations before blocking
	BlockThreshold int

	// WhitelistedIPs are exempt from rate limiting
	WhitelistedIPs []string

	// BlacklistedIPs are always blocked
	BlacklistedIPs []string

	// UseDatabase enables persistent storage of blocked IPs
	UseDatabase bool

	// CleanupInterval is how often to clean up expired entries
	CleanupInterval time.Duration

	// PerEndpointLimits allows different limits per endpoint
	PerEndpointLimits map[string]EndpointLimit
}

// EndpointLimit defines rate limits for a specific endpoint.
type EndpointLimit struct {
	RequestsPerSecond float64
	BurstSize         int
}

// tokenBucket implements a token bucket rate limiter.
type tokenBucket struct {
	tokens     float64
	lastUpdate time.Time
	violations int
}

// RateLimitStatus represents the rate limit status for an IP.
type RateLimitStatus struct {
	IP              string    `json:"ip"`
	TokensRemaining float64   `json:"tokens_remaining"`
	RequestsPerSec  float64   `json:"requests_per_sec"`
	BurstSize       int       `json:"burst_size"`
	Violations      int       `json:"violations"`
	IsBlocked       bool      `json:"is_blocked"`
	BlockedUntil    time.Time `json:"blocked_until,omitempty"`
	IsWhitelisted   bool      `json:"is_whitelisted"`
	IsBlacklisted   bool      `json:"is_blacklisted"`
}

// DefaultRateLimitConfig returns a sensible default configuration.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		RequestsPerSecond: 10,
		BurstSize:         20,
		BlockDuration:     15 * time.Minute,
		BlockThreshold:    10,
		CleanupInterval:   5 * time.Minute,
		PerEndpointLimits: map[string]EndpointLimit{
			"/api/v1/login": {
				RequestsPerSecond: 1, // Strict limit for login
				BurstSize:         5,
			},
			"/api/v1/register": {
				RequestsPerSecond: 0.1, // Very strict for registration
				BurstSize:         2,
			},
		},
	}
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(db *sql.DB, config RateLimitConfig) (*RateLimiter, error) {
	if config.RequestsPerSecond <= 0 {
		config.RequestsPerSecond = 10
	}
	if config.BurstSize <= 0 {
		config.BurstSize = 20
	}
	if config.BlockDuration <= 0 {
		config.BlockDuration = 15 * time.Minute
	}
	if config.BlockThreshold <= 0 {
		config.BlockThreshold = 10
	}
	if config.CleanupInterval <= 0 {
		config.CleanupInterval = 5 * time.Minute
	}

	rl := &RateLimiter{
		db:      db,
		config:  config,
		buckets: make(map[string]*tokenBucket),
		blocked: make(map[string]time.Time),
		stopCh:  make(chan struct{}),
	}

	// Load blocked IPs from database if enabled
	if config.UseDatabase && db != nil {
		if err := rl.loadBlockedIPs(context.Background()); err != nil {
			return nil, fmt.Errorf("load blocked IPs: %w", err)
		}
	}

	// Start cleanup goroutine
	go rl.cleanupLoop()

	return rl, nil
}

// Middleware returns an HTTP middleware that applies rate limiting.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := rl.extractIP(r)
		endpoint := r.URL.Path

		// Check if allowed
		allowed, status := rl.Allow(ip, endpoint)

		// Set rate limit headers
		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%.0f", status.RequestsPerSec))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%.0f", status.TokensRemaining))

		if !allowed {
			if status.IsBlocked {
				retryAfter := int(time.Until(status.BlockedUntil).Seconds())
				if retryAfter < 0 {
					retryAfter = 0
				}
				w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
				http.Error(w, "IP address blocked", http.StatusForbidden)
			} else {
				w.Header().Set("Retry-After", "1")
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			}
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Allow checks if a request from the given IP to the given endpoint is allowed.
func (rl *RateLimiter) Allow(ip, endpoint string) (bool, RateLimitStatus) {
	status := RateLimitStatus{
		IP:             ip,
		RequestsPerSec: rl.config.RequestsPerSecond,
		BurstSize:      rl.config.BurstSize,
	}

	// Check whitelist
	if rl.isWhitelisted(ip) {
		status.IsWhitelisted = true
		status.TokensRemaining = float64(status.BurstSize)
		return true, status
	}

	// Check blacklist
	if rl.isBlacklisted(ip) {
		status.IsBlacklisted = true
		status.IsBlocked = true
		return false, status
	}

	// Check if IP is blocked
	rl.mu.RLock()
	blockedUntil, isBlocked := rl.blocked[ip]
	rl.mu.RUnlock()

	if isBlocked {
		if time.Now().Before(blockedUntil) {
			status.IsBlocked = true
			status.BlockedUntil = blockedUntil
			return false, status
		}
		// Block expired, remove it
		rl.mu.Lock()
		delete(rl.blocked, ip)
		rl.mu.Unlock()
	}

	// Get endpoint-specific limits
	rps := rl.config.RequestsPerSecond
	burst := rl.config.BurstSize
	if limit, ok := rl.config.PerEndpointLimits[endpoint]; ok {
		rps = limit.RequestsPerSecond
		burst = limit.BurstSize
		status.RequestsPerSec = rps
		status.BurstSize = burst
	}

	// Apply token bucket algorithm
	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket, ok := rl.buckets[ip]
	if !ok {
		bucket = &tokenBucket{
			tokens:     float64(burst),
			lastUpdate: time.Now(),
		}
		rl.buckets[ip] = bucket
	}

	// Refill tokens based on time elapsed
	now := time.Now()
	elapsed := now.Sub(bucket.lastUpdate).Seconds()
	bucket.tokens += elapsed * rps
	if bucket.tokens > float64(burst) {
		bucket.tokens = float64(burst)
	}
	bucket.lastUpdate = now

	status.TokensRemaining = bucket.tokens
	status.Violations = bucket.violations

	// Check if we have a token
	if bucket.tokens < 1 {
		bucket.violations++

		// Check if we should block this IP
		if bucket.violations >= rl.config.BlockThreshold {
			blockedUntil := now.Add(rl.config.BlockDuration)
			rl.blocked[ip] = blockedUntil
			status.IsBlocked = true
			status.BlockedUntil = blockedUntil

			// Persist to database if enabled
			if rl.config.UseDatabase && rl.db != nil {
				go func() { _ = rl.saveBlockedIP(context.Background(), ip, blockedUntil, "rate_limit_exceeded") }()
			}
		}

		return false, status
	}

	// Consume a token
	bucket.tokens--
	status.TokensRemaining = bucket.tokens

	return true, status
}

// BlockIP blocks an IP address for the specified duration with a reason.
func (rl *RateLimiter) BlockIP(ctx context.Context, ip, reason string, duration time.Duration) error {
	blockedUntil := time.Now().Add(duration)

	rl.mu.Lock()
	rl.blocked[ip] = blockedUntil
	rl.mu.Unlock()

	if rl.config.UseDatabase && rl.db != nil {
		return rl.saveBlockedIP(ctx, ip, blockedUntil, reason)
	}

	return nil
}

// UnblockIP removes an IP from the block list.
func (rl *RateLimiter) UnblockIP(ctx context.Context, ip string) error {
	rl.mu.Lock()
	delete(rl.blocked, ip)
	rl.mu.Unlock()

	if rl.config.UseDatabase && rl.db != nil {
		_, err := rl.db.ExecContext(ctx, `DELETE FROM blocked_ips WHERE ip_address = ?`, ip)
		return err
	}

	return nil
}

// GetStatus returns the rate limit status for an IP.
func (rl *RateLimiter) GetStatus(ip string) RateLimitStatus {
	status := RateLimitStatus{
		IP:             ip,
		RequestsPerSec: rl.config.RequestsPerSecond,
		BurstSize:      rl.config.BurstSize,
		IsWhitelisted:  rl.isWhitelisted(ip),
		IsBlacklisted:  rl.isBlacklisted(ip),
	}

	rl.mu.RLock()
	defer rl.mu.RUnlock()

	if blockedUntil, ok := rl.blocked[ip]; ok {
		if time.Now().Before(blockedUntil) {
			status.IsBlocked = true
			status.BlockedUntil = blockedUntil
		}
	}

	if bucket, ok := rl.buckets[ip]; ok {
		// Calculate current tokens
		elapsed := time.Since(bucket.lastUpdate).Seconds()
		tokens := bucket.tokens + elapsed*rl.config.RequestsPerSecond
		if tokens > float64(rl.config.BurstSize) {
			tokens = float64(rl.config.BurstSize)
		}
		status.TokensRemaining = tokens
		status.Violations = bucket.violations
	} else {
		status.TokensRemaining = float64(rl.config.BurstSize)
	}

	return status
}

// ListBlockedIPs returns all currently blocked IPs.
func (rl *RateLimiter) ListBlockedIPs(ctx context.Context) ([]BlockedIP, error) {
	if rl.config.UseDatabase && rl.db != nil {
		return rl.loadBlockedIPsList(ctx)
	}

	rl.mu.RLock()
	defer rl.mu.RUnlock()

	var blocked []BlockedIP
	now := time.Now()
	for ip, until := range rl.blocked {
		if now.Before(until) {
			blocked = append(blocked, BlockedIP{
				IPAddress: ip,
				BlockedAt: until.Add(-rl.config.BlockDuration),
				ExpiresAt: until,
				Reason:    "rate_limit_exceeded",
			})
		}
	}

	return blocked, nil
}

// BlockedIP represents a blocked IP address.
type BlockedIP struct {
	ID          int64     `json:"id"`
	IPAddress   string    `json:"ip_address"`
	BlockedAt   time.Time `json:"blocked_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	Reason      string    `json:"reason"`
	ManualBlock bool      `json:"manual_block"`
}

// Stop stops the rate limiter's background goroutines.
// Stop is idempotent and can be called multiple times safely.
func (rl *RateLimiter) Stop() {
	rl.stopOnce.Do(func() {
		close(rl.stopCh)
	})
}

// --- Private methods ---

func (rl *RateLimiter) extractIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For can contain multiple IPs; the first is the client
		if idx := len(xff) - 1; idx >= 0 {
			for i, c := range xff {
				if c == ',' {
					return xff[:i]
				}
			}
			return xff
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func (rl *RateLimiter) isWhitelisted(ip string) bool {
	for _, wip := range rl.config.WhitelistedIPs {
		if wip == ip {
			return true
		}
		// Check CIDR notation
		if _, network, err := net.ParseCIDR(wip); err == nil {
			if network.Contains(net.ParseIP(ip)) {
				return true
			}
		}
	}
	return false
}

func (rl *RateLimiter) isBlacklisted(ip string) bool {
	for _, bip := range rl.config.BlacklistedIPs {
		if bip == ip {
			return true
		}
		// Check CIDR notation
		if _, network, err := net.ParseCIDR(bip); err == nil {
			if network.Contains(net.ParseIP(ip)) {
				return true
			}
		}
	}
	return false
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rl.stopCh:
			return
		case <-ticker.C:
			rl.cleanup()
		}
	}
}

func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Clean up expired blocks
	for ip, until := range rl.blocked {
		if now.After(until) {
			delete(rl.blocked, ip)
		}
	}

	// Clean up stale buckets (no activity for > 1 hour)
	staleThreshold := now.Add(-1 * time.Hour)
	for ip, bucket := range rl.buckets {
		if bucket.lastUpdate.Before(staleThreshold) {
			delete(rl.buckets, ip)
		}
	}
}

func (rl *RateLimiter) loadBlockedIPs(ctx context.Context) error {
	rows, err := rl.db.QueryContext(ctx, `
		SELECT ip_address, expires_at FROM blocked_ips WHERE expires_at > ?
	`, time.Now())
	if err != nil {
		return err
	}
	defer rows.Close()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	for rows.Next() {
		var ip string
		var expiresAt time.Time
		if err := rows.Scan(&ip, &expiresAt); err != nil {
			return err
		}
		rl.blocked[ip] = expiresAt
	}

	return rows.Err()
}

func (rl *RateLimiter) loadBlockedIPsList(ctx context.Context) ([]BlockedIP, error) {
	rows, err := rl.db.QueryContext(ctx, `
		SELECT id, ip_address, blocked_at, expires_at, reason, manual_block
		FROM blocked_ips WHERE expires_at > ?
		ORDER BY blocked_at DESC
	`, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blocked []BlockedIP
	for rows.Next() {
		var b BlockedIP
		if err := rows.Scan(&b.ID, &b.IPAddress, &b.BlockedAt, &b.ExpiresAt, &b.Reason, &b.ManualBlock); err != nil {
			return nil, err
		}
		blocked = append(blocked, b)
	}

	return blocked, rows.Err()
}

func (rl *RateLimiter) saveBlockedIP(ctx context.Context, ip string, expiresAt time.Time, reason string) error {
	_, err := rl.db.ExecContext(ctx, `
		INSERT INTO blocked_ips (ip_address, blocked_at, expires_at, reason, manual_block)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(ip_address) DO UPDATE SET
			expires_at = excluded.expires_at,
			reason = excluded.reason
	`, ip, time.Now(), expiresAt, reason, false)
	return err
}

// --- Rate Limit Store for Per-User Limits ---

// UserRateLimiter provides per-user rate limiting.
type UserRateLimiter struct {
	rl     *RateLimiter
	prefix string
}

// NewUserRateLimiter creates a user-specific rate limiter.
func NewUserRateLimiter(rl *RateLimiter, userID string) *UserRateLimiter {
	return &UserRateLimiter{
		rl:     rl,
		prefix: "user:" + userID + ":",
	}
}

// Allow checks if a request is allowed for this user.
func (url *UserRateLimiter) Allow(endpoint string) (bool, RateLimitStatus) {
	key := url.prefix + endpoint
	return url.rl.Allow(key, endpoint)
}

// --- API Key Rate Limiting ---

// APIKeyRateLimiter provides per-API-key rate limiting.
type APIKeyRateLimiter struct {
	rl     *RateLimiter
	prefix string
}

// NewAPIKeyRateLimiter creates an API-key-specific rate limiter.
func NewAPIKeyRateLimiter(rl *RateLimiter, keyHash string) *APIKeyRateLimiter {
	return &APIKeyRateLimiter{
		rl:     rl,
		prefix: "apikey:" + keyHash[:8] + ":",
	}
}

// Allow checks if a request is allowed for this API key.
func (akl *APIKeyRateLimiter) Allow(endpoint string) (bool, RateLimitStatus) {
	key := akl.prefix + endpoint
	return akl.rl.Allow(key, endpoint)
}
