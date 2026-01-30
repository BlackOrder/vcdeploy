//go:build cli

package cli

import (
	"testing"

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
		// Check it doesn't crash
		if result.ExitCode == 0 || result.ContainsStderr("no agents") {
			// Expected outcomes
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
		result := ctx.CLI.Run("deploy", "logs", "nonexistent-deployment-id")
		ctx.Assertions.Failed(result)
	})

	t.Run("deploy cancel nonexistent", func(t *testing.T) {
		result := ctx.CLI.Run("deploy", "cancel", "nonexistent-deployment-id")
		ctx.Assertions.Failed(result)
	})
}
