package storage

import (
	"context"
	"testing"
	"time"
)

// --- BlockedIP tests ---

func TestMemoryStore_BlockIP(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	block := &BlockedIP{
		IPAddress: "192.168.1.100",
		Reason:    "Too many failed login attempts",
		ExpiresAt: time.Now().Add(time.Hour),
		BlockedBy: "system",
	}
	err := s.BlockIP(ctx, block)
	if err != nil {
		t.Fatalf("BlockIP() error = %v", err)
	}

	if block.ID == "" {
		t.Error("BlockIP() did not assign ID")
	}
}

func TestMemoryStore_BlockIP_Duplicate(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.BlockIP(ctx, &BlockedIP{IPAddress: "192.168.1.100"})

	err := s.BlockIP(ctx, &BlockedIP{IPAddress: "192.168.1.100"})
	if err != ErrDuplicate {
		t.Errorf("BlockIP() error = %v, want ErrDuplicate", err)
	}
}

func TestMemoryStore_UnblockIP(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.BlockIP(ctx, &BlockedIP{IPAddress: "192.168.1.100"})

	err := s.UnblockIP(ctx, "192.168.1.100")
	if err != nil {
		t.Fatalf("UnblockIP() error = %v", err)
	}

	blocked, _ := s.IsIPBlocked(ctx, "192.168.1.100")
	if blocked {
		t.Error("IP still blocked after unblock")
	}
}

func TestMemoryStore_UnblockIP_NotFound(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	err := s.UnblockIP(context.Background(), "192.168.1.100")
	if err != ErrNotFound {
		t.Errorf("UnblockIP() error = %v, want ErrNotFound", err)
	}
}

func TestMemoryStore_IsIPBlocked(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.BlockIP(ctx, &BlockedIP{
		IPAddress: "192.168.1.100",
		ExpiresAt: time.Now().Add(time.Hour), // Not expired
	})

	blocked, err := s.IsIPBlocked(ctx, "192.168.1.100")
	if err != nil {
		t.Fatalf("IsIPBlocked() error = %v", err)
	}
	if !blocked {
		t.Error("IP should be blocked")
	}
}

func TestMemoryStore_IsIPBlocked_Expired(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.BlockIP(ctx, &BlockedIP{
		IPAddress: "192.168.1.100",
		ExpiresAt: time.Now().Add(-time.Hour), // Expired
	})

	blocked, err := s.IsIPBlocked(ctx, "192.168.1.100")
	if err != nil {
		t.Fatalf("IsIPBlocked() error = %v", err)
	}
	if blocked {
		t.Error("Expired block should not be considered blocked")
	}
}

func TestMemoryStore_IsIPBlocked_NotBlocked(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	blocked, err := s.IsIPBlocked(context.Background(), "192.168.1.100")
	if err != nil {
		t.Fatalf("IsIPBlocked() error = %v", err)
	}
	if blocked {
		t.Error("IP should not be blocked")
	}
}

func TestMemoryStore_GetBlockedIP(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	s.BlockIP(ctx, &BlockedIP{IPAddress: "192.168.1.100", Reason: "test reason"})

	block, err := s.GetBlockedIP(ctx, "192.168.1.100")
	if err != nil {
		t.Fatalf("GetBlockedIP() error = %v", err)
	}
	if block.Reason != "test reason" {
		t.Errorf("Reason = %s, want 'test reason'", block.Reason)
	}
}

func TestMemoryStore_GetBlockedIP_NotFound(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	_, err := s.GetBlockedIP(context.Background(), "192.168.1.100")
	if err != ErrNotFound {
		t.Errorf("GetBlockedIP() error = %v, want ErrNotFound", err)
	}
}

func TestMemoryStore_ListBlockedIPs(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	now := time.Now()
	s.BlockIP(ctx, &BlockedIP{IPAddress: "192.168.1.1", BlockedAt: now.Add(-time.Hour)})
	s.BlockIP(ctx, &BlockedIP{IPAddress: "192.168.1.2", BlockedAt: now})

	list, total, err := s.ListBlockedIPs(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListBlockedIPs() error = %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	// Should be newest first
	if list[0].IPAddress != "192.168.1.2" {
		t.Errorf("First IP = %s, want 192.168.1.2 (newest first)", list[0].IPAddress)
	}
}

func TestMemoryStore_ListBlockedIPs_Pagination(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		s.BlockIP(ctx, &BlockedIP{IPAddress: "192.168.1." + string(rune('0'+i))})
	}

	list, total, _ := s.ListBlockedIPs(ctx, 2, 2)
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(list) != 2 {
		t.Errorf("len(list) = %d, want 2", len(list))
	}
}

// --- RateLimit tests ---

func TestMemoryStore_RecordRateLimitRequest(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	windowStart := time.Now().Truncate(time.Minute)
	windowEnd := windowStart.Add(time.Minute)

	err := s.RecordRateLimitRequest(ctx, "192.168.1.100", "api", windowStart, windowEnd)
	if err != nil {
		t.Fatalf("RecordRateLimitRequest() error = %v", err)
	}

	count, _ := s.GetRateLimitCount(ctx, "192.168.1.100", "api", windowStart)
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestMemoryStore_RecordRateLimitRequest_Increment(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	windowStart := time.Now().Truncate(time.Minute)
	windowEnd := windowStart.Add(time.Minute)

	// Record multiple requests in the same window
	s.RecordRateLimitRequest(ctx, "192.168.1.100", "api", windowStart, windowEnd)
	s.RecordRateLimitRequest(ctx, "192.168.1.100", "api", windowStart, windowEnd)
	s.RecordRateLimitRequest(ctx, "192.168.1.100", "api", windowStart, windowEnd)

	count, _ := s.GetRateLimitCount(ctx, "192.168.1.100", "api", windowStart)
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
}

func TestMemoryStore_GetRateLimitCount_NoRecord(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()

	count, err := s.GetRateLimitCount(context.Background(), "192.168.1.100", "api", time.Now())
	if err != nil {
		t.Fatalf("GetRateLimitCount() error = %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestMemoryStore_GetRateLimitCount_ExpiredWindow(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	// Old window
	oldWindowStart := time.Now().Add(-2 * time.Hour)
	oldWindowEnd := oldWindowStart.Add(time.Hour)
	s.RecordRateLimitRequest(ctx, "192.168.1.100", "api", oldWindowStart, oldWindowEnd)

	// Query for recent time (after old window ended)
	count, _ := s.GetRateLimitCount(ctx, "192.168.1.100", "api", time.Now().Add(-30*time.Minute))
	if count != 0 {
		t.Errorf("count = %d, want 0 (window expired)", count)
	}
}

func TestMemoryStore_RateLimitDifferentBuckets(t *testing.T) {
	s := NewMemoryStore(nil)
	defer s.Close()
	ctx := context.Background()

	windowStart := time.Now().Truncate(time.Minute)
	windowEnd := windowStart.Add(time.Minute)

	s.RecordRateLimitRequest(ctx, "192.168.1.100", "api", windowStart, windowEnd)
	s.RecordRateLimitRequest(ctx, "192.168.1.100", "api", windowStart, windowEnd)
	s.RecordRateLimitRequest(ctx, "192.168.1.100", "login", windowStart, windowEnd)

	apiCount, _ := s.GetRateLimitCount(ctx, "192.168.1.100", "api", windowStart)
	loginCount, _ := s.GetRateLimitCount(ctx, "192.168.1.100", "login", windowStart)

	if apiCount != 2 {
		t.Errorf("api count = %d, want 2", apiCount)
	}
	if loginCount != 1 {
		t.Errorf("login count = %d, want 1", loginCount)
	}
}
