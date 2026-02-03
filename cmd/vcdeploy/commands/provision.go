package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// provisionCmd handles agent provisioning commands
var provisionCmd = &cobra.Command{
	Use:   "provision",
	Short: "Agent provisioning",
	Long: `Commands for provisioning agents via SSH.

Agent provisioning allows you to install and configure the vcdeploy agent
on remote servers using SSH. The provisioning process:

1. Connects to the target server via SSH
2. Downloads the agent binary
3. Installs and configures the agent
4. Registers the agent with the master
5. Starts the agent service

All commands require API authentication via --master and --token flags.`,
	RunE: runProvision,
}

func init() {
	rootCmd.AddCommand(provisionCmd)

	// Main provision command flags
	provisionCmd.Flags().String("host", "", "Target host to provision (required)")
	provisionCmd.Flags().IntP("port", "p", 22, "SSH port")
	provisionCmd.Flags().StringP("user", "u", "root", "SSH user")
	provisionCmd.Flags().String("ssh-key", "", "SSH key ID to use for authentication")
	provisionCmd.Flags().String("agent-id", "", "Agent ID to assign (auto-generated if not set)")
	provisionCmd.Flags().String("agent-name", "", "Agent display name")
	provisionCmd.Flags().String("groups", "", "Comma-separated list of groups to assign")
	provisionCmd.Flags().Bool("no-start", false, "Don't start the agent after installation")
	_ = provisionCmd.MarkFlagRequired("host")

	// Status subcommand
	statusCmd := &cobra.Command{
		Use:   "status [provision-id]",
		Short: "Get provisioning job status",
		Long: `Check the status of a provisioning job.

Example:
  vcdeploy provision status abc123 --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(1),
		RunE: runProvisionStatus,
	}
	provisionCmd.AddCommand(statusCmd)

	// Logs subcommand
	logsCmd := &cobra.Command{
		Use:   "logs [provision-id]",
		Short: "Get provisioning job logs",
		Long: `View the output logs from a provisioning job.

Example:
  vcdeploy provision logs abc123 --master localhost:9000 --token <token>`,
		Args: cobra.ExactArgs(1),
		RunE: runProvisionLogs,
	}
	logsCmd.Flags().BoolP("follow", "f", false, "Follow log output (not yet implemented)")
	provisionCmd.AddCommand(logsCmd)

	// List subcommand
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List provisioning jobs",
		Long: `List recent provisioning jobs with their status.

Example:
  vcdeploy provision list --master localhost:9000 --token <token>`,
		RunE: runProvisionList,
	}
	listCmd.Flags().IntP("limit", "n", 20, "Maximum number of jobs to show")
	listCmd.Flags().String("status", "", "Filter by status (pending, running, completed, failed)")
	provisionCmd.AddCommand(listCmd)
}

// --- Provision Types ---

type provisionRequest struct {
	Host      string   `json:"host"`
	Port      int      `json:"port"`
	User      string   `json:"user"`
	SSHKeyID  string   `json:"ssh_key_id,omitempty"`
	AgentID   string   `json:"agent_id,omitempty"`
	AgentName string   `json:"agent_name,omitempty"`
	Groups    []string `json:"groups,omitempty"`
	NoStart   bool     `json:"no_start,omitempty"`
}

type provisionJobInfo struct {
	ID        string    `json:"id"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	User      string    `json:"user"`
	AgentID   string    `json:"agent_id,omitempty"`
	Status    string    `json:"status"`
	Progress  int       `json:"progress,omitempty"`
	Message   string    `json:"message,omitempty"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
	Duration  string    `json:"duration,omitempty"`
}

type provisionListResponse struct {
	Jobs []provisionJobInfo `json:"jobs"`
}

// --- Provision (start) ---

func runProvision(cmd *cobra.Command, args []string) error {
	// Check if this is a subcommand invocation
	if len(args) > 0 {
		return nil // Let subcommand handle it
	}

	host, _ := cmd.Flags().GetString("host")
	port, _ := cmd.Flags().GetInt("port")
	user, _ := cmd.Flags().GetString("user")
	sshKeyID, _ := cmd.Flags().GetString("ssh-key")
	agentID, _ := cmd.Flags().GetString("agent-id")
	agentName, _ := cmd.Flags().GetString("agent-name")
	groupsStr, _ := cmd.Flags().GetString("groups")
	noStart, _ := cmd.Flags().GetBool("no-start")

	if host == "" {
		return fmt.Errorf("--host is required")
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	req := provisionRequest{
		Host:      host,
		Port:      port,
		User:      user,
		SSHKeyID:  sshKeyID,
		AgentID:   agentID,
		AgentName: agentName,
		NoStart:   noStart,
	}

	if groupsStr != "" {
		req.Groups = strings.Split(groupsStr, ",")
		for i, g := range req.Groups {
			req.Groups[i] = strings.TrimSpace(g)
		}
	}

	body, _ := json.Marshal(req)

	fmt.Printf("Starting provisioning on %s:%d...\n", host, port)

	resp, err := client.post("/api/v1/agents/provision", jsonReader(body))
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 && resp.StatusCode != 202 {
		return handleAPIError(resp)
	}

	var job provisionJobInfo
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	fmt.Printf("Provisioning job started.\n")
	fmt.Printf("Job ID:  %s\n", job.ID)
	fmt.Printf("Status:  %s\n", job.Status)
	fmt.Printf("\nTo check status:\n")
	fmt.Printf("  vcdeploy provision status %s\n", job.ID)
	fmt.Printf("\nTo view logs:\n")
	fmt.Printf("  vcdeploy provision logs %s\n", job.ID)

	return nil
}

// --- Provision Status ---

func runProvisionStatus(cmd *cobra.Command, args []string) error {
	jobID := args[0]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/agents/provision/" + jobID + "/status")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return handleAPIError(resp)
	}

	var job provisionJobInfo
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	fmt.Printf("Job ID:     %s\n", job.ID)
	fmt.Printf("Target:     %s@%s:%d\n", job.User, job.Host, job.Port)
	fmt.Printf("Status:     %s\n", job.Status)

	if job.Progress > 0 && job.Progress < 100 {
		fmt.Printf("Progress:   %d%%\n", job.Progress)
	}

	if job.Message != "" {
		fmt.Printf("Message:    %s\n", job.Message)
	}

	if job.AgentID != "" {
		fmt.Printf("Agent ID:   %s\n", job.AgentID)
	}

	if !job.StartedAt.IsZero() {
		fmt.Printf("Started:    %s\n", job.StartedAt.Format(time.RFC3339))
	}

	if !job.EndedAt.IsZero() {
		fmt.Printf("Ended:      %s\n", job.EndedAt.Format(time.RFC3339))
		if job.Duration != "" {
			fmt.Printf("Duration:   %s\n", job.Duration)
		}
	}

	if job.Error != "" {
		fmt.Printf("\nError:\n%s\n", job.Error)
	}

	// Show status indicator
	fmt.Println()
	switch job.Status {
	case "pending":
		fmt.Println("⏳ Job is queued and waiting to start")
	case "running":
		fmt.Println("⚙️  Job is in progress")
	case "completed":
		fmt.Println("✓ Provisioning completed successfully")
	case "failed":
		fmt.Println("✗ Provisioning failed")
	}

	return nil
}

// --- Provision Logs ---

func runProvisionLogs(cmd *cobra.Command, args []string) error {
	jobID := args[0]
	follow, _ := cmd.Flags().GetBool("follow")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	resp, err := client.get("/api/v1/agents/provision/" + jobID + "/logs")
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return handleAPIError(resp)
	}

	var result struct {
		Logs []struct {
			Timestamp time.Time `json:"timestamp"`
			Level     string    `json:"level"`
			Message   string    `json:"message"`
		} `json:"logs"`
		Output string `json:"output,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	// Print structured logs if available
	if len(result.Logs) > 0 {
		for _, entry := range result.Logs {
			ts := entry.Timestamp.Format("15:04:05")
			fmt.Printf("[%s] %s: %s\n", ts, entry.Level, entry.Message)
		}
	}

	// Print raw output if available
	if result.Output != "" {
		if len(result.Logs) > 0 {
			fmt.Println("\n--- Raw Output ---")
		}
		fmt.Println(result.Output)
	}

	if follow {
		fmt.Println("\nNote: Follow mode is not yet implemented. Re-run the command to get updated logs.")
	}

	return nil
}

// --- Provision List ---

func runProvisionList(cmd *cobra.Command, args []string) error {
	limit, _ := cmd.Flags().GetInt("limit")
	statusFilter, _ := cmd.Flags().GetString("status")

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/api/v1/agents/provision?limit=%d", limit)
	if statusFilter != "" {
		path += "&status=" + statusFilter
	}

	resp, err := client.get(path)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return handleAPIError(resp)
	}

	var result provisionListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if len(result.Jobs) == 0 {
		fmt.Println("No provisioning jobs found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tHOST\tSTATUS\tAGENT ID\tSTARTED\tDURATION")

	for i := range result.Jobs {
		job := &result.Jobs[i]
		started := ""
		if !job.StartedAt.IsZero() {
			started = job.StartedAt.Format("01-02 15:04")
		}
		fmt.Fprintf(w, "%s\t%s:%d\t%s\t%s\t%s\t%s\n",
			truncate(job.ID, 12),
			job.Host,
			job.Port,
			job.Status,
			job.AgentID,
			started,
			job.Duration,
		)
	}
	w.Flush()

	return nil
}
