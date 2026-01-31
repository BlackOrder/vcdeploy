//go:build cli

package cli

import (
	"os"
	"testing"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestSecretCommands tests secret management CLI commands.
// NOTE: Secret commands currently use local database mode only.
// These tests require a local master instance with database access.
// When VCDEPLOY_MASTER is set (remote mode), skip these tests.
// CLI syntax:
//   - secret set [project/scope] [key] --stdin
//   - secret list [project]
//   - secret delete [project/scope] [key]
//   - secret import [project/scope]
//   - secret backup -o <file>
//   - secret restore [backup-file]
func TestSecretCommands(t *testing.T) {
	// Skip if in remote mode - secret commands require local database access
	if os.Getenv("VCDEPLOY_MASTER") != "" || os.Getenv("E2E_MASTER_HTTP_URL") != "" {
		t.Skip("Skipping secret tests in remote mode - these commands require local database access")
	}

	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	// Create a test project for project-scoped secrets
	ctx.CLI.Run("project", "add", "cli-secret-project")

	t.Cleanup(func() {
		ctx.CLI.RunWithInput("y\n", "project", "delete", "cli-secret-project")
	})

	t.Run("secret set global", func(t *testing.T) {
		// CLI: secret set [project/scope] [key] --stdin
		// For global scope, use "global" as the scope
		result := ctx.CLI.RunWithInput("secret-value\n", "secret", "set", "global", "CLI_TEST_SECRET", "--stdin")
		ctx.Assertions.Success(result)
	})

	t.Run("secret list global", func(t *testing.T) {
		// CLI: secret list [project] - for global use special marker
		result := ctx.CLI.Run("secret", "list", "global")
		ctx.Assertions.Success(result)
	})

	t.Run("secret delete global", func(t *testing.T) {
		// CLI: secret delete [project/scope] [key]
		result := ctx.CLI.Run("secret", "delete", "global", "CLI_TEST_SECRET")
		ctx.Assertions.Success(result)
	})

	t.Run("secret set project-scoped", func(t *testing.T) {
		result := ctx.CLI.RunWithInput("project-secret-value\n", "secret", "set", "cli-secret-project", "PROJECT_SECRET", "--stdin")
		ctx.Assertions.Success(result)
	})

	t.Run("secret list for project", func(t *testing.T) {
		result := ctx.CLI.Run("secret", "list", "cli-secret-project")
		ctx.Assertions.Success(result)
	})

	t.Run("secret delete project-scoped", func(t *testing.T) {
		result := ctx.CLI.Run("secret", "delete", "cli-secret-project", "PROJECT_SECRET")
		ctx.Assertions.Success(result)
	})
}

// TestSecretImport tests importing secrets from .env format.
// NOTE: Secret commands require local database access.
func TestSecretImport(t *testing.T) {
	// Skip if in remote mode - secret commands require local database access
	if os.Getenv("VCDEPLOY_MASTER") != "" || os.Getenv("E2E_MASTER_HTTP_URL") != "" {
		t.Skip("Skipping secret tests in remote mode - these commands require local database access")
	}

	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	// Create a test project
	ctx.CLI.Run("project", "add", "cli-import-project")

	t.Cleanup(func() {
		ctx.CLI.RunWithInput("y\n", "project", "delete", "cli-import-project")
	})

	t.Run("secret import from stdin", func(t *testing.T) {
		envContent := "IMPORT_KEY1=value1\nIMPORT_KEY2=value2\n"
		result := ctx.CLI.RunWithInput(envContent, "secret", "import", "cli-import-project")
		ctx.Assertions.Success(result)
	})
}
