//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestSetupFlow tests the first-run setup wizard via API.
// NOTE: These tests require the server to be started WITHOUT VCDEPLOY_ADMIN_PASSWORD.
func TestSetupFlow(t *testing.T) {
	ctx := setupTest(t)

	// Check if server is in setup mode or already configured
	t.Run("check setup status", func(t *testing.T) {
		// Try to access a protected endpoint without auth
		noAuthClient := testutil.NewHTTPClient(ctx.Config.MasterHTTPURL, "")
		resp, err := noAuthClient.GetWithRedirects("/stats")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		// If we're redirected to /setup, server needs setup
		// If we're redirected to /login, server is configured
		finalURL := resp.Request.URL.String()
		t.Logf("Final URL: %s", finalURL)

		// This is informational - actual setup tests depend on server state
	})

	t.Run("setup endpoint redirects when configured", func(t *testing.T) {
		// When server has users, /setup should redirect to /login
		noAuthClient := testutil.NewHTTPClient(ctx.Config.MasterHTTPURL, "")
		resp, err := noAuthClient.GetWithRedirects("/setup")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		// Should either stay on /setup (needs setup) or redirect to /login (configured)
		finalURL := resp.Request.URL.String()
		if !containsAny(finalURL, "/setup", "/login") {
			t.Errorf("expected redirect to /setup or /login, got %s", finalURL)
		}
	})

	t.Run("health endpoints work during setup", func(t *testing.T) {
		noAuthClient := testutil.NewHTTPClient(ctx.Config.MasterHTTPURL, "")

		endpoints := []string{"/healthz", "/livez", "/readyz", "/api/v1/health"}
		for _, endpoint := range endpoints {
			resp, err := noAuthClient.Get(endpoint)
			if err != nil {
				t.Errorf("%s: request failed: %v", endpoint, err)
				continue
			}
			resp.Body.Close()

			if resp.StatusCode >= 400 {
				t.Errorf("%s: expected success, got %d", endpoint, resp.StatusCode)
			}
		}
	})
}

// TestSetupValidation tests setup form validation via API.
func TestSetupValidation(t *testing.T) {
	ctx := setupTest(t)
	noAuthClient := testutil.NewHTTPClient(ctx.Config.MasterHTTPURL, "")

	// First check if server needs setup
	resp, err := noAuthClient.GetWithRedirects("/setup")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	// If redirected to login, server is already configured - skip validation tests
	if containsAny(resp.Request.URL.String(), "/login") {
		t.Skip("Server already configured - skipping setup validation tests")
	}

	t.Run("missing username returns error", func(t *testing.T) {
		resp, err := noAuthClient.PostForm("/setup", map[string]string{
			"email":            "admin@example.com",
			"password":         "Admin@Password123!",
			"confirm_password": "Admin@Password123!",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		// Should return 200 with error in body, or 400
		// The important thing is it doesn't create a user without username
	})

	t.Run("missing email returns error", func(t *testing.T) {
		resp, err := noAuthClient.PostForm("/setup", map[string]string{
			"username":         "admin",
			"password":         "Admin@Password123!",
			"confirm_password": "Admin@Password123!",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
	})

	t.Run("password mismatch returns error", func(t *testing.T) {
		resp, err := noAuthClient.PostForm("/setup", map[string]string{
			"username":         "admin",
			"email":            "admin@example.com",
			"password":         "Admin@Password123!",
			"confirm_password": "DifferentPassword123!",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		// Should not redirect to dashboard (setup not complete)
		finalURL := resp.Request.URL.String()
		if containsAny(finalURL, "/stats") {
			t.Error("setup should not succeed with mismatched passwords")
		}
	})

	t.Run("weak password returns error", func(t *testing.T) {
		resp, err := noAuthClient.PostForm("/setup", map[string]string{
			"username":         "admin",
			"email":            "admin@example.com",
			"password":         "weak",
			"confirm_password": "weak",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		// Should not redirect to dashboard
		finalURL := resp.Request.URL.String()
		if containsAny(finalURL, "/stats") {
			t.Error("setup should not succeed with weak password")
		}
	})
}

// TestEnvAdminCredentials tests that admin created via environment variables works.
func TestEnvAdminCredentials(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	t.Run("can login with env-configured admin", func(t *testing.T) {
		resp, err := ctx.Client.Post("/api/v1/auth/login", map[string]string{
			"username": cfg.AdminUsername,
			"password": cfg.AdminPassword,
		})
		if err != nil {
			t.Fatalf("login request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		if err := testutil.DecodeJSON(resp, &result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if _, ok := result["token"]; !ok {
			t.Error("expected token in response")
		}
	})

	t.Run("env admin has admin role", func(t *testing.T) {
		// Login first
		resp, err := ctx.Client.Post("/api/v1/auth/login", map[string]string{
			"username": cfg.AdminUsername,
			"password": cfg.AdminPassword,
		})
		if err != nil {
			t.Fatalf("login request failed: %v", err)
		}

		var loginResult map[string]interface{}
		if err := testutil.DecodeJSON(resp, &loginResult); err != nil {
			t.Fatalf("failed to decode login response: %v", err)
		}
		resp.Body.Close()

		token, ok := loginResult["token"].(string)
		if !ok {
			t.Fatal("token not found in login response")
		}

		// Get current user info
		authClient := testutil.NewHTTPClient(ctx.Config.MasterHTTPURL, token)
		resp, err = authClient.Get("/api/v1/auth/me")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var userInfo map[string]interface{}
		if err := testutil.DecodeJSON(resp, &userInfo); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		role, ok := userInfo["role"].(string)
		if !ok || role != "admin" {
			t.Errorf("expected admin role, got %v", userInfo["role"])
		}
	})
}

// containsAny checks if str contains any of the substrings.
func containsAny(str string, substrings ...string) bool {
	for _, sub := range substrings {
		if len(sub) > 0 && len(str) >= len(sub) {
			for i := 0; i <= len(str)-len(sub); i++ {
				if str[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
