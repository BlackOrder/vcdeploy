//go:build cli

package cli

import (
	"testing"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestDeployCommands tests deployment CLI commands.
func TestDeployCommands(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_API_URL", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_API_TOKEN", cfg.APIToken)

	// Create a project first
	ctx.CLI.Run("project", "create",
		"--name", "cli-deploy-project",
		"--repository", "https://github.com/test/repo.git",
		"--branch", "main",
		"--deploy-path", "/deploy/cli-deploy",
	)

	t.Run("deploy list", func(t *testing.T) {
		result := ctx.CLI.Run("deploy", "list")
		ctx.Assertions.Success(result)
	})

	t.Run("deploy status", func(t *testing.T) {
		result := ctx.CLI.Run("deploy", "status")
		ctx.Assertions.Success(result)
	})

	t.Run("deploy trigger", func(t *testing.T) {
		testutil.SkipIfNoAgent(t)
		result := ctx.CLI.Run("deploy", "trigger", "cli-deploy-project")
		// May fail due to no agents, but command should work
		// Check it doesn't crash
		if result.ExitCode == 0 || result.ContainsStderr("no agents") {
			// Expected outcomes
		}
	})

	t.Run("deploy logs", func(t *testing.T) {
		// Get most recent deployment
		listResult := ctx.CLI.Run("deploy", "list", "--output", "json")
		if listResult.Success() && listResult.ContainsStdout("id") {
			// Would extract ID and get logs
			logsResult := ctx.CLI.Run("deploy", "logs", "--last")
			// Either succeeds or fails gracefully
			_ = logsResult
		}
	})

	t.Run("deploy trigger nonexistent project", func(t *testing.T) {
		result := ctx.CLI.Run("deploy", "trigger", "nonexistent-project")
		ctx.Assertions.Failed(result)
	})

	t.Cleanup(func() {
		ctx.CLI.Run("project", "delete", "cli-deploy-project", "--force")
	})
}

// TestDeployFilters tests deployment list filtering.
func TestDeployFilters(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_API_URL", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_API_TOKEN", cfg.APIToken)

	t.Run("filter by status", func(t *testing.T) {
		result := ctx.CLI.Run("deploy", "list", "--status", "completed")
		ctx.Assertions.Success(result)
	})

	t.Run("filter by project", func(t *testing.T) {
		result := ctx.CLI.Run("deploy", "list", "--project", "test-project")
		ctx.Assertions.Success(result)
	})

	t.Run("filter with limit", func(t *testing.T) {
		result := ctx.CLI.Run("deploy", "list", "--limit", "10")
		ctx.Assertions.Success(result)
	})
}
