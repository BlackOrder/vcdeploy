// Package commands implements the CLI commands for vcdeploy.
package commands

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"slices"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// webhookCmd is the parent command for webhook operations.
var webhookCmd = &cobra.Command{
	Use:   "webhook",
	Short: "Webhook configuration management",
	Long:  "Commands for managing project webhook configurations.",
}

// webhookAddCmd adds a webhook configuration to a project.
var webhookAddCmd = &cobra.Command{
	Use:   "create <project> <provider>",
	Short: "Create a webhook configuration for a project",
	Long: `Create a webhook configuration for a Git provider to a project.

Supported providers: github, gitlab, bitbucket

Examples:
  vcdeploy webhook create myproject github
  vcdeploy webhook create myproject gitlab --secret "mysecret"`,
	Args: cobra.ExactArgs(2),
	RunE: runWebhookAdd,
}

// webhookListCmd lists webhooks for a project.
var webhookListCmd = &cobra.Command{
	Use:   "list <project>",
	Short: "List webhooks for a project",
	Args:  cobra.ExactArgs(1),
	RunE:  runWebhookList,
}

// webhookDeleteCmd removes a webhook configuration.
var webhookDeleteCmd = &cobra.Command{
	Use:   "delete <project> <provider>",
	Short: "Delete a webhook configuration",
	Args:  cobra.ExactArgs(2),
	RunE:  runWebhookDelete,
}

// webhookTestCmd sends a test payload to a webhook.
var webhookTestCmd = &cobra.Command{
	Use:   "test <project> <provider>",
	Short: "Test a webhook configuration (requires running server)",
	Long: `Send a test payload to verify the webhook configuration.

This command requires a running server because it sends a test
payload to the webhook endpoint.

Example:
  vcdeploy webhook test myproject github`,
	Args: cobra.ExactArgs(2),
	RunE: runWebhookTest,
}

// webhookRotateSecretCmd rotates a webhook secret.
var webhookRotateSecretCmd = &cobra.Command{
	Use:   "rotate-secret <project> <provider>",
	Short: "Rotate the webhook secret",
	Long: `Rotate the webhook secret for a project/provider combination.

A new secret will be generated automatically. You must update this secret
in your Git provider's webhook settings.

Example:
  vcdeploy webhook rotate-secret myproject github`,
	Args: cobra.ExactArgs(2),
	RunE: runWebhookRotateSecret,
}

func init() {
	// Register webhook command with root
	rootCmd.AddCommand(webhookCmd)

	// Add subcommands
	webhookCmd.AddCommand(webhookAddCmd)
	webhookCmd.AddCommand(webhookListCmd)
	webhookCmd.AddCommand(webhookDeleteCmd)
	webhookCmd.AddCommand(webhookTestCmd)
	webhookCmd.AddCommand(webhookRotateSecretCmd)

	// Flags for webhook add
	webhookAddCmd.Flags().String("secret", "", "Webhook secret (auto-generated if not provided)")
	webhookAddCmd.Flags().Bool("require-secret", true, "Require secret validation for incoming webhooks")
}

