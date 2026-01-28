//go:build cli

package cli

import (
	"testing"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestSecretCommands tests secret management CLI commands.
func TestSecretCommands(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_API_URL", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_API_TOKEN", cfg.APIToken)

	t.Run("secret list", func(t *testing.T) {
		result := ctx.CLI.Run("secret", "list")
		ctx.Assertions.Success(result)
	})

	t.Run("secret create global", func(t *testing.T) {
		result := ctx.CLI.Run("secret", "create",
			"--key", "CLI_TEST_SECRET",
			"--value", "secret-value",
			"--scope", "global",
		)
		ctx.Assertions.Success(result)
	})

	t.Run("secret get", func(t *testing.T) {
		result := ctx.CLI.Run("secret", "get", "CLI_TEST_SECRET")
		ctx.Assertions.Success(result)
		// Value should NOT be shown
		ctx.Assertions.StdoutNotContains(result, "secret-value")
	})

	t.Run("secret update", func(t *testing.T) {
		result := ctx.CLI.Run("secret", "update", "CLI_TEST_SECRET",
			"--value", "updated-secret-value",
		)
		ctx.Assertions.Success(result)
	})

	t.Run("secret delete", func(t *testing.T) {
		result := ctx.CLI.Run("secret", "delete", "CLI_TEST_SECRET", "--force")
		ctx.Assertions.Success(result)
	})

	t.Run("secret create with invalid scope", func(t *testing.T) {
		result := ctx.CLI.Run("secret", "create",
			"--key", "INVALID_SCOPE",
			"--value", "value",
			"--scope", "invalid",
		)
		ctx.Assertions.Failed(result)
	})
}

// TestSecretProjectScope tests project-scoped secret CLI commands.
func TestSecretProjectScope(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_API_URL", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_API_TOKEN", cfg.APIToken)

	// Create a project first
	ctx.CLI.Run("project", "create",
		"--name", "cli-secret-project",
		"--repository", "https://github.com/test/repo.git",
		"--branch", "main",
		"--deploy-path", "/deploy/secret-test",
	)

	t.Run("secret create project-scoped", func(t *testing.T) {
		result := ctx.CLI.Run("secret", "create",
			"--key", "PROJECT_SECRET",
			"--value", "project-secret-value",
			"--scope", "project",
			"--project", "cli-secret-project",
		)
		ctx.Assertions.Success(result)
	})

	t.Run("secret list for project", func(t *testing.T) {
		result := ctx.CLI.Run("secret", "list", "--project", "cli-secret-project")
		ctx.Assertions.Success(result)
	})

	// Cleanup
	t.Cleanup(func() {
		ctx.CLI.Run("project", "delete", "cli-secret-project", "--force")
	})
}
