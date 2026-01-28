//go:build e2e

// Package e2e provides end-to-end API tests for new vcdeploy endpoints.
package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// TestAPIHostKeys tests the host keys API endpoints.
func TestAPIHostKeys(t *testing.T) {
	cfg := getTestConfig()

	if err := waitForHTTPEndpoint(cfg.MasterHTTPURL + "/api/v1/health"); err != nil {
		t.Skipf("Master not available: %v", err)
	}

	t.Run("list host keys", func(t *testing.T) {
		resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/host-keys", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to list host keys: %v", err)
		}
		defer resp.Body.Close()

		expectStatusOK(t, resp)

		var keys []map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&keys); err != nil {
			t.Errorf("failed to decode response: %v", err)
		}
	})

	t.Run("create host key", func(t *testing.T) {
		hostKey := map[string]interface{}{
			"host":        "test-server.example.com",
			"port":        22,
			"key_type":    "ssh-ed25519",
			"fingerprint": "SHA256:testfingerprint123456789",
			"verified":    true,
		}

		resp, err := doAuthRequest("POST", cfg.MasterHTTPURL+"/api/v1/host-keys", hostKey, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to create host key: %v", err)
		}
		defer resp.Body.Close()

		expectStatusCreatedOrOK(t, resp)
	})

	t.Run("verify host key", func(t *testing.T) {
		verifyReq := map[string]interface{}{
			"host": "test-server.example.com",
			"port": 22,
		}

		resp, err := doAuthRequest("POST", cfg.MasterHTTPURL+"/api/v1/host-keys/verify", verifyReq, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to verify host key: %v", err)
		}
		defer resp.Body.Close()

		// May return 200 OK or error depending on whether host is reachable
		if resp.StatusCode == http.StatusNotFound {
			t.Errorf("endpoint returned 404")
		}
	})
}

// TestAPIJumpServers tests the jump server API endpoints.
func TestAPIJumpServers(t *testing.T) {
	cfg := getTestConfig()

	if err := waitForHTTPEndpoint(cfg.MasterHTTPURL + "/api/v1/health"); err != nil {
		t.Skipf("Master not available: %v", err)
	}

	t.Run("list jump servers", func(t *testing.T) {
		resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/jump-servers", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to list jump servers: %v", err)
		}
		defer resp.Body.Close()

		expectStatusOK(t, resp)
	})

	t.Run("create jump server", func(t *testing.T) {
		jumpServer := map[string]interface{}{
			"name":     "e2e-jump-server",
			"host":     "jump.example.com",
			"port":     22,
			"username": "jumpuser",
			"priority": 10,
		}

		resp, err := doAuthRequest("POST", cfg.MasterHTTPURL+"/api/v1/jump-servers", jumpServer, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to create jump server: %v", err)
		}
		defer resp.Body.Close()

		expectStatusCreatedOrOK(t, resp)
	})

	t.Run("test jump server connection", func(t *testing.T) {
		// Test connection to a jump server (will fail if not configured, but endpoint should exist)
		resp, err := doAuthRequest("POST", cfg.MasterHTTPURL+"/api/v1/jump-servers/e2e-jump-server/test", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to test jump server: %v", err)
		}
		defer resp.Body.Close()

		// Accept various status codes (connection test may fail)
		if resp.StatusCode == http.StatusNotFound {
			t.Errorf("endpoint returned 404")
		}
	})
}

// TestAPIBlockedIPs tests the blocked IPs API endpoints.
func TestAPIBlockedIPs(t *testing.T) {
	cfg := getTestConfig()

	if err := waitForHTTPEndpoint(cfg.MasterHTTPURL + "/api/v1/health"); err != nil {
		t.Skipf("Master not available: %v", err)
	}

	t.Run("list blocked IPs", func(t *testing.T) {
		resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/blocked-ips", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to list blocked IPs: %v", err)
		}
		defer resp.Body.Close()

		expectStatusOK(t, resp)
	})

	t.Run("add blocked IP", func(t *testing.T) {
		blockedIP := map[string]interface{}{
			"ip":       "192.0.2.100",
			"reason":   "E2E test block",
			"duration": "24h",
		}

		resp, err := doAuthRequest("POST", cfg.MasterHTTPURL+"/api/v1/blocked-ips", blockedIP, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to add blocked IP: %v", err)
		}
		defer resp.Body.Close()

		expectStatusCreatedOrOK(t, resp)
	})

	t.Run("remove blocked IP", func(t *testing.T) {
		resp, err := doAuthRequest("DELETE", cfg.MasterHTTPURL+"/api/v1/blocked-ips/192.0.2.100", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to remove blocked IP: %v", err)
		}
		defer resp.Body.Close()

		// Accept 200, 204, or 404 (if not found)
		if resp.StatusCode >= 500 {
			t.Errorf("server error: %d", resp.StatusCode)
		}
	})
}

