package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// deploymentCmd handles deployment commands
var deploymentCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deployment commands",
	Long: `Commands for viewing and managing deployment records.

To trigger a new deployment, use 'project deploy' instead:
  vcdeploy project deploy myproject --branch main`,
}

func init() {
	rootCmd.AddCommand(deploymentCmd)

	// Deployment subcommands (view-only operations)
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List recent deployments",
		RunE:  runDeploymentList,
	}
	listCmd.Flags().IntP("limit", "n", 20, "Maximum number of deployments to show")
	deploymentCmd.AddCommand(listCmd)

	deploymentCmd.AddCommand(&cobra.Command{
		Use:   "show [deployment-id]",
		Short: "Show deployment details",
		Args:  cobra.ExactArgs(1),
		RunE:  runDeploymentStatus,
	})

	deploymentCmd.AddCommand(&cobra.Command{
		Use:   "cancel [deployment-id]",
		Short: "Cancel a running deployment",
		Args:  cobra.ExactArgs(1),
		RunE:  runDeploymentCancel,
	})

	deploymentCmd.AddCommand(&cobra.Command{
		Use:   "logs [deployment-id]",
		Short: "View deployment logs",
		Args:  cobra.ExactArgs(1),
		RunE:  runDeploymentLogs,
	})
}

func runDeploymentList(cmd *cobra.Command, args []string) error {
	limit, _ := cmd.Flags().GetInt("limit")
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get(fmt.Sprintf("/api/v1/deployments?limit=%d", limit))
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
	fmt.Fprintln(w, "ID\tPROJECT\tBRANCH\tSTATUS\tSTARTED")
	for _, d := range result.Items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			d["id"], d["project"], d["branch"], d["status"], d["startedAt"])
	}
	w.Flush()
	return nil
}

func runDeploymentStatus(cmd *cobra.Command, args []string) error {
	deploymentID := args[0]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/deployments/" + deploymentID)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var d map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	fmt.Printf("Deployment:   %s\n", d["id"])
	fmt.Printf("Project:      %s\n", d["project"])
	fmt.Printf("Branch:       %s\n", d["branch"])
	fmt.Printf("Target:       %s\n", d["target"])
	fmt.Printf("Status:       %s\n", d["status"])
	fmt.Printf("Started:      %s\n", d["startedAt"])
	if d["completedAt"] != nil {
		fmt.Printf("Completed:    %s\n", d["completedAt"])
	}
	if d["triggeredBy"] != nil {
		fmt.Printf("Triggered By: %s\n", d["triggeredBy"])
	}
	if d["errorMessage"] != nil && d["errorMessage"] != "" {
		fmt.Printf("Error:        %s\n", d["errorMessage"])
	}
	return nil
}

func runDeploymentCancel(cmd *cobra.Command, args []string) error {
	deploymentID := args[0]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.post("/api/v1/deployments/"+deploymentID+"/cancel", nil)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	fmt.Printf("Deployment '%s' cancelled.\n", deploymentID)
	return nil
}

func runDeploymentLogs(cmd *cobra.Command, args []string) error {
	deploymentID := args[0]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/deployments/" + deploymentID + "/logs")
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

	for _, log := range result.Items {
		timestamp := log["createdAt"]
		level := log["level"]
		message := log["message"]
		fmt.Printf("[%s] %s: %s\n", timestamp, level, message)
	}

	if len(result.Items) == 0 {
		fmt.Println("No logs available for this deployment.")
	}
	return nil
}
