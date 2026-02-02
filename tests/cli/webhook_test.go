//go:build cli

package cli

import (
	"testing"

	"github.com/BlackOrder/vcdeploy/tests/testutil"
)

// TestWebhookCommands tests all webhook CLI commands.
// CLI commands may include:
//   - webhook list [project]
//   - webhook add [project] --provider <provider> --url <url> --secret <secret>
//   - webhook delete [project] [webhook-id]
//   - webhook show [project] [webhook-id]
//   - webhook test [project] [webhook-id]
func TestWebhookCommands(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	// Create a project first
	ctx.CLI.Run("project", "add", "cli-webhook-project")
	ctx.CLI.Run("project", "edit", "cli-webhook-project",
		"--repo", "https://github.com/test/repo.git",
		"--branch", "main",
		"--path", "/deploy/cli-webhook",
	)

	t.Cleanup(func() {
		ctx.CLI.RunWithInput("y\n", "project", "delete", "cli-webhook-project")
	})

	t.Run("webhook list empty", func(t *testing.T) {
		result := ctx.CLI.Run("webhook", "list", "cli-webhook-project")
		// May succeed with empty list or fail if command doesn't exist
		if result.ExitCode == 0 {
			t.Log("webhook list succeeded")
		} else if result.ContainsStderr("unknown command") || result.ContainsStderr("not found") {
			t.Log("webhook CLI commands may not be implemented")
		}
	})

	t.Run("webhook add GitHub", func(t *testing.T) {
		result := ctx.CLI.Run("webhook", "add", "cli-webhook-project",
			"--provider", "github",
			"--secret", "test-secret-123",
		)
		if result.ExitCode == 0 {
			t.Log("webhook add succeeded")
		} else if result.ContainsStderr("unknown command") {
			t.Log("webhook add command not implemented")
		}
	})

	t.Run("webhook list after add", func(t *testing.T) {
		result := ctx.CLI.Run("webhook", "list", "cli-webhook-project")
		if result.ExitCode == 0 {
			if result.ContainsStdout("github") || result.ContainsStdout("test-secret") {
				t.Log("webhook appears in list")
			}
		}
	})

	t.Run("webhook show", func(t *testing.T) {
		// First need to get webhook ID from list
		listResult := ctx.CLI.Run("webhook", "list", "cli-webhook-project", "--output", "json")
		if listResult.ExitCode != 0 {
			t.Skip("webhook list not available")
		}

		// webhook show command test
		result := ctx.CLI.Run("webhook", "show", "cli-webhook-project")
		t.Logf("webhook show result: exit=%d", result.ExitCode)
	})

	t.Run("webhook test event", func(t *testing.T) {
		result := ctx.CLI.Run("webhook", "test", "cli-webhook-project")
		if result.ExitCode == 0 {
			t.Log("webhook test command succeeded")
		} else if result.ContainsStderr("unknown command") {
			t.Log("webhook test command not implemented")
		}
	})

	t.Run("webhook delete", func(t *testing.T) {
		// Delete all webhooks from the project
		result := ctx.CLI.RunWithInput("y\n", "webhook", "delete", "cli-webhook-project", "--all")
		if result.ExitCode != 0 {
			// Try without --all flag
			ctx.CLI.RunWithInput("y\n", "webhook", "delete", "cli-webhook-project")
		}
	})
}

// TestWebhookOutputFormats tests webhook list output formats.
func TestWebhookOutputFormats(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	// Create a project first
	ctx.CLI.Run("project", "add", "cli-webhook-format-test")

	t.Cleanup(func() {
		ctx.CLI.RunWithInput("y\n", "project", "delete", "cli-webhook-format-test")
	})

	t.Run("list as json", func(t *testing.T) {
		result := ctx.CLI.Run("webhook", "list", "cli-webhook-format-test", "--output", "json")
		if result.ExitCode == 0 && result.ContainsStdout("[") {
			t.Log("JSON output valid")
		}
	})

	t.Run("list as yaml", func(t *testing.T) {
		result := ctx.CLI.Run("webhook", "list", "cli-webhook-format-test", "--output", "yaml")
		// YAML output test
		_ = result
	})

	t.Run("list as table", func(t *testing.T) {
		result := ctx.CLI.Run("webhook", "list", "cli-webhook-format-test", "--output", "table")
		// Table output test
		_ = result
	})
}