// TestAPIProvisionJobs tests the provision jobs API endpoints.
func TestAPIProvisionJobs(t *testing.T) {
	cfg := getTestConfig()

	if err := waitForHTTPEndpoint(cfg.MasterHTTPURL + "/api/v1/health"); err != nil {
		t.Skipf("Master not available: %v", err)
	}

	t.Run("list provision jobs", func(t *testing.T) {
		resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/provision/jobs", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to list provision jobs: %v", err)
		}
		defer resp.Body.Close()

		expectStatusOK(t, resp)
	})

	t.Run("create provision job", func(t *testing.T) {
		job := map[string]interface{}{
			"type":     "agent",
			"target":   "new-server.example.com",
			"config":   map[string]interface{}{"port": 22, "user": "deploy"},
			"priority": 5,
		}

		resp, err := doAuthRequest("POST", cfg.MasterHTTPURL+"/api/v1/provision/jobs", job, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to create provision job: %v", err)
		}
		defer resp.Body.Close()

		expectStatusCreatedOrOK(t, resp)
	})

	t.Run("get provision job status", func(t *testing.T) {
		// Try to get a job status (may not exist)
		resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/provision/jobs/test-job-id", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to get provision job: %v", err)
		}
		defer resp.Body.Close()

		// Accept 200 or 404
		if resp.StatusCode >= 500 {
			t.Errorf("server error: %d", resp.StatusCode)
		}
	})
}

// TestAPISettings tests the settings API endpoints.
func TestAPISettings(t *testing.T) {
	cfg := getTestConfig()

	if err := waitForHTTPEndpoint(cfg.MasterHTTPURL + "/api/v1/health"); err != nil {
		t.Skipf("Master not available: %v", err)
	}

	categories := []string{"general", "appearance", "security", "notifications"}

	for _, category := range categories {
		t.Run("get "+category+" settings", func(t *testing.T) {
			resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/settings/"+category, nil, cfg.APIToken)
			if err != nil {
				t.Fatalf("failed to get %s settings: %v", category, err)
			}
			defer resp.Body.Close()

			expectStatusOK(t, resp)

			var settings map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
				t.Errorf("failed to decode settings: %v", err)
			}
		})
	}

	t.Run("update appearance settings", func(t *testing.T) {
		settings := map[string]interface{}{
			"dark_mode":    true,
			"accent_color": "blue",
		}

		resp, err := doAuthRequest("PUT", cfg.MasterHTTPURL+"/api/v1/settings/appearance", settings, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to update appearance settings: %v", err)
		}
		defer resp.Body.Close()

		expectStatusOK(t, resp)
	})

	t.Run("update general settings", func(t *testing.T) {
		settings := map[string]interface{}{
			"site_name":      "E2E Test VCDeploy",
			"session_expiry": "24h",
		}

		resp, err := doAuthRequest("PUT", cfg.MasterHTTPURL+"/api/v1/settings/general", settings, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to update general settings: %v", err)
		}
		defer resp.Body.Close()

		expectStatusOK(t, resp)
	})
}