func runWebhookAdd(cmd *cobra.Command, args []string) error {
	exec, cleanup, err := NewExecutor(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	projectName := args[0]
	provider := args[1]

	// Validate provider
	validProviders := []string{"github", "gitlab", "bitbucket"}
	if !slices.Contains(validProviders, provider) {
		return fmt.Errorf("invalid provider %q, must be one of: %v", provider, validProviders)
	}

	// Get flags
	secret, _ := cmd.Flags().GetString("secret")
	if secret == "" {
		secret = generateWebhookSecret()
	}
	requireSecret, _ := cmd.Flags().GetBool("require-secret")

	if exec.IsRemote() {
		// Use API - POST /api/v1/projects/{name}/webhooks
		payload := map[string]interface{}{
			"provider":       provider,
			"secret":         secret,
			"require_secret": requireSecret,
		}
		_, err := exec.API().Post(fmt.Sprintf("/api/v1/projects/%s/webhooks", projectName), payload)
		if err != nil {
			return fmt.Errorf("add webhook: %w", err)
		}
	} else {
		// Direct mode
		ctx := context.Background()
		project, err := exec.Services().Projects.GetByName(ctx, projectName)
		if err != nil {
			return fmt.Errorf("get project: %w", err)
		}

		err = exec.Services().Webhooks.Set(ctx, project.ID, provider, []byte(secret), true, requireSecret)
		if err != nil {
			return fmt.Errorf("create webhook: %w", err)
		}
	}

	fmt.Printf("✓ Webhook added for project %s (provider: %s)\n", projectName, provider)
	fmt.Printf("Secret: %s\n", secret)
	fmt.Println("\n⚠️  Add this secret to your Git provider's webhook settings.")
	return nil
}

func runWebhookList(cmd *cobra.Command, args []string) error {
	exec, cleanup, err := NewExecutor(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	projectName := args[0]

	if exec.IsRemote() {
		// Use API - GET /api/v1/projects/{name}/webhooks
		resp, err := exec.API().Get(fmt.Sprintf("/api/v1/projects/%s/webhooks", projectName))
		if err != nil {
			return fmt.Errorf("list webhooks: %w", err)
		}

		webhooks, ok := resp["webhooks"].([]interface{})
		if !ok || len(webhooks) == 0 {
			fmt.Println("No webhooks configured for this project.")
			return nil
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "PROVIDER\tENABLED\tREQUIRE SECRET")
		for _, wh := range webhooks {
			webhook := wh.(map[string]interface{})
			fmt.Fprintf(w, "%s\t%v\t%v\n",
				webhook["provider"],
				webhook["enabled"],
				webhook["require_secret"])
		}
		_ = w.Flush() // #nosec G104 - best effort output flush
	} else {
		// Direct mode
		ctx := context.Background()
		project, err := exec.Services().Projects.GetByName(ctx, projectName)
		if err != nil {
			return fmt.Errorf("get project: %w", err)
		}

		webhooks, err := exec.Services().Webhooks.List(ctx, project.ID)
		if err != nil {
			return fmt.Errorf("list webhooks: %w", err)
		}

		if len(webhooks) == 0 {
			fmt.Println("No webhooks configured for this project.")
			return nil
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "PROVIDER\tENABLED\tREQUIRE SECRET")
		for _, webhook := range webhooks {
			fmt.Fprintf(w, "%s\t%v\t%v\n",
				webhook.Provider,
				webhook.Enabled,
				webhook.RequireSecret)
		}
		_ = w.Flush() // #nosec G104 - best effort output flush
	}

	return nil
}

func runWebhookDelete(cmd *cobra.Command, args []string) error {
	exec, cleanup, err := NewExecutor(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	projectName := args[0]
	provider := args[1]

	if exec.IsRemote() {
		// Use API - DELETE /api/v1/projects/{name}/webhooks/{provider}
		err := exec.API().Delete(fmt.Sprintf("/api/v1/projects/%s/webhooks/%s", projectName, provider))
		if err != nil {
			return fmt.Errorf("delete webhook: %w", err)
		}
	} else {
		// Direct mode
		ctx := context.Background()
		project, err := exec.Services().Projects.GetByName(ctx, projectName)
		if err != nil {
			return fmt.Errorf("get project: %w", err)
		}

		err = exec.Services().Webhooks.Delete(ctx, project.ID, provider)
		if err != nil {
			return fmt.Errorf("delete webhook: %w", err)
		}
	}

	fmt.Printf("✓ Webhook deleted for project %s (provider: %s)\n", projectName, provider)
	return nil
}

func runWebhookTest(cmd *cobra.Command, args []string) error {
	exec, cleanup, err := NewExecutor(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	projectName := args[0]
	provider := args[1]

	// Webhook testing requires the server to be running
	// (we need to send a test payload to the webhook endpoint)
	if exec.IsOffline() {
		return fmt.Errorf("webhook test requires running server (cannot test in offline mode)")
	}

	fmt.Printf("Testing webhook for %s/%s...\n", projectName, provider)

	// Call the test endpoint
	resp, err := exec.API().Post(fmt.Sprintf("/api/v1/projects/%s/webhooks/%s/test", projectName, provider), nil)
	if err != nil {
		return fmt.Errorf("test webhook: %w", err)
	}

	// Check result
	if resp != nil {
		if success, ok := resp["success"].(bool); ok && success {
			fmt.Printf("✓ Webhook test successful for %s/%s\n", projectName, provider)
			if message, ok := resp["message"].(string); ok && message != "" {
				fmt.Printf("  %s\n", message)
			}
		} else {
			errMsg := "unknown error"
			if msg, ok := resp["message"].(string); ok {
				errMsg = msg
			}
			fmt.Printf("✗ Webhook test failed: %s\n", errMsg)
			return fmt.Errorf("webhook test failed: %s", errMsg)
		}
	} else {
		fmt.Printf("✓ Webhook test completed for %s/%s\n", projectName, provider)
	}

	return nil
}

func runWebhookRotateSecret(cmd *cobra.Command, args []string) error {
	exec, cleanup, err := NewExecutor(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	projectName := args[0]
	provider := args[1]
	newSecret := generateWebhookSecret()

	if exec.IsRemote() {
		// Use API - PUT /api/v1/projects/{name}/webhooks/{provider}/secret
		payload := map[string]interface{}{
			"secret": newSecret,
		}
		_, err := exec.API().Put(fmt.Sprintf("/api/v1/projects/%s/webhooks/%s/secret", projectName, provider), payload)
		if err != nil {
			return fmt.Errorf("rotate secret: %w", err)
		}
	} else {
		// Direct mode - get existing webhook, update secret
		ctx := context.Background()
		project, err := exec.Services().Projects.GetByName(ctx, projectName)
		if err != nil {
			return fmt.Errorf("get project: %w", err)
		}

		webhook, err := exec.Services().Webhooks.Get(ctx, project.ID, provider)
		if err != nil {
			return fmt.Errorf("get webhook: %w", err)
		}

		err = exec.Services().Webhooks.Set(ctx, project.ID, provider, []byte(newSecret), webhook.Enabled, webhook.RequireSecret)
		if err != nil {
			return fmt.Errorf("update webhook: %w", err)
		}
	}

	fmt.Printf("✓ Webhook secret rotated for %s/%s\n", projectName, provider)
	fmt.Printf("New secret: %s\n", newSecret)
	fmt.Println("\n⚠️  Update this secret in your Git provider's webhook settings.")
	return nil
}

// generateWebhookSecret generates a random webhook secret.
func generateWebhookSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}
