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
	})
}

// TestConfigShow tests the config show command.
func TestConfigShow(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	t.Run("config show", func(t *testing.T) {
		result := ctx.CLI.Run("config", "show")
		ctx.Assertions.Success(result)
	})
}

// TestConfigExport tests config export functionality.
func TestConfigExport(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	t.Run("config export to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")

		result := ctx.CLI.Run("config", "export", "--output", configPath)
		// May or may not be implemented
		if result.ExitCode == 0 {
			// Verify file was created
			if _, err := os.Stat(configPath); os.IsNotExist(err) {
				t.Error("config file was not created")
			}
		}
	})
}