// TestAPIHealthCheck tests the health check API endpoints.
func TestAPIHealthCheck(t *testing.T) {
	cfg := getTestConfig()

	if err := waitForHTTPEndpoint(cfg.MasterHTTPURL + "/api/v1/health"); err != nil {
		t.Skipf("Master not available: %v", err)
	}

	t.Run("get project health config", func(t *testing.T) {
		// Try to get health config for a project
		resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/projects/e2e-test-project/health-check", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to get health config: %v", err)
		}
		defer resp.Body.Close()

		// Accept 200 or 404 (if project doesn't exist)
		if resp.StatusCode >= 500 {
			t.Errorf("server error: %d", resp.StatusCode)
		}
	})

	t.Run("configure project health check", func(t *testing.T) {
		healthConfig := map[string]interface{}{
			"enabled":                true,
			"url":                    "http://localhost:8080/health",
			"method":                 "GET",
			"timeout_seconds":        30,
			"retries":                3,
			"retry_delay_seconds":    5,
			"expected_status":        200,
			"auto_rollback_enabled":  true,
			"auto_rollback_releases": 1,
		}

		resp, err := doAuthRequest("PUT", cfg.MasterHTTPURL+"/api/v1/projects/e2e-test-project/health-check", healthConfig, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to configure health check: %v", err)
		}
		defer resp.Body.Close()

		// Accept 200 or 404 (if project doesn't exist)
		if resp.StatusCode >= 500 {
			t.Errorf("server error: %d", resp.StatusCode)
		}
	})

	t.Run("run project health check", func(t *testing.T) {
		resp, err := doAuthRequest("POST", cfg.MasterHTTPURL+"/api/v1/projects/e2e-test-project/health-check/run", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to run health check: %v", err)
		}
		defer resp.Body.Close()

		// Accept various status codes (health check may fail)
		if resp.StatusCode == http.StatusNotFound {
			t.Logf("Project or health check not configured (404)")
		}
	})
}

// TestAPIAgentBinaries tests the agent binary management endpoints.
func TestAPIAgentBinaries(t *testing.T) {
	cfg := getTestConfig()

	if err := waitForHTTPEndpoint(cfg.MasterHTTPURL + "/api/v1/health"); err != nil {
		t.Skipf("Master not available: %v", err)
	}

	t.Run("list agent binaries", func(t *testing.T) {
		resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/binaries", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to list agent binaries: %v", err)
		}
		defer resp.Body.Close()

		expectStatusOK(t, resp)
	})

	t.Run("get latest agent version", func(t *testing.T) {
		resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/binaries/latest", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to get latest version: %v", err)
		}
		defer resp.Body.Close()

		// Accept 200 or 404 (if no binaries uploaded)
		if resp.StatusCode >= 500 {
			t.Errorf("server error: %d", resp.StatusCode)
		}
	})

	t.Run("check for agent updates", func(t *testing.T) {
		checkReq := map[string]interface{}{
			"current_version": "1.0.0",
			"os":              "linux",
			"arch":            "amd64",
		}

		resp, err := doAuthRequest("POST", cfg.MasterHTTPURL+"/api/v1/binaries/check-update", checkReq, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to check for updates: %v", err)
		}
		defer resp.Body.Close()

		// Accept 200 (update available or not) or 404
		if resp.StatusCode >= 500 {
			t.Errorf("server error: %d", resp.StatusCode)
		}
	})

	t.Run("trigger agent update", func(t *testing.T) {
		updateReq := map[string]interface{}{
			"version": "latest",
		}

		resp, err := doAuthRequest("POST", cfg.MasterHTTPURL+"/api/v1/agents/test-agent/update", updateReq, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to trigger agent update: %v", err)
		}
		defer resp.Body.Close()

		// Accept various status codes
		if resp.StatusCode >= 500 {
			t.Errorf("server error: %d", resp.StatusCode)
		}
	})
}

// TestAPIRollback tests the rollback API endpoints.
func TestAPIRollback(t *testing.T) {
	cfg := getTestConfig()

	if err := waitForHTTPEndpoint(cfg.MasterHTTPURL + "/api/v1/health"); err != nil {
		t.Skipf("Master not available: %v", err)
	}

	t.Run("list rollback targets", func(t *testing.T) {
		resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/projects/e2e-test-project/releases", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to list releases: %v", err)
		}
		defer resp.Body.Close()

		// Accept 200 or 404
		if resp.StatusCode >= 500 {
			t.Errorf("server error: %d", resp.StatusCode)
		}
	})

	t.Run("initiate rollback", func(t *testing.T) {
		rollbackReq := map[string]interface{}{
			"release_number": 1,
			"reason":         "E2E test rollback",
		}

		resp, err := doAuthRequest("POST", cfg.MasterHTTPURL+"/api/v1/projects/e2e-test-project/rollback", rollbackReq, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to initiate rollback: %v", err)
		}
		defer resp.Body.Close()

		// Accept various status codes
		if resp.StatusCode >= 500 {
			t.Errorf("server error: %d", resp.StatusCode)
		}
	})
}

