//go:build e2e

package e2e

import (
	"testing"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestPaginationEndpoints tests pagination behavior across all paginated endpoints.
func TestPaginationEndpoints(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Login as admin
	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	// Test endpoints that return paginated responses
	endpoints := []struct {
		name     string
		endpoint string
	}{
		{"users", "/api/v1/users"},
		{"projects", "/api/v1/projects"},
		{"agents", "/api/v1/agents"},
		{"deployments", "/api/v1/deployments"},
		{"secrets", "/api/v1/secrets"},
		{"api-keys", "/api/v1/api-keys"},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			t.Run("default pagination", func(t *testing.T) {
				resp, err := ctx.Client.Get(ep.endpoint)
				if err != nil {
					t.Fatalf("request failed: %v", err)
				}
				defer resp.Body.Close()

				ctx.Assertions.StatusOK(resp)

				result, err := testutil.DecodePaginatedJSON(resp)
				if err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}

				// Verify pagination structure
				if result.Limit <= 0 {
					t.Errorf("limit should be positive, got %d", result.Limit)
				}
				if result.Offset < 0 {
					t.Errorf("offset should be non-negative, got %d", result.Offset)
				}
				if result.TotalCount < 0 {
					t.Errorf("totalCount should be non-negative, got %d", result.TotalCount)
				}
			})

			t.Run("custom limit", func(t *testing.T) {
				resp, err := ctx.Client.Get(ep.endpoint + "?limit=5")
				if err != nil {
					t.Fatalf("request failed: %v", err)
				}
				defer resp.Body.Close()

				ctx.Assertions.StatusOK(resp)

				result, err := testutil.DecodePaginatedJSON(resp)
				if err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}

				if result.Limit != 5 {
					t.Errorf("expected limit 5, got %d", result.Limit)
				}
				// Items should not exceed limit
				if len(result.Items) > 5 {
					t.Errorf("expected at most 5 items, got %d", len(result.Items))
				}
			})

			t.Run("offset pagination", func(t *testing.T) {
				resp, err := ctx.Client.Get(ep.endpoint + "?limit=5&offset=0")
				if err != nil {
					t.Fatalf("request failed: %v", err)
				}
				defer resp.Body.Close()

				ctx.Assertions.StatusOK(resp)

				result, err := testutil.DecodePaginatedJSON(resp)
				if err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}

				if result.Offset != 0 {
					t.Errorf("expected offset 0, got %d", result.Offset)
				}
			})

			t.Run("large offset returns empty items", func(t *testing.T) {
				resp, err := ctx.Client.Get(ep.endpoint + "?offset=999999")
				if err != nil {
					t.Fatalf("request failed: %v", err)
				}
				defer resp.Body.Close()

				ctx.Assertions.StatusOK(resp)

				result, err := testutil.DecodePaginatedJSON(resp)
				if err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}

				// With large offset, should have 0 items
				if len(result.Items) != 0 {
					t.Errorf("expected 0 items with large offset, got %d", len(result.Items))
				}
				// But totalCount should still reflect actual count
				if result.TotalCount < 0 {
					t.Errorf("totalCount should be non-negative even with large offset, got %d", result.TotalCount)
				}
			})
		})
	}
}

// TestPaginationConsistency verifies totalCount stays consistent across paginated requests.
func TestPaginationConsistency(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	// Get first page
	resp1, err := ctx.Client.Get("/api/v1/users?limit=1&offset=0")
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	defer resp1.Body.Close()

	result1, err := testutil.DecodePaginatedJSON(resp1)
	if err != nil {
		t.Fatalf("failed to decode first response: %v", err)
	}

	// Get second page
	resp2, err := ctx.Client.Get("/api/v1/users?limit=1&offset=1")
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	defer resp2.Body.Close()

	result2, err := testutil.DecodePaginatedJSON(resp2)
	if err != nil {
		t.Fatalf("failed to decode second response: %v", err)
	}

	// totalCount should be consistent
	if result1.TotalCount != result2.TotalCount {
		t.Errorf("totalCount changed between requests: %d vs %d", result1.TotalCount, result2.TotalCount)
	}
}

// TestAuditPagination tests the audit endpoint pagination specifically.
func TestAuditPagination(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	t.Run("audit with limit", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/audit?limit=10")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)

		result, err := testutil.DecodePaginatedJSON(resp)
		if err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if result.Limit != 10 {
			t.Errorf("expected limit 10, got %d", result.Limit)
		}
	})
}
