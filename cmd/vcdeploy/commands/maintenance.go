package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(maintenanceCmd)
	maintenanceCmd.AddCommand(maintenanceEnableCmd)
	maintenanceCmd.AddCommand(maintenanceDisableCmd)
	maintenanceCmd.AddCommand(maintenanceStatusCmd)

	maintenanceCmd.PersistentFlags().String("master", "", "Master server address (or VCDEPLOY_MASTER)")
	maintenanceCmd.PersistentFlags().String("token", "", "API token (or VCDEPLOY_TOKEN)")
}

var maintenanceCmd = &cobra.Command{
	Use:   "maintenance",
	Short: "Maintenance mode management",
	Long:  "Commands for enabling, disabling, and querying maintenance mode on the master server.",
}

var maintenanceEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable maintenance mode",
	Long:  "Enables maintenance mode, blocking all mutations. Pending writes are flushed to disk.",
	RunE:  runMaintenanceEnable,
}

var maintenanceDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable maintenance mode",
	Long:  "Disables maintenance mode, restoring normal operation.",
	RunE:  runMaintenanceDisable,
}

var maintenanceStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show maintenance mode status",
	RunE:  runMaintenanceStatus,
}

func runMaintenanceEnable(cmd *cobra.Command, _ []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	body, _ := json.Marshal(map[string]bool{"enabled": true})
	resp, err := client.post("/api/v1/admin/maintenance", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to contact server: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(data))
	}

	fmt.Println("✅ Maintenance mode enabled. Pending writes flushed to disk.")
	return nil
}

func runMaintenanceDisable(cmd *cobra.Command, _ []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	body, _ := json.Marshal(map[string]bool{"enabled": false})
	resp, err := client.post("/api/v1/admin/maintenance", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to contact server: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(data))
	}

	fmt.Println("✅ Maintenance mode disabled. Normal operation resumed.")
	return nil
}

func runMaintenanceStatus(cmd *cobra.Command, _ []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/admin/maintenance")
	if err != nil {
		return fmt.Errorf("failed to contact server: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	maintenance, _ := result["maintenance"].(bool)
	if maintenance {
		fmt.Println("🔧 Maintenance mode: ENABLED")
		fmt.Println("   Mutations are blocked. Read operations still work.")
	} else {
		fmt.Println("✅ Maintenance mode: DISABLED")
		fmt.Println("   Server is operating normally.")
	}
	return nil
}
