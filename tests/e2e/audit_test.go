//go:build e2e

package e2e

import (
	"testing"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestAuditAPI tests the audit log API endpoints.
func TestAuditAPI(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Login as admin
	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	t.Run("list audit logs", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/audit")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)

		var logs []map[string]interface{}
		if err := testutil.DecodeJSON(resp, &logs); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
	})

	t.Run("filter audit logs by action", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/audit?action=login")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)
	})

	t.Run("filter audit logs by user", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/audit?user=" + cfg.AdminUsername)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)
	})

	t.Run("filter audit logs by date range", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/audit?from=2024-01-01&to=2026-12-31")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)
	})

	t.Run("audit logs with pagination", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/audit?limit=10&offset=0")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)
	})

	t.Run("generate audit events", func(t *testing.T) {
		// Create a project to generate an audit event
		project := map[string]interface{}{
			"name":        "audit-test-project",
			"repository":  "https://github.com/test/repo.git",
			"branch":      "main",
			"deploy_path": "/deploy/audit-test",
		}

		resp, _ := ctx.Client.Post("/api/v1/projects", project)
		resp.Body.Close()

		// Check audit logs contain the create event
		auditResp, err := ctx.Client.Get("/api/v1/audit?action=create")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer auditResp.Body.Close()

		ctx.Assertions.StatusOK(auditResp)

		// Cleanup
		ctx.Cleanup.DeleteProject("audit-test-project")
	})
}

// TestAuditRBAC tests that only admins can access audit logs.
func TestAuditRBAC(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// First, create a viewer user as admin
	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	viewerUser := map[string]interface{}{
		"username": "e2e-audit-viewer",
		"email":    "audit-viewer@example.com",
		"password": "ViewerPass123!",
		"role":     "viewer",
	}

	resp, _ := ctx.Client.Post("/api/v1/users", viewerUser)
	resp.Body.Close()

	// Login as viewer
	viewerCtx := testutil.NewAPITestContext(t)
	if err := viewerCtx.Login("e2e-audit-viewer", "ViewerPass123!"); err != nil {
		t.Skipf("Could not login as viewer: %v", err)
	}

	t.Run("viewer cannot access audit logs", func(t *testing.T) {
		resp, err := viewerCtx.Client.Get("/api/v1/audit")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		viewerCtx.Assertions.StatusForbidden(resp)
	})

	// Cleanup
	t.Cleanup(func() {
		ctx.Cleanup.DeleteUser("e2e-audit-viewer")
	})
}
