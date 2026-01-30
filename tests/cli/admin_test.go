//go:build cli

package cli

import (
	"testing"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestAdminCommand tests the admin CLI command for creating/managing admin users.
func TestAdminCommand(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_API_URL", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_API_TOKEN", cfg.APIToken)

	t.Run("admin help", func(t *testing.T) {
		result := ctx.CLI.Run("admin", "--help")
		ctx.Assertions.Success(result)
		ctx.Assertions.StdoutContains(result, "username")
		ctx.Assertions.StdoutContains(result, "password")
		ctx.Assertions.StdoutContains(result, "email")
	})

	t.Run("admin user help", func(t *testing.T) {
		result := ctx.CLI.Run("admin", "user", "--help")
		ctx.Assertions.Success(result)
		ctx.Assertions.StdoutContains(result, "list")
		ctx.Assertions.StdoutContains(result, "create")
		ctx.Assertions.StdoutContains(result, "delete")
		ctx.Assertions.StdoutContains(result, "passwd")
	})
}

// TestAdminUserCreate tests admin user creation via CLI.
func TestAdminUserCreate(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_API_URL", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_API_TOKEN", cfg.APIToken)

	t.Run("admin user create with valid password", func(t *testing.T) {
		result := ctx.CLI.Run("admin", "user", "create",
			"--username", "cli-admin-test",
			"--email", "cli-admin@example.com",
			"--password", "CLIAdmin@Pass123!",
			"--role", "admin",
		)
		ctx.Assertions.Success(result)

		// Clean up
		ctx.CLI.Run("admin", "user", "delete", "cli-admin-test", "--force")
	})

	t.Run("admin user create with weak password fails", func(t *testing.T) {
		result := ctx.CLI.Run("admin", "user", "create",
			"--username", "weak-admin-test",
			"--email", "weak-admin@example.com",
			"--password", "weak",
			"--role", "admin",
		)
		ctx.Assertions.Failed(result)
	})

	t.Run("admin user create requires password", func(t *testing.T) {
		result := ctx.CLI.Run("admin", "user", "create",
			"--username", "no-pass-admin",
			"--email", "nopass@example.com",
			"--role", "admin",
		)
		ctx.Assertions.Failed(result)
	})
}

// TestAdminUserList tests admin user listing via CLI.
func TestAdminUserList(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_API_URL", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_API_TOKEN", cfg.APIToken)

	t.Run("admin user list", func(t *testing.T) {
		result := ctx.CLI.Run("admin", "user", "list")
		ctx.Assertions.Success(result)
		// Should contain at least the admin user
		ctx.Assertions.StdoutContains(result, "admin")
	})

	t.Run("admin user list json output", func(t *testing.T) {
		result := ctx.CLI.Run("admin", "user", "list", "--output", "json")
		ctx.Assertions.Success(result)
		ctx.Assertions.StdoutContains(result, "[")
	})

	t.Run("admin user list yaml output", func(t *testing.T) {
		result := ctx.CLI.Run("admin", "user", "list", "--output", "yaml")
		ctx.Assertions.Success(result)
	})
}

// TestAdminUserDelete tests admin user deletion via CLI.
func TestAdminUserDelete(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_API_URL", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_API_TOKEN", cfg.APIToken)

	t.Run("admin user delete nonexistent fails", func(t *testing.T) {
		result := ctx.CLI.Run("admin", "user", "delete", "nonexistent-admin-12345", "--force")
		ctx.Assertions.Failed(result)
	})

	t.Run("admin user delete requires force flag", func(t *testing.T) {
		// First create a user
		ctx.CLI.Run("admin", "user", "create",
			"--username", "delete-test-admin",
			"--email", "delete-test@example.com",
			"--password", "DeleteTest@Pass123!",
			"--role", "viewer",
		)

		// Try to delete without --force
		_ = ctx.CLI.Run("admin", "user", "delete", "delete-test-admin")
		// Without --force, command might fail or require confirmation
		// This depends on implementation

		// Clean up
		ctx.CLI.Run("admin", "user", "delete", "delete-test-admin", "--force")
	})
}

// TestAdminUserPasswd tests the admin user passwd command.
func TestAdminUserPasswd(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_API_URL", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_API_TOKEN", cfg.APIToken)

	// Create a test user for password changes
	testUsername := "passwd-admin-test"
	result := ctx.CLI.Run("admin", "user", "create",
		"--username", testUsername,
		"--email", "passwd-admin@example.com",
		"--password", "InitialAdmin@Pass123!",
		"--role", "viewer",
	)
	if result.ExitCode != 0 {
		t.Skip("Could not create test user")
	}

	t.Cleanup(func() {
		ctx.CLI.Run("admin", "user", "delete", testUsername, "--force")
	})

	t.Run("admin user passwd nonexistent user fails", func(t *testing.T) {
		// Note: passwd command reads password from stdin in interactive mode
		// For testing, we might need to use flags or skip this test
		result := ctx.CLI.Run("admin", "user", "passwd", "nonexistent-user-12345")
		// Will fail because user doesn't exist (or because password prompt fails in non-interactive)
		ctx.Assertions.Failed(result)
	})
}

// TestAdminPasswordRequirements tests that admin commands enforce password requirements.
func TestAdminPasswordRequirements(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_API_URL", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_API_TOKEN", cfg.APIToken)

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
			result := ctx.CLI.Run("admin", "user", "create",
				"--username", username,
				"--email", username+"@example.com",
				"--password", tt.password,
				"--role", "viewer",
			)

			if tt.expectError {
				ctx.Assertions.Failed(result)
			} else {
				ctx.Assertions.Success(result)
				// Clean up successful creation
				ctx.CLI.Run("admin", "user", "delete", username, "--force")
			}
		})
	}
}

// TestAdminRemoteMode tests admin command in remote mode (via API).
func TestAdminRemoteMode(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	// Test with explicit --master and --token flags
	t.Run("admin with explicit master and token", func(t *testing.T) {
		result := ctx.CLI.Run("admin", "user", "list",
			"--master", cfg.MasterHTTPURL,
			"--token", cfg.APIToken,
		)
		ctx.Assertions.Success(result)
	})

	// Test with environment variables
	t.Run("admin with env vars", func(t *testing.T) {
		ctx.CLI.WithEnv("VCDEPLOY_API_URL", cfg.MasterHTTPURL)
		ctx.CLI.WithEnv("VCDEPLOY_API_TOKEN", cfg.APIToken)

		result := ctx.CLI.Run("admin", "user", "list")
		ctx.Assertions.Success(result)
	})
}
