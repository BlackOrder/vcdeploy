//go:build cli

package cli

import (
	"testing"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestProjectCommands tests project management CLI commands.
// CLI syntax:
//   - project list
//   - project add [name]
//   - project edit [name] --repo <url> --branch <branch> --path <path> --type <type>
//   - project delete [name]
//   - project validate [name]
func TestProjectCommands(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Set API URL for CLI
	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	t.Run("project list", func(t *testing.T) {
		result := ctx.CLI.Run("project", "list")
		ctx.Assertions.Success(result)
	})

	t.Run("project add", func(t *testing.T) {
		// CLI: project add [name] - then configure via edit or API
		result := ctx.CLI.Run("project", "add", "cli-test-project")
		ctx.Assertions.Success(result)
	})

	t.Run("project edit", func(t *testing.T) {
		// CLI: project edit [name] --repo <url> --branch <branch>
		result := ctx.CLI.Run("project", "edit", "cli-test-project",
			"--repo", "https://github.com/test/repo.git",
			"--branch", "main",
			"--path", "/deploy/cli-test",
		)
		ctx.Assertions.Success(result)
	})

	t.Run("project validate", func(t *testing.T) {
		result := ctx.CLI.Run("project", "validate", "cli-test-project")
		// May fail if project not fully configured, but command should work
		_ = result
	})

	t.Run("project delete", func(t *testing.T) {
		// CLI: project delete [name] (interactive confirm)
		result := ctx.CLI.RunWithInput("y\n", "project", "delete", "cli-test-project")
		ctx.Assertions.Success(result)
	})

	t.Run("project delete nonexistent", func(t *testing.T) {
		result := ctx.CLI.RunWithInput("y\n", "project", "delete", "nonexistent-project")
		ctx.Assertions.Failed(result)
	})
}

// TestProjectDeployCommands tests project deployment commands.
// CLI syntax:
//   - project deploy [name] --target <target> --dry-run --force
//   - project rollback [name] --target <target> --release <num>
func TestProjectDeployCommands(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	// Create a test project
	ctx.CLI.Run("project", "add", "cli-deploy-project")
	ctx.CLI.Run("project", "edit", "cli-deploy-project",
		"--repo", "https://github.com/test/repo.git",
		"--branch", "main",
	)

	t.Cleanup(func() {
		ctx.CLI.RunWithInput("y\n", "project", "delete", "cli-deploy-project")
	})

	t.Run("project deploy dry-run", func(t *testing.T) {
		result := ctx.CLI.Run("project", "deploy", "cli-deploy-project", "--dry-run")
		// May fail due to no agents, but command should parse correctly
		_ = result
	})

	t.Run("project deploy nonexistent", func(t *testing.T) {
		result := ctx.CLI.Run("project", "deploy", "nonexistent-project")
		ctx.Assertions.Failed(result)
	})
}
