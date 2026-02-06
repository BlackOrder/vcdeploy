package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// rollbackCmd handles deployment rollback commands
var rollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "Deployment rollback management",
	Long: `Commands for managing deployment rollbacks.

Rollbacks allow you to revert deployments to previous versions
when issues are detected.

All commands require API authentication via --master and --token flags.`,
}

func init() {
	rootCmd.AddCommand(rollbackCmd)

	// List rollbacks
	listCmd := &cobra.Command{
		Use:   "list [project]",
		Short: "List rollbacks",
		Long: `List rollback history. Optionally filter by project.

Example:
  vcdeploy rollback list --master localhost:9000 --token <token>
  vcdeploy rollback list myproject --master localhost:9000 --token <token>`,
		RunE: runRollbackList,
	}
	listCmd.Flags().Int("limit", 20, "Maximum number of rollbacks to show")
	rollbackCmd.AddCommand(listCmd)

	// Create rollback
	createCmd := &cobra.Command{
		Use:   "create <project>",
		Short: "Create a rollback for a project",
		Long: `Rollback a project to a previous deployment version.

Example:
  vcdeploy rollback create myproject --version v1.2.3 --master localhost:9000 --token <token>
  vcdeploy rollback create myproject --deployment-id 123 --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(1),
		RunE: runRollbackCreate,
	}
	createCmd.Flags().String("version", "", "Target version to rollback to")
	createCmd.Flags().Int64("deployment-id", 0, "Target deployment ID to rollback to")
	createCmd.Flags().String("reason", "", "Reason for rollback")
	createCmd.Flags().Bool("dry-run", false, "Show what would be rolled back without executing")
	rollbackCmd.AddCommand(createCmd)

	// Status of a rollback
	statusCmd := &cobra.Command{
		Use:   "status <rollback-id>",
		Short: "Show rollback status",
		Long: `Show the status and details of a specific rollback operation.

Example:
  vcdeploy rollback status 123 --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(1),
		RunE: runRollbackStatus,
	}
	rollbackCmd.AddCommand(statusCmd)

	// Cancel a rollback
	cancelCmd := &cobra.Command{
		Use:   "cancel <rollback-id>",
		Short: "Cancel a pending rollback",
		Long: `Cancel a rollback operation that is pending or in progress.

Example:
  vcdeploy rollback cancel 123 --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(1),
		RunE: runRollbackCancel,
	}
	rollbackCmd.AddCommand(cancelCmd)
}

// --- Rollback Types ---

type rollbackListResponse struct {
	Rollbacks []rollbackInfo `json:"rollbacks"`
}

type rollbackInfo struct {
	ID           int64  `json:"id"`
	Project      string `json:"project"`
	FromVersion  string `json:"from_version"`
	ToVersion    string `json:"to_version"`
	Status       string `json:"status"`
	Reason       string `json:"reason,omitempty"`
	CreatedAt    string `json:"created_at"`
	CompletedAt  string `json:"completed_at,omitempty"`
	CreatedBy    string `json:"created_by"`
	ErrorMessage string `json:"error_message,omitempty"`
}

func runRollbackList(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	limit, _ := cmd.Flags().GetInt("limit")
	endpoint := fmt.Sprintf("/api/v1/rollbacks?limit=%d", limit)
	if len(args) > 0 {
		endpoint = fmt.Sprintf("/api/v1/projects/%s/rollbacks?limit=%d", args[0], limit)
	}

	resp, err := client.get(endpoint)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var result rollbackListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if len(result.Rollbacks) == 0 {
		fmt.Println("No rollbacks found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tPROJECT\tFROM\tTO\tSTATUS\tCREATED\tCREATED BY")
	for i := range result.Rollbacks {
		r := &result.Rollbacks[i]
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.ID, r.Project, r.FromVersion, r.ToVersion,
			rollbackStatusIcon(r.Status), r.CreatedAt, r.CreatedBy)
	}
	w.Flush()
	return nil
}

func runRollbackCreate(cmd *cobra.Command, args []string) error {
	project := args[0]
	version, _ := cmd.Flags().GetString("version")
	deploymentID, _ := cmd.Flags().GetInt64("deployment-id")
	reason, _ := cmd.Flags().GetString("reason")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	if version == "" && deploymentID == 0 {
		return fmt.Errorf("either --version or --deployment-id is required")
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	data := map[string]interface{}{
		"dry_run": dryRun,
	}
	if version != "" {
		data["version"] = version
	}
	if deploymentID != 0 {
		data["deployment_id"] = deploymentID
	}
	if reason != "" {
		data["reason"] = reason
	}

	body, _ := json.Marshal(data)
	resp, err := client.post("/api/v1/projects/"+project+"/rollbacks",
		bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var result rollbackInfo
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if dryRun {
		fmt.Println("Dry-run rollback preview:")
		fmt.Printf("  Project:      %s\n", result.Project)
		fmt.Printf("  From Version: %s\n", result.FromVersion)
		fmt.Printf("  To Version:   %s\n", result.ToVersion)
		fmt.Println("\nNo changes made (dry-run mode).")
	} else {
		fmt.Printf("Rollback created successfully (ID: %d)\n", result.ID)
		fmt.Printf("  Project:      %s\n", result.Project)
		fmt.Printf("  From Version: %s\n", result.FromVersion)
		fmt.Printf("  To Version:   %s\n", result.ToVersion)
		fmt.Printf("  Status:       %s\n", rollbackStatusIcon(result.Status))
	}
	return nil
}

func runRollbackStatus(cmd *cobra.Command, args []string) error {
	rollbackID := args[0]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/rollbacks/" + rollbackID)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var r rollbackInfo
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	fmt.Printf("Rollback ID:    %d\n", r.ID)
	fmt.Printf("Project:        %s\n", r.Project)
	fmt.Printf("From Version:   %s\n", r.FromVersion)
	fmt.Printf("To Version:     %s\n", r.ToVersion)
	fmt.Printf("Status:         %s\n", rollbackStatusIcon(r.Status))
	fmt.Printf("Created At:     %s\n", r.CreatedAt)
	fmt.Printf("Created By:     %s\n", r.CreatedBy)
	if r.CompletedAt != "" {
		fmt.Printf("Completed At:   %s\n", r.CompletedAt)
	}
	if r.Reason != "" {
		fmt.Printf("Reason:         %s\n", r.Reason)
	}
	if r.ErrorMessage != "" {
		fmt.Printf("Error:          %s\n", r.ErrorMessage)
	}
	return nil
}

func runRollbackCancel(cmd *cobra.Command, args []string) error {
	rollbackID := args[0]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.post("/api/v1/rollbacks/"+rollbackID+"/cancel", nil)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	fmt.Printf("Rollback %s cancelled successfully.\n", rollbackID)
	return nil
}

func rollbackStatusIcon(status string) string {
	switch status {
	case "completed", "success":
		return "✅ " + status
	case "pending", "scheduled":
		return "⏳ " + status
	case "in_progress", "running":
		return "🔄 " + status
	case "failed", "error":
		return "❌ " + status
	case "cancelled":
		return "🚫 " + status
	default:
		return status
	}
}
