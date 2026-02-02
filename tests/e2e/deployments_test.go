//go:build e2e

package e2e

import (
	"testing"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestDeploymentsAPI tests the deployments API endpoints.
func TestDeploymentsAPI(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Login as admin
	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	t.Run("list deployments", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/deployments")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)

		result, err := testutil.DecodePaginatedJSON(resp)
		if err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		_ = result // verify items is accessible
	})

	t.Run("filter deployments by status", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/deployments?status=completed")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)
	})

	t.Run("filter deployments by project", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/deployments?project=test-project")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)
	})

	t.Run("get deployment details", func(t *testing.T) {
		// First get list to find a deployment
		resp, _ := ctx.Client.Get("/api/v1/deployments")
		result, _ := testutil.DecodePaginatedJSON(resp)
		resp.Body.Close()

		if len(result.Items) == 0 {
			t.Skip("no deployments available")
		}

		deployID := result.Items[0]["id"].(string)
		getResp, err := ctx.Client.Get("/api/v1/deployments/" + deployID)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer getResp.Body.Close()

		ctx.Assertions.StatusOK(getResp)
	})

	t.Run("get nonexistent deployment", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/deployments/nonexistent-id")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusNotFound(resp)
	})

	t.Run("deployments with pagination", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/deployments?limit=10&offset=0")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)
	})
}

// TestDeploymentLogs tests deployment log retrieval.
func TestDeploymentLogs(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Login as admin
	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	t.Run("get deployment logs", func(t *testing.T) {
		// First get list to find a deployment
		resp, _ := ctx.Client.Get("/api/v1/deployments")
		result, _ := testutil.DecodePaginatedJSON(resp)
		resp.Body.Close()

		if len(result.Items) == 0 {
			t.Skip("no deployments available")
		}

		deployID := result.Items[0]["id"].(string)
		logsResp, err := ctx.Client.Get("/api/v1/deployments/" + deployID + "/logs")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer logsResp.Body.Close()

		ctx.Assertions.StatusOK(logsResp)
	})
}

// TestDeploymentCancel tests deployment cancellation.
func TestDeploymentCancel(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Login as admin
	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	t.Run("cancel nonexistent deployment", func(t *testing.T) {
		resp, err := ctx.Client.Post("/api/v1/deployments/nonexistent-id/cancel", nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusNotFound(resp)
	})
}

// TestRollbacks tests rollback functionality.
func TestRollbacks(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Login as admin
	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	t.Run("list rollback records", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/rollbacks")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)
	})
}
