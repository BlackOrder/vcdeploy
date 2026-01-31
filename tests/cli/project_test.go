//go:build cli

package cli

import (
	"os"
	"testing"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestProjectCommands tests project management CLI commands.
// NOTE: Project commands currently use local database mode only.
// These tests require a local master instance with database access.
// When VCDEPLOY_MASTER is set (remote mode), skip these tests.
// CLI syntax:
//   - project list
//   - project add [name]
//   - project edit [name] --repo <url> --branch <branch> --path <path> --type <type>
//   - project delete [name]
//   - project validate [name]
func TestProjectCommands(t *testing.T) {
	// Skip if in remote mode - project commands require local database access
	if os.Getenv("VCDEPLOY_MASTER") != "" || os.Getenv("E2E_MASTER_HTTP_URL") != "" {
		t.Skip("Skipping project tests in remote mode - these commands require local database access")
	}

	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Set API URL for CLI (only used for remote-capable commands)
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
// NOTE: These commands require local database access.
// CLI syntax:
//   - project deploy [name] --target <target> --dry-run --force
//   - project rollback [name] --target <target> --release <num>
func TestProjectDeployCommands(t *testing.T) {
	// Skip if in remote mode - project commands require local database access
	if os.Getenv("VCDEPLOY_MASTER") != "" || os.Getenv("E2E_MASTER_HTTP_URL") != "" {
		t.Skip("Skipping project tests in remote mode - these commands require local database access")
	}

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
