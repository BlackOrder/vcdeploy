//go:build cli

package cli

import (
	"testing"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestProjectCommands tests project management CLI commands.
func TestProjectCommands(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Set API URL for CLI
	ctx.CLI.WithEnv("VCDEPLOY_API_URL", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_API_TOKEN", cfg.APIToken)

	t.Run("project list", func(t *testing.T) {
		result := ctx.CLI.Run("project", "list")
		ctx.Assertions.Success(result)
	})

	t.Run("project create", func(t *testing.T) {
		result := ctx.CLI.Run("project", "create",
			"--name", "cli-test-project",
			"--repository", "https://github.com/test/repo.git",
			"--branch", "main",
			"--deploy-path", "/deploy/cli-test",
			"--type", "nodejs",
		)
		ctx.Assertions.Success(result)
	})

	t.Run("project get", func(t *testing.T) {
		result := ctx.CLI.Run("project", "get", "cli-test-project")
		ctx.Assertions.Success(result)
		ctx.Assertions.StdoutContains(result, "cli-test-project")
	})

	t.Run("project update", func(t *testing.T) {
		result := ctx.CLI.Run("project", "update", "cli-test-project",
			"--branch", "develop",
		)
		ctx.Assertions.Success(result)
	})

	t.Run("project list agents", func(t *testing.T) {
		result := ctx.CLI.Run("project", "agents", "cli-test-project")
		ctx.Assertions.Success(result)
	})

	t.Run("project delete", func(t *testing.T) {
		result := ctx.CLI.Run("project", "delete", "cli-test-project", "--force")
		ctx.Assertions.Success(result)
	})

	t.Run("project get nonexistent", func(t *testing.T) {
		result := ctx.CLI.Run("project", "get", "nonexistent-project")
		ctx.Assertions.Failed(result)
	})

	t.Run("project create with missing fields", func(t *testing.T) {
		result := ctx.CLI.Run("project", "create",
			"--name", "incomplete-project",
		)
		ctx.Assertions.Failed(result)
	})

	t.Run("project create with invalid repository", func(t *testing.T) {
		result := ctx.CLI.Run("project", "create",
			"--name", "invalid-repo-project",
			"--repository", "not-a-url",
			"--branch", "main",
			"--deploy-path", "/deploy/invalid",
		)
		ctx.Assertions.Failed(result)
	})
}

// TestProjectOutputFormats tests project command output formats.
func TestProjectOutputFormats(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_API_URL", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_API_TOKEN", cfg.APIToken)

	t.Run("list as json", func(t *testing.T) {
		result := ctx.CLI.Run("project", "list", "--output", "json")
		ctx.Assertions.Success(result)
	})

	t.Run("list as yaml", func(t *testing.T) {
		result := ctx.CLI.Run("project", "list", "--output", "yaml")
		ctx.Assertions.Success(result)
	})
}
