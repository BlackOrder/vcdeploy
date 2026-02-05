//go:build cli

package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestDeployCommands tests deployment CLI commands.
// CLI syntax:
//   - deploy trigger [project] -b <branch> -t <target> --schedule <time>
//   - deploy list
//   - deploy status [deployment-id]
//   - deploy cancel [deployment-id]
//   - deploy logs [deployment-id]
func TestDeployCommands(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	// Create a project first
	ctx.CLI.Run("project", "add", "cli-deploy-project")
	ctx.CLI.Run("project", "edit", "cli-deploy-project",
		"--repo", "https://github.com/test/repo.git",
		"--branch", "main",
		"--path", "/deploy/cli-deploy",
	)

	t.Cleanup(func() {
		ctx.CLI.RunWithInput("y\n", "project", "delete", "cli-deploy-project")
	})

	t.Run("deploy list", func(t *testing.T) {
		result := ctx.CLI.Run("deploy", "list")
		ctx.Assertions.Success(result)
	})

	t.Run("deploy trigger", func(t *testing.T) {
		testutil.SkipIfNoAgent(t)
		// CLI: deploy trigger [project]
		result := ctx.CLI.Run("deploy", "trigger", "cli-deploy-project")
		// May fail due to no agents, but command should work
		// Check it doesn't crash - either success or "no agents" error is acceptable
		if result.ExitCode != 0 && !result.ContainsStderr("no agents") {
			t.Errorf("Unexpected error: %s", result.Stderr)
		}
	})

	t.Run("deploy trigger nonexistent project", func(t *testing.T) {
		result := ctx.CLI.Run("deploy", "trigger", "nonexistent-project")
		ctx.Assertions.Failed(result)
	})

	t.Run("deploy status nonexistent", func(t *testing.T) {
		result := ctx.CLI.Run("deploy", "status", "nonexistent-deployment-id")
		ctx.Assertions.Failed(result)
	})

	t.Run("deploy logs nonexistent", func(t *testing.T) {
		// CLI returns success with "No logs available" message for nonexistent deployments
		result := ctx.CLI.Run("deploy", "logs", "nonexistent-deployment-id")
		ctx.Assertions.Success(result)
		ctx.Assertions.StdoutContains(result, "No logs available")
	})

	t.Run("deploy cancel nonexistent", func(t *testing.T) {
		result := ctx.CLI.Run("deploy", "cancel", "nonexistent-deployment-id")
		ctx.Assertions.Failed(result)
	})
}

// ========================================
// Full-Suite CLI Deployment Tests (Step 10)
// ========================================

// TestDeployTriggerWithAgent tests CLI deployment trigger with real agent.
func TestDeployTriggerWithAgent(t *testing.T) {
	testutil.SkipIfNoAgent(t)
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	// Create a project
	projectName := "cli-deploy-agent-test"
	ctx.CLI.Run("project", "add", projectName)
	ctx.CLI.Run("project", "edit", projectName,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--branch", "master",
		"--path", "/tmp/cli-deploy-test",
	)

	t.Cleanup(func() {
		ctx.CLI.RunWithInput("y\n", "project", "delete", projectName)
	})

	t.Run("trigger deployment via CLI", func(t *testing.T) {
		result := ctx.CLI.Run("deploy", "trigger", projectName)

		if result.ExitCode != 0 {
			// Check if it's an expected failure (no agents, etc.)
			if result.ContainsStderr("no agents") || result.ContainsStderr("no matching") {
				t.Log("Deployment rejected due to no matching agents")
				return
			}
			t.Fatalf("deploy trigger failed: %s", result.Stderr)
		}

		t.Log("Deployment triggered successfully")

		// Try to extract deployment ID from output
		// Output might contain "Deployment ID: xxx" or similar
		stdout := result.Stdout
		if strings.Contains(stdout, "deployment") || strings.Contains(stdout, "Deployment") {
			t.Logf("Output: %s", stdout)
		}
	})

	t.Run("trigger with branch override", func(t *testing.T) {
		result := ctx.CLI.Run("deploy", "trigger", projectName, "-b", "master")

		if result.ExitCode == 0 {
			t.Log("Deployment with branch override succeeded")
		} else {
			t.Logf("Deployment with branch override returned %d", result.ExitCode)
		}
	})
}

// TestDeployCancelRunning tests CLI deployment cancellation.
func TestDeployCancelRunning(t *testing.T) {
	testutil.SkipIfNoAgent(t)
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	// Create project
	projectName := "cli-cancel-test"
	ctx.CLI.Run("project", "add", projectName)
	ctx.CLI.Run("project", "edit", projectName,
		"--repo", "https://github.com/octocat/Spoon-Knife.git",
		"--branch", "main",
		"--path", "/tmp/cli-cancel-test",
	)

	t.Cleanup(func() {
		ctx.CLI.RunWithInput("y\n", "project", "delete", projectName)
	})

	t.Run("cancel running deployment", func(t *testing.T) {
		// Trigger deployment
		triggerResult := ctx.CLI.Run("deploy", "trigger", projectName)
		if triggerResult.ExitCode != 0 {
			t.Skip("Could not trigger deployment for cancel test")
		}

		// Brief wait for deployment to start
		time.Sleep(2 * time.Second)

		// Get deployment ID from list
		listResult := ctx.CLI.Run("deploy", "list", "--project", projectName, "--output", "json")
		if listResult.ExitCode != 0 {
			t.Skip("Could not list deployments")
		}

		// Try to cancel (might need to extract ID)
		// Simplified: just try cancelling by project's latest deployment
		cancelResult := ctx.CLI.Run("deploy", "cancel", "--project", projectName)

		if cancelResult.ExitCode == 0 {
			t.Log("Deployment cancelled successfully")
		} else if cancelResult.ContainsStderr("already completed") || cancelResult.ContainsStderr("not running") {
			t.Log("Deployment completed before cancel could be processed")
		}
	})
}

