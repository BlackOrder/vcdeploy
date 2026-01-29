//go:build cli

//nolint:nopkgdoc
package cli

import (
	"testing"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestUserCommands tests user management CLI commands.
func TestUserCommands(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Set API URL for CLI
	ctx.CLI.WithEnv("VCDEPLOY_API_URL", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_API_TOKEN", cfg.APIToken)

	t.Run("user list", func(t *testing.T) {
		result := ctx.CLI.Run("user", "list")
		ctx.Assertions.Success(result)
	})

	t.Run("user create", func(t *testing.T) {
		result := ctx.CLI.Run("user", "create",
			"--username", "cli-test-user",
			"--email", "cli-test@example.com",
			"--password", "CLITestPass123!",
			"--role", "viewer",
		)
		ctx.Assertions.Success(result)
	})

	t.Run("user get", func(t *testing.T) {
		result := ctx.CLI.Run("user", "get", "cli-test-user")
		ctx.Assertions.Success(result)
		ctx.Assertions.StdoutContains(result, "cli-test-user")
	})

	t.Run("user update", func(t *testing.T) {
		result := ctx.CLI.Run("user", "update", "cli-test-user",
			"--email", "cli-test-updated@example.com",
		)
		ctx.Assertions.Success(result)
	})

	t.Run("user change-password", func(t *testing.T) {
		result := ctx.CLI.Run("user", "change-password", "cli-test-user",
			"--password", "NewCLIPass123!",
		)
		ctx.Assertions.Success(result)
	})

	t.Run("user delete", func(t *testing.T) {
		result := ctx.CLI.Run("user", "delete", "cli-test-user", "--force")
		ctx.Assertions.Success(result)
	})

	t.Run("user get nonexistent", func(t *testing.T) {
		result := ctx.CLI.Run("user", "get", "nonexistent-user")
		ctx.Assertions.Failed(result)
	})

	t.Run("user create with invalid role", func(t *testing.T) {
		result := ctx.CLI.Run("user", "create",
			"--username", "invalid-role-user",
			"--email", "invalid@example.com",
			"--password", "Password123!",
			"--role", "superadmin",
		)
		ctx.Assertions.Failed(result)
	})

	t.Run("user create with weak password", func(t *testing.T) {
		result := ctx.CLI.Run("user", "create",
			"--username", "weak-pass-user",
			"--email", "weak@example.com",
			"--password", "123",
			"--role", "viewer",
		)
		ctx.Assertions.Failed(result)
	})
}

// TestUserListOutput tests user list output formats.
func TestUserListOutput(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_API_URL", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_API_TOKEN", cfg.APIToken)

	t.Run("list as table", func(t *testing.T) {
		result := ctx.CLI.Run("user", "list", "--output", "table")
		ctx.Assertions.Success(result)
	})

	t.Run("list as json", func(t *testing.T) {
		result := ctx.CLI.Run("user", "list", "--output", "json")
		ctx.Assertions.Success(result)
		ctx.Assertions.StdoutContains(result, "[")
	})

	t.Run("list as yaml", func(t *testing.T) {
		result := ctx.CLI.Run("user", "list", "--output", "yaml")
		ctx.Assertions.Success(result)
	})
}
