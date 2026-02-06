package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// apiKeyCmd handles API key commands
var apiKeyCmd = &cobra.Command{
	Use:   "api-key",
	Short: "API key management",
	Long:  "Commands for managing API keys.",
}

func init() {
	rootCmd.AddCommand(apiKeyCmd)

	// API Key subcommands
	apiKeyCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all API keys",
		RunE:  runAPIKeyList,
	})

	createKeyCmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a new API key",
		Args:  cobra.ExactArgs(1),
		RunE:  runAPIKeyCreate,
	}
	createKeyCmd.Flags().Int("expires", 0, "Days until expiry (0 = never)")
	apiKeyCmd.AddCommand(createKeyCmd)

	apiKeyCmd.AddCommand(&cobra.Command{
		Use:   "revoke [key-id]",
		Short: "Revoke an API key",
		Args:  cobra.ExactArgs(1),
		RunE:  runAPIKeyRevoke,
	})
}

func runAPIKeyList(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/api-keys")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var result paginatedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tCREATED\tEXPIRES\tLAST USED")
	for _, k := range result.Items {
		expires := "never"
		if k["expiresAt"] != nil {
			expires = fmt.Sprintf("%v", k["expiresAt"])
		}
		lastUsed := "never"
		if k["lastUsedAt"] != nil {
			lastUsed = fmt.Sprintf("%v", k["lastUsedAt"])
		}
		fmt.Fprintf(w, "%.0f\t%s\t%s\t%s\t%s\n",
			k["id"], k["name"], k["createdAt"], expires, lastUsed)
	}
	w.Flush()

	if len(result.Items) == 0 {
		fmt.Println("No API keys found.")
	}
	return nil
}

func runAPIKeyCreate(cmd *cobra.Command, args []string) error {
	name := args[0]
	expiresIn, _ := cmd.Flags().GetInt("expires")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	data, _ := json.Marshal(map[string]interface{}{
		"name":            name,
		"expires_in_days": expiresIn,
	})

	resp, err := client.post("/api/v1/api-keys", strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	fmt.Println("API Key created successfully!")
	fmt.Println()
	fmt.Printf("  Name: %s\n", result["name"])
	fmt.Printf("  Key:  %s\n", result["key"])
	fmt.Println()
	fmt.Println("IMPORTANT: Save this key now. You won't be able to see it again!")
	return nil
}

func runAPIKeyRevoke(cmd *cobra.Command, args []string) error {
	keyID := args[0]

	fmt.Printf("Are you sure you want to revoke API key '%s'? (y/N): ", keyID)
	var confirm string
	_, _ = fmt.Scanln(&confirm) //nolint:errcheck // user confirmation prompt
	if !strings.EqualFold(confirm, "y") {
		return fmt.Errorf("aborted")
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.delete("/api/v1/api-keys/" + keyID)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	fmt.Printf("API key '%s' revoked successfully.\n", keyID)
	return nil
}
