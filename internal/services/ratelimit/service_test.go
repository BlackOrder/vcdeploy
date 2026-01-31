// Package ratelimit provides rate limiting and IP blocking services.
package ratelimit

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/services"
	"github.com/BlackOrder/vcdeploy/internal/services/testutil"
)

func setupTest(t *testing.T) (*Service, func()) {
	t.Helper()
	db, cleanup := testutil.NewTestStore(t)
	svc := New(db)
	return svc, cleanup
}

func TestService_BlockIP(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("block valid IP", func(t *testing.T) {
		err := svc.BlockIP(ctx, "192.168.1.100", "test reason", time.Hour, "admin")
		if err != nil {
			t.Fatalf("Failed to block IP: %v", err)
		}

		blocked, err := svc.IsBlocked(ctx, "192.168.1.100")
		if err != nil {
			t.Fatalf("Failed to check block: %v", err)
		}
		if !blocked {
			t.Error("Expected IP to be blocked")
		}
	})

	t.Run("block empty IP", func(t *testing.T) {
		err := svc.BlockIP(ctx, "", "test", time.Hour, "admin")
		if err == nil {
			t.Error("Expected error for empty IP")
		}
		if !services.IsInvalidInput(err) {
			t.Errorf("Expected invalid input error, got: %v", err)
		}
	})
}

func TestService_UnblockIP(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Block first
	err := svc.BlockIP(ctx, "192.168.1.101", "test", time.Hour, "admin")
	if err != nil {
		t.Fatalf("Failed to block IP: %v", err)
	}

	// Verify blocked
	blocked, err := svc.IsBlocked(ctx, "192.168.1.101")
	if err != nil {
		t.Fatalf("Failed to check block: %v", err)
	}
	if !blocked {
		t.Error("Expected IP to be blocked before unblock")
	}

	// Unblock
	err = svc.UnblockIP(ctx, "192.168.1.101")
	if err != nil {
		t.Fatalf("Failed to unblock IP: %v", err)
	}

	// Verify unblocked
	blocked, err = svc.IsBlocked(ctx, "192.168.1.101")
	if err != nil {
		t.Fatalf("Failed to check block: %v", err)
	}
	if blocked {
		t.Error("Expected IP to be unblocked")
	}
}

func TestService_GetBlock(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("existing block", func(t *testing.T) {
		err := svc.BlockIP(ctx, "10.0.0.1", "suspicious activity", time.Hour, "system")
		if err != nil {
			t.Fatalf("Failed to block IP: %v", err)
		}

		block, err := svc.GetBlock(ctx, "10.0.0.1")
		if err != nil {
			t.Fatalf("Failed to get block: %v", err)
		}
		if block.Reason != "suspicious activity" {
			t.Errorf("Expected reason 'suspicious activity', got %q", block.Reason)
		}
		if block.BlockedBy != "system" {
			t.Errorf("Expected blocked_by 'system', got %q", block.BlockedBy)
		}
	})

	t.Run("non-existent block", func(t *testing.T) {
		_, err := svc.GetBlock(ctx, "99.99.99.99")
		if err == nil {
			t.Error("Expected error for non-existent block")
		}
		if !services.IsNotFound(err) {
			t.Errorf("Expected not found error, got: %v", err)
		}
	})
}

func TestService_ListBlocked(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Block some IPs
	for i := range 5 {
		ip := fmt.Sprintf("10.0.0.%d", i+1)
		_ = svc.BlockIP(ctx, ip, "test", time.Hour, "admin")
	}

	result, err := svc.ListBlocked(ctx, services.Pagination{Limit: 10})
	if err != nil {
		t.Fatalf("Failed to list blocked IPs: %v", err)
	}

	if len(result.Items) != 5 {
		t.Errorf("Expected 5 blocked IPs, got %d", len(result.Items))
	}
	if result.TotalCount != 5 {
		t.Errorf("Expected total count 5, got %d", result.TotalCount)
	}
}

func TestService_CleanupExpiredBlocks(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Block with valid duration
	err := svc.BlockIP(ctx, "1.1.1.1", "test", time.Hour, "admin")
	if err != nil {
		t.Fatalf("Failed to block IP: %v", err)
	}

	// Cleanup - should remove nothing (block is still valid)
	count, err := svc.CleanupExpiredBlocks(ctx)
	if err != nil {
		t.Fatalf("Failed to cleanup: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 cleaned up, got %d", count)
	}
}

func TestService_RequestTracking(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()
	key := "192.168.1.50"
	bucket := "/api/test"
	window := time.Minute

	// Record some requests
	for range 10 {
		err := svc.RecordRequest(ctx, key, bucket, window)
		if err != nil {
			t.Fatalf("Failed to record request: %v", err)
		}
	}

	// Get count
	count, err := svc.GetRequestCount(ctx, key, bucket, window)
	if err != nil {
		t.Fatalf("Failed to get count: %v", err)
	}

	if count != 10 {
		t.Errorf("Expected 10 requests, got %d", count)
	}
}

func TestService_CleanupOldRequests(t *testing.T) {
	svc, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Record some requests
	_ = svc.RecordRequest(ctx, "test", "bucket", time.Minute)

	// Cleanup requests older than 1 hour from now (should remove nothing since they're recent)
	count, err := svc.CleanupOldRequests(ctx, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("Failed to cleanup: %v", err)
	}
	// Should be 0 since requests are recent
	t.Logf("Cleaned up %d old requests", count)
}
