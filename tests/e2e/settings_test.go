//go:build e2e

package e2e

import (
	"testing"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestSettingsAPI tests the settings API endpoints.
func TestSettingsAPI(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Login as admin
	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	categories := []string{"general", "appearance", "security", "notifications"}

	for _, category := range categories {
		t.Run("get "+category+" settings", func(t *testing.T) {
			resp, err := ctx.Client.Get("/api/v1/settings/" + category)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()
			ctx.Assertions.StatusOK(resp)

			var settings map[string]interface{}
			if err := testutil.DecodeJSON(resp, &settings); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
		})
	}

	t.Run("update appearance settings", func(t *testing.T) {
		// API now accepts native types and coerces to strings
		settings := map[string]interface{}{
			"dark_mode":    true, // bool - coerced to "true"
			"accent_color": "blue",
		}
		resp, err := ctx.Client.Put("/api/v1/settings/appearance", settings)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		ctx.Assertions.StatusOK(resp)

		// Verify the change - value should be stored as "true" string
		getResp, _ := ctx.Client.Get("/api/v1/settings/appearance")
		defer getResp.Body.Close()
		var result map[string]interface{}
		testutil.DecodeJSON(getResp, &result)
		ctx.Assertions.Equal(result["dark_mode"], "true")
	})

	t.Run("update general settings", func(t *testing.T) {
		settings := map[string]interface{}{
			"site_name":      "E2E Test VCDeploy",
			"session_expiry": "24h",
		}
		resp, err := ctx.Client.Put("/api/v1/settings/general", settings)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		ctx.Assertions.StatusOK(resp)
	})

	t.Run("update security settings", func(t *testing.T) {
		// API now accepts native types and coerces to strings
		settings := map[string]interface{}{
			"require_2fa_admin":  false, // bool - coerced to "false"
			"max_login_attempts": 5,     // int - coerced to "5"
		}
		resp, err := ctx.Client.Put("/api/v1/settings/security", settings)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		ctx.Assertions.StatusOK(resp)
	})

	t.Run("get invalid settings category", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/settings/invalid-category")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		// The API returns 200 with an empty object for unknown categories
		// (no validation of category names)
		ctx.Assertions.StatusOK(resp)
	})
}

// TestSettingsExportImport tests settings export and import functionality.
func TestSettingsExportImport(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Login as admin
	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	var exportedSettings map[string]interface{}

	t.Run("export settings", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/settings/export")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		ctx.Assertions.StatusOK(resp)

		if err := testutil.DecodeJSON(resp, &exportedSettings); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		ctx.Assertions.NotNil(exportedSettings)
	})

	t.Run("import settings", func(t *testing.T) {
		if exportedSettings == nil {
			t.Skip("no exported settings")
		}

		resp, err := ctx.Client.Post("/api/v1/settings/import", exportedSettings)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		ctx.Assertions.StatusOK(resp)
	})

	t.Run("import invalid settings", func(t *testing.T) {
		invalidSettings := map[string]interface{}{
			"invalid": "data",
		}
		resp, err := ctx.Client.Post("/api/v1/settings/import", invalidSettings)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		// May return 400 or 200 depending on validation
		ctx.Assertions.NoServerError(resp)
	})
}
