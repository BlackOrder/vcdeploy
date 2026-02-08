package storage

import (
	"context"
	"github.com/rs/xid"
	"time"
)

// --- BlockedIP methods ---

// BlockIP blocks an IP address.
func (s *MemoryStore) BlockIP(ctx context.Context, block *BlockedIP) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already blocked
	if _, exists := s.blockedIPs[block.IPAddress]; exists {
		return ErrDuplicate
	}

	if block.ID == "" {
		block.ID = xid.New().String()
	}
	if block.BlockedAt.IsZero() {
		block.BlockedAt = time.Now()
	}

	// Copy-on-store
	stored := *block
	s.blockedIPs[block.IPAddress] = &stored

	s.queueWrite(s.ratelimitWrites, NewWriteOp(WriteOpInsert, "blocked_ips", &stored))
	return nil
}

// UnblockIP removes an IP from the block list.
func (s *MemoryStore) UnblockIP(ctx context.Context, ip string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.blockedIPs[ip]; !ok {
		return ErrNotFound
	}

	delete(s.blockedIPs, ip)
	s.queueWrite(s.ratelimitWrites, NewWriteOp(WriteOpDelete, "blocked_ips", ip))
	return nil
}

// IsIPBlocked checks if an IP is currently blocked.
func (s *MemoryStore) IsIPBlocked(ctx context.Context, ip string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	block, ok := s.blockedIPs[ip]
	if !ok {
		return false, nil
	}

	// Check if block has expired
	if !block.ExpiresAt.IsZero() && time.Now().After(block.ExpiresAt) {
		return false, nil
	}

	return true, nil
}

// GetBlockedIP retrieves blocked IP info.
func (s *MemoryStore) GetBlockedIP(ctx context.Context, ip string) (*BlockedIP, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	block, ok := s.blockedIPs[ip]
	if !ok {
		return nil, ErrNotFound
	}

	result := *block
	return &result, nil
}

// ListBlockedIPs returns blocked IPs with pagination.
func (s *MemoryStore) ListBlockedIPs(ctx context.Context, limit, offset int) ([]*BlockedIP, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Collect all blocked IPs
	all := make([]*BlockedIP, 0, len(s.blockedIPs))
	for _, b := range s.blockedIPs {
		cp := *b
		all = append(all, &cp)
	}

	// Sort by BlockedAt descending (newest first)
	for i := 0; i < len(all)-1; i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].BlockedAt.After(all[i].BlockedAt) {
				all[i], all[j] = all[j], all[i]
			}
		}
	}

	total := int64(len(all))

	// Apply pagination
	if offset >= len(all) {
		return []*BlockedIP{}, total, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}

	return all[offset:end], total, nil
}

// --- RateLimit methods ---

// RecordRateLimitRequest records a rate limit request.
func (s *MemoryStore) RecordRateLimitRequest(ctx context.Context, key, bucket string, windowStart, windowEnd time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := rateLimitKey(key, bucket)
	existing, ok := s.rateLimits[k]

	if ok && existing.WindowStart.Equal(windowStart) {
		// Same window, increment count
		existing.Count++
		s.queueWrite(s.ratelimitWrites, NewWriteOp(WriteOpUpdate, "rate_limits", existing))
		return nil
	}

	// New window or different key
	record := &RateLimitRecord{
		ID:          xid.New().String(),
		Key:         key,
		Bucket:      bucket,
		Count:       1,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	}

	s.rateLimits[k] = record
	s.queueWrite(s.ratelimitWrites, NewWriteOp(WriteOpInsert, "rate_limits", record))
	return nil
}

// GetRateLimitCount returns the count of requests for a key/bucket since a given time.
func (s *MemoryStore) GetRateLimitCount(ctx context.Context, key, bucket string, since time.Time) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	k := rateLimitKey(key, bucket)
	record, ok := s.rateLimits[k]
	if !ok {
		return 0, nil
	}

	// Only count if the window includes the 'since' time
	if record.WindowEnd.Before(since) {
		return 0, nil
	}

	return int64(record.Count), nil
}
