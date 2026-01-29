//go:build cli

package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestConfigCommands tests configuration management CLI commands.
func TestConfigCommands(t *testing.T) {
	ctx := setupTest(t)

	t.Run("config help", func(t *testing.T) {
		result := ctx.CLI.Run("config", "--help")
		ctx.Assertions.Success(result)
		ctx.Assertions.StdoutContains(result, "export")
		ctx.Assertions.StdoutContains(result, "import")
		ctx.Assertions.StdoutContains(result, "set")
		ctx.Assertions.StdoutContains(result, "show")
	})

	t.Run("config export help", func(t *testing.T) {
		result := ctx.CLI.Run("config", "export", "--help")
		ctx.Assertions.Success(result)
	})

	t.Run("config import help", func(t *testing.T) {
		result := ctx.CLI.Run("config", "import", "--help")
		ctx.Assertions.Success(result)
	})

	t.Run("config set help", func(t *testing.T) {
		result := ctx.CLI.Run("config", "set", "--help")
		ctx.Assertions.Success(result)
	})

	t.Run("config show", func(t *testing.T) {
		cfg := testutil.GetConfig()
		ctx.CLI.WithEnv("VCDEPLOY_API_URL", cfg.MasterHTTPURL)
		ctx.CLI.WithEnv("VCDEPLOY_API_TOKEN", cfg.APIToken)

		result := ctx.CLI.Run("config", "show")
		ctx.Assertions.Success(result)
	})
}

// TestConfigExportImport tests config export and import.
func TestConfigExportImport(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_API_URL", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_API_TOKEN", cfg.APIToken)

	t.Run("config export to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")

		result := ctx.CLI.Run("config", "export", "--output", configPath)
		ctx.Assertions.Success(result)

		// Verify file was created
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Error("config file was not created")
		}
	})
}
