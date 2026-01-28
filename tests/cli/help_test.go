//go:build cli

package cli

import (
	"testing"
)

// TestVersionCommand tests the version command.
func TestVersionCommand(t *testing.T) {
	ctx := setupTest(t)

	t.Run("show version", func(t *testing.T) {
		result := ctx.CLI.Run("version")
		ctx.Assertions.Success(result)
		ctx.Assertions.StdoutContains(result, "vcdeploy")
	})

	t.Run("version with verbose flag", func(t *testing.T) {
		result := ctx.CLI.Run("version", "-v")
		ctx.Assertions.Success(result)
	})
}

// TestHelpCommand tests the help command.
func TestHelpCommand(t *testing.T) {
	ctx := setupTest(t)

	t.Run("show help", func(t *testing.T) {
		result := ctx.CLI.Run("--help")
		ctx.Assertions.Success(result)
		ctx.Assertions.StdoutContains(result, "Usage:")
	})

	t.Run("master subcommand help", func(t *testing.T) {
		result := ctx.CLI.Run("master", "--help")
		ctx.Assertions.Success(result)
		ctx.Assertions.StdoutContains(result, "master")
	})

	t.Run("user subcommand help", func(t *testing.T) {
		result := ctx.CLI.Run("user", "--help")
		ctx.Assertions.Success(result)
	})

	t.Run("project subcommand help", func(t *testing.T) {
		result := ctx.CLI.Run("project", "--help")
		ctx.Assertions.Success(result)
	})

	t.Run("deploy subcommand help", func(t *testing.T) {
		result := ctx.CLI.Run("deploy", "--help")
		ctx.Assertions.Success(result)
	})

	t.Run("agent subcommand help", func(t *testing.T) {
		result := ctx.CLI.Run("agent", "--help")
		ctx.Assertions.Success(result)
	})

	t.Run("secret subcommand help", func(t *testing.T) {
		result := ctx.CLI.Run("secret", "--help")
		ctx.Assertions.Success(result)
	})

	t.Run("config subcommand help", func(t *testing.T) {
		result := ctx.CLI.Run("config", "--help")
		ctx.Assertions.Success(result)
	})
}

// TestInvalidCommand tests invalid command handling.
func TestInvalidCommand(t *testing.T) {
	ctx := setupTest(t)

	t.Run("unknown command", func(t *testing.T) {
		result := ctx.CLI.Run("unknowncommand")
		ctx.Assertions.Failed(result)
		ctx.Assertions.OutputContains(result, "unknown command")
	})

	t.Run("invalid flag", func(t *testing.T) {
		result := ctx.CLI.Run("--invalid-flag")
		ctx.Assertions.Failed(result)
	})
}