// TestAPIUsers tests the users API endpoints.
func TestAPIUsers(t *testing.T) {
	cfg := getTestConfig()

	if err := waitForHTTPEndpoint(cfg.MasterHTTPURL + "/api/v1/health"); err != nil {
		t.Skipf("Master not available: %v", err)
	}

	t.Run("list users", func(t *testing.T) {
		resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/users", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to list users: %v", err)
		}
		defer resp.Body.Close()

		expectStatusOK(t, resp)
	})

	t.Run("create user", func(t *testing.T) {
		user := map[string]interface{}{
			"username": "e2e-test-user",
			"email":    "e2e@test.com",
			"password": "TestP@ssword123!",
			"role":     "viewer",
		}

		resp, err := doAuthRequest("POST", cfg.MasterHTTPURL+"/api/v1/users", user, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		defer resp.Body.Close()

		expectStatusCreatedOrOK(t, resp)
	})

	t.Run("get user", func(t *testing.T) {
		resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/users/e2e-test-user", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to get user: %v", err)
		}
		defer resp.Body.Close()

		// Accept 200 or 404
		if resp.StatusCode >= 500 {
			t.Errorf("server error: %d", resp.StatusCode)
		}
	})

	t.Run("update user role", func(t *testing.T) {
		update := map[string]interface{}{
			"role": "developer",
		}

		resp, err := doAuthRequest("PUT", cfg.MasterHTTPURL+"/api/v1/users/e2e-test-user", update, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to update user: %v", err)
		}
		defer resp.Body.Close()

		// Accept 200 or 404
		if resp.StatusCode >= 500 {
			t.Errorf("server error: %d", resp.StatusCode)
		}
	})
}

// TestAPIKeys tests the API keys endpoints.
func TestAPIKeys(t *testing.T) {
	cfg := getTestConfig()

	if err := waitForHTTPEndpoint(cfg.MasterHTTPURL + "/api/v1/health"); err != nil {
		t.Skipf("Master not available: %v", err)
	}

	t.Run("list API keys", func(t *testing.T) {
		resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/api-keys", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to list API keys: %v", err)
		}
		defer resp.Body.Close()

		expectStatusOK(t, resp)
	})

	t.Run("create API key", func(t *testing.T) {
		apiKey := map[string]interface{}{
			"name":        "e2e-test-key",
			"description": "E2E test API key",
			"scopes":      []string{"read:projects", "read:deployments"},
			"expires_at":  "2099-12-31T23:59:59Z",
		}

		resp, err := doAuthRequest("POST", cfg.MasterHTTPURL+"/api/v1/api-keys", apiKey, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to create API key: %v", err)
		}
		defer resp.Body.Close()

		expectStatusCreatedOrOK(t, resp)
	})

	t.Run("revoke API key", func(t *testing.T) {
		resp, err := doAuthRequest("DELETE", cfg.MasterHTTPURL+"/api/v1/api-keys/e2e-test-key", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to revoke API key: %v", err)
		}
		defer resp.Body.Close()

		// Accept 200, 204, or 404
		if resp.StatusCode >= 500 {
			t.Errorf("server error: %d", resp.StatusCode)
		}
	})
}

// TestAPIAudit tests the audit log endpoints.
func TestAPIAudit(t *testing.T) {
	cfg := getTestConfig()

	if err := waitForHTTPEndpoint(cfg.MasterHTTPURL + "/api/v1/health"); err != nil {
		t.Skipf("Master not available: %v", err)
	}

	t.Run("list audit logs", func(t *testing.T) {
		resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/audit", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to list audit logs: %v", err)
		}
		defer resp.Body.Close()

		expectStatusOK(t, resp)
	})

	t.Run("filter audit logs by action", func(t *testing.T) {
		resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/audit?action=login", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to filter audit logs: %v", err)
		}
		defer resp.Body.Close()

		expectStatusOK(t, resp)
	})

	t.Run("filter audit logs by user", func(t *testing.T) {
		resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/audit?user=admin", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to filter audit logs by user: %v", err)
		}
		defer resp.Body.Close()

		expectStatusOK(t, resp)
	})
}