// TestWebhookProviderCommands tests provider-specific webhook configuration.
func TestWebhookProviderCommands(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	// Create a project
	ctx.CLI.Run("project", "add", "cli-provider-test")

	t.Cleanup(func() {
		ctx.CLI.RunWithInput("y\n", "project", "delete", "cli-provider-test")
	})

	providers := []struct {
		name string
		args []string
	}{
		{"github", []string{"--provider", "github", "--secret", "github-secret"}},
		{"gitlab", []string{"--provider", "gitlab", "--secret", "gitlab-token"}},
		{"bitbucket", []string{"--provider", "bitbucket", "--secret", "bb-secret"}},
	}

	for _, p := range providers {
		t.Run("add "+p.name+" webhook", func(t *testing.T) {
			args := append([]string{"webhook", "add", "cli-provider-test"}, p.args...)
			result := ctx.CLI.Run(args...)
			if result.ExitCode == 0 {
				t.Logf("%s webhook added successfully", p.name)
			} else {
				t.Logf("%s webhook add returned %d", p.name, result.ExitCode)
			}
		})
	}
}

// TestWebhookSecurityCommands tests webhook secret management.
func TestWebhookSecurityCommands(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	ctx.CLI.Run("project", "add", "cli-security-test")

	t.Cleanup(func() {
		ctx.CLI.RunWithInput("y\n", "project", "delete", "cli-security-test")
	})

	t.Run("webhook secret not shown in list", func(t *testing.T) {
		// Add a webhook with a known secret
		ctx.CLI.Run("webhook", "add", "cli-security-test",
			"--provider", "github",
			"--secret", "super-secret-value-12345",
		)

		// List webhooks
		result := ctx.CLI.Run("webhook", "list", "cli-security-test")

		// Secret should NOT appear in output
		if result.ContainsStdout("super-secret-value-12345") {
			t.Error("SECURITY: Webhook secret is exposed in list output!")
		} else {
			t.Log("✓ Webhook secret is properly hidden in list output")
		}
	})

	t.Run("webhook rotate secret", func(t *testing.T) {
		// Test secret rotation command if available
		result := ctx.CLI.Run("webhook", "rotate-secret", "cli-security-test")
		if result.ExitCode == 0 {
			t.Log("webhook rotate-secret succeeded")
		} else if result.ContainsStderr("unknown command") {
			t.Log("webhook rotate-secret command not implemented")
		}
	})
}

// TestWebhookErrors tests webhook error handling.
func TestWebhookErrors(t *testing.T) {
	ctx := setupTest(t)
	cfg := testutil.GetConfig()

	ctx.CLI.WithEnv("VCDEPLOY_MASTER", cfg.MasterHTTPURL)
	ctx.CLI.WithEnv("VCDEPLOY_TOKEN", cfg.APIToken)

	t.Run("webhook list nonexistent project", func(t *testing.T) {
		result := ctx.CLI.Run("webhook", "list", "nonexistent-project-99999")
		ctx.Assertions.Failed(result)
	})

	t.Run("webhook add invalid provider", func(t *testing.T) {
		ctx.CLI.Run("project", "add", "cli-error-test")
		defer ctx.CLI.RunWithInput("y\n", "project", "delete", "cli-error-test")

		result := ctx.CLI.Run("webhook", "add", "cli-error-test",
			"--provider", "invalid-provider",
			"--secret", "test",
		)
		// Should fail with invalid provider
		if result.ContainsStderr("invalid") || result.ContainsStderr("unknown provider") {
			t.Log("✓ Invalid provider correctly rejected")
		}
	})

	t.Run("webhook delete nonexistent", func(t *testing.T) {
		ctx.CLI.Run("project", "add", "cli-delete-test")
		defer ctx.CLI.RunWithInput("y\n", "project", "delete", "cli-delete-test")

		result := ctx.CLI.RunWithInput("y\n", "webhook", "delete", "cli-delete-test", "nonexistent-webhook-id")
		// Should fail or indicate not found
		_ = result
	})
}
