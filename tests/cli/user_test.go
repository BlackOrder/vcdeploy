//go:build cli

package cli

import (
	"testing"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestUserCommands tests user management CLI commands.
// CLI syntax: user create [username] -e <email> -r <role> -p <password>
func TestUserCommands(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Set API URL for CLI (uses --master and --token flags)
	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	t.Run("user list", func(t *testing.T) {
		result := ctx.CLI.Run("user", "list")
		ctx.Assertions.Success(result)
	})

	t.Run("user create", func(t *testing.T) {
		// CLI: user create [username] -e <email> -r <role> -p <password>
		result := ctx.CLI.Run("user", "create", "cli-test-user",
			"-e", "cli-test@example.com",
			"-p", "CLITestPass123!",
			"-r", "viewer",
		)
		ctx.Assertions.Success(result)
	})

	t.Run("user delete", func(t *testing.T) {
		// CLI: user delete [username] (interactive confirm, use yes via stdin)
		result := ctx.CLI.RunWithInput("y\n", "user", "delete", "cli-test-user")
		ctx.Assertions.Success(result)
	})

	t.Run("user create with invalid role", func(t *testing.T) {
		// Note: Currently the server doesn't validate roles, so any role is accepted
		// This test documents current behavior - role validation should be added to the server
		result := ctx.CLI.Run("user", "create", "invalid-role-user",
			"-e", "invalid@example.com",
			"-p", "Password123!",
			"-r", "superadmin",
		)
		ctx.Assertions.Success(result) // Server accepts any role currently
	})

	t.Run("user create with weak password", func(t *testing.T) {
		result := ctx.CLI.Run("user", "create", "weak-pass-user",
			"-e", "weak@example.com",
			"-p", "123",
			"-r", "viewer",
		)
		ctx.Assertions.Failed(result)
	})
}

// TestUserPasswordValidation tests password complexity requirements via CLI.
func TestUserPasswordValidation(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	t.Run("create user with password too short", func(t *testing.T) {
		result := ctx.CLI.Run("user", "create", "short-pass-user",
			"-e", "short@example.com",
			"-p", "Short1!", // Only 7 chars, needs 12+
			"-r", "viewer",
		)
		ctx.Assertions.Failed(result)
	})

	t.Run("create user with password missing uppercase", func(t *testing.T) {
		result := ctx.CLI.Run("user", "create", "no-upper-user",
			"-e", "noupper@example.com",
			"-p", "nouppercase123!", // Missing uppercase
			"-r", "viewer",
		)
		ctx.Assertions.Failed(result)
	})

	t.Run("create user with password missing lowercase", func(t *testing.T) {
		result := ctx.CLI.Run("user", "create", "no-lower-user",
			"-e", "nolower@example.com",
			"-p", "NOLOWERCASE123!", // Missing lowercase
			"-r", "viewer",
		)
		ctx.Assertions.Failed(result)
	})

	t.Run("create user with password missing digit", func(t *testing.T) {
		result := ctx.CLI.Run("user", "create", "no-digit-user",
			"-e", "nodigit@example.com",
			"-p", "NoDigitPassword!", // Missing digit
			"-r", "viewer",
		)
		ctx.Assertions.Failed(result)
	})

	t.Run("create user with password missing special char", func(t *testing.T) {
		result := ctx.CLI.Run("user", "create", "no-special-user",
			"-e", "nospecial@example.com",
			"-p", "NoSpecialChar123", // Missing special char
			"-r", "viewer",
		)
		ctx.Assertions.Failed(result)
	})

	t.Run("create user with strong password succeeds", func(t *testing.T) {
		result := ctx.CLI.Run("user", "create", "strong-pass-user",
			"-e", "strong@example.com",
			"-p", "StrongPassword123!",
			"-r", "viewer",
		)
		ctx.Assertions.Success(result)

		// Clean up
		ctx.CLI.RunWithInput("y\n", "user", "delete", "strong-pass-user")
	})
}

// TestUserChangePasswordCLI tests the passwd command.
// CLI: user passwd [username] -p <password>
func TestUserChangePasswordCLI(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	// Create a test user first
	testUsername := "passwd-test-user"

	result := ctx.CLI.Run("user", "create", testUsername,
		"-e", "passwd@example.com",
		"-p", "InitialPass123!",
		"-r", "viewer",
	)
	ctx.Assertions.Success(result)

	t.Cleanup(func() {
		ctx.CLI.RunWithInput("y\n", "user", "delete", testUsername)
	})

	t.Run("change password for existing user", func(t *testing.T) {
		// Use -p flag for non-interactive password change
		result := ctx.CLI.Run("user", "passwd", testUsername, "-p", "NewSecurePass123!")
		ctx.Assertions.Success(result)
	})

	t.Run("change password for nonexistent user fails", func(t *testing.T) {
		result := ctx.CLI.Run("user", "passwd", "nonexistent-user-12345", "-p", "ValidPass123!")
		ctx.Assertions.Failed(result)
	})
}
