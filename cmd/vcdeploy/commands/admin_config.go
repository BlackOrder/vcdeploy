package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// configCmd handles configuration commands
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management",
	Long:  "Commands for viewing and managing server configuration.",
}

func init() {
	rootCmd.AddCommand(configCmd)

	// Config subcommands
	configCmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		RunE:  runConfigShow,
	})

	configCmd.AddCommand(&cobra.Command{
		Use:   "export",
		Short: "Export configuration to JSON",
		RunE:  runConfigExport,
	})

	importConfigCmd := &cobra.Command{
		Use:   "import [file]",
		Short: "Import configuration from JSON",
		Args:  cobra.ExactArgs(1),
		RunE:  runConfigImport,
	}
	configCmd.AddCommand(importConfigCmd)

	setConfigCmd := &cobra.Command{
		Use:   "set [category.key] [value]",
		Short: "Set a configuration value",
		Args:  cobra.ExactArgs(2),
		RunE:  runConfigSet,
	}
	configCmd.AddCommand(setConfigCmd)
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/settings/export")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var settings map[string]map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	for category, values := range settings {
		fmt.Printf("[%s]\n", category)
		for key, val := range values {
			fmt.Printf("  %s = %v\n", key, val)
		}
		fmt.Println()
	}
	return nil
}

func runConfigExport(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/settings/export")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	// Output as JSON
	_, _ = io.Copy(os.Stdout, resp.Body) //nolint:errcheck // best-effort output to stdout
	fmt.Println()
	return nil
}

func runConfigImport(cmd *cobra.Command, args []string) error {
	filename := args[0]

	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.post("/api/v1/settings/import", strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	fmt.Printf("Imported %v settings successfully.\n", result["imported"])
	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	keyPath := args[0]
	value := args[1]

	parts := strings.SplitN(keyPath, ".", 2)
	if len(parts) != 2 {
		return fmt.Errorf("key must be in format 'category.key'")
	}
	category := parts[0]
	key := parts[1]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	data, _ := json.Marshal(map[string]string{key: value})
	resp, err := client.do("PUT", "/api/v1/settings/"+category, strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	fmt.Printf("Set %s.%s = %s\n", category, key, value)
	return nil
}
