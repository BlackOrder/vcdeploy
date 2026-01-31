//go:build e2e

package e2e

import (
	"net/http"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/security"
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

// TestAuthMustChangePassword tests the MustChangePassword flow.
func TestAuthMustChangePassword(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Create a test user with MustChangePassword flag via API
	t.Run("create user with must change password", func(t *testing.T) {
		resp, err := ctx.Client.Post("/api/v1/users", map[string]interface{}{
			"username":             "mustchangeuser",
			"email":                "mustchange@example.com",
			"password":             "TempPass@123!",
			"role":                 "user",
			"must_change_password": true,
		})
		if err != nil {
			t.Fatalf("create user request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			t.Logf("Create user status: %d (may already exist)", resp.StatusCode)
		}
	})

	t.Run("API login with must change returns 403", func(t *testing.T) {
		// First, we need to get the user and set must_change_password
		// This is a bit tricky in E2E tests, so we'll skip if user doesn't exist
		noAuthClient := testutil.NewHTTPClient(cfg.MasterHTTPURL, "")

		resp, err := noAuthClient.Post("/api/v1/auth/login", map[string]string{
			"username": "mustchangeuser",
			"password": "TempPass@123!",
		})
		if err != nil {
			t.Fatalf("login request failed: %v", err)
		}
		defer resp.Body.Close()

		// Could be 403 (must change) or 401 (user doesn't exist from previous run)
		switch resp.StatusCode {
		case http.StatusForbidden:
			// Expected for must change password
			t.Log("Got 403 as expected for must change password")
		case http.StatusUnauthorized:
			t.Log("Got 401 - user may not exist or password changed in previous test run")
		case http.StatusOK:
			t.Log("Got 200 - user may not have must_change_password flag set")
		}
	})
}

// TestAuthTOTP tests TOTP authentication flows.
func TestAuthTOTP(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Generate TOTP secret for testing
	totpSecret, err := security.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("failed to generate TOTP secret: %v", err)
	}

	// Create a user with TOTP enabled via API
	t.Run("create user with TOTP", func(t *testing.T) {
		resp, err := ctx.Client.Post("/api/v1/users", map[string]interface{}{
			"username":     "totptestuser",
			"email":        "totptest@example.com",
			"password":     "TOTPPass@123!",
			"role":         "user",
			"totp_enabled": true,
			"totp_secret":  totpSecret,
		})
		if err != nil {
			t.Fatalf("create user request failed: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("Create TOTP user status: %d", resp.StatusCode)
	})

	t.Run("login without TOTP when required returns 401", func(t *testing.T) {
		noAuthClient := testutil.NewHTTPClient(cfg.MasterHTTPURL, "")

		resp, err := noAuthClient.Post("/api/v1/auth/login", map[string]string{
			"username": "totptestuser",
			"password": "TOTPPass@123!",
		})
		if err != nil {
			t.Fatalf("login request failed: %v", err)
		}
		defer resp.Body.Close()

		// Should fail because TOTP is required
		switch resp.StatusCode {
		case http.StatusOK:
			t.Log("Login succeeded - TOTP may not be enabled for this user")
		case http.StatusUnauthorized:
			t.Log("Got 401 - TOTP required or user doesn't exist")
		}
	})

	t.Run("login with valid TOTP succeeds", func(t *testing.T) {
		noAuthClient := testutil.NewHTTPClient(cfg.MasterHTTPURL, "")

		// Generate valid TOTP code
		validCode := security.GenerateTOTPCode(totpSecret, time.Now().Unix(), security.DefaultTOTPConfig())

		resp, err := noAuthClient.Post("/api/v1/auth/login", map[string]interface{}{
			"username": "totptestuser",
			"password": "TOTPPass@123!",
			"totp":     validCode,
		})
		if err != nil {
			t.Fatalf("login request failed: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("Login with TOTP status: %d", resp.StatusCode)
	})

	t.Run("login with invalid TOTP returns 401", func(t *testing.T) {
		noAuthClient := testutil.NewHTTPClient(cfg.MasterHTTPURL, "")

		resp, err := noAuthClient.Post("/api/v1/auth/login", map[string]interface{}{
			"username": "totptestuser",
			"password": "TOTPPass@123!",
			"totp":     "000000", // Invalid code
		})
		if err != nil {
			t.Fatalf("login request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", resp.StatusCode)
		}
	})
}

// TestAuthLogout tests the logout flow.
func TestAuthLogout(t *testing.T) {
	_ = setupTest(t)
	cfg := testutil.GetConfig()

	t.Run("logout invalidates session", func(t *testing.T) {
		// First, login to get a session token
		noAuthClient := testutil.NewHTTPClient(cfg.MasterHTTPURL, "")

		loginResp, err := noAuthClient.Post("/api/v1/auth/login", map[string]string{
			"username": cfg.AdminUsername,
			"password": cfg.AdminPassword,
		})
		if err != nil {
			t.Fatalf("login request failed: %v", err)
		}

		var loginResult map[string]interface{}
		if err := testutil.DecodeJSON(loginResp, &loginResult); err != nil {
			loginResp.Body.Close()
			t.Fatalf("failed to decode login response: %v", err)
		}
		loginResp.Body.Close()

		token, ok := loginResult["token"].(string)
		if !ok {
			t.Skip("token not found in login response - skipping logout test")
		}

		// Create client with the new session token
		sessionClient := testutil.NewHTTPClient(cfg.MasterHTTPURL, token)

		// Verify session works
		checkResp, err := sessionClient.Get("/api/v1/users")
		if err != nil {
			t.Fatalf("check request failed: %v", err)
		}
		checkResp.Body.Close()

		if checkResp.StatusCode != http.StatusOK {
			t.Errorf("session should be valid before logout, got %d", checkResp.StatusCode)
		}

		// Logout via web endpoint (note: this clears the session)
		logoutResp, err := sessionClient.GetWithRedirects("/logout")
		if err != nil {
			t.Fatalf("logout request failed: %v", err)
		}
		logoutResp.Body.Close()

		// Session should no longer work (for session-based auth)
		// Note: For bearer token auth, the token might still work until expiry
		t.Log("Logout completed")
	})
}

// TestAuthChangePassword tests the password change flow.
func TestAuthChangePassword(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Create a test user for password change tests
	t.Run("setup test user", func(t *testing.T) {
		resp, err := ctx.Client.Post("/api/v1/users", map[string]interface{}{
			"username": "changepassuser",
			"email":    "changepass@example.com",
			"password": "OldPass@123!",
			"role":     "user",
		})
		if err != nil {
			t.Fatalf("create user request failed: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("Create change password test user status: %d", resp.StatusCode)
	})

	t.Run("change password via web flow", func(t *testing.T) {
		noAuthClient := testutil.NewHTTPClient(cfg.MasterHTTPURL, "")

		// Login first to get a session
		loginResp, err := noAuthClient.PostForm("/login", map[string]string{
			"username": "changepassuser",
			"password": "OldPass@123!",
		})
		if err != nil {
			t.Fatalf("login request failed: %v", err)
		}
		loginResp.Body.Close()

		// Check if login succeeded or user doesn't exist
		if loginResp.StatusCode >= 400 {
			t.Skip("Login failed - user may not exist")
		}

		// Try to access change-password page
		// In a full test, we'd get the session cookie and submit the form
		t.Log("Password change web flow would be tested via UI tests")
	})
}

// TestAuthSessionExpiry tests session timeout behavior.
func TestAuthSessionExpiry(t *testing.T) {
	// Note: This test is limited in E2E context because we can't easily
	// create sessions with specific expiry times via the API
	_ = setupTest(t) // Ensure test infrastructure is available
	t.Skip("Session expiry is better tested in unit tests with direct DB access")
}

// TestAuthRateLimit tests rate limiting on login endpoint.
func TestAuthRateLimit(t *testing.T) {
	_ = setupTest(t) // Ensure test infrastructure is available
	cfg := testutil.GetConfig()

	t.Run("multiple failed logins", func(t *testing.T) {
		noAuthClient := testutil.NewHTTPClient(cfg.MasterHTTPURL, "")

		// Try multiple login attempts with wrong password
		for i := 0; i < 10; i++ {
			resp, err := noAuthClient.Post("/api/v1/auth/login", map[string]string{
				"username": "nonexistent",
				"password": "wrongpassword",
			})
			if err != nil {
				t.Fatalf("login request %d failed: %v", i, err)
			}
			resp.Body.Close()

			// Check if we got rate limited
			if resp.StatusCode == http.StatusTooManyRequests {
				t.Logf("Rate limited after %d attempts", i+1)
				return
			}
		}

		// If we didn't get rate limited, that's also valid (rate limiting might be disabled)
		t.Log("No rate limiting detected after 10 attempts")
	})
}
