//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestAuthLogin tests the login API endpoint.
func TestAuthLogin(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	t.Run("successful login", func(t *testing.T) {
		resp, err := ctx.Client.Post("/api/v1/auth/login", map[string]string{
			"username": cfg.AdminUsername,
			"password": cfg.AdminPassword,
		})
		if err != nil {
			t.Fatalf("login request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)

		var result map[string]interface{}
		if err := testutil.DecodeJSON(resp, &result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		ctx.Assertions.HasField(result, "token")
	})

	t.Run("invalid password", func(t *testing.T) {
		resp, err := ctx.Client.Post("/api/v1/auth/login", map[string]string{
			"username": cfg.AdminUsername,
			"password": "wrongpassword",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusUnauthorized(resp)
	})

	t.Run("invalid username", func(t *testing.T) {
		resp, err := ctx.Client.Post("/api/v1/auth/login", map[string]string{
			"username": "nonexistent",
			"password": "password",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusUnauthorized(resp)
	})

	t.Run("missing credentials", func(t *testing.T) {
		resp, err := ctx.Client.Post("/api/v1/auth/login", map[string]string{})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 400 or 401, got %d", resp.StatusCode)
		}
	})
}

// TestAuthUnauthorized tests that protected endpoints require authentication.
func TestAuthUnauthorized(t *testing.T) {
	ctx := setupTest(t)

	// Test without setting a token
	noAuthClient := testutil.NewHTTPClient(ctx.Config.MasterHTTPURL, "")

	endpoints := []string{
		"/api/v1/users",
		"/api/v1/projects",
		"/api/v1/deployments",
		"/api/v1/agents",
		"/api/v1/secrets",
		"/api/v1/settings/general",
		"/api/v1/apikeys",
		"/api/v1/audit",
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			resp, err := noAuthClient.Get(endpoint)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			ctx.Assertions.StatusUnauthorized(resp)
		})
	}
}

// TestAuthInvalidToken tests that invalid tokens are rejected.
func TestAuthInvalidToken(t *testing.T) {
	ctx := setupTest(t)

	invalidTokenClient := testutil.NewHTTPClient(ctx.Config.MasterHTTPURL, "invalid-token-12345")

	resp, err := invalidTokenClient.Get("/api/v1/users")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	ctx.Assertions.StatusUnauthorized(resp)
}

// TestAuthExpiredToken tests that expired tokens are rejected.
func TestAuthExpiredToken(t *testing.T) {
	// This test would need a way to generate expired tokens
	// For now, we'll test with a malformed JWT
	ctx := setupTest(t)

	malformedClient := testutil.NewHTTPClient(ctx.Config.MasterHTTPURL, "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjF9.invalid")

	resp, err := malformedClient.Get("/api/v1/users")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	ctx.Assertions.StatusUnauthorized(resp)
}
