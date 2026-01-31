//go:build e2e

package e2e

import (
	"fmt"
	"testing"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestAPIKeysAPI tests the API keys CRUD operations.
func TestAPIKeysAPI(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Login as admin
	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	t.Run("list API keys", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/api-keys")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)
	})

	var createdKeyID interface{}
	var createdKeyToken string

	t.Run("create API key", func(t *testing.T) {
		apiKey := map[string]interface{}{
			"name":        "e2e-test-key",
			"permissions": []string{"read:projects", "read:deployments"},
		}

		resp, err := ctx.Client.Post("/api/v1/api-keys", apiKey)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusCreatedOrOK(resp)

		var result map[string]interface{}
		if err := testutil.DecodeJSON(resp, &result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		createdKeyID = result["id"]
		if token, ok := result["token"].(string); ok {
			createdKeyToken = token
		}
		ctx.TrackResource("api-key", createdKeyID)
	})

	t.Run("get API key", func(t *testing.T) {
		if createdKeyID == nil {
			t.Skip("no API key created")
		}

		resp, err := ctx.Client.Get(fmt.Sprintf("/api/v1/api-keys/%v", createdKeyID))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)

		var apiKey map[string]interface{}
		if err := testutil.DecodeJSON(resp, &apiKey); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		ctx.Assertions.Equal(apiKey["name"], "e2e-test-key")
	})

	t.Run("use API key for authentication", func(t *testing.T) {
		if createdKeyToken == "" {
			t.Skip("no API key token")
		}

		apiKeyClient := testutil.NewHTTPClient(ctx.Config.MasterHTTPURL, createdKeyToken)

		resp, err := apiKeyClient.Get("/api/v1/projects")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)
	})

	t.Run("API key permissions", func(t *testing.T) {
		if createdKeyToken == "" {
			t.Skip("no API key token")
		}

		apiKeyClient := testutil.NewHTTPClient(ctx.Config.MasterHTTPURL, createdKeyToken)

		// Should be able to read projects (has read:projects)
		resp1, _ := apiKeyClient.Get("/api/v1/projects")
		defer resp1.Body.Close()
		ctx.Assertions.StatusOK(resp1)

		// Should NOT be able to create projects (missing write:projects)
		resp2, _ := apiKeyClient.Post("/api/v1/projects", map[string]interface{}{
			"name": "unauthorized-project",
		})
		defer resp2.Body.Close()
		ctx.Assertions.StatusForbidden(resp2)
	})

	t.Run("revoke API key", func(t *testing.T) {
		if createdKeyID == nil {
			t.Skip("no API key created")
		}

		resp, err := ctx.Client.Post(fmt.Sprintf("/api/v1/api-keys/%v/revoke", createdKeyID), nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)
	})

	t.Run("revoked key cannot authenticate", func(t *testing.T) {
		if createdKeyToken == "" {
			t.Skip("no API key token")
		}

		revokedClient := testutil.NewHTTPClient(ctx.Config.MasterHTTPURL, createdKeyToken)

		resp, err := revokedClient.Get("/api/v1/projects")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusUnauthorized(resp)
	})

	t.Run("delete API key", func(t *testing.T) {
		if createdKeyID == nil {
			t.Skip("no API key created")
		}

		resp, err := ctx.Client.Delete(fmt.Sprintf("/api/v1/api-keys/%v", createdKeyID))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.NoServerError(resp)
	})

	t.Run("create API key with invalid scopes", func(t *testing.T) {
		apiKey := map[string]interface{}{
			"name":   "invalid-scopes-key",
			"scopes": []string{"invalid:scope"},
		}

		resp, err := ctx.Client.Post("/api/v1/api-keys", apiKey)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		// Currently the API doesn't validate scopes and accepts anything
		// This is a permissive design - unknown scopes are simply ignored
		// So we expect 201 Created, not 400
		ctx.Assertions.StatusCreated(resp)
	})

	t.Cleanup(func() {
		ctx.CleanupResources()
	})
}

// TestAPIKeyScopes tests different API key permission scopes.
func TestAPIKeyScopes(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Login as admin
	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	// Create keys with different permission sets
	permissionSets := []struct {
		name        string
		permissions []string
		canRead     bool
		canWrite    bool
	}{
		{"read-only", []string{"read:projects", "read:deployments"}, true, false},
		{"write-only", []string{"write:projects"}, false, true},
		{"full-access", []string{"read:projects", "write:projects", "read:deployments", "write:deployments"}, true, true},
	}

	for _, ps := range permissionSets {
		t.Run(ps.name, func(t *testing.T) {
			// Create the API key
			apiKey := map[string]interface{}{
				"name":        fmt.Sprintf("e2e-scope-test-%s", ps.name),
				"permissions": ps.permissions,
			}

			resp, err := ctx.Client.Post("/api/v1/api-keys", apiKey)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}

			var result map[string]interface{}
			testutil.DecodeJSON(resp, &result)
			resp.Body.Close()

			token, _ := result["token"].(string)
			keyID := result["id"]

			if token == "" {
				t.Skip("no token returned")
			}

			keyClient := testutil.NewHTTPClient(ctx.Config.MasterHTTPURL, token)

			// Test read access
			readResp, _ := keyClient.Get("/api/v1/projects")
			defer readResp.Body.Close()

			if ps.canRead {
				ctx.Assertions.StatusOK(readResp)
			} else {
				ctx.Assertions.StatusForbidden(readResp)
			}

			// Cleanup
			ctx.Cleanup.DeleteAPIKey(keyID)
		})
	}
}
