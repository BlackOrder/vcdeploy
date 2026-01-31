//go:build cli

package cli

import (
	"testing"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestAdminCommand tests the admin CLI command for creating/managing admin users.
// The admin command works differently - it's for initial admin setup.
// For user management via API, use the `user` command instead.
func TestAdminCommand(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	t.Run("admin help", func(t *testing.T) {
		result := ctx.CLI.Run("admin", "--help")
		ctx.Assertions.Success(result)
		ctx.Assertions.StdoutContains(result, "username")
		ctx.Assertions.StdoutContains(result, "password")
		ctx.Assertions.StdoutContains(result, "email")
	})
}

// TestUserManagementViaCLI tests user management using the `user` command.
// CLI syntax:
//   - user list
//   - user create [username] -e <email> -r <role> -p <password>
//   - user delete [username]
//   - user passwd [username]
func TestUserManagementViaCLI(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	t.Run("user list", func(t *testing.T) {
		result := ctx.CLI.Run("user", "list")
		ctx.Assertions.Success(result)
		// Should contain at least the admin user
		ctx.Assertions.StdoutContains(result, "admin")
	})

	t.Run("user create with valid password", func(t *testing.T) {
		result := ctx.CLI.Run("user", "create", "cli-admin-test",
			"-e", "cli-admin@example.com",
			"-p", "CLIAdmin@Pass123!",
			"-r", "admin",
		)
		ctx.Assertions.Success(result)

		// Clean up
		ctx.CLI.RunWithInput("y\n", "user", "delete", "cli-admin-test")
	})

	t.Run("user create with weak password fails", func(t *testing.T) {
		result := ctx.CLI.Run("user", "create", "weak-admin-test",
			"-e", "weak-admin@example.com",
			"-p", "weak",
			"-r", "admin",
		)
		ctx.Assertions.Failed(result)
	})
}

// TestUserDelete tests user deletion via CLI.
func TestUserDelete(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	t.Run("user delete nonexistent fails", func(t *testing.T) {
		result := ctx.CLI.RunWithInput("y\n", "user", "delete", "nonexistent-admin-12345")
		ctx.Assertions.Failed(result)
	})

	t.Run("user delete with confirmation", func(t *testing.T) {
		// First create a user
		ctx.CLI.Run("user", "create", "delete-test-user",
			"-e", "delete-test@example.com",
			"-p", "DeleteTest@Pass123!",
			"-r", "viewer",
		)

		// Delete with confirmation via stdin
		result := ctx.CLI.RunWithInput("y\n", "user", "delete", "delete-test-user")
		ctx.Assertions.Success(result)
	})
}

// TestUserPasswd tests the user passwd command.
func TestUserPasswd(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	// Create a test user for password changes
	testUsername := "passwd-admin-test"
	result := ctx.CLI.Run("user", "create", testUsername,
		"-e", "passwd-admin@example.com",
		"-p", "InitialAdmin@Pass123!",
		"-r", "viewer",
	)
	if result.ExitCode != 0 {
		t.Skip("Could not create test user")
	}

	t.Cleanup(func() {
		ctx.CLI.RunWithInput("y\n", "user", "delete", testUsername)
	})

	t.Run("user passwd for existing user", func(t *testing.T) {
		// Use -p flag for non-interactive password change
		result := ctx.CLI.Run("user", "passwd", testUsername, "-p", "NewAdmin@Pass123!")
		ctx.Assertions.Success(result)
	})

	t.Run("user passwd nonexistent user fails", func(t *testing.T) {
		result := ctx.CLI.Run("user", "passwd", "nonexistent-user-12345", "-p", "ValidPass123!")
		ctx.Assertions.Failed(result)
	})
}

// TestPasswordRequirements tests that CLI enforces password requirements.
func TestPasswordRequirements(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	tests := []struct {
		name        string
		password    string
		expectError bool
		description string
	}{
		{
			name:        "valid strong password",
			password:    "ValidStrong@Pass123!",
			expectError: false,
			description: "meets all requirements",
		},
		{
			name:        "too short",
			password:    "Short1!",
			expectError: true,
			description: "less than 12 characters",
		},
		{
			name:        "missing uppercase",
			password:    "nouppercase123!",
			expectError: true,
			description: "no uppercase letter",
		},
		{
			name:        "missing lowercase",
			password:    "NOLOWERCASE123!",
			expectError: true,
			description: "no lowercase letter",
		},
		{
			name:        "missing digit",
			password:    "NoDigitPassword!",
			expectError: true,
			description: "no digit",
		},
		{
			name:        "missing special",
			password:    "NoSpecialChar123",
			expectError: true,
			description: "no special character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			username := "pw-test-" + tt.name[:5]
			result := ctx.CLI.Run("user", "create", username,
				"-e", username+"@example.com",
				"-p", tt.password,
				"-r", "viewer",
			)

			if tt.expectError {
				ctx.Assertions.Failed(result)
			} else {
				ctx.Assertions.Success(result)
				// Clean up successful creation
				ctx.CLI.RunWithInput("y\n", "user", "delete", username)
			}
		})
	}
}

// TestRemoteMode tests CLI in remote mode (via API).
func TestRemoteMode(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Test with explicit --master and --token flags
	t.Run("with explicit master and token", func(t *testing.T) {
		result := ctx.CLI.Run("user", "list",
			"--master", cfg.MasterHTTPURL,
			"--token", cfg.APIToken,
		)
		ctx.Assertions.Success(result)
	})

	// Test with environment variables
	t.Run("with env vars", func(t *testing.T) {
		ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
		ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

		result := ctx.CLI.Run("user", "list")
		ctx.Assertions.Success(result)
	})
}
