// Package ratelimit provides rate limiting and IP blocking services.
package ratelimit

import (
	"context"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/storage"
)

// Service handles rate limiting and IP blocking operations.
type Service struct {
	store storage.Store
}

// Ensure Service implements RateLimitServicer.
var _ services.RateLimitServicer = (*Service)(nil)

// New creates a new rate limit service.
func New(store storage.Store) *Service {
	return &Service{store: store}
}

// BlockIP blocks an IP address for the specified duration.
func (s *Service) BlockIP(ctx context.Context, ip, reason string, duration time.Duration, blockedBy string) error {
	const op = "ratelimit.BlockIP"

	if ip == "" {
		return &services.ServiceError{
			Op:  op,
			Err: services.ErrInvalidInput,
		}
	}

	block := &storage.BlockedIP{
		IPAddress: ip,
		Reason:    reason,
		BlockedAt: time.Now(),
		ExpiresAt: time.Now().Add(duration),
		BlockedBy: blockedBy,
	}

	if err := s.store.BlockIP(ctx, block); err != nil {
		return &services.ServiceError{
			Op:  op,
			Err: err,
		}
	}

	return nil
}

// UnblockIP removes a block on an IP address.
func (s *Service) UnblockIP(ctx context.Context, ip string) error {
	const op = "ratelimit.UnblockIP"

	if err := s.store.UnblockIP(ctx, ip); err != nil {
		return &services.ServiceError{
			Op:  op,
			Err: err,
		}
	}

	return nil
}

// IsBlocked checks if an IP is currently blocked.
func (s *Service) IsBlocked(ctx context.Context, ip string) (bool, error) {
	const op = "ratelimit.IsBlocked"

	blocked, err := s.store.IsIPBlocked(ctx, ip)
	if err != nil {
		return false, &services.ServiceError{
			Op:  op,
			Err: err,
		}
	}

	return blocked, nil
}

// GetBlock retrieves block details for an IP.
func (s *Service) GetBlock(ctx context.Context, ip string) (*storage.BlockedIP, error) {
	const op = "ratelimit.GetBlock"

	block, err := s.store.GetBlockedIP(ctx, ip)
	if err != nil {
		if services.IsNotFound(err) {
			return nil, &services.ServiceError{
				Op:       op,
				Err:      services.ErrNotFound,
				Resource: "blocked_ip",
				ID:       ip,
			}
		}
		return nil, &services.ServiceError{
			Op:  op,
			Err: err,
		}
	}

	return block, nil
}

// ListBlocked returns all blocked IPs with pagination.
func (s *Service) ListBlocked(ctx context.Context, pagination services.Pagination) (*services.ListResult[*storage.BlockedIP], error) {
	const op = "ratelimit.ListBlocked"

	pagination = services.NewPagination(pagination.Limit, pagination.Offset)

	blocks, total, err := s.store.ListBlockedIPs(ctx, pagination.Limit, pagination.Offset)
	if err != nil {
		return nil, &services.ServiceError{
			Op:  op,
			Err: err,
		}
	}

	return &services.ListResult[*storage.BlockedIP]{
		Items:      blocks,
		TotalCount: total,
		Pagination: pagination,
	}, nil
}

// CleanupExpiredBlocks removes expired IP blocks.
func (s *Service) CleanupExpiredBlocks(ctx context.Context) (int64, error) {
	const op = "ratelimit.CleanupExpiredBlocks"

	count, err := s.store.CleanupExpiredBlockedIPs(ctx)
	if err != nil {
		return 0, &services.ServiceError{
			Op:  op,
			Err: err,
		}
	}

	return count, nil
}

// RecordRequest records a request for rate limiting.
func (s *Service) RecordRequest(ctx context.Context, key, bucket string, windowDuration time.Duration) error {
	const op = "ratelimit.RecordRequest"

	now := time.Now()
	windowStart := now.Truncate(windowDuration)
	windowEnd := windowStart.Add(windowDuration)

	if err := s.store.RecordRateLimitRequest(ctx, key, bucket, windowStart, windowEnd); err != nil {
		return &services.ServiceError{
			Op:  op,
			Err: err,
		}
	}

	return nil
}

// GetRequestCount returns request count for a key within a window.
func (s *Service) GetRequestCount(ctx context.Context, key, bucket string, window time.Duration) (int64, error) {
	const op = "ratelimit.GetRequestCount"

	since := time.Now().Add(-window)
	count, err := s.store.GetRateLimitCount(ctx, key, bucket, since)
	if err != nil {
		return 0, &services.ServiceError{
			Op:  op,
			Err: err,
		}
	}

	return count, nil
}

// CleanupOldRequests removes old rate limit records.
func (s *Service) CleanupOldRequests(ctx context.Context, before time.Time) (int64, error) {
	const op = "ratelimit.CleanupOldRequests"

	count, err := s.store.CleanupRateLimitRecords(ctx, before)
	if err != nil {
		return 0, &services.ServiceError{
			Op:  op,
			Err: err,
		}
	}

	return count, nil
}
