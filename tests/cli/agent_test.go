//go:build cli

package cli

import (
	"testing"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestAgentCommands tests agent management CLI commands.
func TestAgentCommands(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	t.Run("agent list", func(t *testing.T) {
		result := ctx.CLI.Run("agent", "list")
		ctx.Assertions.Success(result)
	})

	t.Run("agent status", func(t *testing.T) {
		result := ctx.CLI.Run("agent", "status")
		ctx.Assertions.Success(result)
	})

	t.Run("agent show nonexistent", func(t *testing.T) {
		result := ctx.CLI.Run("agent", "show", "nonexistent-agent")
		ctx.Assertions.Failed(result)
	})
}

// TestAgentLabels tests agent label management.
func TestAgentLabels(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	// These tests require an actual agent to be running
	t.Run("agent labels set", func(t *testing.T) {
		// First get list of agents
		listResult := ctx.CLI.Run("agent", "list")
		if !listResult.ContainsStdout("id") && !listResult.ContainsStdout("ID") {
			t.Skip("no agents available")
		}

		// Would extract agent ID and set labels
	})
}

// TestAgentOutputFormats tests agent list output formats.
func TestAgentOutputFormats(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	t.Run("list as json", func(t *testing.T) {
		result := ctx.CLI.Run("agent", "list", "--output", "json")
		ctx.Assertions.Success(result)
	})

	t.Run("list as yaml", func(t *testing.T) {
		result := ctx.CLI.Run("agent", "list", "--output", "yaml")
		ctx.Assertions.Success(result)
	})

	t.Run("list as table", func(t *testing.T) {
		result := ctx.CLI.Run("agent", "list", "--output", "table")
		ctx.Assertions.Success(result)
	})
}
