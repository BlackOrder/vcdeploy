//go:build e2e

package e2e

import (
	"fmt"
	"testing"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestSecretsAPI tests the secrets API CRUD operations.
func TestSecretsAPI(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Login as admin
	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	// Create a test project first for project-scoped secrets
	project := map[string]interface{}{
		"name":        "e2e-secrets-test-project",
		"repository":  "https://github.com/test/repo.git",
		"branch":      "main",
		"deploy_path": "/deploy/secrets-test",
	}

	resp, err := ctx.Client.Post("/api/v1/projects", project)
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}
	resp.Body.Close()

	t.Run("list secrets", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/secrets")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)
	})

	var createdSecretID interface{}

	t.Run("create global secret", func(t *testing.T) {
		secret := map[string]interface{}{
			"key":   "E2E_TEST_SECRET",
			"value": "super-secret-value",
			"scope": "global",
		}

		resp, err := ctx.Client.Post("/api/v1/secrets", secret)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusCreatedOrOK(resp)

		var result map[string]interface{}
		if err := testutil.DecodeJSON(resp, &result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		createdSecretID = result["id"]
		ctx.TrackResource("secret", createdSecretID)
	})

	t.Run("create project secret", func(t *testing.T) {
		secret := map[string]interface{}{
			"key":     "PROJECT_SECRET",
			"value":   "project-secret-value",
			"scope":   "project",
			"project": "e2e-secrets-test-project",
		}

		resp, err := ctx.Client.Post("/api/v1/secrets", secret)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusCreatedOrOK(resp)
	})

	t.Run("get secret metadata", func(t *testing.T) {
		if createdSecretID == nil {
			t.Skip("no secret created")
		}

		resp, err := ctx.Client.Get(fmt.Sprintf("/api/v1/secrets/%v", createdSecretID))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)

		var secret map[string]interface{}
		if err := testutil.DecodeJSON(resp, &secret); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		ctx.Assertions.Equal(secret["key"], "E2E_TEST_SECRET")
		// Value should NOT be returned in plaintext
		if _, ok := secret["value"]; ok {
			t.Error("secret value should not be returned")
		}
	})

	t.Run("update secret", func(t *testing.T) {
		if createdSecretID == nil {
			t.Skip("no secret created")
		}

		updates := map[string]interface{}{
			"value": "updated-secret-value",
		}

		resp, err := ctx.Client.Put(fmt.Sprintf("/api/v1/secrets/%v", createdSecretID), updates)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)
	})

	t.Run("create secret with duplicate key in same scope", func(t *testing.T) {
		secret := map[string]interface{}{
			"key":   "E2E_TEST_SECRET", // Same key as before
			"value": "another-value",
			"scope": "global",
		}

		resp, err := ctx.Client.Post("/api/v1/secrets", secret)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		// Should fail with conflict
		if resp.StatusCode != 409 && resp.StatusCode != 400 {
			t.Errorf("expected 409 or 400, got %d", resp.StatusCode)
		}
	})

	t.Run("create secret with invalid scope", func(t *testing.T) {
		secret := map[string]interface{}{
			"key":   "INVALID_SCOPE_SECRET",
			"value": "value",
			"scope": "invalid-scope",
		}

		resp, err := ctx.Client.Post("/api/v1/secrets", secret)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusBadRequest(resp)
	})

	t.Run("delete secret", func(t *testing.T) {
		if createdSecretID == nil {
			t.Skip("no secret created")
		}

		resp, err := ctx.Client.Delete(fmt.Sprintf("/api/v1/secrets/%v", createdSecretID))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.NoServerError(resp)
	})

	t.Cleanup(func() {
		ctx.Cleanup.DeleteProject("e2e-secrets-test-project")
		ctx.CleanupResources()
	})
}

// TestSecretsProjectScope tests project-scoped secret operations.
func TestSecretsProjectScope(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Login as admin
	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	// Create two projects
	for i := 1; i <= 2; i++ {
		project := map[string]interface{}{
			"name":        fmt.Sprintf("e2e-scope-test-project-%d", i),
			"repository":  "https://github.com/test/repo.git",
			"branch":      "main",
			"deploy_path": fmt.Sprintf("/deploy/scope-test-%d", i),
		}
		resp, _ := ctx.Client.Post("/api/v1/projects", project)
		resp.Body.Close()
	}

	t.Run("list secrets for specific project", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/secrets?project=e2e-scope-test-project-1")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)
	})

	t.Run("same key in different projects", func(t *testing.T) {
		// Create same key in project 1
		secret1 := map[string]interface{}{
			"key":     "SHARED_KEY",
			"value":   "value-for-project-1",
			"scope":   "project",
			"project": "e2e-scope-test-project-1",
		}
		resp1, err := ctx.Client.Post("/api/v1/secrets", secret1)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp1.Body.Close()
		ctx.Assertions.StatusCreatedOrOK(resp1)

		// Create same key in project 2 - should succeed
		secret2 := map[string]interface{}{
			"key":     "SHARED_KEY",
			"value":   "value-for-project-2",
			"scope":   "project",
			"project": "e2e-scope-test-project-2",
		}
		resp2, err := ctx.Client.Post("/api/v1/secrets", secret2)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp2.Body.Close()
		ctx.Assertions.StatusCreatedOrOK(resp2)
	})

	t.Cleanup(func() {
		ctx.Cleanup.DeleteProject("e2e-scope-test-project-1")
		ctx.Cleanup.DeleteProject("e2e-scope-test-project-2")
	})
}