// TestAPISecrets tests the secrets API endpoints.
func TestAPISecrets(t *testing.T) {
	cfg := getTestConfig()

	if err := waitForHTTPEndpoint(cfg.MasterHTTPURL + "/api/v1/health"); err != nil {
		t.Skipf("Master not available: %v", err)
	}

	t.Run("list secrets", func(t *testing.T) {
		resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/secrets", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to list secrets: %v", err)
		}
		defer resp.Body.Close()

		expectStatusOK(t, resp)
	})

	t.Run("create secret", func(t *testing.T) {
		secret := map[string]interface{}{
			"name":  "e2e-test-secret",
			"value": "super-secret-value",
			"scope": "global",
		}

		resp, err := doAuthRequest("POST", cfg.MasterHTTPURL+"/api/v1/secrets", secret, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to create secret: %v", err)
		}
		defer resp.Body.Close()

		expectStatusCreatedOrOK(t, resp)
	})

	t.Run("get secret metadata", func(t *testing.T) {
		resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/secrets/e2e-test-secret", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to get secret: %v", err)
		}
		defer resp.Body.Close()

		// Accept 200 or 404
		if resp.StatusCode >= 500 {
			t.Errorf("server error: %d", resp.StatusCode)
		}
	})
}

// TestAPIProjectTypes tests the project types API endpoints.
func TestAPIProjectTypes(t *testing.T) {
	cfg := getTestConfig()

	if err := waitForHTTPEndpoint(cfg.MasterHTTPURL + "/api/v1/health"); err != nil {
		t.Skipf("Master not available: %v", err)
	}

	t.Run("list project types", func(t *testing.T) {
		resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/project-types", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to list project types: %v", err)
		}
		defer resp.Body.Close()

		expectStatusOK(t, resp)
	})

	t.Run("create project type", func(t *testing.T) {
		projectType := map[string]interface{}{
			"name":        "e2e-nodejs",
			"description": "E2E test Node.js project type",
			"deploy_settings": map[string]interface{}{
				"strategy":      "symlink",
				"keep_releases": 5,
			},
		}

		resp, err := doAuthRequest("POST", cfg.MasterHTTPURL+"/api/v1/project-types", projectType, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to create project type: %v", err)
		}
		defer resp.Body.Close()

		expectStatusCreatedOrOK(t, resp)
	})
}

// Helper functions

func waitForHTTPEndpoint(url string) error {
	return waitForEndpoint(nil, url, 30*1000000000) // 30 seconds in nanoseconds
}

func doAuthRequest(method, url string, body interface{}, token string) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := newAuthenticatedRequest(method, url, bodyReader, token)
	if err != nil {
		return nil, err
	}

	return http.DefaultClient.Do(req)
}

func expectStatusOK(t *testing.T, resp *http.Response) {
	t.Helper()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected status 200, got %d: %s", resp.StatusCode, string(body))
	}
}

func expectStatusCreatedOrOK(t *testing.T, resp *http.Response) {
	t.Helper()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected status 200 or 201, got %d: %s", resp.StatusCode, string(body))
	}
}

// TestAPIStats tests the system statistics endpoint.
func TestAPIStats(t *testing.T) {
	cfg := getTestConfig()

	if err := waitForHTTPEndpoint(cfg.MasterHTTPURL + "/api/v1/health"); err != nil {
		t.Skipf("Master not available: %v", err)
	}

	t.Run("get system stats", func(t *testing.T) {
		resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/stats", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to get stats: %v", err)
		}
		defer resp.Body.Close()

		expectStatusOK(t, resp)

		var stats map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
			t.Errorf("failed to decode stats response: %v", err)
		}

		// Verify expected fields exist
		requiredFields := []string{"projects", "agents", "deployments"}
		for _, field := range requiredFields {
			if _, ok := stats[field]; !ok {
				t.Errorf("missing required field: %s", field)
			}
		}
	})

	t.Run("stats method not allowed", func(t *testing.T) {
		resp, err := doAuthRequest("POST", cfg.MasterHTTPURL+"/api/v1/stats", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("expected 405, got %d", resp.StatusCode)
		}
	})
}

