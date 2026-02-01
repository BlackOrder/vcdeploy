//go:build e2e

package e2e

import (
	"testing"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestAgentsAPI tests the agents API endpoints.
func TestAgentsAPI(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Login as admin
	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	t.Run("list agents", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/agents")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)

		var agents []map[string]interface{}
		if err := testutil.DecodeJSON(resp, &agents); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
	})

	t.Run("get agent by ID", func(t *testing.T) {
		// First get list to find an agent
		resp, _ := ctx.Client.Get("/api/v1/agents")
		var agents []map[string]interface{}
		testutil.DecodeJSON(resp, &agents)
		resp.Body.Close()

		if len(agents) == 0 {
			t.Skip("no agents available")
		}

		agentID := agents[0]["id"]
		getResp, err := ctx.Client.Get("/api/v1/agents/" + agentID.(string))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer getResp.Body.Close()

		ctx.Assertions.StatusOK(getResp)
	})

	t.Run("get nonexistent agent", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/agents/nonexistent-agent-id")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusNotFound(resp)
	})

	t.Run("update agent labels", func(t *testing.T) {
		// First get list to find an agent
		resp, _ := ctx.Client.Get("/api/v1/agents")
		var agents []map[string]interface{}
		testutil.DecodeJSON(resp, &agents)
		resp.Body.Close()

		if len(agents) == 0 {
			t.Skip("no agents available")
		}

		agentID := agents[0]["id"].(string)
		updates := map[string]interface{}{
			"labels": map[string]string{
				"env":  "test",
				"role": "web",
			},
		}

		updateResp, err := ctx.Client.Put("/api/v1/agents/"+agentID, updates)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer updateResp.Body.Close()

		ctx.Assertions.StatusOK(updateResp)
	})
}

// TestAgentUpdates tests agent update management endpoints.
func TestAgentUpdates(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Login as admin
	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	t.Run("list agent update history", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/agents/updates/history")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)
	})

	t.Run("list pending updates", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/agents/updates/pending")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)
	})

	t.Run("configure agent update policy", func(t *testing.T) {
		// First get list to find an agent
		resp, _ := ctx.Client.Get("/api/v1/agents")
		var agents []map[string]interface{}
		testutil.DecodeJSON(resp, &agents)
		resp.Body.Close()

		if len(agents) == 0 {
			t.Skip("no agents available")
		}

		agentID := agents[0]["id"].(string)
		policy := map[string]interface{}{
			"updatePolicy":      "scheduled",
			"updateWindowStart": "02:00",
			"updateWindowEnd":   "04:00",
		}

		policyResp, err := ctx.Client.Put("/api/v1/agents/"+agentID+"/update-policy", policy)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer policyResp.Body.Close()

		ctx.Assertions.StatusOK(policyResp)
	})
}

// TestAgentBinaries tests agent binary management endpoints.
func TestAgentBinaries(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Login as admin
	ctx.MustLogin(cfg.AdminUsername, cfg.AdminPassword)

	t.Run("list agent binaries", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/binaries")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		ctx.Assertions.StatusOK(resp)
	})

	t.Run("get latest binary info", func(t *testing.T) {
		resp, err := ctx.Client.Get("/api/v1/binaries/latest")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		// May return 200 (with binary info) or 404 (no binaries available)
		// StatusOneOf is more specific than NoServerError
		ctx.Assertions.StatusOneOf(resp, 200, 404)
	})
}
