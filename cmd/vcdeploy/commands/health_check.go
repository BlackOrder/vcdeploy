package commands

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// healthCheckCmd handles health check management commands
var healthCheckCmd = &cobra.Command{
	Use:     "health-check",
	Aliases: []string{"health-checks", "healthcheck"},
	Short:   "Health check management",
	Long: `Commands for managing health checks.

Health checks monitor the status of agents, deployments, and services.

All commands require API authentication via --master and --token flags.`,
}

func init() {
	rootCmd.AddCommand(healthCheckCmd)

	// List health checks
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all health checks",
		Long: `List all configured health checks and their status.

Example:
  vcdeploy health-check list --master localhost:9000 --token <token>`,
		RunE: runHealthCheckList,
	}
	healthCheckCmd.AddCommand(listCmd)

	// Run health check
	runCmd := &cobra.Command{
		Use:   "run [name]",
		Short: "Run a health check",
		Long: `Run a specific health check or all health checks if no name provided.

Example:
  vcdeploy health-check run --master localhost:9000 --token <token>
  vcdeploy health-check run agent-connectivity --master localhost:9000 --token <token>`,
		RunE: runHealthCheckRun,
	}
	healthCheckCmd.AddCommand(runCmd)

	// Show health check details
	showCmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show health check details",
		Long: `Show detailed information about a specific health check.

Example:
  vcdeploy health-check show agent-connectivity --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(1),
		RunE: runHealthCheckShow,
	}
	healthCheckCmd.AddCommand(showCmd)

	// Status summary
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show overall health status",
		Long: `Show an overall health status summary of the system.

Example:
  vcdeploy health-check status --master localhost:9000 --token <token>`,
		RunE: runHealthCheckStatus,
	}
	healthCheckCmd.AddCommand(statusCmd)
}

// --- Health Check Types ---

type healthCheckListResponse struct {
	HealthChecks []healthCheckInfo `json:"health_checks"`
}

type healthCheckInfo struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Target      string `json:"target"`
	Status      string `json:"status"`
	LastRun     string `json:"last_run"`
	LastSuccess string `json:"last_success,omitempty"`
	Message     string `json:"message,omitempty"`
}

type healthStatusResponse struct {
	Status     string                  `json:"status"`
	Version    string                  `json:"version"`
	Uptime     string                  `json:"uptime"`
	Components []healthComponentStatus `json:"components"`
}

type healthComponentStatus struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func runHealthCheckList(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/health-checks")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var result healthCheckListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if len(result.HealthChecks) == 0 {
		fmt.Println("No health checks configured.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tTARGET\tSTATUS\tLAST RUN\tMESSAGE")
	for _, hc := range result.HealthChecks {
		msg := hc.Message
		if len(msg) > 40 {
			msg = msg[:37] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			hc.Name, hc.Type, hc.Target, statusIcon(hc.Status), hc.LastRun, msg)
	}
	w.Flush()
	return nil
}

func runHealthCheckRun(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	endpoint := "/api/v1/health-checks/run"
	if len(args) > 0 {
		endpoint = "/api/v1/health-checks/" + args[0] + "/run"
	}

	resp, err := client.post(endpoint, nil)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var result struct {
		Results []healthCheckInfo `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tMESSAGE")
	for _, r := range result.Results {
		fmt.Fprintf(w, "%s\t%s\t%s\n", r.Name, statusIcon(r.Status), r.Message)
	}
	w.Flush()
	return nil
}

func runHealthCheckShow(cmd *cobra.Command, args []string) error {
	name := args[0]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/health-checks/" + name)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var hc healthCheckInfo
	if err := json.NewDecoder(resp.Body).Decode(&hc); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	fmt.Printf("Name:         %s\n", hc.Name)
	fmt.Printf("Type:         %s\n", hc.Type)
	fmt.Printf("Target:       %s\n", hc.Target)
	fmt.Printf("Status:       %s\n", statusIcon(hc.Status))
	fmt.Printf("Last Run:     %s\n", hc.LastRun)
	if hc.LastSuccess != "" {
		fmt.Printf("Last Success: %s\n", hc.LastSuccess)
	}
	if hc.Message != "" {
		fmt.Printf("Message:      %s\n", hc.Message)
	}
	return nil
}

func runHealthCheckStatus(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/health")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	var status healthStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	fmt.Printf("Overall Status: %s\n", statusIcon(status.Status))
	fmt.Printf("Version:        %s\n", status.Version)
	fmt.Printf("Uptime:         %s\n", status.Uptime)
	fmt.Println()

	if len(status.Components) > 0 {
		fmt.Println("Components:")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  NAME\tSTATUS\tMESSAGE")
		for _, c := range status.Components {
			fmt.Fprintf(w, "  %s\t%s\t%s\n", c.Name, statusIcon(c.Status), c.Message)
		}
		w.Flush()
	}
	return nil
}

// statusIcon returns a colored status icon
func statusIcon(status string) string {
	switch status {
	case "healthy", "ok", "pass", "success":
		return "✅ " + status
	case "degraded", "warning", "warn":
		return "⚠️  " + status
	case "unhealthy", "fail", "failed", "error":
		return "❌ " + status
	default:
		return status
	}
}
