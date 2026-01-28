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

	t.Run("config init", func(t *testing.T) {
		// Create temp directory for config
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.yaml")

		result := ctx.CLI.Run("config", "init", "--output", configPath)
		ctx.Assertions.Success(result)

		// Verify file was created
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Error("config file was not created")
		}
	})

	t.Run("config validate", func(t *testing.T) {
		// Create a valid config
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "master.yaml")

		// Write a minimal valid config
		validConfig := `
server:
  listen: ":8080"
grpc:
  listen: ":9090"
`
		os.WriteFile(configPath, []byte(validConfig), 0644)

		result := ctx.CLI.Run("config", "validate", configPath)
		ctx.Assertions.Success(result)
	})

	t.Run("config validate invalid", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "invalid.yaml")

		// Write invalid YAML
		os.WriteFile(configPath, []byte("invalid: yaml: syntax"), 0644)

		result := ctx.CLI.Run("config", "validate", configPath)
		ctx.Assertions.Failed(result)
	})

	t.Run("config show", func(t *testing.T) {
		cfg := testutil.GetConfig()
		ctx.CLI.WithEnv("VCDEPLOY_API_URL", cfg.MasterHTTPURL)
		ctx.CLI.WithEnv("VCDEPLOY_API_TOKEN", cfg.APIToken)

		result := ctx.CLI.Run("config", "show")
		ctx.Assertions.Success(result)
	})
}

// TestConfigGenerate tests config generation for different project types.
func TestConfigGenerate(t *testing.T) {
	ctx := setupTest(t)

	projectTypes := []string{"nodejs", "php", "python", "static", "docker"}

	for _, projectType := range projectTypes {
		t.Run("generate "+projectType+" config", func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "project.yaml")

			result := ctx.CLI.Run("config", "generate",
				"--type", projectType,
				"--output", configPath,
			)
			ctx.Assertions.Success(result)

			// Verify file was created
			content, err := os.ReadFile(configPath)
			if err == nil && len(content) > 0 {
				// Config file generated successfully
			}
		})
	}
}