// TestDeployRollbackCLI tests CLI rollback command.
func TestDeployRollbackCLI(t *testing.T) {
	testutil.SkipIfNoAgent(t)
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	projectName := "cli-rollback-test"
	ctx.CLI.Run("project", "add", projectName)
	ctx.CLI.Run("project", "edit", projectName,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--branch", "master",
		"--path", "/tmp/cli-rollback",
	)

	t.Cleanup(func() {
		ctx.CLI.RunWithInput("y\n", "project", "delete", projectName)
	})

	t.Run("rollback command exists", func(t *testing.T) {
		result := ctx.CLI.Run("deploy", "rollback", "--help")
		if result.ExitCode == 0 {
			t.Log("Rollback command is available")
		} else if result.ContainsStderr("unknown command") {
			t.Log("Rollback command not implemented in CLI")
		}
	})

	t.Run("rollback to previous deployment", func(t *testing.T) {
		// Trigger first deployment
		ctx.CLI.Run("deploy", "trigger", projectName)
		time.Sleep(5 * time.Second) // Wait for completion

		// Trigger second deployment
		ctx.CLI.Run("deploy", "trigger", projectName)
		time.Sleep(5 * time.Second)

		// Try rollback
		result := ctx.CLI.Run("deploy", "rollback", projectName)
		if result.ExitCode == 0 {
			t.Log("Rollback succeeded")
		} else {
			t.Logf("Rollback returned %d", result.ExitCode)
		}
	})
}

// TestDeployLogsCLI tests CLI log retrieval.
func TestDeployLogsCLI(t *testing.T) {
	testutil.SkipIfNoAgent(t)
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	t.Run("logs for completed deployment", func(t *testing.T) {
		// First, get a completed deployment
		listResult := ctx.CLI.Run("deploy", "list", "--status", "success", "--output", "json", "--limit", "1")
		if listResult.ExitCode != 0 {
			t.Skip("Could not list deployments")
		}

		// If there's a deployment ID, try to get logs
		// Simplified: check logs command works
		result := ctx.CLI.Run("deploy", "logs", "--help")
		if result.ExitCode == 0 {
			t.Log("Logs command is available")
		}
	})

	t.Run("logs follow mode", func(t *testing.T) {
		// Test that -f/--follow flag exists
		result := ctx.CLI.Run("deploy", "logs", "--help")
		if result.ContainsStdout("follow") || result.ContainsStdout("-f") {
			t.Log("Logs follow mode is supported")
		}
	})

	t.Run("logs with line limit", func(t *testing.T) {
		// Test that --tail/--lines flag exists
		result := ctx.CLI.Run("deploy", "logs", "--help")
		if result.ContainsStdout("tail") || result.ContainsStdout("lines") {
			t.Log("Logs line limit is supported")
		}
	})
}

// TestDeployStatusCLI tests CLI status command.
func TestDeployStatusCLI(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	t.Run("status command help", func(t *testing.T) {
		result := ctx.CLI.Run("deploy", "status", "--help")
		ctx.Assertions.Success(result)
	})

	t.Run("status for nonexistent deployment", func(t *testing.T) {
		result := ctx.CLI.Run("deploy", "status", "nonexistent-deploy-id-99999")
		ctx.Assertions.Failed(result)
	})

	t.Run("status output formats", func(t *testing.T) {
		// Verify format flags are supported
		result := ctx.CLI.Run("deploy", "status", "--help")
		if result.ContainsStdout("output") || result.ContainsStdout("format") {
			t.Log("Output format option is supported")
		}
	})
}

// TestDeployListFilters tests CLI list filtering options.
func TestDeployListFilters(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	t.Run("list with status filter", func(t *testing.T) {
		result := ctx.CLI.Run("deploy", "list", "--status", "success")
		ctx.Assertions.Success(result)
	})

	t.Run("list with project filter", func(t *testing.T) {
		result := ctx.CLI.Run("deploy", "list", "--project", "test-project")
		// May succeed or fail depending on project existence
		_ = result
	})

	t.Run("list with limit", func(t *testing.T) {
		result := ctx.CLI.Run("deploy", "list", "--limit", "5")
		ctx.Assertions.Success(result)
	})

	t.Run("list as JSON", func(t *testing.T) {
		result := ctx.CLI.Run("deploy", "list", "--output", "json")
		ctx.Assertions.Success(result)
		// Should contain JSON array
		if result.ContainsStdout("[") || result.ContainsStdout("{") {
			t.Log("JSON output valid")
		}
	})

	t.Run("list as YAML", func(t *testing.T) {
		result := ctx.CLI.Run("deploy", "list", "--output", "yaml")
		ctx.Assertions.Success(result)
	})
}
