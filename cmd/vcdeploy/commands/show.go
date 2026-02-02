// Package commands implements the CLI commands for vcdeploy.
package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// showCmd handles show commands for detailed views
var showCmd = &cobra.Command{
	Use:   "show",
	Short: "Show detailed information",
	Long:  "Commands for showing detailed information about resources.",
}

// auditCmd handles audit log commands
var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Audit log management",
	Long:  "Commands for viewing and exporting audit logs.",
}

func init() {
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(auditCmd)

	// Show subcommands
	showCmd.AddCommand(&cobra.Command{
		Use:   "project [name]",
		Short: "Show detailed project information",
		Args:  cobra.ExactArgs(1),
		RunE:  runShowProject,
	})
	showCmd.AddCommand(&cobra.Command{
		Use:   "agent [id]",
		Short: "Show detailed agent information",
		Args:  cobra.ExactArgs(1),
		RunE:  runShowAgentDetail,
	})
	showCmd.AddCommand(&cobra.Command{
		Use:   "deployment [id]",
		Short: "Show detailed deployment information",
		Args:  cobra.ExactArgs(1),
		RunE:  runShowDeployment,
	})

	// Audit subcommands
	auditListCmd := &cobra.Command{
		Use:   "list",
		Short: "List audit log entries",
		RunE:  runAuditList,
	}
	auditListCmd.Flags().IntP("limit", "n", 50, "Maximum entries to show")
	auditListCmd.Flags().String("action", "", "Filter by action")
	auditListCmd.Flags().String("resource", "", "Filter by resource type")
	auditCmd.AddCommand(auditListCmd)

	auditExportCmd := &cobra.Command{
		Use:   "export [file]",
		Short: "Export audit logs to JSON file",
		Args:  cobra.ExactArgs(1),
		RunE:  runAuditExport,
	}
	auditExportCmd.Flags().IntP("limit", "n", 1000, "Maximum entries to export")
	auditCmd.AddCommand(auditExportCmd)
}

// runShowProject shows detailed project information.
func runShowProject(cmd *cobra.Command, args []string) error {
	name := args[0]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/projects/" + name)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, body)
	}

	var project map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		return fmt.Errorf("decode error: %w", err)
	}

	fmt.Printf("Project: %s\n", name)
	fmt.Println("==========" + fmt.Sprintf("%*s", len(name), "")[1:])
	fmt.Printf("  ID:           %v\n", project["id"])
	fmt.Printf("  Repository:   %v\n", project["repository"])
	fmt.Printf("  Branch:       %v\n", project["branch"])
	fmt.Printf("  Deploy Path:  %v\n", project["deploy_path"])
	fmt.Printf("  Type:         %v\n", project["type"])
	fmt.Printf("  Enabled:      %v\n", project["enabled"])
	fmt.Printf("  Created:      %v\n", project["created_at"])
	fmt.Printf("  Updated:      %v\n", project["updated_at"])

	if targets, ok := project["targets"].([]interface{}); ok && len(targets) > 0 {
		fmt.Println("\n  Targets:")
		for _, t := range targets {
			if target, ok := t.(map[string]interface{}); ok {
				fmt.Printf("    - %v (%v)\n", target["name"], target["address"])
			}
		}
	}

	return nil
}

// runShowAgentDetail shows detailed agent information.
func runShowAgentDetail(cmd *cobra.Command, args []string) error {
	id := args[0]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/agents/" + id)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, body)
	}

	var agent map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&agent); err != nil {
		return fmt.Errorf("decode error: %w", err)
	}

	fmt.Printf("Agent: %s\n", id)
	fmt.Println("========" + fmt.Sprintf("%*s", len(id), "")[1:])
	fmt.Printf("  Hostname:     %v\n", agent["hostname"])
	fmt.Printf("  Status:       %v\n", agent["status"])
	fmt.Printf("  Last Seen:    %v\n", agent["lastSeenAt"])
	fmt.Printf("  Registered:   %v\n", agent["registeredAt"])

	if labels, ok := agent["labels"].(map[string]interface{}); ok && len(labels) > 0 {
		fmt.Println("  Labels:")
		for k, v := range labels {
			fmt.Printf("    %s: %v\n", k, v)
		}
	}

	return nil
}

// runShowDeployment shows detailed deployment information.
func runShowDeployment(cmd *cobra.Command, args []string) error {
	id := args[0]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/deployments/" + id)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, body)
	}

	var deployment map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&deployment); err != nil {
		return fmt.Errorf("decode error: %w", err)
	}

	fmt.Printf("Deployment: %s\n", id)
	fmt.Println("============" + fmt.Sprintf("%*s", len(id), "")[1:])
	fmt.Printf("  Project:      %v\n", deployment["projectName"])
	fmt.Printf("  Status:       %v\n", deployment["status"])
	fmt.Printf("  Commit:       %v\n", deployment["commit"])
	fmt.Printf("  Branch:       %v\n", deployment["branch"])
	fmt.Printf("  Triggered By: %v\n", deployment["triggeredBy"])
	fmt.Printf("  Started:      %v\n", deployment["startedAt"])
	fmt.Printf("  Completed:    %v\n", deployment["completedAt"])

	return nil
}

// runAuditList lists audit log entries.
func runAuditList(cmd *cobra.Command, args []string) error {
	limit, _ := cmd.Flags().GetInt("limit")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get(fmt.Sprintf("/api/v1/audit?limit=%d", limit))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, body)
	}

	var result paginatedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode error: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TIMESTAMP\tUSER\tACTION\tRESOURCE\tRESULT")
	for _, e := range result.Items {
		fmt.Fprintf(w, "%v\t%v\t%v\t%v\t%v\n",
			e["timestamp"], e["user"], e["action"], e["resource"], e["result"])
	}
	return w.Flush()
}

// runAuditExport exports audit logs to a JSON file.
func runAuditExport(cmd *cobra.Command, args []string) error {
	filename := args[0]
	limit, _ := cmd.Flags().GetInt("limit")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get(fmt.Sprintf("/api/v1/audit?limit=%d", limit))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, body)
	}

	var entries interface{}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return fmt.Errorf("decode error: %w", err)
	}

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(entries); err != nil {
		return fmt.Errorf("failed to write JSON: %w", err)
	}

	fmt.Printf("Exported audit logs to %s\n", filename)
	return nil
}
