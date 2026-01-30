//go:build cli

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

// TestUserPasswordValidation tests password complexity requirements via CLI.
func TestUserPasswordValidation(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_API_URL", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_API_TOKEN", cfg.APIToken)

	t.Run("create user with password too short", func(t *testing.T) {
		result := ctx.CLI.Run("user", "create",
			"--username", "short-pass-user",
			"--email", "short@example.com",
			"--password", "Short1!", // Only 7 chars, needs 12+
			"--role", "viewer",
		)
		ctx.Assertions.Failed(result)
		// Should contain error about password length
	})

	t.Run("create user with password missing uppercase", func(t *testing.T) {
		result := ctx.CLI.Run("user", "create",
			"--username", "no-upper-user",
			"--email", "noupper@example.com",
			"--password", "nouppercase123!", // Missing uppercase
			"--role", "viewer",
		)
		ctx.Assertions.Failed(result)
	})

	t.Run("create user with password missing lowercase", func(t *testing.T) {
		result := ctx.CLI.Run("user", "create",
			"--username", "no-lower-user",
			"--email", "nolower@example.com",
			"--password", "NOLOWERCASE123!", // Missing lowercase
			"--role", "viewer",
		)
		ctx.Assertions.Failed(result)
	})

	t.Run("create user with password missing digit", func(t *testing.T) {
		result := ctx.CLI.Run("user", "create",
			"--username", "no-digit-user",
			"--email", "nodigit@example.com",
			"--password", "NoDigitPassword!", // Missing digit
			"--role", "viewer",
		)
		ctx.Assertions.Failed(result)
	})

	t.Run("create user with password missing special char", func(t *testing.T) {
		result := ctx.CLI.Run("user", "create",
			"--username", "no-special-user",
			"--email", "nospecial@example.com",
			"--password", "NoSpecialChar123", // Missing special char
			"--role", "viewer",
		)
		ctx.Assertions.Failed(result)
	})

	t.Run("create user with strong password succeeds", func(t *testing.T) {
		result := ctx.CLI.Run("user", "create",
			"--username", "strong-pass-user",
			"--email", "strong@example.com",
			"--password", "StrongPassword123!",
			"--role", "viewer",
		)
		ctx.Assertions.Success(result)

		// Clean up
		ctx.CLI.Run("user", "delete", "strong-pass-user", "--force")
	})
}

// TestUserChangePasswordCLI tests the change-password command.
func TestUserChangePasswordCLI(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_API_URL", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_API_TOKEN", cfg.APIToken)

	// Create a test user first
	initialPassword := "InitialPass123!"
	testUsername := "passwd-test-user"

	result := ctx.CLI.Run("user", "create",
		"--username", testUsername,
		"--email", "passwd@example.com",
		"--password", initialPassword,
		"--role", "viewer",
	)
	ctx.Assertions.Success(result)

	t.Cleanup(func() {
		ctx.CLI.Run("user", "delete", testUsername, "--force")
	})

	t.Run("change password for existing user", func(t *testing.T) {
		result := ctx.CLI.Run("user", "change-password", testUsername,
			"--password", "NewSecurePass123!",
		)
		ctx.Assertions.Success(result)
	})

	t.Run("change password with weak password fails", func(t *testing.T) {
		result := ctx.CLI.Run("user", "change-password", testUsername,
			"--password", "weak",
		)
		ctx.Assertions.Failed(result)
	})

	t.Run("change password for nonexistent user fails", func(t *testing.T) {
		result := ctx.CLI.Run("user", "change-password", "nonexistent-user-12345",
			"--password", "ValidPass123!",
		)
		ctx.Assertions.Failed(result)
	})
}