// TestAPISettingsExportImport tests the settings export/import endpoints.
func TestAPISettingsExportImport(t *testing.T) {
	cfg := getTestConfig()

	if err := waitForHTTPEndpoint(cfg.MasterHTTPURL + "/api/v1/health"); err != nil {
		t.Skipf("Master not available: %v", err)
	}

	var exportedSettings []byte

	t.Run("export settings", func(t *testing.T) {
		resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/settings/export", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to export settings: %v", err)
		}
		defer resp.Body.Close()

		expectStatusOK(t, resp)

		// Verify content type is JSON
		contentType := resp.Header.Get("Content-Type")
		if !bytes.Contains([]byte(contentType), []byte("json")) {
			t.Errorf("expected JSON content type, got %s", contentType)
		}

		exportedSettings, err = io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read response body: %v", err)
		}

		// Verify it's valid JSON
		var settings map[string]interface{}
		if err := json.Unmarshal(exportedSettings, &settings); err != nil {
			t.Errorf("exported settings is not valid JSON: %v", err)
		}
	})

	t.Run("export settings method not allowed", func(t *testing.T) {
		resp, err := doAuthRequest("POST", cfg.MasterHTTPURL+"/api/v1/settings/export", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("expected 405, got %d", resp.StatusCode)
		}
	})

	t.Run("import settings", func(t *testing.T) {
		if len(exportedSettings) == 0 {
			t.Skip("No exported settings available")
		}

		// Import the previously exported settings
		req, err := http.NewRequest("POST", cfg.MasterHTTPURL+"/api/v1/settings/import", bytes.NewReader(exportedSettings))
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+cfg.APIToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("failed to import settings: %v", err)
		}
		defer resp.Body.Close()

		// Accept 200 OK or 409 Conflict (if settings unchanged)
		if resp.StatusCode >= 500 {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("server error: %d - %s", resp.StatusCode, string(body))
		}
	})

	t.Run("import settings method not allowed", func(t *testing.T) {
		resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/settings/import", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("expected 405, got %d", resp.StatusCode)
		}
	})

	t.Run("import invalid settings", func(t *testing.T) {
		invalidJSON := []byte(`{"invalid": }`)
		req, err := http.NewRequest("POST", cfg.MasterHTTPURL+"/api/v1/settings/import", bytes.NewReader(invalidJSON))
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+cfg.APIToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for invalid JSON, got %d", resp.StatusCode)
		}
	})
}

// TestAPIDeploymentCancel tests the deployment cancellation endpoint.
func TestAPIDeploymentCancel(t *testing.T) {
	cfg := getTestConfig()

	if err := waitForHTTPEndpoint(cfg.MasterHTTPURL + "/api/v1/health"); err != nil {
		t.Skipf("Master not available: %v", err)
	}

	t.Run("cancel nonexistent deployment", func(t *testing.T) {
		resp, err := doAuthRequest("POST", cfg.MasterHTTPURL+"/api/v1/deployments/nonexistent-deploy-id/cancel", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to cancel deployment: %v", err)
		}
		defer resp.Body.Close()

		// Should return 404 for nonexistent deployment
		if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusBadRequest {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("expected 404 or 400 for nonexistent deployment, got %d: %s", resp.StatusCode, string(body))
		}
	})

	t.Run("cancel method not allowed", func(t *testing.T) {
		resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/deployments/test/cancel", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("expected 405, got %d", resp.StatusCode)
		}
	})
}

// TestAPIAgentUpdateHistory tests the agent update history endpoint.
func TestAPIAgentUpdateHistory(t *testing.T) {
	cfg := getTestConfig()

	if err := waitForHTTPEndpoint(cfg.MasterHTTPURL + "/api/v1/health"); err != nil {
		t.Skipf("Master not available: %v", err)
	}

	t.Run("list update history", func(t *testing.T) {
		resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/agents/updates/history", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to get update history: %v", err)
		}
		defer resp.Body.Close()

		expectStatusOK(t, resp)

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Errorf("failed to decode response: %v", err)
		}
	})

	t.Run("list pending updates", func(t *testing.T) {
		resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/agents/updates/pending", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to get pending updates: %v", err)
		}
		defer resp.Body.Close()

		expectStatusOK(t, resp)
	})

	t.Run("get agent update history", func(t *testing.T) {
		resp, err := doAuthRequest("GET", cfg.MasterHTTPURL+"/api/v1/agents/test-agent/updates/history", nil, cfg.APIToken)
		if err != nil {
			t.Fatalf("failed to get agent update history: %v", err)
		}
		defer resp.Body.Close()

		// Accept 200 or 404
		if resp.StatusCode >= 500 {
			t.Errorf("server error: %d", resp.StatusCode)
		}
	})
}
