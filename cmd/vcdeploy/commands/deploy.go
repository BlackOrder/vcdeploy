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

// deploymentCmd handles deployment commands
var deploymentCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deployment commands",
	Long:  "Commands for managing and triggering deployments.",
}

func init() {
	rootCmd.AddCommand(deploymentCmd)

	// Deployment subcommands
	triggerCmd := &cobra.Command{
		Use:   "trigger [project]",
		Short: "Trigger a deployment",
		Args:  cobra.ExactArgs(1),
		RunE:  runDeploymentTrigger,
	}
	triggerCmd.Flags().StringP("branch", "b", "", "Branch to deploy")
	triggerCmd.Flags().StringP("target", "t", "", "Target environment")
	triggerCmd.Flags().String("schedule", "", "Schedule deployment (RFC3339 format)")
	deploymentCmd.AddCommand(triggerCmd)

	deploymentCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List recent deployments",
		RunE:  runDeploymentList,
	})

	deploymentCmd.AddCommand(&cobra.Command{
		Use:   "status [deployment-id]",
		Short: "Get deployment status",
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

func runDeploymentTrigger(cmd *cobra.Command, args []string) error {
	project := args[0]
	branch, _ := cmd.Flags().GetString("branch")
	target, _ := cmd.Flags().GetString("target")
	schedule, _ := cmd.Flags().GetString("schedule")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	data := map[string]interface{}{
		"project": project,
	}
	if branch != "" {
		data["branch"] = branch
	}
	if target != "" {
		data["target"] = target
	}
	if schedule != "" {
		data["scheduled_at"] = schedule
	}

	body, _ := json.Marshal(data)
	resp, err := client.post("/api/v1/projects/"+project+"/deploy", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, string(bodyBytes))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if schedule != "" {
		fmt.Printf("Deployment scheduled: %s\n", result["id"])
		fmt.Printf("Scheduled for: %s\n", result["scheduled_at"])
	} else {
		fmt.Printf("Deployment triggered: %s\n", result["id"])
		fmt.Printf("Status: %s\n", result["status"])
	}
	return nil
}

func runDeploymentList(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/deployments?limit=20")
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
